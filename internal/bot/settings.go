package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// persister is where a Store keeps its data between restarts: a JSON file
// (the default) or Postgres (when DATABASE_URL is set, which also makes it
// a sharedStore). save is given every user and the ones that just changed.
type persister interface {
	save(all map[int64]Settings, changed []int64) error
	close() error
}

// Settings are one user's studio preferences.
type Settings struct {
	VoiceID   string  `json:"voice_id"`
	VoiceName string  `json:"voice_name"`
	Model     string  `json:"model"`
	Language  string  `json:"language"` // "" = let Cartesia decide
	Emotion   string  `json:"emotion"`  // "" = model infers from text
	Speed     float64 `json:"speed"`    // 0 or 1 = default
	Volume    float64 `json:"volume"`   // 0 or 1 = default
	DictID    string  `json:"dict_id"`
	DictName  string  `json:"dict_name"`
	AsFile    bool    `json:"as_file"` // send audio files instead of voice notes

	// Daily allowance bookkeeping (UTC day). Kept across /reset.
	UsageDay  string `json:"usage_day,omitempty"`
	UsedChars int    `json:"used_chars,omitempty"`
	UsedSecs  int    `json:"used_secs,omitempty"`
	UsedMakes int    `json:"used_makes,omitempty"` // voices created today
	UsedDicts int    `json:"used_dicts,omitempty"` // dictionaries created today

	// Joined is set the first time the user talks to the bot in its private
	// chat. Only joined users get guest-mode replies (GUEST_ACCESS=members).
	// Kept across /reset, like everything below.
	Joined   bool   `json:"joined,omitempty"`
	JoinedAt string `json:"joined_at,omitempty"` // UTC day of the first private-chat visit

	// Bonus characters (from referrals) are spent only once the daily
	// character allowance is used up, and never expire.
	Bonus int `json:"bonus,omitempty"`

	// Referrals: who invited this user, how many friends they invited, and
	// how many invite rewards they earned on RefDay (capped per day).
	ReferredBy int64  `json:"referred_by,omitempty"`
	Referrals  int    `json:"referrals,omitempty"`
	RefDay     string `json:"ref_day,omitempty"`
	RefToday   int    `json:"ref_today,omitempty"`

	// Lifetime totals for the profile.
	Readings    int `json:"readings,omitempty"`
	SpokenChars int `json:"spoken_chars,omitempty"`

	// TipDay is the last UTC day a language tip was shown (one a day at most).
	TipDay string `json:"tip_day,omitempty"`

	// Ban, set from the admin panel, keeps the user out of Ellie. Kept
	// across /reset.
	Ban *Ban `json:"ban,omitempty"`

	// NoNews turns off the admins' announcements (the button under each).
	NoNews bool `json:"no_news,omitempty"`
}

// Ban is an admin's decision to keep a user out.
type Ban struct {
	Until    *time.Time `json:"until,omitempty"` // nil: until an admin lifts it
	Reason   string     `json:"reason,omitempty"`
	By       int64      `json:"by,omitempty"` // the admin
	At       time.Time  `json:"at"`
	Notified bool       `json:"notified,omitempty"` // the user was told once
}

// Active reports whether the ban is in force at now.
func (b *Ban) Active(now time.Time) bool {
	return b != nil && (b.Until == nil || now.Before(*b.Until))
}

// Usage is what a user has spent today, and their bonus characters left.
type Usage struct {
	Chars, Secs, Makes int
	Bonus              int
	// Studio is set on a refusal caused by the studio-wide daily cap
	// rather than the user's own allowance; StudioUnknown when that cap
	// couldn't be checked at all (the database is unreachable).
	Studio, StudioUnknown bool
}

// usageToday returns v's usage for day, treating an older day as zero.
func usageToday(v Settings, day string) Usage {
	if v.UsageDay != day {
		return Usage{Bonus: v.Bonus}
	}
	return Usage{Chars: v.UsedChars, Secs: v.UsedSecs, Makes: v.UsedMakes, Bonus: v.Bonus}
}

