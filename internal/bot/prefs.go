package bot

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

// featuredEmotions are offered as buttons; /emotion list shows all 58.
var featuredEmotions = []string{
	"neutral", "calm", "content", "happy",
	"excited", "curious", "sympathetic", "peaceful",
	"sad", "angry", "scared", "anxious",
	"sarcastic", "confident", "mysterious", "determined",
}

var (
	speedPresets  = []float64{0.6, 0.8, 1.0, 1.2, 1.5}
	volumePresets = []float64{0.5, 0.75, 1.0, 1.5, 2.0}
	localeRe      = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z]{2,4})?$`)
)

// ---------------------------------------------------------------- welcome / help

func (b *Bot) cmdStart(ctx context.Context, m *telegram.Message) {
	s := b.store.Get(m.From.ID)
	b.replyHTML(ctx, m, fmt.Sprintf(`👋 <b>Welcome to Ellie Studio</b>

Your pocket voice studio, powered by Cartesia.

• <b>Send any text</b> and I'll speak it.
• <b>Send a voice note or audio file</b> and I'll transcribe it.
• Browse voices with /voices, then tune /emotion, /speed and /volume.

Current voice: <b>%s</b>. See /help for everything else.`, html.EscapeString(orDefault(s.VoiceName, "none yet"))), nil)
}

func (b *Bot) cmdHelp(ctx context.Context, m *telegram.Message) {
	b.replyHTML(ctx, m, `<b>🎛 Ellie Studio</b>

<b>Speak</b>
• Send text, or /say &lt;text&gt;: text-to-speech
• /voices [search], /myvoices: browse and pick a voice
• /voice [id]: show or set the current voice
• /preview [id]: play a voice's free preview clip
• /emotion, /speed, /volume: shape the delivery
• /model: choose the Sonic model · /lang: set language (en, fr, en-GB, auto)
• /format: voice notes or audio files
• /dicts, /newdict: custom pronunciations

<b>Listen</b>
• Send a voice note or audio file: transcription
• /transcribe (as a reply): transcribe that audio
• /detect (as a reply): check for a Cartesia watermark

<b>Create</b> (admins)
• /clone &lt;name&gt; as a reply to a 5–15s clip: clone a voice
• /accents [lang]: list accents · /localize &lt;accent&gt; [name]: localize the current voice
• /rename &lt;name&gt; · /deletevoice [id]

