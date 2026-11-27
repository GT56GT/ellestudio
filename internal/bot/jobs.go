package bot

// jobs.go runs Cartesia work fairly when many people ask at once.
//
//   - Jobs wait in one first-come, first-served line with a size cap. Waiting
//     never holds up the handlers that answer taps and commands, and a full
//     line turns new jobs away with a clear message instead of a long stall.
//   - A breaker stops sending work while Cartesia keeps failing (or says the
//     credits ran out), tells users plainly, and alerts the admins once.

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"sort"
	"sync"
	"time"

	"ellestudio/internal/cartesia"
	"ellestudio/internal/telegram"
)

var (
	// errLineFull means too many jobs are already waiting.
	errLineFull = errors.New("bot: the waiting line is full")
	// errServiceDown means the breaker is open: Cartesia is failing.
	errServiceDown = errors.New("bot: the voice service is paused")
)

// lineNoticeAfter is how long a job waits before the user is told their
// place in line, so short waits don't flash a message (tests shorten it).
var lineNoticeAfter = 2 * time.Second

const (
	// defaultQueueLimit is how many jobs may wait when JOB_QUEUE_LIMIT is unset.
	defaultQueueLimit = 50
	// maxHandlers bounds update handlers running at once. It is well above
	// the Cartesia slots plus the line, so taps always find room.
	maxHandlers = 256
)

// ---------------------------------------------------------------- the line

// jobLine hands out a fixed number of slots in arrival order.
type jobLine struct {
	mu      sync.Mutex
	free    int
	limit   int // most callers that may wait; 0 = no cap
	waiting []*ticket
}

// ticket is one waiting caller; ready is closed when it is given a slot.
type ticket struct{ ready chan struct{} }

func newJobLine(slots, limit int) *jobLine {
	return &jobLine{free: max(slots, 1), limit: limit}
}

// wait takes a slot, queuing behind earlier callers. If the caller is still
// waiting after noticeAfter, waiting (when not nil) is called once with their
// place in line. It returns errLineFull when the line is at its cap, or ctx's
// error if ctx ends first.
func (l *jobLine) wait(ctx context.Context, noticeAfter time.Duration, waiting func(pos int)) error {
	l.mu.Lock()
	if l.free > 0 && len(l.waiting) == 0 {
		l.free--
		l.mu.Unlock()
		return nil
	}
	if l.limit > 0 && len(l.waiting) >= l.limit {
		l.mu.Unlock()
		return errLineFull
	}
	t := &ticket{ready: make(chan struct{})}
	l.waiting = append(l.waiting, t)
	l.mu.Unlock()

	var notice <-chan time.Time
	if waiting != nil {
		timer := time.NewTimer(noticeAfter)
		defer timer.Stop()
		notice = timer.C
	}
	for {
		select {
		case <-t.ready:
			return nil
		case <-notice:
			notice = nil
			if pos := l.position(t); pos > 0 {
				waiting(pos)
			}
		case <-ctx.Done():
			if !l.leave(t) {
				l.release() // granted just as ctx ended: pass the slot on
			}
			return ctx.Err()
		}
	}
}

// release frees a slot, handing it straight to the first caller in line.
func (l *jobLine) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.waiting) > 0 {
		t := l.waiting[0]
		l.waiting = l.waiting[1:]
		close(t.ready)
		return
	}
	l.free++
}

// position is t's place in line (1 = next), or 0 once it holds a slot.
func (l *jobLine) position(t *ticket) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, w := range l.waiting {
		if w == t {
			return i + 1
		}
	}
	return 0
}

// leave takes t out of the line; false means it had already got a slot.
func (l *jobLine) leave(t *ticket) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, w := range l.waiting {
		if w == t {
			l.waiting = append(l.waiting[:i], l.waiting[i+1:]...)
			return true
		}
	}
	return false
}

// queued is how many callers are waiting.
func (l *jobLine) queued() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.waiting)
}

// ---------------------------------------------------------------- the breaker

const (
	// breakerFails failures in a row pause Cartesia work for breakerPause.
	breakerFails = 5
	breakerPause = time.Minute
	// creditsPause is the pause when Cartesia says the credits ran out.
	creditsPause = 10 * time.Minute
	// alertEvery spaces out repeated admin alerts of one kind.
	alertEvery = 30 * time.Minute
)

// Why the breaker opened.
const (
	tripErrors  = "errors"
	tripCredits = "credits"
)

// breaker pauses Cartesia work while it keeps failing. After a pause the
// next job is let through as a probe: success closes the breaker, another
// failure reopens it at once.
type breaker struct {
	mu        sync.Mutex
	fails     int
	openUntil time.Time
	now       func() time.Time // tests replace it
}

func (br *breaker) clock() time.Time {
	if br.now != nil {
		return br.now()
	}
	return time.Now()
}

// isOpen reports whether work is paused, and for how much longer.
func (br *breaker) isOpen() (bool, time.Duration) {
	br.mu.Lock()
	defer br.mu.Unlock()
	left := br.openUntil.Sub(br.clock())
	return left > 0, left
}

// record notes how a Cartesia call went. It returns why the breaker just
// opened (tripErrors or tripCredits), or "" if it didn't.
func (br *breaker) record(err error) string {
	fault, credits := cartesiaFault(err)
	br.mu.Lock()
	defer br.mu.Unlock()
	switch {
	case credits:
		br.fails = breakerFails
		br.openUntil = br.clock().Add(creditsPause)
		return tripCredits
	case fault:
		br.fails++
		if br.fails >= breakerFails {
			br.openUntil = br.clock().Add(breakerPause)
			return tripErrors
		}
	case err == nil:
		br.fails = 0
	}
	return ""
}

