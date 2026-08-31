// Package tetris implements a minimal Tetris game for terminalika-core.
package tetris

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
	boardColumns = 10
	boardRows    = 20

	gameName = "tetris"
)

// Command types.
const (
	cmdMove     = "tetris.move"
	cmdRotate   = "tetris.rotate"
	cmdHardDrop = "tetris.hard_drop"
	cmdTick     = "tetris.tick"
	cmdPause    = "tetris.pause"
	cmdResume   = "tetris.resume"
	cmdReset    = "tetris.reset"
)

// Event types.
const (
	evPieceSpawned = "tetris.piece_spawned"
	evPieceMoved   = "tetris.piece_moved"
	evPieceRotated = "tetris.piece_rotated"
	evPieceLocked  = "tetris.piece_locked"
	evLineCleared  = "tetris.line_cleared"
	evGameOver     = "tetris.game_over"
	evPaused       = "game.paused"
	evResumed      = "game.resumed"
	evReset        = "game.reset"
)

// Point is a single board cell.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// PieceKind identifies one of the seven tetrominoes.
type PieceKind int

const (
	PieceI PieceKind = iota
	PieceO
	PieceT
	PieceS
	PieceZ
	PieceJ
	PieceL
)

type pieceDef struct {
	color     tcell.Color
	box       int
	rotations [4][]Point
}

func newPieceDef(color tcell.Color, box int, base []Point) pieceDef {
	d := pieceDef{color: color, box: box}
	cells := base
	for i := 0; i < 4; i++ {
		d.rotations[i] = cells
		cells = rotate(cells, box)
	}
	return d
}

// rotate rotates cells 90 degrees clockwise inside a box x box grid.
func rotate(cells []Point, box int) []Point {
	out := make([]Point, len(cells))
	for i, c := range cells {
		out[i] = Point{X: box - 1 - c.Y, Y: c.X}
	}
	return out
}

var defs = [7]pieceDef{
	PieceI: newPieceDef(tcell.ColorAqua, 4, []Point{{0, 1}, {1, 1}, {2, 1}, {3, 1}}),
	PieceO: newPieceDef(tcell.ColorYellow, 2, []Point{{0, 0}, {1, 0}, {0, 1}, {1, 1}}),
	PieceT: newPieceDef(tcell.ColorPurple, 3, []Point{{1, 0}, {0, 1}, {1, 1}, {2, 1}}),
	PieceS: newPieceDef(tcell.ColorGreen, 3, []Point{{1, 0}, {2, 0}, {0, 1}, {1, 1}}),
	PieceZ: newPieceDef(tcell.ColorRed, 3, []Point{{0, 0}, {1, 0}, {1, 1}, {2, 1}}),
	PieceJ: newPieceDef(tcell.ColorBlue, 3, []Point{{0, 0}, {0, 1}, {1, 1}, {2, 1}}),
	PieceL: newPieceDef(tcell.ColorOrange, 3, []Point{{2, 0}, {0, 1}, {1, 1}, {2, 1}}),
}

var lineScores = [5]int{0, 100, 300, 500, 800}

type cell struct {
	filled bool
	color  tcell.Color
}

type piece struct {
	kind PieceKind
	x    int
	y    int
	rot  int
}

func (p *piece) cells() []Point {
	d := defs[p.kind]
	cells := make([]Point, len(d.rotations[p.rot]))
	for i, b := range d.rotations[p.rot] {
		cells[i] = Point{X: p.x + b.X, Y: p.y + b.Y}
	}
	return cells
}

// Game holds the full Tetris state. It implements core.Game.
type Game struct {
	board       [boardRows][boardColumns]cell
	current     *piece
	gameOver    bool
	paused      bool
	pauseReason string
	score       int
	best        int
	lines       int

	lastTick time.Time
	period   time.Duration

	store *highscore.Store
	keys  core.GlobalKeys
	// The PAUSED / GAME OVER band the last Draw painted, for OverlayArea.
	overlay   core.Rect
	overlayOn bool
	emitter   core.Emitter
}

// Event payloads.
type pieceMoved struct {
	DX int `json:"dx"`
	DY int `json:"dy"`
}

