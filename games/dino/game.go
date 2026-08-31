// Package dino implements the Chrome offline dinosaur runner for
// terminalika-core: a T-rex runs across a desert, cacti and (later) birds
// scroll in from the right, and a single key jumps. There is no duck - that
// would need a held key, and this game is meant to play the same on every
// terminal, key releases or not (see core.KeyStateHandler).
package dino

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
	boardColumns = 48
	boardRows    = 12

	gameName = "dino"

	// groundRow is the board row the ground line is drawn on. The dino and
	// the cacti stand on the row above it; obstacle and dino cells are
	// counted in rows above the ground, so 0 is standing height.
	groundRow = boardRows - 1

	// The dino's left column and footprint, in board cells.
	dinoX      = 4
	dinoWidth  = 2
	dinoHeight = 2

	// Jump physics, in cells per tick. A jump peaks at about
	// jumpVelocity²/(2·gravity) ≈ 4.8 rows and lasts 2·jumpVelocity/gravity
	// ≈ 11 ticks - long enough to clear the widest cactus cluster with
	// room to spare; this is a break, not a challenge.
	jumpVelocity = 1.5
	gravity      = 0.28

	// Gap between an obstacle's right edge and the next one's left edge, in
	// cells. minGap leaves a full jump's airtime plus a beat to react.
	minGap = 16
	maxGap = 30

	// Birds only join once the run is this long; until then it is cacti only,
	// like the original.
	birdsFromScore = 400

	milestoneEvery = 100
	// A milestone flashes the score for this many ticks.
	milestoneFlashTicks = 8
)

// Command types.
const (
	cmdJump   = "dino.jump"
	cmdStep   = "dino.step"
	cmdPause  = "dino.pause"
	cmdResume = "dino.resume"
	cmdReset  = "dino.reset"
)

// Event types.
const (
	evJumped          = "dino.jumped"
	evLanded          = "dino.landed"
	evObstacleCleared = "dino.obstacle_cleared"
	evMilestone       = "dino.milestone"
	evCollision       = "dino.collision"
	evGameOver        = "dino.game_over"
	evPaused          = "game.paused"
	evResumed         = "game.resumed"
	evReset           = "game.reset"
)

// cell is a position relative to an obstacle's left column and the ground:
// dx counts columns rightwards, dy rows upwards from standing height.
type cell struct {
	dx, dy int
}

// kind is one of the obstacle shapes.
type kind int

const (
	cactusSmall kind = iota
	cactusTall
	cactusPair
	cactusCluster
	birdLow
	birdHigh
)

// shape is an obstacle's footprint: width in columns and the cells it
// occupies for collisions. Birds fly two rows above a standing dino's head
// (birdHigh) - safe to run under, deadly to jump into, like the original -
// or right at its head (birdLow), which must be jumped.
type shape struct {
	name   string
	width  int
	cells  []cell
	isBird bool
}

var shapes = map[kind]shape{
	cactusSmall:   {name: "cactus", width: 1, cells: []cell{{0, 0}}},
	cactusTall:    {name: "tall_cactus", width: 1, cells: []cell{{0, 0}, {0, 1}}},
	cactusPair:    {name: "cactus_pair", width: 2, cells: []cell{{0, 0}, {1, 0}}},
	cactusCluster: {name: "cactus_cluster", width: 3, cells: []cell{{0, 0}, {1, 0}, {2, 0}, {1, 1}}},
	birdLow:       {name: "bird", width: 2, cells: []cell{{0, 1}, {1, 1}}, isBird: true},
	birdHigh:      {name: "high_bird", width: 2, cells: []cell{{0, 4}, {1, 4}}, isBird: true},
}

// obstacle is one obstacle on the board, by its left column.
type obstacle struct {
	kind    kind
	x       int
	cleared bool
}

