// Package invaders implements a minimal Space Invaders game for
// terminalika-core.
package invaders

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
	boardColumns = 24
	boardRows    = 16

	gameName = "invaders"

	alienRows     = 4
	alienCols     = 8
	alienSpacingX = 2
	alienSpacingY = 2

	startLives      = 3
	maxAlienBullets = 3

	// The cannon can have a few shots in the air at once, but must reload
	// between them: fireReload is counted in player-shot ticks
	// (playerShotPeriod each), so it's ~140ms and the fire rate is
	// deterministic rather than wall-clock based.
	maxPlayerBullets = 3
	fireReload       = 4

	// playerShotPeriod is how often the cannon's shot advances one cell. It's
	// fast so firing feels snappy; alienShotPeriod is slower so incoming shots
	// can be read and dodged.
	playerShotPeriod = 35 * time.Millisecond
	alienShotPeriod  = 90 * time.Millisecond

	// Game feel: a hit freezes the action for a beat (hit-stop) while its
	// burst plays, and losing the cannon holds longer before it respawns.
	alienHitStop  = 60 * time.Millisecond
	cannonHitStop = 600 * time.Millisecond

	// burstDuration is how long a hit's burst sprite stays on screen;
	// popupDuration how long a score popup floats over a destroyed alien.
	burstDuration = 220 * time.Millisecond
	popupDuration = 500 * time.Millisecond
)

// playerRow is the row the cannon sits on.
const playerRow = boardRows - 1

// formationStart is where the top-left formation slot sits at the start of
// every wave.
var formationStart = Point{X: 1, Y: 1}

// rowScores is the score for shooting an alien, indexed by formation row: the
// top row is worth the most.
var rowScores = [alienRows]int{40, 30, 20, 10}

// Command types.
const (
	cmdMove   = "invaders.move"
	cmdFire   = "invaders.fire"
	cmdTick   = "invaders.tick"
	cmdPause  = "invaders.pause"
	cmdResume = "invaders.resume"
	cmdReset  = "invaders.reset"
)

// Event types.
const (
	evPlayerMoved = "invaders.player_moved"
	evShotFired   = "invaders.shot_fired"
	evAlienFired  = "invaders.alien_fired"
	evAliensMoved = "invaders.aliens_moved"
	evAlienHit    = "invaders.alien_hit"
	evPlayerHit   = "invaders.player_hit"
	evWaveCleared = "invaders.wave_cleared"
	evGameOver    = "invaders.game_over"
	evPaused      = "game.paused"
	evResumed     = "game.resumed"
	evReset       = "game.reset"
)

// Point is a single board cell.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// slot addresses one position in the alien formation grid.
type slot struct {
	row int
	col int
}

// bullet is a shot in flight. Player shots travel up (dy -1), alien shots
// travel down (dy +1).
type bullet struct {
	pos Point
	dy  int
}

func (b bullet) fromPlayer() bool { return b.dy < 0 }

// burstKind tells a burst sprite apart: an alien popping or the cannon being
// destroyed.
type burstKind int

const (
	burstAlien burstKind = iota
	burstCannon
)

// burst is a short explosion animation at a cell.
type burst struct {
	pos  Point
	kind burstKind
	at   time.Time
}

// popup is a score label that floats over a destroyed alien for a moment.
type popup struct {
	pos  Point
	text string
	at   time.Time
}

// Game holds the full Space Invaders state. It implements core.Game.
type Game struct {
	aliens    [alienRows][alienCols]bool
	alive     int
	formation Point // board position of the top-left formation slot
	alienDir  int   // +1 while the formation sweeps right, -1 while it sweeps left
	frame     int   // animation frame (0 or 1); flips on every formation step

	player  int // cannon column on playerRow
	dying   bool
	reload  int // player-shot ticks left before the cannon may fire again
	bullets []bullet

	lives int
	wave  int
	score int
	best  int

	gameOver    bool
	paused      bool
	pauseReason string

	lastAlienTick      time.Time
	lastPlayerShotTick time.Time
	lastAlienShotTick  time.Time
	lastAlienFire      time.Time
	alienPeriod        time.Duration
	alienFirePeriod    time.Duration

	// Game feel: the action is frozen until freezeUntil while a burst plays;
	// bursts and popups are purely visual and expire on their own; the board
	// shakes until shakeUntil after the cannon is hit.
	freezeUntil time.Time
	shakeUntil  time.Time
	bursts      []burst
	popups      []popup

	store   *highscore.Store
	keys    core.GlobalKeys
	emitter core.Emitter
}

