package bot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exerciseStore runs the same changes against any backend and reopens it to
// check that every kind of write survived a restart.
func exerciseStore(t *testing.T, open func() *Store, a, b int64) {
	t.Helper()
	s := open()
	if _, err := s.Update(a, func(v *Settings) { v.VoiceID, v.VoiceName, v.Emotion = "voice-1", "Skylar", "calm" }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(b, func(v *Settings) { v.VoiceID, v.AsFile = "voice-1", true }); err != nil {
		t.Fatal(err)
	}
	if ok, _, err := s.Spend(a, "2026-11-19", 120, 30, 2000, 600); !ok || err != nil {
		t.Fatalf("spend: ok=%v err=%v", ok, err)
	}
	if ok, _, err := s.SpendMake(a, "2026-11-19", 3); !ok || err != nil {
		t.Fatalf("spend make: ok=%v err=%v", ok, err)
	}
	if err := s.ForgetVoice("voice-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	r := open()
	defer r.Close()
	ga, gb := r.Get(a), r.Get(b)
	if ga.VoiceID != "" || gb.VoiceID != "" {
		t.Errorf("ForgetVoice not persisted: %q %q", ga.VoiceID, gb.VoiceID)
	}
	if ga.Emotion != "calm" || !gb.AsFile {
		t.Errorf("preferences lost: %+v / %+v", ga, gb)
	}
	if u := r.UsageOn(a, "2026-11-19"); u.Chars != 120 || u.Secs != 30 || u.Makes != 1 {
		t.Errorf("usage lost: %+v", u)
	}
	if !r.Known(a) || !r.Known(b) {
		t.Error("users not known after reopen")
	}
}

func TestFileStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	exerciseStore(t, func() *Store {
		s, err := OpenStore(path, Settings{Model: "sonic-3.6"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}, 1, 2)
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("settings file perms: %v %v", fi, err)
	}
}

// TestPostgresStorePersists runs only with TEST_DATABASE_URL. It uses
// negative user IDs (never real Telegram users) and removes them afterwards.
func TestPostgresStorePersists(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to test the Postgres store")
	}
	const a, b = -9001, -9002
	ctx := context.Background()
	open := func() *Store {
		s, err := OpenPostgresStore(ctx, url, Settings{Model: "sonic-3.6"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	cleanup := func() {
		s := open()
		defer s.Close()
		_, _ = s.backend.(pgStore).pool.Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id IN ($1, $2)`, a, b)
	}
	cleanup()
	t.Cleanup(cleanup)
	exerciseStore(t, open, a, b)
}

func TestBadDatabaseURLHidesPassword(t *testing.T) {
	_, err := OpenPostgresStore(context.Background(), "postgres://u:s3cret@host:notaport/db", Settings{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); strings.Contains(got, "s3cret") {
		t.Errorf("password leaked: %s", got)
	}
}