// overLimit is how many of used characters lie past the daily limit: the
// part paid from bonus characters. A limit of 0 means unlimited.
func overLimit(used, limit int) int {
	if limit <= 0 {
		return 0
	}
	return max(0, used-limit)
}

// ---------------------------------------------------------------- the store

// Store keeps per-user Settings.
//
// With a file (or nothing) behind it, every user lives in memory and the
// file is rewritten after each change. With Postgres behind it, the database
// is the source of truth: each change is a short transaction that locks only
// the users it touches (so users never wait for each other, and two
// instances running side by side during a deploy can't overwrite each
// other), and reads come from the database. The memory copy then only
// stands in while the database is unreachable.
type Store struct {
	mu       sync.Mutex
	backend  persister // nil = memory only
	shared   sharedStore
	defaults Settings
	data     map[int64]Settings
}

// sharedStore is a backend that owns the data (Postgres).
type sharedStore interface {
	// mutate runs fn on the stored settings of uids (users never stored are
	// absent) while holding a lock on just those users, then saves what fn
	// changed and returns it.
	mutate(uids []int64, fn func(*userTx)) (map[int64]Settings, error)
	// load returns uid's settings, and false if they were never stored.
	load(uid int64) (Settings, bool, error)
	// forget clears the field pair idKey/nameKey from every user whose idKey
	// is id, returning them.
	forget(idKey, nameKey, id string) ([]int64, error)
	// banned returns every user with a ban on record.
	banned() (map[int64]Ban, error)
}

// cacheLimit bounds the memory copy kept next to a shared store; past it
// the copy starts over (it is only a fallback).
const cacheLimit = 100_000

// userTx is the view a change works on: the users it asked for, and which
// of them it changed.
type userTx struct {
	cur   map[int64]Settings
	known map[int64]bool
	dirty map[int64]bool
}

func newUserTx() *userTx {
	return &userTx{cur: map[int64]Settings{}, known: map[int64]bool{}, dirty: map[int64]bool{}}
}

// get returns uid's settings, and false if they were never stored.
func (t *userTx) get(uid int64) (Settings, bool) { return t.cur[uid], t.known[uid] }

// put replaces uid's settings.
func (t *userTx) put(uid int64, v Settings) {
	t.cur[uid], t.known[uid], t.dirty[uid] = v, true, true
}

// OpenStore loads path if it exists. An empty path keeps settings in memory.
func OpenStore(path string, defaults Settings) (*Store, error) {
	s := &Store{defaults: defaults, data: map[int64]Settings{}}
	if path == "" {
		return s, nil
	}
	s.backend = fileStore{path: path}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("settings: %w", err)
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("settings: parse %s: %w", path, err)
	}
	return s, nil
}

// Close releases the backend (the database pool, if any).
func (s *Store) Close() error {
	if s.backend == nil {
		return nil
	}
	return s.backend.close()
}

