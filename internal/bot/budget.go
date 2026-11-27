package bot

// budget.go caps what the whole studio may use per UTC day, across private
// chats, groups and guest replies, so a sudden rush can't run through the
// Cartesia credits. Each user's own allowance still applies first.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

func newMemBudget() *memBudget { return &memBudget{} }

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
func (d *DB) Budget() Budget { return pgBudget{d.pool} }

type pgBudget struct{ pool *pgxpool.Pool }

// Spend is one statement: the row for day is created or added to only if the
// limits still hold, so instances running side by side can't overshoot.
func (p pgBudget) Spend(ctx context.Context, day string, chars, secs, charLimit, secLimit int) (bool, error) {
	if !fits(0, 0, chars, secs, charLimit, secLimit) {
		return false, nil // more than a whole day's budget at once
	}
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
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
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	if _, err := p.pool.Exec(ctx, `UPDATE ellie_studio_usage
		SET chars = GREATEST(0, chars - $2), secs = GREATEST(0, secs - $3), updated_at = now()
		WHERE day = $1`, day, chars, secs); err != nil {
		return fmt.Errorf("database: refund studio budget: %w", err)
	}
	return nil
}

func (p pgBudget) Used(ctx context.Context, day string) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
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

// ---------------------------------------------------------------- the bot side

// SetBudget replaces the in-memory studio counter (use the database's so
// every instance shares one).
func (b *Bot) SetBudget(bu Budget) {
	if bu != nil {
		b.budget = bu
	}
}

func (b *Bot) studioCapped() bool {
	return b.cfg.StudioDailyChars > 0 || b.cfg.StudioDailyAudioSecs > 0
}

// spendStudio takes chars and secs from today's studio-wide budget. A
// database error lets the request through: the budget guards cost, and a
// blip shouldn't stop the bot.
func (b *Bot) spendStudio(chars, secs int) bool {
	if !b.studioCapped() {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	day := today()
	ok, err := b.budget.Spend(ctx, day, chars, secs, b.cfg.StudioDailyChars, b.cfg.StudioDailyAudioSecs)
	if err != nil {
		b.log.Warn("studio budget unavailable; allowing", "err", err)
		return true
	}
	if !ok {
		b.log.Warn("studio daily cap reached", "chars", chars, "secs", secs)
		b.alertAdmins("budget:"+day, header("🎟", "Studio daily cap reached", "")+
			"Everyone together has used today's studio-wide allowance, so new readings and transcriptions are refused until midnight UTC.\n\n"+
			"Raise <code>STUDIO_DAILY_CHAR_LIMIT</code> or <code>STUDIO_DAILY_TRANSCRIBE_MINUTES</code> on the server if you want more.")
	}
	return ok
}

func (b *Bot) refundStudio(chars, secs int) {
	if !b.studioCapped() || (chars <= 0 && secs <= 0) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	if err := b.budget.Refund(ctx, today(), chars, secs); err != nil {
		b.log.Warn("refund studio budget", "err", err)
	}
}

// studioFullScreen explains that the whole studio hit today's cap.
func studioFullScreen() screen {
	return noticeScreen("🎟", "Ellie is resting for today",
		"So many people used Ellie today that the studio reached its daily limit. Nothing was charged.\n\n"+
			"🔄 It opens again in <b>"+fmtWait(untilReset())+"</b> (midnight UTC).")
}

// studioLines is the studio-wide status for the admin Stats screen.
func (b *Bot) studioLines(ctx context.Context) string {
	out := ""
	if chars, secs, err := b.budget.Used(ctx, today()); err == nil && b.studioCapped() {
		c := fmtInt(chars)
		if b.cfg.StudioDailyChars > 0 {
			c += " of " + fmtInt(b.cfg.StudioDailyChars)
		}
		a := fmtMinutes(secs)
		if b.cfg.StudioDailyAudioSecs > 0 {
			a += " of " + fmtMinutes(b.cfg.StudioDailyAudioSecs)
		}
		out += field("🎟", "Studio today", c+" characters · "+a)
	}
	if open, left := b.breaker.isOpen(); open {
		out += field("⚠️", "Voice work paused", "for "+fmtWait(left.Round(time.Second)))
	}
	if q := b.jobs.queued(); q > 0 {
		out += field("⏳", "Waiting in line", fmtInt(q))
	}
	return out
}
