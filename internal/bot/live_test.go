package bot

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

type fakeConfig struct{ o LimitOverrides }

func (f *fakeConfig) LoadOverrides(context.Context) (LimitOverrides, error) { return f.o, nil }

func intp(v int) *int { return &v }

func TestPanelLimitsApplyLive(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyChars: 2000})
	cfg := &fakeConfig{o: LimitOverrides{DailyChars: intp(5)}}
	fx.bot.SetConfigSource(cfg)
	ctx := context.Background()
	fx.bot.refreshLive(ctx)
	if got := fx.bot.lim().DailyChars; got != 5 {
		t.Fatalf("daily chars = %d, want the panel's 5", got)
	}
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	if len(fx.tg.find("sendVoice")) != 0 {
		t.Fatal("the lowered limit wasn't applied")
	}
	cfg.o = LimitOverrides{} // the admin resets to the default
	fx.bot.refreshLive(ctx)
	if got := fx.bot.lim().DailyChars; got != 2000 {
		t.Fatalf("daily chars = %d, want the env default back", got)
	}
}

func TestEmergencyPauseStopsVoiceWork(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyChars: 2000})
	fx.bot.applyOverrides(LimitOverrides{Paused: true})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	if len(fx.tg.find("sendVoice")) != 0 || !strings.Contains(fx.tg.lastTextTo(7), "taking a short break") {
		t.Fatalf("paused reading: voices %d, told %q", len(fx.tg.find("sendVoice")), fx.tg.lastTextTo(7))
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
		t.Fatalf("charged %d characters while paused", u.Chars)
	}
	if !fx.bot.status(fx.bot.started).AdminPaused {
		t.Fatal("the heartbeat doesn't report the pause")
	}
}

func TestOverridesValidate(t *testing.T) {
	for _, o := range []LimitOverrides{{DailyChars: intp(-1)}, {MaxTTSChars: intp(0)}, {GroupCooldownSecs: intp(99999)}} {
		if o.Validate() == nil {
			t.Errorf("%+v accepted", o)
		}
	}
	if err := (LimitOverrides{DailyChars: intp(0), StudioDailyChars: intp(50000)}).Validate(); err != nil {
		t.Fatal(err)
	}
}

// memBroadcasts is a BroadcastQueue in memory.
type memBroadcasts struct {
	mu        sync.Mutex
	pending   []Broadcast
	users     []int64
	sent      int
	failed    int
	batches   int
	cancelled bool
	finished  bool
}

func (m *memBroadcasts) ClaimBroadcast(context.Context) (Broadcast, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pending) == 0 {
		return Broadcast{}, false, nil
	}
	b := m.pending[0]
	m.pending = m.pending[1:]
	return b, true, nil
}

func (m *memBroadcasts) Recipients(_ context.Context, b Broadcast, limit int) ([]int64, error) {
	var out []int64
	for _, u := range m.users {
		if u > b.Cursor && len(out) < limit {
			out = append(out, u)
		}
	}
	return out, nil
}

func (m *memBroadcasts) Progress(_ context.Context, _, _ int64, sent, failed int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent += sent
	m.failed += failed
	m.batches++
	return m.cancelled, nil
}

func (m *memBroadcasts) FinishBroadcast(context.Context, int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finished = true
	return nil
}

func TestBroadcastReachesEveryoneWithAnOptOut(t *testing.T) {
	old := broadcastGap
	broadcastGap = 0
	t.Cleanup(func() { broadcastGap = old })
	fx := newFixture(t, Config{})
	q := &memBroadcasts{users: []int64{7, 8, 9}}
	fx.bot.SetBroadcasts(q)
	fx.bot.sendBroadcast(context.Background(), Broadcast{ID: 1, Text: "New voices are here <3", Audience: AudienceEveryone})
	if !q.finished || q.sent != 3 || q.failed != 0 {
		t.Fatalf("finished=%v sent=%d failed=%d", q.finished, q.sent, q.failed)
	}
	msgs := fx.tg.find("sendMessage")
	if len(msgs) != 3 {
		t.Fatalf("sent %d messages", len(msgs))
	}
	text, _ := msgs[0].JSON["text"].(string)
	kb, _ := msgs[0].JSON["reply_markup"].(map[string]any)
	if !strings.Contains(text, "New voices are here &lt;3") || kb == nil || !strings.Contains(strings.ToLower(toJSON(kb)), "news:off") {
		t.Fatalf("message = %q, buttons %v", text, kb)
	}
}

func TestBroadcastStopsWhenCancelled(t *testing.T) {
	old := broadcastGap
	broadcastGap = 0
	t.Cleanup(func() { broadcastGap = old })
	fx := newFixture(t, Config{})
	users := make([]int64, broadcastBatch*3)
	for i := range users {
		users[i] = int64(100 + i)
	}
	q := &memBroadcasts{users: users, cancelled: true}
	fx.bot.SetBroadcasts(q)
	fx.bot.sendBroadcast(context.Background(), Broadcast{ID: 1, Text: "hi", Audience: AudienceEveryone})
	if q.batches != 1 || q.finished {
		t.Fatalf("batches=%d finished=%v: a cancelled broadcast kept going", q.batches, q.finished)
	}
}

func TestNewsButtonTogglesAnnouncements(t *testing.T) {
	fx := newFixture(t, Config{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, press(7, "news:off"))
	if !fx.bot.store.Get(7).NoNews {
		t.Fatal("opt-out not saved")
	}
	if len(fx.tg.find("editMessageReplyMarkup")) != 1 {
		t.Fatal("the button wasn't swapped for an opt-in")
	}
	fx.bot.handleUpdate(ctx, press(7, "news:on"))
	if fx.bot.store.Get(7).NoNews {
		t.Fatal("opt-in not saved")
	}
}

func toJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
