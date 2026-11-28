package store

import (
	"context"
	"errors"
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

func TestMemBudget(t *testing.T) { exerciseBudget(t, NewMemBudget(), "2026-11-27") }

func TestPostgresBudget(t *testing.T) {
	db := testDB(t)
	const day = "2001-01-01" // never a real day, so tests can't touch live rows
	ctx := context.Background()
	cleanup := func() { _, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_studio_usage WHERE day = $1`, day) }
	cleanup()
	t.Cleanup(cleanup)
	exerciseBudget(t, db.Budget(), day)
}

// flakyBudget is a database-backed budget whose database can go away.
type flakyBudget struct {
	Budget
	down bool
}

var errBudgetDBDown = errors.New("database unreachable")

func (f *flakyBudget) Spend(ctx context.Context, day string, c, s, cl, sl int) (bool, error) {
	if f.down {
		return false, errBudgetDBDown
	}
	return f.Budget.Spend(ctx, day, c, s, cl, sl)
}

func (f *flakyBudget) Used(ctx context.Context, day string) (int, int, error) {
	if f.down {
		return 0, 0, errBudgetDBDown
	}
	return f.Budget.Used(ctx, day)
}

func TestStudioCapHoldsDuringDatabaseOutage(t *testing.T) {
	ctx := context.Background()
	const day = "2026-11-27"
	db := &flakyBudget{Budget: NewMemBudget()}
	_, _ = db.Budget.Spend(ctx, day, 60, 0, 0, 0) // another instance spent 60 earlier
	g := &guardedBudget{primary: db}
	if ok, err := g.Spend(ctx, day, 10, 0, 100, 0); !ok || err != nil {
		t.Fatalf("spend while up: %v %v", ok, err)
	}
	db.down = true
	if ok, _ := g.Spend(ctx, day, 30, 0, 100, 0); !ok {
		t.Fatal("a spend within the cap was refused during the outage")
	}
	if ok, _ := g.Spend(ctx, day, 1, 0, 100, 0); ok {
		t.Fatal("the outage reopened the budget: 60+10+30 is already the cap")
	}
	if c, _, err := g.Used(ctx, day); err != nil || c != 100 {
		t.Fatalf("local count = %d (%v), want 100", c, err)
	}

	// A day never seen while the database was up can't be checked: refuse.
	if ok, err := g.Spend(ctx, "2026-11-28", 1, 0, 100, 0); ok || !errors.Is(err, ErrBudgetUnknown) {
		t.Fatalf("unknown day: ok=%v err=%v, want a refusal", ok, err)
	}
}
