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

// browseSession remembers where a user is in the voice library so buttons stay
// inside Telegram's 64-byte callback_data limit.
type browseSession struct {
	query   string
	gender  string // "", feminine, masculine
	owned   bool
	cursors []string // starting_after per visited page; [0] == ""
	next    string
}

func (b *Bot) session(uid int64) *browseSession {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	return b.sessions[uid]
}

// resetSession starts a fresh browse, keeping the user's filters.
func (b *Bot) resetSession(uid int64, query string, owned *bool) *browseSession {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	s := &browseSession{query: query, cursors: []string{""}}
	if old := b.sessions[uid]; old != nil {
		s.gender, s.owned = old.gender, old.owned
	}
	if owned != nil {
		s.owned = *owned
	}
	b.sessions[uid] = s
	return s
}

func noVoiceScreen() screen {
	return screen{
		header(icVoices, "Pick a voice first", "") + "Choose who should speak, then send your text again.",
		(&keys{}).row(btn(icVoices+" Browse voices", "nav:voices")).nav("", "").build(),
	}
}

// ---------------------------------------------------------------- library

// openVoices posts a fresh voice library screen.
func (b *Bot) openVoices(ctx context.Context, chatID, uid int64, query string, owned *bool) {
	s := b.resetSession(uid, query, owned)
	b.sendScreen(ctx, chatID, 0, b.voicesScreen(ctx, uid, s))
}

func (b *Bot) voicesScreen(ctx context.Context, uid int64, s *browseSession) screen {
	b.sessMu.Lock()
	cursor, page := s.cursors[len(s.cursors)-1], len(s.cursors)
	query, gender, owned := s.query, s.gender, s.owned
	b.sessMu.Unlock()

	res, err := b.ct.ListVoices(ctx, cartesia.ListVoicesParams{
		Limit: voicesPerPage, Q: query, Gender: gender, OwnedOnly: owned, StartingAfter: cursor,
	})
	if err != nil {
		return errorScreen(err)
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

	cur := b.store.Get(uid)
	var sb strings.Builder
	sb.WriteString(header(icVoices, "Voices", "Tap a name to use it · 🔊 to hear it first"))
	sb.WriteString(field(icStudio, "Now using", orDefault(cur.VoiceName, "none yet")))
	var showing []string
	switch gender {
	case "feminine":
		showing = append(showing, "♀ feminine")
	case "masculine":
		showing = append(showing, "♂ masculine")
	}
	if owned {
		showing = append(showing, "mine")
	}
	if query != "" {
		showing = append(showing, "“"+query+"”")
	}
	if len(showing) == 0 {
		showing = append(showing, "all voices")
	}
	sb.WriteString(field("🔎", "Showing", strings.Join(showing, " · ")))
	if page > 1 {
		sb.WriteString(fmt.Sprintf("<i>Page %d</i>\n", page))
	}
	sb.WriteString("\n")

	k := &keys{}
	if len(res.Data) == 0 {
		sb.WriteString("No voices match. Try another search or clear the filters.")
	}
	for _, v := range res.Data {
		sb.WriteString("<b>" + html.EscapeString(v.Name) + "</b>")
		if v.Tagline != "" {
			sb.WriteString(" — " + html.EscapeString(v.Tagline))
		}
		sb.WriteString("\n<i>" + html.EscapeString(voiceFacts(v)) + "</i>\n\n")
		if len("vc:"+v.ID) > 64 {
			continue
		}
		k.row(btn(mark(truncate(v.Name, 28), v.ID == cur.VoiceID), "v:"+v.ID), btn("🔊", "p:"+v.ID))
	}
	var paging []telegram.InlineKeyboardButton
	if page > 1 {
		paging = append(paging, btn(icBack+" Previous", "vb"))
	}
	if hasNext {
		paging = append(paging, btn("Next ▶", "vn"))
	}
	k.row(paging...)
	k.row(
		btn(mark("All", gender == ""), "vf:all"),
		btn(mark("♀", gender == "feminine"), "vf:feminine"),
		btn(mark("♂", gender == "masculine"), "vf:masculine"),
		btn(mark("Mine", owned), "vm"),
	)
	search := btn("🔍 Search", "do:search")
	if query != "" {
		k.row(search, btn(icCancel+" Clear search", "vq"))
	} else if cur.VoiceID != "" {
		k.row(search, btn("ℹ️ About "+truncate(orDefault(cur.VoiceName, "my voice"), 14), "vc:"+cur.VoiceID))
	} else {
		k.row(search)
	}
	k.nav("", "")
	return screen{strings.TrimRight(sb.String(), "\n"), k.build()}
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

// browse applies a library button (paging, filters) and redraws in place.
func (b *Bot) browse(ctx context.Context, q *telegram.CallbackQuery, kind, val string) {
	uid := q.From.ID
	s := b.session(uid)
	if s == nil {
		s = b.resetSession(uid, "", nil)
	}
	b.sessMu.Lock()
	switch kind {
	case "vn":
		if s.next != "" {
			s.cursors = append(s.cursors, s.next)
		}
	case "vb":
		if len(s.cursors) > 1 {
			s.cursors = s.cursors[:len(s.cursors)-1]
		}
	case "vf":
		s.gender = map[string]string{"feminine": "feminine", "masculine": "masculine"}[val]
		s.cursors = []string{""}
	case "vm":
		s.owned = !s.owned
		s.cursors = []string{""}
	case "vq":
		s.query = ""
		s.cursors = []string{""}
	}
	b.sessMu.Unlock()
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "")
	b.show(ctx, q, b.voicesScreen(ctx, uid, s))
}

// ---------------------------------------------------------------- choosing

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

func voiceChosenScreen(v *cartesia.Voice) screen {
	text := header(icOK, "Now speaking as "+v.Name, voiceFacts(*v)) + "Send any text to hear it, or try a sample."
	k := (&keys{}).
		row(btn(icPlay+" Hear a sample", "do:sample"), btn(icStyle+" Style", "nav:style")).
		nav("Voices", "nav:voices")
	return screen{text, k.build()}
}

// chooseVoice handles a "use this voice" button.
func (b *Bot) chooseVoice(ctx context.Context, q *telegram.CallbackQuery, id string) {
	v, err := b.selectVoice(ctx, q.From.ID, id)
	if err != nil {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Couldn't switch to that voice")
		b.show(ctx, q, errorScreen(err))
		return
	}
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, icOK+" Now speaking as "+v.Name)
	// From the library list, redraw the list so the ✅ moves; elsewhere confirm.
	if s := b.session(q.From.ID); s != nil && q.Message.Text != "" && strings.Contains(q.Message.Text, "Tap a name to use it") {
		b.show(ctx, q, b.voicesScreen(ctx, q.From.ID, s))
		return
	}
	b.show(ctx, q, voiceChosenScreen(v))
}

