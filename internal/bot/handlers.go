package bot

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

// ---------------------------------------------------------------- routing

// handleMessage routes a message in a fixed order so the user always gets the
// most specific answer: commands, then menu buttons, then a guided step in
// progress, then the defaults (audio → transcript, text → speech).
func (b *Bot) handleMessage(ctx context.Context, m *telegram.Message) {
	if m.From == nil || m.From.IsBot {
		return
	}
	uid := m.From.ID
	if !b.allowed(uid) {
		s := privateScreen(uid)
		b.replyHTML(ctx, m, s.text, nil)
		return
	}

	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	cmd, args := parseCommand(text)
	a, hasAudio := audioOf(m)

	if cmd != "" {
		if cmd == "cancel" {
			if b.clearFlow(uid) {
				s := cancelledScreen()
				b.replyHTML(ctx, m, s.text, s.kb)
			} else {
				ns := noticeScreen(icOK, "Nothing to cancel", "You're all set 👍")
				b.replyHTML(ctx, m, ns.text, ns.kb)
			}
			return
		}
		b.clearFlow(uid)
		if hasAudio && (cmd == "clone" || cmd == "detect" || cmd == "transcribe") {
			b.audioCommand(ctx, m, cmd, a, args)
			return
		}
		b.command(ctx, m, cmd, args)
		return
	}
	if !hasAudio && b.menuButton(ctx, m, text) {
		return
	}
	if f := b.getFlow(uid); f != nil {
		b.continueFlow(ctx, m, f, text, a, hasAudio)
		return
	}
	switch {
	case hasAudio:
		b.transcribe(ctx, m, a)
	case text != "":
		b.speak(ctx, m, text)
	}
}

// menuButton handles a tap on the persistent menu under the input box.
func (b *Bot) menuButton(ctx context.Context, m *telegram.Message, text string) bool {
	uid := m.From.ID
	var s screen
	switch text {
	case menuSpeak:
		s = b.speakPrompt(uid)
	case menuVoices:
		b.clearFlow(uid)
		b.openVoices(ctx, m.Chat.ID, uid, "", nil)
		return true
	case menuStyle:
		s = b.styleScreen(uid)
	case menuTranscribe:
		s = transcribePrompt()
	case menuHome:
		s = b.homeScreen(uid)
	case menuHelp:
		s = helpScreen("")
	default:
		return false
	}
	b.clearFlow(uid)
	b.sendScreen(ctx, m.Chat.ID, 0, s)
	return true
}

// audioCommand runs a command aimed at an audio attachment.
func (b *Bot) audioCommand(ctx context.Context, m *telegram.Message, cmd string, a audioRef, args string) {
	switch cmd {
	case "transcribe":
		b.transcribe(ctx, m, a)
	case "detect":
		b.detect(ctx, m, a)
	case "clone":
		if !b.isAdmin(m.From.ID) {
			s := b.createScreen(m.From.ID)
			b.replyHTML(ctx, m, s.text, s.kb)
			return
		}
		if strings.TrimSpace(args) == "" {
			b.setFlow(m.From.ID, &flow{kind: flowCloneName, clip: a})
			s := clonePromptName()
			b.replyHTML(ctx, m, s.text, s.kb)
			return
		}
		b.runClone(ctx, m, a, args)
	}
}

// parseCommand splits "/cmd@bot args" into ("cmd", "args").
func parseCommand(text string) (string, string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	head, rest, _ := strings.Cut(text[1:], " ")
	if nl := strings.IndexByte(head, '\n'); nl >= 0 {
		rest = head[nl+1:] + " " + rest
		head = head[:nl]
	}
	head, _, _ = strings.Cut(head, "@")
	return strings.ToLower(head), strings.TrimSpace(rest)
}

// ---------------------------------------------------------------- audio attachments

// audioRef is a downloadable audio attachment.
type audioRef struct {
	FileID   string
	Filename string
	Size     int64
	Seconds  int
}

