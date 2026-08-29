package invaders

import (
	"testing"
	"time"

	"github.com/terminalika/terminalika-core/highscore"
)

func newTestGame() *Game {
	return NewWithStore(highscore.NewInMemory())
}

// clearAliens removes every alien so a test can place exactly the ones it
// needs.
func clearAliens(g *Game) {
	for r := range g.aliens {
		for c := range g.aliens[r] {
			g.aliens[r][c] = false
		}
	}
	g.alive = 0
}

func countBullets(g *Game, fromPlayer bool) int {
	n := 0
	for _, b := range g.bullets {
		if b.fromPlayer() == fromPlayer {
			n++
		}
	}
	return n
}

func TestNewSpawnsFormationAndCannon(t *testing.T) {
	g := newTestGame()

	if g.alive != alienRows*alienCols {
		t.Fatalf("alive = %d, want %d", g.alive, alienRows*alienCols)
	}
	if g.formation != formationStart {
		t.Fatalf("formation = %v, want %v", g.formation, formationStart)
	}
	if g.player != boardColumns/2 {
		t.Fatalf("player = %d, want %d", g.player, boardColumns/2)
	}
	if g.lives != startLives || g.wave != 1 {
		t.Fatalf("lives/wave = %d/%d, want %d/1", g.lives, g.wave, startLives)
	}
	if g.gameOver || g.paused {
		t.Fatal("fresh game should be running")
	}
	if g.score != 0 || g.best != 0 {
		t.Fatalf("score/best = %d/%d, want 0/0", g.score, g.best)
	}
	if len(g.bullets) != 0 {
		t.Fatalf("bullets = %d, want 0", len(g.bullets))
	}
}

func TestFormationFitsTheBoard(t *testing.T) {
	g := newTestGame()

	minX, maxX, maxY := g.formationBounds()
	if minX < 0 || maxX >= boardColumns {
		t.Fatalf("formation x range %d..%d leaves the board (%d columns)", minX, maxX, boardColumns)
	}
	if maxY >= playerRow {
		t.Fatalf("formation bottom row %d already reaches the cannon row %d", maxY, playerRow)
	}
}

func TestMoveStaysInBounds(t *testing.T) {
	g := newTestGame()

	for i := 0; i < boardColumns+2; i++ {
		g.move(-1)
	}
	if g.player != 0 {
		t.Fatalf("player = %d after moving far left, want 0", g.player)
	}
	if g.move(-1) {
		t.Fatal("moving past the left edge must fail")
	}

	for i := 0; i < boardColumns+2; i++ {
		g.move(1)
	}
	if g.player != boardColumns-1 {
		t.Fatalf("player = %d after moving far right, want %d", g.player, boardColumns-1)
	}
	if g.move(1) {
		t.Fatal("moving past the right edge must fail")
	}
}

// reloadFully ticks the cannon's shots until the reload is over, without
// letting them reach the formation.
func reloadFully(t *testing.T, g *Game) {
	t.Helper()
	for i := 0; i < fireReload && g.reload > 0; i++ {
		g.stepShots(true)
	}
	if g.reload != 0 {
		t.Fatalf("reload = %d after %d ticks, want 0", g.reload, fireReload)
	}
}

func TestFireReloadsBetweenShots(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[0][0] = true
	g.alive = 1
	g.formation = Point{X: 20, Y: 1} // out of the cannon's column

	if !g.fire() {
		t.Fatal("first shot must fire")
	}
	if got := countBullets(g, true); got != 1 {
		t.Fatalf("player shots = %d, want 1", got)
	}
	if g.bullets[0].pos != (Point{X: g.player, Y: playerRow - 1}) {
		t.Fatalf("shot spawned at %v, want just above the cannon", g.bullets[0].pos)
	}
	if g.reload != fireReload {
		t.Fatalf("reload = %d after firing, want %d", g.reload, fireReload)
	}

	if g.fire() {
		t.Fatal("firing again at once must be refused while reloading")
	}
	if got := countBullets(g, true); got != 1 {
		t.Fatalf("player shots = %d after refused shot, want 1", got)
	}

	reloadFully(t, g)
	if !g.fire() {
		t.Fatal("shot must fire once reloaded")
	}
	if got := countBullets(g, true); got != 2 {
		t.Fatalf("player shots = %d, want 2 in flight", got)
	}
}

