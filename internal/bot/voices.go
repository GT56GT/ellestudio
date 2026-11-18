package bot

import (
	"context"
	"fmt"
	"html"
	"mime"
	"strings"
	"time"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

const voicesPerPage = 6

// browseSession remembers where a user is in the voice library so that
// callback buttons stay within Telegram's 64-byte callback_data limit.
type browseSession struct {
	query   string
	owned   bool
	cursors []string // starting_after for each visited page; [0] == ""
	next    string
}

func (b *Bot) session(uid int64) *browseSession {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	return b.sessions[uid]
}

func (b *Bot) setSession(uid int64, s *browseSession) {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	b.sessions[uid] = s
}

// ---------------------------------------------------------------- browse

func (b *Bot) cmdVoices(ctx context.Context, m *telegram.Message, query string, owned bool) {
	s := &browseSession{query: query, owned: owned, cursors: []string{""}}
	b.setSession(m.From.ID, s)
	text, kb, err := b.renderVoicePage(ctx, s)
	if err != nil {
		b.fail(ctx, m, "list voices", err)
		return
	}
	b.replyHTML(ctx, m, text, kb)
}

// renderVoicePage fetches the session's current page. The session lock is not
// held during the network call, so the cursor is read up front.
func (b *Bot) renderVoicePage(ctx context.Context, s *browseSession) (string, *telegram.InlineKeyboardMarkup, error) {
	b.sessMu.Lock()
	cursor, page := s.cursors[len(s.cursors)-1], len(s.cursors)
	query, owned := s.query, s.owned
	b.sessMu.Unlock()

	res, err := b.ct.ListVoices(ctx, cartesia.ListVoicesParams{
		Limit: voicesPerPage, Q: query, OwnedOnly: owned, StartingAfter: cursor,
	})
	if err != nil {
		return "", nil, err
	}
	b.sessMu.Lock()
	s.next = ""
	if res.HasMore {
		s.next = res.NextPage
		if s.next == "" && len(res.Data) > 0 {
			s.next = res.Data[len(res.Data)-1].ID
		}
	}
	hasNext := s.next != ""
	b.sessMu.Unlock()

	var sb strings.Builder
	title := "🎧 Voice library"
	if owned {
		title = "🗂 Your voices"
	}
	fmt.Fprintf(&sb, "<b>%s</b>", title)
	if query != "" {
		fmt.Fprintf(&sb, " · “%s”", html.EscapeString(query))
	}
	fmt.Fprintf(&sb, " · page %d\n\n", page)
	if len(res.Data) == 0 {
		sb.WriteString("No voices found. Try another search: /voices <words>")
		return sb.String(), nil, nil
	}

	kb := &telegram.InlineKeyboardMarkup{}
	for i, v := range res.Data {
		fmt.Fprintf(&sb, "%d. <b>%s</b>", i+1, html.EscapeString(v.Name))
		if v.Tagline != "" {
			fmt.Fprintf(&sb, " — %s", html.EscapeString(v.Tagline))
		}
		sb.WriteString("\n   <i>" + html.EscapeString(voiceFacts(v)) + "</i>\n")
		if len("v:"+v.ID) > 64 {
			continue
		}
		kb.InlineKeyboard = append(kb.InlineKeyboard, []telegram.InlineKeyboardButton{
			{Text: fmt.Sprintf("%d. Use %s", i+1, truncate(v.Name, 24)), CallbackData: "v:" + v.ID},
			{Text: "🔊 Preview", CallbackData: "p:" + v.ID},
		})
	}
	var nav []telegram.InlineKeyboardButton
	if page > 1 {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "◀ Back", CallbackData: "vb"})
	}
	if hasNext {
		nav = append(nav, telegram.InlineKeyboardButton{Text: "Next ▶", CallbackData: "vn"})
	}
	if len(nav) > 0 {
		kb.InlineKeyboard = append(kb.InlineKeyboard, nav)
	}
	return sb.String(), kb, nil
}

func voiceFacts(v cartesia.Voice) string {
	var f []string
	if v.Language != "" {
		f = append(f, v.Language)
	}
	if v.Gender != "" {
		f = append(f, strings.ReplaceAll(v.Gender, "_", " "))
	}
	if v.Country != "" {
		f = append(f, v.Country)
	}
	if v.IsPro {
		f = append(f, "pro clone")
	}
	if v.IsOwner {
		f = append(f, "yours")
	}
	return strings.Join(f, " · ")
}

// pageVoices moves the browse session forward (+1) or back (-1).
func (b *Bot) pageVoices(ctx context.Context, q *telegram.CallbackQuery, dir int) {
	s := b.session(q.From.ID)
	if s == nil {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "This list expired. Run /voices again.")
		return
	}
	b.sessMu.Lock()
	switch {
	case dir > 0 && s.next != "":
		s.cursors = append(s.cursors, s.next)
	case dir < 0 && len(s.cursors) > 1:
		s.cursors = s.cursors[:len(s.cursors)-1]
	}
	b.sessMu.Unlock()
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "")
	text, kb, err := b.renderVoicePage(ctx, s)
	if err != nil {
		b.editHTML(ctx, q.Message, "⚠️ "+explain(err), nil)
		return
	}
	b.editHTML(ctx, q.Message, text, kb)
}

