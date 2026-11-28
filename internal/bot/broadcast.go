package bot

// broadcast.go sends the admin panel's announcements. The panel queues a
// broadcast in ellie_broadcasts; the bot sends it to everyone who uses the
// private chat (and hasn't opted out or been banned) at a gentle pace that
// leaves room for normal replies, records progress as it goes so a restart
// resumes where it stopped, and stops early if an admin cancels it.

import (
	"context"
	"errors"
	"fmt"
	"html"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ellestudio/internal/telegram"
)

// Audiences a broadcast can go to.
const (
	AudienceEveryone = "everyone"   // everyone who uses the private chat
	AudienceActive   = "active_30d" // of those, people active in the last 30 days
)

// Broadcast is one announcement being sent.
type Broadcast struct {
	ID       int64
	Text     string
	Audience string
	Cursor   int64 // the last user ID handled
}

// BroadcastQueue is where broadcasts live (the database).
type BroadcastQueue interface {
	// ClaimBroadcast takes a queued broadcast (or one whose sender stopped).
	ClaimBroadcast(ctx context.Context) (Broadcast, bool, error)
	// Recipients returns up to limit user IDs after the cursor.
	Recipients(ctx context.Context, b Broadcast, limit int) ([]int64, error)
	// Progress records a batch and reports whether an admin cancelled.
	Progress(ctx context.Context, id, cursor int64, sent, failed int) (cancelled bool, err error)
	// FinishBroadcast marks it done.
	FinishBroadcast(ctx context.Context, id int64) error
}

// SetBroadcasts turns on the broadcast sender.
func (b *Bot) SetBroadcasts(q BroadcastQueue) { b.broadcasts = q }

const (
	broadcastPoll  = 10 * time.Second
	broadcastBatch = 50
)

// broadcastGap spaces sends to about 10 a second, well under Telegram's
// limit, so readings keep flowing while an announcement goes out (tests
// shorten it).
var broadcastGap = 100 * time.Millisecond

// BroadcastHTML is how an announcement looks: the admin's text, escaped.
func BroadcastHTML(text string) string {
	return "📣 " + html.EscapeString(text)
}

// newsKeys is the opt-out button under every announcement.
func newsKeys() *telegram.InlineKeyboardMarkup {
	return (&keys{}).row(btn("🔕 Stop these messages", "news:off")).build()
}

