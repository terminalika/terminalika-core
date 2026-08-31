package dino

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
)

const cellWidth = 1

type boardOrigin struct {
	leftX int
	topY  int
}

// Sprites, one string per row from the top. The dino's legs alternate
// between two frames while it runs and tuck in while it is in the air.
var (
	dinoHead     = "▄█"
	dinoLegs     = [2]string{"▐▙", "▟▌"}
	dinoLegsAir  = "▐▌"
	birdFrames   = [2]string{"▝▘", "▗▖"}
	cloudSprite  = "░░░░"
	cloudRows    = [2]int{1, 3}
	cloudOffsets = [2]int{5, 29}
)

// Draw renders the desert, the obstacles, the dino and the status bar.
func (g *Game) Draw(screen tcell.Screen) {
	screen.Clear()
	g.overlayOn = false

	w, h := screen.Size()
	boardW := boardColumns * cellWidth
	if w < boardW || h < boardRows {
		return // terminal too small to draw
	}

	origin := boardOrigin{
		leftX: (w - boardW) / 2,
		topY:  (h - boardRows) / 2,
	}

	g.drawSky(screen, origin)
	g.drawGround(screen, origin)
	g.drawObstacles(screen, origin)
	g.drawDino(screen, origin)
	g.drawStatus(screen, origin)

	screen.Show()
}

func (g *Game) drawSky(screen tcell.Screen, origin boardOrigin) {
	sky := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorBlack)
	for y := 0; y < boardRows; y++ {
		for x := 0; x < boardColumns; x++ {
			screen.SetContent(origin.leftX+x, origin.topY+y, ' ', nil, sky)
		}
	}

	// Two clouds drift by at a quarter of the ground's speed and wrap.
	cloud := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorDarkGray)
	for i, row := range cloudRows {
		x := (cloudOffsets[i] - g.distance/4) % boardColumns
		if x < 0 {
			x += boardColumns
		}
		for j, r := range []rune(cloudSprite) {
			screen.SetContent(origin.leftX+(x+j)%boardColumns, origin.topY+row, r, nil, cloud)
		}
	}
}

func (g *Game) drawGround(screen tcell.Screen, origin boardOrigin) {
	line := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorGray)
	pebble := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorDarkGray)
	for x := 0; x < boardColumns; x++ {
		r, style := '─', line
		if (x+g.distance)%11 == 0 {
			r, style = '.', pebble
		}
		screen.SetContent(origin.leftX+x, origin.topY+groundRow, r, nil, style)
	}
}

func (g *Game) drawObstacles(screen tcell.Screen, origin boardOrigin) {
	cactus := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorGreen)
	bird := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	flap := (g.distance / 4) % 2

	for _, o := range g.obstacles {
		s := shapes[o.kind]
		if s.isBird {
			// Birds are one sprite row wide; their cells all share a dy.
			for i, c := range s.cells {
				x := o.x + c.dx
				if x < 0 || x >= boardColumns {
					continue
				}
				r := []rune(birdFrames[flap])[i%2]
				screen.SetContent(origin.leftX+x, origin.topY+groundRow-1-c.dy, r, nil, bird)
			}
			continue
		}
		for _, c := range s.cells {
			x := o.x + c.dx
			if x < 0 || x >= boardColumns {
				continue
			}
			r := 'ψ'
			if c.dy > 0 || tallColumn(s, c.dx) {
				r = 'Ψ'
			}
			screen.SetContent(origin.leftX+x, origin.topY+groundRow-1-c.dy, r, nil, cactus)
		}
	}
}

// tallColumn reports whether a cactus shape has a cell above ground level
// in the given column, so its base is drawn as a trunk rather than a sprout.
func tallColumn(s shape, dx int) bool {
	for _, c := range s.cells {
		if c.dx == dx && c.dy > 0 {
			return true
		}
	}
	return false
}

func (g *Game) drawDino(screen tcell.Screen, origin boardOrigin) {
	style := tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	if g.gameOver {
		style = style.Foreground(tcell.ColorRed)
	}

	legs := dinoLegsAir
	if !g.airborne {
		legs = dinoLegs[(g.distance/3)%2]
	}
	feet := origin.topY + groundRow - 1 - g.altitude()
	emitStr(screen, origin.leftX+dinoX, feet-1, style, dinoHead)
	emitStr(screen, origin.leftX+dinoX, feet, style, legs)
}

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin) {
	statusStyle := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack)
	if g.flashTicks > 0 && (g.flashTicks/2)%2 == 0 {
		statusStyle = statusStyle.Foreground(tcell.ColorYellow)
	}

	boardW := boardColumns * cellWidth
	centerX := origin.leftX + boardW/2

	pauseText := "PAUSED"
	if g.pauseReason != "" {
		pauseText = g.pauseReason
	}

	status := fmt.Sprintf("SCORE: %05d", g.score)
	if g.store.Persistent() {
		status += fmt.Sprintf("  BEST: %05d", g.best)
	}
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.gameOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := g.hint()
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardRows+1, statusStyle, hint)

	if g.paused {
		g.band(screen, centerX-len(pauseText)/2, origin.topY+boardRows/2, pauseText)
	} else if g.gameOver {
		overlay := "GAME OVER"
		g.band(screen, centerX-len(overlay)/2, origin.topY+boardRows/2, overlay)
	}
}

// band paints the game's own overlay - white on dark red, one row - and
// remembers where, for OverlayArea.
func (g *Game) band(screen tcell.Screen, x, y int, text string) {
	style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkRed)
	emitStr(screen, x, y, style, text)
	g.overlay = core.Rect{X: x, Y: y, W: len(text), H: 1}
	g.overlayOn = true
}

// OverlayArea reports the PAUSED / GAME OVER band the last Draw painted.
func (g *Game) OverlayArea() (core.Rect, bool) {
	return g.overlay, g.overlayOn
}

func emitStr(screen tcell.Screen, x, y int, style tcell.Style, str string) {
	for _, r := range str {
		screen.SetContent(x, y, r, nil, style)
		x++
	}
}

func (g *Game) hint() string {
	return core.JoinHints("Up/W/K: jump", g.keys.PauseHint(), g.keys.ResetHint("reset"), g.keys.LeaveHint())
}

// NeededSize is the board with the status line two rows above it and the
// hint line one row below (see drawStatus), as wide as the widest of them.
func (g *Game) NeededSize() core.Size {
	status := "SCORE: 99999  BEST: 99999 - GAME OVER"
	return core.Size{
		Cols: core.Widest(boardColumns*cellWidth, status, g.hint()),
		Rows: boardRows + 4,
	}
}
