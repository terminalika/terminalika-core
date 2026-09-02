// Package g2048 implements 2048 for terminalika-core: slide the tiles, equal
// neighbours merge, a new 2 (or 4) drops into a free cell after every move,
// and the run ends when nothing can move. There is no clock: the board only
// changes when the player presses a key, so it waits as long as it has to.
package g2048

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
	size = 4

	gameName = "2048"

	// winTile is the tile that turns the run into a win. Play continues
	// past it, like the original; the status line just says so.
	winTile = 2048

	// fourChance is the chance, out of a hundred, that a spawned tile is a 4
	// rather than a 2.
	fourChance = 10

	// slideDuration is how long the tiles take to glide to their new cells
	// after a move. The merged values and the spawned tile show up when it
	// ends; a key pressed before that cuts it short and moves on.
	slideDuration = 80 * time.Millisecond
)

// tileMove is one tile's trip during a slide: where it was, where it ends
// up, and the value it carried on the way (merges show their result only
// once the slide is over).
type tileMove struct {
	value        int
	fromX, fromY int
	toX, toY     int
}

// Command types.
const (
	cmdMove   = "2048.move"
	cmdPause  = "2048.pause"
	cmdResume = "2048.resume"
	cmdReset  = "2048.reset"
)

// Event types.
const (
	evMoved    = "2048.moved"
	evWon      = "2048.won"
	evGameOver = "2048.game_over"
	evPaused   = "game.paused"
	evResumed  = "game.resumed"
	evReset    = "game.reset"
)

type direction int

const (
	dirUp direction = iota
	dirDown
	dirLeft
	dirRight
)

// Game holds the full 2048 state. It implements core.Game.
type Game struct {
	// board[y][x] is the tile's value, 0 for an empty cell.
	board [size][size]int

	score       int
	best        int
	won         bool
	gameOver    bool
	paused      bool
	pauseReason string

	// The slide in progress: the trips of every tile that was on the board
	// before the move, where the spawned tile will appear, and when the
	// slide started. Empty when nothing is moving.
	sliding    []tileMove
	spawned    tilePos
	slideStart time.Time

	rng     *rand.Rand
	store   *highscore.Store
	keys    core.GlobalKeys
	emitter core.Emitter
}

// Event payloads.
type tilePos struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Value int `json:"value"`
}

type movedPayload struct {
	Direction string  `json:"direction"`
	Gained    int     `json:"gained"`
	Score     int     `json:"score"`
	Spawned   tilePos `json:"spawned"`
}

type wonPayload struct {
	Score int `json:"score"`
}

type gameOverPayload struct {
	Score   int  `json:"score"`
	Best    int  `json:"best"`
	Largest int  `json:"largest"`
	Won     bool `json:"won"`
}

type pausedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type movePayload struct {
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

// New returns a fresh 2048 game that persists best scores to the default
// location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a fresh 2048 game using the given score store.
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

// Reset clears the board and drops the two opening tiles.
func (g *Game) Reset() {
	g.board = [size][size]int{}
	g.score = 0
	g.best = g.store.Best(gameName)
	g.won = false
	g.gameOver = false
	g.paused = false
	g.pauseReason = ""
	g.sliding = nil
	g.spawn()
	g.spawn()
	g.emit(evReset, nil)
}

// Update ends a finished slide. The board itself has no clock: nothing
// here changes the game, only what Draw shows.
func (g *Game) Update() {
	if g.sliding != nil && time.Since(g.slideStart) >= slideDuration {
		g.sliding = nil
	}
}

// slideProgress is how far the current slide has come, 0 to 1; 1 when
// nothing is sliding.
func (g *Game) slideProgress() float64 {
	if g.sliding == nil {
		return 1
	}
	t := float64(time.Since(g.slideStart)) / float64(slideDuration)
	if t >= 1 {
		return 1
	}
	return t
}

// HandleInput handles the game-specific keys: arrows, WASD and HJKL slide
// the board. SPACE, R and ESC are reserved for the launcher and are never
// claimed here.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.gameOver || g.paused {
		return false
	}

	switch ev.Key() {
	case tcell.KeyUp:
		g.move(dirUp)
		return true
	case tcell.KeyDown:
		g.move(dirDown)
		return true
	case tcell.KeyLeft:
		g.move(dirLeft)
		return true
	case tcell.KeyRight:
		g.move(dirRight)
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W', 'k', 'K':
			g.move(dirUp)
			return true
		case 's', 'S', 'j', 'J':
			g.move(dirDown)
			return true
		case 'a', 'A', 'h', 'H':
			g.move(dirLeft)
			return true
		case 'd', 'D', 'l', 'L':
			g.move(dirRight)
			return true
		}
	}
	return false
}

// Pause blocks input. Nothing moves on its own, so this only exists for the
// launcher's notices and the band they sit on.
func (g *Game) Pause() {
	if g.gameOver || g.paused {
		return
	}
	g.paused = true
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume accepts input again.
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	g.emit(evResumed, nil)
}

// IsPaused reports whether input is blocked.
func (g *Game) IsPaused() bool { return g.paused }