// mutate applies fn to uids' settings and persists what it changed. If the
// shared store is unreachable it carries on from the memory copy (so the bot
// keeps working) and returns the error.
func (s *Store) mutate(uids []int64, fn func(*userTx)) error {
	if s.shared != nil {
		saved, err := s.shared.mutate(uids, fn)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.applyLocked(uids, fn)
			return err
		}
		for uid, v := range saved {
			s.cacheLocked(uid, v, true)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.applyLocked(uids, fn)
	if len(changed) == 0 || s.backend == nil {
		return nil
	}
	return s.backend.save(s.data, changed)
}

// applyLocked runs fn against the memory copy. Callers hold s.mu.
func (s *Store) applyLocked(uids []int64, fn func(*userTx)) []int64 {
	t := newUserTx()
	for _, uid := range uids {
		if v, ok := s.data[uid]; ok {
			t.cur[uid], t.known[uid] = v, true
		}
	}
	fn(t)
	var changed []int64
	for uid := range t.dirty {
		s.data[uid] = t.cur[uid]
		changed = append(changed, uid)
	}
	return changed
}

// cacheLocked records what the shared store holds for uid. Callers hold s.mu.
func (s *Store) cacheLocked(uid int64, v Settings, ok bool) {
	if !ok {
		delete(s.data, uid)
		return
	}
	if _, have := s.data[uid]; !have && len(s.data) >= cacheLimit {
		s.data = map[int64]Settings{}
	}
	s.data[uid] = v
}

// change is mutate for one user: fn edits v (known says whether the user was
// ever stored) and returns whether to save it.
func (s *Store) change(uid int64, fn func(v *Settings, known bool) bool) error {
	return s.mutate([]int64{uid}, func(t *userTx) {
		v, ok := t.get(uid)
		if fn(&v, ok) {
			t.put(uid, v)
		}
	})
}

// lookup returns uid's stored settings, and false if there are none.
func (s *Store) lookup(uid int64) (Settings, bool) {
	if s.shared != nil {
		if v, ok, err := s.shared.load(uid); err == nil {
			s.mu.Lock()
			s.cacheLocked(uid, v, ok)
			s.mu.Unlock()
			return v, ok
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[uid]
	return v, ok
}

// ---------------------------------------------------------------- allowance

// Spend atomically adds chars and secs to a user's usage for day, unless that
// would pass a limit (a limit of 0 means unlimited). Characters past the
// daily limit are paid from bonus characters while they last. It returns
// whether the spend happened and the usage afterwards (or as it stands, if
// refused).
func (s *Store) Spend(uid int64, day string, chars, secs, charLimit, secLimit int) (bool, Usage, error) {
	var spent bool
	var u Usage
	err := s.change(uid, func(v *Settings, _ bool) bool {
		u, spent = usageToday(*v, day), false
		fromBonus := 0
		if chars > 0 {
			fromBonus = overLimit(u.Chars+chars, charLimit) - overLimit(u.Chars, charLimit)
		}
		if fromBonus > v.Bonus || (secLimit > 0 && secs > 0 && u.Secs+secs > secLimit) {
			return false
		}
		u.Chars += chars
		u.Secs += secs
		if v.UsageDay != day {
			v.UsedMakes, v.UsedDicts = 0, 0
		}
		v.Bonus -= fromBonus
		u.Bonus = v.Bonus
		v.UsageDay, v.UsedChars, v.UsedSecs = day, u.Chars, u.Secs
		spent = true
		return true
	})
	return spent, u, err
}

// Refund gives back an earlier Spend (for example when generation failed),
// returning bonus characters that Spend took. charLimit must be the limit
// the Spend used.
func (s *Store) Refund(uid int64, day string, chars, secs, charLimit int) {
	_ = s.change(uid, func(v *Settings, known bool) bool {
		if !known || v.UsageDay != day {
			return false
		}
		after := max(0, v.UsedChars-chars)
		v.Bonus += overLimit(v.UsedChars, charLimit) - overLimit(after, charLimit)
		v.UsedChars = after
		v.UsedSecs = max(0, v.UsedSecs-secs)
		return true
	})
}

// Spoke adds a finished reading to the user's lifetime totals.
func (s *Store) Spoke(uid int64, chars int) {
	_ = s.change(uid, func(v *Settings, _ bool) bool {
		v.Readings++
		v.SpokenChars += chars
		return true
	})
}

// MarkTip records that a tip was shown on day; false means one already was.
func (s *Store) MarkTip(uid int64, day string) bool {
	var marked bool
	_ = s.change(uid, func(v *Settings, _ bool) bool {
		marked = v.TipDay != day
		v.TipDay = day
		return marked
	})
	return marked
}

// Referral records that invitee joined through inviter's link, giving the
// invitee bonus characters and the inviter the same, up to dailyCap rewards
// per UTC day. It does nothing (false, false) unless invitee is new to the
// studio, inviter has joined it, and they differ. rewarded reports whether
// the inviter got the bonus.
func (s *Store) Referral(inviter, invitee int64, day string, bonus, dailyCap int) (applied, rewarded bool, err error) {
	err = s.mutate([]int64{inviter, invitee}, func(t *userTx) {
		applied, rewarded = false, false
		from, ok := t.get(inviter)
		if _, seen := t.get(invitee); seen || !ok || !from.Joined || inviter == invitee || bonus <= 0 {
			return
		}
		if from.RefDay != day {
			from.RefDay, from.RefToday = day, 0
		}
		from.Referrals++
		if dailyCap <= 0 || from.RefToday < dailyCap {
			from.RefToday++
			from.Bonus += bonus
			rewarded = true
		}
		t.put(inviter, from)
		t.put(invitee, Settings{ReferredBy: inviter, Bonus: bonus})
		applied = true
	})
	return applied, rewarded, err
}

// SpendMake records one new voice for day unless that would pass limit
// (0 = unlimited). It returns whether it was recorded and today's usage.
func (s *Store) SpendMake(uid int64, day string, limit int) (bool, Usage, error) {
	var spent bool
	var u Usage
	err := s.change(uid, func(v *Settings, _ bool) bool {
		u, spent = usageToday(*v, day), false
		if limit > 0 && u.Makes+1 > limit {
			return false
		}
		u.Makes++
		if v.UsageDay != day {
			v.UsedDicts = 0
		}
		v.UsageDay, v.UsedChars, v.UsedSecs, v.UsedMakes = day, u.Chars, u.Secs, u.Makes
		spent = true
		return true
	})
	return spent, u, err
}

// SpendDict records one new dictionary for day unless that would pass limit
// (0 = unlimited). It returns whether it was recorded and today's count.
func (s *Store) SpendDict(uid int64, day string, limit int) (bool, int, error) {
	var spent bool
	var n int
	err := s.change(uid, func(v *Settings, _ bool) bool {
		spent = false
		rolled := v.UsageDay != day
		if rolled {
			v.UsageDay, v.UsedChars, v.UsedSecs, v.UsedMakes, v.UsedDicts = day, 0, 0, 0, 0
		}
		n = v.UsedDicts
		if limit > 0 && v.UsedDicts+1 > limit {
			return rolled
		}
		v.UsedDicts++
		n, spent = v.UsedDicts, true
		return true
	})
	return spent, n, err
}

// RefundDict gives back a SpendDict (the creation failed).
func (s *Store) RefundDict(uid int64, day string) {
	_ = s.change(uid, func(v *Settings, known bool) bool {
		if !known || v.UsageDay != day || v.UsedDicts == 0 {
			return false
		}
		v.UsedDicts--
		return true
	})
}

// RefundMake gives back a SpendMake (the creation failed).
func (s *Store) RefundMake(uid int64, day string) {
	_ = s.change(uid, func(v *Settings, known bool) bool {
		if !known || v.UsageDay != day || v.UsedMakes == 0 {
			return false
		}
		v.UsedMakes--
		return true
	})
}

// UsageOn reports a user's usage for day.
func (s *Store) UsageOn(uid int64, day string) Usage {
	v, _ := s.lookup(uid)
	return usageToday(v, day)
}

// ---------------------------------------------------------------- preferences

// Defaults returns the studio-wide defaults.
func (s *Store) Defaults() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.defaults
}

// SetDefaultVoice changes the fallback voice for users who never chose one.
func (s *Store) SetDefaultVoice(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaults.VoiceID, s.defaults.VoiceName = id, name
}

// Known reports whether the user has used the studio before.
func (s *Store) Known(uid int64) bool {
	_, ok := s.lookup(uid)
	return ok
}

// Get returns a user's settings with defaults filled in.
func (s *Store) Get(uid int64) Settings {
	v, _ := s.lookup(uid)
	return s.withDefaults(v)
}

func (s *Store) withDefaults(v Settings) Settings {
	d := s.Defaults()
	if v.VoiceID == "" {
		v.VoiceID, v.VoiceName = d.VoiceID, d.VoiceName
	}
	if v.Model == "" {
		v.Model = d.Model
	}
	if v.Speed == 0 {
		v.Speed = 1
	}
	if v.Volume == 0 {
		v.Volume = 1
	}
	return v
}

// Update applies fn to a user's stored settings and persists the result.
func (s *Store) Update(uid int64, fn func(*Settings)) (Settings, error) {
	var out Settings
	err := s.change(uid, func(v *Settings, _ bool) bool {
		fn(v)
		out = *v
		return true
	})
	return s.withDefaults(out), err
}

// Reset forgets a user's settings but keeps today's usage, so resetting can't
// be used to refill a daily allowance.
func (s *Store) Reset(uid int64) error {
	return s.change(uid, func(v *Settings, known bool) bool {
		if !known {
			return false
		}
		// Only the voice and style preferences go; allowance, bonus, referral
		// and lifetime bookkeeping stay.
		v.VoiceID, v.VoiceName, v.Model, v.Language = "", "", "", ""
		v.Emotion, v.Speed, v.Volume = "", 0, 0
		v.DictID, v.DictName, v.AsFile = "", "", false
		return true
	})
}

// Join marks uid as a user of the private chat as of day. It writes only the
// first time.
func (s *Store) Join(uid int64, day string) error {
	return s.change(uid, func(v *Settings, _ bool) bool {
		if v.Joined {
			return false
		}
		v.Joined, v.JoinedAt = true, day
		return true
	})
}

// ForgetVoice clears a voice from every user who selected it (after delete).
func (s *Store) ForgetVoice(id string) error { return s.forget(voiceRef, id) }

// ForgetDict clears a pronunciation dictionary from every user who chose it.
func (s *Store) ForgetDict(id string) error { return s.forget(dictRef, id) }

// ref names a settings field pair (an ID and its display name) that points
// at a Cartesia resource.
type ref struct {
	idKey, nameKey string // JSON keys, for the database
	id             func(*Settings) *string
	name           func(*Settings) *string
}

var (
	voiceRef = ref{"voice_id", "voice_name", func(v *Settings) *string { return &v.VoiceID }, func(v *Settings) *string { return &v.VoiceName }}
	dictRef  = ref{"dict_id", "dict_name", func(v *Settings) *string { return &v.DictID }, func(v *Settings) *string { return &v.DictName }}
)

func (s *Store) forget(r ref, id string) error {
	if s.shared != nil {
		uids, err := s.shared.forget(r.idKey, r.nameKey, id)
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, uid := range uids {
			delete(s.data, uid) // reloaded on next read
		}
		if err != nil {
			s.forgetLocked(r, id)
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.forgetLocked(r, id)
	if len(changed) == 0 || s.backend == nil {
		return nil
	}
	return s.backend.save(s.data, changed)
}

func (s *Store) forgetLocked(r ref, id string) []int64 {
	var changed []int64
	for uid, v := range s.data {
		if *r.id(&v) == id {
			*r.id(&v), *r.name(&v) = "", ""
			s.data[uid] = v
			changed = append(changed, uid)
		}
	}
	return changed
}

// Banned returns every user with a ban on record (active or expired).
func (s *Store) Banned() (map[int64]Ban, error) {
	if s.shared != nil {
		return s.shared.banned()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[int64]Ban{}
	for uid, v := range s.data {
		if v.Ban != nil {
			out[uid] = *v.Ban
		}
	}
	return out, nil
}

// fileStore keeps every user in one JSON file.
type fileStore struct{ path string }

func (fileStore) close() error { return nil }

func (f fileStore) save(all map[int64]Settings, _ []int64) error {
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(f.path, raw)
}

// writeFileAtomic replaces path with raw (temp file + rename) so a crash never
// leaves a half-written file, and keeps it readable by the bot only.
func writeFileAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("save %s: %w", filepath.Base(path), err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("save %s: %w", filepath.Base(path), err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("save %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("save %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("save %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save %s: %w", filepath.Base(path), err)
	}
	return os.Rename(tmp.Name(), path)
}
