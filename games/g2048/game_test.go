package g2048

import (
	"math/rand"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/terminalika/terminalika-core/highscore"
)

func newTestGame() *Game {
	g := NewWithStore(highscore.NewInMemory())
	g.rng = rand.New(rand.NewSource(1))
	return g
}

func count(g *Game) int {
	n := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if g.board[y][x] != 0 {
				n++
			}
		}
	}
	return n
}

func TestNewBoardHasTwoTiles(t *testing.T) {
	g := newTestGame()

	if n := count(g); n != 2 {
		t.Fatalf("tiles = %d, want 2", n)
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if v := g.board[y][x]; v != 0 && v != 2 && v != 4 {
				t.Fatalf("opening tile = %d, want 2 or 4", v)
			}
		}
	}
	if g.score != 0 || g.gameOver || g.won {
		t.Fatalf("score=%d gameOver=%v won=%v on a fresh board", g.score, g.gameOver, g.won)
	}
}

func TestMergeLine(t *testing.T) {
	cases := []struct {
		in, want [size]int
		points   int
		changed  bool
	}{
		{[size]int{0, 0, 0, 0}, [size]int{0, 0, 0, 0}, 0, false},
		{[size]int{2, 0, 0, 0}, [size]int{2, 0, 0, 0}, 0, false},
		{[size]int{0, 0, 0, 2}, [size]int{2, 0, 0, 0}, 0, true},
		{[size]int{2, 2, 0, 0}, [size]int{4, 0, 0, 0}, 4, true},
		{[size]int{2, 0, 2, 0}, [size]int{4, 0, 0, 0}, 4, true},
		{[size]int{2, 2, 2, 2}, [size]int{4, 4, 0, 0}, 8, true},
		{[size]int{4, 2, 2, 0}, [size]int{4, 4, 0, 0}, 4, true},
		{[size]int{2, 2, 4, 0}, [size]int{4, 4, 0, 0}, 4, true}, // the new 4 does not merge again
		{[size]int{2, 4, 2, 4}, [size]int{2, 4, 2, 4}, 0, false},
		{[size]int{8, 8, 8, 0}, [size]int{16, 8, 0, 0}, 16, true},
	}
	for _, c := range cases {
		got, points, changed, _ := mergeLine(c.in)
		if got != c.want || points != c.points || changed != c.changed {
			t.Errorf("mergeLine(%v) = %v, %d, %v; want %v, %d, %v", c.in, got, points, changed, c.want, c.points, c.changed)
		}
	}
}

func TestMergeLineReportsWhereEachTileLands(t *testing.T) {
	cases := []struct {
		in   [size]int
		dest [size]int
	}{
		{[size]int{0, 2, 0, 2}, [size]int{0, 0, 2, 0}},  // both 2s land on slot 0
		{[size]int{2, 2, 2, 2}, [size]int{0, 0, 1, 1}},  // two pairs
		{[size]int{4, 2, 2, 0}, [size]int{0, 1, 1, 3}},  // the 4 stays, the 2s merge behind it
		{[size]int{0, 0, 0, 8}, [size]int{0, 1, 2, 0}},  // a lone tile slides all the way
		{[size]int{2, 4, 8, 16}, [size]int{0, 1, 2, 3}}, // nothing moves
	}
	for _, c := range cases {
		_, _, _, dest := mergeLine(c.in)
		if dest != c.dest {
			t.Errorf("mergeLine(%v) dest = %v, want %v", c.in, dest, c.dest)
		}
	}
}

func TestMoveRecordsTripsAndTheSlideEnds(t *testing.T) {
	g := newTestGame()
	g.board = [size][size]int{{0, 2, 0, 2}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 4}}

	g.move(dirLeft)

	want := []tileMove{
		{value: 2, fromX: 1, fromY: 0, toX: 0, toY: 0},
		{value: 2, fromX: 3, fromY: 0, toX: 0, toY: 0},
		{value: 4, fromX: 3, fromY: 3, toX: 0, toY: 3},
	}
	if len(g.sliding) != len(want) {
		t.Fatalf("sliding = %+v, want %+v", g.sliding, want)
	}
	for i := range want {
		if g.sliding[i] != want[i] {
			t.Fatalf("trip %d = %+v, want %+v", i, g.sliding[i], want[i])
		}
	}
	if g.slideProgress() >= 1 {
		t.Fatal("a slide that just started should not be finished")
	}

	g.Update()
	if g.sliding == nil {
		t.Fatal("Update must not end a slide early")
	}
	g.slideStart = g.slideStart.Add(-slideDuration)
	g.Update()
	if g.sliding != nil || g.slideProgress() != 1 {
		t.Fatal("the slide should be over once slideDuration has passed")
	}
}

// paintedBoardCells counts the cells inside the board that carry a tile
// background; empty cells only carry the board tint, so this is tiles x
// tileW x tileH.
func paintedBoardCells(screen tcell.SimulationScreen) int {
	cells, w, h := screen.GetContents()
	_, boardBg, _ := boardStyle.Decompose()
	leftX, topY := (w-boardW)/2, (h-boardH)/2
	painted := 0
	for y := topY; y < topY+boardH; y++ {
		for x := leftX; x < leftX+boardW; x++ {
			_, bg, _ := cells[y*w+x].Style.Decompose()
			if bg != tcell.ColorDefault && bg != boardBg {
				painted++
			}
		}
	}
	return painted
}

