package pong

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestHeldKeyMovesThePaddleContinuously(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	top := g.paddles[0]
	w := key(tcell.KeyRune, 'w')

	if !g.HandleKeyState(w, true) || g.paddles[0] != top-1 {
		t.Fatalf("press: paddle = %d, want %d (one row at once)", g.paddles[0], top-1)
	}
	if g.HandleKeyState(w, true); g.paddles[0] != top-1 {
		t.Fatalf("repeat of a held key must not move the paddle again, got %d", g.paddles[0])
	}

	g.Update()
	if g.paddles[0] != top-1 {
		t.Fatalf("paddle moved before the first delay: %d", g.paddles[0])
	}

	g.paddleNext[0] = time.Now().Add(-paddlePeriod)
	g.Update()
	if g.paddles[0] != top-3 {
		t.Fatalf("paddle = %d, want %d (two overdue steps caught up)", g.paddles[0], top-3)
	}

	if !g.HandleKeyState(w, false) || g.held[0] != 0 {
		t.Fatal("release should stop the paddle")
	}
	g.paddleNext[0] = time.Now().Add(-time.Second)
	g.Update()
	if g.paddles[0] != top-3 {
		t.Fatalf("paddle moved after release: %d", g.paddles[0])
	}
}

func TestReleaseOfAnotherDirectionDoesNotStopThePaddle(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)

	g.HandleKeyState(key(tcell.KeyRune, 's'), true)
	g.HandleKeyState(key(tcell.KeyRune, 'w'), true) // switch direction while s is held
	g.HandleKeyState(key(tcell.KeyRune, 's'), false)
	if g.held[0] != -1 {
		t.Fatalf("held = %d, want the paddle still driven up by w", g.held[0])
	}
}

func TestReversingAMovingPaddleSkipsTheFirstDelay(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)

	g.HandleKeyState(key(tcell.KeyRune, 's'), true) // start moving down
	row := g.paddles[0]

	g.HandleKeyState(key(tcell.KeyRune, 'w'), true) // reverse to up mid-move
	if g.paddles[0] != row-1 {
		t.Fatalf("paddle = %d, want %d (reversal still moves one row at once)", g.paddles[0], row-1)
	}
	if until := time.Until(g.paddleNext[0]); until > paddlePeriod {
		t.Fatalf("next step in %v, want at most paddlePeriod (%v) after a reversal", until, paddlePeriod)
	}
}

func TestFreshPressFromAStoppedPaddleKeepsTheFullDelay(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)

	g.HandleKeyState(key(tcell.KeyRune, 'w'), true) // first press, paddle was stopped
	if until := time.Until(g.paddleNext[0]); until <= paddlePeriod {
		t.Fatalf("next step in %v, want the full paddleFirstDelay (%v) from a stop", until, paddleFirstDelay)
	}
}

func TestSameDirectionResumeWithinWindowDoesNotBonusStep(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	w := key(tcell.KeyRune, 'w')
	row := g.paddles[0]

	g.HandleKeyState(w, true) // fresh press: one bonus step
	if g.paddles[0] != row-1 {
		t.Fatalf("paddle = %d, want %d after the fresh press", g.paddles[0], row-1)
	}
	next := g.paddleNext[0]

	// Simulate the host engine's own release-detection jitter: it
	// synthesised a release (clearing held) and then the terminal's next
	// real auto-repeat arrives as an ordinary press for the same direction.
	g.HandleKeyState(w, false)
	g.HandleKeyState(w, true)

	if g.paddles[0] != row-1 {
		t.Fatalf("paddle = %d, want %d: a same-direction resume must not bonus-step", g.paddles[0], row-1)
	}
	if g.held[0] != -1 {
		t.Fatalf("held = %d, want the paddle still driven up", g.held[0])
	}
	if !g.paddleNext[0].Equal(next) {
		t.Fatalf("paddleNext = %v, want unchanged (%v): a resume must not reset the step timer", g.paddleNext[0], next)
	}
}

func TestSameDirectionPressAfterResumeWindowIsAFreshStart(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	w := key(tcell.KeyRune, 'w')
	row := g.paddles[0]

	g.HandleKeyState(w, true)
	g.HandleKeyState(w, false)
	g.lastDirAt[0] = time.Now().Add(-2 * paddleResumeWindow) // well outside the resume window

	g.HandleKeyState(w, true)
	if g.paddles[0] != row-2 {
		t.Fatalf("paddle = %d, want %d: a press outside the resume window is a fresh start (bonus step)", g.paddles[0], row-2)
	}
}

func TestArrowsHoldTheHumanPaddleAgainstABot(t *testing.T) {
	g := newTestGame()
	startRally(g, modeEasy, 5)

	g.HandleKeyState(key(tcell.KeyDown, 0), true)
	if g.held[0] != 1 || g.held[1] != 0 {
		t.Fatalf("held = %v, want the left (human) paddle driven down", g.held)
	}
}

func TestKeyStateIsIgnoredOutsideARally(t *testing.T) {
	g := newTestGame()
	if g.HandleKeyState(key(tcell.KeyRune, 'w'), true) {
		t.Fatal("setup screen must not claim paddle keys")
	}

	startRally(g, modeTwoPlayers, 5)
	g.HandleKeyState(key(tcell.KeyRune, 'w'), true)
	g.Pause()
	if g.held[0] != 0 {
		t.Fatal("pausing should drop held keys")
	}
	if g.HandleKeyState(key(tcell.KeyRune, 'w'), true) {
		t.Fatal("paused game must not claim paddle keys")
	}
	g.Resume()

	g.HandleKeyState(key(tcell.KeyRune, 'w'), true)
	g.Reset()
	if g.held[0] != 0 {
		t.Fatal("reset should drop held keys")
	}
}

func TestBallTicksOnAFixedStep(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	g.ball = Point{X: 10, Y: 5}
	g.vel = Point{X: 1, Y: 1}
	g.period = 50 * time.Millisecond

	// Two and a half periods overdue: exactly two ticks, and the clock keeps
	// the half period instead of snapping to now.
	start := time.Now().Add(-125 * time.Millisecond)
	g.lastTick = start
	g.Update()
	if g.ball != (Point{X: 12, Y: 7}) {
		t.Fatalf("ball = %+v, want two ticks (12,7)", g.ball)
	}
	if !g.lastTick.Equal(start.Add(100 * time.Millisecond)) {
		t.Fatalf("lastTick advanced by %v, want exactly two periods", g.lastTick.Sub(start))
	}

	// A long stall is skipped, not fast-forwarded.
	g.lastTick = time.Now().Add(-time.Hour)
	g.Update()
	if g.ball != (Point{X: 12 + maxCatchUp, Y: 7 + maxCatchUp}) {
		t.Fatalf("ball = %+v, want at most %d catch-up ticks", g.ball, maxCatchUp)
	}
	if time.Since(g.lastTick) > g.period {
		t.Fatal("clock should resync after a stall")
	}
}
