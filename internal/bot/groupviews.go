package bot

// groupviews.go gives each group member a private view of their own settings
// inside the group, using Telegram's ephemeral messages (Bot API 10.2+):
// messages only one member and the bot can see.
//
// Telegram lets a bot that isn't an admin send one only within 15 seconds of
// that member's own action, answering either
//
//   - an ephemeral command (/voices, /style, /help, /groupsettings, marked
//     is_ephemeral in the group command menu), or
//   - a button press, such as "🎛 My voice & style" under a group message.
//
// Buttons inside a private view act only for the member who sees it, so the
// view can be the same voice picker and style screens as the private chat.
// Anything beyond picking a voice and style (cloning, renaming, deleting,
// dictionaries) still opens the private chat.
//
// The group's own admins also get /groupsettings: pause Ellie, keep it to
// admins, turn speaking or transcribing off, and lower the group's daily
// allowance or the longest message. They can never raise the studio's limits.

import (
	"context"
	"fmt"
	"html"
	"mime"
	"strings"
	"time"

	"ellestudio/internal/telegram"
)

// ---------------------------------------------------------------- group settings in context

type groupSettingsKey struct{}

func withGroupSettings(ctx context.Context, gs GroupSettings) context.Context {
	return context.WithValue(ctx, groupSettingsKey{}, gs)
}

func groupSettingsOf(ctx context.Context) GroupSettings {
	gs, _ := ctx.Value(groupSettingsKey{}).(GroupSettings)
	return gs
}

// effCap is the tighter of the studio's limit and a group's own (0 = none).
func effCap(studio, group int) int {
	if group > 0 && (studio == 0 || group < studio) {
		return group
	}
	return studio
}

// groupLimits are the caps for the request in ctx: the studio's, lowered by
// the group's own daily allowance.
func (b *Bot) groupLimits(ctx context.Context) GroupLimits {
	l := b.lim().GroupLimits
	l.Chars = effCap(l.Chars, groupSettingsOf(ctx).DailyChars)
	return l
}

// maxChars is the longest text one request may speak.
func (b *Bot) maxChars(ctx context.Context, m *telegram.Message) int {
	if !isGroupChat(m.Chat.ID) {
		return b.lim().MaxTTSChars
	}
	return effCap(b.lim().MaxTTSChars, groupSettingsOf(ctx).MaxChars)
}

// groupMayRun applies a group's own settings to a /say or /transcribe.
func (b *Bot) groupMayRun(ctx context.Context, m *telegram.Message, cmd string, gs GroupSettings) bool {
	if cmd != "say" && cmd != "speak" && cmd != "transcribe" {
		return true
	}
	chat, uid := m.Chat.ID, m.From.ID
	refuse := func(kind, title, body string) bool {
		if b.noticeDue(chat, uid, kind, 2*time.Minute) {
			b.groupNotice(ctx, m, noticeScreen(icLock, title, body), nil)
		}
		return false
	}
	switch {
	case gs.Paused:
		return refuse("paused", "Ellie is paused here",
			"A group admin paused Ellie in this group. They can turn it back on with /groupsettings.")
	case gs.NoSpeak && cmd != "transcribe":
		return refuse("nospeak", "Speaking is off here", "This group's admins turned off /say. You can still use it in our private chat.")
	case gs.NoTranscribe && cmd == "transcribe":
		return refuse("notranscribe", "Transcribing is off here", "This group's admins turned off /transcribe. You can still use it in our private chat.")
	case gs.AdminsOnly:
		admin, err := b.isGroupAdmin(ctx, chat, uid)
		if err != nil {
			b.log.Warn("group admin check failed", "chat", chat, "err", err)
			return refuse("adminsonly", "Try again in a moment", "Only this group's admins can use Ellie here, and I couldn't check who they are just now.")
		}
		if !admin {
			return refuse("adminsonly", "Only admins can use Ellie here",
				"This group's admins have kept Ellie to themselves. You can still use it in our private chat.")
		}
	}
	return true
}

// ---------------------------------------------------------------- group admins

type adminEntry struct {
	admin bool
	until time.Time
}

// adminTTL is how long a "who runs this group" answer is trusted.
const adminTTL = 2 * time.Minute

