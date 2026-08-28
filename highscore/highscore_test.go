package highscore

import (
	"path/filepath"
	"testing"
)

func TestInMemorySubmitAndBest(t *testing.T) {
	s := NewInMemory()

	if got := s.Best("snake"); got != 0 {
		t.Fatalf("Best = %d, want 0", got)
	}

	if !s.Submit("snake", 100) {
		t.Fatal("first submit should be a new best")
	}
	if s.Submit("snake", 50) {
		t.Fatal("lower score must not be a new best")
	}
	if got := s.Best("snake"); got != 100 {
		t.Fatalf("Best = %d, want 100", got)
	}

	// Independent games keep independent scores.
	if !s.Submit("tetris", 300) {
		t.Fatal("tetris submit should be a new best")
	}
	if got := s.Best("tetris"); got != 300 {
		t.Fatalf("tetris Best = %d, want 300", got)
	}
}

func TestOpenMissingFileStartsEmpty(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := s.Best("snake"); got != 0 {
		t.Fatalf("Best = %d, want 0", got)
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scores.json")

	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !s.Submit("snake", 250) {
		t.Fatal("submit should be a new best")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.Best("snake"); got != 250 {
		t.Fatalf("reopened Best = %d, want 250", got)
	}
}

func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	if got := s.Best("snake"); got != 0 {
		t.Fatalf("nil Best = %d, want 0", got)
	}
	if s.Submit("snake", 10) {
		t.Fatal("nil Submit must not report a new best")
	}
}