// audioOf finds audio in a message: voice notes, audio files, video notes, or
// documents that look like audio.
func audioOf(m *telegram.Message) (audioRef, bool) {
	if m == nil {
		return audioRef{}, false
	}
	switch {
	case m.Voice != nil:
		return audioRef{m.Voice.FileID, "voice.ogg", m.Voice.FileSize, m.Voice.Duration}, true
	case m.Audio != nil:
		name := m.Audio.FileName
		if name == "" {
			name = "audio" + extForMime(m.Audio.MimeType)
		}
		return audioRef{m.Audio.FileID, name, m.Audio.FileSize, m.Audio.Duration}, true
	case m.VideoNote != nil:
		return audioRef{m.VideoNote.FileID, "video_note.mp4", m.VideoNote.FileSize, m.VideoNote.Duration}, true
	case m.Document != nil:
		d := m.Document
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(d.FileName)), ".")
		if strings.HasPrefix(d.MimeType, "audio/") || contains(cartesia.STTFormats, ext) {
			name := d.FileName
			if name == "" {
				name = "audio" + extForMime(d.MimeType)
			}
			return audioRef{d.FileID, name, d.FileSize, 0}, true
		}
	}
	return audioRef{}, false
}

func extForMime(mime string) string {
	switch mime {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/flac", "audio/x-flac":
		return ".flac"
	case "audio/mp4", "audio/m4a", "audio/x-m4a", "audio/aac":
		return ".m4a"
	case "audio/webm":
		return ".webm"
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// fetchAudio downloads an attachment, enforcing limit (and Telegram's 20 MB).
func (b *Bot) fetchAudio(ctx context.Context, a audioRef, limit int64) ([]byte, string, error) {
	limit = min(limit, telegram.MaxDownloadBytes)
	if a.Size > limit {
		return nil, "", fmt.Errorf("That file is %.1f MB; the limit here is %.0f MB.", float64(a.Size)/(1<<20), float64(limit)/(1<<20))
	}
	data, filePath, err := b.tg.Download(ctx, a.FileID, limit)
	if err != nil {
		return nil, "", err
	}
	name := a.Filename
	if path.Ext(name) == "" && path.Ext(filePath) != "" {
		name += path.Ext(filePath)
	}
	return data, name, nil
}

// ---------------------------------------------------------------- progress

// progress is a "⏳ working…" message that later turns into the result.
type progress struct {
	b   *Bot
	msg *telegram.Message
}

func (b *Bot) startProgress(ctx context.Context, m *telegram.Message, text string) *progress {
	msg := b.sendScreen(ctx, m.Chat.ID, m.MessageID, screen{icWait + " " + text, nil})
	return &progress{b, msg}
}

// finish replaces the progress message with s (or posts s if that fails).
func (p *progress) finish(ctx context.Context, chatID int64, s screen) {
	if p.msg != nil {
		err := p.b.tg.EditMessageText(ctx, telegram.SendMessageParams{
			ChatID: chatID, MessageID: p.msg.MessageID, Text: s.text, ParseMode: "HTML", ReplyMarkup: s.kb, NoPreview: true,
		})
		if err == nil {
			return
		}
	}
	p.b.sendScreen(ctx, chatID, 0, s)
}

// ---------------------------------------------------------------- remembered text

func (b *Bot) remember(uid int64, fn func(*recent)) {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	r := b.recent[uid]
	if r == nil {
		r = &recent{}
		b.recent[uid] = r
	}
	fn(r)
}

func (b *Bot) lastSpoken(uid int64) string {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	if r := b.recent[uid]; r != nil {
		return r.spoken
	}
	return ""
}

func (b *Bot) lastTranscript(uid int64) string {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	if r := b.recent[uid]; r != nil {
		return r.transcript
	}
	return ""
}

// ---------------------------------------------------------------- text-to-speech

func (b *Bot) speak(ctx context.Context, m *telegram.Message, text string) {
	uid := m.From.ID
	if n := utf8.RuneCountInString(text); n > b.cfg.MaxTTSChars {
		s := noticeScreen(icWarn, "That's a lot of text", fmt.Sprintf(
			"It's <b>%d</b> characters. I can read up to <b>%d</b> at a time, so split it into smaller messages.", n, b.cfg.MaxTTSChars))
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	s := b.store.Get(uid)
	if s.VoiceID == "" {
		ns := noVoiceScreen()
		b.replyHTML(ctx, m, ns.text, ns.kb)
		return
	}

	req := cartesia.TTSRequest{
		ModelID:             s.Model,
		Transcript:          text,
		Voice:               cartesia.VoiceRef{ID: s.VoiceID},
		Language:            s.Language,
		OutputFormat:        cartesia.MP3(44100, 128000),
		PronunciationDictID: s.DictID,
	}
	if s.Emotion != "" || s.Speed != 1 || s.Volume != 1 {
		req.GenerationConfig = &cartesia.GenerationConfig{Emotion: s.Emotion}
		if s.Speed != 1 {
			req.GenerationConfig.Speed = s.Speed
		}
		if s.Volume != 1 {
			req.GenerationConfig.Volume = s.Volume
		}
	}

	if !b.begin(uid) {
		b.busyNotice(ctx, m)
		return
	}
	defer b.end(uid)
	chars := utf8.RuneCountInString(text)
	if ok, u := b.spendChars(uid, chars); !ok {
		ls := b.limitScreen(true, u, chars)
		b.replyHTML(ctx, m, ls.text, ls.kb)
		return
	}

	action := "record_voice"
	if s.AsFile {
		action = "upload_document"
	}
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, action)

	if !b.acquire(ctx) {
		b.refund(uid, chars, 0)
		return
	}
	audio, err := b.ttsWithRetry(ctx, req)
	b.release()
	if err != nil {
		b.refund(uid, chars, 0)
		b.fail(ctx, m, "tts", err)
		return
	}
	b.remember(uid, func(r *recent) { r.spoken = text })
	b.sendSpeech(ctx, m, s, text, audio)
}

// ttsWithRetry retries briefly when Cartesia reports concurrency limits.
func (b *Bot) ttsWithRetry(ctx context.Context, req cartesia.TTSRequest) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		audio, err := b.ct.TTSBytes(tctx, req)
		if err == nil {
			return audio, nil
		}
		lastErr = err
		ae, ok := cartesia.AsAPIError(err)
		if !ok || ae.ErrorCode != cartesia.ErrConcurrencyLimited {
			break
		}
		sleep(tctx, time.Duration(attempt+1)*1500*time.Millisecond)
	}
	return nil, lastErr
}

