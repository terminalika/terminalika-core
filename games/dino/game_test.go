package dino

import (
	"math/rand"
	"testing"

	"github.com/terminalika/terminalika-core/highscore"
)

func newTestGame() *Game {
	g := NewWithStore(highscore.NewInMemory())
	g.rng = rand.New(rand.NewSource(1))
	return g
}

// runTicks advances the game n ticks with no obstacles ever entering.
func runTicks(g *Game, n int) {
	g.nextSpawn = 1 << 30
	for i := 0; i < n; i++ {
		g.step()
	}
}

func TestNewStartsOnTheGroundWithAnEmptyDesert(t *testing.T) {
	g := newTestGame()

	if g.airborne || g.alt != 0 {
		t.Fatalf("fresh dino airborne=%v alt=%v, want on the ground", g.airborne, g.alt)
	}
	if len(g.obstacles) != 0 {
		t.Fatalf("obstacles = %v, want none", g.obstacles)
	}
	if g.gameOver {
		t.Fatal("fresh game should not be over")
	}
	if g.score != 0 || g.best != 0 {
		t.Fatalf("score/best = %d/%d, want 0/0", g.score, g.best)
	}
}

func TestJumpRisesThenLands(t *testing.T) {
	g := newTestGame()

	if !g.jump() {
		t.Fatal("first jump should take off")
	}
	if g.jump() {
		t.Fatal("a second jump in the air must be ignored")
	}

	peak := 0
	ticks := 0
	for g.airborne {
		g.nextSpawn = 1 << 30
		g.step()
		ticks++
		if a := g.altitude(); a > peak {
			peak = a
		}
		if ticks > 100 {
			t.Fatal("dino never landed")
		}
	}

	if peak < 4 {
		t.Fatalf("peak altitude = %d rows, want at least 4 to clear a tall cactus with room", peak)
	}
	if ticks < minGap-4 {
		t.Fatalf("airtime = %d ticks; minGap %d leaves no room to land before the next obstacle", ticks, minGap)
	}
	if g.alt != 0 || g.vy != 0 {
		t.Fatalf("landed with alt=%v vy=%v, want both zero", g.alt, g.vy)
	}
}

func TestStepScrollsObstaclesAndDropsThemPastTheEdge(t *testing.T) {
	g := newTestGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: cactusSmall, x: 1}}

	g.step()
	if len(g.obstacles) != 1 || g.obstacles[0].x != 0 {
		t.Fatalf("obstacles = %+v, want the cactus at x=0", g.obstacles)
	}

	g.step()
	if len(g.obstacles) != 0 {
		t.Fatalf("obstacles = %+v, want the cactus gone off the left edge", g.obstacles)
	}
}

func TestSpawnEntersFromTheRightAndSchedulesTheNext(t *testing.T) {
	g := newTestGame()
	g.nextSpawn = 1

	g.step()

	if len(g.obstacles) != 1 || g.obstacles[0].x != boardColumns {
		t.Fatalf("obstacles = %+v, want one at the right edge", g.obstacles)
	}
	width := shapes[g.obstacles[0].kind].width
	if gap := g.nextSpawn - g.distance - width; gap < minGap || gap > maxGap {
		t.Fatalf("next gap = %d, want within [%d, %d]", gap, minGap, maxGap)
	}
}

func TestBirdsOnlyAfterTheOpeningStretch(t *testing.T) {
	g := newTestGame()

	g.score = birdsFromScore - 1
	for i := 0; i < 200; i++ {
		if shapes[g.pickKind()].isBird {
			t.Fatal("a bird before birdsFromScore")
		}
	}

	g.score = birdsFromScore
	seen := false
	for i := 0; i < 200 && !seen; i++ {
		seen = shapes[g.pickKind()].isBird
	}
	if !seen {
		t.Fatal("no bird in 200 draws after birdsFromScore")
	}
}

func TestRunningIntoACactusEndsTheRun(t *testing.T) {
	g := newTestGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: cactusSmall, x: dinoX + dinoWidth}}

	g.step()

	if !g.gameOver {
		t.Fatal("a cactus in the dino's cells should end the run")
	}
	if g.deathKind != "cactus" {
		t.Fatalf("deathKind = %q, want cactus", g.deathKind)
	}
}

