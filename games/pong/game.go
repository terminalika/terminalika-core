// Package pong implements the classic Pong game for terminalika-core: a
// two-player table and a bot challenge with three difficulties.
//
// The court physics (a ball that reflects off the top and bottom walls, a
// point when it slips past a paddle, a short breather before the next serve,
// paddles that recentre after every point) and the predictive bot are ported
// from spinzed/tpong and squeezed onto a small cell grid.
package pong

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/highscore"
)

const (
	boardColumns = 31
	boardRows    = 15

	gameName = "pong"

	paddleHeight = 3
	leftPaddleX  = 1
	rightPaddleX = boardColumns - 2

	// serveDelay is the breather after every point (and at match start)
	// before the ball is served; the ball blinks at the centre meanwhile.
	serveDelay = time.Second

	// minBallPeriod caps the rally speed-up: every paddle hit shaves the
	// mode's accel off the ball period, down to this.
	minBallPeriod = 60 * time.Millisecond

	// A held paddle key moves the paddle one row right away, waits
	// paddleFirstDelay (so a tap is one row), then keeps it moving one row
	// every paddlePeriod for as long as the key stays down.
	paddlePeriod     = 90 * time.Millisecond
	paddleFirstDelay = 120 * time.Millisecond

	// paddleResumeWindow absorbs the host engine's own release-detection
	// jitter: without real terminal key-release events, the engine
	// synthesises a release from a gap in the key's auto-repeat, then sees
	// the terminal's next real repeat arrive as an ordinary press -
	// indistinguishable at the protocol level from a genuine fresh key-down.
	// A press for the same direction this paddle was already driving within
	// this window is treated as that catching up, not the player releasing
	// and re-pressing: no bonus step, no timer reset. Without this, a
	// tighter engine release timeout produced more such blips per second,
	// each adding an extra step - so the paddle sped up the more
	// responsive release detection got, instead of just reacting faster.
	paddleResumeWindow = 60 * time.Millisecond

	// maxCatchUp bounds how many overdue steps a single Update makes up for
	// after a stall, so the game skips time instead of fast-forwarding.
	maxCatchUp = 3

	// Challenge scoring (bot modes only), each multiplied by the
	// difficulty's multiplier.
	hitPoints   = 1
	pointPoints = 10
	winPoints   = 50
)

// targets are the match lengths offered on the setup screen ("first to N").
var targets = []int{3, 5, 7, 11, 15, 21}

// Command types.
const (
	cmdConfigure = "pong.configure"
	cmdStart     = "pong.start"
	cmdMove      = "pong.move"
	cmdTick      = "pong.tick"
	cmdPause     = "pong.pause"
	cmdResume    = "pong.resume"
	cmdReset     = "pong.reset"
)

// Event types.
const (
	evConfigured   = "pong.configured"
	evMatchStarted = "pong.match_started"
	evServe        = "pong.serve"
	evPaddleMoved  = "pong.paddle_moved"
	evBallBounced  = "pong.ball_bounced"
	evPointScored  = "pong.point_scored"
	evGameOver     = "pong.game_over"
	evPaused       = "game.paused"
	evResumed      = "game.resumed"
	evReset        = "game.reset"
)

// Point is a single board cell (or a per-tick velocity in cells).
type Point struct {
	X int
	Y int
}

// mode is the opponent: a second human or a bot at one of three
// difficulties.
type mode int

const (
	modeTwoPlayers mode = iota
	modeEasy
	modeNormal
	modeHard
)

var modeNames = [...]string{"2 PLAYERS", "EASY BOT", "NORMAL BOT", "HARD BOT"}

// modeKeys are the mode names used in payloads.
var modeKeys = [...]string{"2p", "easy", "normal", "hard"}

func (m mode) bot() bool { return m != modeTwoPlayers }

