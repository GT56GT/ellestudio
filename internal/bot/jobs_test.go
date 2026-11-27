package bot

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"ellestudio/internal/cartesia"
)

// waitFor polls cond for up to two seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestJobLineServesInArrivalOrder(t *testing.T) {
	l := newJobLine(1, 0)
	ctx := context.Background()
	if err := l.wait(ctx, time.Hour, nil); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.wait(ctx, time.Hour, nil); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			l.release()
		}()
		waitFor(t, "caller to queue", func() bool { return l.queued() == i })
	}
	l.release()
	wg.Wait()
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("served %v, want [1 2 3]", order)
	}
}

func TestJobLineTurnsAwayWhenFull(t *testing.T) {
	l := newJobLine(1, 1)
	ctx := context.Background()
	_ = l.wait(ctx, time.Hour, nil)
	go func() { _ = l.wait(ctx, time.Hour, nil) }()
	waitFor(t, "first waiter", func() bool { return l.queued() == 1 })
	if err := l.wait(ctx, time.Hour, nil); !errors.Is(err, errLineFull) {
		t.Fatalf("err = %v, want errLineFull", err)
	}
}

func TestJobLineCancelledCallerLeaves(t *testing.T) {
	l := newJobLine(1, 0)
	_ = l.wait(context.Background(), time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- l.wait(ctx, time.Hour, nil) }()
	waitFor(t, "waiter", func() bool { return l.queued() == 1 })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if l.queued() != 0 {
		t.Fatal("cancelled caller still in line")
	}
	l.release() // the slot must be free again, not handed to the leaver
	if err := l.wait(context.Background(), time.Hour, nil); err != nil {
		t.Fatal(err)
	}
}

func TestJobLineTellsPlaceInLine(t *testing.T) {
	l := newJobLine(1, 0)
	ctx := context.Background()
	_ = l.wait(ctx, time.Hour, nil)
	go func() { _ = l.wait(ctx, time.Hour, nil) }()
	waitFor(t, "first waiter", func() bool { return l.queued() == 1 })
	told := make(chan int, 1)
	go func() { _ = l.wait(ctx, time.Millisecond, func(pos int) { told <- pos }) }()
	select {
	case pos := <-told:
		if pos != 2 {
			t.Fatalf("told #%d, want #2", pos)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("never told the place in line")
	}
}

func TestBreakerOpensAfterRepeatedFailures(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	br := &breaker{now: func() time.Time { return now }}
	boom := &cartesia.APIError{StatusCode: http.StatusInternalServerError}
	for i := 1; i < breakerFails; i++ {
		if br.record(int64(i), boom) != "" {
			t.Fatalf("opened after %d failures", i)
		}
	}
	if br.record(1, nil) != "" || br.fails != 0 {
		t.Fatal("a success should reset the count")
	}
	for i := 1; i < breakerFails; i++ {
		br.record(int64(i), boom)
	}
	if got := br.record(9, boom); got != tripErrors {
		t.Fatalf("trip = %q, want errors", got)
	}
	if open, left := br.isOpen(); !open || left != breakerPause {
		t.Fatalf("open=%v left=%v", open, left)
	}
	now = now.Add(breakerPause + time.Second)
	if open, _ := br.isOpen(); open {
		t.Fatal("still open after the pause")
	}
	// The probe after a pause fails: reopen at once.
	if br.record(1, boom) != tripErrors {
		t.Fatal("a failed probe should reopen the breaker")
	}
}

func TestOnePersonCantPauseTheStudio(t *testing.T) {
	br := &breaker{}
	boom := &cartesia.APIError{StatusCode: http.StatusInternalServerError}
	for range 3 * breakerFails {
		if br.record(7, boom) != "" {
			t.Fatal("one user's failures paused the studio")
		}
	}
	br.record(8, boom)
	if br.record(9, boom) != tripErrors {
		t.Fatal("failures from three people should pause it")
	}
}

func TestBreakerReadsErrorsCorrectly(t *testing.T) {
	br := &breaker{}
	if got := br.record(7, &cartesia.APIError{StatusCode: http.StatusPaymentRequired, ErrorCode: cartesia.ErrQuotaExceeded}); got != tripCredits {
		t.Fatalf("credits: trip = %q", got)
	}
	for _, err := range []error{
		&cartesia.APIError{StatusCode: http.StatusBadRequest, ErrorCode: cartesia.ErrVoiceNotFound},
		context.Canceled,
		nil,
	} {
		if fault, _ := cartesiaFault(err); fault {
			t.Errorf("%v counted as Cartesia trouble", err)
		}
	}
	for _, err := range []error{context.DeadlineExceeded, errors.New("connection reset"), &cartesia.APIError{StatusCode: 503}} {
		if fault, _ := cartesiaFault(err); !fault {
			t.Errorf("%v not counted as Cartesia trouble", err)
		}
	}
}

func TestCartesiaOutagePausesReadingsAndAlertsAdmins(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyChars: 2000})
	fx.ttsFail = true
	ctx := context.Background()
	for i := range breakerFails {
		fx.bot.handleUpdate(ctx, msg(int64(7+i%breakerUsers), "hello there"))
	}
	fx.ttsFail = false
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	fx.bot.wg.Wait()
	if got := fx.tg.lastTextTo(7); !strings.Contains(got, "taking a short break") {
		t.Fatalf("user told %q", got)
	}
	if n := len(fx.tg.find("sendVoice")); n != 0 {
		t.Fatalf("sent %d voices while paused", n)
	}
	if got := fx.tg.lastTextTo(1); !strings.Contains(got, "Cartesia keeps failing") {
		t.Fatalf("admin alert = %q", got)
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
		t.Fatalf("user charged %d characters for failed readings", u.Chars)
	}
}

