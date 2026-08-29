package pong

import (
	"math/rand"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/terminalika/terminalika-core/highscore"
)

// newTestGame returns a game on its setup screen with a fixed random seed.
func newTestGame() *Game {
	g := NewWithStore(highscore.NewInMemory())
	g.rng = rand.New(rand.NewSource(1))
	return g
}

// startRally sets the game up in mode m (first to target) and serves the
// ball so a rally is in progress. Callers then place the ball and paddles.
func startRally(g *Game, m mode, target int) {
	g.mode = m
	g.target = target
	g.startMatch()
	g.serve()
}

func key(k tcell.Key, r rune) *tcell.EventKey {
	return tcell.NewEventKey(k, r, tcell.ModNone)
}

func TestNewGameOpensSetup(t *testing.T) {
	g := newTestGame()
	if g.phase != phaseSetup {
		t.Fatalf("phase = %v, want setup", g.phase)
	}
	if g.configured {
		t.Fatal("a fresh game must not count as configured")
	}
	if g.mode != modeNormal || g.target != 5 {
		t.Fatalf("defaults = %s first to %d, want NORMAL BOT first to 5", modeNames[g.mode], g.target)
	}
}

func TestSetupKeysChooseModeAndTargetThenStart(t *testing.T) {
	g := newTestGame()

	g.HandleInput(key(tcell.KeyRight, 0))
	if g.mode != modeHard {
		t.Fatalf("mode after Right = %s, want HARD BOT", modeNames[g.mode])
	}
	g.HandleInput(key(tcell.KeyRight, 0))
	if g.mode != modeTwoPlayers {
		t.Fatalf("mode should wrap around to 2 PLAYERS, got %s", modeNames[g.mode])
	}

	g.HandleInput(key(tcell.KeyDown, 0))
	if g.setupRow != setupTarget {
		t.Fatalf("setupRow = %d, want target row", g.setupRow)
	}
	g.HandleInput(key(tcell.KeyLeft, 0))
	if g.target != 3 {
		t.Fatalf("target after Left = %d, want 3", g.target)
	}
	g.HandleInput(key(tcell.KeyLeft, 0))
	if g.target != 21 {
		t.Fatalf("target should wrap around to 21, got %d", g.target)
	}

	if !g.HandleInput(key(tcell.KeyEnter, 0)) {
		t.Fatal("Enter should be consumed on the setup screen")
	}
	if g.phase != phaseServing || !g.configured {
		t.Fatalf("after Enter phase = %v configured = %v, want serving and configured", g.phase, g.configured)
	}
}

func TestUpdateServesOnceTheBreatherIsOver(t *testing.T) {
	g := newTestGame()
	g.mode = modeTwoPlayers
	g.startMatch()

	g.Update()
	if g.phase != phaseServing {
		t.Fatal("the ball must not be served before the breather is over")
	}

	g.serveAt = time.Now().Add(-time.Millisecond)
	g.Update()
	if g.phase != phasePlaying {
		t.Fatalf("phase = %v, want playing after the breather", g.phase)
	}
	if g.ball.X != boardColumns/2 || g.vel.X == 0 || g.vel.Y == 0 {
		t.Fatalf("serve: ball %+v vel %+v, want a diagonal from the centre line", g.ball, g.vel)
	}
}

func TestBallReflectsOffWalls(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)

	g.ball = Point{X: 10, Y: 0}
	g.vel = Point{X: 1, Y: -1}
	g.stepBall()
	if g.ball != (Point{X: 11, Y: 1}) || g.vel != (Point{X: 1, Y: 1}) {
		t.Fatalf("after top wall: ball %+v vel %+v, want (11,1) going down", g.ball, g.vel)
	}

	g.ball = Point{X: 10, Y: boardRows - 1}
	g.vel = Point{X: -1, Y: 1}
	g.stepBall()
	if g.ball != (Point{X: 9, Y: boardRows - 2}) || g.vel != (Point{X: -1, Y: -1}) {
		t.Fatalf("after bottom wall: ball %+v vel %+v, want (9,%d) going up", g.ball, g.vel, boardRows-2)
	}
}