// profile tunes the ball and, for bot modes, the bot. The bot is a port of
// tpong's AI: once the ball crosses reactAt on its way over, it predicts the
// row where the ball will meet its paddle (wall bounces included) and moves
// its paddle one row every moveEvery ball ticks towards it, so from reactAt
// it can cover (rightPaddleX-reactAt)/moveEvery rows. While the ball travels
// away a bot with recenterEvery drifts back to the middle one row every
// that many ticks, so the next shot finds it roughly centred.
//
// What makes a bot beatable: every time the ball heads its way it has a
// missChance of misreading the crossing row by aimError rows (with a
// three-row paddle a two-row error is a clean miss), and a bot that never
// recentres can be pulled to one side and passed in the far corner.
type profile struct {
	period time.Duration // ball tick period at the start of every rally
	accel  time.Duration // period shaved off by every paddle hit

	reactAt       int     // ball column from which the bot tracks the ball
	moveEvery     int     // ball ticks per bot paddle step
	recenterEvery int     // ball ticks per step back to the middle (0: never)
	aimError      int     // rows the bot misreads the crossing row by on a miss
	missChance    float64 // chance of misreading, per trip of the ball
	mult          int     // challenge score multiplier
}

var profiles = map[mode]profile{
	modeTwoPlayers: {period: 110 * time.Millisecond, accel: 2 * time.Millisecond},
	// easy: slow ball, reach ±6 rows from where it stands, drifts back to
	// the middle slowly and misreads two shots in five.
	modeEasy: {period: 130 * time.Millisecond, accel: 1 * time.Millisecond, reactAt: 17, moveEvery: 2, recenterEvery: 4, aimError: 2, missChance: 0.4, mult: 1},
	// normal: reach ±7 rows, recentres a bit faster and misreads one shot in
	// five.
	modeNormal: {period: 110 * time.Millisecond, accel: 2 * time.Millisecond, reactAt: 15, moveEvery: 2, recenterEvery: 3, aimError: 2, missChance: 0.2, mult: 2},
	// hard: fast ball, reach ±6 rows and misreads only one shot in ten - but
	// it never recentres, so shots into the corner away from it get past.
	modeHard: {period: 90 * time.Millisecond, accel: 2 * time.Millisecond, reactAt: 17, moveEvery: 2, aimError: 2, missChance: 0.1, mult: 3},
}

// phase is where the game is in its flow: the setup screen, the breather
// before a serve, a rally, or the end of the match.
type phase int

const (
	phaseSetup phase = iota
	phaseServing
	phasePlaying
	phaseOver
)

// Setup screen rows.
const (
	setupMode = iota
	setupTarget
	setupRows
)

// Game holds the full Pong state. It implements core.Game.
type Game struct {
	phase      phase
	configured bool // a match has been set up: R restarts it instead of reopening setup
	setupRow   int
	mode       mode
	target     int

	paddles [2]int // top row of the left (0) and right (1) paddle
	ball    Point
	vel     Point
	points  [2]int
	winner  int // 0 or 1 once the match is over

	serveTo  int // player the next serve travels towards
	serveAt  time.Time
	period   time.Duration
	lastTick time.Time
	botTicks int
	botErr   int // rows the bot currently misjudges the crossing row by

	held       [2]int       // direction each paddle's held key drives it (-1, 0, 1)
	paddleNext [2]time.Time // when each held paddle moves next

	// lastDir/lastDirAt track the direction each paddle was actually driven
	// in independently of held, which the engine can clear on its own
	// release-detection schedule; see paddleResumeWindow.
	lastDir   [2]int
	lastDirAt [2]time.Time

	score       int
	best        int
	paused      bool
	pauseReason string

	rng     *rand.Rand
	store   *highscore.Store
	emitter core.Emitter
}

// Event payloads.
type cellPos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type setupPayload struct {
	Mode   string `json:"mode"`
	Target int    `json:"target"`
}

type servePayload struct {
	To       int     `json:"to"`
	Ball     cellPos `json:"ball"`
	Velocity cellPos `json:"velocity"`
}

type paddleMovedPayload struct {
	Player int `json:"player"`
	Y      int `json:"y"`
}

type ballBouncedPayload struct {
	Kind     string  `json:"kind"`
	Player   int     `json:"player,omitempty"`
	Ball     cellPos `json:"ball"`
	Velocity cellPos `json:"velocity"`
}

type pointScoredPayload struct {
	Player int `json:"player"`
	P1     int `json:"p1"`
	P2     int `json:"p2"`
	Score  int `json:"score"`
}

type gameOverPayload struct {
	Winner int    `json:"winner"`
	P1     int    `json:"p1"`
	P2     int    `json:"p2"`
	Score  int    `json:"score"`
	Best   int    `json:"best"`
	Reason string `json:"reason"`
}

type pausedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type configurePayload struct {
	Mode   string `json:"mode"`
	Target int    `json:"target"`
}

type movePayload struct {
	Player int `json:"player"`
	DY     int `json:"dy"`
}

// SetEmitter sets the emitter used to publish domain events.
func (g *Game) SetEmitter(e core.Emitter) {
	g.emitter = e
}

// emit publishes an event unless no emitter is configured.
func (g *Game) emit(typ string, payload any) {
	if g.emitter == nil {
		return
	}
	ev := core.Event{Type: typ, Game: gameName, At: time.Now()}
	if payload != nil {
		ev.Payload = core.MustJSON(payload)
	}
	g.emitter.Emit(ev)
}

// New returns a fresh Pong game (on its setup screen) that persists best
// challenge scores to the default location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a fresh Pong game using the given score store.
func NewWithStore(store *highscore.Store) *Game {
	g := &Game{
		store:  store,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		mode:   modeNormal,
		target: targets[1],
	}
	g.Reset()
	return g
}

// Init hides the cursor and resets the game.
func (g *Game) Init(screen tcell.Screen) error {
	screen.HideCursor()
	g.Reset()
	return nil
}

// Reset restarts the match with the current settings, or opens the setup
// screen when no match has been set up yet.
func (g *Game) Reset() {
	g.paused = false
	g.pauseReason = ""
	g.best = g.store.Best(gameName)
	g.score = 0
	g.points = [2]int{}
	g.winner = 0
	g.resetPaddleControl()
	g.centerPaddles()
	g.parkBall()
	g.emit(evReset, nil)

	if g.configured {
		g.startMatch()
		return
	}
	g.phase = phaseSetup
	g.setupRow = setupMode
}

// startMatch begins a match with the current settings: scores to zero,
// paddles in the middle and a serve towards a random side.
func (g *Game) startMatch() {
	g.configured = true
	g.points = [2]int{}
	g.score = 0
	g.winner = 0
	g.resetPaddleControl()
	g.centerPaddles()
	g.serveTo = g.rng.Intn(2)
	g.scheduleServe()
	g.emit(evMatchStarted, g.setupPayload())
}

func (g *Game) setupPayload() setupPayload {
	return setupPayload{Mode: modeKeys[g.mode], Target: g.target}
}

// resetPaddleControl drops any held key, so a new rally (or a pause) always
// starts with the paddles free to move on the very next press.
func (g *Game) resetPaddleControl() {
	g.held = [2]int{}
	g.lastDir = [2]int{}
	g.lastDirAt = [2]time.Time{}
}

func (g *Game) centerPaddles() {
	top := (boardRows - paddleHeight) / 2
	g.paddles = [2]int{top, top}
}

// parkBall puts the ball in the middle of the court, still.
func (g *Game) parkBall() {
	g.ball = Point{X: boardColumns / 2, Y: boardRows / 2}
	g.vel = Point{}
}

// scheduleServe parks the ball and starts the breather before the serve.
// Every rally starts at the mode's base speed.
func (g *Game) scheduleServe() {
	g.parkBall()
	g.period = profiles[g.mode].period
	g.serveAt = time.Now().Add(serveDelay)
	g.phase = phaseServing
}

// serve launches the ball from the centre line towards serveTo, from a
// random row and at a random diagonal.
func (g *Game) serve() {
	g.ball = Point{X: boardColumns / 2, Y: 1 + g.rng.Intn(boardRows-2)}
	g.vel = Point{X: -1, Y: 1}
	if g.serveTo == 1 {
		g.vel.X = 1
	}
	if g.rng.Intn(2) == 0 {
		g.vel.Y = -1
	}
	g.phase = phasePlaying
	g.lastTick = time.Now()
	g.botTicks = 0
	g.rollBotError()
	g.emit(evServe, servePayload{To: g.serveTo + 1, Ball: pos(g.ball), Velocity: pos(g.vel)})
}

