package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ellestudio/internal/telegram"
)

// Guest mode: someone mentions @bot in a chat the bot was never added to.
// Telegram shows the bot only that message (and the one it replies to), and
// the bot gets exactly one reply, sent with answerGuestQuery. Everything the
// bot says there, voice or notice, is that one reply, so there are no
// buttons that call back into the bot: only a link to the private chat.

// guestMaxChars bounds one guest reading. The reply must arrive while the
// query is still open, so long texts are refused rather than risked.
const guestMaxChars = 500

// guestDeadline bounds the whole reading: waiting for Cartesia, synthesis,
// upload and reply.
const guestDeadline = 30 * time.Second

// guestStatus marks chats known only from guest mentions.
const guestStatus = "guest"

// GUEST_ACCESS values: who may use the bot through guest mentions.
const (
	GuestMembers  = "members"  // people who have used the private chat (default)
	GuestEveryone = "everyone" // anyone
)

// join records that uid uses the private chat, which admits them to guest mode.
func (b *Bot) join(uid int64) {
	if err := b.store.Join(uid, today()); err != nil {
		b.log.Warn("save joined", "user", uid, "err", err)
	}
}

func (b *Bot) handleGuest(ctx context.Context, m *telegram.Message) {
	if m.GuestQueryID == "" || m.From == nil || m.From.IsBot {
		return
	}
	uid, chat := m.From.ID, m.Chat.ID
	info, known, err := b.chats.Get(ctx, chat)
	if err != nil {
		b.log.Warn("guest chat lookup", "chat", chat, "err", err)
	}
	if info.Blocked {
		b.log.Info("guest mention ignored, chat blocked", "chat", chat, "user", uid)
		return // a blocked chat gets no reply at all
	}

	text, mentioned := b.stripMention(orDefault(m.Text, m.Caption))
	// Without a mention, a guest query can only come from a reply to Ellie.
	// Text there ("@friend", "nice voice") is chat, not a request, even if
	// the replied message doesn't say who sent it.
	if r := m.ReplyToMessage; !mentioned && r != nil && (text != "" || b.isOwnMessage(r)) {
		// Telegram also sends a guest query when someone merely replies to
		// one of Ellie's messages: people talking about a voice, or tagging
		// a friend to listen. That isn't a request, so stay silent.
		b.log.Info("guest reply to Ellie ignored, no mention", "chat", chat, "user", uid)
		return
	}
	// Links, @usernames and emoji are never read; "@EllieStudioBot @friend"
	// in a reply reads the replied message, not the names.
	text = speakable(text)
	if text == "" {
		replied, _ := b.stripMention(repliedText(m))
		text = speakable(replied)
	}
	// A group that once had Ellie keeps its own settings: guest mentions
	// obey them just like /say would.
	gs := info.Settings
	if s, refused := b.guestRefusal(ctx, m, text, gs); refused {
		b.answerGuestText(ctx, m, s)
		return
	}
	s := b.store.Get(uid)
	if s.VoiceID == "" {
		b.answerGuestText(ctx, m, noticeScreen(icVoices, "Pick a voice first",
			"Open our private chat, choose who should speak, then mention me again."))
		return
	}

	if !b.begin(uid) {
		b.answerGuestText(ctx, m, noticeScreen(icWait, "One moment",
			"I'm still working on your last request. Mention me again once it arrives."))
		return
	}
	defer b.end(uid)
	done, wait, ok := b.guestTurn(chat, uid)
	if !ok {
		body := "I'm already working on a few requests in this chat. Try again in a moment."
		if wait > 0 {
			body = fmt.Sprintf("You can send your next request in <b>%ds</b>.", int((wait+time.Second-1)/time.Second))
		}
		b.answerGuestText(ctx, m, noticeScreen(icWait, "One moment", body))
		return
	}
	defer done()
	chars := utf8.RuneCountInString(text)
	if ok, u := b.spendChars(uid, chars); !ok {
		if u.Studio {
			b.answerGuestText(ctx, m, studioFullScreen())
			return
		}
		b.answerGuestText(ctx, m, noticeScreen("🎟", "Your daily allowance is used up",
			"You can read more in <b>"+fmtWait(untilReset())+"</b> (midnight UTC)."))
		return
	}
	if !known {
		if err := b.chats.Upsert(ctx, ChatInfo{ID: chat, Type: m.Chat.Type, Title: m.Chat.Title, Status: guestStatus}); err != nil {
			b.log.Warn("record guest chat", "chat", chat, "err", err)
		}
	}
	charged, refusal, ok := b.spendGuestChat(ctx, m, chars, gs)
	if !ok {
		b.refund(uid, chars, 0)
		b.answerGuestText(ctx, m, refusal)
		return
	}
	undo := func() {
		b.refund(uid, chars, 0)
		if charged {
			if err := b.chats.Refund(context.WithoutCancel(ctx), chat, today(), chars, 0); err != nil {
				b.log.Warn("refund guest chat", "chat", chat, "err", err)
			}
		}
	}

	// A guest query does not wait forever: give up (and refund) rather than
	// queue behind long jobs and answer after it has expired.
	ctx, cancel := context.WithTimeout(ctx, guestDeadline)
	defer cancel()
	if err := b.acquire(ctx, nil); err != nil {
		undo()
		b.log.Warn("guest reply could not start", "user", uid, "reason", err, "queued", b.jobs.queued())
		if ctx.Err() == nil {
			b.answerGuestText(ctx, m, busyScreen(err))
		}
		return
	}
	audio, err := b.ttsWithRetry(ctx, ttsRequest(s, text))
	b.release(err)
	if err != nil {
		undo()
		b.log.Error("guest tts", "user", uid, "err", err)
		b.answerGuestText(ctx, m, errorScreen(err))
		return
	}
	if err := b.answerGuestVoice(ctx, m, s, text, audio); err != nil {
		undo()
		b.log.Error("guest reply", "user", uid, "chat", chat, "err", err)
		return
	}
	b.log.Info("guest reply sent", "chat", chat, "chat_type", m.Chat.Type, "user", uid, "chars", chars)
	b.remember(uid, func(r *recent) { r.spoken = text })
	b.store.Spoke(uid, chars)
	b.track(ctx, uid, evReading, srcGuest, chars)
}