// Event payloads.
type playerMovedPayload struct {
	X int `json:"x"`
}

type shotPayload struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type aliensMovedPayload struct {
	DX int `json:"dx"`
	DY int `json:"dy"`
}

type alienHitPayload struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Score int `json:"score"`
	Alive int `json:"alive"`
}

type playerHitPayload struct {
	Lives int `json:"lives"`
}

type waveClearedPayload struct {
	Wave  int `json:"wave"`
	Score int `json:"score"`
}

type gameOverPayload struct {
	Score  int    `json:"score"`
	Best   int    `json:"best"`
	Reason string `json:"reason"`
}

type pausedPayload struct {
	Reason string `json:"reason,omitempty"`
}

type movePayload struct {
	DX int `json:"dx"`
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

// New returns a freshly reset Space Invaders game that persists best scores
// to the default location.
func New() *Game {
	store, err := highscore.Open(highscore.DefaultPath())
	if err != nil {
		store = highscore.NewInMemory()
	}
	return NewWithStore(store)
}

// NewWithStore returns a freshly reset Space Invaders game using the given
// score store.
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

// Reset clears the board and starts a fresh round on wave 1.
func (g *Game) Reset() {
	g.lives = startLives
	g.wave = 1
	g.score = 0
	g.best = g.store.Best(gameName)
	g.gameOver = false
	g.paused = false
	g.pauseReason = ""
	g.player = boardColumns / 2
	g.dying = false
	g.reload = 0
	g.freezeUntil = time.Time{}
	g.shakeUntil = time.Time{}
	g.bursts = nil
	g.popups = nil
	g.spawnWave()
	g.resetClocks()
	g.emit(evReset, nil)
}

// spawnWave fills the formation, parks it at its start position and clears
// the sky, keeping the current wave's speed settings.
func (g *Game) spawnWave() {
	for r := range g.aliens {
		for c := range g.aliens[r] {
			g.aliens[r][c] = true
		}
	}
	g.alive = alienRows * alienCols
	g.formation = formationStart
	g.alienDir = 1
	g.frame = 0
	g.bullets = nil
	g.alienPeriod = alienPeriod(g.wave, g.alive)
	g.alienFirePeriod = alienFirePeriod(g.wave)
}

func (g *Game) resetClocks() {
	now := time.Now()
	g.lastAlienTick = now
	g.lastPlayerShotTick = now
	g.lastAlienShotTick = now
	g.lastAlienFire = now
}

// Update advances shots, the formation and the aliens' fire on their own
// clocks. The cannon's shot moves on a fast clock and alien shots on a slower
// one; the formation and its fire speed up with the wave, and the formation
// also speeds up as it thins out. While a hit's freeze is in effect nothing
// moves - the burst on screen does the talking - and a destroyed cannon
// respawns once its freeze is over.
func (g *Game) Update() {
	if g.paused || g.gameOver {
		return
	}
	now := time.Now()
	g.expireEffects(now)

	if now.Before(g.freezeUntil) {
		return
	}
	if g.dying {
		g.respawn()
		g.resetClocks()
		return
	}

	if now.Sub(g.lastPlayerShotTick) >= playerShotPeriod {
		g.stepShots(true)
		g.lastPlayerShotTick = now
	}
	if g.gameOver || now.Before(g.freezeUntil) {
		return
	}
	if now.Sub(g.lastAlienShotTick) >= alienShotPeriod {
		g.stepShots(false)
		g.lastAlienShotTick = now
	}
	if g.gameOver || now.Before(g.freezeUntil) {
		return
	}
	if now.Sub(g.lastAlienTick) >= g.alienPeriod {
		g.stepAliens()
		g.lastAlienTick = now
	}
	if g.gameOver || now.Before(g.freezeUntil) {
		return
	}
	if now.Sub(g.lastAlienFire) >= g.alienFirePeriod {
		g.alienFire()
		g.lastAlienFire = now
	}
}

// expireEffects drops bursts and popups that have finished playing.
func (g *Game) expireEffects(now time.Time) {
	bursts := g.bursts[:0]
	for _, b := range g.bursts {
		if now.Sub(b.at) < burstDuration {
			bursts = append(bursts, b)
		}
	}
	g.bursts = bursts

	popups := g.popups[:0]
	for _, p := range g.popups {
		if now.Sub(p.at) < popupDuration {
			popups = append(popups, p)
		}
	}
	g.popups = popups
}

// respawn puts a fresh, loaded cannon back in the middle after it was
// destroyed.
func (g *Game) respawn() {
	g.dying = false
	g.reload = 0
	g.player = boardColumns / 2
}

// HandleInput handles the game-specific keys: left/right (or A/D) move the
// cannon, up (or W/X) fires. SPACE, R and ESC are reserved for the launcher
// and are never claimed here. Keys are swallowed while the cannon respawns.
func (g *Game) HandleInput(ev *tcell.EventKey) bool {
	if g.gameOver || g.paused {
		return false
	}
	if g.dying {
		return true
	}

	switch ev.Key() {
	case tcell.KeyLeft:
		g.moveAndEmit(-1)
		return true
	case tcell.KeyRight:
		g.moveAndEmit(1)
		return true
	case tcell.KeyUp:
		g.fire()
		return true
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'a', 'A':
			g.moveAndEmit(-1)
			return true
		case 'd', 'D':
			g.moveAndEmit(1)
			return true
		case 'w', 'W', 'x', 'X':
			g.fire()
			return true
		}
	}
	return false
}

