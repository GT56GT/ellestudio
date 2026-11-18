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

func (b *Bot) handleMessage(ctx context.Context, m *telegram.Message) {
	if m.From == nil || m.From.IsBot {
		return
	}
	if !b.allowed(m.From.ID) {
		b.reply(ctx, m, "Sorry, Ellie Studio is private. Ask the owner to add your Telegram ID: "+fmt.Sprint(m.From.ID))
		return
	}

	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(m.Caption)
	}
	cmd, args := parseCommand(text)

	// Audio sent directly: the caption may carry a command.
	if a, ok := audioOf(m); ok {
		switch cmd {
		case "clone":
			b.cmdClone(ctx, m, a, args)
		case "detect":
			b.cmdDetect(ctx, m, a)
		default:
			b.cmdTranscribe(ctx, m, a)
		}
		return
	}
	if cmd == "" {
		if text != "" {
			b.speak(ctx, m, text)
		}
		return
	}

	switch cmd {
	case "start":
		b.cmdStart(ctx, m)
	case "help":
		b.cmdHelp(ctx, m)
	case "say":
		if args == "" {
			b.reply(ctx, m, "Usage: /say <text>  (or just send me any text)")
			return
		}
		b.speak(ctx, m, args)
	case "transcribe", "clone", "detect":
		a, ok := audioOf(m.ReplyToMessage)
		if !ok {
			b.reply(ctx, m, fmt.Sprintf("Reply to a voice note or audio file with /%s, or send the audio with /%s as its caption.", cmd, cmd))
			return
		}
		switch cmd {
		case "transcribe":
			b.cmdTranscribe(ctx, m, a)
		case "clone":
			b.cmdClone(ctx, m, a, args)
		case "detect":
			b.cmdDetect(ctx, m, a)
		}
	case "voices":
		b.cmdVoices(ctx, m, args, false)
	case "myvoices":
		b.cmdVoices(ctx, m, args, true)
	case "voice":
		b.cmdVoice(ctx, m, args)
	case "preview":
		b.cmdPreview(ctx, m, args)
	case "accents":
		b.cmdAccents(ctx, m, args)
	case "localize":
		b.cmdLocalize(ctx, m, args)
	case "rename":
		b.cmdRename(ctx, m, args)
	case "deletevoice":
		b.cmdDeleteVoice(ctx, m, args)
	case "emotion":
		b.cmdEmotion(ctx, m, args)
	case "speed":
		b.cmdSpeed(ctx, m, args)
	case "volume":
		b.cmdVolume(ctx, m, args)
	case "model":
		b.cmdModel(ctx, m, args)
	case "lang", "language":
		b.cmdLang(ctx, m, args)
	case "format":
		b.cmdFormat(ctx, m, args)
	case "dicts":
		b.cmdDicts(ctx, m)
	case "newdict":
		b.cmdNewDict(ctx, m, args)
	case "settings":
		b.cmdSettings(ctx, m)
	case "reset":
		if err := b.store.Reset(m.From.ID); err != nil {
			b.fail(ctx, m, "reset", err)
			return
		}
		b.reply(ctx, m, "Settings reset to the studio defaults.")
	case "status":
		b.cmdStatus(ctx, m)
	case "usage":
		b.cmdUsage(ctx, m)
	default:
		b.reply(ctx, m, "Unknown command. Try /help.")
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

// audioRef is a downloadable audio attachment.
type audioRef struct {
	FileID   string
	Filename string
	Size     int64
	Seconds  int
}

// audioOf finds audio in a message: voice notes, audio files, video notes,
// or documents that look like audio.
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
		return nil, "", fmt.Errorf("that file is %.1f MB; the limit here is %.0f MB", float64(a.Size)/(1<<20), float64(limit)/(1<<20))
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

// ---------------------------------------------------------------- text-to-speech

func (b *Bot) speak(ctx context.Context, m *telegram.Message, text string) {
	if n := utf8.RuneCountInString(text); n > b.cfg.MaxTTSChars {
		b.reply(ctx, m, fmt.Sprintf("That's %d characters; the studio limit is %d per message. Split it up and send again.", n, b.cfg.MaxTTSChars))
		return
	}
	s := b.store.Get(m.From.ID)
	if s.VoiceID == "" {
		b.reply(ctx, m, "Pick a voice first with /voices.")
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

	action := "record_voice"
	if s.AsFile {
		action = "upload_document"
	}
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, action)

	if !b.acquire(ctx) {
		return
	}
	audio, err := b.ttsWithRetry(ctx, req)
	b.release()
	if err != nil {
		b.fail(ctx, m, "tts", err)
		return
	}
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
	caption := speechCaption(s)
	p := telegram.MediaParams{
		ChatID: m.Chat.ID, ReplyTo: m.MessageID, Caption: caption,
		Filename: "ellie-studio.mp3", Data: audio,
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
	parts := []string{"🎙 " + orDefault(s.VoiceName, "voice")}
	if s.Emotion != "" {
		parts = append(parts, s.Emotion)
	}
	if s.Speed != 1 {
		parts = append(parts, fmt.Sprintf("%.2gx speed", s.Speed))
	}
	if s.Volume != 1 {
		parts = append(parts, fmt.Sprintf("%.2gx volume", s.Volume))
	}
	if s.Language != "" {
		parts = append(parts, s.Language)
	}
	return strings.Join(parts, " · ")
}

// ---------------------------------------------------------------- speech-to-text

func (b *Bot) cmdTranscribe(ctx context.Context, m *telegram.Message, a audioRef) {
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, "typing")
	data, name, err := b.fetchAudio(ctx, a, telegram.MaxDownloadBytes)
	if err != nil {
		b.fail(ctx, m, "download", err)
		return
	}
	s := b.store.Get(m.From.ID)
	if !b.acquire(ctx) {
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
		b.fail(ctx, m, "stt", err)
		return
	}
	text := strings.TrimSpace(tr.Text)
	if text == "" {
		b.reply(ctx, m, "I couldn't hear any speech in that.")
		return
	}
	header := "📝 Transcript"
	var meta []string
	if tr.Duration > 0 {
		meta = append(meta, fmt.Sprintf("%.1fs", tr.Duration))
	}
	if tr.Language != "" {
		meta = append(meta, tr.Language)
	}
	if len(meta) > 0 {
		header += " (" + strings.Join(meta, ", ") + ")"
	}
	b.reply(ctx, m, header+"\n\n"+text)
}

// ---------------------------------------------------------------- voice cloning

func (b *Bot) cmdClone(ctx context.Context, m *telegram.Message, a audioRef, name string) {
	if !b.isAdmin(m.From.ID) {
		b.reply(ctx, m, "Only studio admins can clone voices.")
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		b.reply(ctx, m, "Give the voice a name: /clone <name>  (reply to a 5–15 second clean clip of one speaker)")
		return
	}
	if utf8.RuneCountInString(name) > 100 {
		b.reply(ctx, m, "That name is too long.")
		return
	}
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, "typing")
	data, filename, err := b.fetchAudio(ctx, a, cartesia.MaxCloneClipBytes)
	if err != nil {
		b.fail(ctx, m, "download", err)
		return
	}
	s := b.store.Get(m.From.ID)
	lang := orDefault(baseLang(s.Language), "en")
	if !b.acquire(ctx) {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	v, err := b.ct.CloneVoice(tctx, cartesia.CloneVoiceRequest{
		Clip:        bytes.NewReader(data),
		Filename:    filename,
		Name:        name,
		Description: "Cloned in Ellie Studio",
		Language:    lang,
	})
	cancel()
	b.release()
	if err != nil {
		b.fail(ctx, m, "clone", err)
		return
	}
	if _, err := b.store.Update(m.From.ID, func(st *Settings) { st.VoiceID, st.VoiceName = v.ID, v.Name }); err != nil {
		b.log.Warn("save settings", "err", err)
	}
	b.replyHTML(ctx, m, fmt.Sprintf(
		"✨ Cloned <b>%s</b> (%s) and made it your current voice.\nID: <code>%s</code>\n\nSend any text to hear it.",
		html.EscapeString(v.Name), html.EscapeString(v.Language), html.EscapeString(v.ID)), nil)
}

// ---------------------------------------------------------------- watermark

func (b *Bot) cmdDetect(ctx context.Context, m *telegram.Message, a audioRef) {
	if a.Seconds > 0 && (a.Seconds < cartesia.MinWatermarkSecs || a.Seconds > cartesia.MaxWatermarkSecs) {
		b.reply(ctx, m, "Watermark checks need audio between 5 seconds and 5 minutes long.")
		return
	}
	_ = b.tg.SendChatAction(ctx, m.Chat.ID, "typing")
	data, name, err := b.fetchAudio(ctx, a, cartesia.MaxWatermarkBytes)
	if err != nil {
		b.fail(ctx, m, "download", err)
		return
	}
	if !b.acquire(ctx) {
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	found, err := b.ct.DetectWatermark(tctx, bytes.NewReader(data), name)
	cancel()
	b.release()
	if err != nil {
		b.fail(ctx, m, "watermark", err)
		return
	}
	if found {
		b.reply(ctx, m, "🔏 Cartesia watermark detected: this audio was generated with Cartesia.")
	} else {
		b.reply(ctx, m, "No Cartesia watermark detected.")
	}
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
