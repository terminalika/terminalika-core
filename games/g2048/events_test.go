package g2048

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

func TestMoveCommandEmitsMoved(t *testing.T) {
	g, r := newEventGame()
	g.board = [size][size]int{{2, 2, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}

	err := g.HandleCommand(core.Command{
		ID:      "c1",
		Type:    cmdMove,
		Payload: core.MustJSON(movePayload{Direction: "left"}),
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evMoved {
		t.Fatalf("events = %v, want single %s", types(r.events), evMoved)
	}
	var p movedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Direction != "left" || p.Gained != 4 || p.Score != 4 {
		t.Fatalf("payload = %+v, want left, gained 4, score 4", p)
	}
	if p.Spawned.Value != 2 && p.Spawned.Value != 4 {
		t.Fatalf("spawned = %+v, want a 2 or a 4", p.Spawned)
	}
	if g.board[p.Spawned.Y][p.Spawned.X] != p.Spawned.Value {
		t.Fatalf("spawned %+v is not on the board", p.Spawned)
	}
}

func TestRejectedMoveReturnsErrorAndEmitsNothing(t *testing.T) {
	g, r := newEventGame()
	g.board = [size][size]int{{2, 4, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}

	err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Direction: "left"})})
	if err == nil {
		t.Fatal("a slide that changes nothing should be rejected")
	}
	if len(r.events) != 0 {
		t.Fatalf("events = %v, want none", types(r.events))
	}
}

func TestWinAndGameOverEvents(t *testing.T) {
	g, r := newEventGame()
	g.board = [size][size]int{{1024, 1024, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}
	g.move(dirLeft)

	if len(r.events) != 2 || r.events[0].Type != evMoved || r.events[1].Type != evWon {
		t.Fatalf("events = %v, want moved then won", types(r.events))
	}

	r.events = nil
	g.board = stuckAfterLeft
	g.board[0][0] = 2048 // keep the win on the board for the payload
	g.move(dirLeft)

	if len(r.events) != 2 || r.events[0].Type != evMoved || r.events[1].Type != evGameOver {
		t.Fatalf("events = %v, want moved then game over", types(r.events))
	}
	var p gameOverPayload
	if err := json.Unmarshal(r.events[1].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if !p.Won || p.Largest != 2048 {
		t.Fatalf("game over payload = %+v, want won with largest 2048", p)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: "nope"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
	err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Direction: "sideways"})})
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
}

func TestCommandsListsSupportedCommands(t *testing.T) {
	g, _ := newEventGame()

	names := make(map[string]bool)
	for _, spec := range g.Commands() {
		names[spec.Name] = true
	}
	for _, want := range []string{cmdMove, cmdPause, cmdResume, cmdReset} {
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

func TestPauseCommandCarriesReasonAndBlocksMoves(t *testing.T) {
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
		t.Fatalf("reason = %q (err %v), want %q", p.Reason, err, "Paused by PI")
	}

	g.board = [size][size]int{{2, 2, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Direction: "left"})}); err == nil {
		t.Fatal("a move while paused should be rejected")
	}
}
