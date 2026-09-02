// Package snake implements the classic Snake game for terminalika-core.
package snake

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
	boardColumns = 20
	boardRows    = 12

	gameName = "snake"

	scorePerFood = 10

	maxQueuedTurns = 3
)

// Command types.
const (
	cmdSetDirection = "snake.set_direction"
	cmdStep         = "snake.step"
	cmdPause        = "snake.pause"
	cmdResume       = "snake.resume"
	cmdReset        = "snake.reset"
)

// Event types.
const (
	evDirectionChanged = "snake.direction_changed"
	evMoved            = "snake.moved"
	evFoodEaten        = "snake.food_eaten"
	evCollision        = "snake.collision"
	evGameOver         = "snake.game_over"
	evPaused           = "game.paused"
	evResumed          = "game.resumed"
	evReset            = "game.reset"
)

// Point is a single board cell.
type Point struct {
	X int
	Y int
}

type direction int

const (
	dirUp direction = iota
	dirDown
	dirLeft
	dirRight
)

// Game holds the full Snake state. It implements core.Game.
type Game struct {
	snake   []Point
	dir     direction
	turns   []direction
	food    Point
	hasFood bool

	score       int
	best        int
	gameOver    bool
	paused      bool
	pauseReason string

	lastTick time.Time
	period   time.Duration

	store   *highscore.Store
	keys    core.GlobalKeys
	emitter core.Emitter
}

// Event payloads.
type cellPos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type directionChanged struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type movedPayload struct {
	Head cellPos `json:"head"`
}

type foodEatenPayload struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Score int `json:"score"`
	Level int `json:"level"`
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

type setDirectionPayload struct {
	Direction string `json:"direction"`
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

// New returns a freshly reset Snake game that persists best scores to the
// default location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a freshly reset Snake game using the given score store.
func NewWithStore(store *highscore.Store) *Game {
	g := &Game{store: store}
	g.Reset()
	return g
}

// Init hides the cursor and resets the game.
func (g *Game) Init(screen tcell.Screen) error {
	screen.HideCursor()
	g.Reset()
	return nil
}

// Reset clears the board and starts a fresh round.
func (g *Game) Reset() {
	cx := boardColumns / 2
	cy := boardRows / 2
	g.snake = []Point{
		{X: cx, Y: cy},
		{X: cx - 1, Y: cy},
		{X: cx - 2, Y: cy},
	}
	g.dir = dirRight
	g.turns = nil
	g.score = 0
	g.best = g.store.Best(gameName)
	g.gameOver = false
	g.paused = false
	g.pauseReason = ""
	g.lastTick = time.Now()
	g.period = tickPeriod(0)
	g.spawnFood()
	g.emit(evReset, nil)
}

// Update advances the snake one cell when the tick period has elapsed.
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

// HandleInput handles the game-specific keys: arrows/WASD change direction.
// SPACE, R and ESC are reserved for the launcher and are never claimed here.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.gameOver || g.paused {
		return false
	}

	switch ev.Key() {
	case tcell.KeyUp:
		g.setDirection(dirUp)
		return true
	case tcell.KeyDown:
		g.setDirection(dirDown)
		return true
	case tcell.KeyLeft:
		g.setDirection(dirLeft)
		return true
	case tcell.KeyRight:
		g.setDirection(dirRight)
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W':
			g.setDirection(dirUp)
			return true
		case 's', 'S':
			g.setDirection(dirDown)
			return true
		case 'a', 'A':
			g.setDirection(dirLeft)
			return true
		case 'd', 'D':
			g.setDirection(dirRight)
			return true
		}
	}
	return false
}

// Pause stops the snake.
func (g *Game) Pause() {
	if g.gameOver || g.paused {
		return
	}
	g.paused = true
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume continues the game and resets the tick clock.
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	g.lastTick = time.Now()
	g.emit(evResumed, nil)
}

// IsPaused reports whether the snake is paused.
func (g *Game) IsPaused() bool { return g.paused }

// HandleCommand applies an externally triggered command.
func (g *Game) HandleCommand(cmd core.Command) error {
	switch cmd.Type {
	case cmdSetDirection:
		var p setDirectionPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		d, err := parseDirection(p.Direction)
		if err != nil {
			return err
		}
		if !g.setDirection(d) {
			return fmt.Errorf("cannot turn %s", p.Direction)
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
		{Name: cmdSetDirection, Description: "queue a direction change", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}},
			},
			"required": []string{"direction"},
		})},
		{Name: cmdStep, Description: "advance the snake one tick"},
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

