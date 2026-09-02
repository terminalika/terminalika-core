// Package mines implements Minesweeper for terminalika-core: a field of
// mines at one of three sizes, a cursor to walk it, one key to reveal and
// one to flag. The first reveal is always safe. Nothing moves on its own -
// the clock only times the run - so the field waits as long as the player
// does.
package mines

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/highscore"
)

const (
	gameName = "mines"

	// Scoring: every safe cell revealed, and a bonus for clearing the field
	// that shrinks with the seconds it took.
	pointsPerCell = 10
	clearBonus    = 500
	timeBonusMax  = 300

	// Animation pacing. A flood reveal shows its cells ring by ring from
	// the cell that was opened; a hit shows the mines one by one, nearest
	// first; a clear plants the remaining flags one by one.
	cascadeStep   = 25 * time.Millisecond
	explodeDelay  = 60 * time.Millisecond
	explodeStep   = 40 * time.Millisecond
	plantFlagStep = 50 * time.Millisecond
)

// Level is a field size and mine count.
type Level struct {
	Name       string
	Cols, Rows int
	Mines      int
	scoreName  string // the highscore entry; the first level owns the plain game name
}

// Levels are the classic three; the expert field is the widest that still
// fits an 80-column terminal at two cells per square. The game opens on a
// picker listing them.
var Levels = []Level{
	{Name: "beginner", Cols: 9, Rows: 9, Mines: 10, scoreName: gameName},
	{Name: "intermediate", Cols: 16, Rows: 16, Mines: 40, scoreName: gameName + "-intermediate"},
	{Name: "expert", Cols: 30, Rows: 16, Mines: 99, scoreName: gameName + "-expert"},
}

// Command types.
const (
	cmdMove   = "mines.move"
	cmdCursor = "mines.cursor"
	cmdReveal = "mines.reveal"
	cmdFlag   = "mines.flag"
	cmdLevel  = "mines.level"
	cmdPause  = "mines.pause"
	cmdResume = "mines.resume"
	cmdReset  = "mines.reset"
)

// Event types.
const (
	evRevealed = "mines.revealed"
	evFlagged  = "mines.flagged"
	evExploded = "mines.exploded"
	evCleared  = "mines.cleared"
	evGameOver = "mines.game_over"
	evLevel    = "mines.level_changed"
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

// cell is one square of the field. revealed and flagged are the truth the
// game plays by; showAt is when Draw starts showing that truth, so a flood
// reveal can ripple outwards and a hit can go off mine by mine.
type cell struct {
	mine     bool
	revealed bool
	flagged  bool
	adjacent int
	exploded bool
	showAt   time.Time
}

// Game holds the full Minesweeper state. It implements core.Game.
type Game struct {
	// choosing is the picker the game opens on: level is the highlighted
	// entry until Enter starts a field of that size.
	choosing bool
	level    Level
	cells    [][]cell // cells[y][x]
	placed   bool     // mines are placed on the first reveal, around it
	cx, cy   int      // cursor

	revealedCount int
	flagCount     int
	score         int
	best          int
	won           bool
	gameOver      bool
	paused        bool
	pauseReason   string

	// The run's clock: elapsed before the current stretch, and when that
	// stretch started (zero while stopped - before the first reveal, while
	// paused, after the end).
	elapsed      time.Duration
	runningSince time.Time

	rng     *rand.Rand
	store   *highscore.Store
	keys    core.GlobalKeys
	emitter core.Emitter
}

// Event payloads.
type cellPos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type revealedPayload struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Cells int `json:"cells"`
	Score int `json:"score"`
}

type flaggedPayload struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Flagged bool `json:"flagged"`
	Flags   int  `json:"flags"`
}

type clearedPayload struct {
	Score   int     `json:"score"`
	Seconds float64 `json:"seconds"`
}

type gameOverPayload struct {
	Score int  `json:"score"`
	Best  int  `json:"best"`
	Won   bool `json:"won"`
}