// Update moves held paddles, serves the ball once the breather is over and
// then advances the rally one cell per tick period. Ticks run on a fixed
// step (the clock advances by whole periods, never "now"), so the ball moves
// evenly no matter how the frame rate lines up with the period. The bot
// moves on the ball's clock.
func (g *Game) Update() {
	if g.paused {
		return
	}
	now := time.Now()
	switch g.phase {
	case phaseServing:
		g.stepPaddles(now)
		if now.Before(g.serveAt) {
			return
		}
		g.serve()
	case phasePlaying:
		g.stepPaddles(now)
		for steps := 0; steps < maxCatchUp && !now.Before(g.lastTick.Add(g.period)); steps++ {
			g.lastTick = g.lastTick.Add(g.period)
			g.tick()
			if g.phase != phasePlaying {
				return
			}
		}
		if now.Sub(g.lastTick) > g.period {
			g.lastTick = now // still behind after a stall: skip, don't fast-forward
		}
	}
}

// stepPaddles moves every paddle whose key is held, one row per
// paddlePeriod on a fixed step, for as long as the key stays down.
func (g *Game) stepPaddles(now time.Time) {
	for p, dir := range g.held {
		if dir == 0 {
			continue
		}
		for steps := 0; steps < maxCatchUp && !now.Before(g.paddleNext[p]); steps++ {
			g.movePaddle(p, dir)
			g.paddleNext[p] = g.paddleNext[p].Add(paddlePeriod)
		}
		if now.Sub(g.paddleNext[p]) > paddlePeriod {
			g.paddleNext[p] = now
		}
	}
}

// tick is one step of the rally: the bot reacts, then the ball moves.
func (g *Game) tick() {
	if g.mode.bot() {
		g.botMove()
	}
	g.stepBall()
}

// stepBall moves the ball one cell. It reflects off the top and bottom
// walls, bounces off a paddle it would run into (holding still for that one
// tick, the way tpong's ball sits on the contact point for a frame) and
// scores a point when it leaves the court behind a paddle.
func (g *Game) stepBall() {
	next := Point{X: g.ball.X + g.vel.X, Y: g.ball.Y + g.vel.Y}

	if next.Y < 0 || next.Y >= boardRows {
		g.vel.Y = -g.vel.Y
		next.Y = g.ball.Y + g.vel.Y
		g.emit(evBallBounced, ballBouncedPayload{Kind: "wall", Ball: pos(next), Velocity: pos(g.vel)})
	}

	switch {
	case g.vel.X < 0 && next.X == leftPaddleX && g.covers(0, next.Y):
		g.hitPaddle(0, next.Y)
		return
	case g.vel.X > 0 && next.X == rightPaddleX && g.covers(1, next.Y):
		g.hitPaddle(1, next.Y)
		return
	case next.X < 0:
		g.scorePoint(1)
		return
	case next.X >= boardColumns:
		g.scorePoint(0)
		return
	}

	g.ball = next
}

// covers reports whether player p's paddle occupies row y.
func (g *Game) covers(p, y int) bool {
	top := g.paddles[p]
	return y >= top && y < top+paddleHeight
}

// hitPaddle bounces the ball back off player p's paddle. Where the ball
// meets the paddle sets its new angle: the top third sends it up, the
// bottom third down, the middle keeps its current angle. Every hit speeds
// the rally up a little, and a human return earns challenge points.
func (g *Game) hitPaddle(p, y int) {
	g.vel.X = -g.vel.X
	switch y - g.paddles[p] {
	case 0:
		g.vel.Y = -1
	case paddleHeight - 1:
		g.vel.Y = 1
	}

	g.period -= profiles[g.mode].accel
	if g.period < minBallPeriod {
		g.period = minBallPeriod
	}

	if g.mode.bot() && p == 0 {
		g.addScore(hitPoints)
		g.rollBotError()
	}
	g.emit(evBallBounced, ballBouncedPayload{Kind: "paddle", Player: p + 1, Ball: pos(g.ball), Velocity: pos(g.vel)})
}

// scorePoint awards a point to player p, then either ends the match or
// lines up the next serve towards the player who conceded.
func (g *Game) scorePoint(p int) {
	g.points[p]++
	if g.mode.bot() && p == 0 {
		g.addScore(pointPoints)
	}
	g.emit(evPointScored, pointScoredPayload{Player: p + 1, P1: g.points[0], P2: g.points[1], Score: g.score})

	if g.points[p] >= g.target {
		g.winner = p
		g.phase = phaseOver
		g.parkBall()
		if g.mode.bot() && p == 0 {
			g.addScore(winPoints)
		}
		g.emit(evGameOver, gameOverPayload{
			Winner: p + 1, P1: g.points[0], P2: g.points[1],
			Score: g.score, Best: g.best, Reason: "win",
		})
		return
	}

	g.serveTo = 1 - p
	g.centerPaddles()
	g.scheduleServe()
}

