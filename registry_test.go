package core

import "testing"

func TestRegistryRegisterGetAndNames(t *testing.T) {
	r := NewRegistry()

	if r.Has("snake") {
		t.Fatal("empty registry should not have snake")
	}

	r.Register("snake", func() Game { return nil })
	r.Register("tetris", func() Game { return nil })

	if !r.Has("snake") || !r.Has("tetris") {
		t.Fatal("registered games should be found")
	}

	game, ok := r.Get("snake")
	if !ok || game != nil {
		t.Fatal("expected to get the registered snake factory result")
	}

	names := r.Names()
	if len(names) != 2 || names[0] != "snake" || names[1] != "tetris" {
		t.Fatalf("unexpected names: %v", names)
	}
}
