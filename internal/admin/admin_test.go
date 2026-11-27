package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"ellestudio/internal/bot"
)

const testToken = "123456:TEST-token"

var testNow = time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)

// signWidget builds Login Widget fields signed as Telegram would.
func signWidget(token string, id int64, at time.Time) url.Values {
	v := url.Values{
		"id": {strconv.FormatInt(id, 10)}, "first_name": {"Angel"}, "username": {"angel"},
		"auth_date": {strconv.FormatInt(at.Unix(), 10)},
	}
	key := sha256.Sum256([]byte(token))
	v.Set("hash", hex.EncodeToString(hmacSHA256(key[:], checkString(v))))
	return v
}

// signWebApp builds Mini App initData signed as Telegram would.
func signWebApp(token string, id int64, at time.Time) string {
	v := url.Values{
		"user":      {`{"id":` + strconv.FormatInt(id, 10) + `,"first_name":"Angel"}`},
		"auth_date": {strconv.FormatInt(at.Unix(), 10)},
		"query_id":  {"AAH"},
	}
	v.Set("hash", hex.EncodeToString(hmacSHA256(hmacSHA256([]byte("WebAppData"), token), checkString(v))))
	return v.Encode()
}

func TestVerifyLoginWidget(t *testing.T) {
	v := signWidget(testToken, 42, testNow.Add(-time.Minute))
	u, err := VerifyLoginWidget(testToken, v, testNow, 10*time.Minute)
	if err != nil || u.ID != 42 || u.Name() != "Angel" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
	if _, err := VerifyLoginWidget("other:token", v, testNow, 10*time.Minute); err != errBadSignature {
		t.Fatalf("another bot's token: err=%v", err)
	}
	tampered := signWidget(testToken, 42, testNow)
	tampered.Set("id", "43")
	if _, err := VerifyLoginWidget(testToken, tampered, testNow, 10*time.Minute); err != errBadSignature {
		t.Fatalf("tampered id: err=%v", err)
	}
	old := signWidget(testToken, 42, testNow.Add(-time.Hour))
	if _, err := VerifyLoginWidget(testToken, old, testNow, 10*time.Minute); err != errStale {
		t.Fatalf("old proof: err=%v", err)
	}
	if _, err := VerifyLoginWidget(testToken, url.Values{"id": {"42"}}, testNow, time.Minute); err != errMalformed {
		t.Fatalf("no hash: err=%v", err)
	}
}

func TestVerifyWebAppInitData(t *testing.T) {
	d := signWebApp(testToken, 42, testNow)
	if u, err := VerifyWebAppInitData(testToken, d, testNow, 10*time.Minute); err != nil || u.ID != 42 {
		t.Fatalf("u=%+v err=%v", u, err)
	}
	// A Login Widget signature isn't valid as Mini App data, and vice versa.
	if _, err := VerifyWebAppInitData(testToken, signWidget(testToken, 42, testNow).Encode(), testNow, time.Hour); err == nil {
		t.Fatal("widget proof accepted as Mini App data")
	}
	if _, err := VerifyWebAppInitData(testToken, strings.Replace(d, "42", "43", 1), testNow, time.Hour); err == nil {
		t.Fatal("tampered initData accepted")
	}
}

// ---------------------------------------------------------------- fake store

type memStore struct {
	mu       sync.Mutex
	sessions map[string]Session // by token hash
	audit    []AuditEntry
}

func newMemStore() *memStore { return &memStore{sessions: map[string]Session{}} }

func (m *memStore) CreateSession(_ context.Context, h []byte, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.ID, s.LastSeen = sessionID(h), s.CreatedAt
	m.sessions[string(h)] = s
	return nil
}

func (m *memStore) LookupSession(_ context.Context, h []byte, now time.Time, idle time.Duration) (Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[string(h)]
	if !ok || !now.Before(s.ExpiresAt) || now.Sub(s.LastSeen) > idle {
		return Session{}, false, nil
	}
	s.LastSeen = now
	m.sessions[string(h)] = s
	return s, true, nil
}

func (m *memStore) DeleteSession(_ context.Context, h []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, string(h))
	return nil
}

func (m *memStore) DeleteSessionByID(_ context.Context, admin int64, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for h, s := range m.sessions {
		if s.AdminID == admin && s.ID == id {
			delete(m.sessions, h)
			return true, nil
		}
	}
	return false, nil
}

func (m *memStore) DeleteOtherSessions(_ context.Context, admin int64, keep []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for h, s := range m.sessions {
		if s.AdminID == admin && h != string(keep) {
			delete(m.sessions, h)
			n++
		}
	}
	return n, nil
}

