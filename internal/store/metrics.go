package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Metrics answers "is Ellie growing, and is it worth it?" with a few numbers
// per UTC day: active users, readings per user, new users and where they came
// from, and who ran out of allowance. It records small events (who, what,
// when, how much) and nothing a user said.

// Event kinds.
const (
	EvStart       = "start"        // a new user opened the private chat (Source says how)
	EvReading     = "reading"      // a finished reading (Source: private, group, guest; N: characters)
	EvTranscribe  = "transcribe"   // a finished transcription (N: seconds)
	EvLimit       = "limit"        // a request refused by the daily allowance (Source: speech, transcription)
	EvGuestInvite = "guest_invite" // a "Start Ellie" invite shown to a stranger in another chat
)

// Where new users come from (EvStart sources).
const (
	SrcDirect   = "direct"   // found the bot on their own
	SrcGroup    = "group"    // a "private chat" link under a group message
	SrcGuest    = "guest"    // a guest-mode voice or invite in another chat
	SrcReferral = "referral" // a friend's invite link
	SrcShared   = "shared"   // a shared voice link
)

// Event is one thing that happened.
type Event struct {
	At     time.Time
	User   int64
	Kind   string
	Source string
	N      int
}

// DayStats summarises one UTC day.
type DayStats struct {
	Day          string // 2006-01-02
	Active       int    // distinct users who started, read or transcribed
	New          int    // new users
	FromGroups   int    // new users who came from a group or guest-mode link
	FromInvites  int    // new users who came from a friend's invite
	Readings     int
	Chars        int
	LimitUsers   int // distinct users refused by their daily allowance
	GuestInvites int // "Start Ellie" invites shown in other chats
}

// Metrics stores events and summarises them. Implementations must be safe
// for concurrent use.
type Metrics interface {
	Record(ctx context.Context, e Event) error
	// Days summarises each UTC day from since on, newest first.
	Days(ctx context.Context, since time.Time) ([]DayStats, error)
	// Active counts distinct active users from since on.
	Active(ctx context.Context, since time.Time) (int, error)
	// Prune forgets events before before.
	Prune(ctx context.Context, before time.Time) error
}

// activeKind reports whether an event kind means the user used Ellie.
func activeKind(kind string) bool {
	return kind == EvStart || kind == EvReading || kind == EvTranscribe
}

// memMetrics keeps events in memory: for tests and file-based setups, where
// stats start over on every restart. It keeps the newest memMetricsCap.
type memMetrics struct {
	mu     sync.Mutex
	events []Event
}

const memMetricsCap = 100_000

// NewMemMetrics returns in-memory events, for tests and file-based setups.
func NewMemMetrics() Metrics { return &memMetrics{} }

func (m *memMetrics) Record(_ context.Context, e Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) >= memMetricsCap {
		m.events = append(m.events[:0], m.events[len(m.events)/10:]...)
	}
	m.events = append(m.events, e)
	return nil
}

func (m *memMetrics) Days(_ context.Context, since time.Time) ([]DayStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	type sets struct{ active, limited map[int64]bool }
	days, seen := map[string]*DayStats{}, map[string]sets{}
	for _, e := range m.events {
		if e.At.Before(since) {
			continue
		}
		d := e.At.UTC().Format("2006-01-02")
		ds, ok := days[d]
		if !ok {
			ds = &DayStats{Day: d}
			days[d], seen[d] = ds, sets{map[int64]bool{}, map[int64]bool{}}
		}
		if activeKind(e.Kind) {
			seen[d].active[e.User] = true
		}
		switch e.Kind {
		case EvStart:
			ds.New++
			switch e.Source {
			case SrcGroup, SrcGuest:
				ds.FromGroups++
			case SrcReferral:
				ds.FromInvites++
			}
		case EvReading:
			ds.Readings++
			ds.Chars += e.N
		case EvLimit:
			seen[d].limited[e.User] = true
		case EvGuestInvite:
			ds.GuestInvites++
		}
	}
	out := make([]DayStats, 0, len(days))
	for d, ds := range days {
		ds.Active, ds.LimitUsers = len(seen[d].active), len(seen[d].limited)
		out = append(out, *ds)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day > out[j].Day })
	return out, nil
}

func (m *memMetrics) Active(_ context.Context, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	users := map[int64]bool{}
	for _, e := range m.events {
		if !e.At.Before(since) && activeKind(e.Kind) {
			users[e.User] = true
		}
	}
	return len(users), nil
}

func (m *memMetrics) Prune(_ context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.events[:0]
	for _, e := range m.events {
		if !e.At.Before(before) {
			kept = append(kept, e)
		}
	}
	m.events = kept
	return nil
}

// ---------------------------------------------------------------- Postgres

// Metrics returns the Postgres-backed usage events.
func (d *DB) Metrics() Metrics { return pgMetrics{d.Pool()} }

type pgMetrics struct{ pool *pgxpool.Pool }

func (p pgMetrics) Record(ctx context.Context, e Event) error {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `INSERT INTO ellie_events (at, user_id, kind, source, n) VALUES ($1, $2, $3, $4, $5)`,
		e.At, e.User, e.Kind, e.Source, e.N)
	if err != nil {
		return fmt.Errorf("database: record event: %w", err)
	}
	return nil
}

func (p pgMetrics) Days(ctx context.Context, since time.Time) ([]DayStats, error) {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT to_char(at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day,
			count(DISTINCT user_id) FILTER (WHERE kind IN ('start', 'reading', 'transcribe')),
			count(*) FILTER (WHERE kind = 'start'),
			count(*) FILTER (WHERE kind = 'start' AND source IN ('group', 'guest')),
			count(*) FILTER (WHERE kind = 'start' AND source = 'referral'),
			count(*) FILTER (WHERE kind = 'reading'),
			COALESCE(sum(n) FILTER (WHERE kind = 'reading'), 0),
			count(DISTINCT user_id) FILTER (WHERE kind = 'limit'),
			count(*) FILTER (WHERE kind = 'guest_invite')
		FROM ellie_events WHERE at >= $1 GROUP BY day ORDER BY day DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("database: summarise events: %w", err)
	}
	defer rows.Close()
	var out []DayStats
	for rows.Next() {
		var d DayStats
		if err := rows.Scan(&d.Day, &d.Active, &d.New, &d.FromGroups, &d.FromInvites,
			&d.Readings, &d.Chars, &d.LimitUsers, &d.GuestInvites); err != nil {
			return nil, fmt.Errorf("database: summarise events: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: summarise events: %w", err)
	}
	return out, nil
}

func (p pgMetrics) Active(ctx context.Context, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	var n int
	err := p.pool.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM ellie_events
		WHERE at >= $1 AND kind IN ('start', 'reading', 'transcribe')`, since).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database: count active users: %w", err)
	}
	return n, nil
}

func (p pgMetrics) Prune(ctx context.Context, before time.Time) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM ellie_events WHERE at < $1`, before)
	if err != nil {
		return fmt.Errorf("database: prune events: %w", err)
	}
	return nil
}
