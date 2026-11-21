package bot

// groups.go runs Ellie Studio inside Telegram groups.
//
// Privacy mode stays on, so Telegram only delivers commands, @mentions and
// replies to the bot's own messages, and the bot answers only when it is
// addressed:
//
//	/say <text>          speak the text in the sender's own voice
//	/say (as a reply)    read the replied-to message aloud
//	/transcribe (reply)  write down a voice note
//	@bot <text>          the same as /say
//
// Everything personal (voices, style, cloning, dictionaries) stays in the
// private chat. Group messages never carry callback buttons, only a link into
// the private chat, so nobody can press someone else's menu, and the guided
// steps of the private chat (which are keyed by user) never see group text.
//
// Each person keeps their own settings and daily allowance. On top of that a
// group shares a daily allowance, every group together has one, each person
// waits a short cooldown between requests, and only a few jobs run per group
// at once.

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"ellestudio/internal/telegram"
)

// Who may add the bot to groups (Config.GroupMode).
const (
	GroupsOpen   = "open"   // anyone
	GroupsAdmins = "admins" // only the studio admins
	GroupsOff    = "off"    // nobody; the bot leaves
)

// GroupCommands is the command menu Telegram shows in groups.
var GroupCommands = []telegram.BotCommand{
	{Command: "say", Description: "Speak text in your voice, or reply to a message"},
	{Command: "transcribe", Description: "Reply to a voice note to write it down"},
	{Command: "help", Description: "How Ellie works in groups"},
}

// deepLinks are /start payloads that open a screen in the private chat.
var deepLinks = map[string]bool{
	"studio": true, "voices": true, "myvoices": true, "voice": true,
	"style": true, "clone": true, "dicts": true, "help": true,
}

type chatUser struct{ chat, user int64 }

// isGroupChat reports whether a chat ID belongs to a group (or channel):
// Telegram gives private chats positive IDs and everything else negative.
func isGroupChat(id int64) bool { return id < 0 }

// isOwner reports whether uid is one of the studio's named admins. Unlike
// isAdmin it is false for everyone when no admins are configured, so it can
// guard commands and exemptions that affect other people: blocking groups,
// adding the bot under GroupsAdmins, and skipping a group's shared limits.
func (b *Bot) isOwner(uid int64) bool { return b.cfg.AdminUsers[uid] }

// groupBacklogLimit is how far behind a group's sends may be before new
// requests there are dropped unanswered (Telegram would reject the replies).
const groupBacklogLimit = 6 * time.Second

// ---------------------------------------------------------------- keyboards

// startLink opens the private chat, on the screen named by payload.
func (b *Bot) startLink(payload string) string {
	if b.username == "" {
		return ""
	}
	link := "https://t.me/" + b.username
	if payload != "" {
		link += "?start=" + payload
	}
	return link
}

// groupKeys is the only keyboard a group message carries: a link into the
// private chat.
func (b *Bot) groupKeys(label, payload string) *telegram.InlineKeyboardMarkup {
	link := b.startLink(orDefault(payload, "studio"))
	if link == "" {
		return nil
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: orDefault(label, icStudio+" Open Ellie Studio"), URL: link}},
	}}
}

func hasCallbacks(kb *telegram.InlineKeyboardMarkup) bool {
	for _, row := range kb.InlineKeyboard {
		for _, k := range row {
			if k.CallbackData != "" {
				return true
			}
		}
	}
	return false
}

// inlineFor adapts an inline keyboard to the chat: groups get a link into
// the private chat in place of callback buttons.
func (b *Bot) inlineFor(chatID int64, kb *telegram.InlineKeyboardMarkup) *telegram.InlineKeyboardMarkup {
	if !isGroupChat(chatID) || kb == nil || !hasCallbacks(kb) {
		return kb
	}
	return b.groupKeys("", "")
}

// markupFor is inlineFor for any keyboard; groups never get the reply
// keyboard, which would cover the input box of everyone in the group.
func (b *Bot) markupFor(chatID int64, kb any) any {
	if !isGroupChat(chatID) {
		return kb
	}
	switch k := kb.(type) {
	case *telegram.InlineKeyboardMarkup:
		return b.inlineFor(chatID, k)
	case *telegram.ReplyKeyboardMarkup:
		return nil
	}
	return kb
}