// isGroupAdmin reports whether uid runs the group (or is a studio admin).
func (b *Bot) isGroupAdmin(ctx context.Context, chat, uid int64) (bool, error) {
	if b.isOwner(uid) {
		return true, nil
	}
	key := chatUser{chat, uid}
	now := time.Now()
	b.groupMu.Lock()
	if e, ok := b.adminCache[key]; ok && now.Before(e.until) {
		b.groupMu.Unlock()
		return e.admin, nil
	}
	b.groupMu.Unlock()

	cm, err := b.tg.GetChatMember(ctx, chat, uid)
	if err != nil {
		return false, err
	}
	b.groupMu.Lock()
	defer b.groupMu.Unlock()
	b.adminCache[key] = adminEntry{cm.IsAdmin(), now.Add(adminTTL)}
	if len(b.adminCache) > 20000 {
		for k, e := range b.adminCache {
			if now.After(e.until) {
				delete(b.adminCache, k)
			}
		}
	}
	return cm.IsAdmin(), nil
}

// ---------------------------------------------------------------- public notices

type noticeKey struct {
	chat, user int64
	kind       string
}

// noticeDue reports whether a member may be told about kind again, so
// repeated requests get one reminder instead of one reply each.
func (b *Bot) noticeDue(chat, uid int64, kind string, every time.Duration) bool {
	if every <= 0 {
		every = time.Minute
	}
	now := time.Now()
	key := noticeKey{chat, uid, kind}
	b.groupMu.Lock()
	defer b.groupMu.Unlock()
	if now.Sub(b.noticeAt[key]) < every {
		return false
	}
	b.noticeAt[key] = now
	if len(b.noticeAt) > 20000 {
		for k, t := range b.noticeAt {
			if now.Sub(t) > 10*time.Minute {
				delete(b.noticeAt, k)
			}
		}
	}
	return true
}

// groupNotice posts a short-lived reply in a group (hints, cooldowns,
// refusals) and deletes it after GroupNoticeTTL so it doesn't clutter the
// chat. kb is sent as given (use groupKeys).
func (b *Bot) groupNotice(ctx context.Context, m *telegram.Message, s screen, kb *telegram.InlineKeyboardMarkup) {
	msg, err := b.tg.SendMessage(ctx, telegram.SendMessageParams{
		ChatID: m.Chat.ID, ThreadID: m.Thread(), Text: s.text, ParseMode: "HTML", ReplyTo: m.MessageID,
		ReplyMarkup: b.inlineFor(m.Chat.ID, kb), NoPreview: true,
	})
	if err != nil {
		b.log.Warn("sendMessage failed", "chat", m.Chat.ID, "err", err)
		return
	}
	b.expire(m.Chat.ID, msg.MessageID)
}

// expire deletes a bot message after GroupNoticeTTL.
func (b *Bot) expire(chat, id int64) {
	ttl := b.cfg.GroupNoticeTTL
	if ttl <= 0 || id == 0 {
		return
	}
	time.AfterFunc(ttl, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := b.tg.DeleteMessage(ctx, chat, id); err != nil {
			b.log.Debug("notice not deleted", "chat", chat, "err", err)
		}
	})
}