// turn queues a direction change. Reversals and no-ops are ignored, and a
// small queue lets quick successive keypresses all register instead of being
// dropped.
func (g *Game) turn(d direction) bool {
	if len(g.turns) > 0 {
		last := g.turns[len(g.turns)-1]
		if d == last || d == opposite(last) {
			return false
		}
	} else if d == g.dir || d == opposite(g.dir) {
		return false
	}

	if len(g.turns) >= maxQueuedTurns {
		return false
	}
	g.turns = append(g.turns, d)
	return true
}

// setDirection queues a direction change and emits an event on success.
func (g *Game) setDirection(d direction) bool {
	from := g.pendingDirection()
	if !g.turn(d) {
		return false
	}
	g.emit(evDirectionChanged, directionChanged{From: dirName(from), To: dirName(d)})
	return true
}

// pendingDirection returns the direction the snake will move on the next tick
// once any queued turns are applied.
func (g *Game) pendingDirection() direction {
	if len(g.turns) > 0 {
		return g.turns[len(g.turns)-1]
	}
	return g.dir
}

func (g *Game) step() {
	if len(g.turns) > 0 {
		g.dir = g.turns[0]
		g.turns = g.turns[1:]
	}

	head := g.snake[0]
	next := Point{X: head.X, Y: head.Y}
	switch g.dir {
	case dirUp:
		next.Y--
	case dirDown:
		next.Y++
	case dirLeft:
		next.X--
	case dirRight:
		next.X++
	}

	if g.hitsWall(next) {
		g.gameOver = true
		g.emit(evCollision, collisionPayload{Kind: "wall"})
		g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Reason: "wall"})
		return
	}
	if g.hitsSelf(next) {
		g.gameOver = true
		g.emit(evCollision, collisionPayload{Kind: "self"})
		g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Reason: "self"})
		return
	}

	// Move by prepending the new head.
	g.snake = append([]Point{next}, g.snake...)

	if g.hasFood && next == g.food {
		// Eat: keep the tail so the snake grows, then spawn new food.
		g.score += scorePerFood
		if g.score > g.best {
			g.best = g.score
			g.store.Submit(gameName, g.score)
		}
		g.period = tickPeriod(g.score)
		g.emit(evFoodEaten, foodEatenPayload{X: next.X, Y: next.Y, Score: g.score, Level: g.score / scorePerFood})
		g.spawnFood()
		return
	}

	// No food: drop the tail.
	g.snake = g.snake[:len(g.snake)-1]
	g.emit(evMoved, movedPayload{Head: cellPos{X: next.X, Y: next.Y}})
}

func (g *Game) hitsWall(p Point) bool {
	return p.X < 0 || p.X >= boardColumns || p.Y < 0 || p.Y >= boardRows
}

func (g *Game) hitsSelf(next Point) bool {
	// The tail moves away this tick, so it can only be hit when the snake is
	// about to grow (the next cell is food and the tail stays put).
	limit := len(g.snake)
	if !(g.hasFood && next == g.food) {
		limit--
	}
	for i := 0; i < limit; i++ {
		if g.snake[i] == next {
			return true
		}
	}
	return false
}

func (g *Game) spawnFood() {
	occupied := make(map[Point]bool, len(g.snake))
	for _, p := range g.snake {
		occupied[p] = true
	}

	free := make([]Point, 0, boardColumns*boardRows)
	for y := 0; y < boardRows; y++ {
		for x := 0; x < boardColumns; x++ {
			p := Point{X: x, Y: y}
			if !occupied[p] {
				free = append(free, p)
			}
		}
	}

	if len(free) == 0 {
		// The board is full: the player has won.
		g.hasFood = false
		g.gameOver = true
		g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Reason: "won"})
		return
	}

	g.food = free[rand.Intn(len(free))]
	g.hasFood = true
}

func opposite(d direction) direction {
	switch d {
	case dirUp:
		return dirDown
	case dirDown:
		return dirUp
	case dirLeft:
		return dirRight
	case dirRight:
		return dirLeft
	}
	return d
}

func dirName(d direction) string {
	switch d {
	case dirUp:
		return "up"
	case dirDown:
		return "down"
	case dirLeft:
		return "left"
	case dirRight:
		return "right"
	}
	return ""
}

func parseDirection(s string) (direction, error) {
	switch s {
	case "up":
		return dirUp, nil
	case "down":
		return dirDown, nil
	case "left":
		return dirLeft, nil
	case "right":
		return dirRight, nil
	default:
		return 0, fmt.Errorf("invalid direction %q", s)
	}
}

func tickPeriod(score int) time.Duration {
	// One level is one eaten food (scorePerFood points). Speed increases
	// every 10 levels instead of on every level, and gently: this is a
	// two-minute break for someone waiting on an agent, not a challenge.
	level := score / scorePerFood
	speedUps := (level - 1) / 10

	period := 220*time.Millisecond - time.Duration(speedUps)*3*time.Millisecond
	if period < 180*time.Millisecond {
		return 180 * time.Millisecond
	}
	return period
}
