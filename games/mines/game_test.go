package mines

import (
	"math/rand"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/highscore"
)

// newTestGame is a beginner field past the picker.
func newTestGame() *Game {
	g := NewWithStore(highscore.NewInMemory())
	g.rng = rand.New(rand.NewSource(1))
	g.start(Levels[0])
	return g
}

// layout builds a beginner field from rows of '*' (mine) and '.' (safe),
// skipping the random placement.
func layout(g *Game, rows []string) {
	g.start(Levels[0])
	for y, row := range rows {
		for x, ch := range row {
			g.cells[y][x].mine = ch == '*'
		}
	}
	g.countAdjacents()
	g.placed = true
	g.runningSince = time.Now()
}

// corner has a mine-free top-left region so a reveal there floods, three
// mines tucked into the top-right corner, one in the open and the rest
// along the bottom: ten in all, like a real beginner field.
var corner = []string{
	".......**",
	"........*",
	".........",
	"......*..",
	".........",
	".........",
	".........",
	"..*......",
	"*.*..*.**",
}

func mines(g *Game) int {
	n := 0
	for y := 0; y < g.level.Rows; y++ {
		for x := 0; x < g.level.Cols; x++ {
			if g.cells[y][x].mine {
				n++
			}
		}
	}
	return n
}

func TestFirstRevealPlacesMinesAwayFromIt(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		g := NewWithStore(highscore.NewInMemory())
		g.rng = rand.New(rand.NewSource(seed))
		g.start(Levels[0])
		if g.placed {
			t.Fatal("mines placed before the first reveal")
		}

		g.reveal(4, 4)

		if n := mines(g); n != g.level.Mines {
			t.Fatalf("seed %d: %d mines, want %d", seed, n, g.level.Mines)
		}
		for y := 3; y <= 5; y++ {
			for x := 3; x <= 5; x++ {
				if g.cells[y][x].mine {
					t.Fatalf("seed %d: mine at %d,%d next to the first reveal", seed, x, y)
				}
			}
		}
		if g.gameOver {
			t.Fatalf("seed %d: the first reveal ended the run", seed)
		}
		if g.revealedCount < 9 {
			t.Fatalf("seed %d: first reveal opened %d cells, want the 3x3 at least", seed, g.revealedCount)
		}
	}
}

func TestAdjacentCounts(t *testing.T) {
	g := newTestGame()
	layout(g, corner)

	cases := map[[2]int]int{{0, 0}: 0, {5, 3}: 1, {6, 2}: 1, {7, 4}: 1, {1, 8}: 3, {7, 7}: 2, {6, 8}: 2, {1, 7}: 3}
	for pos, want := range cases {
		if got := g.cells[pos[1]][pos[0]].adjacent; got != want {
			t.Errorf("adjacent(%d,%d) = %d, want %d", pos[0], pos[1], got, want)
		}
	}
}

func TestFloodOpensTheRegionAndRipplesOutwards(t *testing.T) {
	g := newTestGame()
	layout(g, corner)

	if !g.reveal(0, 0) {
		t.Fatal("reveal refused")
	}

	if !g.cells[0][0].revealed || !g.cells[2][8].revealed || !g.cells[6][0].revealed {
		t.Fatal("the flood should reach the whole open region")
	}
	if !g.cells[3][5].revealed || g.cells[3][6].revealed {
		t.Fatal("the flood should stop at the numbers around the mine and never open it")
	}
	if g.cells[8][1].revealed {
		t.Fatal("a cell walled off by mines must stay hidden")
	}
	if g.score != g.revealedCount*pointsPerCell {
		t.Fatalf("score = %d for %d cells", g.score, g.revealedCount)
	}

	// Ripple: cells further from the opened cell show later.
	if !g.cells[0][0].showAt.Before(g.cells[0][1].showAt) || !g.cells[0][1].showAt.Before(g.cells[0][4].showAt) {
		t.Fatalf("showAt should grow with distance: %v, %v, %v", g.cells[0][0].showAt, g.cells[0][1].showAt, g.cells[0][4].showAt)
	}
	if d := g.cells[0][4].showAt.Sub(g.cells[0][0].showAt); d != 4*cascadeStep {
		t.Fatalf("cell four rings out shows %v later, want %v", d, 4*cascadeStep)
	}
}

