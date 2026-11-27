package admin

// server.go is the panel's HTTP side: a JSON API under /api/ and the
// single-page UI for everything else.
//
// Security, in order of the request's path through it:
//   - every response carries a strict Content-Security-Policy and the usual
//     hardening headers; API responses are never cached;
//   - sign-in only accepts proofs signed by Telegram for an ID listed in
//     ADMIN_USER_IDS, and is rate-limited per IP;
//   - a session is a random 256-bit token; the database stores only its
//     SHA-256, sessions expire after SessionTTL or IdleTTL of inactivity,
//     and removing someone from ADMIN_USER_IDS locks them out at once;
//   - the token travels in a __Host- cookie (HttpOnly, Secure,
//     SameSite=Strict) or, inside Telegram where cookies can be blocked, in
//     an Authorization header kept in page memory;
//   - cookie-authenticated writes must come from the panel's own origin and
//     carry a custom header, which a cross-site form can't send;
//   - request bodies are capped, and every sign-in, sign-out and
//     revocation is written to the audit log.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config sets up a Server.
type Config struct {
	BotToken    string
	BotUsername string         // for the Login Widget
	Admins      map[int64]bool // ADMIN_USER_IDS
	// PublicURL is the panel's own origin (https://…), used to check where
	// writes come from. Secure is false only for local http:// runs.
	PublicURL string
	Secure    bool
	// SessionTTL and IdleTTL bound a session's life; AuthMaxAge bounds how
	// old a Telegram sign-in proof may be.
	SessionTTL time.Duration
	IdleTTL    time.Duration
	AuthMaxAge time.Duration
	// UI is the built single-page app (nil: the API alone).
	UI fs.FS
}

// Server is the admin panel.
type Server struct {
	cfg   Config
	store Store
	log   *slog.Logger
	now   func() time.Time
	login *limiter
}

// New creates a Server, filling in default lifetimes.
func New(cfg Config, store Store, log *slog.Logger) *Server {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 2 * time.Hour
	}
	if cfg.AuthMaxAge <= 0 {
		cfg.AuthMaxAge = 10 * time.Minute
	}
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, store: store, log: log, now: time.Now, login: newLimiter(10, 10*time.Minute)}
}

// maxBody caps request bodies; the API only takes small JSON.
const maxBody = 16 << 10

// cookieName is the session cookie. The __Host- prefix makes browsers refuse
// it unless it is Secure, host-only and for path /.
func (s *Server) cookieName() string {
	if s.cfg.Secure {
		return "__Host-ellie_admin"
	}
	return "ellie_admin"
}

// Handler routes every request.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /api/auth/config", s.authConfig)
	mux.HandleFunc("GET /api/auth/telegram", s.loginWidget)
	mux.HandleFunc("POST /api/auth/webapp", s.loginWebApp)
	mux.Handle("POST /api/auth/logout", s.authed(s.logout))

	mux.Handle("GET /api/me", s.authed(s.me))
	mux.Handle("GET /api/status", s.authed(s.botStatus))
	mux.Handle("GET /api/overview", s.authed(s.overview))
	mux.Handle("GET /api/users", s.authed(s.users))
	mux.Handle("GET /api/users/{id}", s.authed(s.user))
	mux.Handle("GET /api/groups", s.authed(s.groups))
	mux.Handle("GET /api/audit", s.authed(s.auditLog))
	mux.Handle("GET /api/sessions", s.authed(s.sessions))
	mux.Handle("POST /api/sessions/{id}/revoke", s.authed(s.revokeSession))
	mux.Handle("POST /api/sessions/revoke-others", s.authed(s.revokeOthers))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { writeError(w, http.StatusNotFound, "No such endpoint.") })

	mux.Handle("/", s.ui())
	return s.headers(mux)
}

// ---------------------------------------------------------------- headers

