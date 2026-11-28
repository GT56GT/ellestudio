package bot

// status.go publishes a heartbeat: every few seconds the bot writes its live
// state (the waiting line, the breaker, its limits) to the database, so the
// admin panel, a separate app, can show whether Ellie is up and how busy it
// is.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// heartbeatEvery is how often the bot reports; the panel calls the bot
// offline after a few missed beats.
const heartbeatEvery = 15 * time.Second

// Status is one instance's live state.
type Status struct {
	Instance    string       `json:"instance"`
	Version     string       `json:"version"`
	StartedAt   time.Time    `json:"started_at"`
	At          time.Time    `json:"at"`
	Running     int          `json:"running"` // Cartesia jobs in progress
	Slots       int          `json:"slots"`   // CARTESIA_CONCURRENCY
	Queued      int          `json:"queued"`  // jobs waiting in line
	QueueLimit  int          `json:"queue_limit"`
	PausedUntil *time.Time   `json:"paused_until,omitempty"` // breaker open
	AdminPaused bool         `json:"admin_paused,omitempty"` // the panel's emergency switch
	Limits      StatusLimits `json:"limits"`                 // in force
	Defaults    StatusLimits `json:"defaults"`               // from the env vars
}

// StatusLimits are the daily limits the bot runs with.
type StatusLimits struct {
	UserChars         int `json:"user_chars"`
	UserAudioSecs     int `json:"user_audio_secs"`
	UserCreations     int `json:"user_creations"`
	UserDicts         int `json:"user_dicts"`
	MaxTTSChars       int `json:"max_tts_chars"`
	StudioChars       int `json:"studio_chars"`
	StudioAudioSecs   int `json:"studio_audio_secs"`
	GroupChars        int `json:"group_chars"`
	GroupSecs         int `json:"group_secs"`
	GroupsTotalChars  int `json:"groups_total_chars"`
	GroupsTotalSecs   int `json:"groups_total_secs"`
	GroupCooldownSecs int `json:"group_cooldown_secs"`
	ReferralBonus     int `json:"referral_bonus"`
	ReferralDailyCap  int `json:"referral_daily_cap"`
}

func statusLimits(l Limits) StatusLimits {
	return StatusLimits{
		UserChars: l.DailyChars, UserAudioSecs: l.DailyAudioSecs, UserCreations: l.DailyCreations, UserDicts: l.DailyDicts,
		MaxTTSChars: l.MaxTTSChars, StudioChars: l.StudioDailyChars, StudioAudioSecs: l.StudioDailyAudioSecs,
		GroupChars: l.GroupLimits.Chars, GroupSecs: l.GroupLimits.Secs,
		GroupsTotalChars: l.GroupLimits.AllChars, GroupsTotalSecs: l.GroupLimits.AllSecs,
		GroupCooldownSecs: int(l.GroupCooldown / time.Second),
		ReferralBonus:     l.ReferralBonus, ReferralDailyCap: l.ReferralDailyCap,
	}
}

// StatusReporter stores heartbeats (the database's, in production).
type StatusReporter interface {
	ReportStatus(ctx context.Context, s Status) error
}

// SetStatusReporter turns the heartbeat on. instance and version name this
// process (Render's instance ID and git commit).
func (b *Bot) SetStatusReporter(r StatusReporter, instance, version string) {
	b.reporter, b.instance, b.version = r, instance, version
}

// status snapshots the live state.
func (b *Bot) status(now time.Time) Status {
	running, queued := b.jobs.load()
	s := Status{
		Instance: b.instance, Version: b.version, StartedAt: b.started, At: now,
		Running: running, Slots: b.cfg.Concurrency, Queued: queued, QueueLimit: b.jobs.limit,
		Limits: statusLimits(b.lim()), Defaults: statusLimits(b.base), AdminPaused: b.lim().Paused,
	}
	if open, left := b.breaker.isOpen(); open {
		until := now.Add(left)
		s.PausedUntil = &until
	}
	return s
}

func (b *Bot) heartbeat(ctx context.Context) {
	if b.reporter == nil {
		return
	}
	t := time.NewTicker(heartbeatEvery)
	defer t.Stop()
	for {
		rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := b.reporter.ReportStatus(rctx, b.status(time.Now())); err != nil && ctx.Err() == nil {
			b.log.Warn("heartbeat", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ---------------------------------------------------------------- Postgres

// ReportStatus upserts this instance's heartbeat and forgets instances
// silent for a day (old deploys).
func (d *DB) ReportStatus(ctx context.Context, s Status) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO ellie_status (instance, started_at, at, info) VALUES ($1, $2, $3, $4)
		ON CONFLICT (instance) DO UPDATE SET at = EXCLUDED.at, info = EXCLUDED.info`, s.Instance, s.StartedAt, s.At, raw)
	batch.Queue(`DELETE FROM ellie_status WHERE at < now() - interval '1 day'`)
	if err := d.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("database: report status: %w", err)
	}
	return nil
}

// LatestStatus returns the most recent heartbeat, and false if there is none.
func LatestStatus(ctx context.Context, pool *pgxpool.Pool) (Status, bool, error) {
	var raw []byte
	err := pool.QueryRow(ctx, `SELECT info FROM ellie_status ORDER BY at DESC LIMIT 1`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, false, nil
	}
	if err != nil {
		return Status{}, false, fmt.Errorf("database: latest status: %w", err)
	}
	var s Status
	if err := json.Unmarshal(raw, &s); err != nil {
		return Status{}, false, fmt.Errorf("database: latest status: %w", err)
	}
	return s, true, nil
}

// Pool exposes the connection pool to the admin panel, which runs its own
// read queries against the same database.
func (d *DB) Pool() *pgxpool.Pool { return d.pool }
