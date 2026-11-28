package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// exerciseOwners checks the registry contract on any backend.
func exerciseOwners(t *testing.T, o Owners, alice, bob int64, id func(string) string) {
	t.Helper()
	ctx := context.Background()
	for _, n := range []string{"v1", "v2", "v3"} {
		if err := o.Claim(ctx, KindVoice, id(n), alice); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.Claim(ctx, KindVoice, id("v1"), alice); err != nil {
		t.Errorf("re-claiming your own voice is a no-op, got %v", err)
	}
	if err := o.Claim(ctx, KindVoice, id("v1"), bob); !errors.Is(err, ErrAlreadyOwned) {
		t.Errorf("claiming someone else's voice must fail, got %v", err)
	}
	if added, err := o.Adopt(ctx, KindVoice, id("v2"), bob); err != nil || added {
		t.Errorf("adopt must never take over an owned voice: added=%v err=%v", added, err)
	}
	if added, err := o.Adopt(ctx, KindVoice, id("b1"), bob); err != nil || !added {
		t.Errorf("adopt should record an unowned voice: added=%v err=%v", added, err)
	}
	// Kinds are separate namespaces.
	if err := o.Claim(ctx, KindDict, id("v1"), bob); err != nil {
		t.Errorf("a dictionary may share an ID with a voice: %v", err)
	}
	got, err := o.Lookup(ctx, KindVoice, []string{id("v1"), id("b1"), id("nope")})
	if err != nil || got[id("v1")] != alice || got[id("b1")] != bob || len(got) != 2 {
		t.Fatalf("lookup = %v, %v", got, err)
	}

	// Paging walks every voice exactly once, newest first.
	var seen []string
	after := Cursor{}
	for {
		page, err := o.List(ctx, KindVoice, alice, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, it := range page {
			seen = append(seen, it.ID)
		}
		after = ParseCursor(CursorOf(page[len(page)-1]).String())
	}
	if len(seen) != 3 || seen[0] != id("v3") || seen[2] != id("v1") {
		t.Errorf("alice's voices, newest first = %v", seen)
	}

	if err := o.Release(ctx, KindVoice, id("v2")); err != nil {
		t.Fatal(err)
	}
	if got, _ := o.Lookup(ctx, KindVoice, []string{id("v2")}); len(got) != 0 {
		t.Errorf("released voice still owned: %v", got)
	}
	if err := o.Release(ctx, KindVoice, id("v2")); err != nil {
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
	if got, _ := reopened.Lookup(context.Background(), KindVoice, []string{"v1", "v2"}); got["v1"] != 7 || len(got) != 1 {
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
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_ownership WHERE resource_id LIKE $1`, prefix+"%")
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

func TestCursorRoundTrip(t *testing.T) {
	c := Cursor{time.Date(2026, 11, 19, 9, 30, 0, 123456000, time.UTC), "v-1|odd"}
	if got := ParseCursor(c.String()); !got.Created.Equal(c.Created) || got.ID != c.ID {
		t.Fatalf("round trip = %+v, want %+v", got, c)
	}
	for _, bad := range []string{"", "nope", "x|y", "123|"} {
		if !ParseCursor(bad).IsZero() {
			t.Errorf("ParseCursor(%q) should be the start", bad)
		}
	}
}