// Pause freezes the formation and every shot.
func (g *Game) Pause() {
	if g.gameOver || g.paused {
		return
	}
	g.paused = true
	g.emit(evPaused, pausedPayload{Reason: g.pauseReason})
}

// Resume continues the game and restarts its clocks.
func (g *Game) Resume() {
	if !g.paused {
		return
	}
	g.paused = false
	g.pauseReason = ""
	g.resetClocks()
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
		if p.DX != -1 && p.DX != 1 {
			return fmt.Errorf("invalid move %d", p.DX)
		}
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if g.dying {
			return fmt.Errorf("cannon is respawning")
		}
		if !g.moveAndEmit(p.DX) {
			return fmt.Errorf("cannot move %d", p.DX)
		}
		return nil
	case cmdFire:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		if g.dying {
			return fmt.Errorf("cannon is respawning")
		}
		if g.reload > 0 {
			return fmt.Errorf("cannon is reloading")
		}
		if !g.fire() {
			return fmt.Errorf("too many shots in flight")
		}
		return nil
	case cmdTick:
		if g.paused || g.gameOver {
			return fmt.Errorf("game is not running")
		}
		g.stepBullets()
		if !g.gameOver {
			g.stepAliens()
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
		{Name: cmdMove, Description: "move the cannon one cell", Schema: core.MustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dx": map[string]any{"type": "integer", "enum": []int{-1, 1}},
			},
			"required": []string{"dx"},
		})},
		{Name: cmdFire, Description: "fire the cannon (a few shots in flight at a time, with a short reload between them)"},
		{Name: cmdTick, Description: "advance every shot one cell, then the formation one step"},
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

func (g *Game) move(dx int) bool {
	if g.dying {
		return false
	}
	x := g.player + dx
	if x < 0 || x >= boardColumns {
		return false
	}
	g.player = x
	return true
}

// moveAndEmit moves the cannon and emits an event on success.
func (g *Game) moveAndEmit(dx int) bool {
	if !g.move(dx) {
		return false
	}
	g.emit(evPlayerMoved, playerMovedPayload{X: g.player})
	return true
}

// fire launches a cannon shot. It reports false while the cannon is
// reloading, while maxPlayerBullets shots are already in flight, or while
// the cannon is being respawned.
func (g *Game) fire() bool {
	if g.dying || g.reload > 0 {
		return false
	}
	if g.playerBulletCount() >= maxPlayerBullets {
		return false
	}
	b := bullet{pos: Point{X: g.player, Y: playerRow - 1}, dy: -1}
	g.bullets = append(g.bullets, b)
	g.reload = fireReload
	g.emit(evShotFired, shotPayload{X: b.pos.X, Y: b.pos.Y})
	g.resolveCollisions()
	return true
}

func (g *Game) playerBulletCount() int {
	n := 0
	for _, b := range g.bullets {
		if b.fromPlayer() {
			n++
		}
	}
	return n
}

