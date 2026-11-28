package bot

// status.go publishes a heartbeat: every few seconds the bot writes its live
// state (the waiting line, the breaker, its limits) to the database, so the
// admin panel, a separate app, can show whether Ellie is up and how busy it
// is.

import (
	"context"
	"time"
)

// heartbeatEvery is how often the bot reports; the panel calls the bot
// offline after a few missed beats.
const heartbeatEvery = 15 * time.Second

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
