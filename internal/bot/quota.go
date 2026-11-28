package bot

// quota.go keeps the studio fair when it is open to everyone: each user gets a
// daily allowance of spoken characters and transcribed audio (admins are
// exempt), and each user runs one Cartesia job at a time.

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"ellestudio/internal/telegram"
)

func today() string { return time.Now().UTC().Format("2006-01-02") }

// untilReset is the time left until the allowance resets (midnight UTC).
func untilReset() time.Duration {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return next.Sub(now)
}

func fmtWait(d time.Duration) string {
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h == 0 {
		return fmt.Sprintf("%dm", max(m, 1))
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// fmtInt writes 12345 as "12,345".
func fmtInt(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func fmtMinutes(secs int) string {
	if secs%60 == 0 {
		return strconv.Itoa(secs/60) + " min"
	}
	return strconv.FormatFloat(float64(secs)/60, 'f', 1, 64) + " min"
}

func (b *Bot) limited(uid int64) bool {
	return !b.isAdmin(uid) && (b.lim().DailyChars > 0 || b.lim().DailyAudioSecs > 0 || b.lim().DailyCreations > 0)
}

// spendMake reserves one voice creation; on refusal it returns the screen.
func (b *Bot) spendMake(uid int64) (bool, screen) {
	if b.isAdmin(uid) {
		return true, screen{}
	}
	ok, u, err := b.store.SpendMake(uid, today(), b.lim().DailyCreations)
	if err != nil {
		b.log.Warn("save usage", "err", err)
	}
	if ok {
		return true, screen{}
	}
	body := fmt.Sprintf("You've created <b>%d</b> of your <b>%d</b> voices for today.", u.Makes, b.lim().DailyCreations) +
		"\n\n🔄 You can make more in <b>" + fmtWait(untilReset()) + "</b> (midnight UTC). Your existing voices still work."
	k := (&keys{}).row(btn("🗂 My voices", "do:mine")).nav("", "")
	return false, screen{header("🎟", "Daily voice limit reached", "") + body, k.build()}
}

func (b *Bot) refundMake(uid int64) {
	if !b.isAdmin(uid) {
		b.store.RefundMake(uid, today())
	}
}

// spendChars reserves n spoken characters; false means over the allowance.
func (b *Bot) spendChars(uid int64, n int) (bool, Usage) {
	if b.isAdmin(uid) {
		return true, Usage{}
	}
	ok, u, err := b.store.Spend(uid, today(), n, 0, b.lim().DailyChars, 0)
	if err != nil {
		b.log.Warn("save usage", "err", err)
	}
	if ok {
		if sok, unknown := b.spendStudio(n, 0); !sok {
			b.store.Refund(uid, today(), n, 0, b.lim().DailyChars)
			return false, Usage{Studio: true, StudioUnknown: unknown}
		}
	}
	return ok, u
}

// spendSecs reserves n seconds of audio; false means over the allowance.
func (b *Bot) spendSecs(uid int64, n int) (bool, Usage) {
	if b.isAdmin(uid) {
		return true, Usage{}
	}
	ok, u, err := b.store.Spend(uid, today(), 0, n, 0, b.lim().DailyAudioSecs)
	if err != nil {
		b.log.Warn("save usage", "err", err)
	}
	if ok {
		if sok, unknown := b.spendStudio(0, n); !sok {
			b.store.Refund(uid, today(), 0, n, b.lim().DailyChars)
			return false, Usage{Studio: true, StudioUnknown: unknown}
		}
	}
	return ok, u
}

// refund gives back a user's spend and the studio-wide share of it.
func (b *Bot) refund(uid int64, chars, secs int) {
	if !b.isAdmin(uid) {
		b.store.Refund(uid, today(), chars, secs, b.lim().DailyChars)
		b.refundStudio(chars, secs)
	}
}

// allowanceLines renders today's allowance for the Studio screen.
func (b *Bot) allowanceLines(uid int64) string {
	if !b.limited(uid) {
		return field("🎟", "Today", "unlimited")
	}
	u := b.store.UsageOn(uid, today())
	out := ""
	if b.lim().DailyChars > 0 {
		out += field("🎟", "Speech left today", fmtInt(max(0, b.lim().DailyChars-u.Chars))+" of "+fmtInt(b.lim().DailyChars)+" characters")
	}
	if b.lim().DailyAudioSecs > 0 {
		out += field("🎟", "Transcription left today", fmtMinutes(max(0, b.lim().DailyAudioSecs-u.Secs))+" of "+fmtMinutes(b.lim().DailyAudioSecs))
	}
	if b.lim().DailyCreations > 0 {
		out += field("🎟", "New voices left today", strconv.Itoa(max(0, b.lim().DailyCreations-u.Makes))+" of "+strconv.Itoa(b.lim().DailyCreations))
	}
	return out
}

// limitScreen explains a refused request and when the allowance comes back.
func (b *Bot) limitScreen(speech bool, u Usage, need int) screen {
	var body string
	if speech {
		left := max(0, b.lim().DailyChars-u.Chars)
		body = fmt.Sprintf("You've used <b>%s</b> of your <b>%s</b> characters today.", fmtInt(u.Chars), fmtInt(b.lim().DailyChars))
		if left > 0 {
			body += fmt.Sprintf("\nThat message needs %s, and <b>%s</b> are left, so a shorter one still fits.", fmtInt(need), fmtInt(left))
		}
	} else {
		left := max(0, b.lim().DailyAudioSecs-u.Secs)
		body = fmt.Sprintf("You've used <b>%s</b> of your <b>%s</b> of transcription today.", fmtMinutes(u.Secs), fmtMinutes(b.lim().DailyAudioSecs))
		if left > 0 {
			body += fmt.Sprintf("\nThat recording is %s, and <b>%s</b> is left, so a shorter one still fits.", fmtMinutes(need), fmtMinutes(left))
		}
	}
	body += "\n\n🔄 Your allowance refreshes in <b>" + fmtWait(untilReset()) + "</b> (midnight UTC)."
	k := &keys{}
	if speech && b.referrals() {
		if u.Bonus > 0 {
			body += "\n🎁 You have <b>" + fmtInt(u.Bonus) + "</b> bonus characters, not enough for this message."
		}
		body += "\n🤝 Can't wait? Invite a friend and you both get <b>+" + fmtInt(b.lim().ReferralBonus) + "</b> bonus characters."
		k.row(btn("🤝 Invite friends", "nav:invite"))
	}
	k.row(btn("👤 Profile", "nav:profile")).nav("", "")
	return screen{header("🎟", "Daily limit reached", "") + body, k.build()}
}

// ---------------------------------------------------------------- one job per user

// begin marks uid busy; false means they already have a job running.
func (b *Bot) begin(uid int64) bool {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	if b.busy[uid] {
		return false
	}
	b.busy[uid] = true
	return true
}

func (b *Bot) end(uid int64) {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	delete(b.busy, uid)
}

func (b *Bot) busyNotice(ctx context.Context, m *telegram.Message) {
	s := noticeScreen(icWait, "One moment", "I'm still working on your last request. Send this again once it arrives.")
	b.replyNotice(ctx, m, s)
}