func TestJumpingOverACactusSurvives(t *testing.T) {
	g := newTestGame()
	g.nextSpawn = 1 << 30
	// The cluster's leading edge is one tick from the dino: jump now and
	// the whole thing should pass underneath.
	g.obstacles = []obstacle{{kind: cactusCluster, x: dinoX + dinoWidth + 1}}
	g.jump()

	for i := 0; i < 20 && !g.gameOver; i++ {
		g.step()
	}

	if g.gameOver {
		t.Fatalf("died to %s while jumping a cluster from one cell away", g.deathKind)
	}
	if len(g.obstacles) != 0 {
		t.Fatalf("obstacles = %+v, want the cluster long gone", g.obstacles)
	}
}

func TestRunningUnderAHighBirdSurvivesJumpingIntoItDoesNot(t *testing.T) {
	g := newTestGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: birdHigh, x: dinoX + dinoWidth + 1}}
	for i := 0; i < 6 && !g.gameOver; i++ {
		g.step()
	}
	if g.gameOver {
		t.Fatal("a high bird should pass over a dino that keeps running")
	}

	g = newTestGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: birdHigh, x: dinoX + dinoWidth + 3}}
	g.jump()
	for i := 0; i < 8 && !g.gameOver; i++ {
		g.step()
	}
	if !g.gameOver || g.deathKind != "high_bird" {
		t.Fatalf("gameOver=%v deathKind=%q, want to have jumped into the high bird", g.gameOver, g.deathKind)
	}
}

func TestALowBirdMustBeJumped(t *testing.T) {
	g := newTestGame()
	g.nextSpawn = 1 << 30
	g.obstacles = []obstacle{{kind: birdLow, x: dinoX + dinoWidth}}

	g.step()

	if !g.gameOver || g.deathKind != "bird" {
		t.Fatalf("gameOver=%v deathKind=%q, want the low bird to hit a standing dino", g.gameOver, g.deathKind)
	}
}

func TestScoreIsDistanceAndSpeedsUpEveryHundred(t *testing.T) {
	g := newTestGame()
	runTicks(g, 250)

	if g.score != 250 {
		t.Fatalf("score = %d after 250 ticks, want 250", g.score)
	}
	if g.period >= tickPeriod(0) {
		t.Fatalf("period = %v after two milestones, want faster than %v", g.period, tickPeriod(0))
	}
	if got := tickPeriod(100000); got != 50*1e6 {
		t.Fatalf("tickPeriod(100000) = %v, want the 50ms floor", got)
	}
}

func TestMilestoneFlashesAndPersistsBest(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)
	runTicks(g, milestoneEvery)

	if g.flashTicks != milestoneFlashTicks {
		t.Fatalf("flashTicks = %d at the milestone, want %d", g.flashTicks, milestoneFlashTicks)
	}
	if got := store.Best(gameName); got != milestoneEvery {
		t.Fatalf("store best = %d, want %d saved at the milestone", got, milestoneEvery)
	}
	runTicks(g, milestoneFlashTicks)
	if g.flashTicks != 0 {
		t.Fatalf("flashTicks = %d after the flash, want 0", g.flashTicks)
	}
}

func TestGameOverPersistsBest(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)
	runTicks(g, 42)
	g.obstacles = []obstacle{{kind: cactusSmall, x: dinoX + dinoWidth}}

	g.step()

	if !g.gameOver {
		t.Fatal("expected the cactus to end the run")
	}
	if g.best != 43 || store.Best(gameName) != 43 {
		t.Fatalf("best = %d, store = %d; want 43", g.best, store.Best(gameName))
	}
}

func TestExistingBestIsNotLowered(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 500)

	g := NewWithStore(store)
	if g.best != 500 {
		t.Fatalf("best = %d, want 500", g.best)
	}
	runTicks(g, milestoneEvery)

	if g.best != 500 || store.Best(gameName) != 500 {
		t.Fatalf("best = %d, store = %d; want 500 to remain", g.best, store.Best(gameName))
	}
}

func TestResetClearsTheRun(t *testing.T) {
	g := newTestGame()
	runTicks(g, 30)
	g.jump()
	g.obstacles = []obstacle{{kind: cactusSmall, x: dinoX + dinoWidth}}
	g.step()

	g.Reset()

	if g.gameOver || g.airborne || g.score != 0 || len(g.obstacles) != 0 || g.distance != 0 {
		t.Fatalf("after Reset: gameOver=%v airborne=%v score=%d obstacles=%v distance=%d", g.gameOver, g.airborne, g.score, g.obstacles, g.distance)
	}
}