func (b *Bot) broadcastLoop(ctx context.Context) {
	if b.broadcasts == nil {
		return
	}
	t := time.NewTicker(broadcastPoll)
	defer t.Stop()
	for {
		bc, ok, err := b.broadcasts.ClaimBroadcast(ctx)
		if err != nil && ctx.Err() == nil {
			b.log.Warn("broadcast: claim", "err", err)
		}
		if ok {
			b.sendBroadcast(ctx, bc)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (b *Bot) sendBroadcast(ctx context.Context, bc Broadcast) {
	b.log.Info("broadcast: sending", "id", bc.ID, "audience", bc.Audience, "from", bc.Cursor)
	text := BroadcastHTML(bc.Text)
	for ctx.Err() == nil {
		ids, err := b.broadcasts.Recipients(ctx, bc, broadcastBatch)
		if err != nil {
			b.log.Warn("broadcast: recipients", "id", bc.ID, "err", err)
			return // resumed once the claim goes stale
		}
		if len(ids) == 0 {
			if err := b.broadcasts.FinishBroadcast(ctx, bc.ID); err != nil {
				b.log.Warn("broadcast: finish", "id", bc.ID, "err", err)
			}
			b.log.Info("broadcast: done", "id", bc.ID)
			return
		}
		sent, failed := 0, 0
		for _, uid := range ids {
			if _, err := b.tg.SendMessage(ctx, telegram.SendMessageParams{
				ChatID: uid, Text: text, ParseMode: "HTML", NoPreview: true, ReplyMarkup: newsKeys(),
			}); err != nil {
				if ctx.Err() != nil {
					return
				}
				failed++ // most often: the person blocked Ellie
			} else {
				sent++
			}
			bc.Cursor = uid
			sleep(ctx, broadcastGap)
		}
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pgTimeout)
		cancelled, err := b.broadcasts.Progress(pctx, bc.ID, bc.Cursor, sent, failed)
		cancel()
		if err != nil {
			b.log.Warn("broadcast: progress", "id", bc.ID, "err", err)
			return
		}
		if cancelled {
			b.log.Info("broadcast: cancelled by an admin", "id", bc.ID)
			return
		}
	}
}

// setNews turns announcements on or off for the user who tapped the button
// under one.
func (b *Bot) setNews(ctx context.Context, q *telegram.CallbackQuery, on bool) {
	if _, err := b.store.Update(q.From.ID, func(v *Settings) { v.NoNews = !on }); err != nil {
		b.log.Warn("news setting", "err", err)
		_ = b.tg.AnswerCallbackQuery(ctx, q.ID, "Couldn't save that. Try again.")
		return
	}
	k, note := (&keys{}).row(btn("🔔 Get these messages again", "news:on")).build(), "You won't get these messages any more."
	if on {
		k, note = newsKeys(), "You'll get these messages again."
	}
	_ = b.tg.AnswerCallbackQuery(ctx, q.ID, note)
	if q.Message != nil {
		if err := b.tg.EditMessageReplyMarkup(ctx, q.Message.Chat.ID, q.Message.MessageID, k); err != nil {
			b.log.Debug("news: edit buttons", "err", err)
		}
	}
}

// ---------------------------------------------------------------- Postgres

// recipientsWhere picks who gets a broadcast: people who use the private
// chat, haven't opted out and aren't banned ($1 = audience). Every test is
// NULL-safe: a NOT over a NULL would silently drop the row.
const recipientsWhere = `(settings->>'joined')::boolean
	AND NOT COALESCE((settings->>'no_news')::boolean, false)
	AND NOT (COALESCE(jsonb_typeof(settings->'ban'), '') = 'object'
	         AND (settings->'ban'->'until' IS NULL OR (settings->'ban'->>'until')::timestamptz > now()))
	AND ($1 = 'everyone' OR user_id IN (
	     SELECT user_id FROM ellie_events WHERE at > now() - interval '30 days'))`

// CountRecipients is how many people a broadcast to audience would reach.
func CountRecipients(ctx context.Context, pool *pgxpool.Pool, audience string) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM ellie_user_settings WHERE user_id > 0 AND `+recipientsWhere, audience).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("database: count recipients: %w", err)
	}
	return n, nil
}

// Broadcasts returns the database's broadcast queue.
func NewBroadcasts(d *DB) BroadcastQueue { return pgBroadcasts{d.Pool()} }

type pgBroadcasts struct{ pool *pgxpool.Pool }

// broadcastStale is how long a sending broadcast may go without progress
// before another instance resumes it (its sender stopped).
const broadcastStale = 2 * time.Minute

func (p pgBroadcasts) ClaimBroadcast(ctx context.Context) (Broadcast, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	var bc Broadcast
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT id, text, audience, cursor, status FROM ellie_broadcasts
			WHERE status = 'queued' OR (status = 'sending' AND updated_at < now() - make_interval(secs => $1))
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1`, broadcastStale.Seconds()).
			Scan(&bc.ID, &bc.Text, &bc.Audience, &bc.Cursor, &status)
		if err != nil {
			return err
		}
		if status == "queued" {
			var total int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM ellie_user_settings WHERE user_id > 0 AND `+recipientsWhere, bc.Audience).Scan(&total); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE ellie_broadcasts SET status = 'sending', total = $2, started_at = now(), updated_at = now() WHERE id = $1`, bc.ID, total)
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE ellie_broadcasts SET updated_at = now() WHERE id = $1`, bc.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, false, nil
	}
	if err != nil {
		return Broadcast{}, false, fmt.Errorf("database: claim broadcast: %w", err)
	}
	return bc, true, nil
}

func (p pgBroadcasts) Recipients(ctx context.Context, bc Broadcast, limit int) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	rows, err := p.pool.Query(ctx, `SELECT user_id FROM ellie_user_settings
		WHERE user_id > $2 AND `+recipientsWhere+` ORDER BY user_id LIMIT $3`, bc.Audience, max(bc.Cursor, 0), limit)
	if err != nil {
		return nil, fmt.Errorf("database: recipients: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("database: recipients: %w", err)
	}
	return ids, nil
}

func (p pgBroadcasts) Progress(ctx context.Context, id, cursor int64, sent, failed int) (bool, error) {
	var status string
	err := p.pool.QueryRow(ctx, `UPDATE ellie_broadcasts
		SET cursor = $2, sent = sent + $3, failed = failed + $4, updated_at = now()
		WHERE id = $1 RETURNING status`, id, cursor, sent, failed).Scan(&status)
	if err != nil {
		return false, fmt.Errorf("database: broadcast progress: %w", err)
	}
	return status == "cancelled", nil
}

func (p pgBroadcasts) FinishBroadcast(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, pgTimeout)
	defer cancel()
	_, err := p.pool.Exec(ctx, `UPDATE ellie_broadcasts SET status = 'done', finished_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'sending'`, id)
	if err != nil {
		return fmt.Errorf("database: finish broadcast: %w", err)
	}
	return nil
}