func (m *memStore) Sessions(_ context.Context, admin int64, cur []byte) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Session
	for h, s := range m.sessions {
		if s.AdminID == admin {
			s.Current = h == string(cur)
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memStore) PruneSessions(context.Context, time.Time) error { return nil }

func (m *memStore) Audit(_ context.Context, e AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = int64(len(m.audit) + 1)
	m.audit = append(m.audit, e)
	return nil
}

func (m *memStore) actions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, e := range m.audit {
		out = append(out, e.Action)
	}
	return out
}

func (m *memStore) AuditLog(context.Context, int64, int) ([]AuditEntry, error) { return m.audit, nil }
func (m *memStore) BotStatus(context.Context, time.Time) (*bot.Status, bool, error) {
	return nil, false, nil
}
func (m *memStore) Overview(context.Context, time.Time) (Overview, error) {
	return Overview{Users: 3}, nil
}
func (m *memStore) Users(context.Context, UserQuery, string) ([]UserRow, int, error) {
	return nil, 0, nil
}
func (m *memStore) User(_ context.Context, id int64, _ string) (UserDetail, bool, error) {
	return UserDetail{UserRow: UserRow{ID: id}}, id == 7, nil
}
func (m *memStore) Groups(context.Context, int, int, string) ([]GroupRow, int, error) {
	return nil, 0, nil
}

// ---------------------------------------------------------------- server

type harness struct {
	srv   *Server
	store *memStore
	h     http.Handler
	now   time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store := newMemStore()
	srv := New(Config{
		BotToken: testToken, BotUsername: "EllieStudioBot", Admins: map[int64]bool{42: true},
		PublicURL: "https://panel.example", Secure: true,
		UI: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>panel")}, "assets/app.js": {Data: []byte("js")}},
	}, store, nil)
	hs := &harness{srv: srv, store: store, now: testNow}
	srv.now = func() time.Time { return hs.now }
	hs.h = srv.Handler()
	return hs
}

func (hs *harness) do(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	hs.h.ServeHTTP(w, r)
	return w
}

// signIn goes through the Login Widget redirect and returns the cookie.
func (hs *harness) signIn(t *testing.T, id int64) *http.Cookie {
	t.Helper()
	w := hs.do(httptest.NewRequest("GET", "/api/auth/telegram?"+signWidget(testToken, id, hs.now).Encode(), nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("sign-in status %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "__Host-ellie_admin" && c.Value != "" {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
				t.Fatalf("weak cookie: %+v", c)
			}
			return c
		}
	}
	t.Fatalf("no session cookie (redirect to %s)", w.Header().Get("Location"))
	return nil
}