func (b *Bot) sendSpeech(ctx context.Context, m *telegram.Message, s Settings, text string, audio []byte) {
	p := telegram.MediaParams{
		ChatID: m.Chat.ID, ReplyTo: m.MessageID, Caption: speechCaption(s),
		Filename: "ellie-studio.mp3", Data: audio, ReplyMarkup: afterSpeech(),
	}
	if !s.AsFile {
		_, err := b.tg.SendVoice(ctx, p)
		if err == nil {
			return
		}
		// Users can forbid voice notes in their privacy settings; fall back.
		if !strings.Contains(err.Error(), "VOICE_MESSAGES_FORBIDDEN") {
			b.fail(ctx, m, "sendVoice", err)
			return
		}
	}
	p.Title = truncate(text, 60)
	if _, err := b.tg.SendAudio(ctx, p); err != nil {
		b.fail(ctx, m, "sendAudio", err)
	}
}

func speechCaption(s Settings) string {
	parts := []string{icStudio + " " + orDefault(s.VoiceName, "voice")}
	if s.Emotion != "" {
		parts = append(parts, s.Emotion)
	}
	if s.Speed != 1 {
		parts = append(parts, fmtFactor(s.Speed)+" speed")
	}
	if s.Volume != 1 {
		parts = append(parts, fmtFactor(s.Volume)+" volume")
	}
	if s.Language != "" {
		parts = append(parts, langLabel(s.Language))
	}
	return strings.Join(parts, " · ")
}