func TestFireCapsShotsInFlight(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[0][0] = true
	g.alive = 1
	g.formation = Point{X: 20, Y: 1} // out of the cannon's column

	for i := 0; i < maxPlayerBullets; i++ {
		if !g.fire() {
			t.Fatalf("shot %d must fire", i+1)
		}
		reloadFully(t, g)
	}
	if got := countBullets(g, true); got != maxPlayerBullets {
		t.Fatalf("player shots = %d, want cap %d", got, maxPlayerBullets)
	}

	if g.fire() {
		t.Fatalf("shot %d must be refused: cap is %d in flight", maxPlayerBullets+1, maxPlayerBullets)
	}
	if got := countBullets(g, true); got != maxPlayerBullets {
		t.Fatalf("player shots = %d after refused shot, want %d", got, maxPlayerBullets)
	}
}

func TestPlayerShotDestroysAlienAndScores(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[1][2] = true // second row: worth rowScores[1]
	g.alive = 1
	g.aliens[0][0] = true // keep the wave alive after the kill
	g.alive = 2

	target := g.alienCell(slot{row: 1, col: 2})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}

	g.stepBullets()

	if g.aliens[1][2] {
		t.Fatal("alien should be destroyed")
	}
	if g.alive != 1 {
		t.Fatalf("alive = %d, want 1", g.alive)
	}
	if g.score != rowScores[1] {
		t.Fatalf("score = %d, want %d", g.score, rowScores[1])
	}
	if len(g.bullets) != 0 {
		t.Fatalf("bullets = %v, want the shot consumed", g.bullets)
	}
}

func TestPlayerShotPassesBetweenAliens(t *testing.T) {
	g := newTestGame()

	// The gap column between the first two aliens holds nothing to hit.
	gapX := g.alienCell(slot{row: 0, col: 0}).X + 1
	bottom := g.alienCell(slot{row: alienRows - 1, col: 0}).Y
	g.bullets = []bullet{{pos: Point{X: gapX, Y: bottom + 1}, dy: -1}}

	g.stepBullets()

	if g.alive != alienRows*alienCols {
		t.Fatalf("alive = %d, want the full formation", g.alive)
	}
	if len(g.bullets) != 1 {
		t.Fatalf("bullets = %d, want the shot still flying", len(g.bullets))
	}
}

func TestShotsLeavingTheBoardAreDropped(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[0][0] = true
	g.alive = 1
	g.formation = Point{X: 10, Y: 5} // away from both shots' paths

	g.bullets = []bullet{
		{pos: Point{X: 0, Y: 0}, dy: -1},            // about to fly off the top
		{pos: Point{X: 1, Y: boardRows - 1}, dy: 1}, // about to fly off the bottom
	}

	g.stepBullets()

	if len(g.bullets) != 0 {
		t.Fatalf("bullets = %v, want both dropped", g.bullets)
	}
	if g.lives != startLives {
		t.Fatalf("lives = %d, want %d (shot off-board must not hit)", g.lives, startLives)
	}
}

func TestOpposingShotsCancelOut(t *testing.T) {
	g := newTestGame()
	g.bullets = []bullet{
		{pos: Point{X: 5, Y: 9}, dy: -1},
		{pos: Point{X: 5, Y: 8}, dy: 1},
	}

	// Shots move one side at a time (the cannon's first), so a head-on pair
	// meets in a cell instead of swapping past each other: the player shot
	// steps up onto the alien shot and both are cancelled.
	g.stepBullets()
	if len(g.bullets) != 0 {
		t.Fatalf("bullets = %v after head-on step, want both cancelled", g.bullets)
	}

	// Already sharing a cell: cancelled on resolve alone.
	g.bullets = []bullet{
		{pos: Point{X: 5, Y: 8}, dy: -1},
		{pos: Point{X: 5, Y: 8}, dy: 1},
	}
	g.resolveCollisions()
	if len(g.bullets) != 0 {
		t.Fatalf("bullets = %v, want both cancelled", g.bullets)
	}

	// Shots in different columns pass each other untouched.
	g.bullets = []bullet{
		{pos: Point{X: 5, Y: 9}, dy: -1},
		{pos: Point{X: 6, Y: 8}, dy: 1},
	}
	g.stepBullets()
	if len(g.bullets) != 2 {
		t.Fatalf("bullets = %v, want both still flying", g.bullets)
	}
}

