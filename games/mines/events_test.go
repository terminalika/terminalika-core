package mines

import (
	"encoding/json"
	"testing"

	core "github.com/terminalika/terminalika-core"
)

type recorder struct {
	events []core.Event
}

func (r *recorder) Emit(ev core.Event) { r.events = append(r.events, ev) }

func newEventGame() (*Game, *recorder) {
	g := newTestGame()
	layout(g, corner)
	r := &recorder{}
	g.SetEmitter(r)
	return g, r
}

func types(evs []core.Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Type
	}
	return out
}

func at(x, y int) json.RawMessage {
	return core.MustJSON(map[string]int{"x": x, "y": y})
}

func TestRevealCommandEmitsRevealed(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{ID: "c1", Type: cmdReveal, Payload: at(0, 0)}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evRevealed {
		t.Fatalf("events = %v, want single %s", types(r.events), evRevealed)
	}
	var p revealedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.X != 0 || p.Y != 0 || p.Cells != g.revealedCount || p.Score != g.score {
		t.Fatalf("payload = %+v, want the flood's size and score", p)
	}

	if err := g.HandleCommand(core.Command{Type: cmdReveal, Payload: at(0, 0)}); err == nil {
		t.Fatal("revealing an open, numberless cell again should be rejected")
	}
}

func TestRevealCommandDefaultsToTheCursor(t *testing.T) {
	g, r := newEventGame()
	g.cx, g.cy = 0, 0

	if err := g.HandleCommand(core.Command{Type: cmdReveal}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(r.events) != 1 || !g.cells[0][0].revealed {
		t.Fatalf("events = %v, revealed=%v; want the cursor's cell opened", types(r.events), g.cells[0][0].revealed)
	}
}

func TestFlagCommandEmitsFlagged(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdFlag, Payload: at(6, 3)}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	var p flaggedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil || r.events[0].Type != evFlagged {
		t.Fatalf("event = %s (err %v), want %s", r.events[0].Type, err, evFlagged)
	}
	if !p.Flagged || p.Flags != 1 || p.X != 6 || p.Y != 3 {
		t.Fatalf("payload = %+v", p)
	}
}

func TestHitEmitsExplodedThenGameOver(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdReveal, Payload: at(6, 3)}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 2 || r.events[0].Type != evExploded || r.events[1].Type != evGameOver {
		t.Fatalf("events = %v, want exploded then game over", types(r.events))
	}
	var p gameOverPayload
	if err := json.Unmarshal(r.events[1].Payload, &p); err != nil || p.Won {
		t.Fatalf("game over payload = %+v (err %v), want a loss", p, err)
	}
	if err := g.HandleCommand(core.Command{Type: cmdReveal, Payload: at(0, 0)}); err == nil {
		t.Fatal("commands after the end should be rejected")
	}
}

func TestClearEmitsClearedThenGameOver(t *testing.T) {
	g, r := newEventGame()
	for y := 0; y < g.level.Rows; y++ {
		for x := 0; x < g.level.Cols; x++ {
			if !g.cells[y][x].mine && !g.cells[y][x].revealed {
				g.reveal(x, y)
			}
		}
	}

	n := len(r.events)
	if n < 3 || r.events[n-2].Type != evCleared || r.events[n-1].Type != evGameOver {
		t.Fatalf("events end with %v, want cleared then game over", types(r.events[max(0, n-3):]))
	}
	var p gameOverPayload
	if err := json.Unmarshal(r.events[n-1].Payload, &p); err != nil || !p.Won {
		t.Fatalf("game over payload = %+v (err %v), want a win", p, err)
	}
}

func TestCursorCommands(t *testing.T) {
	g, _ := newEventGame()
	g.cx, g.cy = 0, 0

	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Direction: "left"})}); err == nil {
		t.Fatal("moving off the field should be rejected")
	}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Direction: "right"})}); err != nil || g.cx != 1 {
		t.Fatalf("move right: err=%v cx=%d", err, g.cx)
	}
	if err := g.HandleCommand(core.Command{Type: cmdCursor, Payload: at(8, 8)}); err != nil || g.cx != 8 || g.cy != 8 {
		t.Fatalf("cursor: err=%v at %d,%d", err, g.cx, g.cy)
	}
	if err := g.HandleCommand(core.Command{Type: cmdCursor, Payload: at(9, 0)}); err == nil {
		t.Fatal("a cursor off the field should be rejected")
	}
	if err := g.HandleCommand(core.Command{Type: cmdCursor}); err == nil {
		t.Fatal("cursor needs x and y")
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: "nope"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestCommandsListsSupportedCommands(t *testing.T) {
	g, _ := newEventGame()

	names := make(map[string]bool)
	for _, spec := range g.Commands() {
		names[spec.Name] = true
	}
	for _, want := range []string{cmdMove, cmdCursor, cmdReveal, cmdFlag, cmdLevel, cmdPause, cmdResume, cmdReset} {
		if !names[want] {
			t.Fatalf("Commands() missing %q", want)
		}
	}
}

func TestLifecycleCommandsEmitEvents(t *testing.T) {
	g, r := newEventGame()
	r.events = nil // discard construction-time events

	g.Pause()
	g.Resume()
	g.Reset()

	if len(r.events) != 3 || r.events[0].Type != evPaused || r.events[1].Type != evResumed || r.events[2].Type != evReset {
		t.Fatalf("events = %v, want paused, resumed, reset", types(r.events))
	}
}

func TestPauseCommandCarriesReasonAndBlocksInput(t *testing.T) {
	g, r := newEventGame()
	r.events = nil

	err := g.HandleCommand(core.Command{
		Type:    cmdPause,
		Payload: core.MustJSON(pausedPayload{Reason: "Paused by PI"}),
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(r.events) != 1 || r.events[0].Type != evPaused {
		t.Fatalf("events = %v, want single %s", types(r.events), evPaused)
	}
	var p pausedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil || p.Reason != "Paused by PI" {
		t.Fatalf("reason = %q (err %v)", p.Reason, err)
	}
	if err := g.HandleCommand(core.Command{Type: cmdReveal, Payload: at(0, 0)}); err == nil {
		t.Fatal("a reveal while paused should be rejected")
	}
}
