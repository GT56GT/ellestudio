package bot

import (
	"context"
	"time"
)

// metricsRetention is how long usage events are kept before they are pruned.
const metricsRetention = 90 * 24 * time.Hour

// track records an event without ever failing or slowing the request.
func (b *Bot) track(ctx context.Context, uid int64, kind, source string, n int) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := b.metrics.Record(ctx, Event{At: time.Now().UTC(), User: uid, Kind: kind, Source: source, N: n}); err != nil {
		b.log.Warn("record metric", "kind", kind, "err", err)
	}
}

// SetMetrics replaces the in-memory metrics (used when no database is set).
func (b *Bot) SetMetrics(m Metrics) { b.metrics = m }

// pruneMetrics forgets old events now and then once a day until ctx ends.
func (b *Bot) pruneMetrics(ctx context.Context) {
	for {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := b.metrics.Prune(pctx, time.Now().Add(-metricsRetention)); err != nil && ctx.Err() == nil {
			b.log.Warn("prune metrics", "err", err)
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}
