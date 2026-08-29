package pong

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

func (r *recorder) types() []string {
	out := make([]string, len(r.events))
	for i, ev := range r.events {
		out[i] = ev.Type
	}
	return out
}

func TestConfigureCommandEmitsConfigured(t *testing.T) {
	g, r := newEventGame()

	err := g.HandleCommand(core.Command{
		ID:      "c1",
		Type:    cmdConfigure,
		Payload: core.MustJSON(configurePayload{Mode: "hard", Target: 11}),
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if g.mode != modeHard || g.target != 11 {
		t.Fatalf("settings = %s first to %d, want HARD BOT first to 11", modeNames[g.mode], g.target)
	}

	if len(r.events) != 1 || r.events[0].Type != evConfigured {
		t.Fatalf("events = %v, want single %s", r.types(), evConfigured)
	}
	var p setupPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Mode != "hard" || p.Target != 11 {
		t.Fatalf("payload = %+v, want mode hard target 11", p)
	}
}

func TestConfigureRejectsBadSettingsAndRunningMatches(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdConfigure, Payload: core.MustJSON(configurePayload{Mode: "impossible", Target: 5})}); err == nil {
		t.Fatal("expected error for an unknown mode")
	}
	if err := g.HandleCommand(core.Command{Type: cmdConfigure, Payload: core.MustJSON(configurePayload{Mode: "easy", Target: 0})}); err == nil {
		t.Fatal("expected error for a zero target")
	}

	startRally(g, modeTwoPlayers, 5)
	if err := g.HandleCommand(core.Command{Type: cmdConfigure, Payload: core.MustJSON(configurePayload{Mode: "easy", Target: 5})}); err == nil {
		t.Fatal("expected error while a match is in progress")
	}
}

func TestStartCommandEmitsMatchStarted(t *testing.T) {
	g, r := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdStart}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(r.events) != 1 || r.events[0].Type != evMatchStarted {
		t.Fatalf("events = %v, want single %s", r.types(), evMatchStarted)
	}
	if g.phase != phaseServing {
		t.Fatalf("phase = %v, want serving", g.phase)
	}

	if err := g.HandleCommand(core.Command{Type: cmdStart}); err == nil {
		t.Fatal("expected error when starting during a match")
	}
}

func TestTickServesThenAdvancesTheBall(t *testing.T) {
	g, r := newEventGame()
	g.mode = modeTwoPlayers
	g.startMatch()
	r.events = nil

	if err := g.HandleCommand(core.Command{Type: cmdTick}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(r.events) != 1 || r.events[0].Type != evServe {
		t.Fatalf("events = %v, want single %s", r.types(), evServe)
	}
	var p servePayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.To != 1 && p.To != 2 {
		t.Fatalf("serve to = %d, want 1 or 2", p.To)
	}

	g.ball = Point{X: 10, Y: 5}
	g.vel = Point{X: 1, Y: 1}
	if err := g.HandleCommand(core.Command{Type: cmdTick}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if g.ball != (Point{X: 11, Y: 6}) {
		t.Fatalf("ball = %+v, want (11,6)", g.ball)
	}
}

func TestMoveCommandEmitsPaddleMoved(t *testing.T) {
	g, r := newEventGame()
	startRally(g, modeTwoPlayers, 5)
	r.events = nil
	top := g.paddles[1]

	err := g.HandleCommand(core.Command{
		Type:    cmdMove,
		Payload: core.MustJSON(movePayload{Player: 2, DY: -1}),
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	if len(r.events) != 1 || r.events[0].Type != evPaddleMoved {
		t.Fatalf("events = %v, want single %s", r.types(), evPaddleMoved)
	}
	var p paddleMovedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Player != 2 || p.Y != top-1 {
		t.Fatalf("payload = %+v, want player 2 at row %d", p, top-1)
	}
}

func TestMoveCommandRejectsTheBotPaddleAndBadMoves(t *testing.T) {
	g, _ := newEventGame()
	startRally(g, modeNormal, 5)

	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Player: 2, DY: 1})}); err == nil {
		t.Fatal("expected error when moving the bot's paddle")
	}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Player: 3, DY: 1})}); err == nil {
		t.Fatal("expected error for an invalid player")
	}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Player: 1, DY: 2})}); err == nil {
		t.Fatal("expected error for an invalid step")
	}
	g.paddles[0] = 0
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Player: 1, DY: -1})}); err == nil {
		t.Fatal("expected error when the paddle is against the wall")
	}
}

