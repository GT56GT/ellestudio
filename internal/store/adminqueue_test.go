package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdminQueueClaimFinishAndNames(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	p := db.Pool()
	const target, owner = "test-voice-store-1", int64(-8001)
	cleanup := func() {
		_, _ = p.Exec(ctx, `DELETE FROM ellie_admin_jobs WHERE target = $1`, target)
		_, _ = p.Exec(ctx, `DELETE FROM ellie_ownership WHERE resource_id = $1`, target)
	}
	cleanup()
	t.Cleanup(cleanup)

	if err := db.Owners().Claim(ctx, KindVoice, target, owner); err != nil {
		t.Fatal(err)
	}
	q := db.AdminQueue()
	if err := q.SetNames(ctx, KindVoice, map[string]string{target: "Named"}); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := p.QueryRow(ctx, `SELECT name FROM ellie_ownership WHERE resource_id = $1`, target).Scan(&name); err != nil || name != "Named" {
		t.Fatalf("name = %q %v", name, err)
	}

	var id int64
	if err := p.QueryRow(ctx, `INSERT INTO ellie_admin_jobs (kind, target, requested_by) VALUES ($1, $2, 1) RETURNING id`, JobDeleteVoice, target).Scan(&id); err != nil {
		t.Fatal(err)
	}
	j, ok, err := q.Claim(ctx)
	if err != nil || !ok || j.ID != id || j.Kind != JobDeleteVoice || j.Target != target {
		t.Fatalf("claim: %+v %v %v", j, ok, err)
	}
	if _, ok, _ := q.Claim(ctx); ok {
		t.Fatal("a running job was claimed twice")
	}
	if err := q.Finish(ctx, id, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	var status, msg string
	if err := p.QueryRow(ctx, `SELECT status, error FROM ellie_admin_jobs WHERE id = $1`, id).Scan(&status, &msg); err != nil || status != "failed" || msg != "boom" {
		t.Fatalf("job = %q %q %v", status, msg, err)
	}
}

func TestStatusRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const inst = "store-test-instance"
	cleanup := func() { _, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_status WHERE instance = $1`, inst) }
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC().Truncate(time.Second)
	if err := db.ReportStatus(ctx, Status{Instance: inst, Version: "v1", StartedAt: now, At: now.Add(time.Hour), Queued: 3}); err != nil {
		t.Fatal(err)
	}
	s, ok, err := LatestStatus(ctx, db.Pool())
	if err != nil || !ok || s.Instance != inst || s.Queued != 3 {
		t.Fatalf("latest = %+v %v %v", s, ok, err)
	}
}