// addScore adds n challenge points (scaled by the difficulty) and records a
// new best right away.
func (g *Game) addScore(n int) {
	g.score += n * profiles[g.mode].mult
	if g.score > g.best {
		g.best = g.score
		g.store.Submit(gameName, g.score)
	}
}

// movePaddle moves player p's paddle dy rows, staying inside the court. It
// reports whether the paddle moved.
func (g *Game) movePaddle(p, dy int) bool {
	top := g.paddles[p] + dy
	if top < 0 || top > boardRows-paddleHeight {
		return false
	}
	g.paddles[p] = top
	g.emit(evPaddleMoved, paddleMovedPayload{Player: p + 1, Y: top})
	return true
}

// rollBotError decides whether the bot misreads the ball's next trip over
// and, if so, by how much (aimError rows up or down).
func (g *Game) rollBotError() {
	prof := profiles[g.mode]
	g.botErr = 0
	if prof.aimError > 0 && g.rng.Float64() < prof.missChance {
		g.botErr = prof.aimError
		if g.rng.Intn(2) == 0 {
			g.botErr = -prof.aimError
		}
	}
}

// botMove drives the right paddle (tpong's AI). Once the ball is heading
// its way and past its reaction column the bot steers the paddle's middle
// towards the row where it reads the ball crossing its paddle. While the
// ball travels away a recentring bot drifts back to the middle; any other
// bot stays where the rally left it.
func (g *Game) botMove() {
	prof := profiles[g.mode]
	g.botTicks++

	var target int
	switch {
	case g.vel.X > 0 && g.ball.X >= prof.reactAt:
		if g.botTicks%prof.moveEvery != 0 {
			return
		}
		target = g.predictRow() + g.botErr
		if target < 0 {
			target = 0
		}
		if target > boardRows-1 {
			target = boardRows - 1
		}
	case g.vel.X < 0 && prof.recenterEvery > 0:
		if g.botTicks%prof.recenterEvery != 0 {
			return
		}
		target = boardRows / 2
	default:
		return
	}

	center := g.paddles[1] + paddleHeight/2
	switch {
	case target > center:
		g.movePaddle(1, 1)
	case target < center:
		g.movePaddle(1, -1)
	}
}

// predictRow returns the row the ball will be on when it reaches the right
// paddle's column, folding its path at the walls the way stepBall reflects
// it.
func (g *Game) predictRow() int {
	dist := rightPaddleX - g.ball.X
	y := g.ball.Y + g.vel.Y*dist

	period := 2 * (boardRows - 1)
	y = ((y % period) + period) % period
	if y > boardRows-1 {
		y = period - y
	}
	return y
}

// HandleInput handles the game-specific keys. On the setup screen the
// arrows (or WASD) pick the mode and match length and Enter starts. In a
// match W/S drive the left paddle and Up/Down the right one; against a bot
// both pairs drive the player's (left) paddle. Once the match is over Enter
// starts a rematch and M reopens setup. SPACE, R and ESC are reserved for
// the launcher and are never claimed here.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.paused {
		return false
	}

	switch g.phase {
	case phaseSetup:
		return g.handleSetupKey(ev)
	case phaseOver:
		switch {
		case ev.Key() == tcell.KeyEnter:
			g.startMatch()
			return true
		case ev.Key() == tcell.KeyRune && (ev.Rune() == 'm' || ev.Rune() == 'M'):
			g.openSetup()
			return true
		}
		return false
	}

	p, dy, ok := g.paddleKey(ev)
	if !ok {
		return false
	}
	g.movePaddle(p, dy)
	return true
}

