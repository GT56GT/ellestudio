package bot

// budget.go is the bot's side of the studio-wide daily cap, so a sudden rush
// can't run through the Cartesia credits. Each user's own allowance still
// applies first. The counters live in internal/store.

import (
	"context"
	"errors"
	"time"

	"ellestudio/internal/store"
)

// SetBudget replaces the in-memory studio counter (use the database's so
// every instance shares one). It is guarded so the cap still holds while
// the database is unreachable.
func (b *Bot) SetBudget(bu Budget) {
	if bu != nil {
		b.budget = store.GuardBudget(bu)
	}
}

func (b *Bot) studioCapped() bool {
	return b.lim().StudioDailyChars > 0 || b.lim().StudioDailyAudioSecs > 0
}

// spendStudio takes chars and secs from today's studio-wide budget. While
// the database is unreachable the cap is enforced from a local count;
// unknown reports that it couldn't be checked at all (the request is
// refused, failing closed).
func (b *Bot) spendStudio(chars, secs int) (ok, unknown bool) {
	if !b.studioCapped() {
		return true, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	day := today()
	ok, err := b.budget.Spend(ctx, day, chars, secs, b.lim().StudioDailyChars, b.lim().StudioDailyAudioSecs)
	if err != nil {
		b.log.Warn("studio budget database unreachable; using the local count", "allowed", ok, "err", err)
	}
	if errors.Is(err, errBudgetUnknown) {
		return false, true
	}
	if !ok {
		b.log.Warn("studio daily cap reached", "chars", chars, "secs", secs)
		b.alertAdmins("budget:"+day, header("🎟", "Studio daily cap reached", "")+
			"Everyone together has used today's studio-wide allowance, so new readings and transcriptions are refused until midnight UTC.\n\n"+
			"Raise <code>STUDIO_DAILY_CHAR_LIMIT</code> or <code>STUDIO_DAILY_TRANSCRIBE_MINUTES</code> on the server if you want more.")
	}
	return ok, false
}

func (b *Bot) refundStudio(chars, secs int) {
	if !b.studioCapped() || (chars <= 0 && secs <= 0) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pgTimeout)
	defer cancel()
	if err := b.budget.Refund(ctx, today(), chars, secs); err != nil {
		b.log.Warn("refund studio budget", "err", err)
	}
}

// studioRefusal explains a refusal by the studio-wide cap.
func studioRefusal(u Usage) screen {
	if u.StudioUnknown {
		return busyScreen(errServiceDown)
	}
	return studioFullScreen()
}

// studioFullScreen explains that the whole studio hit today's cap.
func studioFullScreen() screen {
	return noticeScreen("🎟", "Ellie is resting for today",
		"So many people used Ellie today that the studio reached its daily limit. Nothing was charged.\n\n"+
			"🔄 It opens again in <b>"+fmtWait(untilReset())+"</b> (midnight UTC).")
}

// studioLines is the studio-wide status for the admin Stats screen.
func (b *Bot) studioLines(ctx context.Context) string {
	out := ""
	if chars, secs, err := b.budget.Used(ctx, today()); err == nil && b.studioCapped() {
		c := fmtInt(chars)
		if b.lim().StudioDailyChars > 0 {
			c += " of " + fmtInt(b.lim().StudioDailyChars)
		}
		a := fmtMinutes(secs)
		if b.lim().StudioDailyAudioSecs > 0 {
			a += " of " + fmtMinutes(b.lim().StudioDailyAudioSecs)
		}
		out += field("🎟", "Studio today", c+" characters · "+a)
	}
	if open, left := b.breaker.isOpen(); open {
		out += field("⚠️", "Voice work paused", "for "+fmtWait(left.Round(time.Second)))
	}
	if q := b.jobs.queued(); q > 0 {
		out += field("⏳", "Waiting in line", fmtInt(q))
	}
	return out
}
