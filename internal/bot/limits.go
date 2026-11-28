package bot

// limits.go makes the daily limits live: the env vars set the defaults, and
// admins can override any of them (and pause voice work) from the admin
// panel without a redeploy. The bot reloads the overrides every
// configRefresh and swaps them in atomically, so handlers always see one
// consistent set.

import (
	"context"
	"errors"
	"time"
)

// limitsFrom takes the defaults from a Config (the env vars).
func limitsFrom(c Config) Limits {
	return Limits{
		DailyChars: c.DailyChars, DailyAudioSecs: c.DailyAudioSecs,
		DailyCreations: c.DailyCreations, DailyDicts: c.DailyDicts, MaxTTSChars: c.MaxTTSChars,
		StudioDailyChars: c.StudioDailyChars, StudioDailyAudioSecs: c.StudioDailyAudioSecs,
		GroupLimits: c.GroupLimits, GroupCooldown: c.GroupCooldown,
		ReferralBonus: c.ReferralBonus, ReferralDailyCap: c.ReferralDailyCap,
	}
}

// lim returns the limits in force.
func (b *Bot) lim() Limits { return *b.limits.Load() }

// applyOverrides puts o on top of the defaults.
func (b *Bot) applyOverrides(o LimitOverrides) {
	l := o.Apply(b.base)
	b.limits.Store(&l)
}

// ConfigSource is where the overrides are stored (the database).
type ConfigSource interface {
	LoadOverrides(ctx context.Context) (LimitOverrides, error)
}

// SetConfigSource turns on live limits.
func (b *Bot) SetConfigSource(c ConfigSource) { b.config = c }

// configRefresh is how quickly a change in the panel takes effect.
const configRefresh = 30 * time.Second

// refreshLive reloads what admins can change from the panel: the limits
// and the ban list.
func (b *Bot) refreshLive(ctx context.Context) {
	b.refreshBans()
	if b.config == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	o, err := b.config.LoadOverrides(cctx)
	if err != nil {
		b.log.Warn("limits: refresh", "err", err)
		return
	}
	before := b.lim()
	b.applyOverrides(o)
	if after := b.lim(); after != before {
		b.log.Info("limits changed from the admin panel", "paused", after.Paused,
			"daily_chars", after.DailyChars, "studio_chars", after.StudioDailyChars)
	}
}

// errAdminPause means an admin paused voice work from the panel.
var errAdminPause = errors.New("bot: voice work is paused by an admin")