// HandleCommand applies an externally triggered command.
func (g *Game) HandleCommand(cmd core.Command) error {
	switch cmd.Type {
	case cmdMove:
		var p movePayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		d, err := parseDirection(p.Direction)
		if err != nil {
			return err
		}
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if !g.move(d) {
			return fmt.Errorf("nothing moves %s", p.Direction)
		}
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
		{Name: cmdMove, Description: "slide the board (rejected when nothing moves)", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}},
			},
			"required": []string{"direction"},
		})},
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

// move slides every line of the board towards d, merging equal neighbours
// once per move. A move that changes nothing is not a move: no tile spawns
// and it reports false. Otherwise a tile spawns, the score grows by the
// merged values, and the run ends if the new board is stuck.
func (g *Game) move(d direction) bool {
	gained, moved, trips := g.slide(d)
	if !moved {
		return false
	}

	g.score += gained
	if g.score > g.best {
		g.best = g.score
		g.store.Submit(gameName, g.score)
	}
	spawned := g.spawn()
	g.sliding = trips
	g.spawned = spawned
	g.slideStart = time.Now()
	g.emit(evMoved, movedPayload{Direction: dirName(d), Gained: gained, Score: g.score, Spawned: spawned})

	if !g.won && g.largest() >= winTile {
		g.won = true
		g.emit(evWon, wonPayload{Score: g.score})
	}
	if !g.canMove() {
		g.gameOver = true
		g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Largest: g.largest(), Won: g.won})
	}
	return true
}

// slide applies d to the board in place and reports the points merged,
// whether anything changed, and every tile's trip - the ones that stayed
// put included, so Draw can paint the whole pre-move board in motion.
func (g *Game) slide(d direction) (gained int, moved bool, trips []tileMove) {
	for i := 0; i < size; i++ {
		line := g.line(d, i)
		merged, points, changed, dest := mergeLine(line)
		if changed {
			g.setLine(d, i, merged)
			moved = true
		}
		gained += points
		for j, v := range line {
			if v == 0 {
				continue
			}
			fx, fy := lineCell(d, i, j)
			tx, ty := lineCell(d, i, dest[j])
			trips = append(trips, tileMove{value: v, fromX: fx, fromY: fy, toX: tx, toY: ty})
		}
	}
	return gained, moved, trips
}

// line reads the i-th row or column in sliding order: its first element is
// the cell the tiles move towards.
func (g *Game) line(d direction, i int) [size]int {
	var out [size]int
	for j := 0; j < size; j++ {
		x, y := lineCell(d, i, j)
		out[j] = g.board[y][x]
	}
	return out
}

func (g *Game) setLine(d direction, i int, line [size]int) {
	for j := 0; j < size; j++ {
		x, y := lineCell(d, i, j)
		g.board[y][x] = line[j]
	}
}

// lineCell maps the j-th cell of the i-th line for direction d to board
// coordinates, so that j = 0 is the edge tiles slide into.
func lineCell(d direction, i, j int) (x, y int) {
	switch d {
	case dirLeft:
		return j, i
	case dirRight:
		return size - 1 - j, i
	case dirUp:
		return i, j
	default: // dirDown
		return i, size - 1 - j
	}
}

// mergeLine slides a line towards index 0: gaps close, then each pair of
// equal neighbours becomes one tile of twice the value, once per tile and
// nearest-the-edge first - 2 2 2 2 turns into 4 4, not 8, and 4 2 2 into
// 4 4, not 8. dest says where each input tile ended up (both halves of a
// merge land on the same slot; empty inputs map to themselves).
func mergeLine(in [size]int) (out [size]int, points int, changed bool, dest [size]int) {
	var merged [size]bool
	n := 0
	for j, v := range in {
		if v == 0 {
			dest[j] = j
			continue
		}
		if n > 0 && out[n-1] == v && !merged[n-1] {
			out[n-1] = v * 2
			merged[n-1] = true
			points += v * 2
			dest[j] = n - 1
			continue
		}
		out[n] = v
		dest[j] = n
		n++
	}
	return out, points, out != in, dest
}

// spawn drops a 2 (or, one time in ten, a 4) into a random empty cell and
// reports where. A full board spawns nothing.
func (g *Game) spawn() tilePos {
	var free []tilePos
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if g.board[y][x] == 0 {
				free = append(free, tilePos{X: x, Y: y})
			}
		}
	}
	if len(free) == 0 {
		return tilePos{X: -1, Y: -1}
	}
	t := free[g.rng.Intn(len(free))]
	t.Value = 2
	if g.rng.Intn(100) < fourChance {
		t.Value = 4
	}
	g.board[t.Y][t.X] = t.Value
	return t
}

// canMove reports whether any slide would change the board: an empty cell,
// or two equal neighbours.
func (g *Game) canMove() bool {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			v := g.board[y][x]
			if v == 0 {
				return true
			}
			if x+1 < size && g.board[y][x+1] == v {
				return true
			}
			if y+1 < size && g.board[y+1][x] == v {
				return true
			}
		}
	}
	return false
}

// largest is the biggest tile on the board.
func (g *Game) largest() int {
	max := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if g.board[y][x] > max {
				max = g.board[y][x]
			}
		}
	}
	return max
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