func TestDrawMidSlideShowsTilesOnTheWay(t *testing.T) {
	g := newTestGame()
	g.board = [size][size]int{{0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 2}}
	g.move(dirLeft)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(g.NeededSize().Cols, g.NeededSize().Rows)

	// Half-way: the 2 is on its way and the spawned tile is not on screen
	// yet, so exactly one tile's worth of cells is painted.
	g.slideStart = time.Now().Add(-slideDuration / 2)
	g.Draw(screen)
	if got := paintedBoardCells(screen); got != tileW*tileH {
		t.Fatalf("%d cells painted mid-slide, want one tile (%d): the spawn must wait", got, tileW*tileH)
	}

	// Over: the 2 has landed and the spawned tile is there too.
	g.slideStart = time.Now().Add(-2 * slideDuration)
	g.Update()
	g.Draw(screen)
	if got := paintedBoardCells(screen); got != 2*tileW*tileH {
		t.Fatalf("%d cells painted after the slide, want two tiles (%d)", got, 2*tileW*tileH)
	}
}

func TestSlideEveryDirection(t *testing.T) {
	start := [size][size]int{
		{2, 0, 0, 2},
		{0, 4, 4, 0},
		{0, 0, 0, 0},
		{8, 0, 0, 8},
	}
	cases := []struct {
		d    direction
		want [size][size]int
	}{
		{dirLeft, [size][size]int{{4, 0, 0, 0}, {8, 0, 0, 0}, {0, 0, 0, 0}, {16, 0, 0, 0}}},
		{dirRight, [size][size]int{{0, 0, 0, 4}, {0, 0, 0, 8}, {0, 0, 0, 0}, {0, 0, 0, 16}}},
		{dirUp, [size][size]int{{2, 4, 4, 2}, {8, 0, 0, 8}, {0, 0, 0, 0}, {0, 0, 0, 0}}},
		{dirDown, [size][size]int{{0, 0, 0, 0}, {0, 0, 0, 0}, {2, 0, 0, 2}, {8, 4, 4, 8}}},
	}
	for _, c := range cases {
		g := newTestGame()
		g.board = start
		gained, moved, _ := g.slide(c.d)
		if !moved || g.board != c.want {
			t.Errorf("slide(%s): moved=%v board=%v, want %v", dirName(c.d), moved, g.board, c.want)
		}
		wantGained := 28 // 4 + 8 + 16
		if c.d == dirUp || c.d == dirDown {
			wantGained = 0
		}
		if gained != wantGained {
			t.Errorf("slide(%s): gained %d, want %d", dirName(c.d), gained, wantGained)
		}
	}
}

func TestMoveThatChangesNothingSpawnsNothing(t *testing.T) {
	g := newTestGame()
	g.board = [size][size]int{{2, 4, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}

	if g.move(dirLeft) {
		t.Fatal("a slide that changes nothing must not count as a move")
	}
	if n := count(g); n != 2 {
		t.Fatalf("tiles = %d after a rejected move, want 2 (no spawn)", n)
	}
	if g.score != 0 {
		t.Fatalf("score = %d, want 0", g.score)
	}
}

func TestMoveScoresSpawnsAndTracksBest(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)
	g.rng = rand.New(rand.NewSource(1))
	g.board = [size][size]int{{2, 2, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}

	if !g.move(dirLeft) {
		t.Fatal("expected the pair to merge")
	}
	if g.board[0][0] != 4 {
		t.Fatalf("board[0][0] = %d, want 4", g.board[0][0])
	}
	if n := count(g); n != 2 {
		t.Fatalf("tiles = %d, want the merged 4 plus one spawn", n)
	}
	if g.score != 4 || g.best != 4 || store.Best(gameName) != 4 {
		t.Fatalf("score=%d best=%d store=%d, want 4 each", g.score, g.best, store.Best(gameName))
	}
}

func TestReachingTheWinTileMarksWonAndPlayOn(t *testing.T) {
	g := newTestGame()
	g.board = [size][size]int{{1024, 1024, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}

	g.move(dirLeft)

	if !g.won {
		t.Fatal("merging into 2048 should mark the run as won")
	}
	if g.gameOver {
		t.Fatal("winning must not end the run")
	}
}

// stuckAfterLeft has exactly one move: sliding left merges the bottom 2 2
// into 4 8 16 _, and the spawn lands in that gap next to a 32 and a 16, so
// whether it is a 2 or a 4 the board is stuck.
var stuckAfterLeft = [size][size]int{
	{2, 4, 8, 16},
	{16, 8, 4, 2},
	{2, 4, 8, 32},
	{2, 2, 8, 16},
}

func TestStuckBoardEndsTheRun(t *testing.T) {
	g := newTestGame()
	g.board = stuckAfterLeft
	if !g.canMove() {
		t.Fatal("the pair at the bottom left is a move")
	}

	if !g.move(dirLeft) {
		t.Fatal("expected the pair to merge")
	}

	if g.board[3][3] == 0 {
		t.Fatal("spawn should have filled the last cell")
	}
	if g.canMove() || !g.gameOver {
		t.Fatalf("board %v still has moves or the run did not end", g.board)
	}
}

func TestExistingBestIsNotLowered(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 500)
	g := NewWithStore(store)
	g.rng = rand.New(rand.NewSource(1))
	g.board = [size][size]int{{2, 2, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}

	g.move(dirLeft)

	if g.best != 500 || store.Best(gameName) != 500 {
		t.Fatalf("best = %d, store = %d; want 500 to remain", g.best, store.Best(gameName))
	}
}

func TestResetStartsOver(t *testing.T) {
	g := newTestGame()
	g.board = [size][size]int{{2, 2, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}, {0, 0, 0, 0}}
	g.move(dirLeft)
	g.won = true
	g.gameOver = true

	g.Reset()

	if g.score != 0 || g.won || g.gameOver || count(g) != 2 {
		t.Fatalf("after Reset: score=%d won=%v gameOver=%v tiles=%d", g.score, g.won, g.gameOver, count(g))
	}
}