// Game holds the full dino state. It implements core.Game.
type Game struct {
	// alt is the dino's height above standing, in rows; vy its vertical
	// speed. Both stay at zero on the ground.
	alt      float64
	vy       float64
	airborne bool

	obstacles []obstacle
	// distance is how many ticks the run has lasted, which is also the
	// score and what the ground scrolls by; nextSpawn is the distance at
	// which the next obstacle enters from the right.
	distance  int
	nextSpawn int

	score       int
	best        int
	flashTicks  int
	gameOver    bool
	deathKind   string
	paused      bool
	pauseReason string

	lastTick time.Time
	period   time.Duration

	rng   *rand.Rand
	store *highscore.Store
	keys  core.GlobalKeys
	// The PAUSED / GAME OVER band the last Draw painted, for OverlayArea.
	overlay   core.Rect
	overlayOn bool
	emitter   core.Emitter
}

// Event payloads.
type jumpedPayload struct {
	Score int `json:"score"`
}

type clearedPayload struct {
	Kind  string `json:"kind"`
	Score int    `json:"score"`
}

type milestonePayload struct {
	Score int `json:"score"`
}

type collisionPayload struct {
	Kind string `json:"kind"`
}

type gameOverPayload struct {
	Score  int    `json:"score"`
	Best   int    `json:"best"`
	Reason string `json:"reason"`
}

type pausedPayload struct {
	Reason string `json:"reason,omitempty"`
}

// SetEmitter sets the emitter used to publish domain events.
func (g *Game) SetEmitter(e core.Emitter) {
	g.emitter = e
}

// SetGlobalKeys tells the game what the launcher calls its pause, reset and
// leave keys, for the hint line.
func (g *Game) SetGlobalKeys(keys core.GlobalKeys) {
	g.keys = keys
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

// New returns a freshly reset dino game that persists best scores to the
// default location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a freshly reset dino game using the given score store.
func NewWithStore(store *highscore.Store) *Game {
	g := &Game{store: store, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	g.Reset()
	return g
}

// Init hides the cursor and resets the game.
func (g *Game) Init(screen tcell.Screen) error {
	screen.HideCursor()
	g.Reset()
	return nil
}

// Reset puts the dino back on the ground with an empty desert ahead.
func (g *Game) Reset() {
	g.alt = 0
	g.vy = 0
	g.airborne = false
	g.obstacles = nil
	g.distance = 0
	g.nextSpawn = boardColumns / 2
	g.score = 0
	g.best = g.store.Best(gameName)
	g.flashTicks = 0
	g.gameOver = false
	g.deathKind = ""
	g.paused = false
	g.pauseReason = ""
	g.lastTick = time.Now()
	g.period = tickPeriod(0)
	g.emit(evReset, nil)
}

// Update advances the run one tick when the tick period has elapsed.
func (g *Game) Update() {
	if g.paused || g.gameOver {
		return
	}
	if time.Since(g.lastTick) < g.period {
		return
	}
	g.step()
	g.lastTick = time.Now()
}

// HandleInput handles the game-specific keys: up, W and K jump. SPACE, R and
// ESC are reserved for the launcher and are never claimed here.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.gameOver || g.paused {
		return false
	}

	switch ev.Key() {
	case tcell.KeyUp:
		g.jump()
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W', 'k', 'K':
			g.jump()
			return true
		}
	}
	return false
}

// Pause freezes the run.
func (g *Game) Pause() {
	if g.gameOver || g.paused {
		return
	}
	g.paused = true
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume continues the run and resets the tick clock.
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	g.lastTick = time.Now()
	g.emit(evResumed, nil)
}

// IsPaused reports whether the run is paused.
func (g *Game) IsPaused() bool { return g.paused }

// HandleCommand applies an externally triggered command.
func (g *Game) HandleCommand(cmd core.Command) error {
	switch cmd.Type {
	case cmdJump:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if !g.jump() {
			return fmt.Errorf("already in the air")
		}
		return nil
	case cmdStep:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		g.step()
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

// Commands lists the commands this game supports.
func (g *Game) Commands() []core.CommandSpec {
	return []core.CommandSpec{
		{Name: cmdJump, Description: "jump (ignored while in the air)"},
		{Name: cmdStep, Description: "advance the run one tick"},
		{Name: cmdPause, Description: "pause the game", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"reason": map[string]any{"type": "string"},
			},
		})},
		{Name: cmdResume, Description: "resume the game"},
		{Name: cmdReset, Description: "reset the game"},
	}
}