// ---------------------------------------------------------------- voice card

func (b *Bot) voiceCard(ctx context.Context, uid int64, id string) screen {
	if id == "" {
		id = b.store.Get(uid).VoiceID
	}
	if id == "" {
		return noVoiceScreen()
	}
	v, err := b.ct.GetVoice(ctx, id, false)
	if err != nil {
		return errorScreen(err)
	}
	current := b.store.Get(uid).VoiceID == v.ID
	text := voiceDetails(v)
	if current {
		text += "\n" + icOK + " <i>This is your current voice.</i>"
	}
	k := &keys{}
	if current {
		k.row(btn("🔊 Preview", "p:"+v.ID), btn(icPlay+" Hear a sample", "do:sample"))
	} else {
		k.row(btn(icOK+" Use this voice", "v:"+v.ID), btn("🔊 Preview", "p:"+v.ID))
	}
	if b.isAdmin(uid) {
		if v.IsOwner && current {
			k.row(btn("✏️ Rename", "do:rename"), btn("🗑 Delete", "dv:"+v.ID))
		} else if v.IsOwner {
			k.row(btn("🗑 Delete", "dv:"+v.ID))
		}
		if current {
			k.row(btn(icLocalize+" Localize into another accent", "nav:localize"))
		}
	}
	k.nav("Voices", "nav:voices")
	return screen{text, k.build()}
}

func voiceDetails(v *cartesia.Voice) string {
	var sb strings.Builder
	sb.WriteString(header(icStudio, v.Name, v.Tagline))
	if v.Description != "" {
		sb.WriteString(html.EscapeString(truncate(v.Description, 400)) + "\n\n")
	}
	if f := voiceFacts(*v); f != "" {
		sb.WriteString(html.EscapeString(f) + "\n")
	}
	if len(v.Accents) > 0 {
		var acc []string
		for _, a := range v.Accents {
			label := a.Accent
			if a.IsNative {
				label += " (native)"
			}
			acc = append(acc, label)
		}
		sb.WriteString("Accents: " + html.EscapeString(strings.Join(acc, ", ")) + "\n")
	}
	sb.WriteString("ID: <code>" + html.EscapeString(v.ID) + "</code>\n")
	return sb.String()
}