func TestPaddleHitAnglesTheBall(t *testing.T) {
	cases := []struct {
		name string
		from Point
		vel  Point
		want Point
	}{
		{"top third sends it up", Point{X: 2, Y: 7}, Point{X: -1, Y: -1}, Point{X: 1, Y: -1}},
		{"bottom third sends it down", Point{X: 2, Y: 7}, Point{X: -1, Y: 1}, Point{X: 1, Y: 1}},
		{"middle keeps the angle", Point{X: 2, Y: 8}, Point{X: -1, Y: -1}, Point{X: 1, Y: -1}},
		{"middle keeps a downward angle", Point{X: 2, Y: 6}, Point{X: -1, Y: 1}, Point{X: 1, Y: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGame()
			startRally(g, modeTwoPlayers, 5)
			g.paddles[0] = 6 // rows 6, 7, 8
			g.ball = tc.from
			g.vel = tc.vel

			g.stepBall()

			if g.vel != tc.want {
				t.Fatalf("vel = %+v, want %+v", g.vel, tc.want)
			}
			if g.ball != tc.from {
				t.Fatalf("ball = %+v, want it to hold at %+v for the contact tick", g.ball, tc.from)
			}
		})
	}
}

func TestRightPaddleHitsToo(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	g.paddles[1] = 6
	g.ball = Point{X: rightPaddleX - 1, Y: 7}
	g.vel = Point{X: 1, Y: 1}

	g.stepBall()

	if g.vel != (Point{X: -1, Y: 1}) {
		t.Fatalf("vel = %+v, want the ball sent back down-left", g.vel)
	}
}

func TestRallySpeedsUpWithEveryHitDownToTheCap(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	base := profiles[modeTwoPlayers].period
	if g.period != base {
		t.Fatalf("period at serve = %v, want %v", g.period, base)
	}

	g.hitPaddle(0, g.paddles[0]+1)
	if g.period != base-profiles[modeTwoPlayers].accel {
		t.Fatalf("period after a hit = %v, want %v", g.period, base-profiles[modeTwoPlayers].accel)
	}

	for i := 0; i < 100; i++ {
		g.hitPaddle(0, g.paddles[0]+1)
	}
	if g.period != minBallPeriod {
		t.Fatalf("period after many hits = %v, want the cap %v", g.period, minBallPeriod)
	}
}

func TestMissingThePaddleScoresForTheOpponent(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	g.paddles[0] = 6
	g.paddles[1] = 2
	g.period = minBallPeriod
	g.ball = Point{X: 1, Y: 1}
	g.vel = Point{X: -1, Y: -1}

	g.stepBall() // slips behind the paddle to column 0
	if g.ball != (Point{X: 0, Y: 0}) || g.points != [2]int{} {
		t.Fatalf("ball %+v points %v, want the ball at (0,0) and no point yet", g.ball, g.points)
	}

	g.stepBall() // leaves the court
	if g.points != [2]int{0, 1} {
		t.Fatalf("points = %v, want player 2 to score", g.points)
	}
	if g.phase != phaseServing || g.serveTo != 0 {
		t.Fatalf("phase %v serveTo %d, want a serve towards player 1 (who conceded)", g.phase, g.serveTo)
	}
	if g.ball != (Point{X: boardColumns / 2, Y: boardRows / 2}) || g.vel != (Point{}) {
		t.Fatalf("ball %+v vel %+v, want it parked in the centre", g.ball, g.vel)
	}
	top := (boardRows - paddleHeight) / 2
	if g.paddles != [2]int{top, top} {
		t.Fatalf("paddles = %v, want both recentred to %d", g.paddles, top)
	}
	if g.period != profiles[modeTwoPlayers].period {
		t.Fatalf("period = %v, want the rally speed reset to %v", g.period, profiles[modeTwoPlayers].period)
	}

	g.serve()
	if g.vel.X != -1 {
		t.Fatalf("serve vel = %+v, want it travelling towards player 1", g.vel)
	}
}

