package tetris

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/terminalika/terminalika-core/highscore"
)

func newTestGame() *Game {
	return NewWithStore(highscore.NewInMemory())
}

func TestNewSpawnsPiece(t *testing.T) {
	g := newTestGame()

	if g.current == nil {
		t.Fatal("expected a current piece after reset")
	}
	if g.gameOver {
		t.Fatal("fresh game should not be over")
	}
}

func TestMoveRightStaysInBounds(t *testing.T) {
	g := newTestGame()

	// Move far right; the piece must never leave the board.
	for i := 0; i < boardColumns+2; i++ {
		g.move(1, 0)
	}

	for _, c := range g.current.cells() {
		if c.X < 0 || c.X >= boardColumns {
			t.Fatalf("piece cell out of horizontal bounds: %v", c)
		}
	}
}

func TestRotateDoesNotPanic(t *testing.T) {
	g := newTestGame()

	for i := 0; i < 8; i++ {
		g.rotate()
	}

	if g.current == nil {
		t.Fatal("rotate should keep a current piece")
	}
}

func TestClearLinesScoresAndClears(t *testing.T) {
	g := newTestGame()
	g.current = nil

	for x := 0; x < boardColumns; x++ {
		g.board[boardRows-1][x] = cell{filled: true, color: tcell.ColorWhite}
	}

	g.clearLines()

	if g.lines != 1 {
		t.Fatalf("lines = %d, want 1", g.lines)
	}
	if g.score != 100 {
		t.Fatalf("score = %d, want 100", g.score)
	}
	for x := 0; x < boardColumns; x++ {
		if g.board[boardRows-1][x].filled {
			t.Fatal("bottom row should be empty after clearing")
		}
	}
}

func TestBestScoreTracksAndPersists(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)
	g.current = nil

	for x := 0; x < boardColumns; x++ {
		g.board[boardRows-1][x] = cell{filled: true, color: tcell.ColorWhite}
	}

	g.clearLines()

	if g.best != 100 {
		t.Fatalf("best = %d, want 100", g.best)
	}
	if got := store.Best(gameName); got != 100 {
		t.Fatalf("store best = %d, want 100", got)
	}
}

func TestExistingBestIsNotLowered(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 900)

	g := NewWithStore(store)
	if g.best != 900 {
		t.Fatalf("best = %d, want 900", g.best)
	}

	g.current = nil
	for x := 0; x < boardColumns; x++ {
		g.board[boardRows-1][x] = cell{filled: true, color: tcell.ColorWhite}
	}

	g.clearLines()

	if g.best != 900 {
		t.Fatalf("best = %d, want 900 to remain", g.best)
	}
	if got := store.Best(gameName); got != 900 {
		t.Fatalf("store best = %d, want 900", got)
	}
}
