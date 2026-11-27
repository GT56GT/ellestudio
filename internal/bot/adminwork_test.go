package bot

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"ellestudio/internal/telegram"
)

func ban(t *testing.T, fx *fixture, uid int64, until time.Time) {
	t.Helper()
	var u *time.Time
	if !until.IsZero() {
		u = &until
	}
	if _, err := fx.bot.store.Update(uid, func(v *Settings) {
		v.Ban = &Ban{Until: u, Reason: "spam", By: 1, At: time.Now()}
	}); err != nil {
		t.Fatal(err)
	}
	fx.bot.refreshBans()
}

func TestBannedUserIsToldOnceThenIgnored(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	ban(t, fx, 7, time.Time{})
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	fx.bot.handleUpdate(ctx, msg(7, "hello again"))
	if n := len(fx.tg.find("sendVoice")); n != 0 {
		t.Fatalf("banned user got %d readings", n)
	}
	notices := 0
	for _, c := range fx.tg.find("sendMessage") {
		if s, _ := c.JSON["text"].(string); strings.Contains(s, "Access paused") {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("banned user told %d times, want once", notices)
	}
	if v := fx.bot.store.Get(7); v.Ban == nil || !v.Ban.Notified {
		t.Fatal("notice not recorded")
	}
}

func TestBanCoversButtonsGroupsAndGuestReplies(t *testing.T) {
	fx := guestFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	ban(t, fx, 7, time.Time{})
	ctx := context.Background()

	fx.bot.handleUpdate(ctx, press(7, "nav:home"))
	cb := fx.tg.find("answerCallbackQuery")
	if len(cb) != 1 || !strings.Contains(cb[0].JSON["text"].(string), "paused") {
		t.Fatalf("button answer = %+v", cb)
	}

	before := len(fx.tg.find("sendMessage")) + len(fx.tg.find("sendVoice"))
	fx.bot.handleUpdate(ctx, telegram.Update{Message: &telegram.Message{
		MessageID: 11, From: &telegram.User{ID: 7}, Chat: telegram.Chat{ID: -100500, Type: "supergroup"}, Text: "/say hello",
	}})
	if after := len(fx.tg.find("sendMessage")) + len(fx.tg.find("sendVoice")); after != before {
		t.Fatal("a banned user's group message got a response")
	}

	fx.bot.handleUpdate(ctx, guest(7, "@EllieStudioBot hello"))
	if !strings.Contains(noticeText(guestAnswer(t, fx)), "Access paused") {
		t.Fatal("guest mention not turned away")
	}
}

func TestBansExpireAndSpareAdmins(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	ban(t, fx, 7, time.Now().Add(-time.Minute)) // already over
	ban(t, fx, 1, time.Time{})                  // an admin
	ctx := context.Background()
	fx.bot.handleUpdate(ctx, msg(7, "hello there"))
	fx.bot.handleUpdate(ctx, msg(1, "hello there"))
	if n := len(fx.tg.find("sendVoice")); n != 2 {
		t.Fatalf("readings = %d, want 2 (expired ban and admin)", n)
	}
}

// memQueue is an AdminQueue in memory.
type memQueue struct {
	mu       sync.Mutex
	jobs     []AdminJob
	finished map[int64]error
	names    map[string]map[string]string
}

func (q *memQueue) Claim(context.Context) (AdminJob, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.jobs) == 0 {
		return AdminJob{}, false, nil
	}
	j := q.jobs[0]
	q.jobs = q.jobs[1:]
	return j, true, nil
}

func (q *memQueue) Finish(_ context.Context, id int64, err error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.finished == nil {
		q.finished = map[int64]error{}
	}
	q.finished[id] = err
	return nil
}

func (q *memQueue) SetNames(_ context.Context, kind string, names map[string]string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.names == nil {
		q.names = map[string]map[string]string{}
	}
	q.names[kind] = names
	return nil
}

func TestPanelDeleteRunsInTheBot(t *testing.T) {
	fx := newFixture(t, Config{AdminUsers: map[int64]bool{1: true}})
	ctx := context.Background()
	b := fx.bot
	if err := b.owners.Claim(ctx, kindVoice, "v-mine", 7); err != nil {
		t.Fatal(err)
	}
	if err := b.owners.Claim(ctx, kindDict, "d-mine", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := b.store.Update(9, func(v *Settings) { v.VoiceID, v.VoiceName, v.DictID = "v-mine", "My Clone", "d-mine" }); err != nil {
		t.Fatal(err)
	}
	q := &memQueue{}
	b.SetAdminQueue(q)
	b.runAdminJob(ctx, AdminJob{ID: 1, Kind: JobDeleteVoice, Target: "v-mine", RequestedBy: 1})
	b.runAdminJob(ctx, AdminJob{ID: 2, Kind: JobDeleteDict, Target: "d-mine", RequestedBy: 1})
	b.runAdminJob(ctx, AdminJob{ID: 3, Kind: "rename_everything", Target: "x", RequestedBy: 1})

	if err1, err2 := q.finished[1], q.finished[2]; err1 != nil || err2 != nil {
		t.Fatalf("jobs failed: %v %v", err1, err2)
	}
	if q.finished[3] == nil {
		t.Fatal("an unknown job kind should fail")
	}
	if !strings.Contains(strings.Join(fx.deleted, " "), "/voices/v-mine") {
		t.Fatalf("Cartesia deletes = %v", fx.deleted)
	}
	owners, _ := b.owners.Lookup(ctx, kindVoice, []string{"v-mine"})
	if len(owners) != 0 {
		t.Fatal("deleted voice still has an owner")
	}
	if v := b.store.Get(9); v.VoiceID != "v-default" || v.DictID != "" {
		t.Fatalf("user still points at deleted resources: voice %q dict %q", v.VoiceID, v.DictID)
	}
}

func TestAdminJobsLoopDrainsTheQueueAndSyncsNames(t *testing.T) {
	fx := newFixture(t, Config{})
	q := &memQueue{jobs: []AdminJob{{ID: 1, Kind: JobDeleteVoice, Target: "v-old"}}}
	fx.bot.SetAdminQueue(q)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { fx.bot.adminJobs(ctx); close(done) }()
	waitFor(t, "the job and the name sync", func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		_, finished := q.finished[1]
		return finished && q.names[kindVoice] != nil
	})
	cancel()
	<-done
	if err := q.finished[1]; err != nil {
		t.Fatalf("job error = %v", err)
	}
	if q.names[kindVoice]["v-new"] != "Nova" {
		t.Fatalf("names not synced: %v", q.names)
	}
}

func TestStoreBannedListsBans(t *testing.T) {
	s, _ := OpenStore("", Settings{})
	if _, err := s.Update(3, func(v *Settings) { v.Ban = &Ban{Reason: "x"} }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(4, func(v *Settings) { v.Bonus = 5 }); err != nil {
		t.Fatal(err)
	}
	bans, err := s.Banned()
	if err != nil || len(bans) != 1 || bans[3].Reason != "x" {
		t.Fatalf("bans = %v %v", bans, err)
	}
}
