package tetris

import (
	"encoding/json"
	"testing"

	"github.com/gdamore/tcell/v2"

	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/highscore"
)

type recorder struct {
	events []core.Event
}

func (r *recorder) Emit(ev core.Event) { r.events = append(r.events, ev) }

func newEventGame() (*Game, *recorder) {
	g := NewWithStore(highscore.NewInMemory())
	r := &recorder{}
	g.SetEmitter(r)
	return g, r
}

func TestMoveCommandEmitsPieceMoved(t *testing.T) {
	g, r := newEventGame()

	err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{DX: 1, DY: 0})})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evPieceMoved {
		t.Fatalf("events = %+v, want single %s", r.events, evPieceMoved)
	}

	var p pieceMoved
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.DX != 1 || p.DY != 0 {
		t.Fatalf("move = (%d,%d), want (1,0)", p.DX, p.DY)
	}
}

func TestRotateCommandEmitsPieceRotated(t *testing.T) {
	g, r := newEventGame()

	err := g.HandleCommand(core.Command{Type: cmdRotate})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evPieceRotated {
		t.Fatalf("events = %+v, want single %s", r.events, evPieceRotated)
	}
}

func TestHardDropEmitsPieceLocked(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdHardDrop}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	found := false
	for _, ev := range r.events {
		if ev.Type == evPieceLocked {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %+v, want to include %s", r.events, evPieceLocked)
	}
}

func TestLineClearEmitsLineCleared(t *testing.T) {
	g, r := newEventGame()
	g.current = nil

	for x := 0; x < boardColumns; x++ {
		g.board[boardRows-1][x] = cell{filled: true, color: tcell.ColorWhite}
	}

	g.clearLines()

	if len(r.events) != 1 || r.events[0].Type != evLineCleared {
		t.Fatalf("events = %+v, want single %s", r.events, evLineCleared)
	}

	var p lineCleared
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Count != 1 || p.Points != 100 {
		t.Fatalf("line cleared = %+v, want count=1 points=100", p)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: "nope"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestInvalidMoveReturnsError(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{DX: 5, DY: 5})}); err == nil {
		t.Fatal("expected error for out-of-range move")
	}
}

func TestCommandsListsSupportedCommands(t *testing.T) {
	g, _ := newEventGame()

	names := make(map[string]bool)
	for _, spec := range g.Commands() {
		names[spec.Name] = true
	}

	for _, want := range []string{cmdMove, cmdRotate, cmdHardDrop, cmdTick, cmdPause, cmdResume, cmdReset} {
		if !names[want] {
			t.Fatalf("Commands() missing %q", want)
		}
	}
}

func TestLifecycleCommandsEmitEvents(t *testing.T) {
	g, r := newEventGame()
	r.events = nil

	g.Pause()
	g.Resume()
	g.Reset()

	// Reset also emits a piece_spawned event; keep only the lifecycle ones.
	var got []string
	for _, ev := range r.events {
		switch ev.Type {
		case evPaused, evResumed, evReset:
			got = append(got, ev.Type)
		}
	}

	want := []string{evPaused, evResumed, evReset}
	if len(got) != len(want) {
		t.Fatalf("lifecycle events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lifecycle events = %v, want %v", got, want)
		}
	}
}