// ---------------------------------------------------------------- addressing

// forMe reports whether a command's @target is this bot (or no one).
func (b *Bot) forMe(target string) bool {
	return target == "" || b.username == "" || strings.EqualFold(target, b.username)
}

// stripMention removes "@bot" from text and reports whether it was there.
func (b *Bot) stripMention(text string) (string, bool) {
	if b.username == "" {
		return text, false
	}
	tag := "@" + strings.ToLower(b.username)
	lower := strings.ToLower(text)
	for from := 0; ; {
		i := strings.Index(lower[from:], tag)
		if i < 0 {
			return text, false
		}
		i += from
		end := i + len(tag)
		if end < len(text) {
			if r, _ := utf8.DecodeRuneInString(text[end:]); r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
				from = end // part of a longer username
				continue
			}
		}
		return strings.TrimSpace(strings.TrimRight(text[:i], " ") + " " + strings.TrimLeft(text[end:], " ,:")), true
	}
}

// repliedText is the text of the message m replies to.
func repliedText(m *telegram.Message) string {
	if m.ReplyToMessage == nil {
		return ""
	}
	return strings.TrimSpace(orDefault(m.ReplyToMessage.Text, m.ReplyToMessage.Caption))
}

// ---------------------------------------------------------------- routing

// handleGroupMessage answers a group message that is addressed to the bot
// and ignores every other one.
func (b *Bot) handleGroupMessage(ctx context.Context, m *telegram.Message) {
	if m.MigrateToChatID != 0 {
		// The group became a supergroup with a new ID; carry its record over.
		if err := b.chats.Migrate(ctx, m.Chat.ID, m.MigrateToChatID); err != nil {
			b.log.Error("group migration not recorded", "from", m.Chat.ID, "to", m.MigrateToChatID, "err", err)
			return
		}
		b.log.Info("group upgraded to supergroup", "from", m.Chat.ID, "to", m.MigrateToChatID)
		return
	}
	// Anonymous admins and linked-channel posts have no person behind them
	// whose voice and allowance could be used.
	if m.From == nil || m.From.IsBot || m.SenderChat != nil {
		return
	}

	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	cmd, target, args := parseCommandTarget(text)
	if cmd != "" {
		if !b.forMe(target) {
			return
		}
	} else {
		rest, mentioned := b.stripMention(text)
		if !mentioned {
			return
		}
		cmd, args = mentionCommand(rest, m)
	}

	if ok, err := b.admitGroup(ctx, m.Chat); err != nil {
		b.log.Error("group registry unavailable", "chat", m.Chat.ID, "err", err)
		b.replyHTML(ctx, m, errorScreen(storageError{err}).text, nil)
		return
	} else if !ok {
		return
	}
	uid := m.From.ID
	if !b.allowed(uid) {
		s := noticeScreen(icLock, "Ellie Studio is private", "Only people the studio owner added can use it.")
		b.replyHTML(ctx, m, s.text, nil)
		return
	}

	switch cmd {
	case "start", "help":
		s := b.groupHelpScreen()
		b.replyHTML(ctx, m, s.text, s.kb)
	case "say", "speak":
		b.groupSpeak(ctx, m, args)
	case "transcribe":
		a, ok := audioOf(m)
		if !ok {
			a, ok = audioOf(m.ReplyToMessage)
		}
		if !ok {
			s := noticeScreen(icListen, "Transcribe", "Reply <code>/transcribe</code> to a voice note or audio file and I'll write it down.")
			b.replyHTML(ctx, m, s.text, nil)
			return
		}
		b.groupTranscribe(ctx, m, a)
	case "cancel":
		s := noticeScreen(icOK, "Nothing to cancel", "Requests here finish on their own.")
		b.replyHTML(ctx, m, s.text, nil)
	case "blockgroup":
		if !b.isOwner(uid) {
			b.privateOnly(ctx, m, cmd)
			return
		}
		_ = b.blockGroup(ctx, m.Chat.ID)
	default:
		b.privateOnly(ctx, m, cmd)
	}
}