// guestLimit is the longest text a guest mention may ask for.
func (b *Bot) guestLimit(gs GroupSettings) int {
	return effCap(min(guestMaxChars, b.cfg.MaxTTSChars), gs.MaxChars)
}

// guestTurn applies the group cooldown and per-chat job limit to a guest
// mention. When refused, wait is the cooldown left (0 means the chat is
// busy). On success the caller must call done when the job ends.
func (b *Bot) guestTurn(chat, uid int64) (done func(), wait time.Duration, ok bool) {
	if b.isOwner(uid) {
		return func() {}, 0, true
	}
	key, now := chatUser{chat, uid}, time.Now()
	b.groupMu.Lock()
	defer b.groupMu.Unlock()
	if cd := b.cfg.GroupCooldown; cd > 0 {
		if since := now.Sub(b.lastUse[key]); since < cd {
			return nil, cd - since, false
		}
	}
	if n := b.cfg.GroupConcurrency; n > 0 && b.groupJobs[chat] >= n {
		return nil, 0, false
	}
	b.groupJobs[chat]++
	b.lastUse[key] = now
	return func() {
		b.groupMu.Lock()
		defer b.groupMu.Unlock()
		if b.groupJobs[chat]--; b.groupJobs[chat] <= 0 {
			delete(b.groupJobs, chat)
		}
	}, 0, true
}

// guestRefusal is the notice for a mention Ellie won't read, if any.
func (b *Bot) guestRefusal(ctx context.Context, m *telegram.Message, text string, gs GroupSettings) (screen, bool) {
	uid := m.From.ID
	switch {
	case !b.allowed(uid):
		return noticeScreen(icWarn, "Ellie is private", "This studio is open to invited people only."), true
	case b.cfg.GuestAccess != GuestEveryone && !b.isOwner(uid) && !b.store.Get(uid).Joined:
		b.track(ctx, uid, evGuestInvite, "", 0)
		return b.guestInviteScreen(), true
	case b.cfg.GroupMode == GroupsOff, b.cfg.GroupMode == GroupsAdmins && !b.isOwner(uid):
		return noticeScreen(icWarn, "Not available here", "Ellie only works in our private chat right now."), true
	case b.cfg.GuestStorageChat == 0:
		return noticeScreen(icWarn, "Not set up yet", "Ellie can't reply in other chats yet. Try our private chat."), true
	case text == "":
		return noticeScreen(icStudio, "Ellie Studio",
			"Mention me with something to read, like <code>@"+b.username+" good morning everyone</code>, "+
				"or mention me in a reply to a message and I'll read that out."), true
	case gs.Paused && !b.isOwner(uid):
		return noticeScreen(icLock, "Ellie is paused here",
			"A group admin paused Ellie in this group. You can still use it in our private chat."), true
	case gs.NoSpeak && !b.isOwner(uid):
		return noticeScreen(icLock, "Speaking is off here",
			"This group's admins turned off reading aloud. You can still use it in our private chat."), true
	}
	if gs.AdminsOnly {
		// isGroupAdmin treats studio admins as admins. If the check fails
		// (the bot may not be able to see members here), refuse.
		admin, err := b.isGroupAdmin(ctx, m.Chat.ID, uid)
		if err != nil {
			b.log.Warn("guest admin check failed", "chat", m.Chat.ID, "err", err)
		}
		if !admin {
			return noticeScreen(icLock, "Only admins can use Ellie here",
				"This group's admins have kept Ellie to themselves. You can still use it in our private chat."), true
		}
	}
	if n, limit := utf8.RuneCountInString(text), b.guestLimit(gs); n > limit {
		return noticeScreen(icWarn, "That's a lot of text", fmt.Sprintf(
			"It's <b>%d</b> characters. Here I can read up to <b>%d</b> at a time; our private chat takes longer texts.",
			n, limit)), true
	}
	return screen{}, false
}

