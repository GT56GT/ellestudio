package store

// adminqueue.go is the queue the admin panel puts work on (deleting a voice)
// and the bot works off: only the bot holds the Cartesia key. It also carries
// resource names from the bot to the panel.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Admin job kinds.
const (
	JobDeleteVoice = "delete_voice"
	JobDeleteDict  = "delete_dict"
)

// AdminJob is one piece of work the panel queued.
type AdminJob struct {
	ID          int64
	Kind        string
	Target      string // the Cartesia resource ID
	RequestedBy int64
}

// AdminQueue is where admin jobs and resource names live (the database).
type AdminQueue interface {
	// Claim takes the oldest pending job (or one stuck running), if any.
	Claim(ctx context.Context) (AdminJob, bool, error)
	// Finish records how a job went (err nil = done).
	Finish(ctx context.Context, id int64, err error) error
	// SetNames records the display names of resources of one kind.
	SetNames(ctx context.Context, kind string, names map[string]string) error
}

// AdminQueue returns the database's job queue and name registry.
func (d *DB) AdminQueue() AdminQueue { return pgAdminQueue{d.pool} }

type pgAdminQueue struct{ pool *pgxpool.Pool }

// jobStuckAfter is how long a running job may go without finishing before
// another instance takes it over (its runner died mid-way).
const jobStuckAfter = 5 * time.Minute

func (q pgAdminQueue) Claim(ctx context.Context) (AdminJob, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, PgTimeout)
	defer cancel()
	var j AdminJob
	err := q.pool.QueryRow(ctx, `UPDATE ellie_admin_jobs SET status = 'running', updated_at = now()
		WHERE id = (
			SELECT id FROM ellie_admin_jobs
			WHERE status = 'pending' OR (status = 'running' AND updated_at < now() - make_interval(secs => $1))
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, kind, target, requested_by`, jobStuckAfter.Seconds()).Scan(&j.ID, &j.Kind, &j.Target, &j.RequestedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminJob{}, false, nil
	}
	if err != nil {
		return AdminJob{}, false, fmt.Errorf("database: claim admin job: %w", err)
	}
	return j, true, nil
}

func (q pgAdminQueue) Finish(ctx context.Context, id int64, jobErr error) error {
	status, msg := "done", ""
	if jobErr != nil {
		status, msg = "failed", jobErr.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
	}
	_, err := q.pool.Exec(ctx, `UPDATE ellie_admin_jobs SET status = $2, error = $3, updated_at = now() WHERE id = $1`, id, status, msg)
	if err != nil {
		return fmt.Errorf("database: finish admin job: %w", err)
	}
	return nil
}

func (q pgAdminQueue) SetNames(ctx context.Context, kind string, names map[string]string) error {
	if len(names) == 0 {
		return nil
	}
	ids := make([]string, 0, len(names))
	vals := make([]string, 0, len(names))
	for id, name := range names {
		ids, vals = append(ids, id), append(vals, name)
	}
	_, err := q.pool.Exec(ctx, `UPDATE ellie_ownership o SET name = v.name
		FROM unnest($2::text[], $3::text[]) AS v(id, name)
		WHERE o.kind = $1 AND o.resource_id = v.id AND o.name <> v.name`, kind, ids, vals)
	if err != nil {
		return fmt.Errorf("database: save names: %w", err)
	}
	return nil
}