// ---------------------------------------------------------------- select / show

func (b *Bot) selectVoice(ctx context.Context, uid int64, id string) (*cartesia.Voice, error) {
	v, err := b.ct.GetVoice(ctx, id, false)
	if err != nil {
		return nil, err
	}
	if _, err := b.store.Update(uid, func(s *Settings) { s.VoiceID, s.VoiceName = v.ID, v.Name }); err != nil {
		return nil, err
	}
	return v, nil
}

func (b *Bot) cmdVoice(ctx context.Context, m *telegram.Message, args string) {
	if args != "" {
		v, err := b.selectVoice(ctx, m.From.ID, args)
		if err != nil {
			b.fail(ctx, m, "select voice", err)
			return
		}
		b.replyHTML(ctx, m, "✅ Now speaking as <b>"+html.EscapeString(v.Name)+"</b>. Send any text to hear it.", nil)
		return
	}
	s := b.store.Get(m.From.ID)
	if s.VoiceID == "" {
		b.reply(ctx, m, "No voice selected yet. Browse with /voices.")
		return
	}
	v, err := b.ct.GetVoice(ctx, s.VoiceID, false)
	if err != nil {
		b.fail(ctx, m, "get voice", err)
		return
	}
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "🔊 Preview", CallbackData: "p:" + v.ID}},
	}}
	b.replyHTML(ctx, m, voiceDetails(v), kb)
}

func voiceDetails(v *cartesia.Voice) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎙 <b>%s</b>", html.EscapeString(v.Name))
	if v.Tagline != "" {
		fmt.Fprintf(&sb, " — %s", html.EscapeString(v.Tagline))
	}
	sb.WriteString("\n")
	if v.Description != "" {
		sb.WriteString(html.EscapeString(truncate(v.Description, 400)) + "\n")
	}
	fmt.Fprintf(&sb, "\nID: <code>%s</code>\n", html.EscapeString(v.ID))
	if f := voiceFacts(*v); f != "" {
		sb.WriteString(html.EscapeString(f) + "\n")
	}
	if len(v.Accents) > 0 {
		var acc []string
		for _, a := range v.Accents {
			label := a.Accent + " (" + a.Locale
			if a.IsNative {
				label += ", native"
			}
			acc = append(acc, label+")")
		}
		sb.WriteString("Accents: " + html.EscapeString(strings.Join(acc, ", ")) + "\n")
	}
	return sb.String()
}

// ---------------------------------------------------------------- preview

func (b *Bot) cmdPreview(ctx context.Context, m *telegram.Message, id string) {
	if id == "" {
		id = b.store.Get(m.From.ID).VoiceID
	}
	if id == "" {
		b.reply(ctx, m, "Usage: /preview <voice id>, or pick a voice first with /voices.")
		return
	}
	b.sendPreview(ctx, m, id)
}

// sendPreview plays a voice's free preview clip (no credits used).
func (b *Bot) sendPreview(ctx context.Context, m *telegram.Message, id string) {
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, "upload_voice")
	v, err := b.ct.GetVoice(ctx, id, true)
	if err != nil {
		b.fail(ctx, m, "get voice", err)
		return
	}
	if v.PreviewFileURL == "" {
		b.reply(ctx, m, "That voice has no preview clip. Send text after selecting it to hear it.")
		return
	}
	data, ct, err := b.ct.DownloadPreview(ctx, v.PreviewFileURL)
	if err != nil {
		b.fail(ctx, m, "download preview", err)
		return
	}
	ext := ".mp3"
	if mt, _, perr := mime.ParseMediaType(ct); perr == nil {
		if e := extForMime(mt); e != "" {
			ext = e
		}
	}
	_, err = b.tg.SendAudio(ctx, telegram.MediaParams{
		ChatID: m.Chat.ID, ReplyTo: m.MessageID, Data: data,
		Filename: "preview" + ext, Title: v.Name + " — preview",
		Caption: "🔊 " + v.Name + " (preview)",
	})
	if err != nil {
		b.fail(ctx, m, "send preview", err)
	}
}

// ---------------------------------------------------------------- accents

func (b *Bot) cmdAccents(ctx context.Context, m *telegram.Message, args string) {
	f := cartesia.AccentFilter{}
	arg := strings.TrimSpace(args)
	switch {
	case arg == "all":
	case strings.Contains(arg, "-"):
		f.Locale = normalizeLocale(arg)
	case arg != "":
		f.Language = strings.ToLower(arg)
	default:
		f.Language = orDefault(baseLang(b.store.Get(m.From.ID).Language), "en")
	}
	list, err := b.ct.ListAccents(ctx, f)
	if err != nil {
		b.fail(ctx, m, "list accents", err)
		return
	}
	if len(list) == 0 {
		b.reply(ctx, m, "No accents found for that. Try /accents all.")
		return
	}
	lines := []string{"<b>🌍 Accents</b> (🔁 = you can /localize into it)", ""}
	for _, a := range list {
		mark := ""
		if a.IsLocalizable {
			mark = " 🔁"
		}
		lines = append(lines, fmt.Sprintf("• <code>%s</code> — %s (%s)%s",
			html.EscapeString(a.ID), html.EscapeString(a.Name), html.EscapeString(a.Locale), mark))
	}
	b.replyHTMLLines(ctx, m, lines)
}

