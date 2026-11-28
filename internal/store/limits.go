package store

// limits.go holds the daily limits and the admin panel's overrides of them.
// The env vars set the defaults; admins can override any of them (and pause
// voice work) from the panel, stored in ellie_config, and the bot reloads
// them every few seconds.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Limits are the limits in force.
type Limits struct {
	DailyChars, DailyAudioSecs, DailyCreations, DailyDicts int
	MaxTTSChars                                            int
	StudioDailyChars, StudioDailyAudioSecs                 int
	GroupLimits                                            GroupLimits
	GroupCooldown                                          time.Duration
	ReferralBonus, ReferralDailyCap                        int
	// Paused stops new voice work (an admin's emergency switch).
	Paused bool
}

// LimitOverrides are an admin's changes to the defaults; nil leaves a
// default alone. It is what the panel stores and the bot loads.
type LimitOverrides struct {
	DailyChars           *int `json:"daily_chars,omitempty"`
	DailyAudioSecs       *int `json:"daily_audio_secs,omitempty"`
	DailyCreations       *int `json:"daily_creations,omitempty"`
	DailyDicts           *int `json:"daily_dicts,omitempty"`
	MaxTTSChars          *int `json:"max_tts_chars,omitempty"`
	StudioDailyChars     *int `json:"studio_daily_chars,omitempty"`
	StudioDailyAudioSecs *int `json:"studio_daily_audio_secs,omitempty"`
	GroupChars           *int `json:"group_chars,omitempty"`
	GroupSecs            *int `json:"group_secs,omitempty"`
	GroupsTotalChars     *int `json:"groups_total_chars,omitempty"`
	GroupsTotalSecs      *int `json:"groups_total_secs,omitempty"`
	GroupCooldownSecs    *int `json:"group_cooldown_secs,omitempty"`
	ReferralBonus        *int `json:"referral_bonus,omitempty"`
	ReferralDailyCap     *int `json:"referral_daily_cap,omitempty"`
	Paused               bool `json:"paused,omitempty"`
}

// Apply returns base with the overrides on top.
func (o LimitOverrides) Apply(base Limits) Limits {
	set := func(dst *int, v *int) {
		if v != nil {
			*dst = *v
		}
	}
	l := base
	set(&l.DailyChars, o.DailyChars)
	set(&l.DailyAudioSecs, o.DailyAudioSecs)
	set(&l.DailyCreations, o.DailyCreations)
	set(&l.DailyDicts, o.DailyDicts)
	set(&l.MaxTTSChars, o.MaxTTSChars)
	set(&l.StudioDailyChars, o.StudioDailyChars)
	set(&l.StudioDailyAudioSecs, o.StudioDailyAudioSecs)
	set(&l.GroupLimits.Chars, o.GroupChars)
	set(&l.GroupLimits.Secs, o.GroupSecs)
	set(&l.GroupLimits.AllChars, o.GroupsTotalChars)
	set(&l.GroupLimits.AllSecs, o.GroupsTotalSecs)
	set(&l.ReferralBonus, o.ReferralBonus)
	set(&l.ReferralDailyCap, o.ReferralDailyCap)
	if o.GroupCooldownSecs != nil {
		l.GroupCooldown = time.Duration(*o.GroupCooldownSecs) * time.Second
	}
	l.Paused = l.Paused || o.Paused
	return l
}

// Validate checks every override is in a sane range.
func (o LimitOverrides) Validate() error {
	checks := []struct {
		name string
		v    *int
		max  int
	}{
		{"characters per person", o.DailyChars, 1_000_000},
		{"transcription per person", o.DailyAudioSecs, 24 * 3600},
		{"new voices per person", o.DailyCreations, 1000},
		{"dictionaries per person", o.DailyDicts, 1000},
		{"longest text", o.MaxTTSChars, 10_000},
		{"characters for everyone", o.StudioDailyChars, 100_000_000},
		{"transcription for everyone", o.StudioDailyAudioSecs, 100 * 24 * 3600},
		{"characters per group", o.GroupChars, 10_000_000},
		{"transcription per group", o.GroupSecs, 24 * 3600},
		{"characters for all groups", o.GroupsTotalChars, 100_000_000},
		{"transcription for all groups", o.GroupsTotalSecs, 100 * 24 * 3600},
		{"group cooldown", o.GroupCooldownSecs, 3600},
		{"invite bonus", o.ReferralBonus, 1_000_000},
		{"invite rewards a day", o.ReferralDailyCap, 1000},
	}
	for _, c := range checks {
		if c.v != nil && (*c.v < 0 || *c.v > c.max) {
			return fmt.Errorf("%s must be between 0 and %d", c.name, c.max)
		}
	}
	if o.MaxTTSChars != nil && *o.MaxTTSChars == 0 {
		return errors.New("longest text can't be 0")
	}
	return nil
}

// configKey is the ellie_config row holding the overrides.
const configKey = "limits"

// LoadOverrides reads the overrides from pool.
func LoadOverrides(ctx context.Context, pool *pgxpool.Pool) (LimitOverrides, error) {
	var raw []byte
	err := pool.QueryRow(ctx, `SELECT value FROM ellie_config WHERE key = $1`, configKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return LimitOverrides{}, nil
	}
	if err != nil {
		return LimitOverrides{}, fmt.Errorf("database: load limits: %w", err)
	}
	var o LimitOverrides
	if err := json.Unmarshal(raw, &o); err != nil {
		return LimitOverrides{}, fmt.Errorf("database: load limits: %w", err)
	}
	return o, nil
}

// SaveOverrides stores the overrides, returning what they were.
func SaveOverrides(ctx context.Context, pool *pgxpool.Pool, o LimitOverrides, by int64) (LimitOverrides, error) {
	raw, err := json.Marshal(o)
	if err != nil {
		return LimitOverrides{}, err
	}
	var before LimitOverrides
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var old []byte
		err := tx.QueryRow(ctx, `SELECT value FROM ellie_config WHERE key = $1 FOR UPDATE`, configKey).Scan(&old)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			if err := json.Unmarshal(old, &before); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO ellie_config (key, value, updated_by) VALUES ($1, $2, $3)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
			configKey, raw, by)
		return err
	})
	if err != nil {
		return LimitOverrides{}, fmt.Errorf("database: save limits: %w", err)
	}
	return before, nil
}

// LoadOverrides reads the overrides (none if never saved); with it *DB is the
// bot's ConfigSource.
func (d *DB) LoadOverrides(ctx context.Context) (LimitOverrides, error) {
	return LoadOverrides(ctx, d.pool)
}