// ---------------------------------------------------------------- preview

// sendPreview plays a voice's free preview clip with a button to choose it.
func (b *Bot) sendPreview(ctx context.Context, chatID, replyTo int64, id string) {
	_ = b.tg.SendChatAction(ctx, chatID, "upload_voice")
	v, err := b.ct.GetVoice(ctx, id, true)
	if err != nil {
		b.sendScreen(ctx, chatID, replyTo, errorScreen(err))
		return
	}
	if v.PreviewFileURL == "" {
		b.sendScreen(ctx, chatID, replyTo, screen{
			header("🔊", v.Name, "") + "This voice has no preview clip. Choose it and send some text to hear it.",
			(&keys{}).row(btn(icOK+" Use this voice", "v:"+v.ID)).nav("Voices", "nav:voices").build(),
		})
		return
	}
	data, ct, err := b.ct.DownloadPreview(ctx, v.PreviewFileURL)
	if err != nil {
		b.sendScreen(ctx, chatID, replyTo, errorScreen(err))
		return
	}
	ext := ".mp3"
	if mt, _, perr := mime.ParseMediaType(ct); perr == nil {
		if e := extForMime(mt); e != "" {
			ext = e
		}
	}
	_, err = b.tg.SendAudio(ctx, telegram.MediaParams{
		ChatID: chatID, ReplyTo: replyTo, Data: data,
		Filename: "preview" + ext, Title: v.Name + " — preview",
		Caption:     "🔊 " + v.Name + " · free preview",
		ReplyMarkup: (&keys{}).row(btn(icOK+" Use "+truncate(v.Name, 24), "v:"+v.ID)).nav("Voices", "nav:voices").build(),
	})
	if err != nil {
		b.log.Warn("send preview", "err", err)
	}
}

// ---------------------------------------------------------------- accents (command)

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
		ns := noticeScreen(icLocalize, "Accents", "No accents found for that. Try <code>/accents all</code>.")
		b.replyHTML(ctx, m, ns.text, ns.kb)
		return
	}
	lines := []string{header(icLocalize, "Accents", "🔁 = you can localize a voice into it")}
	for _, a := range list {
		mk := ""
		if a.IsLocalizable {
			mk = " 🔁"
		}
		lines = append(lines, fmt.Sprintf("• <code>%s</code> — %s (%s)%s",
			html.EscapeString(a.ID), html.EscapeString(a.Name), html.EscapeString(a.Locale), mk))
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
		b.replyHTML(ctx, m, cur.String(), (&keys{}).nav("", "").build())
	}
}

// ---------------------------------------------------------------- localize

func (b *Bot) localizeScreen(ctx context.Context, uid int64) screen {
	if !b.isAdmin(uid) {
		return b.createScreen(uid)
	}
	s := b.store.Get(uid)
	if s.VoiceID == "" {
		return noVoiceScreen()
	}
	v, err := b.ct.GetVoice(ctx, s.VoiceID, false)
	if err != nil {
		return errorScreen(err)
	}
	accents, err := b.ct.ListAccents(ctx, cartesia.AccentFilter{LocalizableOnly: true, Language: v.Language})
	if err != nil {
		return errorScreen(err)
	}
	has := map[string]bool{}
	for _, a := range v.Accents {
		has[a.Accent] = true
	}
	var bs []telegram.InlineKeyboardButton
	for _, a := range accents {
		if !has[a.ID] && len("loc:"+a.ID) <= 64 {
			bs = append(bs, btn(icLocalize+" "+a.Name, "loc:"+a.ID))
		}
	}
	text := header(icLocalize, "Localize "+v.Name, "Make this voice sound native in another accent")
	if len(bs) == 0 {
		text += "There are no other accents to localize this voice into yet."
	} else {
		text += "Pick an accent. I'll create a <b>new voice</b> and switch you to it; the original stays as it is."
	}
	return screen{text, (&keys{}).grid(2, bs).nav("Create", "nav:create").build()}
}

