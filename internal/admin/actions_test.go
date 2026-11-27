package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// act posts an action as a signed-in admin from the panel.
func (hs *harness) act(t *testing.T, c *http.Cookie, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.AddCookie(c)
	r.Header.Set("X-Ellie-Admin", "1")
	r.Header.Set("Origin", "https://panel.example")
	return hs.do(r)
}

func (hs *harness) lastAudit() AuditEntry {
	hs.store.mu.Lock()
	defer hs.store.mu.Unlock()
	return hs.store.audit[len(hs.store.audit)-1]
}

func TestActionsNeedAReasonAndASession(t *testing.T) {
	hs := newHarness(t)
	r := httptest.NewRequest("POST", "/api/users/7/bonus", strings.NewReader(`{"amount":10,"reason":"thanks"}`))
	if w := hs.do(r); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", w.Code)
	}
	c := hs.signIn(t, 42)
	if w := hs.act(t, c, "/api/users/7/bonus", `{"amount":10,"reason":" "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("without a reason: %d", w.Code)
	}
	r = httptest.NewRequest("POST", "/api/users/7/bonus", strings.NewReader(`{"amount":10,"reason":"thanks"}`))
	r.AddCookie(c) // no CSRF header
	if w := hs.do(r); w.Code != http.StatusForbidden {
		t.Fatalf("without the panel's header: %d", w.Code)
	}
}

func TestGiveBonusNotifiesAndAudits(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	w := hs.act(t, c, "/api/users/7/bonus", `{"amount":500,"reason":"Sorry for the outage","notify":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"after":600`) {
		t.Fatalf("bonus: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(hs.tg.sent[7], "500 bonus characters") {
		t.Fatalf("user told %q", hs.tg.sent[7])
	}
	e := hs.lastAudit()
	if e.Action != "bonus_given" || e.Target != "user:7" || !strings.Contains(string(e.Detail), "Sorry for the outage") {
		t.Fatalf("audit = %+v %s", e, e.Detail)
	}
	for _, body := range []string{`{"amount":0,"reason":"zero"}`, `{"amount":100001,"reason":"too much"}`} {
		if w := hs.act(t, c, "/api/users/7/bonus", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	if w := hs.act(t, c, "/api/users/999/bonus", `{"amount":5,"reason":"nobody"}`); w.Code != http.StatusNotFound {
		t.Errorf("unknown user: %d", w.Code)
	}
}

func TestNotifyFailureIsReportedNotFatal(t *testing.T) {
	hs := newHarness(t)
	hs.tg.failTo = 7
	c := hs.signIn(t, 42)
	w := hs.act(t, c, "/api/users/7/bonus", `{"amount":5,"reason":"thanks","notify":true}`)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out["notified"] != false || out["notify_error"] == "" {
		t.Fatalf("bonus with a blocked user: %d %s", w.Code, w.Body)
	}
}

func TestBanRules(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	if w := hs.act(t, c, "/api/users/42/ban", `{"days":7,"reason":"testing"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("banning an admin: %d", w.Code)
	}
	if w := hs.act(t, c, "/api/users/7/ban", `{"days":3,"reason":"odd length"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a 3-day ban: %d", w.Code)
	}
	w := hs.act(t, c, "/api/users/7/ban", `{"days":7,"reason":"spamming groups"}`)
	if w.Code != 200 {
		t.Fatalf("ban: %d %s", w.Code, w.Body)
	}
	b := hs.store.users[7].Ban
	if b == nil || b.Until == nil || b.Until.Sub(testNow).Hours() != 7*24 || b.By != 42 || b.Reason != "spamming groups" {
		t.Fatalf("ban = %+v", b)
	}
	if hs.lastAudit().Action != "user_banned" {
		t.Fatal("ban not audited")
	}
	w = hs.act(t, c, "/api/users/7/unban", `{"reason":"appealed","notify":true}`)
	if w.Code != 200 || hs.store.users[7].Ban != nil || !strings.Contains(hs.tg.sent[7], "access to Ellie is back") {
		t.Fatalf("unban: %d %s, ban %+v", w.Code, w.Body, hs.store.users[7].Ban)
	}
	if w := hs.act(t, c, "/api/users/7/ban", `{"days":0,"reason":"for good"}`); w.Code != 200 || hs.store.users[7].Ban.Until != nil {
		t.Fatalf("permanent ban: %d", w.Code)
	}
}

func TestResetToday(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	w := hs.act(t, c, "/api/users/7/reset-today", `{"reason":"failed readings"}`)
	if w.Code != 200 || hs.store.users[7].UsedChars != 0 || !strings.Contains(string(hs.lastAudit().Detail), `"chars":1500`) {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
}

func TestGroupActions(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	w := hs.act(t, c, "/api/groups/-100/block", `{"reason":"spam group"}`)
	if w.Code != 200 || !hs.store.groups[-100].Blocked || len(hs.tg.left) != 1 || hs.store.groups[-100].Status != "left" {
		t.Fatalf("block: %d %s %+v", w.Code, w.Body, hs.store.groups[-100])
	}
	if w := hs.act(t, c, "/api/groups/-100/unblock", `{"reason":"they apologised"}`); w.Code != 200 || hs.store.groups[-100].Blocked {
		t.Fatalf("unblock: %d", w.Code)
	}
	w = hs.act(t, c, "/api/groups/-100/settings", `{"reason":"too busy","settings":{"paused":true,"daily_chars":2000}}`)
	if g := hs.store.groups[-100]; w.Code != 200 || !g.Settings.Paused || g.Settings.DailyChars != 2000 {
		t.Fatalf("settings: %d %s", w.Code, w.Body)
	}
	if w := hs.act(t, c, "/api/groups/-100/settings", `{"reason":"bad","settings":{"daily_chars":-5}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("negative limit: %d", w.Code)
	}
	if w := hs.act(t, c, "/api/groups/-5/block", `{"reason":"unknown"}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown group: %d", w.Code)
	}
}

func TestVoiceDeleteIsQueuedOnce(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	w := hs.act(t, c, "/api/voices/voice/v1/delete", `{"reason":"impersonation"}`)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"already_requested":false`) {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if e := hs.lastAudit(); e.Action != "delete_requested" || e.Target != "voice:v1" {
		t.Fatalf("audit = %+v", e)
	}
	w = hs.act(t, c, "/api/voices/voice/v1/delete", `{"reason":"again"}`)
	if !strings.Contains(w.Body.String(), `"already_requested":true`) || len(hs.store.jobs) != 1 {
		t.Fatalf("second request: %s, jobs %d", w.Body, len(hs.store.jobs))
	}
	for path, want := range map[string]int{
		"/api/voices/model/v1/delete":   http.StatusBadRequest,
		"/api/voices/voice/nope/delete": http.StatusNotFound,
	} {
		if w := hs.act(t, c, path, `{"reason":"checking"}`); w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
	r := httptest.NewRequest("GET", "/api/voices", nil)
	r.AddCookie(c)
	if w := hs.do(r); w.Code != 200 || !strings.Contains(w.Body.String(), `"Nova"`) {
		t.Fatalf("voices list: %d %s", w.Code, w.Body)
	}
}