// mentionCommand reads "@bot …" as a command: "@bot say hi", "@bot hi",
// or a bare "@bot" replying to a voice note (transcribe) or text (say).
func mentionCommand(rest string, m *telegram.Message) (cmd, args string) {
	word, tail, _ := strings.Cut(rest, " ")
	switch w := strings.ToLower(strings.TrimPrefix(word, "/")); w {
	case "say", "speak", "transcribe", "help":
		return w, strings.TrimSpace(tail)
	}
	if rest != "" {
		return "say", rest
	}
	if _, ok := audioOf(m); ok {
		return "transcribe", ""
	}
	if _, ok := audioOf(m.ReplyToMessage); ok {
		return "transcribe", ""
	}
	if repliedText(m) != "" {
		return "say", ""
	}
	return "help", ""
}

// privateOnly points a group member to the private chat for cmd.
func (b *Bot) privateOnly(ctx context.Context, m *telegram.Message, cmd string) {
	payload := "studio"
	if deepLinks[cmd] {
		payload = cmd
	}
	s := noticeScreen(icLock, "Let's do that in private",
		"Your voices, style and settings live in our private chat, so they stay yours. Tap below and I'll open it there.")
	b.replyHTML(ctx, m, s.text, b.groupKeys(icStudio+" Continue in private chat", payload))
}

func (b *Bot) groupHelpScreen() screen {
	text := header("👥", "Ellie Studio in groups", "") +
		"• <code>/say your text</code>: I'll say it in <b>your</b> voice\n" +
		"• Reply <code>/say</code> to a message: I'll read it aloud\n" +
		"• Reply <code>/transcribe</code> to a voice note: I'll write it down\n"
	if b.username != "" {
		text += "• Or mention me: <code>@" + html.EscapeString(b.username) + " your text</code>\n"
	}
	text += "\n🎛 Everyone picks their own voice and style in our private chat, and keeps their own daily allowance."
	if l := b.cfg.GroupLimits; l.Chars > 0 {
		text += "\n🎟 This group shares <b>" + fmtInt(l.Chars) + "</b> spoken characters a day."
	}
	return screen{text, b.groupKeys("", "")}
}

// ---------------------------------------------------------------- jobs

func (b *Bot) groupSpeak(ctx context.Context, m *telegram.Message, args string) {
	text := args
	if text == "" {
		text = repliedText(m)
	}
	if text == "" {
		s := noticeScreen(icStudio, "Say something", "Send <code>/say your text</code>, or reply <code>/say</code> to a message and I'll read it aloud.")
		b.replyHTML(ctx, m, s.text, nil)
		return
	}
	done, ok := b.groupTurn(ctx, m)
	if !ok {
		return
	}
	defer done()
	b.speak(ctx, m, text)
}

func (b *Bot) groupTranscribe(ctx context.Context, m *telegram.Message, a audioRef) {
	done, ok := b.groupTurn(ctx, m)
	if !ok {
		return
	}
	defer done()
	b.transcribe(ctx, m, a)
}

// groupTurn applies the per-user cooldown and the per-group job limit. On
// success the caller must call done when the job ends.
func (b *Bot) groupTurn(ctx context.Context, m *telegram.Message) (done func(), ok bool) {
	chat, uid := m.Chat.ID, m.From.ID
	key := chatUser{chat, uid}
	now := time.Now()
	exempt := b.isOwner(uid)
	if !exempt && b.tg.Backlog(chat) > groupBacklogLimit {
		// The group is flooding; don't start work whose answer can't be sent.
		b.log.Warn("group request dropped, send backlog full", "chat", chat)
		return nil, false
	}

	b.groupMu.Lock()
	if cd := b.cfg.GroupCooldown; !exempt && cd > 0 {
		if since := now.Sub(b.lastUse[key]); since < cd {
			notify := now.Sub(b.lastNotice[key]) >= cd // one reminder per cooldown, not one per message
			if notify {
				b.lastNotice[key] = now
			}
			b.groupMu.Unlock()
			if notify {
				wait := int((cd - since + time.Second - 1) / time.Second)
				s := noticeScreen(icWait, "One moment", fmt.Sprintf("You can send your next request in <b>%ds</b>.", wait))
				b.replyHTML(ctx, m, s.text, nil)
			}
			return nil, false
		}
	}
	if n := b.cfg.GroupConcurrency; n > 0 && b.groupJobs[chat] >= n {
		b.groupMu.Unlock()
		s := noticeScreen(icWait, "I'm a bit busy here", "I'm already working on a few requests in this group. Try again in a moment.")
		b.replyHTML(ctx, m, s.text, nil)
		return nil, false
	}
	b.groupJobs[chat]++
	b.lastUse[key] = now
	if len(b.lastUse) > 20000 { // forget cooldowns that have long passed
		for k, t := range b.lastUse {
			if now.Sub(t) > b.cfg.GroupCooldown {
				delete(b.lastUse, k)
				delete(b.lastNotice, k)
			}
		}
	}
	b.groupMu.Unlock()

	return func() {
		b.groupMu.Lock()
		defer b.groupMu.Unlock()
		if b.groupJobs[chat]--; b.groupJobs[chat] <= 0 {
			delete(b.groupJobs, chat)
		}
	}, true
}

