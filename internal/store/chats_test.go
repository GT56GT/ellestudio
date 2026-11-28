package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// exerciseChats checks the group registry contract on any backend. g1-g3
// are group IDs and day/next two consecutive days, chosen by the caller so
// a run against Postgres never touches real groups or real days.
func exerciseChats(t *testing.T, c Chats, g1, g2, g3 int64, day, next string) {
	t.Helper()
	ctx := context.Background()
	lim := GroupLimits{Chars: 100, Secs: 60, AllChars: 150}

	if _, ok, err := c.Get(ctx, g1); ok || err != nil {
		t.Fatalf("unknown group: ok=%v err=%v", ok, err)
	}
	if err := c.Upsert(ctx, ChatInfo{ID: g1, Type: "group", Title: "Club", Status: ChatMember, AddedBy: 7}); err != nil {
		t.Fatal(err)
	}
	// Empty fields keep what is stored.
	if err := c.Upsert(ctx, ChatInfo{ID: g1, Status: ChatLeft}); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := c.Get(ctx, g1); !ok || got.Title != "Club" || got.Type != "group" || got.AddedBy != 7 || got.Status != ChatLeft {
		t.Errorf("upsert lost fields: %+v", got)
	}

	if err := c.Spend(ctx, g1, day, 90, 30, lim); err != nil {
		t.Fatal(err)
	}
	if err := c.Spend(ctx, g1, day, 20, 0, lim); !errors.Is(err, ErrGroupLimit) {
		t.Errorf("over the group cap: %v", err)
	}
	if err := c.Spend(ctx, g1, day, 0, 40, lim); !errors.Is(err, ErrGroupLimit) {
		t.Errorf("over the group's transcription cap: %v", err)
	}
	if err := c.Spend(ctx, g2, day, 70, 0, lim); !errors.Is(err, ErrGroupsLimit) {
		t.Errorf("over the all-groups cap: %v", err)
	}
	if err := c.Spend(ctx, g2, day, 60, 0, lim); err != nil {
		t.Errorf("inside every cap: %v", err)
	}
	if got, _, _ := c.Get(ctx, g1); got.Chars != 90 || got.Secs != 30 {
		t.Errorf("a refused spend changed usage: %+v", got)
	}
	if err := c.Refund(ctx, g1, day, 50, 100); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := c.Get(ctx, g1); got.Chars != 40 || got.Secs != 0 {
		t.Errorf("refund = %+v, want 40 chars and never below 0 secs", got)
	}

	top, err := c.Busiest(ctx, day, 10)
	if err != nil || len(top) != 2 || top[0].ID != g2 || top[1].ID != g1 {
		t.Errorf("busiest on %s = %+v, %v", day, top, err)
	}
	// A new day starts from zero.
	if err := c.Spend(ctx, g1, next, 100, 0, lim); err != nil {
		t.Errorf("the allowance did not reset on a new day: %v", err)
	}

	if err := c.SetBlocked(ctx, g2, true); err != nil {
		t.Fatal(err)
	}
	gs := GroupSettings{Paused: true, AdminsOnly: true, NoTranscribe: true, DailyChars: 1000, MaxChars: 200}
	if err := c.SetSettings(ctx, g2, gs); err != nil {
		t.Fatal(err)
	}
	// Recording the group again must not reset its settings or block.
	if err := c.Upsert(ctx, ChatInfo{ID: g2, Title: "Renamed", Status: ChatMember}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := c.Get(ctx, g2); got.Settings != gs || !got.Blocked || got.Title != "Renamed" {
		t.Errorf("settings or block lost on upsert: %+v", got)
	}
	if err := c.Migrate(ctx, g2, g3); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Get(ctx, g2); ok {
		t.Error("the old ID survived the migration")
	}
	if got, ok, _ := c.Get(ctx, g3); !ok || !got.Blocked || got.Chars != 60 || got.Type != "supergroup" || got.Settings != gs {
		t.Errorf("migration lost the block, usage or settings: %+v", got)
	}
	if got, _, _ := c.Get(ctx, g1); got.Settings != (GroupSettings{}) {
		t.Errorf("another group's settings leaked: %+v", got.Settings)
	}
	if err := c.Migrate(ctx, g2, g3); err != nil {
		t.Errorf("migrating again is a no-op, got %v", err)
	}
}

func TestFileChats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chats.json")
	c, err := OpenFileChats(path)
	if err != nil {
		t.Fatal(err)
	}
	exerciseChats(t, c, -11, -12, -1000012, "2026-11-22", "2026-11-23")
	reopened, err := OpenFileChats(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := reopened.Get(context.Background(), -1000012); !ok || !got.Blocked {
		t.Errorf("registry did not survive a restart: %+v", got)
	}
}

// testChats is a Postgres registry with its own total row and group IDs far
// below any real chat, so tests never read or change production rows.
func testChats(t *testing.T, db *DB, n int) (pgChats, []int64) {
	t.Helper()
	base := -9_000_000_000_000 - (time.Now().UnixNano()%1_000_000)*100
	ids := make([]int64, n+1)
	for i := range ids {
		ids[i] = base - int64(i)
	}
	t.Cleanup(func() {
		_, _ = db.Pool().Exec(context.Background(), `DELETE FROM ellie_chats WHERE chat_id = ANY($1)`, ids)
	})
	return pgChats{db.Pool(), ids[0]}, ids[1:]
}

func TestPostgresChats(t *testing.T) {
	db := testDB(t)
	c, ids := testChats(t, db, 3)
	exerciseChats(t, c, ids[0], ids[1], ids[2], "1999-01-01", "1999-01-02")
}

func TestPostgresChatSpendsAreAtomic(t *testing.T) {
	db := testDB(t)
	c, ids := testChats(t, db, 1)
	ctx := context.Background()
	// 20 instances race for a cap of 10: exactly 10 may win.
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.Spend(ctx, ids[0], "1999-01-01", 1, 0, GroupLimits{Chars: 10}) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	got, _, _ := c.Get(ctx, ids[0])
	if wins.Load() != 10 || got.Chars != 10 {
		t.Errorf("%d spends won and usage is %d, want exactly 10", wins.Load(), got.Chars)
	}
}
