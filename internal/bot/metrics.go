package bot

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Metrics answers "is Ellie growing, and is it worth it?" with a few numbers
// per UTC day: active users, readings per user, new users and where they came
// from, and who ran out of allowance. It records small events (who, what,
// when, how much) and nothing a user said. Events are kept for
// metricsRetention, then pruned.

// Event kinds.
const (
	evStart       = "start"        // a new user opened the private chat (Source says how)
	evReading     = "reading"      // a finished reading (Source: private, group, guest; N: characters)
	evTranscribe  = "transcribe"   // a finished transcription (N: seconds)
	evLimit       = "limit"        // a request refused by the daily allowance (Source: speech, transcription)
	evGuestInvite = "guest_invite" // a "Start Ellie" invite shown to a stranger in another chat
)

// Where new users come from (evStart sources).
const (
	srcDirect   = "direct"   // found the bot on their own
	srcGroup    = "group"    // a "private chat" link under a group message
	srcGuest    = "guest"    // a guest-mode voice or invite in another chat
	srcReferral = "referral" // a friend's invite link
	srcShared   = "shared"   // a shared voice link
)

// metricsRetention is how long events are kept.
const metricsRetention = 90 * 24 * time.Hour

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

// track records an event without ever failing or slowing the request.
func (b *Bot) track(ctx context.Context, uid int64, kind, source string, n int) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := b.metrics.Record(ctx, Event{At: time.Now().UTC(), User: uid, Kind: kind, Source: source, N: n}); err != nil {
		b.log.Warn("record metric", "kind", kind, "err", err)
	}
}

// SetMetrics replaces the in-memory metrics (used when no database is set).
func (b *Bot) SetMetrics(m Metrics) { b.metrics = m }

// pruneMetrics forgets old events now and then once a day until ctx ends.
func (b *Bot) pruneMetrics(ctx context.Context) {
	for {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := b.metrics.Prune(pctx, time.Now().Add(-metricsRetention)); err != nil && ctx.Err() == nil {
			b.log.Warn("prune metrics", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}

// activeKind reports whether an event kind means the user used Ellie.
func activeKind(kind string) bool {
	return kind == evStart || kind == evReading || kind == evTranscribe
}

// memMetrics keeps events in memory: for tests and file-based setups, where
// stats start over on every restart. It keeps the newest memMetricsCap.
type memMetrics struct {
	mu     sync.Mutex
	events []Event
}

const memMetricsCap = 100_000

func newMemMetrics() *memMetrics { return &memMetrics{} }

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
		case evStart:
			ds.New++
			switch e.Source {
			case srcGroup, srcGuest:
				ds.FromGroups++
			case srcReferral:
				ds.FromInvites++
			}
		case evReading:
			ds.Readings++
			ds.Chars += e.N
		case evLimit:
			seen[d].limited[e.User] = true
		case evGuestInvite:
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
