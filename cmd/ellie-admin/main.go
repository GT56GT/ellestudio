// Command ellie-admin is Ellie Studio's admin panel, deployed as its own web
// service next to the bot. It shares the bot's Postgres database.
//
// Environment:
//
//	DATABASE_URL        the bot's database (required)
//	TELEGRAM_BOT_TOKEN  the bot's token, to check Telegram sign-ins (required)
//	ADMIN_USER_IDS      who may sign in (required)
//	PUBLIC_URL          the panel's https:// origin (default RENDER_EXTERNAL_URL)
//	PORT                listen port (default 8080)
//	ADMIN_UI_DIR        serve the UI from this folder instead (local development)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ellestudio/internal/admin"
	"ellestudio/internal/bot"
	"ellestudio/internal/telegram"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("admin panel stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	dbURL := os.Getenv("DATABASE_URL")
	if token == "" || dbURL == "" {
		return errors.New("set TELEGRAM_BOT_TOKEN and DATABASE_URL")
	}
	admins, err := parseIDs(os.Getenv("ADMIN_USER_IDS"))
	if err != nil {
		return fmt.Errorf("ADMIN_USER_IDS: %w", err)
	}
	if len(admins) == 0 {
		return errors.New("ADMIN_USER_IDS is empty: nobody could sign in")
	}
	public := strings.TrimRight(envOr("PUBLIC_URL", os.Getenv("RENDER_EXTERNAL_URL")), "/")
	secure := strings.HasPrefix(public, "https://")
	if !secure {
		log.Warn("PUBLIC_URL is not https: running for local development (insecure cookies)")
	}

	db, err := bot.OpenDatabase(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	tg := telegram.New(token, "")
	me, err := tg.GetMe(tctx)
	cancel()
	if err != nil {
		return fmt.Errorf("telegram getMe (check TELEGRAM_BOT_TOKEN): %w", err)
	}

	ui := admin.BuiltUI()
	if dir := os.Getenv("ADMIN_UI_DIR"); dir != "" {
		ui = os.DirFS(dir)
	}
	settings, err := bot.NewSettingsStore(ctx, db, bot.Settings{})
	if err != nil {
		return err
	}
	store := admin.NewPGStore(db, settings)
	srv := admin.New(admin.Config{
		BotToken: token, BotUsername: me.Username, Admins: admins,
		PublicURL: public, Secure: secure, UI: ui, Messenger: messenger{tg},
	}, store, log)

	go prune(ctx, log, store)

	hs := &http.Server{
		Addr:              ":" + envOr("PORT", "8080"),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("admin panel listening", "addr", hs.Addr, "public_url", public, "bot", "@"+me.Username, "ui", ui != nil)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return hs.Shutdown(sctx)
}

// prune deletes expired sessions every hour.
func prune(ctx context.Context, log *slog.Logger, store admin.Store) {
	for {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := store.PruneSessions(pctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Warn("prune sessions", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

// messenger sends the panel's Telegram messages as the bot.
type messenger struct{ tg *telegram.Client }

func (m messenger) Notify(ctx context.Context, chatID int64, html string) error {
	_, err := m.tg.SendMessage(ctx, telegram.SendMessageParams{ChatID: chatID, Text: html, ParseMode: "HTML", NoPreview: true})
	return err
}

func (m messenger) Leave(ctx context.Context, chatID int64) error { return m.tg.LeaveChat(ctx, chatID) }

func parseIDs(s string) (map[int64]bool, error) {
	out := map[int64]bool{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		id, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a Telegram user ID", f)
		}
		out[id] = true
	}
	return out, nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