func TestFlagBlocksRevealAndToggles(t *testing.T) {
	g := newTestGame()
	layout(g, corner)

	if !g.flag(6, 3) || !g.cells[3][6].flagged || g.flagCount != 1 {
		t.Fatal("flagging a hidden cell should stick")
	}
	if g.reveal(6, 3) {
		t.Fatal("a flagged cell must not reveal")
	}
	if g.gameOver {
		t.Fatal("revealing a flagged mine must be a no-op, not a hit")
	}
	if !g.flag(6, 3) || g.cells[3][6].flagged || g.flagCount != 0 {
		t.Fatal("flagging again should unflag")
	}

	g.reveal(0, 0)
	if g.flag(0, 0) {
		t.Fatal("a revealed cell cannot be flagged")
	}
}

func TestHittingAMineEndsTheRunAndShowsMinesInTurn(t *testing.T) {
	g := newTestGame()
	layout(g, corner)
	g.flag(0, 8) // right
	g.flag(4, 0) // wrong

	g.reveal(6, 3)

	if !g.gameOver || g.won {
		t.Fatalf("gameOver=%v won=%v, want a lost run", g.gameOver, g.won)
	}
	hit := g.cells[3][6]
	if !hit.exploded || !hit.revealed {
		t.Fatal("the hit mine should have exploded")
	}
	if g.cells[8][0].revealed {
		t.Fatal("a correctly flagged mine stays flagged, not revealed")
	}
	if !g.cells[0][4].revealed || !g.cells[0][4].flagged {
		t.Fatal("a wrong flag is shown as such")
	}
	near, far := g.cells[8][5], g.cells[8][2] // 6 and 9 cells from the hit
	if !near.revealed || !far.revealed {
		t.Fatal("every unflagged mine shows after a hit")
	}
	if !hit.showAt.Before(near.showAt) || !near.showAt.Before(far.showAt) {
		t.Fatalf("mines should show nearest first: hit %v, near %v, far %v", hit.showAt, near.showAt, far.showAt)
	}
	if g.reveal(0, 0) {
		t.Fatal("no reveals after the run ended")
	}
}

// After opening the corner, (1,7) is a 3 - mines at (2,7), (0,8) and (2,8) -
// whose only hidden neighbour is the safe cell (1,8), walled in by them.
func TestChordOpensNeighboursOnlyWhenFlagsMatch(t *testing.T) {
	g := newTestGame()
	layout(g, corner)
	g.reveal(0, 0)
	if !g.cells[7][1].revealed || g.cells[8][1].revealed {
		t.Fatal("test layout: (1,7) should be open and (1,8) hidden")
	}

	if g.chord(1, 7) {
		t.Fatal("a chord with no flags around must do nothing")
	}
	g.flag(2, 7)
	g.flag(0, 8)
	if g.reveal(1, 7) {
		t.Fatal("a chord with too few flags must do nothing")
	}
	g.flag(2, 8)
	if !g.reveal(1, 7) {
		t.Fatal("a satisfied number should chord")
	}
	if !g.cells[8][1].revealed {
		t.Fatal("chord should open the unflagged neighbour")
	}
	if g.gameOver {
		t.Fatal("a correct chord must not end the run")
	}
}

func TestWrongFlagChordHitsAMine(t *testing.T) {
	g := newTestGame()
	layout(g, corner)
	g.reveal(0, 0)
	g.flag(2, 7)
	g.flag(0, 8)
	g.flag(1, 8) // wrong: the third mine is at (2,8)

	g.reveal(1, 7) // three flags around a 3, one of them on the wrong cell
	if !g.gameOver || !g.cells[8][2].exploded {
		t.Fatal("chording with a wrong flag should hit the real mine")
	}
	if !g.cells[8][1].revealed || !g.cells[8][1].flagged {
		t.Fatal("the wrong flag should be shown as such")
	}
}

