package store

// budget.go caps what the whole studio may use per UTC day, across private
// chats, groups and guest replies, so a sudden rush can't run through the
// Cartesia credits. Each user's own allowance still applies first.

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Budget is the studio-wide daily usage counter.
type Budget interface {
	// Spend adds chars and secs to day's total unless that would pass a
	// limit (0 = no limit). It reports whether the spend happened.
	Spend(ctx context.Context, day string, chars, secs, charLimit, secLimit int) (bool, error)
	// Refund takes back an earlier Spend.
	Refund(ctx context.Context, day string, chars, secs int) error
	// Used is day's total so far.
	Used(ctx context.Context, day string) (chars, secs int, err error)
}

// fits reports whether adding chars and secs to used stays within limits.
func fits(usedChars, usedSecs, chars, secs, charLimit, secLimit int) bool {
	return (charLimit <= 0 || chars <= 0 || usedChars+chars <= charLimit) &&
		(secLimit <= 0 || secs <= 0 || usedSecs+secs <= secLimit)
}

// ---------------------------------------------------------------- memory

type memBudget struct {
	mu   sync.Mutex
	day  string
	used [2]int // chars, secs
}

// NewMemBudget returns an in-memory counter, for tests and file-based setups.
func NewMemBudget() Budget { return &memBudget{} }

func (m *memBudget) roll(day string) {
	if m.day != day {
		m.day, m.used = day, [2]int{}
	}
}

func (m *memBudget) Spend(_ context.Context, day string, chars, secs, charLimit, secLimit int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roll(day)
	if !fits(m.used[0], m.used[1], chars, secs, charLimit, secLimit) {
		return false, nil
	}
	m.used[0] += chars
	m.used[1] += secs
	return true, nil
}

func (m *memBudget) Refund(_ context.Context, day string, chars, secs int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.day == day {
		m.used[0] = max(0, m.used[0]-chars)
		m.used[1] = max(0, m.used[1]-secs)
	}
	return nil
}

func (m *memBudget) Used(_ context.Context, day string) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roll(day)
	return m.used[0], m.used[1], nil
}

// ---------------------------------------------------------------- Postgres

// Budget returns the studio-wide counter, shared by every instance.
func (d *DB) Budget() Budget { return pgBudget{d.Pool()} }

type pgBudget struct{ pool *pgxpool.Pool }

// Spend is one statement: the row for day is created or added to only if the
// limits still hold, so instances running side by side can't overshoot.
func (p pgBudget) Spend(ctx context.Context, day string, chars, secs, charLimit, secLimit int) (bool, error) {
	if !fits(0, 0, chars, secs, charLimit, secLimit) {
		return false, nil // more than a whole day's budget at once
	}
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	var total int
	err := p.pool.QueryRow(ctx, `INSERT INTO ellie_studio_usage AS u (day, chars, secs) VALUES ($1, $2, $3)
		ON CONFLICT (day) DO UPDATE SET chars = u.chars + EXCLUDED.chars, secs = u.secs + EXCLUDED.secs, updated_at = now()
		WHERE ($4 <= 0 OR $2 <= 0 OR u.chars + EXCLUDED.chars <= $4)
		  AND ($5 <= 0 OR $3 <= 0 OR u.secs + EXCLUDED.secs <= $5)
		RETURNING chars`, day, chars, secs, charLimit, secLimit).Scan(&total)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("database: spend studio budget: %w", err)
	}
	return true, nil
}

func (p pgBudget) Refund(ctx context.Context, day string, chars, secs int) error {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `UPDATE ellie_studio_usage
		SET chars = GREATEST(0, chars - $2), secs = GREATEST(0, secs - $3), updated_at = now()
		WHERE day = $1`, day, chars, secs); err != nil {
		return fmt.Errorf("database: refund studio budget: %w", err)
	}
	return nil
}

func (p pgBudget) Used(ctx context.Context, day string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	var chars, secs int
	err := p.pool.QueryRow(ctx, `SELECT chars, secs FROM ellie_studio_usage WHERE day = $1`, day).Scan(&chars, &secs)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("database: read studio budget: %w", err)
	}
	return chars, secs, nil
}

// ---------------------------------------------------------------- outages

// guardedBudget keeps enforcing the cap while the database can't be
// reached, so an outage can't reopen the day's budget. A local count
// follows every spend this instance makes and is seeded from the database
// once a day. If today's total was never learned, spends are refused.
type guardedBudget struct {
	primary Budget

	mu    sync.Mutex
	day   string
	known bool // the local count includes the database's total for day
	chars int
	secs  int
}

// ErrBudgetUnknown means the cap couldn't be checked at all.
var ErrBudgetUnknown = errors.New("bot: studio budget unknown while the database is unreachable")

func (g *guardedBudget) roll(day string) {
	if g.day != day {
		g.day, g.known, g.chars, g.secs = day, false, 0, 0
	}
}

// seed learns day's total from the database the first time it's needed.
func (g *guardedBudget) seed(ctx context.Context, day string) {
	g.mu.Lock()
	g.roll(day)
	known := g.known
	g.mu.Unlock()
	if known {
		return
	}
	chars, secs, err := g.primary.Used(ctx, day)
	if err != nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.day == day && !g.known {
		g.known, g.chars, g.secs = true, g.chars+chars, g.secs+secs
	}
}

func (g *guardedBudget) Spend(ctx context.Context, day string, chars, secs, charLimit, secLimit int) (bool, error) {
	g.seed(ctx, day)
	ok, err := g.primary.Spend(ctx, day, chars, secs, charLimit, secLimit)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.roll(day)
	if err == nil {
		if ok {
			g.chars += chars
			g.secs += secs
		}
		return ok, nil
	}
	if !g.known {
		return false, errors.Join(ErrBudgetUnknown, err)
	}
	if !fits(g.chars, g.secs, chars, secs, charLimit, secLimit) {
		return false, err
	}
	g.chars += chars
	g.secs += secs
	return true, err
}

func (g *guardedBudget) Refund(ctx context.Context, day string, chars, secs int) error {
	err := g.primary.Refund(ctx, day, chars, secs)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.day == day {
		g.chars, g.secs = max(0, g.chars-chars), max(0, g.secs-secs)
	}
	return err
}

func (g *guardedBudget) Used(ctx context.Context, day string) (int, int, error) {
	chars, secs, err := g.primary.Used(ctx, day)
	if err == nil {
		return chars, secs, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.day == day && g.known {
		return g.chars, g.secs, nil
	}
	return 0, 0, err
}

// GuardBudget wraps b so the cap still holds while the database is
// unreachable.
func GuardBudget(b Budget) Budget { return &guardedBudget{primary: b} }
