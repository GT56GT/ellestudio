package admin

// actions.go is what admins can change: users' bonus, allowance and bans;
// groups' block and settings; and voice or dictionary deletions, which the
// bot carries out (it alone holds the Cartesia key).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ellestudio/internal/bot"
)

// ErrNotFound means the user, group or resource doesn't exist.
var ErrNotFound = errors.New("admin: not found")

// Allowance is one user's usage for a day.
type Allowance struct {
	Chars int `json:"chars"`
	Secs  int `json:"secs"`
	Makes int `json:"makes"`
	Dicts int `json:"dicts"`
}

// VoiceRow is a voice or dictionary made through Ellie.
type VoiceRow struct {
	Kind      string    `json:"kind"` // voice or dict
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   int64     `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	// The latest deletion request, if any: pending, running or failed (a
	// done deletion removes the row).
	JobStatus string `json:"job_status,omitempty"`
	JobError  string `json:"job_error,omitempty"`
}

// VoiceQuery filters the voices list.
type VoiceQuery struct {
	OwnerID int64
	Kind    string // "" = both
	Offset  int
	Limit   int
}

// JobRow is a deletion the panel asked the bot to do.
type JobRow struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	Target      string    `json:"target"`
	RequestedBy int64     `json:"requested_by"`
	Status      string    `json:"status"`
	Error       string    `json:"error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Actions is the write side of the Store.
type Actions interface {
	// GiveBonus adds amount bonus characters, returning the balance before
	// and after.
	GiveBonus(ctx context.Context, uid int64, amount int) (before, after int, err error)
	// ResetToday zeroes today's usage, returning what it was.
	ResetToday(ctx context.Context, uid int64, today string) (Allowance, error)
	// SetBan bans uid (ban != nil) or lifts the ban (nil), returning the
	// previous one.
	SetBan(ctx context.Context, uid int64, ban *bot.Ban) (*bot.Ban, error)

	Group(ctx context.Context, id int64) (bot.ChatInfo, error)
	SetGroupBlocked(ctx context.Context, id int64, blocked bool) error
	MarkGroupLeft(ctx context.Context, id int64) error
	SetGroupSettings(ctx context.Context, id int64, gs bot.GroupSettings) error

	Voices(ctx context.Context, q VoiceQuery) ([]VoiceRow, int, error)
	// QueueDelete asks the bot to delete a resource. created is false when
	// a request for it was already open (its ID is returned).
	QueueDelete(ctx context.Context, kind, id string, by int64) (job int64, row VoiceRow, created bool, err error)
	Jobs(ctx context.Context, limit int) ([]JobRow, error)
}

// ---------------------------------------------------------------- users

// existing applies fn to an existing user's settings, under the same
// per-user lock the bot uses, so nothing the bot writes at the same moment
// is lost.
func (p *PGStore) existing(uid int64, fn func(*bot.Settings)) error {
	if !p.settings.Known(uid) {
		return ErrNotFound
	}
	_, err := p.settings.Update(uid, fn)
	return err
}

func (p *PGStore) GiveBonus(_ context.Context, uid int64, amount int) (int, int, error) {
	var before, after int
	err := p.existing(uid, func(v *bot.Settings) {
		before = v.Bonus
		v.Bonus += amount
		after = v.Bonus
	})
	return before, after, err
}

func (p *PGStore) ResetToday(_ context.Context, uid int64, today string) (Allowance, error) {
	var was Allowance
	err := p.existing(uid, func(v *bot.Settings) {
		if v.UsageDay != today {
			was = Allowance{}
			return
		}
		was = Allowance{Chars: v.UsedChars, Secs: v.UsedSecs, Makes: v.UsedMakes, Dicts: v.UsedDicts}
		v.UsedChars, v.UsedSecs, v.UsedMakes, v.UsedDicts = 0, 0, 0, 0
	})
	return was, err
}

func (p *PGStore) SetBan(_ context.Context, uid int64, ban *bot.Ban) (*bot.Ban, error) {
	var was *bot.Ban
	err := p.existing(uid, func(v *bot.Settings) {
		was, v.Ban = v.Ban, ban
	})
	return was, err
}

// ---------------------------------------------------------------- groups

func (p *PGStore) Group(ctx context.Context, id int64) (bot.ChatInfo, error) {
	if id == 0 {
		return bot.ChatInfo{}, ErrNotFound // row 0 is the all-groups total
	}
	c, ok, err := bot.NewChats(p.db).Get(ctx, id)
	if err != nil {
		return bot.ChatInfo{}, err
	}
	if !ok {
		return bot.ChatInfo{}, ErrNotFound
	}
	return c, nil
}

func (p *PGStore) SetGroupBlocked(ctx context.Context, id int64, blocked bool) error {
	return bot.NewChats(p.db).SetBlocked(ctx, id, blocked)
}

func (p *PGStore) MarkGroupLeft(ctx context.Context, id int64) error {
	return bot.NewChats(p.db).Upsert(ctx, bot.ChatInfo{ID: id, Status: "left"})
}

func (p *PGStore) SetGroupSettings(ctx context.Context, id int64, gs bot.GroupSettings) error {
	return bot.NewChats(p.db).SetSettings(ctx, id, gs)
}

// ---------------------------------------------------------------- voices

const voiceCols = `o.kind, o.resource_id, o.name, o.owner_id, o.created_at,
	COALESCE(j.status, ''), COALESCE(j.error, '')`

const voiceJoin = `FROM ellie_ownership o
	LEFT JOIN LATERAL (
		SELECT status, error FROM ellie_admin_jobs j
		WHERE j.kind = 'delete_' || o.kind AND j.target = o.resource_id
		ORDER BY j.id DESC LIMIT 1
	) j ON true`

func scanVoice(r pgx.Row, extra ...any) (VoiceRow, error) {
	var v VoiceRow
	dest := append([]any{&v.Kind, &v.ID, &v.Name, &v.OwnerID, &v.CreatedAt, &v.JobStatus, &v.JobError}, extra...)
	err := r.Scan(dest...)
	if v.JobStatus == "done" {
		v.JobStatus = "" // the row is about to go; nothing to show
	}
	return v, err
}

func (p *PGStore) Voices(ctx context.Context, q VoiceQuery) ([]VoiceRow, int, error) {
	rows, err := p.db.Pool().Query(ctx, `SELECT `+voiceCols+`, count(*) OVER () `+voiceJoin+`
		WHERE ($1 = 0 OR o.owner_id = $1) AND ($2 = '' OR o.kind = $2)
		ORDER BY o.created_at DESC, o.resource_id LIMIT $3 OFFSET $4`, q.OwnerID, q.Kind, q.Limit, q.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: voices: %w", err)
	}
	defer rows.Close()
	var out []VoiceRow
	var total int
	for rows.Next() {
		v, err := scanVoice(rows, &total)
		if err != nil {
			return nil, 0, fmt.Errorf("admin: voices: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("admin: voices: %w", err)
	}
	return out, total, nil
}

func (p *PGStore) QueueDelete(ctx context.Context, kind, id string, by int64) (int64, VoiceRow, bool, error) {
	pool := p.db.Pool()
	row, err := scanVoice(pool.QueryRow(ctx, `SELECT `+voiceCols+` `+voiceJoin+`
		WHERE o.kind = $1 AND o.resource_id = $2`, kind, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, VoiceRow{}, false, ErrNotFound
	}
	if err != nil {
		return 0, VoiceRow{}, false, fmt.Errorf("admin: find resource: %w", err)
	}
	jobKind := "delete_" + kind
	var job int64
	err = pool.QueryRow(ctx, `INSERT INTO ellie_admin_jobs (kind, target, requested_by) VALUES ($1, $2, $3)
		ON CONFLICT (kind, target) WHERE status IN ('pending', 'running') DO NOTHING
		RETURNING id`, jobKind, id, by).Scan(&job)
	if err == nil {
		row.JobStatus, row.JobError = "pending", ""
		return job, row, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, VoiceRow{}, false, fmt.Errorf("admin: queue delete: %w", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM ellie_admin_jobs
		WHERE kind = $1 AND target = $2 AND status IN ('pending', 'running')`, jobKind, id).Scan(&job); err != nil {
		return 0, VoiceRow{}, false, fmt.Errorf("admin: queue delete: %w", err)
	}
	return job, row, false, nil
}

func (p *PGStore) Jobs(ctx context.Context, limit int) ([]JobRow, error) {
	rows, err := p.db.Pool().Query(ctx, `SELECT id, kind, target, requested_by, status, error, created_at, updated_at
		FROM ellie_admin_jobs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("admin: jobs: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[JobRow])
	if err != nil {
		return nil, fmt.Errorf("admin: jobs: %w", err)
	}
	return out, nil
}
