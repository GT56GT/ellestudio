package bot

import (
	"context"
	"strings"
	"testing"
)

func TestStudioCapStopsEveryoneAndRefunds(t *testing.T) {
	fx := newFixture(t, Config{
		AdminUsers: map[int64]bool{1: true}, DailyChars: 2000, StudioDailyChars: 10,
	})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "hello"))
	if len(fx.tg.find("sendVoice")) != 1 {
		t.Fatal("a reading within the studio cap was refused")
	}
	fx.bot.handleUpdate(ctx, msg(8, "hello world"))
	fx.bot.wg.Wait()
	if got := fx.tg.lastTextTo(8); !strings.Contains(got, "resting for today") {
		t.Fatalf("user told %q", got)
	}
	if u := fx.bot.store.UsageOn(8, today()); u.Chars != 0 {
		t.Fatalf("refused user charged %d characters", u.Chars)
	}
	if c, _, _ := fx.bot.budget.Used(ctx, today()); c != 5 {
		t.Fatalf("studio used %d, want 5", c)
	}
	if got := fx.tg.lastTextTo(1); !strings.Contains(got, "Studio daily cap reached") {
		t.Fatalf("admin alert = %q", got)
	}
	// Admins aren't held back by the studio cap.
	fx.bot.handleUpdate(ctx, msg(1, "hello world"))
	if len(fx.tg.find("sendVoice")) != 2 {
		t.Fatal("admin reading refused by the studio cap")
	}
}