// ---------------------------------------------------------------- shared allowance

// spendGroup reserves chars and secs from the group's shared allowance (and
// the all-groups total). On refusal it tells the group why and returns false.
// Private chats and named admins are not counted.
func (b *Bot) spendGroup(ctx context.Context, m *telegram.Message, chars, secs int) bool {
	if !isGroupChat(m.Chat.ID) || b.isOwner(m.From.ID) {
		return true
	}
	err := b.chats.Spend(ctx, m.Chat.ID, today(), chars, secs, b.cfg.GroupLimits)
	if err == nil {
		return true
	}
	var s screen
	switch {
	case errors.Is(err, errGroupLimit):
		l := b.cfg.GroupLimits
		what := fmtInt(l.Chars) + " spoken characters"
		if secs > 0 {
			what = fmtMinutes(l.Secs) + " of transcription"
		}
		s = noticeScreen("🎟", "This group's daily allowance is used up",
			"Everyone here shares <b>"+what+"</b> a day. It refreshes in <b>"+fmtWait(untilReset())+
				"</b> (midnight UTC). You can still use Ellie in our private chat.")
	case errors.Is(err, errGroupsLimit):
		s = noticeScreen("🎟", "Ellie is fully booked in groups today",
			"Groups have used today's shared allowance. It refreshes in <b>"+fmtWait(untilReset())+
				"</b> (midnight UTC). You can still use Ellie in our private chat.")
	default:
		b.log.Error("group allowance unavailable", "chat", m.Chat.ID, "err", err)
		s = errorScreen(storageError{err})
	}
	b.replyHTML(ctx, m, s.text, b.groupKeys("", ""))
	return false
}

// refundGroup returns usage to a group's allowance after a failed job.
func (b *Bot) refundGroup(m *telegram.Message, chars, secs int) {
	if !isGroupChat(m.Chat.ID) || b.isOwner(m.From.ID) || (chars == 0 && secs == 0) {
		return
	}
	// The job may have failed because ctx ended; the refund must still land.
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	if err := b.chats.Refund(ctx, m.Chat.ID, today(), chars, secs); err != nil {
		b.log.Warn("group refund failed", "chat", m.Chat.ID, "err", err)
	}
}

// refundAll returns usage to the user and, in a group, to the group.
func (b *Bot) refundAll(m *telegram.Message, chars, secs int) {
	b.refund(m.From.ID, chars, secs)
	b.refundGroup(m, chars, secs)
}

// settleGroup corrects a group's audio reservation to the real length.
func (b *Bot) settleGroup(m *telegram.Message, claimed int, actual float64) {
	if actual <= 0 || !isGroupChat(m.Chat.ID) || b.isOwner(m.From.ID) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	var err error
	switch diff := int(actual+0.999) - claimed; {
	case diff > 0: // the work is done, so record it without a cap
		err = b.chats.Spend(ctx, m.Chat.ID, today(), 0, diff, GroupLimits{})
	case diff < 0:
		err = b.chats.Refund(ctx, m.Chat.ID, today(), 0, -diff)
	}
	if err != nil {
		b.log.Warn("group usage not settled", "chat", m.Chat.ID, "err", err)
	}
}

// ---------------------------------------------------------------- membership