type levelPayload struct {
	Level string `json:"level"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	Mines int    `json:"mines"`
}

type pausedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type movePayload struct {
	Direction string `json:"direction"`
}

// cellPayload names a cell; a command without one acts on the cursor.
type cellPayload struct {
	X *int `json:"x,omitempty"`
	Y *int `json:"y,omitempty"`
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

// New returns a fresh beginner field that persists best scores to the
// default location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a fresh beginner field using the given score store.
func NewWithStore(store *highscore.Store) *Game {
	g := &Game{level: Levels[0], store: store, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	g.Reset()
	return g
}

// Init hides the cursor and resets the game.
func (g *Game) Init(screen tcell.Screen) error {
	screen.HideCursor()
	g.Reset()
	return nil
}

// Reset goes back to the picker, with the last level highlighted.
func (g *Game) Reset() {
	g.choosing = true
	g.clear()
	g.emit(evReset, nil)
}

// clear covers the field of the current level; the mines are placed on the
// first reveal.
func (g *Game) clear() {
	g.cells = make([][]cell, g.level.Rows)
	for y := range g.cells {
		g.cells[y] = make([]cell, g.level.Cols)
	}
	g.placed = false
	g.cx, g.cy = g.level.Cols/2, g.level.Rows/2
	g.revealedCount = 0
	g.flagCount = 0
	g.score = 0
	g.best = g.store.Best(g.level.scoreName)
	g.won = false
	g.gameOver = false
	g.paused = false
	g.pauseReason = ""
	g.elapsed = 0
	g.runningSince = time.Time{}
}

// start leaves the picker and opens a fresh field of the given level.
func (g *Game) start(l Level) {
	g.level = l
	g.choosing = false
	g.clear()
	g.emit(evLevel, levelPayload{Level: l.Name, Cols: l.Cols, Rows: l.Rows, Mines: l.Mines})
}

// levelIndex is the position of the current level in Levels.
func (g *Game) levelIndex() int {
	for i, l := range Levels {
		if l.Name == g.level.Name {
			return i
		}
	}
	return 0
}

// pick moves the picker's highlight by delta, staying within Levels.
func (g *Game) pick(delta int) bool {
	i := g.levelIndex() + delta
	if i < 0 || i >= len(Levels) {
		return false
	}
	g.level = Levels[i]
	g.best = g.store.Best(g.level.scoreName)
	return true
}

// safeCells is how many cells have to be revealed to clear the field.
func (g *Game) safeCells() int {
	return g.level.Cols*g.level.Rows - g.level.Mines
}

// Update does nothing: the field has no clock of its own, and the run's
// timer is read when drawn.
func (g *Game) Update() {}

// HandleInput handles the game-specific keys. On the picker, up/down move
// the highlight and Enter starts. On the field, arrows, WASD and HJKL move
// the cursor, Enter and X reveal, F flags. SPACE, R and ESC are reserved
// for the launcher and are never claimed here.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.paused || g.gameOver {
		return false
	}
	if g.choosing {
		return g.handlePickerKey(ev)
	}

	switch ev.Key() {
	case tcell.KeyUp:
		g.moveCursor(dirUp)
		return true
	case tcell.KeyDown:
		g.moveCursor(dirDown)
		return true
	case tcell.KeyLeft:
		g.moveCursor(dirLeft)
		return true
	case tcell.KeyRight:
		g.moveCursor(dirRight)
		return true
	case tcell.KeyEnter:
		g.reveal(g.cx, g.cy)
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W', 'k', 'K':
			g.moveCursor(dirUp)
			return true
		case 's', 'S', 'j', 'J':
			g.moveCursor(dirDown)
			return true
		case 'a', 'A', 'h', 'H':
			g.moveCursor(dirLeft)
			return true
		case 'd', 'D', 'l', 'L':
			g.moveCursor(dirRight)
			return true
		case 'x', 'X':
			g.reveal(g.cx, g.cy)
			return true
		case 'f', 'F':
			g.flag(g.cx, g.cy)
			return true
		}
	}
	return false
}

// handlePickerKey moves the picker's highlight or starts the field.
func (g *Game) handlePickerKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyUp, tcell.KeyLeft:
		g.pick(-1)
		return true
	case tcell.KeyDown, tcell.KeyRight:
		g.pick(1)
		return true
	case tcell.KeyEnter:
		g.start(g.level)
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'w', 'W', 'k', 'K', 'a', 'A', 'h', 'H':
			g.pick(-1)
			return true
		case 's', 'S', 'j', 'J', 'd', 'D', 'l', 'L':
			g.pick(1)
			return true
		case 'x', 'X':
			g.start(g.level)
			return true
		}
	}
	return false
}

// Pause blocks input and stops the clock.
func (g *Game) Pause() {
	if g.gameOver || g.paused {
		return
	}
	g.paused = true
	g.stopClock()
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume accepts input again and restarts the clock if the run had begun.
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	if g.placed && !g.gameOver {
		g.runningSince = time.Now()
	}
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
		if !g.running() {
			return fmt.Errorf("game is not running")
		}
		if !g.moveCursor(d) {
			return fmt.Errorf("cursor is at the edge")
		}
		return nil
	case cmdCursor:
		x, y, err := g.cellFrom(cmd.Payload, false)
		if err != nil {
			return err
		}
		if !g.running() {
			return fmt.Errorf("game is not running")
		}
		g.cx, g.cy = x, y
		return nil
	case cmdReveal:
		x, y, err := g.cellFrom(cmd.Payload, true)
		if err != nil {
			return err
		}
		if !g.running() {
			return fmt.Errorf("game is not running")
		}
		if !g.reveal(x, y) {
			return fmt.Errorf("nothing to reveal at %d,%d", x, y)
		}
		return nil
	case cmdFlag:
		x, y, err := g.cellFrom(cmd.Payload, true)
		if err != nil {
			return err
		}
		if !g.running() {
			return fmt.Errorf("game is not running")
		}
		if !g.flag(x, y) {
			return fmt.Errorf("cell %d,%d is already revealed", x, y)
		}
		return nil
	case cmdLevel:
		var p levelPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return fmt.Errorf("invalid payload: %w", err)
		}
		for _, l := range Levels {
			if l.Name == p.Level {
				if g.paused {
					return fmt.Errorf("game is paused")
				}
				g.start(l)
				return nil
			}
		}
		return fmt.Errorf("unknown level %q", p.Level)
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

// cellFrom reads an optional cell from a payload, defaulting to the cursor
// when allowed, and checks it is on the field.
func (g *Game) cellFrom(payload json.RawMessage, cursorDefault bool) (x, y int, err error) {
	var p cellPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &p); err != nil {
			return 0, 0, fmt.Errorf("invalid payload: %w", err)
		}
	}
	if p.X == nil || p.Y == nil {
		if !cursorDefault || p.X != nil || p.Y != nil {
			return 0, 0, fmt.Errorf("x and y are required")
		}
		return g.cx, g.cy, nil
	}
	if !g.inField(*p.X, *p.Y) {
		return 0, 0, fmt.Errorf("cell %d,%d is off the field", *p.X, *p.Y)
	}
	return *p.X, *p.Y, nil
}

// Commands lists the commands this game supports.
func (g *Game) Commands() []core.CommandSpec {
	cellSchema := core.MustJSON(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer", "minimum": 0},
			"y": map[string]any{"type": "integer", "minimum": 0},
		},
	})
	levelNames := make([]string, len(Levels))
	for i, l := range Levels {
		levelNames[i] = l.Name
	}
	return []core.CommandSpec{
		{Name: cmdMove, Description: "move the cursor one cell", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"direction": map[string]any{"type": "string", "enum": []string{"up", "down", "left", "right"}},
			},
			"required": []string{"direction"},
		})},
		{Name: cmdCursor, Description: "put the cursor on a cell", Schema: cellSchema},
		{Name: cmdReveal, Description: "reveal a cell (the cursor's by default); on a satisfied number, its neighbours", Schema: cellSchema},
		{Name: cmdFlag, Description: "toggle the flag on a cell (the cursor's by default)", Schema: cellSchema},
		{Name: cmdLevel, Description: "start a field of this size (also leaves the picker)", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"level": map[string]any{"type": "string", "enum": levelNames},
			},
			"required": []string{"level"},
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

// running reports whether a field is open to play: past the picker, not
// paused, not over.
func (g *Game) running() bool {
	return !g.choosing && !g.paused && !g.gameOver
}

func (g *Game) inField(x, y int) bool {
	return x >= 0 && x < g.level.Cols && y >= 0 && y < g.level.Rows
}

// moveCursor steps the cursor, staying on the field.
func (g *Game) moveCursor(d direction) bool {
	x, y := g.cx, g.cy
	switch d {
	case dirUp:
		y--
	case dirDown:
		y++
	case dirLeft:
		x--
	case dirRight:
		x++
	}
	if !g.inField(x, y) {
		return false
	}
	g.cx, g.cy = x, y
	return true
}

// neighbours calls fn for every cell around (x, y) that is on the field.
func (g *Game) neighbours(x, y int, fn func(nx, ny int)) {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if nx, ny := x+dx, y+dy; g.inField(nx, ny) {
				fn(nx, ny)
			}
		}
	}
}

// placeMines scatters the mines everywhere but on (x, y) and its
// neighbours, so the first reveal always opens up, then counts adjacents.
func (g *Game) placeMines(x, y int) {
	var candidates []cellPos
	for cy := 0; cy < g.level.Rows; cy++ {
		for cx := 0; cx < g.level.Cols; cx++ {
			if abs(cx-x) <= 1 && abs(cy-y) <= 1 {
				continue
			}
			candidates = append(candidates, cellPos{X: cx, Y: cy})
		}
	}
	g.rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	for _, c := range candidates[:g.level.Mines] {
		g.cells[c.Y][c.X].mine = true
	}
	g.countAdjacents()
	g.placed = true
	g.runningSince = time.Now()
}

// countAdjacents fills in how many mines surround every cell.
func (g *Game) countAdjacents() {
	for cy := 0; cy < g.level.Rows; cy++ {
		for cx := 0; cx < g.level.Cols; cx++ {
			n := 0
			g.neighbours(cx, cy, func(nx, ny int) {
				if g.cells[ny][nx].mine {
					n++
				}
			})
			g.cells[cy][cx].adjacent = n
		}
	}
}

// reveal opens a cell. A hidden cell opens - and, when it has no mines
// around it, floods outwards. A revealed number whose flags match it opens
// its unflagged neighbours (a chord). Flagged cells, empty chords and a
// finished run do nothing and report false.
func (g *Game) reveal(x, y int) bool {
	if g.choosing || g.gameOver {
		return false
	}
	c := &g.cells[y][x]
	if c.flagged {
		return false
	}
	if c.revealed {
		return g.chord(x, y)
	}
	if !g.placed {
		g.placeMines(x, y)
	}
	if c.mine {
		g.explode(x, y)
		return true
	}
	n := g.flood(x, y, time.Now())
	g.afterReveal(x, y, n)
	return true
}

// chord opens the unflagged neighbours of a revealed number once exactly as
// many flags sit around it as it says. A wrong flag then costs the run.
func (g *Game) chord(x, y int) bool {
	c := g.cells[y][x]
	if c.adjacent == 0 {
		return false
	}
	flags, hidden := 0, 0
	g.neighbours(x, y, func(nx, ny int) {
		switch {
		case g.cells[ny][nx].flagged:
			flags++
		case !g.cells[ny][nx].revealed:
			hidden++
		}
	})
	if flags != c.adjacent || hidden == 0 {
		return false
	}
	now := time.Now()
	total := 0
	var hit *cellPos
	g.neighbours(x, y, func(nx, ny int) {
		n := &g.cells[ny][nx]
		if n.flagged || n.revealed || hit != nil {
			return
		}
		if n.mine {
			hit = &cellPos{X: nx, Y: ny}
			return
		}
		total += g.flood(nx, ny, now)
	})
	if hit != nil {
		g.explode(hit.X, hit.Y)
		return true
	}
	g.afterReveal(x, y, total)
	return true
}

// flood reveals (x, y) and, from every cell with no mines around it, its
// neighbours, breadth-first. Each ring shows one cascadeStep after the
// previous, starting at from. It returns how many cells it opened.
func (g *Game) flood(x, y int, from time.Time) int {
	type item struct {
		x, y, ring int
	}
	queue := []item{{x, y, 0}}
	g.cells[y][x].revealed = true
	opened := 0
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		c := &g.cells[it.y][it.x]
		c.showAt = from.Add(time.Duration(it.ring) * cascadeStep)
		opened++
		if c.adjacent != 0 {
			continue
		}
		g.neighbours(it.x, it.y, func(nx, ny int) {
			n := &g.cells[ny][nx]
			if n.revealed || n.flagged || n.mine {
				return
			}
			n.revealed = true
			queue = append(queue, item{nx, ny, it.ring + 1})
		})
	}
	g.revealedCount += opened
	return opened
}

// afterReveal scores an opening and ends the run when the field is clear.
func (g *Game) afterReveal(x, y, opened int) {
	g.score += opened * pointsPerCell
	g.emit(evRevealed, revealedPayload{X: x, Y: y, Cells: opened, Score: g.score})
	if g.revealedCount < g.safeCells() {
		return
	}
	g.stopClock()
	seconds := g.elapsed.Seconds()
	bonus := timeBonusMax - int(seconds)
	if bonus < 0 {
		bonus = 0
	}
	g.score += clearBonus + bonus
	g.won = true
	g.plantRemainingFlags()
	g.finish()
	g.emit(evCleared, clearedPayload{Score: g.score, Seconds: seconds})
	g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Won: true})
}

// plantRemainingFlags flags every mine still hidden after a clear, one
// after another outwards from the cursor.
func (g *Game) plantRemainingFlags() {
	var todo []cellPos
	for y := 0; y < g.level.Rows; y++ {
		for x := 0; x < g.level.Cols; x++ {
			if g.cells[y][x].mine && !g.cells[y][x].flagged {
				todo = append(todo, cellPos{X: x, Y: y})
			}
		}
	}
	sortByDistance(todo, g.cx, g.cy)
	now := time.Now()
	for i, p := range todo {
		c := &g.cells[p.Y][p.X]
		c.flagged = true
		c.showAt = now.Add(time.Duration(i+1) * plantFlagStep)
		g.flagCount++
	}
}

// explode ends the run on the mine at (x, y): it goes off at once and the
// others show one by one, nearest first; wrong flags show with the last.
func (g *Game) explode(x, y int) {
	now := time.Now()
	hit := &g.cells[y][x]
	hit.revealed = true
	hit.exploded = true
	hit.showAt = now

	var others []cellPos
	for cy := 0; cy < g.level.Rows; cy++ {
		for cx := 0; cx < g.level.Cols; cx++ {
			c := g.cells[cy][cx]
			if (cx == x && cy == y) || c.revealed {
				continue
			}
			if c.mine != c.flagged {
				others = append(others, cellPos{X: cx, Y: cy})
			}
		}
	}
	sortByDistance(others, x, y)
	for i, p := range others {
		c := &g.cells[p.Y][p.X]
		c.revealed = true
		c.showAt = now.Add(explodeDelay + time.Duration(i)*explodeStep)
	}

	g.stopClock()
	g.finish()
	g.emit(evExploded, cellPos{X: x, Y: y})
	g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Won: false})
}

// finish closes the run and records the score for this level.
func (g *Game) finish() {
	g.gameOver = true
	if g.score > g.best {
		g.best = g.score
	}
	g.store.Submit(g.level.scoreName, g.score)
}

// flag toggles the flag on a hidden cell.
func (g *Game) flag(x, y int) bool {
	if g.choosing || g.gameOver {
		return false
	}
	c := &g.cells[y][x]
	if c.revealed {
		return false
	}
	c.flagged = !c.flagged
	c.showAt = time.Time{}
	if c.flagged {
		g.flagCount++
	} else {
		g.flagCount--
	}
	g.emit(evFlagged, flaggedPayload{X: x, Y: y, Flagged: c.flagged, Flags: g.flagCount})
	return true
}

// sortByDistance orders cells by how far they are from (x, y), nearest
// first, reading order on ties.
func sortByDistance(cells []cellPos, x, y int) {
	sort.SliceStable(cells, func(i, j int) bool {
		di := abs(cells[i].X-x) + abs(cells[i].Y-y)
		dj := abs(cells[j].X-x) + abs(cells[j].Y-y)
		return di < dj
	})
}

// stopClock banks the running stretch of the timer.
func (g *Game) stopClock() {
	if !g.runningSince.IsZero() {
		g.elapsed += time.Since(g.runningSince)
		g.runningSince = time.Time{}
	}
}

// runTime is how long the run has taken so far.
func (g *Game) runTime() time.Duration {
	if g.runningSince.IsZero() {
		return g.elapsed
	}
	return g.elapsed + time.Since(g.runningSince)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
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
