package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	users  map[int64]*bot.Settings
	groups map[int64]bot.ChatInfo
	voices map[string]VoiceRow // by kind:id
	jobs   []JobRow

	overrides  bot.LimitOverrides
	broadcasts []BroadcastRow
}

func (m *memStore) Overrides(context.Context) (bot.LimitOverrides, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.overrides, nil
}

func (m *memStore) SaveOverrides(_ context.Context, o bot.LimitOverrides, _ int64) (bot.LimitOverrides, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	before := m.overrides
	m.overrides = o
	return before, nil
}

func (m *memStore) Broadcasts(context.Context, int) ([]BroadcastRow, error) { return m.broadcasts, nil }

func (m *memStore) AudienceSize(_ context.Context, audience string) (int, error) {
	if audience == bot.AudienceEveryone {
		return 120, nil
	}
	return 45, nil
}

func (m *memStore) CreateBroadcast(_ context.Context, text, audience string, by int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.broadcasts {
		if b.Status == "queued" || b.Status == "sending" {
			return 0, ErrBusy
		}
	}
	id := int64(len(m.broadcasts) + 1)
	m.broadcasts = append(m.broadcasts, BroadcastRow{ID: id, Text: text, Audience: audience, CreatedBy: by, Status: "queued"})
	return id, nil
}

func (m *memStore) CancelBroadcast(_ context.Context, id int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range m.broadcasts {
		if b.ID == id && (b.Status == "queued" || b.Status == "sending") {
			m.broadcasts[i].Status = "cancelled"
			return true, nil
		}
	}
	return false, nil
}

func newMemStore() *memStore {
	return &memStore{
		sessions: map[string]Session{},
		users:    map[int64]*bot.Settings{7: {Bonus: 100, UsageDay: testNow.Format("2006-01-02"), UsedChars: 1500}},
		groups:   map[int64]bot.ChatInfo{-100: {ID: -100, Title: "Book Club", Status: "member"}},
		voices:   map[string]VoiceRow{"voice:v1": {Kind: "voice", ID: "v1", Name: "Nova", OwnerID: 7}},
	}
}

func (m *memStore) GiveBonus(_ context.Context, uid int64, amount int) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[uid]
	if !ok {
		return 0, 0, ErrNotFound
	}
	before := u.Bonus
	u.Bonus += amount
	return before, u.Bonus, nil
}

func (m *memStore) ResetToday(_ context.Context, uid int64, today string) (Allowance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[uid]
	if !ok {
		return Allowance{}, ErrNotFound
	}
	was := Allowance{Chars: u.UsedChars}
	u.UsedChars = 0
	return was, nil
}

func (m *memStore) SetBan(_ context.Context, uid int64, b *bot.Ban) (*bot.Ban, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[uid]
	if !ok {
		return nil, ErrNotFound
	}
	was := u.Ban
	u.Ban = b
	return was, nil
}

func (m *memStore) Group(_ context.Context, id int64) (bot.ChatInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	if !ok {
		return bot.ChatInfo{}, ErrNotFound
	}
	return g, nil
}

func (m *memStore) SetGroupBlocked(_ context.Context, id int64, blocked bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.groups[id]
	g.Blocked = blocked
	m.groups[id] = g
	return nil
}

func (m *memStore) MarkGroupLeft(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.groups[id]
	g.Status = "left"
	m.groups[id] = g
	return nil
}

func (m *memStore) SetGroupSettings(_ context.Context, id int64, gs bot.GroupSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.groups[id]
	g.Settings = gs
	m.groups[id] = g
	return nil
}

func (m *memStore) Voices(context.Context, VoiceQuery) ([]VoiceRow, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []VoiceRow
	for _, v := range m.voices {
		out = append(out, v)
	}
	return out, len(out), nil
}

func (m *memStore) QueueDelete(_ context.Context, kind, id string, by int64) (int64, VoiceRow, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.voices[kind+":"+id]
	if !ok {
		return 0, VoiceRow{}, false, ErrNotFound
	}
	for _, j := range m.jobs {
		if j.Target == id && (j.Status == "pending" || j.Status == "running") {
			return j.ID, v, false, nil
		}
	}
	j := JobRow{ID: int64(len(m.jobs) + 1), Kind: "delete_" + kind, Target: id, RequestedBy: by, Status: "pending"}
	m.jobs = append(m.jobs, j)
	return j.ID, v, true, nil
}

func (m *memStore) Jobs(context.Context, int) ([]JobRow, error) { return m.jobs, nil }

// fakeMessenger records what the panel sent to Telegram.
type fakeMessenger struct {
	mu     sync.Mutex
	sent   map[int64]string
	left   []int64
	failTo int64
}

func (f *fakeMessenger) Notify(_ context.Context, chat int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if chat == f.failTo {
		return errors.New("Forbidden: bot was blocked by the user")
	}
	if f.sent == nil {
		f.sent = map[int64]string{}
	}
	f.sent[chat] = text
	return nil
}

func (f *fakeMessenger) Leave(_ context.Context, chat int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.left = append(f.left, chat)
	return nil
}

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
	tg    *fakeMessenger
	h     http.Handler
	now   time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store := newMemStore()
	tg := &fakeMessenger{}
	srv := New(Config{
		Messenger: tg,
		BotToken:  testToken, BotUsername: "EllieStudioBot", Admins: map[int64]bool{42: true},
		PublicURL: "https://panel.example", Secure: true,
		UI: fstest.MapFS{"index.html": {Data: []byte("<!doctype html>panel")}, "assets/app.js": {Data: []byte("js")}},
	}, store, nil)
	hs := &harness{srv: srv, store: store, tg: tg, now: testNow}
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

func TestClientIPIgnoresSpoofableHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:1234"
	r.Header.Set("CF-Connecting-IP", "1.2.3.4")
	if got := clientIP(r); got != "10.0.0.5" {
		t.Errorf("without a proxy header: %q", got)
	}
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 203.0.113.9")
	if got := clientIP(r); got != "203.0.113.9" {
		t.Errorf("should use the hop the proxy added: %q", got)
	}
}

func TestLimiterMemoryIsBounded(t *testing.T) {
	l := newLimiter(10, time.Minute)
	l.allow("admin", testNow.Add(-time.Second)) // the real admin, earlier
	for i := range limiterKeys + 50 {
		l.allow(strconv.Itoa(i), testNow)
	}
	if len(l.seen) > limiterKeys {
		t.Fatalf("limiter holds %d keys", len(l.seen))
	}
	// A flood of new addresses must not lock anyone else out.
	if !l.allow("new-address", testNow) {
		t.Fatal("a full limiter refused a new address")
	}
}

func TestRateKeyGroupsIPv6(t *testing.T) {
	if rateKey("2001:db8:1:2:aaaa::1") != rateKey("2001:db8:1:2:bbbb::9") {
		t.Error("addresses in one /64 should share a key")
	}
	if rateKey("2001:db8:1:3::1") == rateKey("2001:db8:1:2::1") {
		t.Error("different /64s should differ")
	}
	if rateKey("203.0.113.9") != "203.0.113.9" {
		t.Error("IPv4 addresses are their own key")
	}
}
