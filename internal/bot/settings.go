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

// Reset forgets a user's settings.
func (s *Store) Reset(uid int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, uid)
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
