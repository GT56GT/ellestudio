package store

// status.go holds the bot's heartbeat: the bot writes its live state to the
// database every few seconds, and the admin panel reads the latest one.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