// cartesiaFault reports whether err means Cartesia itself is in trouble (as
// opposed to a bad request, which says nothing about its health), and
// whether it is out of credits.
func cartesiaFault(err error) (fault, credits bool) {
	if err == nil || errors.Is(err, context.Canceled) {
		return false, false
	}
	if ae, ok := cartesia.AsAPIError(err); ok {
		switch {
		case ae.ErrorCode == cartesia.ErrQuotaExceeded || ae.StatusCode == http.StatusPaymentRequired:
			return true, true
		case ae.StatusCode >= 500:
			return true, false
		}
		return false, false
	}
	return true, false // timeouts and network errors
}

// ---------------------------------------------------------------- the bot side

// acquire takes a Cartesia slot, waiting in line behind earlier jobs.
// waiting, if not nil, is told the caller's place in line once they have
// waited a moment. It returns errServiceDown while Cartesia is paused,
// errLineFull when the line is full, or ctx's error.
func (b *Bot) acquire(ctx context.Context, waiting func(pos int)) error {
	if open, _ := b.breaker.isOpen(); open {
		return errServiceDown
	}
	return b.jobs.wait(ctx, lineNoticeAfter, waiting)
}

// release frees the slot and records how the Cartesia call went.
func (b *Bot) release(err error) {
	b.jobs.release()
	switch b.breaker.record(err) {
	case tripCredits:
		b.log.Error("Cartesia is out of credits; pausing voice work", "pause", creditsPause, "err", err)
		b.alertAdmins(tripCredits, header("⚠️", "Cartesia credits ran out", "")+
			"Cartesia refused a request for lack of credits or quota, so Ellie paused voice work for "+
			fmtWait(creditsPause)+". Users are told to try again later and aren't charged.\n\n"+
			"Top up at https://play.cartesia.ai, then Ellie resumes by itself.")
	case tripErrors:
		b.log.Error("Cartesia keeps failing; pausing voice work", "pause", breakerPause, "err", err)
		b.alertAdmins(tripErrors, header("⚠️", "Cartesia keeps failing", "")+
			fmt.Sprintf("%d Cartesia calls failed in a row, so Ellie paused voice work for %s and will then try again. Users aren't charged.\n\nLast error: <code>%s</code>",
				breakerFails, fmtWait(breakerPause), html.EscapeString(truncate(err.Error(), 300))))
	}
}

// waitTurn takes a Cartesia slot for a chat request, showing the user their
// place in line while they wait (in pr when there is one). On false the
// user has been told why (unless the request was cancelled) and the caller
// refunds and returns.
func (b *Bot) waitTurn(ctx context.Context, m *telegram.Message, pr *progress) bool {
	var note *telegram.Message
	err := b.acquire(ctx, func(pos int) {
		if pr != nil {
			pr.update(ctx, m.Chat.ID, lineScreen(pos))
			return
		}
		note = b.sendScreenTo(ctx, m.Chat.ID, m.Thread(), m.MessageID, lineScreen(pos))
	})
	if note != nil {
		if derr := b.tg.DeleteMessage(ctx, m.Chat.ID, note.MessageID); derr != nil {
			b.log.Debug("delete line notice", "err", derr)
		}
	}
	if err == nil {
		return true
	}
	if ctx.Err() != nil {
		return false
	}
	b.log.Warn("job turned away", "user", m.From.ID, "reason", err, "queued", b.jobs.queued())
	if pr != nil {
		pr.finish(ctx, m.Chat.ID, busyScreen(err))
	} else {
		b.replyNotice(ctx, m, busyScreen(err))
	}
	return false
}

// lineScreen tells a waiting user their place in line.
func lineScreen(pos int) screen {
	return screen{header(icWait, "You're in line", "") + fmt.Sprintf(
		"Lots of people are using Ellie right now. You're <b>#%d</b> in line, and yours starts by itself in a moment.", pos), nil}
}

// busyScreen explains why a job couldn't start.
func busyScreen(err error) screen {
	if errors.Is(err, errServiceDown) {
		return noticeScreen(icWarn, "Voices are taking a short break",
			"Ellie's voice service is having trouble right now. Nothing was charged. Please try again in a few minutes.")
	}
	return noticeScreen(icWait, "Ellie is very busy",
		"Too many people are using Ellie right now. Nothing was charged. Please try again in a minute.")
}

// ---------------------------------------------------------------- admin alerts

// alertAdmins messages every admin about a problem, at most once per
// alertEvery for each kind. It never blocks the caller.
func (b *Bot) alertAdmins(kind, text string) {
	b.alertMu.Lock()
	if last, ok := b.alerted[kind]; ok && time.Since(last) < alertEvery {
		b.alertMu.Unlock()
		return
	}
	b.alerted[kind] = time.Now()
	b.alertMu.Unlock()

	admins := make([]int64, 0, len(b.cfg.AdminUsers))
	for uid, ok := range b.cfg.AdminUsers {
		if ok {
			admins = append(admins, uid)
		}
	}
	sort.Slice(admins, func(i, j int) bool { return admins[i] < admins[j] })
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, uid := range admins {
			if _, err := b.tg.SendMessage(ctx, telegram.SendMessageParams{
				ChatID: uid, Text: text, ParseMode: "HTML", NoPreview: true,
			}); err != nil {
				b.log.Warn("admin alert failed", "admin", uid, "kind", kind, "err", err)
			}
		}
	}()
}