// spendGuestChat charges the chat's shared allowance and the all-groups
// total, the same caps a group that added Ellie has. charged reports whether
// anything was taken (admins are exempt).
func (b *Bot) spendGuestChat(ctx context.Context, m *telegram.Message, chars int, gs GroupSettings) (charged bool, refusal screen, ok bool) {
	if b.isOwner(m.From.ID) {
		return false, screen{}, true
	}
	l := b.cfg.GroupLimits
	l.Chars = effCap(l.Chars, gs.DailyChars)
	err := b.chats.Spend(ctx, m.Chat.ID, today(), chars, 0, l)
	switch {
	case err == nil:
		return true, screen{}, true
	case errors.Is(err, errGroupLimit):
		return false, noticeScreen("🎟", "This chat's daily allowance is used up",
			"Everyone here shares <b>"+fmtInt(l.Chars)+" spoken characters</b> a day. "+
				"It refreshes in <b>"+fmtWait(untilReset())+"</b> (midnight UTC)."), false
	case errors.Is(err, errGroupsLimit):
		return false, noticeScreen("🎟", "Ellie is fully booked today",
			"Shared chats have used today's allowance. It refreshes in <b>"+fmtWait(untilReset())+
				"</b> (midnight UTC). You can still use Ellie in our private chat."), false
	default:
		b.log.Error("guest chat allowance unavailable", "chat", m.Chat.ID, "err", err)
		return false, errorScreen(storageError{err}), false
	}
}

// isOwnMessage reports whether r is a message Ellie posted (as a guest, via
// inline, or as a member).
func (b *Bot) isOwnMessage(r *telegram.Message) bool {
	if r == nil {
		return false
	}
	me := func(u *telegram.User) bool {
		return u != nil && u.IsBot && b.username != "" && strings.EqualFold(u.Username, b.username)
	}
	// Only Ellie's own guest messages reach Ellie as replies, so any guest
	// bot message here is Ellie's.
	return r.GuestBotCallerUser != nil || me(r.From) || me(r.ViaBot)
}

// firstLine is a notice's title as plain text, for the logs.
func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return html.UnescapeString(htmlTagRe.ReplaceAllString(s, ""))
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// guestKeys links to the private chat, the only button a guest reply
// carries. The "g" start payload counts who joins from guest mode.
func (b *Bot) guestKeys(label string) *telegram.InlineKeyboardMarkup {
	link := b.startLink("g")
	if link == "" {
		return nil
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: label, URL: link}},
	}}
}

// answerGuestText replies to a guest mention with a notice.
func (b *Bot) answerGuestText(ctx context.Context, m *telegram.Message, s screen) {
	res := telegram.InlineQueryResultArticle{
		Type: "article", ID: "ellie-notice", Title: "Ellie Studio",
		InputMessageContent: telegram.InputTextMessageContent{MessageText: strings.TrimSpace(s.text), ParseMode: "HTML"},
		ReplyMarkup:         b.guestKeys("🎁 Start Ellie, it's free"),
	}
	if _, err := b.tg.AnswerGuestQuery(ctx, m.GuestQueryID, res); err != nil {
		b.log.Warn("guest notice", "chat", m.Chat.ID, "err", err)
		return
	}
	b.log.Info("guest notice sent", "chat", m.Chat.ID, "chat_type", m.Chat.Type, "user", m.From.ID, "notice", firstLine(s.text))
}

// answerGuestVoice uploads the reading to the storage chat to get a file id,
// replies with it, and deletes the upload again.
func (b *Bot) answerGuestVoice(ctx context.Context, m *telegram.Message, s Settings, text string, audio []byte) error {
	up, err := b.tg.SendVoice(ctx, telegram.MediaParams{
		ChatID: b.cfg.GuestStorageChat, Filename: "ellie-studio.mp3", Data: audio, Silent: true,
	})
	if err != nil {
		return fmt.Errorf("upload to storage chat: %w", err)
	}
	defer func() {
		if err := b.tg.DeleteMessage(context.WithoutCancel(ctx), b.cfg.GuestStorageChat, up.MessageID); err != nil {
			b.log.Warn("delete guest upload", "err", err)
		}
	}()
	if up.Voice == nil || up.Voice.FileID == "" {
		return errors.New("upload to storage chat: no voice file in reply")
	}
	_, err = b.tg.AnswerGuestQuery(ctx, m.GuestQueryID, telegram.InlineQueryResultCachedVoice{
		Type: "voice", ID: "ellie-voice", VoiceFileID: up.Voice.FileID,
		Title: truncate(text, 60), Caption: speechCaption(s),
		// Everyone in the chat hears it; the button lets them make their own.
		ReplyMarkup: b.guestKeys("🎙 Make your own voice notes"),
	})
	return err
}