// ---------------------------------------------------------------- speech-to-text

// bytesPerAudioSec turns a file size into a generous length estimate: 16 kbps
// is below what voice notes and music files use, so most recordings are
// estimated longer than they really are.
const bytesPerAudioSec = 16000 / 8

// audioEstimate is the seconds to reserve for a recording. Durations come
// from the sender's app (and files sent as documents have none), so they are
// not trusted alone: the file size sets a floor on the estimate.
func audioEstimate(a audioRef) int {
	return max(a.Seconds, int(a.Size/bytesPerAudioSec), 1)
}

// reserveAudio claims the estimated length of a recording from the daily
// allowance up front, refusing it if that would pass the limit.
func (b *Bot) reserveAudio(ctx context.Context, m *telegram.Message, a audioRef, capSecs int) (int, bool) {
	secs := audioEstimate(a)
	if capSecs > 0 {
		secs = min(secs, capSecs)
	}
	if ok, u := b.spendSecs(m.From.ID, secs); !ok {
		ls := b.limitScreen(false, u, secs)
		b.replyHTML(ctx, m, ls.text, ls.kb)
		return 0, false
	}
	return secs, true
}

// settleAudio corrects a reservation to the real length Cartesia reported:
// it refunds an over-estimate and records an under-estimate (without
// refusing, since the work is already done).
func (b *Bot) settleAudio(uid int64, claimed int, actual float64) {
	if actual <= 0 || b.isAdmin(uid) {
		return
	}
	switch diff := int(actual+0.999) - claimed; {
	case diff > 0:
		if _, _, err := b.store.Spend(uid, today(), 0, diff, 0, 0); err != nil {
			b.log.Warn("save usage", "err", err)
		}
	case diff < 0:
		b.store.Refund(uid, today(), 0, -diff)
	}
}