// paddleKey maps a key to the paddle it drives and the direction: W/S the
// left paddle, Up/Down the right one - or, against a bot, the player's left
// paddle as well.
func (g *Game) paddleKey(ev *tcell.EventKey) (player, dy int, ok bool) {
	right := 1
	if g.mode.bot() {
		right = 0
	}
	switch ev.Key() {
	case tcell.KeyUp:
		return right, -1, true
	case tcell.KeyDown:
		return right, 1, true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W':
			return 0, -1, true
		case 's', 'S':
			return 0, 1, true
		}
	}
	return 0, 0, false
}

// HandleKeyState drives the paddles from held keys (core.KeyStateHandler). A
// press moves the paddle one row at once, then stepPaddles keeps moving it
// one row every paddlePeriod for as long as the key stays down. Repeated
// presses of an already-held key are ignored, so terminal auto-repeat can't
// double-step it, and a press that only looks fresh because the engine's own
// release-detection jitter cleared held is absorbed instead of double-
// stepping it too (see paddleResumeWindow). Paddle keys are only claimed
// while a rally (or its serve) is on.
func (g *Game) HandleKeyState(ev *tcell.EventKey, pressed bool) bool {
	if !g.running() {
		return false
	}
	p, dy, ok := g.paddleKey(ev)
	if !ok {
		return false
	}
	if !pressed {
		if g.held[p] == dy {
			g.held[p] = 0
		}
		return true
	}
	if g.held[p] == dy {
		return true
	}

	// A press for a direction the engine does not currently believe is
	// held. If this paddle was already being driven in the very same
	// direction moments ago, it is the engine's own release-detection
	// jitter catching up rather than the player releasing and re-pressing
	// (see paddleResumeWindow): just restore held and let the existing step
	// timer carry on untouched, with no bonus step.
	now := time.Now()
	resuming := dy == g.lastDir[p] && now.Sub(g.lastDirAt[p]) < paddleResumeWindow
	// Reversing while the paddle is already moving (the other direction was
	// held) skips the tap-precision delay: the player is already engaged in
	// continuous control, most often correcting an overshoot, and waiting
	// out paddleFirstDelay again on every reversal reads as the paddle
	// lagging. A fresh press from a stopped paddle keeps the full delay, so
	// a tap still moves exactly one row.
	wasMoving := g.held[p] != 0

	g.held[p] = dy
	g.lastDir[p] = dy
	g.lastDirAt[p] = now
	if resuming {
		return true
	}

	g.movePaddle(p, dy)
	delay := paddleFirstDelay
	if wasMoving {
		delay = paddlePeriod
	}
	g.paddleNext[p] = now.Add(delay)
	return true
}

func (g *Game) handleSetupKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp:
		g.setupRow = (g.setupRow + setupRows - 1) % setupRows
	case tcell.KeyDown:
		g.setupRow = (g.setupRow + 1) % setupRows
	case tcell.KeyLeft:
		g.cycleSetup(-1)
	case tcell.KeyRight:
		g.cycleSetup(1)
	case tcell.KeyEnter:
		g.startMatch()
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W', 'k':
			g.setupRow = (g.setupRow + setupRows - 1) % setupRows
		case 's', 'S', 'j':
			g.setupRow = (g.setupRow + 1) % setupRows
		case 'a', 'A', 'h':
			g.cycleSetup(-1)
		case 'd', 'D', 'l':
			g.cycleSetup(1)
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// cycleSetup steps the selected setup row's value by delta, wrapping around.
func (g *Game) cycleSetup(delta int) {
	switch g.setupRow {
	case setupMode:
		n := len(modeNames)
		g.mode = mode((int(g.mode) + delta + n) % n)
	case setupTarget:
		i := -1
		for j, t := range targets {
			if t == g.target {
				i = j
			}
		}
		n := len(targets)
		if i < 0 {
			i = 0
			delta = 0
		}
		g.target = targets[(i+delta+n)%n]
	}
	g.emit(evConfigured, g.setupPayload())
}

// openSetup returns to the setup screen; the next R reopens it as well
// until a match is started again.
func (g *Game) openSetup() {
	g.configured = false
	g.phase = phaseSetup
	g.setupRow = setupMode
	g.points = [2]int{}
	g.score = 0
	g.resetPaddleControl()
	g.centerPaddles()
	g.parkBall()
}