func (s *Server) headers(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' https://telegram.org", // the Login Widget and the Mini App script
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: https://t.me https://*.telegram.org",
		"connect-src 'self'",
		"frame-src https://oauth.telegram.org",
		"frame-ancestors 'self' https://web.telegram.org", // Telegram Web shows Mini Apps in a frame
		"base-uri 'none'",
		"form-action 'self'",
		"object-src 'none'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups") // the widget signs in through a popup
		if s.cfg.Secure {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------- sessions

type ctxKey struct{}

// authCtx is the signed-in admin of a request.
type authCtx struct {
	session Session
	hash    []byte
	cookie  bool // authenticated by cookie (so writes need CSRF checks)
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// token finds the session token: the Authorization header first, then the
// cookie.
func (s *Server) token(r *http.Request) (string, bool) {
	if t, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && t != "" {
		return t, false
	}
	if c, err := r.Cookie(s.cookieName()); err == nil && c.Value != "" {
		return c.Value, true
	}
	return "", false
}

// authed lets a request through only with a live session of a current
// admin, and only from the panel itself for cookie-authenticated writes.
func (s *Server) authed(h func(http.ResponseWriter, *http.Request, authCtx)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, fromCookie := s.token(r)
		if tok == "" {
			writeError(w, http.StatusUnauthorized, "Sign in first.")
			return
		}
		hash := hashToken(tok)
		sess, ok, err := s.store.LookupSession(r.Context(), hash, s.now(), s.cfg.IdleTTL)
		if err != nil {
			s.log.Error("session lookup", "err", err)
			writeError(w, http.StatusServiceUnavailable, "The database is unreachable. Try again in a moment.")
			return
		}
		if !ok || !s.cfg.Admins[sess.AdminID] {
			s.clearCookie(w)
			writeError(w, http.StatusUnauthorized, "Your session ended. Sign in again.")
			return
		}
		if fromCookie && r.Method != http.MethodGet && !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "This request didn't come from the admin panel.")
			return
		}
		if r.Method != http.MethodGet {
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		sess.Current = true
		h(w, r, authCtx{session: sess, hash: hash, cookie: fromCookie})
	})
}

// sameOrigin is the CSRF check for cookie-authenticated writes: the custom
// header can't be set by a cross-site form, and Origin, when sent, must be
// the panel's own.
func (s *Server) sameOrigin(r *http.Request) bool {
	if r.Header.Get("X-Ellie-Admin") != "1" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == strings.TrimRight(s.cfg.PublicURL, "/")
}

// startSession creates a session for u and sets the cookie. It returns the
// token for callers that can't rely on cookies (inside Telegram).
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u TelegramUser, method string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := s.now()
	sess := Session{
		AdminID: u.ID, Name: u.Name(), CreatedAt: now, ExpiresAt: now.Add(s.cfg.SessionTTL),
		UserAgent: truncate(r.UserAgent(), 200), IP: clientIP(r),
	}
	if err := s.store.CreateSession(r.Context(), hashToken(token), sess); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName(), Value: token, Path: "/", MaxAge: int(s.cfg.SessionTTL.Seconds()),
		HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteStrictMode,
	})
	s.audit(r, u.ID, "login", "", map[string]string{"method": method, "name": u.Name(), "user_agent": sess.UserAgent})
	return token, nil
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: s.cookieName(), Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteStrictMode,
	})
}

// audit records an admin action; a failure is logged, never shown.
func (s *Server) audit(r *http.Request, adminID int64, action, target string, detail any) {
	raw, _ := json.Marshal(detail)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if err := s.store.Audit(ctx, AuditEntry{AdminID: adminID, Action: action, Target: target, Detail: raw, IP: clientIP(r)}); err != nil {
		s.log.Error("audit", "action", action, "err", err)
	}
}

// ---------------------------------------------------------------- sign-in

func (s *Server) authConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"bot_username": s.cfg.BotUsername})
}

// admit runs the checks every sign-in shares: rate limit, then the proof,
// then the admin list. It writes the failure response itself.
func (s *Server) admit(w http.ResponseWriter, r *http.Request, verify func() (TelegramUser, error)) (TelegramUser, bool) {
	ip := clientIP(r)
	if !s.login.allow(ip, s.now()) {
		writeError(w, http.StatusTooManyRequests, "Too many sign-in attempts. Wait a few minutes and try again.")
		return TelegramUser{}, false
	}
	u, err := verify()
	if err != nil {
		s.log.Warn("sign-in proof rejected", "ip", ip, "err", err)
		msg := "Telegram's sign-in proof didn't check out. Try signing in again."
		if errors.Is(err, errStale) {
			msg = "That sign-in link has expired. Sign in again."
		}
		writeError(w, http.StatusUnauthorized, msg)
		return TelegramUser{}, false
	}
	if !s.cfg.Admins[u.ID] {
		s.log.Warn("sign-in refused: not an admin", "user", u.ID, "ip", ip)
		s.audit(r, u.ID, "login_refused", "", map[string]string{"name": u.Name()})
		writeError(w, http.StatusForbidden, "This Telegram account isn't a studio admin.")
		return TelegramUser{}, false
	}
	return u, true
}