func TestWidgetSignInSetsSecureCookie(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.AddCookie(c)
	w := hs.do(r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"Angel"`) {
		t.Fatalf("me: %d %s", w.Code, w.Body)
	}
	if got := hs.store.actions(); len(got) != 1 || got[0] != "login" {
		t.Fatalf("audit = %v", got)
	}
}

func TestNonAdminCantSignIn(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(httptest.NewRequest("GET", "/api/auth/telegram?"+signWidget(testToken, 99, testNow).Encode(), nil))
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=not-admin") {
		t.Fatalf("redirect = %q", loc)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("a non-admin got a cookie")
	}
	if got := hs.store.actions(); len(got) != 1 || got[0] != "login_refused" {
		t.Fatalf("audit = %v", got)
	}
}

func TestForgedOrOldProofsRejected(t *testing.T) {
	hs := newHarness(t)
	for name, q := range map[string]url.Values{
		"wrong token": signWidget("999:other", 42, testNow),
		"too old":     signWidget(testToken, 42, testNow.Add(-time.Hour)),
	} {
		w := hs.do(httptest.NewRequest("GET", "/api/auth/telegram?"+q.Encode(), nil))
		if loc := w.Header().Get("Location"); !strings.Contains(loc, "error=failed") || len(w.Result().Cookies()) != 0 {
			t.Errorf("%s: redirect %q, cookies %v", name, loc, w.Result().Cookies())
		}
	}
}

func TestMiniAppSignInReturnsBearerToken(t *testing.T) {
	hs := newHarness(t)
	body := `{"init_data":` + strconv.Quote(signWebApp(testToken, 42, testNow)) + `}`
	w := hs.do(httptest.NewRequest("POST", "/api/auth/webapp", strings.NewReader(body)))
	var out struct{ Token string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Token == "" {
		t.Fatalf("webapp sign-in: %d %s", w.Code, w.Body)
	}
	r := httptest.NewRequest("GET", "/api/overview", nil)
	r.Header.Set("Authorization", "Bearer "+out.Token)
	if w := hs.do(r); w.Code != 200 || !strings.Contains(w.Body.String(), `"users":3`) {
		t.Fatalf("overview with bearer: %d %s", w.Code, w.Body)
	}
	// Bearer writes need no CSRF header: a cross-site page can't attach it.
	r = httptest.NewRequest("POST", "/api/sessions/revoke-others", nil)
	r.Header.Set("Authorization", "Bearer "+out.Token)
	if w := hs.do(r); w.Code != 200 {
		t.Fatalf("bearer write: %d %s", w.Code, w.Body)
	}
}

func TestAPINeedsASession(t *testing.T) {
	hs := newHarness(t)
	for _, p := range []string{"/api/me", "/api/overview", "/api/users", "/api/users/7", "/api/groups", "/api/audit", "/api/sessions"} {
		if w := hs.do(httptest.NewRequest("GET", p, nil)); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: %d", p, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/api/me", nil)
	r.Header.Set("Authorization", "Bearer made-up")
	if w := hs.do(r); w.Code != http.StatusUnauthorized {
		t.Errorf("made-up token: %d", w.Code)
	}
}

func TestCookieWritesNeedSameOrigin(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	post := func(origin, header string) int {
		r := httptest.NewRequest("POST", "/api/sessions/revoke-others", nil)
		r.AddCookie(c)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if header != "" {
			r.Header.Set("X-Ellie-Admin", header)
		}
		return hs.do(r).Code
	}
	if got := post("https://evil.example", "1"); got != http.StatusForbidden {
		t.Errorf("other origin: %d", got)
	}
	if got := post("https://panel.example", ""); got != http.StatusForbidden {
		t.Errorf("no custom header: %d", got)
	}
	if got := post("https://panel.example", "1"); got != 200 {
		t.Errorf("from the panel: %d", got)
	}
}

func TestSessionsEnd(t *testing.T) {
	hs := newHarness(t)
	c := hs.signIn(t, 42)
	get := func() int {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.AddCookie(c)
		return hs.do(r).Code
	}
	hs.now = hs.now.Add(3 * time.Hour) // idle past IdleTTL
	if get() != http.StatusUnauthorized {
		t.Error("idle session still works")
	}

	hs.now = testNow
	c = hs.signIn(t, 42)
	hs.srv.cfg.Admins = map[int64]bool{} // removed from ADMIN_USER_IDS
	if get() != http.StatusUnauthorized {
		t.Error("a removed admin's session still works")
	}

	hs.srv.cfg.Admins = map[int64]bool{42: true}
	c = hs.signIn(t, 42)
	r := httptest.NewRequest("POST", "/api/auth/logout", nil)
	r.AddCookie(c)
	r.Header.Set("X-Ellie-Admin", "1")
	if w := hs.do(r); w.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", w.Code)
	}
	if get() != http.StatusUnauthorized {
		t.Error("session works after sign-out")
	}
}

func TestSignInIsRateLimited(t *testing.T) {
	hs := newHarness(t)
	var w *httptest.ResponseRecorder
	for range 11 {
		w = hs.do(httptest.NewRequest("POST", "/api/auth/webapp", strings.NewReader(`{"init_data":"hash=00"}`)))
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: %d", w.Code)
	}
}

func TestSecurityHeadersAndUI(t *testing.T) {
	hs := newHarness(t)
	w := hs.do(httptest.NewRequest("GET", "/users/7", nil)) // a deep link
	if w.Code != 200 || !strings.Contains(w.Body.String(), "panel") {
		t.Fatalf("deep link: %d %s", w.Code, w.Body)
	}
	h := w.Header()
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'self' https://web.telegram.org", "object-src 'none'"} {
		if !strings.Contains(h.Get("Content-Security-Policy"), want) {
			t.Errorf("CSP lacks %q", want)
		}
	}
	if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Strict-Transport-Security") == "" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("hardening headers missing: %v", h)
	}
	w = hs.do(httptest.NewRequest("GET", "/api/nope", nil))
	if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("unknown API path: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	body, _ := io.ReadAll(hs.do(httptest.NewRequest("GET", "/assets/app.js", nil)).Body)
	if string(body) != "js" {
		t.Errorf("asset = %q", body)
	}
}