func TestReachingTheTargetEndsTheMatch(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 2)
	g.points = [2]int{1, 0}
	g.ball = Point{X: boardColumns - 1, Y: 7}
	g.vel = Point{X: 1, Y: 0}
	g.paddles[1] = 0

	g.stepBall()

	if g.phase != phaseOver || g.winner != 0 {
		t.Fatalf("phase %v winner %d, want game over won by player 1", g.phase, g.winner)
	}
	if g.winnerText() != "P1 WINS" {
		t.Fatalf("winner text = %q, want P1 WINS", g.winnerText())
	}

	// Nothing moves once the match is over.
	g.lastTick = time.Now().Add(-time.Hour)
	g.Update()
	if g.ball != (Point{X: boardColumns / 2, Y: boardRows / 2}) {
		t.Fatalf("ball moved after game over: %+v", g.ball)
	}
}

func TestChallengeScoreCountsHitsPointsAndTheWin(t *testing.T) {
	g := newTestGame()
	startRally(g, modeHard, 1) // multiplier 3
	mult := profiles[modeHard].mult

	g.paddles[0] = 6
	g.ball = Point{X: 2, Y: 7}
	g.vel = Point{X: -1, Y: -1}
	g.stepBall() // human return
	if g.score != hitPoints*mult {
		t.Fatalf("score after a return = %d, want %d", g.score, hitPoints*mult)
	}

	g.paddles[1] = 0
	g.ball = Point{X: boardColumns - 1, Y: 7}
	g.vel = Point{X: 1, Y: 0}
	g.stepBall() // human wins the point and, with it, the match
	want := (hitPoints + pointPoints + winPoints) * mult
	if g.score != want {
		t.Fatalf("score after winning = %d, want %d", g.score, want)
	}
	if g.best != want || g.store.Best(gameName) != want {
		t.Fatalf("best = %d (store %d), want %d", g.best, g.store.Best(gameName), want)
	}
	if g.winnerText() != "YOU WIN" {
		t.Fatalf("winner text = %q, want YOU WIN", g.winnerText())
	}
}

func TestBotPointsEarnNothing(t *testing.T) {
	g := newTestGame()
	startRally(g, modeEasy, 1)

	g.paddles[1] = 6
	g.ball = Point{X: rightPaddleX - 1, Y: 7}
	g.vel = Point{X: 1, Y: 0}
	g.stepBall() // bot return

	g.paddles[0] = 6
	g.ball = Point{X: 0, Y: 2}
	g.vel = Point{X: -1, Y: 0}
	g.stepBall() // bot wins the point and the match

	if g.score != 0 || g.store.Best(gameName) != 0 {
		t.Fatalf("score = %d best = %d, want 0 (only the human scores)", g.score, g.store.Best(gameName))
	}
	if g.winnerText() != "BOT WINS" {
		t.Fatalf("winner text = %q, want BOT WINS", g.winnerText())
	}
}

func TestTwoPlayerMatchesDoNotTouchTheHighscore(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 1)

	g.paddles[0] = 6
	g.ball = Point{X: 2, Y: 7}
	g.vel = Point{X: -1, Y: -1}
	g.stepBall()

	g.paddles[1] = 0
	g.ball = Point{X: boardColumns - 1, Y: 7}
	g.vel = Point{X: 1, Y: 0}
	g.stepBall()

	if g.score != 0 || g.store.Best(gameName) != 0 {
		t.Fatalf("score = %d best = %d, want 0 in a two-player match", g.score, g.store.Best(gameName))
	}
}

