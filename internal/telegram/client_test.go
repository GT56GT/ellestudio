package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"chat":{"id":1}}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFloodWaitIsRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":5,"chat":{"id":1}}}`)
	}))
	defer srv.Close()
	start := time.Now()
	// A multipart upload: the body must be sent again in full on the retry.
	m, err := New("T", srv.URL).SendVoice(context.Background(), MediaParams{ChatID: 1, Filename: "a.mp3", Data: []byte("ID3")})
	if err != nil || m.MessageID != 5 {
		t.Fatalf("send = %+v, %v", m, err)
	}
	if calls.Load() != 2 || time.Since(start) < time.Second {
		t.Errorf("want one retry after the flood wait, got %d calls in %v", calls.Load(), time.Since(start))
	}
}

func TestLongFloodWaitIsReturned(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":600}}`)
	}))
	defer srv.Close()
	_, err := New("T", srv.URL).SendMessage(context.Background(), SendMessageParams{ChatID: 1, Text: "hi"})
	var te *Error
	if !errors.As(err, &te) || te.RetryAfter != 600 || calls.Load() != 1 {
		t.Fatalf("a 10-minute wait should be returned at once, got %v after %d calls", err, calls.Load())
	}
}

func TestGroupSendsArePaced(t *testing.T) {
	c := New("T", okServer(t).URL)
	c.SetGroupPacing(100*time.Millisecond, 2, time.Minute)
	ctx := context.Background()
	send := func(chat int64) {
		t.Helper()
		if _, err := c.SendMessage(ctx, SendMessageParams{ChatID: chat, Text: "hi"}); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	for range 4 { // two from the burst, then two paced
		send(-100)
	}
	if d := time.Since(start); d < 180*time.Millisecond {
		t.Errorf("4 group sends took %v, want them paced to about 200ms", d)
	}

	start = time.Now()
	for range 5 {
		send(42) // private chats are never paced
	}
	send(-200) // another group has its own burst
	if d := time.Since(start); d > 80*time.Millisecond {
		t.Errorf("private chats or other groups were held up: %v", d)
	}

	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.SendMessage(cctx, SendMessageParams{ChatID: -100, Text: "hi"}); err == nil {
		t.Error("a paced send ignored a cancelled context")
	}
}

func TestThreadIDOnlyOnSends(t *testing.T) {
	p := SendMessageParams{ChatID: -1, ThreadID: 7, Text: "x"}.payload()
	if p["message_thread_id"] != int64(7) {
		t.Errorf("send payload = %v", p)
	}
	e := SendMessageParams{ChatID: -1, MessageID: 3, ThreadID: 7, Text: "x"}.payload()
	if _, ok := e["message_thread_id"]; ok {
		t.Errorf("edits must not carry a thread: %v", e)
	}
}

func TestMemberPresence(t *testing.T) {
	for status, want := range map[string]bool{
		"creator": true, "administrator": true, "member": true, "left": false, "kicked": false,
	} {
		if got := (ChatMember{Status: status}).Present(); got != want {
			t.Errorf("%s: present = %v", status, got)
		}
	}
	if !(ChatMember{Status: "restricted", IsMember: true}).Present() || (ChatMember{Status: "restricted"}).Present() {
		t.Error("restricted members are present only while is_member")
	}
}

func TestFloodedGroupSendsFailFast(t *testing.T) {
	c := New("T", okServer(t).URL)
	c.SetGroupPacing(100*time.Millisecond, 1, 250*time.Millisecond)
	ctx := context.Background()
	errs := make(chan error, 6)
	start := time.Now()
	for range 6 {
		go func() {
			_, err := c.SendMessage(ctx, SendMessageParams{ChatID: -100, Text: "hi"})
			errs <- err
		}()
	}
	flood := 0
	for range 6 {
		if err := <-errs; errors.Is(err, ErrFlood) {
			flood++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	// One goes at once and two wait 100ms and 200ms; the rest would wait
	// past 250ms, so they fail instead of queueing.
	if flood != 3 {
		t.Errorf("%d of 6 sends failed with ErrFlood, want 3", flood)
	}
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Errorf("flooded sends were held for %v", d)
	}
	if b := c.Backlog(-100); b <= 0 {
		t.Errorf("backlog = %v right after a flood", b)
	}
	if c.Backlog(42) != 0 {
		t.Error("private chats have no backlog")
	}
}
