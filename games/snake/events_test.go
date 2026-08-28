package snake

import (
	"encoding/json"
	"testing"

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

func TestSetDirectionCommandEmitsEvent(t *testing.T) {
	g, r := newEventGame()

	err := g.HandleCommand(core.Command{
		ID:      "c1",
		Type:    cmdSetDirection,
		Payload: core.MustJSON(setDirectionPayload{Direction: "up"}),
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 {
		t.Fatalf("events = %d, want 1", len(r.events))
	}
	if r.events[0].Type != evDirectionChanged {
		t.Fatalf("event type = %q, want %q", r.events[0].Type, evDirectionChanged)
	}

	var p directionChanged
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.From != "right" || p.To != "up" {
		t.Fatalf("direction changed = %+v, want from right to up", p)
	}
}

func TestStepCommandEmitsMoved(t *testing.T) {
	g, r := newEventGame()
	g.snake = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirRight
	g.turns = nil
	g.hasFood = false

	if err := g.HandleCommand(core.Command{Type: cmdStep}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evMoved {
		t.Fatalf("events = %+v, want single %s", r.events, evMoved)
	}

	var p movedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Head != (cellPos{X: 6, Y: 5}) {
		t.Fatalf("head = %+v, want (6,5)", p.Head)
	}
}

func TestEatingFoodEmitsFoodEaten(t *testing.T) {
	g, r := newEventGame()
	g.snake = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirRight
	g.turns = nil
	g.food = Point{X: 6, Y: 5}
	g.hasFood = true

	if err := g.HandleCommand(core.Command{Type: cmdStep}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evFoodEaten {
		t.Fatalf("events = %+v, want single %s", r.events, evFoodEaten)
	}

	var p foodEatenPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.X != 6 || p.Y != 5 || p.Score != scorePerFood || p.Level != 1 {
		t.Fatalf("payload = %+v, want x=6 y=5 score=%d level=1", p, scorePerFood)
	}
}

func TestCollisionEmitsCollisionAndGameOver(t *testing.T) {
	g, r := newEventGame()
	g.snake = []Point{{X: 0, Y: 5}, {X: 1, Y: 5}, {X: 2, Y: 5}}
	g.dir = dirLeft
	g.turns = nil
	g.hasFood = false

	if err := g.HandleCommand(core.Command{Type: cmdStep}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 2 {
		t.Fatalf("events = %d, want 2", len(r.events))
	}
	if r.events[0].Type != evCollision || r.events[1].Type != evGameOver {
		t.Fatalf("events = %q, %q; want %s then %s", r.events[0].Type, r.events[1].Type, evCollision, evGameOver)
	}

	var c collisionPayload
	if err := json.Unmarshal(r.events[0].Payload, &c); err != nil || c.Kind != "wall" {
		t.Fatalf("collision payload = %+v (err %v), want kind wall", c, err)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: "nope"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestInvalidDirectionReturnsError(t *testing.T) {
	g, _ := newEventGame()

	err := g.HandleCommand(core.Command{
		Type:    cmdSetDirection,
		Payload: core.MustJSON(setDirectionPayload{Direction: "sideways"}),
	})
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

	for _, want := range []string{cmdSetDirection, cmdStep, cmdPause, cmdResume, cmdReset} {
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

	if len(r.events) != 3 {
		t.Fatalf("events = %d, want 3 (paused, resumed, reset)", len(r.events))
	}
	if r.events[0].Type != evPaused || r.events[1].Type != evResumed || r.events[2].Type != evReset {
		t.Fatalf("events = %q, %q, %q", r.events[0].Type, r.events[1].Type, r.events[2].Type)
	}
}

func TestPauseCommandCarriesReason(t *testing.T) {
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
		t.Fatalf("events = %+v, want single %s", r.events, evPaused)
	}

	var p pausedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Reason != "Paused by PI" {
		t.Fatalf("reason = %q, want %q", p.Reason, "Paused by PI")
	}
}