type pieceRotated struct {
	Rot int `json:"rotation"`
}

type pieceLocked struct {
	Cells []Point `json:"cells"`
}

type pieceSpawned struct {
	Kind string `json:"kind"`
}

type lineCleared struct {
	Count  int `json:"count"`
	Points int `json:"points"`
}

type gameOverPayload struct {
	Score int `json:"score"`
	Best  int `json:"best"`
}

type movePayload struct {
	DX int `json:"dx"`
	DY int `json:"dy"`
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

func kindName(k PieceKind) string {
	switch k {
	case PieceI:
		return "I"
	case PieceO:
		return "O"
	case PieceT:
		return "T"
	case PieceS:
		return "S"
	case PieceZ:
		return "Z"
	case PieceJ:
		return "J"
	case PieceL:
		return "L"
	}
	return ""
}

// New returns a freshly reset Tetris game that persists best scores to the
// default location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a freshly reset Tetris game using the given score store.
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
	for y := range g.board {
		for x := range g.board[y] {
			g.board[y][x] = cell{}
		}
	}

	g.current = nil
	g.gameOver = false
	g.paused = false
	g.pauseReason = ""
	g.score = 0
	g.best = g.store.Best(gameName)
	g.lines = 0
	g.lastTick = time.Now()
	g.period = dropPeriod(0)
	g.spawn()
	g.emit(evReset, nil)
}

// Update drops the active piece one row when the gravity period has elapsed.
func (g *Game) Update() {
	if g.paused || g.gameOver {
		return
	}
	if time.Since(g.lastTick) < g.period {
		return
	}

	if !g.move(0, 1) {
		g.lock()
	}
	g.lastTick = time.Now()
}

// HandleInput handles the game-specific keys: arrows/WASD for movement and
// rotation, and X for a hard drop. SPACE, R and ESC are reserved for the
// launcher and are never claimed here.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.gameOver || g.paused {
		return false
	}

	switch ev.Key() {
	case tcell.KeyLeft:
		g.moveAndEmit(-1, 0)
		return true
	case tcell.KeyRight:
		g.moveAndEmit(1, 0)
		return true
	case tcell.KeyDown:
		g.moveAndEmit(0, 1)
		return true
	case tcell.KeyUp:
		g.rotateAndEmit()
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'a', 'A':
			g.moveAndEmit(-1, 0)
			return true
		case 'd', 'D':
			g.moveAndEmit(1, 0)
			return true
		case 's', 'S':
			g.moveAndEmit(0, 1)
			return true
		case 'w', 'W':
			g.rotateAndEmit()
			return true
		case 'x', 'X':
			g.hardDrop()
			return true
		}
	}
	return false
}

// Pause stops gravity.
func (g *Game) Pause() {
	if g.gameOver || g.paused {
		return
	}
	g.paused = true
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume continues the game and resets the gravity clock.
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	g.lastTick = time.Now()
	g.emit(evResumed, nil)
}

// IsPaused reports whether the game is paused.
func (g *Game) IsPaused() bool { return g.paused }

// HandleCommand applies an externally triggered command.
func (g *Game) HandleCommand(cmd core.Command) error {
	switch cmd.Type {
	case cmdMove:
		var p movePayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		if p.DX < -1 || p.DX > 1 || p.DY < 0 || p.DY > 1 {
			return fmt.Errorf("invalid move (%d,%d)", p.DX, p.DY)
		}
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if !g.moveAndEmit(p.DX, p.DY) {
			return fmt.Errorf("cannot move (%d,%d)", p.DX, p.DY)
		}
		return nil
	case cmdRotate:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if !g.rotateAndEmit() {
			return fmt.Errorf("cannot rotate")
		}
		return nil
	case cmdHardDrop:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		g.hardDrop()
		return nil
	case cmdTick:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if !g.move(0, 1) {
			g.lock()
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
		{Name: cmdMove, Description: "move the active piece", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dx": map[string]any{"type": "integer"},
				"dy": map[string]any{"type": "integer"},
			},
			"required": []string{"dx", "dy"},
		})},
		{Name: cmdRotate, Description: "rotate the active piece"},
		{Name: cmdHardDrop, Description: "drop the active piece instantly"},
		{Name: cmdTick, Description: "apply one gravity tick"},
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

