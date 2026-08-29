package pong

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func newSimScreen(t *testing.T, w, h int) tcell.SimulationScreen {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	screen.SetSize(w, h)
	t.Cleanup(screen.Fini)
	return screen
}

// screenContains reports whether text appears on one row of the screen.
func screenContains(screen tcell.SimulationScreen, text string) bool {
	cells, w, h := screen.GetContents()
	for y := 0; y < h; y++ {
		var row strings.Builder
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				row.WriteRune(c.Runes[0])
			} else {
				row.WriteRune(' ')
			}
		}
		if strings.Contains(row.String(), text) {
			return true
		}
	}
	return false
}

// countBackground counts the screen cells painted with the given background.
func countBackground(screen tcell.SimulationScreen, color tcell.Color) int {
	cells, _, _ := screen.GetContents()
	n := 0
	for _, c := range cells {
		if _, bg, _ := c.Style.Decompose(); bg == color {
			n++
		}
	}
	return n
}

func TestDrawSetupScreen(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}

	g.Draw(screen)

	for _, want := range []string{"PONG", "MODE", "NORMAL BOT", "FIRST TO", "5", "Enter: play", "BEST: 0"} {
		if !screenContains(screen, want) {
			t.Fatalf("setup screen should show %q", want)
		}
	}
	if countBackground(screen, paddleColor) != 0 || countBackground(screen, ballColor) != 0 {
		t.Fatal("no paddles or ball on the setup screen")
	}
}

func TestDrawCourtPaddlesAreOneColumnThick(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}
	startRally(g, modeTwoPlayers, 5)

	g.Draw(screen)

	if got := countBackground(screen, paddleColor); got != 2*paddleHeight*paddleWidth {
		t.Fatalf("paddle columns painted = %d, want %d (two paddles, %d rows, %d column each)", got, 2*paddleHeight*paddleWidth, paddleHeight, paddleWidth)
	}
	if got := countBackground(screen, ballColor); got != cellWidth {
		t.Fatalf("ball columns painted = %d, want one cell (%d columns)", got, cellWidth)
	}
	if got := countBackground(screen, netColor); got != ((boardRows+1)/2)*cellWidth {
		t.Fatalf("net columns painted = %d, want %d", got, ((boardRows+1)/2)*cellWidth)
	}
	if !screenContains(screen, "P1 0 : 0 P2") {
		t.Fatal("status bar should show the match score")
	}
}

func TestDrawPaddlesFaceTheCourt(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}
	startRally(g, modeTwoPlayers, 5)
	g.Draw(screen)

	cells, w, h := screen.GetContents()
	leftX := (w - boardColumns*cellWidth) / 2
	topY := (h - boardRows) / 2
	y := topY + g.paddles[0]

	bgAt := func(x int) tcell.Color {
		_, bg, _ := cells[y*w+x].Style.Decompose()
		return bg
	}
	// Left paddle: right column of its cell; right paddle: left column.
	if bgAt(leftX+leftPaddleX*cellWidth+1) != paddleColor || bgAt(leftX+leftPaddleX*cellWidth) == paddleColor {
		t.Fatal("left paddle should hug the right edge of its cell")
	}
	if bgAt(leftX+rightPaddleX*cellWidth) != paddleColor || bgAt(leftX+rightPaddleX*cellWidth+1) == paddleColor {
		t.Fatal("right paddle should hug the left edge of its cell")
	}
}

func TestDrawBigScoresUseTheBlockFont(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}
	startRally(g, modeTwoPlayers, 5)
	g.points = [2]int{0, 11}
	g.Draw(screen)

	pixels := func(text string) int {
		n := 0
		for _, r := range text {
			for _, line := range digitGlyphs[r] {
				n += strings.Count(line, "#")
			}
		}
		return n
	}
	want := (pixels("0") + pixels("11")) * cellWidth
	if got := countBackground(screen, digitColor); got != want {
		t.Fatalf("digit columns painted = %d, want %d for 0 : 11", got, want)
	}
}

func TestDrawOverlaysAndBotStatus(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}
	startRally(g, modeHard, 3)
	g.score = 12

	g.Pause()
	g.Draw(screen)
	if !screenContains(screen, "PAUSED") || !screenContains(screen, "YOU 0 : 0 BOT  SCORE: 12  BEST: 0") {
		t.Fatal("paused bot match should show PAUSED and the challenge score")
	}
	g.Resume()

	g.phase = phaseOver
	g.winner = 1
	g.Draw(screen)
	if !screenContains(screen, "BOT WINS") || !screenContains(screen, "GAME OVER") {
		t.Fatal("finished match should show the winner and GAME OVER")
	}
}

func TestDrawSkipsTooSmallTerminals(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 30, 10)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}
	startRally(g, modeTwoPlayers, 5)

	g.Draw(screen) // must not panic

	if countBackground(screen, boardColor) != 0 {
		t.Fatal("nothing should be drawn when the terminal is too small")
	}
}

func TestDigitGlyphsAreWellFormed(t *testing.T) {
	for r := '0'; r <= '9'; r++ {
		glyph, ok := digitGlyphs[r]
		if !ok {
			t.Fatalf("missing glyph for %q", r)
		}
		if len(glyph) != digitHeight {
			t.Fatalf("glyph %q has %d rows, want %d", r, len(glyph), digitHeight)
		}
		for _, line := range glyph {
			if len(line) != digitWidth {
				t.Fatalf("glyph %q row %q is %d wide, want %d", r, line, len(line), digitWidth)
			}
		}
	}
}
