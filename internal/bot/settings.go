package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// persister is where a Store keeps its data between restarts: a JSON file
// (the default) or Postgres (when DATABASE_URL is set). The Store keeps every
// user in memory and calls save with the users that just changed.
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
	// Kept across /reset.
	Joined bool `json:"joined,omitempty"`
}

// Usage is what a user has spent today.
type Usage struct {
	Chars, Secs, Makes int
}

// usageToday returns v's usage for day, treating an older day as zero.
func usageToday(v Settings, day string) Usage {
	if v.UsageDay != day {
		return Usage{}
	}
	return Usage{v.UsedChars, v.UsedSecs, v.UsedMakes}
}

// Spend atomically adds chars and secs to a user's usage for day, unless that
// would pass a limit (a limit of 0 means unlimited). It returns whether the
// spend happened and the usage afterwards (or as it stands, if refused).
func (s *Store) Spend(uid int64, day string, chars, secs, charLimit, secLimit int) (bool, Usage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[uid]
	u := usageToday(v, day)
	if (charLimit > 0 && chars > 0 && u.Chars+chars > charLimit) ||
		(secLimit > 0 && secs > 0 && u.Secs+secs > secLimit) {
		return false, u, nil
	}
	u.Chars += chars
	u.Secs += secs
	if v.UsageDay != day {
		v.UsedMakes, v.UsedDicts = 0, 0
	}
	v.UsageDay, v.UsedChars, v.UsedSecs = day, u.Chars, u.Secs
	s.data[uid] = v
	return true, u, s.saveLocked(uid)
}

// Refund gives back an earlier Spend (for example when generation failed).
func (s *Store) Refund(uid int64, day string, chars, secs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[uid]
	if !ok || v.UsageDay != day {
		return
	}
	v.UsedChars = max(0, v.UsedChars-chars)
	v.UsedSecs = max(0, v.UsedSecs-secs)
	s.data[uid] = v
	_ = s.saveLocked(uid)
}

// SpendMake records one new voice for day unless that would pass limit
// (0 = unlimited). It returns whether it was recorded and today's usage.
func (s *Store) SpendMake(uid int64, day string, limit int) (bool, Usage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[uid]
	u := usageToday(v, day)
	if limit > 0 && u.Makes+1 > limit {
		return false, u, nil
	}
	u.Makes++
	if v.UsageDay != day {
		v.UsedDicts = 0
	}
	v.UsageDay, v.UsedChars, v.UsedSecs, v.UsedMakes = day, u.Chars, u.Secs, u.Makes
	s.data[uid] = v
	return true, u, s.saveLocked(uid)
}

// SpendDict records one new dictionary for day unless that would pass limit
// (0 = unlimited). It returns whether it was recorded and today's count.
func (s *Store) SpendDict(uid int64, day string, limit int) (bool, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[uid]
	if v.UsageDay != day {
		v.UsageDay, v.UsedChars, v.UsedSecs, v.UsedMakes, v.UsedDicts = day, 0, 0, 0, 0
	}
	if limit > 0 && v.UsedDicts+1 > limit {
		return false, v.UsedDicts, nil
	}
	v.UsedDicts++
	s.data[uid] = v
	return true, v.UsedDicts, s.saveLocked(uid)
}

// RefundDict gives back a SpendDict (the creation failed).
func (s *Store) RefundDict(uid int64, day string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.data[uid]; ok && v.UsageDay == day && v.UsedDicts > 0 {
		v.UsedDicts--
		s.data[uid] = v
		_ = s.saveLocked(uid)
	}
}

// RefundMake gives back a SpendMake (the creation failed).
func (s *Store) RefundMake(uid int64, day string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.data[uid]; ok && v.UsageDay == day && v.UsedMakes > 0 {
		v.UsedMakes--
		s.data[uid] = v
		_ = s.saveLocked(uid)
	}
}

// UsageOn reports a user's usage for day.
func (s *Store) UsageOn(uid int64, day string) Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return usageToday(s.data[uid], day)
}

// Store keeps per-user Settings in memory and persists them as JSON.
type Store struct {
	mu       sync.Mutex
	backend  persister // nil = memory only
	defaults Settings
	data     map[int64]Settings
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
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data[uid]
	return ok
}

// Get returns a user's settings with defaults filled in.
func (s *Store) Get(uid int64) Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withDefaults(s.data[uid])
}

func (s *Store) withDefaults(v Settings) Settings {
	if v.VoiceID == "" {
		v.VoiceID, v.VoiceName = s.defaults.VoiceID, s.defaults.VoiceName
	}
	if v.Model == "" {
		v.Model = s.defaults.Model
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
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[uid]
	fn(&v)
	s.data[uid] = v
	return s.withDefaults(v), s.saveLocked(uid)
}

// Reset forgets a user's settings but keeps today's usage, so resetting can't
// be used to refill a daily allowance.
func (s *Store) Reset(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.data[uid]
	if !ok {
		return nil
	}
	s.data[uid] = Settings{UsageDay: old.UsageDay, UsedChars: old.UsedChars, UsedSecs: old.UsedSecs,
		UsedMakes: old.UsedMakes, UsedDicts: old.UsedDicts, Joined: old.Joined}
	return s.saveLocked(uid)
}

// Join marks uid as a user of the private chat. It writes only the first time.
func (s *Store) Join(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.data[uid]
	if v.Joined {
		return nil
	}
	v.Joined = true
	s.data[uid] = v
	return s.saveLocked(uid)
}

// ForgetVoice clears a voice from every user who selected it (after delete).
func (s *Store) ForgetVoice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var changed []int64
	for uid, v := range s.data {
		if v.VoiceID == id {
			v.VoiceID, v.VoiceName = "", ""
			s.data[uid] = v
			changed = append(changed, uid)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return s.saveLocked(changed...)
}

// saveLocked persists the users in changed. Callers hold s.mu.
func (s *Store) saveLocked(changed ...int64) error {
	if s.backend == nil {
		return nil
	}
	return s.backend.save(s.data, changed)
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