func TestBotTracksThePredictedRowOnceTheBallIsPastItsReactionColumn(t *testing.T) {
	g := newTestGame()
	startRally(g, modeHard, 5)
	prof := profiles[modeHard]
	g.botErr = 0
	center := g.paddles[1] + paddleHeight/2

	// Ball heading towards the bot but not yet past the reaction column.
	g.ball = Point{X: prof.reactAt - 1, Y: 0}
	g.vel = Point{X: 1, Y: 1}
	for i := 0; i < 2*prof.moveEvery; i++ {
		g.botMove()
	}
	if g.paddles[1]+paddleHeight/2 != center {
		t.Fatal("the bot must not react before the ball crosses its reaction column")
	}

	// Ball heading away: the bot stays where the rally left it.
	g.ball = Point{X: prof.reactAt + 1, Y: 0}
	g.vel = Point{X: -1, Y: 1}
	for i := 0; i < 2*prof.moveEvery; i++ {
		g.botMove()
	}
	if g.paddles[1]+paddleHeight/2 != center {
		t.Fatal("the bot must not move while the ball travels away from it")
	}

	// Ball heading its way and past the column, crossing above the paddle:
	// one step every moveEvery ticks, towards the predicted row.
	g.ball = Point{X: prof.reactAt, Y: 12}
	g.vel = Point{X: 1, Y: -1}
	if want := g.predictRow(); want >= center {
		t.Fatalf("test setup: predicted row %d should be above the centre %d", want, center)
	}
	g.botTicks = 0
	for i := 0; i < prof.moveEvery-1; i++ {
		g.botMove()
	}
	if g.paddles[1]+paddleHeight/2 != center {
		t.Fatal("the bot moves only every moveEvery ticks")
	}
	g.botMove()
	if g.paddles[1]+paddleHeight/2 != center-1 {
		t.Fatalf("paddle centre = %d, want %d (one step up towards the crossing row)", g.paddles[1]+paddleHeight/2, center-1)
	}
}

func TestBotAimErrorIsRolledWhenTheBallHeadsItsWay(t *testing.T) {
	g := newTestGame()
	startRally(g, modeEasy, 5)
	maxErr := profiles[modeEasy].aimError

	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		g.paddles[0] = 6
		g.ball = Point{X: 2, Y: 7}
		g.vel = Point{X: -1, Y: -1}
		g.stepBall() // human return re-rolls the error
		if g.botErr < -maxErr || g.botErr > maxErr {
			t.Fatalf("botErr = %d, want within ±%d", g.botErr, maxErr)
		}
		seen[g.botErr] = true
	}
	if !seen[0] || !seen[maxErr] || !seen[-maxErr] {
		t.Fatalf("bot error should sometimes be 0, +%d and -%d; saw %v", maxErr, maxErr, seen)
	}

	g.mode = modeTwoPlayers
	g.rollBotError()
	if g.botErr != 0 {
		t.Fatalf("two-player error = %d, want 0", g.botErr)
	}
}

func TestRecentringBotDriftsBackWhileTheBallMovesAway(t *testing.T) {
	g := newTestGame()
	startRally(g, modeNormal, 5)
	prof := profiles[modeNormal]
	g.paddles[1] = 0
	g.ball = Point{X: 20, Y: 3}
	g.vel = Point{X: -1, Y: 1}

	g.botTicks = 0
	for i := 0; i < 2*prof.recenterEvery; i++ {
		g.botMove()
	}
	if g.paddles[1] != 2 {
		t.Fatalf("paddle top = %d, want 2 (two steps back towards the middle)", g.paddles[1])
	}

	// Hard never recentres.
	g.mode = modeHard
	g.paddles[1] = 0
	for i := 0; i < 8; i++ {
		g.botMove()
	}
	if g.paddles[1] != 0 {
		t.Fatalf("hard bot moved to %d while the ball travels away; want it to stay put", g.paddles[1])
	}
}

func TestBotAimsAtThePredictedRowPlusItsError(t *testing.T) {
	g := newTestGame()
	startRally(g, modeEasy, 5)
	prof := profiles[modeEasy]
	g.paddles[1] = 0
	g.ball = Point{X: rightPaddleX - 12, Y: 12}
	g.vel = Point{X: 1, Y: -1}
	if want := g.predictRow(); want != 0 {
		t.Fatalf("test setup: predicted row = %d, want 0", want)
	}
	// With a +2 error the bot aims at row 2: one step down from a paddle
	// centred on row 1, then it holds.
	g.botErr = 2
	g.botTicks = 0
	for i := 0; i < 4*prof.moveEvery; i++ {
		g.botMove()
	}
	if g.paddles[1] != 1 {
		t.Fatalf("paddle top = %d, want 1 (centre on the misjudged row 2)", g.paddles[1])
	}
}