func (b *Bot) transcribe(ctx context.Context, m *telegram.Message, a audioRef) {
	uid := m.From.ID
	if !b.begin(uid) {
		b.busyNotice(ctx, m)
		return
	}
	defer b.end(uid)
	claimed, ok := b.reserveAudio(ctx, m, a, 0)
	if !ok {
		return
	}
	pr := b.startProgress(ctx, m, "Listening…")
	data, name, err := b.fetchAudio(ctx, a, telegram.MaxDownloadBytes)
	if err != nil {
		b.refund(uid, 0, claimed)
		pr.finish(ctx, m.Chat.ID, errorScreen(err))
		return
	}
	s := b.store.Get(uid)
	if !b.acquire(ctx) {
		b.refund(uid, 0, claimed)
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	tr, err := b.ct.Transcribe(tctx, cartesia.STTRequest{
		Audio:    bytes.NewReader(data),
		Filename: name,
		Language: baseLang(s.Language),
	})
	cancel()
	b.release()
	if err != nil {
		b.refund(uid, 0, claimed)
		pr.finish(ctx, m.Chat.ID, errorScreen(err))
		return
	}
	b.settleAudio(uid, claimed, tr.Duration)
	text := strings.TrimSpace(tr.Text)
	if text == "" {
		pr.finish(ctx, m.Chat.ID, screen{
			header(icListen, "No speech found", "") + "I couldn't hear any words in that. Try a clearer recording.",
			(&keys{}).nav("", "").build(),
		})
		return
	}
	b.remember(uid, func(r *recent) { r.transcript = truncate(text, b.cfg.MaxTTSChars) })

	var meta []string
	if tr.Duration > 0 {
		meta = append(meta, fmt.Sprintf("%.1fs", tr.Duration))
	}
	if tr.Language != "" {
		meta = append(meta, tr.Language)
	}
	chunks := splitText(text, telegram.MaxMessageLen-400)
	head := header(icListen, "Transcript", strings.Join(meta, " · "))
	if len(chunks) == 1 {
		pr.finish(ctx, m.Chat.ID, screen{head + html.EscapeString(chunks[0]), afterTranscript()})
		return
	}
	pr.finish(ctx, m.Chat.ID, screen{head + html.EscapeString(chunks[0]), nil})
	for i, c := range chunks[1:] {
		var kb *telegram.InlineKeyboardMarkup
		if i == len(chunks)-2 {
			kb = afterTranscript()
		}
		b.sendScreen(ctx, m.Chat.ID, 0, screen{html.EscapeString(c), kb})
	}
}

// ---------------------------------------------------------------- voice cloning

func (b *Bot) runClone(ctx context.Context, m *telegram.Message, a audioRef, name string) {
	uid := m.From.ID
	if !b.isAdmin(uid) {
		s := b.createScreen(uid)
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	name = strings.TrimSpace(name)
	pr := b.startProgress(ctx, m, "Cloning <b>"+html.EscapeString(name)+"</b>… this takes a few seconds.")
	data, filename, err := b.fetchAudio(ctx, a, cartesia.MaxCloneClipBytes)
	if err != nil {
		pr.finish(ctx, m.Chat.ID, errorScreen(err))
		return
	}
	lang := orDefault(baseLang(b.store.Get(uid).Language), "en")
	if !b.acquire(ctx) {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	v, err := b.ct.CloneVoice(tctx, cartesia.CloneVoiceRequest{
		Clip:        bytes.NewReader(data),
		Filename:    filename,
		Name:        name,
		Description: withOwner("Cloned in Ellie Studio", uid),
		Language:    lang,
	})
	cancel()
	b.release()
	if err != nil {
		pr.finish(ctx, m.Chat.ID, errorScreen(err))
		return
	}
	if _, err := b.store.Update(uid, func(st *Settings) { st.VoiceID, st.VoiceName = v.ID, v.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	pr.finish(ctx, m.Chat.ID, createdScreen(icCreate, v.Name, v.ID))
}

// ---------------------------------------------------------------- watermark

func (b *Bot) detect(ctx context.Context, m *telegram.Message, a audioRef) {
	nav := (&keys{}).nav("", "").build()
	if a.Seconds > 0 && (a.Seconds < cartesia.MinWatermarkSecs || a.Seconds > cartesia.MaxWatermarkSecs) {
		s := noticeScreen("🔏", "Watermark check", "This check needs audio between <b>5 seconds and 5 minutes</b> long.")
		b.replyHTML(ctx, m, s.text, s.kb)
		return
	}
	uid := m.From.ID
	if !b.begin(uid) {
		b.busyNotice(ctx, m)
		return
	}
	defer b.end(uid)
	claimed, ok := b.reserveAudio(ctx, m, a, cartesia.MaxWatermarkSecs)
	if !ok {
		return
	}
	pr := b.startProgress(ctx, m, "Checking for a watermark…")
	data, name, err := b.fetchAudio(ctx, a, cartesia.MaxWatermarkBytes)
	if err != nil {
		b.refund(uid, 0, claimed)
		pr.finish(ctx, m.Chat.ID, errorScreen(err))
		return
	}
	if !b.acquire(ctx) {
		b.refund(uid, 0, claimed)
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	found, err := b.ct.DetectWatermark(tctx, bytes.NewReader(data), name)
	cancel()
	b.release()
	if err != nil {
		b.refund(uid, 0, claimed)
		pr.finish(ctx, m.Chat.ID, errorScreen(err))
		return
	}
	verdict := "No Cartesia watermark found in this audio."
	if found {
		verdict = "✅ <b>Watermark found.</b> This audio was generated with Cartesia."
	}
	pr.finish(ctx, m.Chat.ID, screen{header("🔏", "Watermark check", "") + verdict, nav})
}

// ---------------------------------------------------------------- small helpers

func baseLang(code string) string {
	lang, _, _ := strings.Cut(code, "-")
	return strings.ToLower(lang)
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