// alienFire has the bottom-most alien of a random column shoot at the cannon,
// unless the alien shot cap is already reached.
func (g *Game) alienFire() {
	if g.alienBulletCount() >= maxAlienBullets {
		return
	}

	var shooters []slot
	for c := 0; c < alienCols; c++ {
		for r := alienRows - 1; r >= 0; r-- {
			if g.aliens[r][c] {
				shooters = append(shooters, slot{row: r, col: c})
				break
			}
		}
	}
	if len(shooters) == 0 {
		return
	}

	s := shooters[rand.Intn(len(shooters))]
	from := g.alienCell(s)
	b := bullet{pos: Point{X: from.X, Y: from.Y + 1}, dy: 1}
	g.bullets = append(g.bullets, b)
	g.emit(evAlienFired, shotPayload{X: b.pos.X, Y: b.pos.Y})
	g.resolveCollisions()
}

func (g *Game) alienBulletCount() int {
	n := 0
	for _, b := range g.bullets {
		if !b.fromPlayer() {
			n++
		}
	}
	return n
}

// stepBullets advances every shot one cell, drops the ones that left the
// board, then resolves what the rest hit.
func (g *Game) stepBullets() {
	g.stepShots(true)
	if !g.gameOver {
		g.stepShots(false)
	}
}

// stepShots advances one side's shots (the cannon's or the aliens') one cell,
// drops the ones that left the board, then resolves what the rest hit. The
// cannon's reload counts down on its shots' ticks.
func (g *Game) stepShots(fromPlayer bool) {
	if fromPlayer && g.reload > 0 {
		g.reload--
	}
	kept := g.bullets[:0]
	for _, b := range g.bullets {
		if b.fromPlayer() == fromPlayer {
			b.pos.Y += b.dy
			if b.pos.Y < 0 || b.pos.Y >= boardRows {
				continue
			}
		}
		kept = append(kept, b)
	}
	g.bullets = kept
	g.resolveCollisions()
}

// stepAliens moves the formation one cell sideways, or one row down (turning
// around) when it would leave the board. Only living aliens count towards the
// edges, so a thinned-out formation keeps sweeping the full board width. The
// game is lost as soon as the formation reaches the cannon's row.
func (g *Game) stepAliens() {
	if g.alive == 0 {
		return
	}
	g.frame ^= 1

	minX, maxX, maxY := g.formationBounds()
	if minX+g.alienDir < 0 || maxX+g.alienDir >= boardColumns {
		g.formation.Y++
		g.alienDir = -g.alienDir
		g.emit(evAliensMoved, aliensMovedPayload{DX: 0, DY: 1})
		if maxY+1 >= playerRow {
			g.endGame("invaded")
			return
		}
	} else {
		g.formation.X += g.alienDir
		g.emit(evAliensMoved, aliensMovedPayload{DX: g.alienDir, DY: 0})
	}
	g.resolveCollisions()
}

// resolveCollisions applies every hit on the current board: opposing shots in
// the same cell cancel out, player shots destroy the alien they touch, and an
// alien shot reaching the cannon costs a life. Wiping out the formation starts
// the next wave.
func (g *Game) resolveCollisions() {
	cancelled := make([]bool, len(g.bullets))
	for i, b := range g.bullets {
		for j := i + 1; j < len(g.bullets); j++ {
			o := g.bullets[j]
			if b.pos == o.pos && b.dy != o.dy {
				cancelled[i] = true
				cancelled[j] = true
			}
		}
	}

	var kept []bullet
	for i, b := range g.bullets {
		if cancelled[i] {
			continue
		}
		if b.fromPlayer() {
			if s, ok := g.alienAt(b.pos); ok {
				g.killAlien(s)
				continue
			}
		} else if !g.dying && b.pos.X == g.player && b.pos.Y == playerRow {
			g.hitPlayer()
			return
		}
		kept = append(kept, b)
	}
	g.bullets = kept

	if g.alive == 0 {
		g.nextWave()
	}
}