<b>Studio</b>
• /settings · /reset · /status · /usage`, nil)
}

// ---------------------------------------------------------------- tuning

func (b *Bot) cmdEmotion(ctx context.Context, m *telegram.Message, args string) {
	arg := strings.ToLower(strings.TrimSpace(args))
	switch {
	case arg == "":
		b.replyHTML(ctx, m, "🎭 <b>Pick an emotion</b>\n<i>Auto lets the model read the mood from your text. /emotion list shows all 58.</i>", emotionKeyboard())
	case arg == "list" || arg == "all":
		b.reply(ctx, m, "All emotions:\n"+strings.Join(cartesia.Emotions, ", ")+"\n\nSet one with /emotion <name>.")
	default:
		b.setEmotion(ctx, m.From.ID, arg, func(msg string) { b.replyHTML(ctx, m, msg, nil) })
	}
}

func emotionKeyboard() *telegram.InlineKeyboardMarkup {
	kb := &telegram.InlineKeyboardMarkup{}
	var row []telegram.InlineKeyboardButton
	for _, e := range featuredEmotions {
		row = append(row, telegram.InlineKeyboardButton{Text: e, CallbackData: "e:" + e})
		if len(row) == 4 {
			kb.InlineKeyboard = append(kb.InlineKeyboard, row)
			row = nil
		}
	}
	kb.InlineKeyboard = append(kb.InlineKeyboard, []telegram.InlineKeyboardButton{{Text: "🤖 Auto", CallbackData: "e:auto"}})
	return kb
}

func (b *Bot) setEmotion(_ context.Context, uid int64, e string, done func(string)) {
	if e == "auto" || e == "none" || e == "off" {
		e = ""
	} else if !cartesia.IsEmotion(e) {
		done("Unknown emotion. Try /emotion list.")
		return
	}
	if _, err := b.store.Update(uid, func(s *Settings) { s.Emotion = e }); err != nil {
		done("⚠️ " + explain(err))
		return
	}
	done("🎭 Emotion: <b>" + html.EscapeString(orDefault(e, "auto")) + "</b>")
}

func (b *Bot) cmdSpeed(ctx context.Context, m *telegram.Message, args string) {
	b.cmdRange(ctx, m, args, "Speed", "s:", speedPresets, cartesia.MinSpeed, cartesia.MaxSpeed,
		func(s *Settings, v float64) { s.Speed = v })
}

func (b *Bot) cmdVolume(ctx context.Context, m *telegram.Message, args string) {
	b.cmdRange(ctx, m, args, "Volume", "vo:", volumePresets, cartesia.MinVolume, cartesia.MaxVolume,
		func(s *Settings, v float64) { s.Volume = v })
}

func (b *Bot) cmdRange(ctx context.Context, m *telegram.Message, args, label, prefix string, presets []float64, lo, hi float64, set func(*Settings, float64)) {
	if args == "" {
		kb := &telegram.InlineKeyboardMarkup{}
		var row []telegram.InlineKeyboardButton
		for _, p := range presets {
			row = append(row, telegram.InlineKeyboardButton{Text: fmtFactor(p), CallbackData: prefix + strconv.FormatFloat(p, 'f', -1, 64)})
		}
		kb.InlineKeyboard = append(kb.InlineKeyboard, row)
		b.replyHTML(ctx, m, fmt.Sprintf("<b>%s</b> (%.1f–%.1f). Tap a preset or send /%s 1.1", label, lo, hi, strings.ToLower(label)), kb)
		return
	}
	v, err := parseFactor(args, lo, hi)
	if err != nil {
		b.reply(ctx, m, err.Error())
		return
	}
	if _, err := b.store.Update(m.From.ID, func(s *Settings) { set(s, v) }); err != nil {
		b.fail(ctx, m, "save", err)
		return
	}
	b.replyHTML(ctx, m, fmt.Sprintf("%s: <b>%s</b>", label, fmtFactor(v)), nil)
}

func parseFactor(s string, lo, hi float64) (float64, error) {
	s = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(s)), "x")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("send a number between %.1f and %.1f", lo, hi)
	}
	if v < lo || v > hi {
		return 0, fmt.Errorf("must be between %.1f and %.1f", lo, hi)
	}
	return v, nil
}

func fmtFactor(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) + "×" }

func (b *Bot) cmdModel(ctx context.Context, m *telegram.Message, args string) {
	models := b.ttsModels(ctx)
	if args != "" {
		if !contains(models, args) {
			b.reply(ctx, m, "Unknown model. Options: "+strings.Join(models, ", "))
			return
		}
		if _, err := b.store.Update(m.From.ID, func(s *Settings) { s.Model = args }); err != nil {
			b.fail(ctx, m, "save", err)
			return
		}
		b.replyHTML(ctx, m, "🧠 Model: <b>"+html.EscapeString(args)+"</b>", nil)
		return
	}
	cur := b.store.Get(m.From.ID).Model
	kb := &telegram.InlineKeyboardMarkup{}
	for _, id := range models {
		label := id
		if id == cur {
			label = "✅ " + id
		}
		kb.InlineKeyboard = append(kb.InlineKeyboard, []telegram.InlineKeyboardButton{{Text: label, CallbackData: "m:" + id}})
	}
	b.replyHTML(ctx, m, "🧠 <b>Choose a Sonic model</b>", kb)
}

// ttsModels asks GET /models, falling back to the spec's enum.
func (b *Bot) ttsModels(ctx context.Context) []string {
	list, err := b.ct.ListModels(ctx, "tts")
	if err != nil || len(list) == 0 {
		return cartesia.TTSModels
	}
	ids := make([]string, 0, len(list))
	for _, m := range list {
		if len(m.ID)+2 <= 64 {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

func (b *Bot) cmdLang(ctx context.Context, m *telegram.Message, args string) {
	arg := strings.TrimSpace(args)
	if arg == "" {
		s := b.store.Get(m.From.ID)
		msg := fmt.Sprintf("🌐 Language: <b>%s</b>\nSet with /lang fr, /lang en-GB, or /lang auto.", html.EscapeString(orDefault(s.Language, "auto")))
		if langs := b.modelLanguages(ctx, s.Model); len(langs) > 0 {
			msg += "\n\n<i>" + html.EscapeString(s.Model) + " supports:</i> " + html.EscapeString(strings.Join(langs, ", "))
		}
		b.replyHTML(ctx, m, msg, nil)
		return
	}
	lang := ""
	if l := strings.ToLower(arg); l != "auto" && l != "off" && l != "none" {
		if !localeRe.MatchString(arg) {
			b.reply(ctx, m, "That doesn't look like a language code. Examples: en, fr, ja, en-GB, es-MX.")
			return
		}
		lang = normalizeLocale(arg)
	}
	if _, err := b.store.Update(m.From.ID, func(s *Settings) { s.Language = lang }); err != nil {
		b.fail(ctx, m, "save", err)
		return
	}
	b.replyHTML(ctx, m, "🌐 Language: <b>"+html.EscapeString(orDefault(lang, "auto"))+"</b>\n<i>Also used to transcribe and clone.</i>", nil)
}

func (b *Bot) modelLanguages(ctx context.Context, model string) []string {
	list, err := b.ct.ListModels(ctx, "tts")
	if err != nil {
		return nil
	}
	for _, m := range list {
		if m.ID == model {
			return m.SupportedLanguages
		}
	}
	return nil
}

// normalizeLocale turns "en-gb" into "en-GB" and "FR" into "fr".
func normalizeLocale(s string) string {
	lang, region, ok := strings.Cut(strings.TrimSpace(s), "-")
	lang = strings.ToLower(lang)
	if !ok {
		return lang
	}
	if len(region) == 2 {
		region = strings.ToUpper(region)
	}
	return lang + "-" + region
}

func (b *Bot) cmdFormat(ctx context.Context, m *telegram.Message, args string) {
	switch strings.ToLower(strings.TrimSpace(args)) {
	case "voice", "note":
		b.setFormat(m.From.ID, false)
		b.reply(ctx, m, "🎙 You'll get voice notes.")
	case "file", "audio", "mp3":
		b.setFormat(m.From.ID, true)
		b.reply(ctx, m, "🎵 You'll get MP3 audio files.")
	default:
		kb := &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "🎙 Voice notes", CallbackData: "f:voice"},
			{Text: "🎵 MP3 files", CallbackData: "f:file"},
		}}}
		b.replyHTML(ctx, m, "How should I send speech?", kb)
	}
}

func (b *Bot) setFormat(uid int64, asFile bool) {
	if _, err := b.store.Update(uid, func(s *Settings) { s.AsFile = asFile }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
}

// ---------------------------------------------------------------- pronunciation dictionaries

func (b *Bot) cmdDicts(ctx context.Context, m *telegram.Message) {
	page, err := b.ct.ListPronunciationDicts(ctx, 20, "")
	if err != nil {
		b.fail(ctx, m, "list dicts", err)
		return
	}
	cur := b.store.Get(m.From.ID).DictID
	kb := &telegram.InlineKeyboardMarkup{}
	for _, d := range page.Data {
		if len("d:"+d.ID) > 64 {
			continue
		}
		label := fmt.Sprintf("%s (%d)", truncate(d.Name, 30), len(d.Items))
		if d.ID == cur {
			label = "✅ " + label
		}
		kb.InlineKeyboard = append(kb.InlineKeyboard, []telegram.InlineKeyboardButton{{Text: label, CallbackData: "d:" + d.ID}})
	}
	kb.InlineKeyboard = append(kb.InlineKeyboard, []telegram.InlineKeyboardButton{{Text: "No dictionary", CallbackData: "d:-"}})
	msg := "📖 <b>Pronunciation dictionaries</b>"
	if len(page.Data) == 0 {
		msg += "\nNone yet. Create one with /newdict."
	}
	b.replyHTML(ctx, m, msg, kb)
}

func (b *Bot) cmdNewDict(ctx context.Context, m *telegram.Message, args string) {
	if !b.isAdmin(m.From.ID) {
		b.reply(ctx, m, "Only studio admins can create dictionaries.")
		return
	}
	name, items, err := parseDict(args)
	if err != nil {
		b.reply(ctx, m, err.Error()+"\n\nFormat:\n/newdict Brand words\nCartesia = car-TEE-zhuh\nEllie = EH-lee")
		return
	}
	d, err := b.ct.CreatePronunciationDict(ctx, cartesia.PronunciationDictRequest{
		Name: name, Description: "Created in Ellie Studio", Items: items,
	})
	if err != nil {
		b.fail(ctx, m, "create dict", err)
		return
	}
	if _, err := b.store.Update(m.From.ID, func(s *Settings) { s.DictID, s.DictName = d.ID, d.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	b.replyHTML(ctx, m, fmt.Sprintf("📖 Created <b>%s</b> with %d entries and turned it on.",
		html.EscapeString(d.Name), len(d.Items)), nil)
}

// parseDict reads "name\ntext = pronunciation\n..." (also accepts "=>" or ":").
func parseDict(args string) (string, []cartesia.PronunciationItem, error) {
	lines := strings.Split(strings.TrimSpace(args), "\n")
	name := strings.TrimSpace(lines[0])
	if name == "" || strings.ContainsAny(name, "=") {
		return "", nil, fmt.Errorf("start with a dictionary name on the first line")
	}
	var items []cartesia.PronunciationItem
	for i, l := range lines[1:] {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		var text, pron string
		var ok bool
		for _, sep := range []string{"=>", "=", ":"} {
			if text, pron, ok = strings.Cut(l, sep); ok {
				break
			}
		}
		text, pron = strings.TrimSpace(text), strings.TrimSpace(pron)
		if !ok || text == "" || pron == "" {
			return "", nil, fmt.Errorf("line %d isn't “word = pronunciation”", i+2)
		}
		items = append(items, cartesia.PronunciationItem{Text: text, Pronunciation: pron})
	}
	if len(items) == 0 {
		return "", nil, fmt.Errorf("add at least one “word = pronunciation” line")
	}
	return name, items, nil
}

// ---------------------------------------------------------------- settings / status / usage

func (b *Bot) cmdSettings(ctx context.Context, m *telegram.Message) {
	s := b.store.Get(m.From.ID)
	out := "voice note"
	if s.AsFile {
		out = "MP3 file"
	}
	b.replyHTML(ctx, m, fmt.Sprintf(`⚙️ <b>Your studio</b>