func TestBotsAreBeatableAndNotHopeless(t *testing.T) {
	// A returned ball is aimed straight back at the human's paddle, which
	// sits still at a random row; the human always returns it. Over many
	// rallies each bot must return some balls but not all of them.
	for _, m := range []mode{modeEasy, modeNormal, modeHard} {
		g := newTestGame()
		startRally(g, m, 99)
		returns, misses := 0, 0
		for i := 0; i < 400; i++ {
			g.phase = phasePlaying
			g.paddles[0] = g.rng.Intn(boardRows - paddleHeight + 1)
			g.ball = Point{X: 2, Y: g.paddles[0] + g.rng.Intn(paddleHeight)}
			g.vel = Point{X: -1, Y: 1 - 2*g.rng.Intn(2)}
			g.stepBall() // human return, bot error rolled
			botHits := g.points[1]
			for steps := 0; g.phase == phasePlaying && g.vel.X > 0; steps++ {
				g.tick()
				if steps > 3*boardColumns {
					t.Fatalf("%s: rally never resolved", modeNames[m])
				}
			}
			if g.phase == phaseServing || g.points[1] != botHits {
				misses++
			} else {
				returns++
			}
		}
		if returns == 0 || misses == 0 {
			t.Fatalf("%s: %d returns, %d misses; want both", modeNames[m], returns, misses)
		}
		t.Logf("%s: %d returns, %d misses", modeNames[m], returns, misses)
	}
}

func TestPredictRowMatchesTheSimulatedPath(t *testing.T) {
	g := newTestGame()
	startRally(g, modeHard, 5)
	g.paddles[1] = 0 // out of the predicted row's way

	starts := []struct {
		ball Point
		vel  Point
	}{
		{Point{X: 17, Y: 3}, Point{X: 1, Y: 1}},
		{Point{X: 5, Y: 14}, Point{X: 1, Y: -1}},
		{Point{X: 20, Y: 7}, Point{X: 1, Y: 1}},
		{Point{X: 2, Y: 0}, Point{X: 1, Y: -1}},
	}
	for _, s := range starts {
		g.ball, g.vel = s.ball, s.vel
		want := g.predictRow()

		// Park the paddle away from the predicted row so the ball crosses
		// the paddle column instead of bouncing back.
		g.paddles[1] = 0
		if want < paddleHeight {
			g.paddles[1] = boardRows - paddleHeight
		}

		for steps := 0; g.ball.X < rightPaddleX; steps++ {
			if steps > 2*boardColumns {
				t.Fatalf("from %+v %+v: ball never reached the paddle column (at %+v)", s.ball, s.vel, g.ball)
			}
			g.stepBall()
		}
		if g.ball.Y != want {
			t.Fatalf("from %+v %+v: predicted row %d, ball crossed at row %d", s.ball, s.vel, want, g.ball.Y)
		}
	}
}

func TestInputDrivesThePaddles(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)
	top := g.paddles[0]

	if !g.HandleInput(key(tcell.KeyRune, 'w')) || g.paddles[0] != top-1 {
		t.Fatalf("W: left paddle = %d, want %d", g.paddles[0], top-1)
	}
	if !g.HandleInput(key(tcell.KeyRune, 's')) || g.paddles[0] != top {
		t.Fatalf("S: left paddle = %d, want %d", g.paddles[0], top)
	}
	if !g.HandleInput(key(tcell.KeyUp, 0)) || g.paddles[1] != top-1 {
		t.Fatalf("Up: right paddle = %d, want %d", g.paddles[1], top-1)
	}
	if !g.HandleInput(key(tcell.KeyDown, 0)) || g.paddles[1] != top {
		t.Fatalf("Down: right paddle = %d, want %d", g.paddles[1], top)
	}
	if g.HandleInput(key(tcell.KeyRune, 'x')) {
		t.Fatal("unbound keys must not be consumed")
	}
}