func TestFormationTurnsAndDescendsAtEdge(t *testing.T) {
	g := newTestGame()
	g.alienDir = 1

	// Park the formation so its next step to the right would leave the board.
	_, maxX, _ := g.formationBounds()
	g.formation.X += boardColumns - 1 - maxX
	startY := g.formation.Y

	g.stepAliens()

	if g.formation.Y != startY+1 {
		t.Fatalf("formation Y = %d, want %d (descend one row)", g.formation.Y, startY+1)
	}
	if g.alienDir != -1 {
		t.Fatalf("alienDir = %d, want -1 (turned around)", g.alienDir)
	}
	if g.gameOver {
		t.Fatal("descending one row must not end the game this far up")
	}

	// The following step sweeps left without descending.
	x := g.formation.X
	g.stepAliens()
	if g.formation.X != x-1 || g.formation.Y != startY+1 {
		t.Fatalf("formation = %v, want (%d,%d)", g.formation, x-1, startY+1)
	}
}

func TestThinnedFormationSweepsFullWidth(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[0][0] = true // only the leftmost column survives
	g.alive = 1
	g.alienDir = 1

	// The single alien starts at formationStart.X and must reach the last
	// column before the formation turns, i.e. more steps than a full
	// formation could take.
	steps := 0
	for g.alienDir == 1 {
		g.stepAliens()
		steps++
		if steps > boardColumns {
			t.Fatal("formation never turned")
		}
	}
	if steps != boardColumns-formationStart.X {
		t.Fatalf("turned after %d steps, want %d", steps, boardColumns-formationStart.X)
	}
}

func TestFormationReachingCannonRowEndsGame(t *testing.T) {
	g := newTestGame()
	g.alienDir = 1

	// Lower the formation so the bottom row sits right above the cannon, then
	// force an edge turn: the descend lands on the cannon's row.
	_, maxX, maxY := g.formationBounds()
	g.formation.Y += playerRow - 1 - maxY
	g.formation.X += boardColumns - 1 - maxX

	g.stepAliens()

	if !g.gameOver {
		t.Fatal("formation reaching the cannon row should end the game")
	}
}

func TestAlienShotHitsCannonAndCostsLife(t *testing.T) {
	g := newTestGame()
	g.bullets = []bullet{{pos: Point{X: g.player, Y: playerRow - 1}, dy: 1}}

	g.stepBullets()

	if g.lives != startLives-1 {
		t.Fatalf("lives = %d, want %d", g.lives, startLives-1)
	}
	if len(g.bullets) != 0 {
		t.Fatal("a hit must clear every shot")
	}
	if g.gameOver {
		t.Fatal("losing one life must not end the game")
	}
}

func TestLosingLastLifeEndsGame(t *testing.T) {
	g := newTestGame()
	g.lives = 1
	g.bullets = []bullet{{pos: Point{X: g.player, Y: playerRow - 1}, dy: 1}}

	g.stepBullets()

	if g.lives != 0 {
		t.Fatalf("lives = %d, want 0", g.lives)
	}
	if !g.gameOver {
		t.Fatal("losing the last life should end the game")
	}
}

func TestAlienShotMissesCannonBeside(t *testing.T) {
	g := newTestGame()
	g.bullets = []bullet{{pos: Point{X: g.player + 1, Y: playerRow - 1}, dy: 1}}

	g.stepBullets()

	if g.lives != startLives {
		t.Fatalf("lives = %d, want %d (shot next to the cannon must miss)", g.lives, startLives)
	}
}

func TestAlienFireComesFromBottomOfColumn(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[0][3] = true
	g.aliens[2][3] = true // bottom-most alien of the only populated column
	g.alive = 2

	g.alienFire()

	if got := countBullets(g, false); got != 1 {
		t.Fatalf("alien shots = %d, want 1", got)
	}
	from := g.alienCell(slot{row: 2, col: 3})
	if g.bullets[0].pos != (Point{X: from.X, Y: from.Y + 1}) {
		t.Fatalf("alien shot at %v, want just below %v", g.bullets[0].pos, from)
	}
}

func TestAlienFireIsCapped(t *testing.T) {
	g := newTestGame()

	for i := 0; i < maxAlienBullets+3; i++ {
		g.alienFire()
	}

	if got := countBullets(g, false); got != maxAlienBullets {
		t.Fatalf("alien shots = %d, want cap %d", got, maxAlienBullets)
	}
}