Voice: <b>%s</b> <code>%s</code>
Model: %s
Language: %s
Emotion: %s
Speed: %s · Volume: %s
Dictionary: %s
Output: %s`,
		html.EscapeString(orDefault(s.VoiceName, "none")), html.EscapeString(s.VoiceID),
		html.EscapeString(s.Model), html.EscapeString(orDefault(s.Language, "auto")),
		html.EscapeString(orDefault(s.Emotion, "auto")), fmtFactor(s.Speed), fmtFactor(s.Volume),
		html.EscapeString(orDefault(s.DictName, "none")), out), nil)
}

func (b *Bot) cmdStatus(ctx context.Context, m *telegram.Message) {
	info, err := b.ct.Status(ctx)
	if err != nil {
		b.fail(ctx, m, "status", err)
		return
	}
	state := "🟢 OK"
	if !info.OK {
		state = "🔴 degraded"
	}
	msg := fmt.Sprintf("<b>Cartesia API</b> %s\nLatest API version: <code>%s</code>\nStudio uses: <code>%s</code>",
		state, html.EscapeString(info.Version), cartesia.APIVersion)
	if models, err := b.ct.ListModels(ctx, ""); err == nil {
		var tts, stt []string
		for _, md := range models {
			if md.Capability == "stt" {
				stt = append(stt, md.ID)
			} else {
				tts = append(tts, md.ID)
			}
		}
		msg += "\n\nTTS: " + html.EscapeString(strings.Join(tts, ", ")) +
			"\nSTT: " + html.EscapeString(strings.Join(stt, ", "))
	}
	b.replyHTML(ctx, m, msg, nil)
}

func (b *Bot) cmdUsage(ctx context.Context, m *telegram.Message) {
	if !b.isAdmin(m.From.ID) {
		b.reply(ctx, m, "Only studio admins can see usage.")
		return
	}
	if !b.ct.HasAdminKey() {
		b.reply(ctx, m, "Usage needs an admin key. Set CARTESIA_ADMIN_KEY (sk_car_admin_...) and restart.")
		return
	}
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -28)
	buckets, err := b.ct.CreditUsage(ctx, start, end, "week")
	if err != nil {
		b.fail(ctx, m, "usage", err)
		return
	}
	var total int64
	lines := []string{"📊 <b>Credit usage, last 4 weeks</b>", ""}
	for _, bk := range buckets {
		total += bk.Credits
		lines = append(lines, fmt.Sprintf("%s → %s: <b>%d</b>",
			bk.StartTS.Format("Jan 2"), bk.EndTS.Format("Jan 2"), bk.Credits))
	}
	lines = append(lines, "", fmt.Sprintf("Total: <b>%d</b> credits", total))
	b.replyHTMLLines(ctx, m, lines)
}

// ---------------------------------------------------------------- callbacks

func (b *Bot) handleCallback(ctx context.Context, q *telegram.CallbackQuery) {
	if !b.allowed(q.From.ID) {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Ellie Studio is private.")
		return
	}
	if q.Message == nil {
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "")
		return
	}
	kind, val, _ := strings.Cut(q.Data, ":")
	ack := func(text string) { _ = b.tg.AnswerCallbackQuery(ctx, q.ID, text) }

	switch kind {
	case "v": // select voice
		v, err := b.selectVoice(ctx, q.From.ID, val)
		if err != nil {
			ack("Couldn't select that voice")
			return
		}
		ack("Voice: " + v.Name)
		b.replyHTML(ctx, q.Message, "✅ Now speaking as <b>"+html.EscapeString(v.Name)+"</b>. Send any text to hear it.", nil)
	case "p": // preview
		ack("Fetching preview…")
		b.sendPreview(ctx, q.Message, val)
	case "vn":
		b.pageVoices(ctx, q, +1)
	case "vb":
		b.pageVoices(ctx, q, -1)
	case "e":
		b.setEmotion(ctx, q.From.ID, val, func(msg string) {
			ack("")
			b.editHTML(ctx, q.Message, msg, nil)
		})
	case "s", "vo":
		lo, hi, label := cartesia.MinSpeed, cartesia.MaxSpeed, "Speed"
		if kind == "vo" {
			lo, hi, label = cartesia.MinVolume, cartesia.MaxVolume, "Volume"
		}
		v, err := parseFactor(val, lo, hi)
		if err != nil {
			ack(err.Error())
			return
		}
		_, err = b.store.Update(q.From.ID, func(s *Settings) {
			if kind == "s" {
				s.Speed = v
			} else {
				s.Volume = v
			}
		})
		if err != nil {
			ack("Couldn't save")
			return
		}
		ack(label + ": " + fmtFactor(v))
		b.editHTML(ctx, q.Message, fmt.Sprintf("%s: <b>%s</b>", label, fmtFactor(v)), nil)
	case "m":
		if !contains(b.ttsModels(ctx), val) {
			ack("Unknown model")
			return
		}
		if _, err := b.store.Update(q.From.ID, func(s *Settings) { s.Model = val }); err != nil {
			ack("Couldn't save")
			return
		}
		ack("Model: " + val)
		b.editHTML(ctx, q.Message, "🧠 Model: <b>"+html.EscapeString(val)+"</b>", nil)
	case "f":
		b.setFormat(q.From.ID, val == "file")
		ack("")
		if val == "file" {
			b.editHTML(ctx, q.Message, "🎵 You'll get MP3 audio files.", nil)
		} else {
			b.editHTML(ctx, q.Message, "🎙 You'll get voice notes.", nil)
		}
	case "d":
		id, name := "", ""
		if val != "-" {
			d, err := b.ct.GetPronunciationDict(ctx, val)
			if err != nil {
				ack("Couldn't load that dictionary")
				return
			}
			id, name = d.ID, d.Name
		}
		if _, err := b.store.Update(q.From.ID, func(s *Settings) { s.DictID, s.DictName = id, name }); err != nil {
			ack("Couldn't save")
			return
		}
		ack("")
		b.editHTML(ctx, q.Message, "📖 Dictionary: <b>"+html.EscapeString(orDefault(name, "none"))+"</b>", nil)
	case "dv":
		b.confirmDeleteVoice(ctx, q, val)
	case "x":
		ack("Cancelled")
		b.editHTML(ctx, q.Message, "Cancelled.", nil)
	default:
		ack("")
	}
}