// killAlien removes the alien in the slot, scores it and speeds the
// formation up, with a burst, a score popup and a beat of hit-stop so the
// hit lands.
func (g *Game) killAlien(s slot) {
	g.aliens[s.row][s.col] = false
	g.alive--
	points := rowScores[s.row]
	g.score += points
	if g.score > g.best {
		g.best = g.score
		g.store.Submit(gameName, g.score)
	}
	g.alienPeriod = alienPeriod(g.wave, g.alive)

	now := time.Now()
	p := g.alienCell(s)
	g.bursts = append(g.bursts, burst{pos: p, kind: burstAlien, at: now})
	g.popups = append(g.popups, popup{pos: p, text: fmt.Sprintf("+%d", points), at: now})
	g.freezeUntil = now.Add(alienHitStop)

	g.emit(evAlienHit, alienHitPayload{X: p.X, Y: p.Y, Score: g.score, Alive: g.alive})
}

// hitPlayer costs a life and clears the sky. The cannon bursts where it
// stood and the board shakes; the action holds until the burst has played,
// then a fresh cannon respawns in the middle (see Update). Losing the last
// life ends the game with the wreck still on screen.
func (g *Game) hitPlayer() {
	g.lives--
	g.bullets = nil

	now := time.Now()
	g.bursts = append(g.bursts, burst{pos: Point{X: g.player, Y: playerRow}, kind: burstCannon, at: now})
	g.freezeUntil = now.Add(cannonHitStop)
	g.shakeUntil = now.Add(burstDuration)
	g.dying = true

	g.emit(evPlayerHit, playerHitPayload{Lives: g.lives})
	if g.lives <= 0 {
		g.endGame("destroyed")
	}
}

// nextWave starts a fresh, faster formation once the current one is wiped
// out.
func (g *Game) nextWave() {
	g.emit(evWaveCleared, waveClearedPayload{Wave: g.wave, Score: g.score})
	g.wave++
	g.spawnWave()
	g.resetClocks()
}

func (g *Game) endGame(reason string) {
	g.gameOver = true
	g.emit(evGameOver, gameOverPayload{Score: g.score, Best: g.best, Reason: reason})
}

// alienCell returns the board cell of a formation slot.
func (g *Game) alienCell(s slot) Point {
	return Point{
		X: g.formation.X + s.col*alienSpacingX,
		Y: g.formation.Y + s.row*alienSpacingY,
	}
}

// alienAt returns the living alien occupying the board cell, if any.
func (g *Game) alienAt(p Point) (slot, bool) {
	dx := p.X - g.formation.X
	dy := p.Y - g.formation.Y
	if dx < 0 || dy < 0 || dx%alienSpacingX != 0 || dy%alienSpacingY != 0 {
		return slot{}, false
	}
	s := slot{row: dy / alienSpacingY, col: dx / alienSpacingX}
	if s.row >= alienRows || s.col >= alienCols {
		return slot{}, false
	}
	return s, g.aliens[s.row][s.col]
}

// formationBounds returns the leftmost and rightmost columns and the lowest
// row occupied by living aliens.
func (g *Game) formationBounds() (minX, maxX, maxY int) {
	minX, maxX, maxY = boardColumns, -1, -1
	for r := 0; r < alienRows; r++ {
		for c := 0; c < alienCols; c++ {
			if !g.aliens[r][c] {
				continue
			}
			p := g.alienCell(slot{row: r, col: c})
			if p.X < minX {
				minX = p.X
			}
			if p.X > maxX {
				maxX = p.X
			}
			if p.Y > maxY {
				maxY = p.Y
			}
		}
	}
	return minX, maxX, maxY
}

// alienPeriod is the formation's step interval. A full formation on wave 1
// is leisurely; every wave starts a little faster (down to a floor), and the
// formation speeds up as it thins out, from the wave's base period towards
// the last alien's fastest one.
func alienPeriod(wave, alive int) time.Duration {
	const (
		firstBase = 650 * time.Millisecond // wave 1, full formation
		slowest   = 280 * time.Millisecond // floor for a full formation on late waves
		fastest   = 120 * time.Millisecond // the last alien standing
		perWave   = 45 * time.Millisecond
	)

	base := firstBase - time.Duration(wave-1)*perWave
	if base < slowest {
		base = slowest
	}

	total := alienRows * alienCols
	return fastest + (base-fastest)*time.Duration(alive)/time.Duration(total)
}

// alienFirePeriod is how often the formation shoots; it tightens each wave.
func alienFirePeriod(wave int) time.Duration {
	period := 1300*time.Millisecond - time.Duration(wave-1)*100*time.Millisecond
	if period < 400*time.Millisecond {
		return 400 * time.Millisecond
	}
	return period
}
