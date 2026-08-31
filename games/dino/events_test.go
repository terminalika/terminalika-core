package dino

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

func TestJumpCommandEmitsJumpedThenLanded(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{ID: "c1", Type: cmdJump}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(r.events) != 1 || r.events[0].Type != evJumped {
		t.Fatalf("events = %v, want single %s", types(r.events), evJumped)
	}

	if err := g.HandleCommand(core.Command{Type: cmdJump}); err == nil {
		t.Fatal("a jump in the air should be rejected")
	}

	r.events = nil
	for g.airborne {
		if err := g.HandleCommand(core.Command{Type: cmdStep}); err != nil {
			t.Fatalf("HandleCommand: %v", err)
		}
	}
	if len(r.events) != 1 || r.events[0].Type != evLanded {
		t.Fatalf("events = %v, want single %s", types(r.events), evLanded)
	}
}

func TestClearingAnObstacleEmitsObstacleCleared(t *testing.T) {
	g, r := newEventGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: cactusPair, x: dinoX - 1}}

	if err := g.HandleCommand(core.Command{Type: cmdStep}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evObstacleCleared {
		t.Fatalf("events = %v, want single %s", types(r.events), evObstacleCleared)
	}
	var p clearedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Kind != "cactus_pair" || p.Score != 1 {
		t.Fatalf("payload = %+v, want cactus_pair at score 1", p)
	}

	// Once cleared, an obstacle is never reported again.
	g.HandleCommand(core.Command{Type: cmdStep})
	if len(r.events) != 1 {
		t.Fatalf("events = %v, want no second clear", types(r.events))
	}
}

func TestMilestoneEmitsEveryHundred(t *testing.T) {
	g, r := newEventGame()
	runTicks(g, milestoneEvery*2)

	var milestones []int
	for _, ev := range r.events {
		if ev.Type != evMilestone {
			continue
		}
		var p milestonePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		milestones = append(milestones, p.Score)
	}
	if len(milestones) != 2 || milestones[0] != milestoneEvery || milestones[1] != 2*milestoneEvery {
		t.Fatalf("milestones = %v, want [%d %d]", milestones, milestoneEvery, 2*milestoneEvery)
	}
}

func TestCollisionEmitsCollisionAndGameOver(t *testing.T) {
	g, r := newEventGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: cactusTall, x: dinoX + dinoWidth}}

	if err := g.HandleCommand(core.Command{Type: cmdStep}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 2 || r.events[0].Type != evCollision || r.events[1].Type != evGameOver {
		t.Fatalf("events = %v; want %s then %s", types(r.events), evCollision, evGameOver)
	}
	var c collisionPayload
	if err := json.Unmarshal(r.events[0].Payload, &c); err != nil || c.Kind != "tall_cactus" {
		t.Fatalf("collision payload = %+v (err %v), want kind tall_cactus", c, err)
	}
	var over gameOverPayload
	if err := json.Unmarshal(r.events[1].Payload, &over); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if over.Score != 1 || over.Best != 1 || over.Reason != "tall_cactus" {
		t.Fatalf("game over payload = %+v", over)
	}

	if err := g.HandleCommand(core.Command{Type: cmdStep}); err == nil {
		t.Fatal("stepping a finished run should be rejected")
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

	for _, want := range []string{cmdJump, cmdStep, cmdPause, cmdResume, cmdReset} {
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
		t.Fatalf("events = %v, want paused, resumed, reset", types(r.events))
	}
	if r.events[0].Type != evPaused || r.events[1].Type != evResumed || r.events[2].Type != evReset {
		t.Fatalf("events = %v", types(r.events))
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
		t.Fatalf("events = %v, want single %s", types(r.events), evPaused)
	}

	var p pausedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Reason != "Paused by PI" {
		t.Fatalf("reason = %q, want %q", p.Reason, "Paused by PI")
	}
}

func TestPausedRunIgnoresJumpsAndSteps(t *testing.T) {
	g, _ := newEventGame()
	g.Pause()

	if err := g.HandleCommand(core.Command{Type: cmdJump}); err == nil {
		t.Fatal("jump while paused should be rejected")
	}
	if err := g.HandleCommand(core.Command{Type: cmdStep}); err == nil {
		t.Fatal("step while paused should be rejected")
	}
}