func TestBouncesEmitBallBounced(t *testing.T) {
	g, r := newEventGame()
	startRally(g, modeTwoPlayers, 5)
	r.events = nil

	g.ball = Point{X: 10, Y: 0}
	g.vel = Point{X: 1, Y: -1}
	g.stepBall()

	g.paddles[1] = 6
	g.ball = Point{X: rightPaddleX - 1, Y: 7}
	g.vel = Point{X: 1, Y: 0}
	g.stepBall()

	if len(r.events) != 2 || r.events[0].Type != evBallBounced || r.events[1].Type != evBallBounced {
		t.Fatalf("events = %v, want two %s", r.types(), evBallBounced)
	}
	var wall, paddle ballBouncedPayload
	if err := json.Unmarshal(r.events[0].Payload, &wall); err != nil || wall.Kind != "wall" || wall.Player != 0 {
		t.Fatalf("wall bounce = %+v (err %v)", wall, err)
	}
	if wall.Ball != (cellPos{X: 11, Y: 1}) || wall.Velocity != (cellPos{X: 1, Y: 1}) {
		t.Fatalf("wall bounce ball/velocity = %+v/%+v, want (11,1)/(1,1)", wall.Ball, wall.Velocity)
	}
	if err := json.Unmarshal(r.events[1].Payload, &paddle); err != nil || paddle.Kind != "paddle" || paddle.Player != 2 {
		t.Fatalf("paddle bounce = %+v (err %v), want kind paddle by player 2", paddle, err)
	}
	if paddle.Velocity != (cellPos{X: -1, Y: 0}) {
		t.Fatalf("paddle bounce velocity = %+v, want (-1,0)", paddle.Velocity)
	}
}

func TestPointAndGameOverEvents(t *testing.T) {
	g, r := newEventGame()
	startRally(g, modeEasy, 2)
	g.points = [2]int{1, 0}
	g.paddles[1] = 0
	r.events = nil

	g.ball = Point{X: boardColumns - 1, Y: 7}
	g.vel = Point{X: 1, Y: 0}
	g.stepBall()

	if len(r.events) != 2 || r.events[0].Type != evPointScored || r.events[1].Type != evGameOver {
		t.Fatalf("events = %v, want %s then %s", r.types(), evPointScored, evGameOver)
	}
	var p pointScoredPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Player != 1 || p.P1 != 2 || p.P2 != 0 || p.Score != pointPoints*profiles[modeEasy].mult {
		t.Fatalf("point scored = %+v, want player 1, 2:0, score %d", p, pointPoints*profiles[modeEasy].mult)
	}
	var over gameOverPayload
	if err := json.Unmarshal(r.events[1].Payload, &over); err != nil {
		t.Fatalf("payload: %v", err)
	}
	want := (pointPoints + winPoints) * profiles[modeEasy].mult
	if over.Winner != 1 || over.P1 != 2 || over.Score != want || over.Best != want || over.Reason != "win" {
		t.Fatalf("game over = %+v, want winner 1 with score/best %d", over, want)
	}
}

func TestPointScoredCentresAndSchedulesTheServe(t *testing.T) {
	g, r := newEventGame()
	startRally(g, modeTwoPlayers, 5)
	r.events = nil

	g.paddles[0] = 0
	g.ball = Point{X: 0, Y: 7}
	g.vel = Point{X: -1, Y: 0}
	g.stepBall()

	if len(r.events) != 1 || r.events[0].Type != evPointScored {
		t.Fatalf("events = %v, want single %s", r.types(), evPointScored)
	}
	if g.phase != phaseServing {
		t.Fatalf("phase = %v, want serving", g.phase)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: "nope"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestCommandsRejectedOutsideAMatch(t *testing.T) {
	g, _ := newEventGame()

	if err := g.HandleCommand(core.Command{Type: cmdTick}); err == nil {
		t.Fatal("expected error ticking on the setup screen")
	}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{Player: 1, DY: 1})}); err == nil {
		t.Fatal("expected error moving on the setup screen")
	}
}

func TestCommandsListsSupportedCommands(t *testing.T) {
	g, _ := newEventGame()

	names := make(map[string]bool)
	for _, spec := range g.Commands() {
		names[spec.Name] = true
	}

	for _, want := range []string{cmdConfigure, cmdStart, cmdMove, cmdTick, cmdPause, cmdResume, cmdReset} {
		if !names[want] {
			t.Fatalf("Commands() missing %q", want)
		}
	}
}

func TestLifecycleCommandsEmitEvents(t *testing.T) {
	g, r := newEventGame()
	startRally(g, modeTwoPlayers, 5)
	r.events = nil // discard setup-time events

	g.Pause()
	g.Resume()
	g.Reset()

	want := []string{evPaused, evResumed, evReset, evMatchStarted}
	got := r.types()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}

func TestResetOnSetupEmitsOnlyReset(t *testing.T) {
	g, r := newEventGame()
	r.events = nil

	g.Reset()

	if len(r.events) != 1 || r.events[0].Type != evReset {
		t.Fatalf("events = %v, want single %s", r.types(), evReset)
	}
}

func TestPauseCommandCarriesReason(t *testing.T) {
	g, r := newEventGame()
	startRally(g, modeTwoPlayers, 5)
	r.events = nil

	err := g.HandleCommand(core.Command{
		Type:    cmdPause,
		Payload: core.MustJSON(pausedPayload{Reason: "Paused by PI"}),
	})
	if err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	if len(r.events) != 1 || r.events[0].Type != evPaused {
		t.Fatalf("events = %v, want single %s", r.types(), evPaused)
	}

	var p pausedPayload
	if err := json.Unmarshal(r.events[0].Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.Reason != "Paused by PI" {
		t.Fatalf("reason = %q, want %q", p.Reason, "Paused by PI")
	}
}
