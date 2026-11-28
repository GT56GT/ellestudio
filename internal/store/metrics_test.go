package store

import (
	"context"
	"testing"
	"time"
)

// exerciseMetrics runs the same checks against any Metrics. Events are
// placed on two days starting at base, far from real data.
func exerciseMetrics(t *testing.T, m Metrics, base time.Time) {
	t.Helper()
	ctx := context.Background()
	day1, day2 := base, base.Add(24*time.Hour)
	for _, e := range []Event{
		{At: day1, User: -1, Kind: EvStart, Source: SrcGuest},
		{At: day1, User: -1, Kind: EvReading, Source: "private", N: 10},
		{At: day1.Add(time.Minute), User: -1, Kind: EvReading, Source: "guest", N: 5},
		{At: day1, User: -2, Kind: EvStart, Source: SrcReferral},
		{At: day1, User: -2, Kind: EvLimit, Source: "speech"},
		{At: day1, User: -2, Kind: EvLimit, Source: "speech"}, // the same user twice
		{At: day1, User: -3, Kind: EvGuestInvite},             // shown an invite, never used Ellie
		{At: day2, User: -3, Kind: EvStart, Source: SrcGroup},
		{At: day2, User: -3, Kind: EvTranscribe, N: 30},
	} {
		if err := m.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	days, err := m.Days(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	var got []DayStats
	for _, d := range days {
		if d.Day == day1.Format("2006-01-02") || d.Day == day2.Format("2006-01-02") {
			got = append(got, d)
		}
	}
	want := []DayStats{
		{Day: day2.Format("2006-01-02"), Active: 1, New: 1, FromGroups: 1},
		{Day: day1.Format("2006-01-02"), Active: 2, New: 2, FromGroups: 1, FromInvites: 1, Readings: 2, Chars: 15, LimitUsers: 1, GuestInvites: 1},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("days =\n%+v\nwant\n%+v", got, want)
	}

	if n, err := m.Active(ctx, base); err != nil || n < 3 {
		t.Errorf("active = %d, %v; want at least the 3 test users", n, err)
	}

	if err := m.Prune(ctx, day2); err != nil {
		t.Fatal(err)
	}
	days, _ = m.Days(ctx, base)
	for _, d := range days {
		if d.Day == day1.Format("2006-01-02") {
			t.Errorf("pruned day still reported: %+v", d)
		}
	}
}

func TestMemMetrics(t *testing.T) {
	exerciseMetrics(t, NewMemMetrics(), time.Date(2001, 1, 2, 10, 0, 0, 0, time.UTC))
}

func TestPostgresMetrics(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	// Test events live in January 2001 with negative user IDs: no real
	// event can be there, and they are all removed afterwards.
	cleanup := func() {
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_events WHERE at < '2001-02-01' AND user_id < 0`)
	}
	cleanup()
	t.Cleanup(cleanup)
	exerciseMetrics(t, db.Metrics(), time.Date(2001, 1, 2, 10, 0, 0, 0, time.UTC))
}

func TestMemMetricsKeepsTheNewest(t *testing.T) {
	m := NewMemMetrics().(*memMetrics)
	ctx := context.Background()
	for i := range memMetricsCap + 5 {
		_ = m.Record(ctx, Event{At: time.Unix(int64(i), 0), User: int64(i), Kind: EvReading})
	}
	if len(m.events) > memMetricsCap {
		t.Fatalf("kept %d events, cap %d", len(m.events), memMetricsCap)
	}
	if last := m.events[len(m.events)-1]; last.User != memMetricsCap+4 {
		t.Errorf("newest event lost, last kept is user %d", last.User)
	}
}
