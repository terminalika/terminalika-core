// Package highscore persists best scores for terminalika games.
package highscore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store keeps best scores in memory and optionally persists them to a JSON
// file. A Store with an empty path works purely in memory.
type Store struct {
	mu     sync.Mutex
	path   string
	scores map[string]int
}

// NewInMemory returns a Store that never touches the filesystem.
func NewInMemory() *Store {
	return &Store{scores: make(map[string]int)}
}

// Open loads the store from path. A missing or empty file starts as an empty
// store.
func Open(path string) (*Store, error) {
	s := &Store{path: path, scores: make(map[string]int)}
	if path == "" {
		return s, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.scores); err != nil {
		return nil, err
	}
	if s.scores == nil {
		s.scores = make(map[string]int)
	}
	return s, nil
}

// Best returns the best score recorded for name.
func (s *Store) Best(name string) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scores[name]
}

// Submit records score when it beats the current best and persists the store.
// It reports whether a new best score was set.
func (s *Store) Submit(name string, score int) bool {
	if s == nil || score <= 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if score <= s.scores[name] {
		return false
	}
	s.scores[name] = score
	_ = s.save()
	return true
}

func (s *Store) save() error {
	if s.path == "" {
		return nil
	}

	data, err := json.MarshalIndent(s.scores, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// DefaultPath returns the default location of the scores file.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		if home, herr := os.UserHomeDir(); herr == nil {
			dir = filepath.Join(home, ".config")
		} else {
			dir = "."
		}
	}
	return filepath.Join(dir, "terminalika", "scores.json")
}