func TestClearingWaveStartsNextWave(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[3][5] = true
	g.alive = 1
	g.formation = Point{X: 3, Y: 4}

	target := g.alienCell(slot{row: 3, col: 5})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}

	g.stepBullets()

	if g.wave != 2 {
		t.Fatalf("wave = %d, want 2", g.wave)
	}
	if g.alive != alienRows*alienCols {
		t.Fatalf("alive = %d, want a full new formation", g.alive)
	}
	if g.formation != formationStart {
		t.Fatalf("formation = %v, want reset to %v", g.formation, formationStart)
	}
	if len(g.bullets) != 0 {
		t.Fatal("a new wave must start with a clear sky")
	}
	if g.score != rowScores[3] {
		t.Fatalf("score = %d, want %d carried into the next wave", g.score, rowScores[3])
	}
	if g.gameOver {
		t.Fatal("clearing a wave must not end the game")
	}
}

func TestAlienPeriodSpeedsUpAsFormationThins(t *testing.T) {
	total := alienRows * alienCols

	full := alienPeriod(1, total)
	half := alienPeriod(1, total/2)
	last := alienPeriod(1, 1)

	if !(full > half && half > last) {
		t.Fatalf("periods full=%v half=%v last=%v, want strictly decreasing", full, half, last)
	}
	if last < 120*time.Millisecond {
		t.Fatalf("last alien period = %v, want no faster than 120ms", last)
	}
	// Wave 1 with a full formation must be leisurely.
	if full < 600*time.Millisecond {
		t.Fatalf("wave 1 full-formation period = %v, want at least 600ms", full)
	}
}

func TestAlienPeriodSpeedsUpEachWave(t *testing.T) {
	total := alienRows * alienCols

	if w1, w2 := alienPeriod(1, total), alienPeriod(2, total); w2 >= w1 {
		t.Fatalf("wave 2 period %v not faster than wave 1 %v", w2, w1)
	}
	// Late waves bottom out at the floor instead of speeding up forever.
	if w100, w200 := alienPeriod(100, total), alienPeriod(200, total); w100 != w200 {
		t.Fatalf("wave 100/200 periods %v/%v, want the same floor", w100, w200)
	}
	if got := alienPeriod(100, total); got != 280*time.Millisecond {
		t.Fatalf("late-wave full-formation period = %v, want 280ms floor", got)
	}
	// Even on the fastest wave the last alien never drops below the floor.
	if got := alienPeriod(100, 1); got < 120*time.Millisecond {
		t.Fatalf("last-alien period on a late wave = %v, want at least 120ms", got)
	}
}

func TestAlienFirePeriodTightensEachWave(t *testing.T) {
	if w1, w2 := alienFirePeriod(1), alienFirePeriod(2); w2 >= w1 {
		t.Fatalf("wave 2 fire period %v not faster than wave 1 %v", w2, w1)
	}
	if got := alienFirePeriod(100); got != 400*time.Millisecond {
		t.Fatalf("wave 100 fire period = %v, want floor 400ms", got)
	}
}

func TestBestScoreTracksAndPersists(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)
	clearAliens(g)
	g.aliens[0][0] = true
	g.aliens[0][1] = true
	g.alive = 2

	target := g.alienCell(slot{row: 0, col: 0})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}

	g.stepBullets()

	if g.best != rowScores[0] {
		t.Fatalf("best = %d, want %d", g.best, rowScores[0])
	}
	if got := store.Best(gameName); got != rowScores[0] {
		t.Fatalf("store best = %d, want %d", got, rowScores[0])
	}
}

func TestExistingBestIsNotLowered(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 5000)

	g := NewWithStore(store)
	if g.best != 5000 {
		t.Fatalf("best = %d, want 5000", g.best)
	}

	clearAliens(g)
	g.aliens[0][0] = true
	g.aliens[0][1] = true
	g.alive = 2

	target := g.alienCell(slot{row: 0, col: 0})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}

	g.stepBullets()

	if g.best != 5000 {
		t.Fatalf("best = %d, want 5000 to remain", g.best)
	}
	if got := store.Best(gameName); got != 5000 {
		t.Fatalf("store best = %d, want 5000", got)
	}
}

func TestResetRestoresFreshRound(t *testing.T) {
	g := newTestGame()
	g.lives = 1
	g.wave = 4
	g.score = 900
	g.gameOver = true
	g.player = 0
	clearAliens(g)

	g.Reset()

	if g.lives != startLives || g.wave != 1 || g.score != 0 || g.gameOver {
		t.Fatalf("after Reset lives/wave/score/over = %d/%d/%d/%v", g.lives, g.wave, g.score, g.gameOver)
	}
	if g.alive != alienRows*alienCols || g.player != boardColumns/2 {
		t.Fatalf("after Reset alive/player = %d/%d", g.alive, g.player)
	}
}
