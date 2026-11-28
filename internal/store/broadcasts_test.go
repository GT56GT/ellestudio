package store

import (
	"context"
	"testing"
	"time"
)

func TestBroadcastClaimAndRecipients(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	p := db.Pool()
	users := []int64{9_100_000_001, 9_100_000_002, 9_100_000_003}
	cleanup := func() {
		_, _ = p.Exec(ctx, `DELETE FROM ellie_broadcasts`)
		_, _ = p.Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id = ANY($1)`, users)
	}
	cleanup()
	t.Cleanup(cleanup)

	settings, err := db.SettingsStore(ctx, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	// Three people use the private chat; the last opted out of announcements.
	day := time.Now().UTC().Format("2006-01-02")
	for i, uid := range users {
		if err := settings.Join(uid, day); err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			if _, err := settings.Update(uid, func(v *Settings) { v.NoNews = true }); err != nil {
				t.Fatal(err)
			}
		}
	}
	if n, err := CountRecipients(ctx, p, AudienceEveryone); err != nil || n < 2 {
		t.Fatalf("count = %d %v", n, err)
	}

	var id int64
	if err := p.QueryRow(ctx, `INSERT INTO ellie_broadcasts (text, audience, created_by) VALUES ('hi', 'everyone', 1) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	q := db.Broadcasts()
	bc, ok, err := q.ClaimBroadcast(ctx)
	if err != nil || !ok || bc.ID != id || bc.Text != "hi" {
		t.Fatalf("claim: %+v %v %v", bc, ok, err)
	}
	if _, ok, _ := q.ClaimBroadcast(ctx); ok {
		t.Fatal("a sending broadcast was claimed twice")
	}
	ids, err := q.Recipients(ctx, bc, 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]bool{}
	for _, u := range ids {
		got[u] = true
	}
	if !got[users[0]] || !got[users[1]] || got[users[2]] {
		t.Fatalf("recipients = %v: the opted-out user must be skipped", ids)
	}
	if cancelled, err := q.Progress(ctx, bc.ID, ids[len(ids)-1], len(ids), 0); err != nil || cancelled {
		t.Fatalf("progress: %v %v", cancelled, err)
	}
	bc.Cursor = ids[len(ids)-1]
	if rest, _ := q.Recipients(ctx, bc, 100); len(rest) != 0 {
		t.Fatalf("recipients after the cursor: %v", rest)
	}
	if err := q.FinishBroadcast(ctx, bc.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := q.ClaimBroadcast(ctx); ok {
		t.Fatal("a finished broadcast was claimed")
	}
}
