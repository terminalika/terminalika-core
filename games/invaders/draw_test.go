package invaders

import (
	"fmt"
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

func TestDrawRendersFormationShotsAndCannon(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	if err := g.Init(screen); err != nil {
		t.Fatalf("Init: %v", err)
	}

	g.fire()
	g.alienFire()
	g.Draw(screen)

	cells, w, _ := screen.GetContents()
	filled := 0
	playerShots := 0
	alienShots := 0
	for _, c := range cells {
		if len(c.Runes) > 0 {
			switch c.Runes[0] {
			case '|':
				playerShots++
			case '!':
				alienShots++
			}
		}
		if _, bg, _ := c.Style.Decompose(); bg != tcell.ColorDefault {
			filled++
		}
	}
	if w != 80 {
		t.Fatalf("screen width = %d, want 80", w)
	}
	if filled < boardColumns*boardRows*cellWidth {
		t.Fatalf("painted cells = %d, want at least the %d board cells", filled, boardColumns*boardRows*cellWidth)
	}
	if playerShots != 1 || alienShots != 1 {
		t.Fatalf("shot glyphs = %d player / %d alien, want 1 / 1", playerShots, alienShots)
	}
	if !screenContains(screen, cannonSprite) {
		t.Fatal("cannon sprite should be on screen")
	}
	if !screenContains(screen, alienSprites[0][0]) {
		t.Fatal("top-row alien sprite (frame 0) should be on screen")
	}
}

func TestSpritesFitTheCell(t *testing.T) {
	check := func(name, sprite string) {
		t.Helper()
		if n := len([]rune(sprite)); n != cellWidth {
			t.Fatalf("%s sprite %q is %d runes wide, want %d", name, sprite, n, cellWidth)
		}
		for _, r := range sprite {
			if r > 0x7f {
				t.Fatalf("%s sprite %q must be plain ASCII (found %q)", name, sprite, r)
			}
		}
	}
	check("cannon", cannonSprite)
	check("player shot", playerShotSprite)
	check("alien shot", alienShotSprite)
	for r, frames := range alienSprites {
		for f, sprite := range frames {
			check(fmt.Sprintf("alien row %d frame %d", r, f), sprite)
		}
	}
	for kind, frames := range burstSprites {
		for f, sprite := range frames {
			check(fmt.Sprintf("burst kind %d frame %d", kind, f), sprite)
		}
	}
}

func TestDrawSpriteCentresShortText(t *testing.T) {
	screen := newSimScreen(t, 20, 5)
	origin := boardOrigin{leftX: 0, topY: 0}

	drawSprite(screen, origin, Point{X: 1, Y: 0}, tcell.StyleDefault, "+9")
	screen.Show()

	cells, w, _ := screen.GetContents()
	got := ""
	for x := cellWidth; x < 2*cellWidth; x++ {
		got += string(cells[0*w+x].Runes)
	}
	if got != "+9 " {
		t.Fatalf("drew %q, want %q", got, "+9 ")
	}
}

func TestFormationAnimatesOnEveryStep(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)

	if g.frame != 0 {
		t.Fatalf("frame = %d at start, want 0", g.frame)
	}
	g.stepAliens()
	if g.frame != 1 {
		t.Fatalf("frame = %d after one step, want 1", g.frame)
	}
	g.Draw(screen)
	if !screenContains(screen, alienSprites[0][1]) {
		t.Fatal("frame 1 sprites should be drawn after a step")
	}

	g.stepAliens()
	if g.frame != 0 {
		t.Fatalf("frame = %d after two steps, want 0", g.frame)
	}
}

func TestCannonColorIsUniqueOnBoard(t *testing.T) {
	for r, c := range rowColors {
		if c == playerColor {
			t.Fatalf("alien row %d shares the cannon's colour", r)
		}
	}
	for _, taken := range []tcell.Color{tcell.ColorWhite, tcell.ColorRed, boardColor} {
		if taken == playerColor {
			t.Fatalf("cannon colour %v is already used by shots or the board", taken)
		}
	}
}

func TestDrawSkipsTooSmallTerminal(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 10, 5)

	g.Draw(screen) // must not panic or write past the screen

	cells, _, _ := screen.GetContents()
	for _, c := range cells {
		if _, bg, _ := c.Style.Decompose(); bg != tcell.ColorDefault {
			t.Fatal("nothing should be painted on a terminal smaller than the board")
		}
	}
}

func TestDrawShowsPauseReason(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	g.pauseReason = "Paused by PI"
	g.Pause()

	g.Draw(screen)

	if !screenContains(screen, "Paused by PI") {
		t.Fatal("pause overlay should show the pause reason")
	}
}

// TestSimulatedPlayKeepsInvariants drives the game through many steps with
// the cannon firing constantly, checking the state never goes inconsistent:
// shots stay on the board, the alive counter matches the grid, and the
// formation never leaves the board.
func TestSimulatedPlayKeepsInvariants(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)

	for step := 0; step < 5000 && !g.gameOver; step++ {
		g.fire()
		g.stepBullets()
		if step%4 == 0 && !g.gameOver {
			g.stepAliens()
		}
		if step%10 == 0 && !g.gameOver {
			g.alienFire()
		}
		if step%50 == 0 {
			g.move(1)
		}
		g.Draw(screen)

		for _, b := range g.bullets {
			if b.pos.X < 0 || b.pos.X >= boardColumns || b.pos.Y < 0 || b.pos.Y >= boardRows {
				t.Fatalf("step %d: shot off-board at %v", step, b.pos)
			}
		}
		alive := 0
		for r := range g.aliens {
			for c := range g.aliens[r] {
				if g.aliens[r][c] {
					alive++
				}
			}
		}
		if alive != g.alive {
			t.Fatalf("step %d: alive counter %d, grid has %d", step, g.alive, alive)
		}
		if g.alive > 0 {
			minX, maxX, _ := g.formationBounds()
			if minX < 0 || maxX >= boardColumns {
				t.Fatalf("step %d: formation x range %d..%d leaves the board", step, minX, maxX)
			}
		}
		if got := countBullets(g, true); got > maxPlayerBullets {
			t.Fatalf("step %d: %d player shots in flight, want at most %d", step, got, maxPlayerBullets)
		}
		if got := countBullets(g, false); got > maxAlienBullets {
			t.Fatalf("step %d: %d alien shots in flight, want at most %d", step, got, maxAlienBullets)
		}
	}

	if g.score == 0 {
		t.Fatal("constant fire over thousands of steps should have scored")
	}
}

func screenContains(screen tcell.SimulationScreen, text string) bool {
	cells, w, h := screen.GetContents()
	for y := 0; y < h; y++ {
		var row []rune
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				row = append(row, c.Runes[0])
			} else {
				row = append(row, ' ')
			}
		}
		if containsRunes(row, []rune(text)) {
			return true
		}
	}
	return false
}

func containsRunes(hay, needle []rune) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		match := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