func TestArrowsDriveTheHumanAgainstABot(t *testing.T) {
	g := newTestGame()
	startRally(g, modeEasy, 5)
	top := g.paddles[0]

	g.HandleInput(key(tcell.KeyUp, 0))
	if g.paddles[0] != top-1 || g.paddles[1] != top {
		t.Fatalf("paddles = %v, want only the left (human) paddle to move up", g.paddles)
	}
}

func TestPaddlesStayInsideTheCourt(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 5)

	g.paddles[0] = 0
	if g.movePaddle(0, -1) || g.paddles[0] != 0 {
		t.Fatal("paddle must not move above the top wall")
	}
	g.paddles[0] = boardRows - paddleHeight
	if g.movePaddle(0, 1) || g.paddles[0] != boardRows-paddleHeight {
		t.Fatal("paddle must not move below the bottom wall")
	}
}

func TestPauseAndResume(t *testing.T) {
	g := newTestGame()

	g.Pause()
	if g.IsPaused() {
		t.Fatal("the setup screen has nothing to pause")
	}

	startRally(g, modeTwoPlayers, 5)
	g.Pause()
	if !g.IsPaused() {
		t.Fatal("a rally should pause")
	}
	before := g.ball
	g.lastTick = time.Now().Add(-time.Hour)
	g.Update()
	if g.ball != before {
		t.Fatal("the ball must not move while paused")
	}
	if g.HandleInput(key(tcell.KeyRune, 'w')) {
		t.Fatal("paddles must not move while paused")
	}

	g.Resume()
	if g.IsPaused() {
		t.Fatal("Resume should unpause")
	}

	g.phase = phaseOver
	g.Pause()
	if g.IsPaused() {
		t.Fatal("a finished match has nothing to pause")
	}
}

func TestResetRestartsAConfiguredMatch(t *testing.T) {
	g := newTestGame()
	startRally(g, modeEasy, 7)
	g.points = [2]int{2, 3}
	g.score = 40
	g.paddles = [2]int{0, 0}

	g.Reset()

	if g.phase != phaseServing {
		t.Fatalf("phase = %v, want a fresh serve with the same settings", g.phase)
	}
	if g.mode != modeEasy || g.target != 7 {
		t.Fatalf("settings changed on reset: %s first to %d", modeNames[g.mode], g.target)
	}
	if g.points != [2]int{} || g.score != 0 {
		t.Fatalf("points %v score %d, want a clean sheet", g.points, g.score)
	}
	top := (boardRows - paddleHeight) / 2
	if g.paddles != [2]int{top, top} {
		t.Fatalf("paddles = %v, want both recentred", g.paddles)
	}
}

func TestResetBeforeSetupStaysOnSetup(t *testing.T) {
	g := newTestGame()
	g.HandleInput(key(tcell.KeyRight, 0))

	g.Reset()

	if g.phase != phaseSetup {
		t.Fatalf("phase = %v, want setup", g.phase)
	}
}

func TestGameOverKeysRematchOrReopenSetup(t *testing.T) {
	g := newTestGame()
	startRally(g, modeTwoPlayers, 1)
	g.phase = phaseOver
	g.winner = 1

	if !g.HandleInput(key(tcell.KeyEnter, 0)) || g.phase != phaseServing {
		t.Fatalf("Enter should start a rematch, phase = %v", g.phase)
	}

	g.phase = phaseOver
	if !g.HandleInput(key(tcell.KeyRune, 'm')) || g.phase != phaseSetup {
		t.Fatalf("M should reopen setup, phase = %v", g.phase)
	}
	if g.configured {
		t.Fatal("reopening setup must clear the configured flag so R reopens setup too")
	}
	g.Reset()
	if g.phase != phaseSetup {
		t.Fatalf("Reset after reopening setup: phase = %v, want setup", g.phase)
	}
}
