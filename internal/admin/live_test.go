package admin

import (
	"net/http"
	"strings"
	"testing"
)

func TestLimitsAreValidatedSavedAndAudited(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	if w := hs.act(t, c, "/api/settings", `{"reason":"too low","overrides":{"daily_chars":-1}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("negative limit: %d", w.Code)
	}
	hs.store.overrides.Paused = true
	w := hs.act(t, c, "/api/settings", `{"reason":"busy week","overrides":{"daily_chars":1500,"paused":false}}`)
	if w.Code != 200 || hs.store.overrides.DailyChars == nil || *hs.store.overrides.DailyChars != 1500 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if !hs.store.overrides.Paused {
		t.Fatal("saving limits must not lift the pause; it has its own switch")
	}
	if e := hs.lastAudit(); e.Action != "limits_changed" || !strings.Contains(string(e.Detail), "busy week") {
		t.Fatalf("audit = %+v", e)
	}
}

func TestEmergencyPauseSwitch(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	if w := hs.act(t, c, "/api/pause", `{"reason":"Cartesia incident","paused":true}`); w.Code != 200 || !hs.store.overrides.Paused {
		t.Fatalf("pause: %d", w.Code)
	}
	if hs.lastAudit().Action != "voice_paused" {
		t.Fatal("pause not audited")
	}
	if w := hs.act(t, c, "/api/pause", `{"reason":"all clear","paused":false}`); w.Code != 200 || hs.store.overrides.Paused {
		t.Fatalf("resume: %d", w.Code)
	}
	if hs.lastAudit().Action != "voice_resumed" {
		t.Fatal("resume not audited")
	}
}

func TestBroadcastFlow(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	w := hs.act(t, c, "/api/broadcasts/test", `{"text":"New voices <b>today</b>"}`)
	if w.Code != 200 || !strings.Contains(hs.tg.sent[42], "New voices &lt;b&gt;today") || !strings.Contains(hs.tg.sent[42], "Test") {
		t.Fatalf("test send: %d %s -> %q", w.Code, w.Body, hs.tg.sent[42])
	}
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"reason":"launch","text":"","audience":"everyone"}`, http.StatusBadRequest},
		{`{"reason":"launch","text":"hi","audience":"the moon"}`, http.StatusBadRequest},
		{`{"reason":"","text":"hi","audience":"everyone"}`, http.StatusBadRequest},
		{`{"reason":"launch","text":"New voices!","audience":"everyone"}`, http.StatusOK},
		{`{"reason":"again","text":"Second one","audience":"active_30d"}`, http.StatusConflict},
	} {
		if w := hs.act(t, c, "/api/broadcasts", tc.body); w.Code != tc.want {
			t.Errorf("%s: %d, want %d (%s)", tc.body, w.Code, tc.want, w.Body)
		}
	}
	if e := hs.store.audit[len(hs.store.audit)-1]; e.Action != "broadcast_queued" {
		t.Fatalf("last audit = %s", e.Action)
	}
	if e := hs.lastAudit(); e.Action != "broadcast_queued" || !strings.Contains(string(e.Detail), `"recipients":120`) {
		t.Fatalf("audit = %+v %s", e, e.Detail)
	}
	if w := hs.act(t, c, "/api/broadcasts/1/cancel", `{"reason":"typo"}`); w.Code != 200 || hs.store.broadcasts[0].Status != "cancelled" {
		t.Fatalf("cancel: %d", w.Code)
	}
	if w := hs.act(t, c, "/api/broadcasts/1/cancel", `{"reason":"twice"}`); w.Code != http.StatusConflict {
		t.Fatalf("second cancel: %d", w.Code)
	}
}
