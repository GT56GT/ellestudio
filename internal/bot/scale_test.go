package bot

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// exerciseBudget checks the studio-wide counter contract on any backend.
func exerciseBudget(t *testing.T, bu Budget, day string) {
	t.Helper()
	ctx := context.Background()
	spend := func(chars, secs int) bool {
		t.Helper()
		ok, err := bu.Spend(ctx, day, chars, secs, 100, 60)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !spend(60, 0) || !spend(40, 30) {
		t.Fatal("spends within the cap refused")
	}
	if spend(1, 0) {
		t.Fatal("spend past the character cap allowed")
	}
	if spend(0, 31) {
		t.Fatal("spend past the audio cap allowed")
	}
	if !spend(0, 30) {
		t.Fatal("audio up to the cap refused")
	}
	if err := bu.Refund(ctx, day, 50, 10); err != nil {
		t.Fatal(err)
	}
	if c, s, err := bu.Used(ctx, day); err != nil || c != 50 || s != 50 {
		t.Fatalf("used = %d chars, %d secs (%v), want 50, 50", c, s, err)
	}
	if ok, _ := bu.Spend(ctx, day, 101, 0, 100, 0); ok {
		t.Fatal("a single spend bigger than the cap allowed")
	}
	// Racing spends never overshoot.
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = bu.Spend(ctx, day, 5, 0, 100, 0) }()
	}
	wg.Wait()
	if c, _, _ := bu.Used(ctx, day); c != 100 {
		t.Fatalf("after racing spends used = %d, want exactly 100", c)
	}
}

func TestMemBudget(t *testing.T) { exerciseBudget(t, newMemBudget(), "2026-11-27") }

func TestPostgresBudget(t *testing.T) {
	db := testDB(t)
	const day = "2001-01-01" // never a real day, so tests can't touch live rows
	ctx := context.Background()
	cleanup := func() { _, _ = db.pool.Exec(ctx, `DELETE FROM ellie_studio_usage WHERE day = $1`, day) }
	cleanup()
	t.Cleanup(cleanup)
	exerciseBudget(t, db.Budget(), day)
}

func TestStudioCapStopsEveryoneAndRefunds(t *testing.T) {
	fx := newFixture(t, Config{
		AdminUsers: map[int64]bool{1: true}, DailyChars: 2000, StudioDailyChars: 10,
	})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "hello"))
	if len(fx.tg.find("sendVoice")) != 1 {
		t.Fatal("a reading within the studio cap was refused")
	}
	fx.bot.handleUpdate(ctx, msg(8, "hello world"))
	fx.bot.wg.Wait()
	if got := fx.tg.lastTextTo(8); !strings.Contains(got, "resting for today") {
		t.Fatalf("user told %q", got)
	}
	if u := fx.bot.store.UsageOn(8, today()); u.Chars != 0 {
		t.Fatalf("refused user charged %d characters", u.Chars)
	}
	if c, _, _ := fx.bot.budget.Used(ctx, today()); c != 5 {
		t.Fatalf("studio used %d, want 5", c)
	}
	if got := fx.tg.lastTextTo(1); !strings.Contains(got, "Studio daily cap reached") {
		t.Fatalf("admin alert = %q", got)
	}
	// Admins aren't held back by the studio cap.
	fx.bot.handleUpdate(ctx, msg(1, "hello world"))
	if len(fx.tg.find("sendVoice")) != 2 {
		t.Fatal("admin reading refused by the studio cap")
	}
}

// exerciseConcurrentSpends spends from many goroutines through stores (one
// per bot instance) and checks that no spend was lost.
func exerciseConcurrentSpends(t *testing.T, stores []*Store, uid int64) {
	t.Helper()
	const day, perSpend, spends = "2001-01-02", 7, 40
	var wg sync.WaitGroup
	for i := range spends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _, err := stores[i%len(stores)].Spend(uid, day, perSpend, 0, 0, 0); !ok || err != nil {
				t.Errorf("spend: ok=%v err=%v", ok, err)
			}
		}()
	}
	wg.Wait()
	for _, s := range stores {
		if u := s.UsageOn(uid, day); u.Chars != perSpend*spends {
			t.Fatalf("usage = %d, want %d: spends were lost", u.Chars, perSpend*spends)
		}
	}
}

func TestFileStoreConcurrentSpends(t *testing.T) {
	s, err := OpenStore(t.TempDir()+"/settings.json", Settings{})
	if err != nil {
		t.Fatal(err)
	}
	exerciseConcurrentSpends(t, []*Store{s}, 1)
}

// Two instances share the database during a deploy; neither may overwrite
// the other's changes.
func TestPostgresStoreSharedByTwoInstances(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const uid, inviter, inviteeA = -9101, -9102, -9103
	cleanup := func() {
		_, _ = db.pool.Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id = ANY($1)`, []int64{uid, inviter, inviteeA})
	}
	cleanup()
	t.Cleanup(cleanup)
	open := func() *Store {
		s, err := db.SettingsStore(ctx, Settings{})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a, b := open(), open()
	exerciseConcurrentSpends(t, []*Store{a, b}, uid)

	// A change made by one instance is seen by the other at once.
	if _, err := a.Update(uid, func(v *Settings) { v.Emotion = "calm" }); err != nil {
		t.Fatal(err)
	}
	if got := b.Get(uid).Emotion; got != "calm" {
		t.Fatalf("other instance sees emotion %q", got)
	}

	// The same newcomer arriving through two links at once is rewarded once.
	if err := a.Join(inviter, "2001-01-02"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied := 0
	for _, s := range []*Store{a, b, a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := s.Referral(inviter, inviteeA, "2001-01-02", 100, 5)
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				applied++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if applied != 1 || a.Get(inviter).Bonus != 100 {
		t.Fatalf("referral applied %d times, inviter bonus %d", applied, a.Get(inviter).Bonus)
	}
}