// loginWidget is where the Telegram Login Widget redirects after sign-in,
// with the signed fields in the query string. It answers with a redirect
// back into the panel.
func (s *Server) loginWidget(w http.ResponseWriter, r *http.Request) {
	rec := &statusRecorder{}
	u, ok := s.admit(rec, r, func() (TelegramUser, error) {
		return VerifyLoginWidget(s.cfg.BotToken, r.URL.Query(), s.now(), s.cfg.AuthMaxAge)
	})
	if !ok {
		reason := map[int]string{http.StatusForbidden: "not-admin", http.StatusTooManyRequests: "slow-down"}[rec.status]
		http.Redirect(w, r, "/#/signin?error="+url.QueryEscape(orDefault(reason, "failed")), http.StatusSeeOther)
		return
	}
	if _, err := s.startSession(w, r, u, "login_widget"); err != nil {
		s.log.Error("start session", "err", err)
		http.Redirect(w, r, "/#/signin?error=unavailable", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/#/", http.StatusSeeOther)
}

// loginWebApp signs in from inside Telegram with the Mini App's initData.
func (s *Server) loginWebApp(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var in struct {
		InitData string `json:"init_data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.InitData == "" {
		writeError(w, http.StatusBadRequest, "Open the panel from the button in Ellie's /admin screen.")
		return
	}
	u, ok := s.admit(w, r, func() (TelegramUser, error) {
		return VerifyWebAppInitData(s.cfg.BotToken, in.InitData, s.now(), s.cfg.AuthMaxAge)
	})
	if !ok {
		return
	}
	token, err := s.startSession(w, r, u, "mini_app")
	if err != nil {
		s.log.Error("start session", "err", err)
		writeError(w, http.StatusServiceUnavailable, "The database is unreachable. Try again in a moment.")
		return
	}
	writeJSON(w, map[string]any{"token": token, "admin": adminJSON(u.ID, u.Name())})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, a authCtx) {
	if err := s.store.DeleteSession(r.Context(), a.hash); err != nil {
		s.log.Error("logout", "err", err)
	}
	s.clearCookie(w)
	s.audit(r, a.session.AdminID, "logout", "", nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- API

func adminJSON(id int64, name string) map[string]any {
	return map[string]any{"id": id, "name": name}
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, a authCtx) {
	writeJSON(w, map[string]any{
		"admin":      adminJSON(a.session.AdminID, a.session.Name),
		"expires_at": a.session.ExpiresAt,
	})
}

func (s *Server) today() string { return s.now().UTC().Format("2006-01-02") }

func (s *Server) botStatus(w http.ResponseWriter, r *http.Request, _ authCtx) {
	st, online, err := s.store.BotStatus(r.Context(), s.now().UTC())
	if err != nil {
		s.fail(w, "bot status", err)
		return
	}
	writeJSON(w, map[string]any{"bot": st, "bot_online": online})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request, _ authCtx) {
	o, err := s.store.Overview(r.Context(), s.now().UTC())
	if err != nil {
		s.fail(w, "overview", err)
		return
	}
	writeJSON(w, o)
}

// page reads ?page= (from 1) into an offset for pages of size n.
func page(r *http.Request, n int) (offset, number int) {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		p = 1
	}
	p = min(p, 10_000)
	return (p - 1) * n, p
}

const pageSize = 50

func (s *Server) users(w http.ResponseWriter, r *http.Request, _ authCtx) {
	q := UserQuery{Sort: r.URL.Query().Get("sort"), Limit: pageSize}
	if raw := strings.TrimSpace(r.URL.Query().Get("q")); raw != "" {
		id, err := strconv.ParseInt(strings.TrimPrefix(raw, "#"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Search by Telegram user ID (numbers only).")
			return
		}
		q.ID = id
	}
	var number int
	q.Offset, number = page(r, pageSize)
	rows, total, err := s.store.Users(r.Context(), q, s.today())
	if err != nil {
		s.fail(w, "users", err)
		return
	}
	writeJSON(w, map[string]any{"users": nonNil(rows), "total": total, "page": number, "page_size": pageSize})
}

func (s *Server) user(w http.ResponseWriter, r *http.Request, _ authCtx) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "That isn't a Telegram user ID.")
		return
	}
	d, ok, err := s.store.User(r.Context(), id, s.today())
	if err != nil {
		s.fail(w, "user", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "No user with that ID has used Ellie.")
		return
	}
	d.Activity = nonNil(d.Activity)
	writeJSON(w, d)
}

func (s *Server) groups(w http.ResponseWriter, r *http.Request, _ authCtx) {
	offset, number := page(r, pageSize)
	rows, total, err := s.store.Groups(r.Context(), offset, pageSize, s.today())
	if err != nil {
		s.fail(w, "groups", err)
		return
	}
	writeJSON(w, map[string]any{"groups": nonNil(rows), "total": total, "page": number, "page_size": pageSize})
}

func (s *Server) auditLog(w http.ResponseWriter, r *http.Request, _ authCtx) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	entries, err := s.store.AuditLog(r.Context(), max(before, 0), pageSize)
	if err != nil {
		s.fail(w, "audit log", err)
		return
	}
	writeJSON(w, map[string]any{"entries": nonNil(entries), "page_size": pageSize})
}

func (s *Server) sessions(w http.ResponseWriter, r *http.Request, a authCtx) {
	list, err := s.store.Sessions(r.Context(), a.session.AdminID, a.hash)
	if err != nil {
		s.fail(w, "sessions", err)
		return
	}
	writeJSON(w, map[string]any{"sessions": nonNil(list)})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request, a authCtx) {
	id := r.PathValue("id")
	if len(id) != 16 {
		writeError(w, http.StatusBadRequest, "That isn't a session.")
		return
	}
	ok, err := s.store.DeleteSessionByID(r.Context(), a.session.AdminID, id)
	if err != nil {
		s.fail(w, "revoke session", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "That session has already ended.")
		return
	}
	s.audit(r, a.session.AdminID, "session_revoked", id, nil)
	if id == sessionID(a.hash) {
		s.clearCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeOthers(w http.ResponseWriter, r *http.Request, a authCtx) {
	n, err := s.store.DeleteOtherSessions(r.Context(), a.session.AdminID, a.hash)
	if err != nil {
		s.fail(w, "revoke sessions", err)
		return
	}
	s.audit(r, a.session.AdminID, "sessions_revoked", "others", map[string]int{"count": n})
	writeJSON(w, map[string]any{"revoked": n})
}

// ---------------------------------------------------------------- UI

// ui serves the built app, falling back to index.html so deep links work.
func (s *Server) ui() http.Handler {
	if s.cfg.UI == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "The admin panel UI isn't built into this binary (build with -tags embedui).", http.StatusServiceUnavailable)
		})
	}
	files := http.FileServerFS(s.cfg.UI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "index.html" {
			s.serveIndex(w, r)
			return
		}
		if _, err := fs.Stat(s.cfg.UI, name); err != nil {
			s.serveIndex(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // names carry a content hash
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.cfg.UI, "index.html")
}

// ---------------------------------------------------------------- helpers

func (s *Server) fail(w http.ResponseWriter, what string, err error) {
	s.log.Error("admin api", "what", what, "err", err)
	writeError(w, http.StatusInternalServerError, "Couldn't load this. Try again in a moment.")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// nonNil makes empty lists encode as [] rather than null.
func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// clientIP is the caller's address, used only for rate limiting and the
// audit log. Behind Render's proxy it is the last X-Forwarded-For entry,
// the one the proxy itself appended; earlier entries (and headers like
// CF-Connecting-IP) can be set by the caller, so they are never trusted.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1]) // the hop our proxy added
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// statusRecorder swallows a response, keeping only its status: the sign-in
// redirect reuses admit's checks but answers with a redirect instead.
type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}
func (r *statusRecorder) WriteHeader(code int)        { r.status = code }
func (r *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }

// limiterKeys caps how many addresses the sign-in limiter remembers.
const limiterKeys = 10_000

// limiter allows n attempts per key in each window.
type limiter struct {
	mu     sync.Mutex
	n      int
	window time.Duration
	seen   map[string][]time.Time
}

func newLimiter(n int, window time.Duration) *limiter {
	return &limiter{n: n, window: window, seen: map[string][]time.Time{}}
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.seen) >= limiterKeys { // forget old keys
		for k, ts := range l.seen {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > l.window {
				delete(l.seen, k)
			}
		}
		if len(l.seen) >= limiterKeys {
			// Still full: someone is cycling addresses. Refuse new ones
			// until old entries age out, so memory stays bounded.
			if _, known := l.seen[key]; !known {
				return false
			}
		}
	}
	recent := l.seen[key][:0]
	for _, t := range l.seen[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.n {
		l.seen[key] = recent
		return false
	}
	l.seen[key] = append(recent, now)
	return true
}
