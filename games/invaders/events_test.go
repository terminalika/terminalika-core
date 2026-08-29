package invaders

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

func eventTypes(events []core.Event) []string {
	types := make([]string, len(events))
	for i, ev := range events {
		types[i] = ev.Type
	}
	return types
}

func TestMoveCommandEmitsPlayerMoved(t *testing.T) {
	g, r := newEventGame()

	err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{DX: 1})})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evPlayerMoved {
		t.Fatalf("events = %v, want single %s", eventTypes(r.events), evPlayerMoved)
	}

	var p playerMovedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.X != boardColumns/2+1 {
		t.Fatalf("player x = %d, want %d", p.X, boardColumns/2+1)
	}
}

func TestFireCommandEmitsShotFired(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdFire}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evShotFired {
		t.Fatalf("events = %v, want single %s", eventTypes(r.events), evShotFired)
	}

	var p shotPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.X != g.player || p.Y != playerRow-1 {
		t.Fatalf("shot = %+v, want (%d,%d)", p, g.player, playerRow-1)
	}
}

func TestFireCommandIsRejectedWhileReloading(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdFire}); err != nil {
		t.Fatalf("first fire: %v", err)
	}
	if err := g.HandleCommand(core.Command{Type: cmdFire}); err == nil {
		t.Fatal("expected error while reloading")
	}
}

func TestTickCommandEmitsAliensMoved(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdTick}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evAliensMoved {
		t.Fatalf("events = %v, want single %s", eventTypes(r.events), evAliensMoved)
	}

	var p aliensMovedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.DX != 1 || p.DY != 0 {
		t.Fatalf("aliens moved = %+v, want (1,0)", p)
	}
}

func TestDestroyingAlienEmitsAlienHit(t *testing.T) {
	g, r := newEventGame()
	clearAliens(g)
	g.aliens[0][0] = true
	g.aliens[1][1] = true
	g.alive = 2

	target := g.alienCell(slot{row: 1, col: 1})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}

	g.stepBullets()

	if len(r.events) != 1 || r.events[0].Type != evAlienHit {
		t.Fatalf("events = %v, want single %s", eventTypes(r.events), evAlienHit)
	}

	var p alienHitPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.X != target.X || p.Y != target.Y || p.Score != rowScores[1] || p.Alive != 1 {
		t.Fatalf("alien hit = %+v, want x=%d y=%d score=%d alive=1", p, target.X, target.Y, rowScores[1])
	}
}

func TestClearingWaveEmitsWaveCleared(t *testing.T) {
	g, r := newEventGame()
	clearAliens(g)
	g.aliens[0][0] = true
	g.alive = 1

	target := g.alienCell(slot{row: 0, col: 0})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}

	g.stepBullets()

	got := eventTypes(r.events)
	if len(got) != 2 || got[0] != evAlienHit || got[1] != evWaveCleared {
		t.Fatalf("events = %v, want [%s %s]", got, evAlienHit, evWaveCleared)
	}

	var p waveClearedPayload
	if err := json.Unmarshal(r.events[1].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Wave != 1 || p.Score != rowScores[0] {
		t.Fatalf("wave cleared = %+v, want wave=1 score=%d", p, rowScores[0])
	}
}

func TestCannonHitEmitsPlayerHitThenGameOver(t *testing.T) {
	g, r := newEventGame()
	g.lives = 1
	g.bullets = []bullet{{pos: Point{X: g.player, Y: playerRow - 1}, dy: 1}}

	g.stepBullets()

	got := eventTypes(r.events)
	if len(got) != 2 || got[0] != evPlayerHit || got[1] != evGameOver {
		t.Fatalf("events = %v, want [%s %s]", got, evPlayerHit, evGameOver)
	}

	var over gameOverPayload
	if err := json.Unmarshal(r.events[1].Payload, &over); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if over.Reason != "destroyed" {
		t.Fatalf("game over reason = %q, want destroyed", over.Reason)
	}
}

func TestInvasionEmitsGameOver(t *testing.T) {
	g, r := newEventGame()
	g.alienDir = 1
	_, maxX, maxY := g.formationBounds()
	g.formation.Y += playerRow - 1 - maxY
	g.formation.X += boardColumns - 1 - maxX

	g.stepAliens()

	got := eventTypes(r.events)
	if len(got) != 2 || got[0] != evAliensMoved || got[1] != evGameOver {
		t.Fatalf("events = %v, want [%s %s]", got, evAliensMoved, evGameOver)
	}

	var over gameOverPayload
	if err := json.Unmarshal(r.events[1].Payload, &over); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if over.Reason != "invaded" {
		t.Fatalf("game over reason = %q, want invaded", over.Reason)
	}
}

func TestAlienFireEmitsAlienFired(t *testing.T) {
	g, r := newEventGame()
	clearAliens(g)
	g.aliens[2][4] = true
	g.alive = 1

	g.alienFire()

	if len(r.events) != 1 || r.events[0].Type != evAlienFired {
		t.Fatalf("events = %v, want single %s", eventTypes(r.events), evAlienFired)
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

	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{DX: 3})}); err == nil {
		t.Fatal("expected error for out-of-range move")
	}
}

func TestCommandsRejectedWhilePaused(t *testing.T) {
	g, _ := newEventGame()
	g.Pause()

	for _, typ := range []string{cmdFire, cmdTick} {
		if err := g.HandleCommand(core.Command{Type: typ}); err == nil {
			t.Fatalf("%s: expected error while paused", typ)
		}
	}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{DX: 1})}); err == nil {
		t.Fatal("move: expected error while paused")
	}
}

func TestCommandsListsSupportedCommands(t *testing.T) {
	g, _ := newEventGame()

	names := make(map[string]bool)
	for _, spec := range g.Commands() {
		names[spec.Name] = true
	}

	for _, want := range []string{cmdMove, cmdFire, cmdTick, cmdPause, cmdResume, cmdReset} {
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

	got := eventTypes(r.events)
	if len(got) != 3 || got[0] != evPaused || got[1] != evResumed || got[2] != evReset {
		t.Fatalf("events = %v, want [%s %s %s]", got, evPaused, evResumed, evReset)
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
		t.Fatalf("events = %v, want single %s", eventTypes(r.events), evPaused)
	}

	var p pausedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Reason != "Paused by PI" {
		t.Fatalf("reason = %q, want %q", p.Reason, "Paused by PI")
	}
}