// Pause freezes the rally. The setup screen and a finished match have
// nothing to pause.
func (g *Game) Pause() {
	if g.paused || g.phase == phaseSetup || g.phase == phaseOver {
		return
	}
	g.paused = true
	g.resetPaddleControl()
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume continues the rally and restarts its clocks (a pending serve waits
// its full breather again).
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	g.lastTick = time.Now()
	if g.phase == phaseServing {
		g.serveAt = time.Now().Add(serveDelay)
	}
	g.emit(evResumed, nil)
}

// IsPaused reports whether the game is paused.
func (g *Game) IsPaused() bool { return g.paused }

// HandleCommand applies an externally triggered command.
func (g *Game) HandleCommand(cmd core.Command) error {
	switch cmd.Type {
	case cmdConfigure:
		var p configurePayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		if g.phase != phaseSetup && g.phase != phaseOver {
			return fmt.Errorf("match in progress")
		}
		m, err := parseMode(p.Mode)
		if err != nil {
			return err
		}
		if p.Target < 1 || p.Target > 99 {
			return fmt.Errorf("invalid target %d", p.Target)
		}
		if g.phase == phaseOver {
			g.openSetup()
		}
		g.mode = m
		g.target = p.Target
		g.emit(evConfigured, g.setupPayload())
		return nil
	case cmdStart:
		if g.phase != phaseSetup && g.phase != phaseOver {
			return fmt.Errorf("match in progress")
		}
		g.startMatch()
		return nil
	case cmdMove:
		var p movePayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		if p.Player != 1 && p.Player != 2 {
			return fmt.Errorf("invalid player %d", p.Player)
		}
		if p.DY != -1 && p.DY != 1 {
			return fmt.Errorf("invalid move %d", p.DY)
		}
		if !g.running() {
			return fmt.Errorf("game is not running")
		}
		if p.Player == 2 && g.mode.bot() {
			return fmt.Errorf("player 2 is the bot")
		}
		if !g.movePaddle(p.Player-1, p.DY) {
			return fmt.Errorf("cannot move %d", p.DY)
		}
		return nil
	case cmdTick:
		if !g.running() {
			return fmt.Errorf("game is not running")
		}
		if g.phase == phaseServing {
			g.serve()
			return nil
		}
		g.tick()
		return nil
	case cmdPause:
		var p pausedPayload
		if len(cmd.Payload) > 0 {
			if err := json.Unmarshal(cmd.Payload, &p); err != nil {
				return fmt.Errorf("invalid payload: %w", err)
			}
		}
		g.pauseReason = p.Reason
		g.Pause()
		return nil
	case cmdResume:
		g.Resume()
		return nil
	case cmdReset:
		g.Reset()
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd.Type)
	}
}

// running reports whether a match is in progress and not paused.
func (g *Game) running() bool {
	return !g.paused && (g.phase == phaseServing || g.phase == phasePlaying)
}

// Commands lists the commands this game supports.
func (g *Game) Commands() []core.CommandSpec {
	return []core.CommandSpec{
		{Name: cmdConfigure, Description: "choose the opponent and the match length (setup screen or after a match)", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode":   map[string]any{"type": "string", "enum": modeKeys[:]},
				"target": map[string]any{"type": "integer", "minimum": 1, "maximum": 99},
			},
			"required": []string{"mode", "target"},
		})},
		{Name: cmdStart, Description: "start the match with the current settings (setup screen or rematch)"},
		{Name: cmdMove, Description: "move a paddle one row (player 2 is the bot in bot modes)", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"player": map[string]any{"type": "integer", "enum": []int{1, 2}},
				"dy":     map[string]any{"type": "integer", "enum": []int{-1, 1}},
			},
			"required": []string{"player", "dy"},
		})},
		{Name: cmdTick, Description: "serve a pending ball, or advance the rally one tick"},
		{Name: cmdPause, Description: "pause the game", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{"type": "string"},
			},
		})},
		{Name: cmdResume, Description: "resume the game"},
		{Name: cmdReset, Description: "restart the match (or reopen setup when none was set up)"},
	}
}

func parseMode(s string) (mode, error) {
	for i, k := range modeKeys {
		if k == s {
			return mode(i), nil
		}
	}
	return 0, fmt.Errorf("invalid mode %q", s)
}

func pos(p Point) cellPos { return cellPos{X: p.X, Y: p.Y} }