func TestFullLineTurnsReadingsAway(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}, DailyChars: 2000})
	fx.bot.jobs = newJobLine(1, 1)
	ctx := context.Background()
	_ = fx.bot.jobs.wait(ctx, time.Hour, nil) // every slot busy
	go func() { _ = fx.bot.jobs.wait(ctx, time.Hour, nil) }()
	waitFor(t, "line to fill", func() bool { return fx.bot.jobs.queued() == 1 })
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	if got := fx.tg.lastTextTo(7); !strings.Contains(got, "Ellie is very busy") {
		t.Fatalf("user told %q", got)
	}
	if u := fx.bot.store.UsageOn(7, today()); u.Chars != 0 {
		t.Fatalf("user charged %d characters", u.Chars)
	}
}

func TestWaitingReaderSeesPlaceThenGetsVoice(t *testing.T) {
	old := lineNoticeAfter
	lineNoticeAfter = time.Millisecond
	t.Cleanup(func() { lineNoticeAfter = old })
	fx := newFixture(t, Config{Concurrency: 1})
	ctx := context.Background()
	_ = fx.bot.jobs.wait(ctx, time.Hour, nil) // someone else's job is running
	done := make(chan struct{})
	go func() { fx.bot.handleUpdate(ctx, msg(7, "hello there")); close(done) }()
	waitFor(t, "line notice", func() bool { return strings.Contains(fx.tg.lastTextTo(7), "#1</b> in line") })
	fx.bot.jobs.release()
	<-done
	if len(fx.tg.find("deleteMessage")) != 1 {
		t.Error("the line notice should be deleted once the reading starts")
	}
	if len(fx.tg.find("sendVoice")) != 1 {
		t.Error("the reading never arrived")
	}
}

func TestSweepDropsIdleState(t *testing.T) {
	fx := newFixture(t, Config{})
	b := fx.bot
	b.remember(7, func(r *recent) { r.spoken = "hi" })
	b.resetSession(7, "", nil)
	b.setFlow(7, &flow{kind: flowSearch})
	b.remember(8, func(r *recent) { r.spoken = "fresh" })

	b.sweep(time.Now().Add(sessionIdle + time.Minute))
	if b.session(7) != nil {
		t.Error("idle browse session kept")
	}
	if b.lastSpoken(7) == "" {
		t.Error("remembered text dropped too early")
	}
	b.sweep(time.Now().Add(recentIdle + time.Minute))
	if b.lastSpoken(7) != "" || b.lastSpoken(8) != "" || len(b.flows) != 0 {
		t.Error("old remembered text or expired flows kept")
	}
}

// lastTextTo returns the text of the most recent message sent or edited in chat.
func (f *fakeTelegram) lastTextTo(chat int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		c := f.calls[i]
		if c.Method != "sendMessage" && c.Method != "editMessageText" {
			continue
		}
		if id, _ := c.JSON["chat_id"].(float64); int64(id) == chat {
			s, _ := c.JSON["text"].(string)
			return s
		}
	}
	return ""
}
