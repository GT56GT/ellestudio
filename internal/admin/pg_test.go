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
	settings, err := db.SettingsStore(context.Background(), bot.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	return NewPGStore(db, settings), db
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

func TestPGActionsEndToEnd(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	const uid, gid, voice = -7201, -7202, "test-voice-7201"
	today := time.Now().UTC().Format("2006-01-02")
	cleanup := func() {
		p := db.Pool()
		_, _ = p.Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id = $1`, uid)
		_, _ = p.Exec(ctx, `DELETE FROM ellie_chats WHERE chat_id = $1`, gid)
		_, _ = p.Exec(ctx, `DELETE FROM ellie_ownership WHERE resource_id = $1`, voice)
		_, _ = p.Exec(ctx, `DELETE FROM ellie_admin_jobs WHERE target = $1`, voice)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, _, err := s.GiveBonus(ctx, uid, 10); err != ErrNotFound {
		t.Fatalf("bonus for a stranger: %v", err)
	}
	if _, _, err := s.settings.Spend(uid, today, 300, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if before, after, err := s.GiveBonus(ctx, uid, 250); err != nil || before != 0 || after != 250 {
		t.Fatalf("bonus: %d %d %v", before, after, err)
	}
	if was, err := s.ResetToday(ctx, uid, today); err != nil || was.Chars != 300 {
		t.Fatalf("reset: %+v %v", was, err)
	}
	until := time.Now().Add(time.Hour)
	if _, err := s.SetBan(ctx, uid, &bot.Ban{Until: &until, Reason: "test", By: 1, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	rows, _, err := s.Users(ctx, UserQuery{Banned: true, Limit: 500}, today)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		found = found || (r.ID == uid && r.Banned)
	}
	if !found {
		t.Fatal("banned user missing from the banned filter")
	}
	if bans, err := s.settings.Banned(); err != nil || bans[uid].Reason != "test" {
		t.Fatalf("the bot doesn't see the ban: %v %v", bans[uid], err)
	}

	if err := db.Chats().Upsert(ctx, bot.ChatInfo{ID: gid, Title: "Test group", Status: "member"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGroupBlocked(ctx, gid, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGroupSettings(ctx, gid, bot.GroupSettings{Paused: true}); err != nil {
		t.Fatal(err)
	}
	if g, err := s.Group(ctx, gid); err != nil || !g.Blocked || !g.Settings.Paused {
		t.Fatalf("group: %+v %v", g, err)
	}

	// The panel queues a delete; the bot claims, does and finishes it.
	if err := db.Owners().Claim(ctx, "voice", voice, uid); err != nil {
		t.Fatal(err)
	}
	q := db.AdminQueue()
	if err := q.SetNames(ctx, "voice", map[string]string{voice: "Test Voice"}); err != nil {
		t.Fatal(err)
	}
	job, row, created, err := s.QueueDelete(ctx, "voice", voice, 1)
	if err != nil || !created || row.Name != "Test Voice" || row.OwnerID != uid {
		t.Fatalf("queue: %d %+v %v %v", job, row, created, err)
	}
	if again, _, created, _ := s.QueueDelete(ctx, "voice", voice, 1); created || again != job {
		t.Fatal("a second request opened a second job")
	}
	claimed, ok, err := q.Claim(ctx)
	if err != nil || !ok || claimed.ID != job || claimed.Kind != bot.JobDeleteVoice {
		t.Fatalf("claim: %+v %v %v", claimed, ok, err)
	}
	if err := db.Owners().Release(ctx, "voice", voice); err != nil {
		t.Fatal(err)
	}
	if err := q.Finish(ctx, job, nil); err != nil {
		t.Fatal(err)
	}
	if vs, _, err := s.Voices(ctx, VoiceQuery{OwnerID: uid, Limit: 10}); err != nil || len(vs) != 0 {
		t.Fatalf("deleted voice still listed: %v %v", vs, err)
	}
	jobs, err := s.Jobs(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range jobs {
		if j.ID == job && j.Status != "done" {
			t.Fatalf("job status %q", j.Status)
		}
	}
}