// admitGroup checks that the bot may work in a group, recording groups it
// meets for the first time. It leaves blocked groups, and groups it may not
// be in under the current GroupMode.
func (b *Bot) admitGroup(ctx context.Context, c telegram.Chat) (bool, error) {
	info, found, err := b.chats.Get(ctx, c.ID)
	if err != nil {
		return false, err
	}
	switch {
	case found && info.Blocked:
		b.leaveGroup(ctx, c.ID, "")
		return false, nil
	case b.cfg.GroupMode == GroupsOff:
		b.leaveGroup(ctx, c.ID, groupsOffText)
		return false, nil
	case found && info.Status == chatMember:
		return true, nil
	case b.cfg.GroupMode == GroupsAdmins:
		// Someone other than an admin added the bot, or it joined before
		// the registry existed; either way no admin vouched for this group.
		b.leaveGroup(ctx, c.ID, groupsAdminsText)
		return false, nil
	}
	if err := b.chats.Upsert(ctx, ChatInfo{ID: c.ID, Type: c.Type, Title: c.Title, Status: chatMember}); err != nil {
		return false, err
	}
	b.log.Info("group registered", "chat", c.ID)
	return true, nil
}

const (
	groupsOffText    = "Ellie Studio isn't available in groups right now. You can still use it in a private chat."
	groupsAdminsText = "Only the studio owner can add Ellie Studio to groups. You can still use it in a private chat."
)

// onMembership reacts to the bot being added to or removed from a chat.
func (b *Bot) onMembership(ctx context.Context, u *telegram.ChatMemberUpdated) {
	c := u.Chat
	if c.Type == "channel" {
		if u.NewChatMember.Present() {
			b.leaveGroup(ctx, c.ID, "") // channels have no one to speak for
		}
		return
	}
	if !c.IsGroup() {
		return // a private chat: the user blocked or unblocked the bot
	}
	if !u.NewChatMember.Present() {
		if err := b.chats.Upsert(ctx, ChatInfo{ID: c.ID, Type: c.Type, Title: c.Title, Status: chatLeft}); err != nil {
			b.log.Warn("group status not saved", "chat", c.ID, "err", err)
		}
		b.log.Info("removed from group", "chat", c.ID)
		return
	}
	if u.OldChatMember.Present() {
		return // promoted or restricted, still in the group
	}

	info, found, err := b.chats.Get(ctx, c.ID)
	if err != nil {
		b.log.Error("group registry unavailable", "chat", c.ID, "err", err)
	}
	switch {
	case found && info.Blocked:
		b.leaveGroup(ctx, c.ID, "")
		return
	case b.cfg.GroupMode == GroupsOff:
		b.leaveGroup(ctx, c.ID, groupsOffText)
		return
	case b.cfg.GroupMode == GroupsAdmins && !b.isOwner(u.From.ID):
		b.leaveGroup(ctx, c.ID, groupsAdminsText)
		return
	}
	if err := b.chats.Upsert(ctx, ChatInfo{ID: c.ID, Type: c.Type, Title: c.Title, Status: chatMember, AddedBy: u.From.ID}); err != nil {
		b.log.Error("group not registered", "chat", c.ID, "err", err)
	}
	b.log.Info("added to group", "chat", c.ID, "by", u.From.ID)
	s := b.groupHelpScreen()
	s.text = "👋 <b>Hi everyone, I'm Ellie!</b> I turn text into lifelike speech.\n\n" + s.text
	b.sendScreen(ctx, c.ID, 0, s)
}

// leaveGroup leaves a group, first posting text if there is any.
func (b *Bot) leaveGroup(ctx context.Context, chatID int64, text string) {
	if text != "" {
		b.sendScreen(ctx, chatID, 0, screen{icLock + " " + html.EscapeString(text), nil})
	}
	if err := b.tg.LeaveChat(ctx, chatID); err != nil {
		b.log.Warn("leaveChat failed", "chat", chatID, "err", err)
	}
	if err := b.chats.Upsert(ctx, ChatInfo{ID: chatID, Status: chatLeft}); err != nil {
		b.log.Warn("group status not saved", "chat", chatID, "err", err)
	}
	b.log.Info("left group", "chat", chatID)
}

