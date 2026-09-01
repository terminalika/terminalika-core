package g2048

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
		got, points, changed := mergeLine(c.in)
		if got != c.want || points != c.points || changed != c.changed {
			t.Errorf("mergeLine(%v) = %v, %d, %v; want %v, %d, %v", c.in, got, points, changed, c.want, c.points, c.changed)
		}
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
		gained, moved := g.slide(c.d)
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