// replyHTMLLines sends self-contained HTML lines in as few messages as fit.
func (b *Bot) replyHTMLLines(ctx context.Context, m *telegram.Message, lines []string) {
	var cur strings.Builder
	for _, l := range lines {
		if cur.Len()+len(l)+1 > telegram.MaxMessageLen-96 {
			b.replyHTML(ctx, m, cur.String(), nil)
			cur.Reset()
		}
		cur.WriteString(l + "\n")
	}
	if cur.Len() > 0 {
		b.replyHTML(ctx, m, cur.String(), nil)
	}
}

// ---------------------------------------------------------------- localize / rename / delete

func (b *Bot) cmdLocalize(ctx context.Context, m *telegram.Message, args string) {
	if !b.isAdmin(m.From.ID) {
		b.reply(ctx, m, "Only studio admins can localize voices.")
		return
	}
	accent, name, _ := strings.Cut(strings.TrimSpace(args), " ")
	if accent == "" {
		b.reply(ctx, m, "Usage: /localize <accent id> [new voice name]\nFind accent ids with /accents (🔁 marks localizable ones).")
		return
	}
	s := b.store.Get(m.From.ID)
	if s.VoiceID == "" {
		b.reply(ctx, m, "Pick the voice to localize first with /voices.")
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("%s (%s)", orDefault(s.VoiceName, "Voice"), accent)
	}
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, "typing")
	if !b.acquire(ctx) {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	v, err := b.ct.LocalizeVoice(tctx, cartesia.LocalizeVoiceRequest{
		VoiceID: s.VoiceID, Name: name, Accent: accent,
		Description: "Localized in Ellie Studio from " + s.VoiceID,
	})
	cancel()
	b.release()
	if err != nil {
		b.fail(ctx, m, "localize", err)
		return
	}
	if _, err := b.store.Update(m.From.ID, func(st *Settings) { st.VoiceID, st.VoiceName = v.ID, v.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	b.replyHTML(ctx, m, fmt.Sprintf("🌍 Created <b>%s</b> and made it your current voice.\nID: <code>%s</code>",
		html.EscapeString(v.Name), html.EscapeString(v.ID)), nil)
}

func (b *Bot) cmdRename(ctx context.Context, m *telegram.Message, name string) {
	if !b.isAdmin(m.From.ID) {
		b.reply(ctx, m, "Only studio admins can rename voices.")
		return
	}
	name = strings.TrimSpace(name)
	s := b.store.Get(m.From.ID)
	if name == "" || s.VoiceID == "" {
		b.reply(ctx, m, "Usage: /rename <new name>  (renames your current voice; you must own it)")
		return
	}
	v, err := b.ct.UpdateVoice(ctx, s.VoiceID, cartesia.UpdateVoiceRequest{Name: name})
	if err != nil {
		b.fail(ctx, m, "rename", err)
		return
	}
	if _, err := b.store.Update(m.From.ID, func(st *Settings) { st.VoiceName = v.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	b.replyHTML(ctx, m, "✏️ Renamed to <b>"+html.EscapeString(v.Name)+"</b>.", nil)
}

func (b *Bot) cmdDeleteVoice(ctx context.Context, m *telegram.Message, id string) {
	if !b.isAdmin(m.From.ID) {
		b.reply(ctx, m, "Only studio admins can delete voices.")
		return
	}
	if id == "" {
		id = b.store.Get(m.From.ID).VoiceID
	}
	if id == "" {
		b.reply(ctx, m, "Usage: /deletevoice <voice id>")
		return
	}
	v, err := b.ct.GetVoice(ctx, id, false)
	if err != nil {
		b.fail(ctx, m, "get voice", err)
		return
	}
	if !v.IsOwner {
		b.reply(ctx, m, "You can only delete voices your organization owns.")
		return
	}
	kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "🗑 Delete permanently", CallbackData: "dv:" + v.ID},
		{Text: "Cancel", CallbackData: "x"},
	}}}
	b.replyHTML(ctx, m, fmt.Sprintf("Delete <b>%s</b> (<code>%s</code>)? This can't be undone.",
		html.EscapeString(v.Name), html.EscapeString(v.ID)), kb)
}

func (b *Bot) confirmDeleteVoice(ctx context.Context, q *telegram.CallbackQuery, id string) {
	if !b.isAdmin(q.From.ID) {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Only studio admins can delete voices.")
		return
	}
	if err := b.ct.DeleteVoice(ctx, id); err != nil {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Delete failed")
		b.editHTML(ctx, q.Message, "⚠️ "+explain(err), nil)
		return
	}
	if err := b.store.ForgetVoice(id); err != nil {
		b.log.Warn("forget voice", "err", err)
	}
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Deleted")
	b.editHTML(ctx, q.Message, "🗑 Voice <code>"+html.EscapeString(id)+"</code> deleted.", nil)
}
