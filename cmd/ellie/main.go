// Command ellie runs the Ellie Studio Telegram bot.
//
// Configuration comes from environment variables (and an optional .env file
// in the working directory). See .env.example.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ellestudio/internal/bot"
	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("ellie studio stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	if err := loadDotEnv(".env"); err != nil {
		return err
	}
	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	ctKey := os.Getenv("CARTESIA_API_KEY")
	if tgToken == "" || ctKey == "" {
		return errors.New("set TELEGRAM_BOT_TOKEN and CARTESIA_API_KEY (see .env.example)")
	}
	if strings.HasPrefix(ctKey, "sk_car_admin_") {
		return errors.New("CARTESIA_API_KEY is an admin key; TTS/STT need a standard key (sk_car_...). Put the admin key in CARTESIA_ADMIN_KEY")
	}

	allowed, err := parseIDs(os.Getenv("ALLOWED_USER_IDS"))
	if err != nil {
		return fmt.Errorf("ALLOWED_USER_IDS: %w", err)
	}
	admins, err := parseIDs(os.Getenv("ADMIN_USER_IDS"))
	if err != nil {
		return fmt.Errorf("ADMIN_USER_IDS: %w", err)
	}
	if len(allowed) == 0 {
		log.Warn("ALLOWED_USER_IDS is empty: anyone who finds the bot can spend your Cartesia credits")
	}

	opts := []cartesia.Option{}
	if base := os.Getenv("CARTESIA_BASE_URL"); base != "" {
		opts = append(opts, cartesia.WithBaseURL(base))
	}
	if ak := os.Getenv("CARTESIA_ADMIN_KEY"); ak != "" {
		opts = append(opts, cartesia.WithAdminKey(ak))
	}
	ct := cartesia.New(ctKey, opts...)
	tg := telegram.New(tgToken, os.Getenv("TELEGRAM_API_URL"))

	dataDir := envOr("DATA_DIR", "data")
	store, err := bot.OpenStore(filepath.Join(dataDir, "settings.json"), bot.Settings{
		VoiceID: os.Getenv("DEFAULT_VOICE_ID"),
		Model:   envOr("DEFAULT_MODEL", cartesia.DefaultTTSModel),
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if port := os.Getenv("PORT"); port != "" {
		startHealthServer(ctx, log, port)
	}

	resolveDefaultVoice(ctx, log, ct, store)

	b := bot.New(tg, ct, store, bot.Config{
		AllowedUsers:   allowed,
		AdminUsers:     admins,
		MaxTTSChars:    envInt("MAX_TTS_CHARS", 1500),
		Concurrency:    envInt("CARTESIA_CONCURRENCY", 2),
		DailyChars:     envLimit("DAILY_CHAR_LIMIT", 2000),
		DailyAudioSecs: envLimit("DAILY_TRANSCRIBE_MINUTES", 10) * 60,
		DailyCreations: envLimit("DAILY_VOICE_CREATIONS", 3),
	}, log)
	log.Info("Ellie Studio starting", "cartesia_version", cartesia.APIVersion, "default_voice", store.Defaults().VoiceName)
	return b.Run(ctx)
}

// startHealthServer answers GET / and /health so hosts that expect an HTTP
// port (Render web services) can health-check the long-polling bot.
func startHealthServer(ctx context.Context, log *slog.Logger, port string) {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Ellie Studio is running\n"))
	}
	mux.HandleFunc("GET /{$}", ok)
	mux.HandleFunc("GET /health", ok)
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("health server", "err", err)
		}
	}()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("health endpoint listening", "port", port)
}

// resolveDefaultVoice names the configured default voice, or picks a
// feminine English voice from the library when none is configured.
func resolveDefaultVoice(ctx context.Context, log *slog.Logger, ct *cartesia.Client, store *bot.Store) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if id := store.Defaults().VoiceID; id != "" {
		v, err := ct.GetVoice(ctx, id, false)
		if err != nil {
			log.Warn("DEFAULT_VOICE_ID lookup failed", "id", id, "err", err)
			store.SetDefaultVoice(id, "default voice")
			return
		}
		store.SetDefaultVoice(v.ID, v.Name)
		return
	}
	page, err := ct.ListVoices(ctx, cartesia.ListVoicesParams{Limit: 1, Gender: "feminine", Language: "en"})
	if err != nil || len(page.Data) == 0 {
		log.Warn("could not pick a default voice; users must choose one with /voices", "err", err)
		return
	}
	store.SetDefaultVoice(page.Data[0].ID, page.Data[0].Name)
}

// loadDotEnv sets KEY=value pairs from path without overriding real env vars.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=value", path, n)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		if _, set := os.LookupEnv(k); !set {
			_ = os.Setenv(k, v)
		}
	}
	return sc.Err()
}

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

// envLimit reads a daily limit where 0 means unlimited and unset means d.
func envLimit(k string, d int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(k))); err == nil && v >= 0 {
		return v
	}
	return d
}

func envInt(k string, d int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return d
}