// jump launches the dino unless it is already in the air. Only the first
// press counts: repeated presses (a held key's auto-repeat, say) change
// nothing until it has landed.
func (g *Game) jump() bool {
	if g.airborne {
		return false
	}
	g.airborne = true
	g.vy = jumpVelocity
	g.emit(evJumped, jumpedPayload{Score: g.score})
	return true
}

// step advances the run by one tick: the dino moves through its arc, the
// desert scrolls one cell, a new obstacle may enter, and anything the dino
// overlaps ends the run.
func (g *Game) step() {
	g.distance++
	g.score = g.distance
	if g.score > g.best {
		g.best = g.score
	}
	g.period = tickPeriod(g.score)
	if g.flashTicks > 0 {
		g.flashTicks--
	}

	if g.airborne {
		g.alt += g.vy
		g.vy -= gravity
		if g.alt <= 0 {
			g.alt = 0
			g.vy = 0
			g.airborne = false
			g.emit(evLanded, nil)
		}
	}

	kept := g.obstacles[:0]
	for _, o := range g.obstacles {
		o.x--
		if o.x+shapes[o.kind].width <= 0 {
			continue
		}
		if !o.cleared && o.x+shapes[o.kind].width <= dinoX {
			o.cleared = true
			g.emit(evObstacleCleared, clearedPayload{Kind: shapes[o.kind].name, Score: g.score})
		}
		kept = append(kept, o)
	}
	g.obstacles = kept

	if g.distance >= g.nextSpawn {
		g.spawn()
	}

	if k, hit := g.collision(); hit {
		g.gameOver = true
		g.deathKind = shapes[k].name
		g.store.Submit(gameName, g.score)
		g.emit(evCollision, collisionPayload{Kind: g.deathKind})
		g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Reason: g.deathKind})
		return
	}

	if g.score%milestoneEvery == 0 {
		g.flashTicks = milestoneFlashTicks
		g.store.Submit(gameName, g.score)
		g.emit(evMilestone, milestonePayload{Score: g.score})
	}
}

// spawn puts a new obstacle just past the right edge and schedules the one
// after it.
func (g *Game) spawn() {
	k := g.pickKind()
	g.obstacles = append(g.obstacles, obstacle{kind: k, x: boardColumns})
	gap := minGap + g.rng.Intn(maxGap-minGap+1)
	g.nextSpawn = g.distance + shapes[k].width + gap
}

// pickKind draws the next obstacle: mostly cacti of assorted widths, with
// birds mixed in once the run is long enough.
func (g *Game) pickKind() kind {
	roll := g.rng.Intn(100)
	if g.score >= birdsFromScore {
		switch {
		case roll < 12:
			return birdLow
		case roll < 20:
			return birdHigh
		}
		roll = g.rng.Intn(100)
	}
	switch {
	case roll < 35:
		return cactusSmall
	case roll < 60:
		return cactusTall
	case roll < 80:
		return cactusPair
	default:
		return cactusCluster
	}
}

// altitude is the dino's height above standing in whole rows, as drawn and
// as collided with.
func (g *Game) altitude() int {
	return int(g.alt + 0.5)
}

// collision reports the first obstacle the dino overlaps.
func (g *Game) collision() (kind, bool) {
	alt := g.altitude()
	for _, o := range g.obstacles {
		for _, c := range shapes[o.kind].cells {
			x := o.x + c.dx
			if x < dinoX || x >= dinoX+dinoWidth {
				continue
			}
			if c.dy >= alt && c.dy < alt+dinoHeight {
				return o.kind, true
			}
		}
	}
	return 0, false
}

// tickPeriod is how long one tick lasts at the given score. The desert
// speeds up a little every hundred points and settles at a pace that stays
// playable with a single jump key.
func tickPeriod(score int) time.Duration {
	speedUps := score / milestoneEvery
	period := 80*time.Millisecond - time.Duration(speedUps)*2*time.Millisecond
	if period < 50*time.Millisecond {
		return 50 * time.Millisecond
	}
	return period
}
