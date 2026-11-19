// Package bot implements Ellie Studio, a Telegram front end for Cartesia's
// speech APIs: text-to-speech, speech-to-text, voice library browsing,
// cloning, localization, pronunciation dictionaries, and watermark checks.
package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

// Name is the bot's display name.
const Name = "Ellie Studio"

// Config controls access and limits.
type Config struct {
	// AllowedUsers may use the bot. Empty means anyone can.
	AllowedUsers map[int64]bool
	// AdminUsers may create or delete Cartesia resources (clone, localize,
	// rename, delete, new dictionaries, usage). Empty means every allowed user.
	AdminUsers map[int64]bool
	// MaxTTSChars caps one synthesis request to protect credits.
	MaxTTSChars int
	// Concurrency caps simultaneous Cartesia calls (Free plan TTS limit is 2).
	Concurrency int
	// PollTimeout is the getUpdates long-poll duration.
	PollTimeout time.Duration
	// DailyChars caps characters spoken per user per UTC day (0 = unlimited).
	DailyChars int
	// DailyAudioSecs caps seconds of audio transcribed or checked per user
	// per UTC day (0 = unlimited). Admins are exempt from both.
	DailyAudioSecs int
	// DailyCreations caps new voices (clones + localizations) per user per
	// UTC day (0 = unlimited). Admins are exempt.
	DailyCreations int
	// DailyDicts caps new pronunciation dictionaries per user per UTC day
	// (0 = unlimited). Admins are exempt.
	DailyDicts int
}

// Bot is the running studio.
type Bot struct {
	tg     *telegram.Client
	ct     *cartesia.Client
	store  *Store
	owners Owners // who created each voice and dictionary
	cfg    Config
	log    *slog.Logger

	cartesiaSlots chan struct{} // bounds concurrent Cartesia work
	inflight      chan struct{} // bounds concurrent update handlers
	wg            sync.WaitGroup

	sessMu   sync.Mutex
	sessions map[int64]*browseSession
	flows    map[int64]*flow   // guided steps in progress
	recent   map[int64]*recent // last spoken text / transcript, for Again and Read aloud
	busy     map[int64]bool    // users with a Cartesia job running
	username string            // the bot's @username, for share links
}

// recent remembers what a user last heard or transcribed.
type recent struct {
	spoken     string
	transcript string
}