func (g *Game) move(dx, dy int) bool {
	if g.current == nil {
		return false
	}

	p := *g.current
	p.x += dx
	p.y += dy

	cells := p.cells()
	if g.collides(cells) {
		return false
	}

	g.current = &p
	return true
}

// moveAndEmit moves the piece and emits an event on success.
func (g *Game) moveAndEmit(dx, dy int) bool {
	if !g.move(dx, dy) {
		return false
	}
	g.emit(evPieceMoved, pieceMoved{DX: dx, DY: dy})
	return true
}

// rotateAndEmit rotates the piece and emits an event on success.
func (g *Game) rotateAndEmit() bool {
	if !g.rotate() {
		return false
	}
	g.emit(evPieceRotated, pieceRotated{Rot: g.current.rot})
	return true
}

func (g *Game) rotate() bool {
	if g.current == nil {
		return false
	}

	p := *g.current
	p.rot = (p.rot + 1) % 4

	if g.collides(p.cells()) {
		// Minimal wall kicks: nudge left/right before giving up.
		for _, dx := range []int{-1, 1, -2, 2} {
			kicked := p
			kicked.x += dx
			if !g.collides(kicked.cells()) {
				g.current = &kicked
				return true
			}
		}
		return false
	}

	g.current = &p
	return true
}

func (g *Game) hardDrop() {
	if g.current == nil {
		return
	}
	for g.move(0, 1) {
	}
	g.lock()
}

func (g *Game) collides(cells []Point) bool {
	for _, c := range cells {
		if c.X < 0 || c.X >= boardColumns || c.Y < 0 || c.Y >= boardRows {
			return true
		}
		if g.board[c.Y][c.X].filled {
			return true
		}
	}
	return false
}

func (g *Game) lock() {
	if g.current == nil {
		return
	}

	color := defs[g.current.kind].color
	cells := g.current.cells()
	for _, c := range cells {
		if c.Y < 0 {
			// The piece locked entirely above the visible board.
			g.gameOver = true
			g.current = nil
			g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best})
			return
		}
		g.board[c.Y][c.X] = cell{filled: true, color: color}
	}

	g.emit(evPieceLocked, pieceLocked{Cells: cells})
	g.current = nil
	g.clearLines()
	g.spawn()
}

func (g *Game) clearLines() {
	cleared := 0
	for y := boardRows - 1; y >= 0; {
		full := true
		for x := 0; x < boardColumns; x++ {
			if !g.board[y][x].filled {
				full = false
				break
			}
		}

		if !full {
			y--
			continue
		}

		for yy := y; yy > 0; yy-- {
			g.board[yy] = g.board[yy-1]
		}
		g.board[0] = [boardColumns]cell{}
		cleared++
	}

	if cleared == 0 {
		return
	}

	g.score += lineScores[cleared]
	if g.score > g.best {
		g.best = g.score
		g.store.Submit(gameName, g.score)
	}
	g.lines += cleared
	g.period = dropPeriod(g.lines)
	g.emit(evLineCleared, lineCleared{Count: cleared, Points: lineScores[cleared]})
}

func (g *Game) spawn() {
	kind := PieceKind(rand.Intn(len(defs)))
	def := defs[kind]

	p := &piece{
		kind: kind,
		x:    (boardColumns - def.box) / 2,
		y:    0,
	}

	if g.collides(p.cells()) {
		g.gameOver = true
		g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best})
		return
	}

	g.current = p
	g.emit(evPieceSpawned, pieceSpawned{Kind: kindName(kind)})
}

// dropPeriod is the gravity interval: it tightens a little every ten lines,
// gently - this is a two-minute break for someone waiting on an agent, not
// a challenge - and never past the floor.
func dropPeriod(lines int) time.Duration {
	period := 500*time.Millisecond - time.Duration(lines/10)*17*time.Millisecond
	if period < 350*time.Millisecond {
		return 350 * time.Millisecond
	}
	return period
}