func TestClearingTheFieldWinsPlantsFlagsAndScores(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)
	layout(g, corner)
	g.runningSince = time.Now().Add(-10 * time.Second)

	for y := 0; y < g.level.Rows; y++ {
		for x := 0; x < g.level.Cols; x++ {
			if !g.cells[y][x].mine && !g.cells[y][x].revealed && !g.gameOver {
				g.reveal(x, y)
			}
		}
	}

	if !g.gameOver || !g.won {
		t.Fatalf("gameOver=%v won=%v, want a cleared field", g.gameOver, g.won)
	}
	if g.revealedCount != g.safeCells() {
		t.Fatalf("revealed %d, want %d", g.revealedCount, g.safeCells())
	}
	if g.flagCount != mines(g) {
		t.Fatalf("flags = %d after the clear, want every one of the %d mines flagged", g.flagCount, mines(g))
	}
	cells := g.safeCells()
	if g.score < cells*pointsPerCell+clearBonus+timeBonusMax-11 || g.score > cells*pointsPerCell+clearBonus+timeBonusMax-10 {
		t.Fatalf("score = %d, want cells + clear bonus + ~290 time bonus", g.score)
	}
	if store.Best(gameName) != g.score {
		t.Fatalf("store best = %d, want %d", store.Best(gameName), g.score)
	}

	var planted []time.Time
	for y := 0; y < g.level.Rows; y++ {
		for x := 0; x < g.level.Cols; x++ {
			if c := g.cells[y][x]; c.mine && !c.showAt.IsZero() {
				planted = append(planted, c.showAt)
			}
		}
	}
	if len(planted) != mines(g) {
		t.Fatalf("%d flags planted with a delay, want %d", len(planted), mines(g))
	}
}

func TestPauseStopsTheClock(t *testing.T) {
	g := newTestGame()
	layout(g, corner)
	g.runningSince = time.Now().Add(-5 * time.Second)

	g.Pause()
	frozen := g.runTime()
	time.Sleep(20 * time.Millisecond)
	if g.runTime() != frozen {
		t.Fatal("the clock should not run while paused")
	}
	if frozen < 5*time.Second {
		t.Fatalf("runTime = %v, want the 5s banked", frozen)
	}

	g.Resume()
	if g.runningSince.IsZero() {
		t.Fatal("resume should restart the clock of a started run")
	}
}

func TestClockWaitsForTheFirstReveal(t *testing.T) {
	g := newTestGame()
	time.Sleep(5 * time.Millisecond)
	if g.runTime() != 0 {
		t.Fatal("the clock should not run before the first reveal")
	}
	g.reveal(4, 4)
	if g.runningSince.IsZero() {
		t.Fatal("the first reveal should start the clock")
	}
}

func TestExistingBestIsNotLowered(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 5000)
	g := NewWithStore(store)
	layout(g, corner)

	g.reveal(6, 3) // hit

	if g.best != 5000 || store.Best(gameName) != 5000 {
		t.Fatalf("best = %d, store = %d; want 5000 to remain", g.best, store.Best(gameName))
	}
}

func key(k tcell.Key) *tcell.EventKey { return tcell.NewEventKey(k, 0, 0) }

