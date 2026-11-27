package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"ellestudio/internal/bot"
)

// testStore opens TEST_DATABASE_URL or skips. Rows use negative IDs so they
// can't clash with real users.
func testStore(t *testing.T) (*PGStore, *bot.DB) {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		t.Skip("set TEST_DATABASE_URL to test the Postgres store")
	}
	db, err := bot.OpenDatabase(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return NewPGStore(db), db
}

func TestPGSessionsAndAudit(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	const admin = -7001
	cleanup := func() {
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_admin_sessions WHERE admin_id = $1`, admin)
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_admin_audit WHERE admin_id = $1`, admin)
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC().Truncate(time.Second)
	a, b := hashToken("token-a"), hashToken("token-b")
	for _, h := range [][]byte{a, b} {
		if err := s.CreateSession(ctx, h, Session{AdminID: admin, Name: "Angel", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	got, ok, err := s.LookupSession(ctx, a, now.Add(time.Minute), time.Hour)
	if err != nil || !ok || got.AdminID != admin || got.Name != "Angel" {
		t.Fatalf("lookup: %+v %v %v", got, ok, err)
	}
	if _, ok, _ := s.LookupSession(ctx, a, now.Add(2*time.Hour), 3*time.Hour); ok {
		t.Fatal("expired session found")
	}
	if _, ok, _ := s.LookupSession(ctx, a, now.Add(50*time.Minute), 10*time.Minute); ok {
		t.Fatal("idle session found")
	}
	list, err := s.Sessions(ctx, admin, a)
	if err != nil || len(list) != 2 {
		t.Fatalf("sessions: %v %v", list, err)
	}
	if ok, err := s.DeleteSessionByID(ctx, admin, sessionID(b)); !ok || err != nil {
		t.Fatalf("revoke by id: %v %v", ok, err)
	}
	if ok, _ := s.DeleteSessionByID(ctx, admin+1, sessionID(a)); ok {
		t.Fatal("revoked another admin's session")
	}
	if n, err := s.DeleteOtherSessions(ctx, admin, a); n != 0 || err != nil {
		t.Fatalf("revoke others: %d %v", n, err)
	}

	if err := s.Audit(ctx, AuditEntry{AdminID: admin, Action: "login", Detail: []byte(`{"method":"mini_app"}`), IP: "203.0.113.9"}); err != nil {
		t.Fatal(err)
	}
	entries, err := s.AuditLog(ctx, 0, 50)
	if err != nil || len(entries) == 0 || entries[0].Action != "login" {
		t.Fatalf("audit log: %v %v", entries, err)
	}
}

func TestPGReadViews(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	const uid, gid = -7101, -7102
	today := time.Now().UTC().Format("2006-01-02")
	cleanup := func() {
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id = $1`, uid)
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_chats WHERE chat_id = $1`, gid)
	}
	cleanup()
	t.Cleanup(cleanup)

	store, err := db.SettingsStore(ctx, bot.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(uid, func(v *bot.Settings) { v.VoiceName, v.Joined, v.Bonus, v.Readings = "Skylar", true, 300, 4 }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Spend(uid, today, 120, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.Chats().Upsert(ctx, bot.ChatInfo{ID: gid, Type: "supergroup", Title: "Test group", Status: "member"}); err != nil {
		t.Fatal(err)
	}

	rows, total, err := s.Users(ctx, UserQuery{ID: uid, Limit: 50}, today)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("users: %v %d %v", rows, total, err)
	}
	if r := rows[0]; r.Voice != "Skylar" || r.Bonus != 300 || r.TodayChars != 120 || !r.Joined {
		t.Fatalf("user row = %+v", r)
	}
	for _, sort := range []string{"recent", "readings", "bonus", "'; DROP TABLE x; --"} {
		if _, _, err := s.Users(ctx, UserQuery{Sort: sort, Limit: 5}, today); err != nil {
			t.Fatalf("sort %q: %v", sort, err)
		}
	}
	d, ok, err := s.User(ctx, uid, today)
	if err != nil || !ok || d.Readings != 4 {
		t.Fatalf("user detail: %+v %v %v", d, ok, err)
	}
	if _, ok, _ := s.User(ctx, -1, today); ok {
		t.Fatal("found a user that doesn't exist")
	}
	groups, _, err := s.Groups(ctx, 0, 500, today)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups {
		found = found || (g.ID == gid && g.Title == "Test group")
	}
	if !found {
		t.Fatal("test group not listed")
	}
	o, err := s.Overview(ctx, time.Now().UTC())
	if err != nil || len(o.Days) != 30 || o.Users < 1 {
		t.Fatalf("overview: days=%d users=%d err=%v", len(o.Days), o.Users, err)
	}
}