// New wires a bot together. A nil owners keeps ownership in memory only.
func New(tg *telegram.Client, ct *cartesia.Client, store *Store, owners Owners, cfg Config, log *slog.Logger) *Bot {
	if owners == nil {
		owners, _ = OpenFileOwners("")
	}
	if cfg.MaxTTSChars <= 0 {
		cfg.MaxTTSChars = 1500
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 50 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Bot{
		tg:            tg,
		ct:            ct,
		store:         store,
		owners:        owners,
		cfg:           cfg,
		log:           log,
		cartesiaSlots: make(chan struct{}, cfg.Concurrency),
		inflight:      make(chan struct{}, 32),
		sessions:      map[int64]*browseSession{},
		flows:         map[int64]*flow{},
		recent:        map[int64]*recent{},
		busy:          map[int64]bool{},
	}
}

// Commands is the short menu Telegram shows; every other command still works
// and is listed in Help → Commands.
var Commands = []telegram.BotCommand{
	{Command: "start", Description: "Open Ellie Studio"},
	{Command: "menu", Description: "Home screen"},
	{Command: "voices", Description: "Browse and pick a voice"},
	{Command: "style", Description: "Emotion, speed, volume, language"},
	{Command: "help", Description: "How everything works"},
	{Command: "cancel", Description: "Stop what you're doing"},
}

// Profile texts shown in Telegram before the first /start.
const (
	botDescription = "🎙 Ellie Studio turns your text into lifelike speech and your voice notes into text.\n\n" +
		"• Hundreds of natural voices\n• Emotion, speed and language controls\n• Clone or localize your own voice\n\n" +
		"Tap Start, then just send a message."
	botShortDescription = "Lifelike text-to-speech and transcription, powered by Cartesia."
)

// Run long-polls Telegram until ctx is cancelled, then waits for in-flight
// handlers to finish.
func (b *Bot) Run(ctx context.Context) error {
	me, err := b.tg.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("telegram getMe (check TELEGRAM_BOT_TOKEN): %w", err)
	}
	b.username = me.Username
	b.log.Info("connected to Telegram", "bot", "@"+me.Username)
	if err := b.tg.SetMyCommands(ctx, Commands); err != nil {
		b.log.Warn("setMyCommands failed", "err", err)
	}
	if err := b.tg.SetMyDescription(ctx, botDescription); err != nil {
		b.log.Warn("setMyDescription failed", "err", err)
	}
	if err := b.tg.SetMyShortDescription(ctx, botShortDescription); err != nil {
		b.log.Warn("setMyShortDescription failed", "err", err)
	}
	b.importLegacyOwners(ctx)

	var offset int64
	backoff := time.Second
	for ctx.Err() == nil {
		pollCtx, cancel := context.WithTimeout(ctx, b.cfg.PollTimeout+15*time.Second)
		updates, err := b.tg.GetUpdates(pollCtx, offset, b.cfg.PollTimeout)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			wait := backoff
			var te *telegram.Error
			if errors.As(err, &te) {
				if te.RetryAfter > 0 {
					wait = time.Duration(te.RetryAfter) * time.Second
				}
				if te.Code == http.StatusConflict {
					b.log.Error("another bot instance or a webhook is consuming updates", "err", err)
				}
				if te.Code == http.StatusUnauthorized {
					return fmt.Errorf("telegram rejected the bot token: %w", err)
				}
			}
			b.log.Warn("getUpdates failed", "err", err, "retry_in", wait)
			sleep(ctx, wait)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = time.Second
		for _, u := range updates {
			offset = u.UpdateID + 1
			select {
			case b.inflight <- struct{}{}:
			case <-ctx.Done():
				continue
			}
			b.wg.Add(1)
			go func(u telegram.Update) {
				defer func() { <-b.inflight; b.wg.Done() }()
				b.handleUpdate(ctx, u)
			}(u)
		}
	}
	b.wg.Wait()
	return nil
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func (b *Bot) handleUpdate(ctx context.Context, u telegram.Update) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("handler panic", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	switch {
	case u.Message != nil:
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

// ---------------------------------------------------------------- access

func (b *Bot) allowed(uid int64) bool {
	return len(b.cfg.AllowedUsers) == 0 || b.cfg.AllowedUsers[uid]
}

func (b *Bot) isAdmin(uid int64) bool {
	if len(b.cfg.AdminUsers) == 0 {
		return b.allowed(uid)
	}
	return b.cfg.AdminUsers[uid]
}

// acquire takes a Cartesia slot, or returns false if ctx ends first.
func (b *Bot) acquire(ctx context.Context) bool {
	select {
	case b.cartesiaSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (b *Bot) release() { <-b.cartesiaSlots }

// ---------------------------------------------------------------- replies

// reply sends plain text, splitting anything longer than Telegram allows.
func (b *Bot) reply(ctx context.Context, m *telegram.Message, text string) {
	for i, chunk := range splitText(text, telegram.MaxMessageLen-96) {
		p := telegram.SendMessageParams{ChatID: m.Chat.ID, Text: chunk, NoPreview: true}
		if i == 0 {
			p.ReplyTo = m.MessageID
		}
		if _, err := b.tg.SendMessage(ctx, p); err != nil {
			b.log.Warn("sendMessage failed", "chat", m.Chat.ID, "err", err)
			return
		}
	}
}

// replyHTML sends one HTML-formatted message, optionally with a keyboard.
func (b *Bot) replyHTML(ctx context.Context, m *telegram.Message, text string, kb *telegram.InlineKeyboardMarkup) {
	p := telegram.SendMessageParams{
		ChatID: m.Chat.ID, Text: text, ParseMode: "HTML", ReplyTo: m.MessageID,
		ReplyMarkup: kb, NoPreview: true,
	}
	if _, err := b.tg.SendMessage(ctx, p); err != nil {
		b.log.Warn("sendMessage failed", "chat", m.Chat.ID, "err", err)
	}
}

// editHTML replaces a bot message in place (used by keyboards).
func (b *Bot) editHTML(ctx context.Context, m *telegram.Message, text string, kb *telegram.InlineKeyboardMarkup) {
	p := telegram.SendMessageParams{
		ChatID: m.Chat.ID, MessageID: m.MessageID, Text: text, ParseMode: "HTML",
		ReplyMarkup: kb, NoPreview: true,
	}
	if err := b.tg.EditMessageText(ctx, p); err != nil && !strings.Contains(err.Error(), "message is not modified") {
		b.log.Warn("editMessageText failed", "chat", m.Chat.ID, "err", err)
	}
}

// sendScreen posts a screen as a new message.
func (b *Bot) sendScreen(ctx context.Context, chatID, replyTo int64, s screen) *telegram.Message {
	msg, err := b.tg.SendMessage(ctx, telegram.SendMessageParams{
		ChatID: chatID, Text: s.text, ParseMode: "HTML", ReplyTo: replyTo,
		ReplyMarkup: s.kb, NoPreview: true,
	})
	if err != nil {
		b.log.Warn("sendMessage failed", "chat", chatID, "err", err)
		return nil
	}
	return msg
}

// show answers a button press with a screen: it redraws the pressed message
// in place, or posts a new one when that message is audio (which has no text
// to replace).
func (b *Bot) show(ctx context.Context, q *telegram.CallbackQuery, s screen) {
	m := q.Message
	if m.Voice != nil || m.Audio != nil || m.Document != nil || m.VideoNote != nil {
		b.sendScreen(ctx, m.Chat.ID, 0, s)
		return
	}
	b.editHTML(ctx, m, s.text, s.kb)
}

// fail reports an error with buttons that lead to the fix.
func (b *Bot) fail(ctx context.Context, m *telegram.Message, what string, err error) {
	b.log.Warn(what, "chat", m.Chat.ID, "err", err)
	retry := ""
	if what == "tts" {
		retry = "do:again"
	}
	s := errorScreen(err)
	s.kb = recovery(err, retry)
	b.replyHTML(ctx, m, s.text, s.kb)
}

// explain turns an error into a short, user-facing HTML string.
func explain(err error) string {
	ae, ok := cartesia.AsAPIError(err)
	if !ok {
		if errors.Is(err, context.DeadlineExceeded) {
			return "That took too long. Please try again."
		}
		return html.EscapeString(err.Error())
	}
	var msg string
	switch ae.ErrorCode {
	case cartesia.ErrQuotaExceeded:
		msg = "The studio is out of Cartesia credits for now."
	case cartesia.ErrConcurrencyLimited:
		msg = "The studio is busy (too many requests at once). Try again in a moment."
	case cartesia.ErrVoiceModelMismatch:
		msg = "That voice doesn't work with the selected model. Try /model or another voice."
	case cartesia.ErrVoiceNotFound:
		msg = "That voice no longer exists. Pick one with /voices."
	case cartesia.ErrModelNotFound:
		msg = "That model isn't available. Pick one with /model."
	case cartesia.ErrLanguageNotSupported:
		msg = "That language isn't supported here. Try /lang auto."
	case cartesia.ErrFileTooLarge:
		msg = "That file is too large for Cartesia."
	case cartesia.ErrUnsupportedAudio:
		msg = "Cartesia can't read that audio format. Try mp3, ogg, wav, flac, m4a, or webm."
	case cartesia.ErrPlanUpgradeRequired:
		msg = "This feature needs a higher Cartesia plan."
	default:
		switch ae.StatusCode {
		case http.StatusUnauthorized:
			msg = "Cartesia rejected the studio's API key."
		case http.StatusForbidden:
			msg = "The studio isn't allowed to do that (you may not own this resource)."
		case http.StatusNotFound:
			msg = "Not found."
		default:
			msg = strings.TrimSpace(ae.Title + ". " + ae.Message)
		}
	}
	out := html.EscapeString(msg)
	if ae.RequestID != "" {
		out += "\n<i>request " + html.EscapeString(ae.RequestID) + "</i>"
	}
	return out
}

// splitText breaks s into pieces of at most n bytes, preferring newlines and
// spaces, and never splitting a UTF-8 sequence.
func splitText(s string, n int) []string {
	var out []string
	for len(s) > n {
		cut := strings.LastIndex(s[:n], "\n")
		if cut < n/2 {
			cut = strings.LastIndex(s[:n], " ")
		}
		if cut < n/2 {
			cut = n
			for cut > 0 && s[cut]&0xC0 == 0x80 { // back off a continuation byte
				cut--
			}
		}
		out = append(out, s[:cut])
		s = strings.TrimLeft(s[cut:], " \n")
	}
	if s != "" || len(out) == 0 {
		out = append(out, s)
	}
	return out
}