func TestGameOpensOnThePickerAndEnterStartsTheHighlightedLevel(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 700)
	store.Submit(gameName+"-expert", 9000)
	g := NewWithStore(store)
	g.rng = rand.New(rand.NewSource(3))

	if !g.choosing || g.level.Name != "beginner" || g.best != 700 {
		t.Fatalf("fresh game: choosing=%v level=%s best=%d; want the picker on beginner", g.choosing, g.level.Name, g.best)
	}
	if g.reveal(4, 4) || g.flag(4, 4) || g.placed {
		t.Fatal("nothing should happen on the field while the picker is up")
	}

	if !g.HandleInput(key(tcell.KeyUp)) || g.level.Name != "beginner" {
		t.Fatal("up on the first entry should stay put")
	}
	g.HandleInput(key(tcell.KeyDown))
	g.HandleInput(key(tcell.KeyDown))
	if g.level.Name != "expert" || g.best != 9000 || !g.choosing {
		t.Fatalf("after two downs: level=%s best=%d choosing=%v; want expert highlighted with its best", g.level.Name, g.best, g.choosing)
	}
	g.HandleInput(key(tcell.KeyDown))
	if g.level.Name != "expert" {
		t.Fatal("down on the last entry should stay put")
	}

	g.HandleInput(key(tcell.KeyEnter))

	if g.choosing || len(g.cells) != 16 || len(g.cells[0]) != 30 {
		t.Fatalf("after Enter: choosing=%v field %dx%d; want an expert field", g.choosing, len(g.cells[0]), len(g.cells))
	}
	if g.cx != 15 || g.cy != 8 {
		t.Fatalf("cursor = %d,%d, want the middle of the new field", g.cx, g.cy)
	}
	g.reveal(15, 8)
	if n := mines(g); n != 99 {
		t.Fatalf("%d mines on the expert field, want 99", n)
	}

	g.Reset()
	if !g.choosing || g.level.Name != "expert" || g.placed {
		t.Fatalf("Reset: choosing=%v level=%s placed=%v; want the picker back on expert", g.choosing, g.level.Name, g.placed)
	}
}

var wide = core.Size{Cols: 200, Rows: 100}

func TestNeededSizeFitsEveryLevel(t *testing.T) {
	g := NewWithStore(highscore.NewInMemory())
	size := g.NeededSize(wide)
	if size.Cols < 60 || size.Rows != 20 {
		t.Fatalf("NeededSize = %+v, want at least 60 wide and 20 tall for the expert field", size)
	}
	g.start(Levels[0])
	if g.NeededSize(wide) != size {
		t.Fatalf("NeededSize changed to %+v after starting; the launcher only asks once", g.NeededSize(wide))
	}
}

func TestLevelCommandStartsThatLevel(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 700)
	g := NewWithStore(store)

	err := g.HandleCommand(core.Command{Type: cmdLevel, Payload: core.MustJSON(levelPayload{Level: "intermediate"})})
	if err != nil || g.choosing || g.level.Name != "intermediate" || len(g.cells) != 16 {
		t.Fatalf("level command: err=%v choosing=%v level=%s", err, g.choosing, g.level.Name)
	}
	err = g.HandleCommand(core.Command{Type: cmdLevel, Payload: core.MustJSON(levelPayload{Level: "beginner"})})
	if err != nil || g.level.Name != "beginner" || g.best != 700 {
		t.Fatalf("level command: err=%v level=%s best=%d", err, g.level.Name, g.best)
	}
	if err := g.HandleCommand(core.Command{Type: cmdLevel, Payload: core.MustJSON(levelPayload{Level: "nightmare"})}); err == nil {
		t.Fatal("an unknown level should be rejected")
	}
}

func TestResetCoversTheField(t *testing.T) {
	g := newTestGame()
	layout(g, corner)
	g.reveal(0, 0)
	g.flag(6, 3)

	g.Reset()
	g.start(g.level)

	if g.placed || g.revealedCount != 0 || g.flagCount != 0 || g.score != 0 || g.runTime() != 0 {
		t.Fatalf("after Reset: placed=%v revealed=%d flags=%d score=%d time=%v", g.placed, g.revealedCount, g.flagCount, g.score, g.runTime())
	}
	if mines(g) != 0 {
		t.Fatal("mines should be gone until the next first reveal")
	}
}
