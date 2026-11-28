package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// exerciseStore runs the same changes against any backend and reopens it to
// check that every kind of write survived a restart.
func exerciseStore(t *testing.T, open func() *SettingsStore, a, b int64) {
	t.Helper()
	s := open()
	if _, err := s.Update(a, func(v *Settings) { v.VoiceID, v.VoiceName, v.Emotion = "voice-1", "Skylar", "calm" }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(b, func(v *Settings) { v.VoiceID, v.AsFile = "voice-1", true }); err != nil {
		t.Fatal(err)
	}
	if ok, _, err := s.Spend(a, "2026-11-19", 120, 30, 2000, 600); !ok || err != nil {
		t.Fatalf("spend: ok=%v err=%v", ok, err)
	}
	if ok, _, err := s.SpendMake(a, "2026-11-19", 3); !ok || err != nil {
		t.Fatalf("spend make: ok=%v err=%v", ok, err)
	}
	if err := s.ForgetVoice("voice-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	r := open()
	defer r.Close()
	ga, gb := r.Get(a), r.Get(b)
	if ga.VoiceID != "" || gb.VoiceID != "" {
		t.Errorf("ForgetVoice not persisted: %q %q", ga.VoiceID, gb.VoiceID)
	}
	if ga.Emotion != "calm" || !gb.AsFile {
		t.Errorf("preferences lost: %+v / %+v", ga, gb)
	}
	if u := r.UsageOn(a, "2026-11-19"); u.Chars != 120 || u.Secs != 30 || u.Makes != 1 {
		t.Errorf("usage lost: %+v", u)
	}
	if !r.Known(a) || !r.Known(b) {
		t.Error("users not known after reopen")
	}
}

func TestFileStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	exerciseStore(t, func() *SettingsStore {
		s, err := OpenStore(path, Settings{Model: "sonic-3.6"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}, 1, 2)
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("settings file perms: %v %v", fi, err)
	}
}

func TestPostgresStorePersists(t *testing.T) {
	db := testDB(t)
	const a, b = -9001, -9002
	ctx := context.Background()
	cleanup := func() {
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id IN ($1, $2)`, a, b)
	}
	cleanup()
	t.Cleanup(cleanup)
	exerciseStore(t, func() *SettingsStore {
		s, err := db.SettingsStore(ctx, Settings{Model: "sonic-3.6"})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}, a, b)
}

// exerciseConcurrentSpends spends from many goroutines through stores (one
// per bot instance) and checks that no spend was lost.
func exerciseConcurrentSpends(t *testing.T, stores []*SettingsStore, uid int64) {
	t.Helper()
	const day, perSpend, spends = "2001-01-02", 7, 40
	var wg sync.WaitGroup
	for i := range spends {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _, err := stores[i%len(stores)].Spend(uid, day, perSpend, 0, 0, 0); !ok || err != nil {
				t.Errorf("spend: ok=%v err=%v", ok, err)
			}
		}()
	}
	wg.Wait()
	for _, s := range stores {
		if u := s.UsageOn(uid, day); u.Chars != perSpend*spends {
			t.Fatalf("usage = %d, want %d: spends were lost", u.Chars, perSpend*spends)
		}
	}
}

func TestFileStoreConcurrentSpends(t *testing.T) {
	s, err := OpenStore(t.TempDir()+"/settings.json", Settings{})
	if err != nil {
		t.Fatal(err)
	}
	exerciseConcurrentSpends(t, []*SettingsStore{s}, 1)
}

// Two instances share the database during a deploy; neither may overwrite
// the other's changes.
func TestPostgresStoreSharedByTwoInstances(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	const uid, inviter, inviteeA = -9101, -9102, -9103
	cleanup := func() {
		_, _ = db.Pool().Exec(ctx, `DELETE FROM ellie_user_settings WHERE user_id = ANY($1)`, []int64{uid, inviter, inviteeA})
	}
	cleanup()
	t.Cleanup(cleanup)
	open := func() *SettingsStore {
		s, err := db.SettingsStore(ctx, Settings{})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	a, b := open(), open()
	exerciseConcurrentSpends(t, []*SettingsStore{a, b}, uid)

	// A change made by one instance is seen by the other at once.
	if _, err := a.Update(uid, func(v *Settings) { v.Emotion = "calm" }); err != nil {
		t.Fatal(err)
	}
	if got := b.Get(uid).Emotion; got != "calm" {
		t.Fatalf("other instance sees emotion %q", got)
	}

	// The same newcomer arriving through two links at once is rewarded once.
	if err := a.Join(inviter, "2001-01-02"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied := 0
	for _, s := range []*SettingsStore{a, b, a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := s.Referral(inviter, inviteeA, "2001-01-02", 100, 5)
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				applied++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if applied != 1 || a.Get(inviter).Bonus != 100 {
		t.Fatalf("referral applied %d times, inviter bonus %d", applied, a.Get(inviter).Bonus)
	}
}

func TestSettingsJSONUnchanged(t *testing.T) {
	until := time.Date(2026, 12, 1, 8, 30, 0, 0, time.UTC)
	v := Settings{
		VoiceID: "v1", VoiceName: "Skylar", Model: "sonic-3.6", Language: "en", Emotion: "calm",
		Speed: 1.25, Volume: 0.75, DictID: "d1", DictName: "Names", AsFile: true,
		UsageDay: "2026-11-28", UsedChars: 120, UsedSecs: 30, UsedMakes: 2, UsedDicts: 1,
		Joined: true, JoinedAt: "2026-11-01", Bonus: 500,
		ReferredBy: 77, Referrals: 3, RefDay: "2026-11-27", RefToday: 2,
		Readings: 9, SpokenChars: 4321, TipDay: "2026-11-26",
		Ban:    &Ban{Until: &until, Reason: "spam", By: 5, At: until.Add(-time.Hour), Notified: true},
		NoNews: true,
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"voice_id":"v1","voice_name":"Skylar","model":"sonic-3.6","language":"en","emotion":"calm","speed":1.25,"volume":0.75,"dict_id":"d1","dict_name":"Names","as_file":true,"usage_day":"2026-11-28","used_chars":120,"used_secs":30,"used_makes":2,"used_dicts":1,"joined":true,"joined_at":"2026-11-01","bonus":500,"referred_by":77,"referrals":3,"ref_day":"2026-11-27","ref_today":2,"readings":9,"spoken_chars":4321,"tip_day":"2026-11-26","ban":{"until":"2026-12-01T08:30:00Z","reason":"spam","by":5,"at":"2026-12-01T07:30:00Z","notified":true},"no_news":true}`
	if string(raw) != want {
		t.Errorf("settings JSON changed:\n got %s\nwant %s", raw, want)
	}
}
