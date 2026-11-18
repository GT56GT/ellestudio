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
	v.UsageDay, v.UsedChars, v.UsedSecs, v.UsedMakes = day, u.Chars, u.Secs, u.Makes
	s.data[uid] = v
	return true, u, s.saveLocked()
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
	_ = s.saveLocked()
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
	v.UsageDay, v.UsedChars, v.UsedSecs, v.UsedMakes = day, u.Chars, u.Secs, u.Makes
	s.data[uid] = v
	return true, u, s.saveLocked()
}

// RefundMake gives back a SpendMake (the creation failed).
func (s *Store) RefundMake(uid int64, day string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.data[uid]; ok && v.UsageDay == day && v.UsedMakes > 0 {
		v.UsedMakes--
		s.data[uid] = v
		_ = s.saveLocked()
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
	path     string
	defaults Settings
	data     map[int64]Settings
}

// OpenStore loads path if it exists. An empty path keeps settings in memory.
func OpenStore(path string, defaults Settings) (*Store, error) {
	s := &Store{path: path, defaults: defaults, data: map[int64]Settings{}}
	if path == "" {
		return s, nil
	}
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
	return s.withDefaults(v), s.saveLocked()
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
	s.data[uid] = Settings{UsageDay: old.UsageDay, UsedChars: old.UsedChars, UsedSecs: old.UsedSecs, UsedMakes: old.UsedMakes}
	return s.saveLocked()
}

// ForgetVoice clears a voice from every user who selected it (after delete).
func (s *Store) ForgetVoice(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for uid, v := range s.data {
		if v.VoiceID == id {
			v.VoiceID, v.VoiceName = "", ""
			s.data[uid] = v
		}
	}
	return s.saveLocked()
}

// saveLocked writes atomically (temp file + rename) with owner-only perms.
func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.json")
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return os.Rename(tmp.Name(), s.path)
}