// blockGroup stops the bot working in a group and leaves it.
func (b *Bot) blockGroup(ctx context.Context, chatID int64) error {
	if err := b.chats.SetBlocked(ctx, chatID, true); err != nil {
		return err
	}
	b.leaveGroup(ctx, chatID, "")
	return nil
}

// ---------------------------------------------------------------- admin screens

// groupsScreen lists today's busiest groups for the studio owner.
func (b *Bot) groupsScreen(ctx context.Context) screen {
	nav := (&keys{}).nav("", "").build()
	day := today()
	top, err := b.chats.Busiest(ctx, day, 10)
	if err != nil {
		return errorScreen(storageError{err})
	}
	total, _, err := b.chats.Get(ctx, allGroups)
	if err != nil {
		return errorScreen(storageError{err})
	}
	tc, ts := total.usageOn(day)
	var sb strings.Builder
	sb.WriteString(header("👥", "Groups today", "mode: "+orDefault(b.cfg.GroupMode, GroupsOpen)))
	l := b.cfg.GroupLimits
	fmt.Fprintf(&sb, "All groups: <b>%s</b> chars · <b>%s</b> transcribed\n", fmtInt(tc), fmtMinutes(ts))
	fmt.Fprintf(&sb, "<i>Caps: %s chars and %s per group, %s chars and %s in total (0 = none)</i>\n\n",
		fmtInt(l.Chars), fmtMinutes(l.Secs), fmtInt(l.AllChars), fmtMinutes(l.AllSecs))
	if len(top) == 0 {
		sb.WriteString("No group has used Ellie today.")
	}
	for _, c := range top {
		flag := ""
		if c.Blocked {
			flag = " 🚫"
		}
		fmt.Fprintf(&sb, "• <b>%s</b>%s\n  <code>%d</code> · %s chars · %s\n",
			html.EscapeString(orDefault(c.Title, "Untitled group")), flag, c.ID, fmtInt(c.Chars), fmtMinutes(c.Secs))
	}
	sb.WriteString("\n<i>/blockgroup &lt;id&gt; stops a group; /unblockgroup &lt;id&gt; lets it back.</i>")
	return screen{sb.String(), nav}
}

// cmdBlockGroup handles /blockgroup and /unblockgroup in the private chat.
func (b *Bot) cmdBlockGroup(ctx context.Context, m *telegram.Message, block bool, args string) {
	id, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil || !isGroupChat(id) {
		s := noticeScreen(icWarn, "Which group?", "Send the group's ID from /groups, like <code>/blockgroup -1001234567890</code>.")
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	if block {
		err = b.blockGroup(ctx, id)
	} else {
		err = b.chats.SetBlocked(ctx, id, false)
	}
	if err != nil {
		b.replyHTML(ctx, m, errorScreen(storageError{err}).text, nil)
		return
	}
	verb := "unblocked. It can add Ellie again."
	if block {
		verb = "blocked. Ellie left it and won't rejoin."
	}
	s := noticeScreen(icOK, "Done", fmt.Sprintf("Group <code>%d</code> is %s", id, verb))
	b.replyHTML(ctx, m, s.text, s.kb)
}

// openDeepLink shows the screen a t.me/<bot>?start=<payload> link names.
func (b *Bot) openDeepLink(ctx context.Context, m *telegram.Message, payload string) {
	uid, chat := m.From.ID, m.Chat.ID
	switch payload {
	case "voices":
		b.openVoices(ctx, chat, uid, "", nil)
	case "myvoices":
		owned := true
		b.openVoices(ctx, chat, uid, "", &owned)
	case "voice":
		b.sendScreen(ctx, chat, 0, b.voiceCard(ctx, uid, ""))
	case "style":
		b.sendScreen(ctx, chat, 0, b.styleScreen(uid))
	case "clone":
		b.startFlow(ctx, chat, uid, flowCloneClip)
	case "dicts":
		b.sendScreen(ctx, chat, 0, b.dictsScreen(ctx, uid))
	case "help":
		b.sendScreen(ctx, chat, 0, helpScreen(""))
	default:
		b.sendScreen(ctx, chat, 0, b.homeScreen(uid))
	}
}
