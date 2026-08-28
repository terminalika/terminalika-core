package snake

import (
	"testing"

	"github.com/terminalika/terminalika-core/highscore"
)

func newTestGame() *Game {
	return NewWithStore(highscore.NewInMemory())
}

func TestNewSpawnsSnakeAndFood(t *testing.T) {
	g := newTestGame()

	if len(g.snake) != 3 {
		t.Fatalf("snake length = %d, want 3", len(g.snake))
	}
	if !g.hasFood {
		t.Fatal("fresh game should have food")
	}
	if g.gameOver {
		t.Fatal("fresh game should not be over")
	}
	if g.score != 0 || g.best != 0 {
		t.Fatalf("score/best = %d/%d, want 0/0", g.score, g.best)
	}
}

func TestTurnQueuesQuickInputs(t *testing.T) {
	g := newTestGame()
	g.dir = dirRight
	g.turns = nil

	g.turn(dirLeft) // reversal
	if len(g.turns) != 0 {
		t.Fatal("reversal must be ignored")
	}

	g.turn(dirUp)
	g.turn(dirLeft) // valid after up
	if len(g.turns) != 2 || g.turns[0] != dirUp || g.turns[1] != dirLeft {
		t.Fatalf("turns = %v, want [up left]", g.turns)
	}

	g.turn(dirRight) // opposite of the last queued turn
	if len(g.turns) != 2 {
		t.Fatal("reversal relative to queued turn must be ignored")
	}
}

func TestTurnQueueIsCapped(t *testing.T) {
	g := newTestGame()
	g.dir = dirRight

	g.turn(dirUp)
	g.turn(dirLeft)
	g.turn(dirDown)
	g.turn(dirRight) // exceeds the cap

	if len(g.turns) != 3 {
		t.Fatalf("turns length = %d, want 3", len(g.turns))
	}
}

func TestStepConsumesQueuedTurn(t *testing.T) {
	g := newTestGame()
	g.snake = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirRight
	g.turns = []direction{dirUp}
	g.hasFood = false

	g.step()

	if g.snake[0] != (Point{X: 5, Y: 4}) {
		t.Fatalf("head = %v, want (5,4)", g.snake[0])
	}
	if len(g.turns) != 0 {
		t.Fatalf("turns should be consumed, got %v", g.turns)
	}
}

func TestStepEatsFoodAndGrows(t *testing.T) {
	g := newTestGame()
	g.snake = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirRight
	g.turns = nil
	g.food = Point{X: 6, Y: 5}
	g.hasFood = true
	g.score = 0

	g.step()

	if g.gameOver {
		t.Fatal("eating food should not end the game")
	}
	if len(g.snake) != 4 {
		t.Fatalf("snake length = %d, want 4 after eating", len(g.snake))
	}
	if g.snake[0] != (Point{X: 6, Y: 5}) {
		t.Fatalf("new head = %v, want (6,5)", g.snake[0])
	}
	if g.score != scorePerFood {
		t.Fatalf("score = %d, want %d", g.score, scorePerFood)
	}
}

func TestStepHitsWallAndEnds(t *testing.T) {
	g := newTestGame()
	g.snake = []Point{{X: 0, Y: 5}, {X: 1, Y: 5}, {X: 2, Y: 5}}
	g.dir = dirLeft
	g.turns = nil
	g.hasFood = false

	g.step()

	if !g.gameOver {
		t.Fatal("hitting a wall should end the game")
	}
}

func TestStepHitsSelfAndEnds(t *testing.T) {
	g := newTestGame()
	// Head (5,5) moving left into (4,5), which is a non-tail segment.
	g.snake = []Point{{X: 5, Y: 5}, {X: 5, Y: 6}, {X: 4, Y: 6}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirLeft
	g.turns = nil
	g.hasFood = false

	g.step()

	if !g.gameOver {
		t.Fatal("hitting itself should end the game")
	}
}

func TestBestScoreTracksAndPersists(t *testing.T) {
	store := highscore.NewInMemory()
	g := NewWithStore(store)

	g.snake = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirRight
	g.turns = nil
	g.food = Point{X: 6, Y: 5}
	g.hasFood = true
	g.score = 0

	g.step()

	if g.best != scorePerFood {
		t.Fatalf("best = %d, want %d", g.best, scorePerFood)
	}
	if got := store.Best(gameName); got != scorePerFood {
		t.Fatalf("store best = %d, want %d", got, scorePerFood)
	}
}

func TestTickPeriodIncreasesEveryTenLevels(t *testing.T) {
	// Levels 1..10 all share the same speed.
	base := tickPeriod(scorePerFood) // level 1
	for level := 2; level <= 10; level++ {
		if got := tickPeriod(scorePerFood * level); got != base {
			t.Fatalf("level %d period = %v, want %v (same speed band)", level, got, base)
		}
	}

	// Level 11 crosses into the next speed band and must be faster.
	if got := tickPeriod(scorePerFood * 11); got >= base {
		t.Fatalf("level 11 period = %v, want faster than %v", got, base)
	}
}

func TestExistingBestIsNotLowered(t *testing.T) {
	store := highscore.NewInMemory()
	store.Submit(gameName, 500)

	g := NewWithStore(store)
	if g.best != 500 {
		t.Fatalf("best = %d, want 500", g.best)
	}

	g.snake = []Point{{X: 5, Y: 5}, {X: 4, Y: 5}, {X: 3, Y: 5}}
	g.dir = dirRight
	g.turns = nil
	g.food = Point{X: 6, Y: 5}
	g.hasFood = true

	g.step()

	if g.best != 500 {
		t.Fatalf("best = %d, want 500 to remain", g.best)
	}
	if got := store.Best(gameName); got != 500 {
		t.Fatalf("store best = %d, want 500", got)
	}
}
