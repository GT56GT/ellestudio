package bot

// broadcast.go sends the admin panel's announcements. The panel queues a
// broadcast in ellie_broadcasts; the bot sends it to everyone who uses the
// private chat (and hasn't opted out or been banned) at a gentle pace that
// leaves room for normal replies, records progress as it goes so a restart
// resumes where it stopped, and stops early if an admin cancels it.

import (
	"context"
	"time"

	"ellestudio/internal/telegram"
)

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
