package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// testDB opens TEST_DATABASE_URL or skips. The Postgres tests use negative
// user IDs and "test-" resource IDs (never real data) and remove them.
func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to test the Postgres backend")
	}
	db, err := OpenDatabase(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}

func TestPostgresStorePersists(t *testing.T) {
	db := testDB(t)
	const a, b = -9001, -9002
	ctx := context.Background()
	cleanup := func() {
		_, _ = db.pool.Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id IN ($1, $2)`, a, b)
	}
	cleanup()
	t.Cleanup(cleanup)
	exerciseStore(t, func() *Store {
		s, err := db.SettingsStore(ctx, Settings{Model: "sonic-3.6"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}, a, b)
}

func TestMigrationsAreIdempotent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	// Two instances starting at once (as during a deploy) must both succeed.
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- migrate(ctx, db.pool) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var v int
	if err := db.pool.QueryRow(ctx, `SELECT max(version) FROM ellie_schema_migrations`).Scan(&v); err != nil || v != len(migrations) {
		t.Fatalf("schema version = %d (%v), want %d", v, err, len(migrations))
	}
}

// exerciseOwners checks the registry contract on any backend.
func exerciseOwners(t *testing.T, o Owners, alice, bob int64, id func(string) string) {
	t.Helper()
	ctx := context.Background()
	for _, n := range []string{"v1", "v2", "v3"} {
		if err := o.Claim(ctx, kindVoice, id(n), alice); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.Claim(ctx, kindVoice, id("v1"), alice); err != nil {
		t.Errorf("re-claiming your own voice is a no-op, got %v", err)
	}
	if err := o.Claim(ctx, kindVoice, id("v1"), bob); !errors.Is(err, errAlreadyOwned) {
		t.Errorf("claiming someone else's voice must fail, got %v", err)
	}
	if added, err := o.Adopt(ctx, kindVoice, id("v2"), bob); err != nil || added {
		t.Errorf("adopt must never take over an owned voice: added=%v err=%v", added, err)
	}
	if added, err := o.Adopt(ctx, kindVoice, id("b1"), bob); err != nil || !added {
		t.Errorf("adopt should record an unowned voice: added=%v err=%v", added, err)
	}
	// Kinds are separate namespaces.
	if err := o.Claim(ctx, kindDict, id("v1"), bob); err != nil {
		t.Errorf("a dictionary may share an ID with a voice: %v", err)
	}
	got, err := o.Lookup(ctx, kindVoice, []string{id("v1"), id("b1"), id("nope")})
	if err != nil || got[id("v1")] != alice || got[id("b1")] != bob || len(got) != 2 {
		t.Fatalf("lookup = %v, %v", got, err)
	}

	// Paging walks every voice exactly once, newest first.
	var seen []string
	after := Cursor{}
	for {
		page, err := o.List(ctx, kindVoice, alice, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, it := range page {
			seen = append(seen, it.ID)
		}
		after = parseCursor(cursorOf(page[len(page)-1]).String())
	}
	if len(seen) != 3 || seen[0] != id("v3") || seen[2] != id("v1") {
		t.Errorf("alice's voices, newest first = %v", seen)
	}

	if err := o.Release(ctx, kindVoice, id("v2")); err != nil {
		t.Fatal(err)
	}
	if got, _ := o.Lookup(ctx, kindVoice, []string{id("v2")}); len(got) != 0 {
		t.Errorf("released voice still owned: %v", got)
	}
	if err := o.Release(ctx, kindVoice, id("v2")); err != nil {
		t.Errorf("releasing twice is fine, got %v", err)
	}
}

func TestFileOwners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owners.json")
	o, err := OpenFileOwners(path)
	if err != nil {
		t.Fatal(err)
	}
	// Distinct, increasing creation times so "newest first" is well defined.
	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	o.(*fileOwners).now = func() time.Time { tick = tick.Add(time.Second); return tick }
	exerciseOwners(t, o, 7, 8, func(s string) string { return s })

	reopened, err := OpenFileOwners(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reopened.Lookup(context.Background(), kindVoice, []string{"v1", "v2"}); got["v1"] != 7 || len(got) != 1 {
		t.Errorf("registry did not survive a restart: %v", got)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("owners file perms: %v %v", fi, err)
	}
}

func TestPostgresOwners(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("test-%d-", time.Now().UnixNano())
	cleanup := func() {
		_, _ = db.pool.Exec(ctx, `DELETE FROM ellie_ownership WHERE resource_id LIKE $1`, prefix+"%")
	}
	t.Cleanup(cleanup)
	o := db.Owners()
	// Postgres stamps created_at itself; space the claims so order is defined.
	slow := slowClaims{o}
	exerciseOwners(t, slow, -9001, -9002, func(s string) string { return prefix + s })
}

// slowClaims spaces claims a millisecond apart so newest-first is well defined.
type slowClaims struct{ Owners }

func (s slowClaims) Claim(ctx context.Context, kind, id string, owner int64) error {
	time.Sleep(time.Millisecond)
	return s.Owners.Claim(ctx, kind, id, owner)
}

func TestBadDatabaseURLHidesPassword(t *testing.T) {
	_, err := OpenDatabase(context.Background(), "postgres://u:s3cret@host:notaport/db")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); strings.Contains(got, "s3cret") {
		t.Errorf("password leaked: %s", got)
	}
}