// replyNotice answers m with s: as is in a private chat, as a short-lived
// notice in a group, where its buttons become the "My voice & style" menu.
func (b *Bot) replyNotice(ctx context.Context, m *telegram.Message, s screen) {
	if !isGroupChat(m.Chat.ID) {
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	var kb *telegram.InlineKeyboardMarkup
	if s.kb != nil {
		kb = b.groupKeys("home", "", "")
	}
	b.groupNotice(ctx, m, s, kb)
}

// refuseLimit explains a refused request. In a group it doesn't show anyone's
// usage to everyone; the numbers are one tap away, privately.
func (b *Bot) refuseLimit(ctx context.Context, m *telegram.Message, speech bool, u Usage, need int) {
	kind := "speech"
	if !speech {
		kind = "transcription"
	}
	if u.Studio {
		b.track(ctx, m.From.ID, evLimit, "studio", need)
		b.replyNotice(ctx, m, studioRefusal(u))
		return
	}
	b.track(ctx, m.From.ID, evLimit, kind, need)
	if !isGroupChat(m.Chat.ID) {
		ls := b.limitScreen(speech, u, need)
		b.replyHTML(ctx, m, ls.text, ls.kb)
		return
	}
	what := "speaking"
	if !speech {
		what = "transcription"
	}
	s := noticeScreen("🎟", "Daily limit reached",
		"You've used today's "+what+" allowance. It refreshes in <b>"+fmtWait(untilReset())+"</b> (midnight UTC).")
	b.groupNotice(ctx, m, s, b.groupKeys("studio", "", ""))
}

// ---------------------------------------------------------------- private views

// sendPrivately posts s so that only one member of the group sees it.
func (b *Bot) sendPrivately(ctx context.Context, chat, thread int64, e *telegram.Ephemeral, replyToEphemeral int64, s screen) *telegram.Message {
	msg, err := b.tg.SendMessage(ctx, telegram.SendMessageParams{
		ChatID: chat, ThreadID: thread, Text: s.text, ParseMode: "HTML", ReplyMarkup: s.kb, NoPreview: true,
		Ephemeral: e, ReplyToEphemeral: replyToEphemeral,
	})
	if err != nil {
		b.log.Warn("ephemeral send failed", "chat", chat, "err", err)
		return nil
	}
	return msg
}

// replyPrivately answers an ephemeral command so only its sender sees it.
func (b *Bot) replyPrivately(ctx context.Context, m *telegram.Message, s screen) {
	b.sendPrivately(ctx, m.Chat.ID, m.Thread(), &telegram.Ephemeral{ReceiverUserID: m.From.ID}, m.EphemeralMessageID, s)
}

// showPrivately answers a button press in a group with a screen only the
// presser sees: it redraws their private view, or opens one.
func (b *Bot) showPrivately(ctx context.Context, q *telegram.CallbackQuery, s screen) {
	m := q.Message
	if m.EphemeralMessageID != 0 && m.Voice == nil && m.Audio == nil {
		err := b.tg.EditEphemeralMessageText(ctx, telegram.EphemeralEdit{
			ChatID: m.Chat.ID, UserID: q.From.ID, EphemeralID: m.EphemeralMessageID,
			Text: s.text, ParseMode: "HTML", ReplyMarkup: s.kb, NoPreview: true,
		})
		if err == nil || strings.Contains(err.Error(), "message is not modified") {
			return
		}
		b.log.Warn("editEphemeralMessageText failed", "chat", m.Chat.ID, "err", err)
	}
	b.sendPrivately(ctx, m.Chat.ID, m.Thread(), &telegram.Ephemeral{CallbackQueryID: q.ID}, 0, s)
}

// ephemeralCommand answers a command only its sender can see.
func (b *Bot) ephemeralCommand(ctx context.Context, m *telegram.Message, cmd, args string) {
	uid, chat := m.From.ID, m.Chat.ID
	reply := func(s screen) { b.replyPrivately(ctx, m, s) }
	switch cmd {
	case "voices":
		reply(b.voicesScreen(ctx, uid, b.resetSession(uid, args, nil)))
	case "myvoices":
		owned := true
		reply(b.voicesScreen(ctx, uid, b.resetSession(uid, "", &owned)))
	case "voice":
		reply(b.voiceCard(ctx, uid, ""))
	case "style":
		reply(b.styleScreen(uid))
	case "start", "menu", "home", "settings", "studio":
		reply(b.groupMenu(ctx, chat, uid, "home"))
	case "help":
		reply(b.groupHelpScreen(groupSettingsOf(ctx)))
	case "groupsettings":
		admin, err := b.isGroupAdmin(ctx, chat, uid)
		switch {
		case err != nil:
			b.log.Warn("group admin check failed", "chat", chat, "err", err)
			reply(noticeScreen(icWarn, "Try again in a moment", "I couldn't check who runs this group just now."))
		case !admin:
			reply(noticeScreen(icLock, "Only group admins", "Only this group's admins can change Ellie's settings here."))
		default:
			reply(b.groupSettingsScreen(ctx, chat))
		}
	case "say", "speak", "transcribe":
		reply(noticeScreen(icStudio, "Only you can see that",
			"Commands only you can see get answers only you can see, so the group wouldn't hear me. "+
				"Send <code>/"+cmd+"</code> as a normal message instead."))
	default:
		payload := "studio"
		if deepLinks[cmd] {
			payload = cmd
		}
		s := privateOnlyScreen()
		s.kb = b.groupKeys("", icStudio+" Continue in private chat", payload)
		reply(s)
	}
}

// offerMenu answers a public /voices or /style with a button that opens the
// presser's own view privately.
func (b *Bot) offerMenu(ctx context.Context, m *telegram.Message, cmd string) {
	view := map[string]string{"voices": "voices", "myvoices": "voices", "voice": "voice", "style": "style"}[cmd]
	if view == "" {
		view = "home"
	}
	s := noticeScreen(icStudio, "Your settings, privately",
		"Tap below and only you will see your settings. Next time, pick <code>/"+cmd+
			"</code> from the command menu and it opens privately straight away.")
	b.groupNotice(ctx, m, s, b.groupKeys(view, "", ""))
}

// offerGroupSettings answers a public /groupsettings.
func (b *Bot) offerGroupSettings(ctx context.Context, m *telegram.Message) {
	admin, err := b.isGroupAdmin(ctx, m.Chat.ID, m.From.ID)
	if err != nil {
		b.log.Warn("group admin check failed", "chat", m.Chat.ID, "err", err)
	}
	if !admin {
		b.groupNotice(ctx, m, noticeScreen(icLock, "Only group admins", "Only this group's admins can change Ellie's settings here."), nil)
		return
	}
	s := noticeScreen("⚙️", "Group settings", "Tap below to open them. Only you will see them.")
	b.groupNotice(ctx, m, s, (&keys{}).row(btn("⚙️ Open group settings", "gs:open")).build())
}

// ---------------------------------------------------------------- buttons in groups

// groupCallbackKinds are the private-chat buttons that also work inside a
// private view in a group: they only browse voices and change the presser's
// own voice and style.
var groupCallbackKinds = map[string]bool{
	"help": true, "v": true, "vc": true, "vn": true, "vb": true, "vf": true, "vm": true, "vq": true,
	"e": true, "s": true, "vo": true, "m": true, "l": true, "f": true,
}

// groupNav are the screens a private view in a group may open.
var groupNav = map[string]bool{
	"style": true, "emotion": true, "emotions": true, "speed": true, "volume": true,
	"lang": true, "model": true, "output": true, "voices": true, "help": true, "studio": true,
}

// handleGroupCallback handles a button press in a group. Public messages
// carry only "gm:" (my settings) and "gs:" (group settings) buttons; every
// other button must be inside the presser's own private view.
func (b *Bot) handleGroupCallback(ctx context.Context, q *telegram.CallbackQuery) {
	m, uid := q.Message, q.From.ID
	ack := func(text string) { _ = b.tg.AnswerCallbackQuery(ctx, q.ID, text) }
	if !b.allowed(uid) {
		ack("Ellie Studio is private.")
		return
	}
	kind, val, _ := strings.Cut(q.Data, ":")
	switch {
	case kind == "gm":
		ack("")
		b.show(ctx, q, b.groupMenu(ctx, m.Chat.ID, uid, val))
	case kind == "gs":
		b.groupSettingsPress(ctx, q, val)
	case m.EphemeralMessageID == 0:
		b.openPrivateChat(ctx, q) // an older public message's buttons
	case kind == "nav" && val == "home":
		ack("")
		b.show(ctx, q, b.groupMenu(ctx, m.Chat.ID, uid, "home"))
	case kind == "nav" && groupNav[val], groupCallbackKinds[kind]:
		b.dispatchCallback(ctx, q)
	case kind == "p":
		ack("Fetching the preview…")
		b.previewPrivately(ctx, q, val)
	default:
		b.openPrivateChat(ctx, q)
	}
}

// openPrivateChat answers a press by opening the private chat.
func (b *Bot) openPrivateChat(ctx context.Context, q *telegram.CallbackQuery) {
	if link := b.startLink("studio"); link != "" {
		_ = b.tg.AnswerCallbackQueryURL(ctx, q.ID, link)
		return
	}
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Open Ellie Studio in a private chat for that.")
}

// previewPrivately plays a voice's preview clip to the presser only.
func (b *Bot) previewPrivately(ctx context.Context, q *telegram.CallbackQuery, id string) {
	m := q.Message
	v, err := b.ct.GetVoice(ctx, id, true)
	if err != nil {
		b.show(ctx, q, errorScreen(err))
		return
	}
	choose := (&keys{}).row(btn(icOK+" Use "+truncate(v.Name, 24), "v:"+v.ID)).nav("Voices", "nav:voices").build()
	if v.PreviewFileURL == "" {
		b.show(ctx, q, screen{header("🔊", v.Name, "") + "This voice has no preview clip. Choose it and send /say to hear it.", choose})
		return
	}
	data, ct, err := b.ct.DownloadPreview(ctx, v.PreviewFileURL)
	if err != nil {
		b.show(ctx, q, errorScreen(err))
		return
	}
	ext := ".mp3"
	if mt, _, perr := mime.ParseMediaType(ct); perr == nil {
		if e := extForMime(mt); e != "" {
			ext = e
		}
	}
	if _, err := b.tg.SendAudio(ctx, telegram.MediaParams{
		ChatID: m.Chat.ID, ThreadID: m.Thread(), Ephemeral: &telegram.Ephemeral{CallbackQueryID: q.ID},
		Data: data, Filename: "preview" + ext, Title: v.Name + " — preview",
		Caption: "🔊 " + v.Name + " · free preview", ReplyMarkup: choose,
	}); err != nil {
		b.log.Warn("ephemeral preview failed", "chat", m.Chat.ID, "err", err)
	}
}

// groupMenu is a member's own settings, shown privately in a group.
func (b *Bot) groupMenu(ctx context.Context, chat, uid int64, view string) screen {
	switch view {
	case "voices":
		return b.voicesScreen(ctx, uid, b.resetSession(uid, "", nil))
	case "voice":
		return b.voiceCard(ctx, uid, "")
	case "style":
		return b.styleScreen(uid)
	case "studio":
		return b.studioScreen(uid)
	}
	s := b.store.Get(uid)
	text := header(icStudio, "Your Ellie settings", "Only you can see this") +
		field(icVoices, "Voice", orDefault(s.VoiceName, "not chosen yet")) +
		field(icStyle, "Style", styleSummary(s)) +
		"\n" + b.allowanceLines(uid)
	if info, _, err := b.chats.Get(ctx, chat); err == nil {
		if c := effCap(b.lim().GroupLimits.Chars, info.Settings.DailyChars); c > 0 {
			used, _ := info.UsageOn(today())
			text += field("👥", "Group allowance left today", fmtInt(max(0, c-used))+" of "+fmtInt(c)+" characters")
		}
	}
	text += "\n<i>Your voice and style apply everywhere you use Ellie.</i>"
	k := (&keys{}).row(btn(icVoices+" Voices", "nav:voices"), btn(icStyle+" Style", "nav:style"))
	if s.VoiceID != "" {
		k.row(btn("🔊 Preview my voice", "p:"+s.VoiceID))
	}
	if admin, err := b.isGroupAdmin(ctx, chat, uid); err == nil && admin {
		k.row(btn("⚙️ Group settings", "gs:open"))
	}
	if link := b.startLink("studio"); link != "" {
		k.row(telegram.InlineKeyboardButton{Text: "💬 Clone, dictionaries and more", URL: link})
	}
	return screen{text, k.build()}
}

// ---------------------------------------------------------------- group settings

// The choices a group admin cycles through. 0 is the studio's own limit,
// and choices at or above the studio's limit are never offered.
var (
	dailyPresets  = []int{0, 2500, 1000, 500}
	lengthPresets = []int{0, 500, 200}
)

// nextPreset returns the choice after cur.
func nextPreset(cur int, presets []int, ceiling int) int {
	var opts []int
	for _, p := range presets {
		if p == 0 || ceiling == 0 || p < ceiling {
			opts = append(opts, p)
		}
	}
	for i, p := range opts {
		if p == cur {
			return opts[(i+1)%len(opts)]
		}
	}
	return opts[0]
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// charsLabel describes a limit, noting when it is the group's own.
func charsLabel(studio, group int) string {
	c := effCap(studio, group)
	if c == 0 {
		return "unlimited"
	}
	label := fmtInt(c) + " characters"
	if group > 0 && c == group {
		label += " (lowered by this group)"
	}
	return label
}

func (b *Bot) groupSettingsScreen(ctx context.Context, chat int64) screen {
	info, _, err := b.chats.Get(ctx, chat)
	if err != nil {
		return errorScreen(storageError{err})
	}
	gs := info.Settings
	status, who := "on", "everyone"
	if gs.Paused {
		status = "paused"
	}
	if gs.AdminsOnly {
		who = "admins only"
	}
	used, _ := info.UsageOn(today())
	text := header("⚙️", "Group settings", html.EscapeString(info.Title)) +
		"Only this group's admins can see and change these.\n\n" +
		field("⏯", "Ellie here", status) +
		field("👥", "Who can use it", who) +
		field("🗣", "Speaking (/say)", onOff(!gs.NoSpeak)) +
		field(icListen, "Transcribing", onOff(!gs.NoTranscribe)) +
		field("🎟", "Shared daily allowance", charsLabel(b.lim().GroupLimits.Chars, gs.DailyChars)) +
		field("✂️", "Longest message", charsLabel(b.lim().MaxTTSChars, gs.MaxChars)) +
		field("📊", "Spoken here today", fmtInt(used)+" characters")
	pause, admins := "⏸ Pause Ellie", "🛡 Admins only"
	if gs.Paused {
		pause = "▶️ Turn Ellie back on"
	}
	if gs.AdminsOnly {
		admins = "👥 Let everyone use it"
	}
	toggle := func(on bool, what string) string {
		if on {
			return "Turn " + what + " off"
		}
		return "Turn " + what + " on"
	}
	k := (&keys{}).
		row(btn(pause, "gs:pause"), btn(admins, "gs:who")).
		row(btn("🗣 "+toggle(!gs.NoSpeak, "speaking"), "gs:speak"), btn(icListen+" "+toggle(!gs.NoTranscribe, "transcribing"), "gs:tr")).
		row(btn("🎟 Change allowance", "gs:cap"), btn("✂️ Change longest", "gs:len")).
		row(btn(icBack+" My settings", "gm:home"))
	return screen{text, k.build()}
}

// groupSettingsPress applies a "gs:<setting>" button. Every press checks
// again that the presser runs the group.
func (b *Bot) groupSettingsPress(ctx context.Context, q *telegram.CallbackQuery, val string) {
	chat, uid := q.Message.Chat.ID, q.From.ID
	ack := func(text string) { _ = b.tg.AnswerCallbackQuery(ctx, q.ID, text) }
	admin, err := b.isGroupAdmin(ctx, chat, uid)
	if err != nil {
		b.log.Warn("group admin check failed", "chat", chat, "err", err)
		ack("I couldn't check who runs this group. Try again in a moment.")
		return
	}
	if !admin {
		ack("Only this group's admins can change Ellie's settings.")
		return
	}
	if val != "open" {
		info, _, err := b.chats.Get(ctx, chat)
		if err != nil {
			b.log.Error("group settings unavailable", "chat", chat, "err", err)
			ack("I couldn't load the settings. Try again in a moment.")
			return
		}
		gs := info.Settings
		switch val {
		case "pause":
			gs.Paused = !gs.Paused
		case "who":
			gs.AdminsOnly = !gs.AdminsOnly
		case "speak":
			gs.NoSpeak = !gs.NoSpeak
		case "tr":
			gs.NoTranscribe = !gs.NoTranscribe
		case "cap":
			gs.DailyChars = nextPreset(gs.DailyChars, dailyPresets, b.lim().GroupLimits.Chars)
		case "len":
			gs.MaxChars = nextPreset(gs.MaxChars, lengthPresets, b.lim().MaxTTSChars)
		default:
			ack("")
			return
		}
		if err := b.chats.SetSettings(ctx, chat, gs); err != nil {
			b.log.Error("group settings not saved", "chat", chat, "err", err)
			ack("I couldn't save that. Try again in a moment.")
			return
		}
		b.log.Info("group settings changed", "chat", chat, "by", uid, "setting", val)
		ack(icOK + " Saved")
	} else {
		ack("")
	}
	b.show(ctx, q, b.groupSettingsScreen(ctx, chat))
}

// describeGroupSettings summarises non-default settings for /groups.
func describeGroupSettings(gs GroupSettings) string {
	var parts []string
	if gs.Paused {
		parts = append(parts, "paused")
	}
	if gs.AdminsOnly {
		parts = append(parts, "admins only")
	}
	if gs.NoSpeak {
		parts = append(parts, "no /say")
	}
	if gs.NoTranscribe {
		parts = append(parts, "no /transcribe")
	}
	if gs.DailyChars > 0 {
		parts = append(parts, fmt.Sprintf("%s chars/day", fmtInt(gs.DailyChars)))
	}
	if gs.MaxChars > 0 {
		parts = append(parts, fmt.Sprintf("max %d chars", gs.MaxChars))
	}
	return strings.Join(parts, ", ")
}