// runLocalize creates a localized copy of the user's voice. target is a bot
// message that is redrawn with the progress and then the result.
func (b *Bot) runLocalize(ctx context.Context, uid int64, target *telegram.Message, accent string) {
	draw := func(s screen) { b.editHTML(ctx, target, s.text, s.kb) }
	if !b.isAdmin(uid) {
		draw(b.createScreen(uid))
		return
	}
	s := b.store.Get(uid)
	if s.VoiceID == "" {
		draw(noVoiceScreen())
		return
	}
	draw(screen{header(icWait, "Localizing "+orDefault(s.VoiceName, "your voice"), "") + "This usually takes 10–30 seconds.", nil})
	if !b.acquire(ctx) {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	v, err := b.ct.LocalizeVoice(tctx, cartesia.LocalizeVoiceRequest{
		VoiceID: s.VoiceID, Name: fmt.Sprintf("%s (%s)", orDefault(s.VoiceName, "Voice"), accent), Accent: accent,
		Description: "Localized in Ellie Studio from " + s.VoiceID,
	})
	cancel()
	b.release()
	if err != nil {
		draw(errorScreen(err))
		return
	}
	if _, err := b.store.Update(uid, func(st *Settings) { st.VoiceID, st.VoiceName = v.ID, v.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	draw(createdScreen(icLocalize, v.Name, v.ID))
}

// createdScreen confirms a new voice (clone or localize).
func createdScreen(icon, name, id string) screen {
	text := header(icon, name+" is ready", "It's now your current voice") +
		"ID: <code>" + html.EscapeString(id) + "</code>\n\nSend any text to hear it."
	k := (&keys{}).row(btn(icPlay+" Hear a sample", "do:sample"), btn(icStyle+" Style", "nav:style")).nav("", "")
	return screen{text, k.build()}
}

// ---------------------------------------------------------------- rename / delete

func (b *Bot) renameVoice(ctx context.Context, m *telegram.Message, id, name string) {
	if !b.isAdmin(m.From.ID) {
		s := noticeScreen(icLock, "Rename voice", "Only studio admins can rename voices.")
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	v, err := b.ct.UpdateVoice(ctx, id, cartesia.UpdateVoiceRequest{Name: strings.TrimSpace(name)})
	if err != nil {
		b.fail(ctx, m, "rename", err)
		return
	}
	if _, err := b.store.Update(m.From.ID, func(st *Settings) {
		if st.VoiceID == v.ID {
			st.VoiceName = v.Name
		}
	}); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	s := b.voiceCard(ctx, m.From.ID, v.ID)
	s.text += "\n" + icOK + " <i>Renamed.</i>"
	b.replyHTML(ctx, m, s.text, s.kb)
}

func (b *Bot) deleteConfirmScreen(ctx context.Context, uid int64, id string) screen {
	if !b.isAdmin(uid) {
		return noticeScreen(icLock, "Delete voice", "Only studio admins can delete voices.")
	}
	v, err := b.ct.GetVoice(ctx, id, false)
	if err != nil {
		return errorScreen(err)
	}
	if !v.IsOwner {
		return screen{header("🗑", "Can't delete "+v.Name, "") + "You can only delete voices your studio created.",
			(&keys{}).nav("Voice", "vc:"+v.ID).build()}
	}
	text := header("🗑", "Delete "+v.Name+"?", "") + "This removes the voice for good. It can't be undone."
	k := (&keys{}).row(btn("🗑 Yes, delete", "dvy:"+v.ID), btn(icCancel+" Keep it", "vc:"+v.ID)).nav("", "")
	return screen{text, k.build()}
}

func (b *Bot) deleteVoice(ctx context.Context, q *telegram.CallbackQuery, id string) {
	if !b.isAdmin(q.From.ID) {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Only studio admins can delete voices.")
		return
	}
	if err := b.ct.DeleteVoice(ctx, id); err != nil {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Delete failed")
		b.show(ctx, q, errorScreen(err))
		return
	}
	if err := b.store.ForgetVoice(id); err != nil {
		b.log.Warn("forget voice", "err", err)
	}
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Deleted")
	b.show(ctx, q, screen{header("🗑", "Voice deleted", "") + "Pick another voice to keep going.",
		(&keys{}).row(btn(icVoices+" Browse voices", "nav:voices")).nav("", "").build()})
}

// errorScreen is the in-place version of fail().
func errorScreen(err error) screen {
	return screen{header(icWarn, "Something went wrong", "") + explain(err), recovery(err, "")}
}

// noticeScreen is a short titled message with a way home.
func noticeScreen(icon, title, body string) screen {
	return screen{header(icon, title, "") + body, (&keys{}).nav("", "").build()}
}
