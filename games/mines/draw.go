package mines

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
)

const cellWidth = 2

type boardOrigin struct {
	leftX int
	topY  int
}

var (
	hiddenStyle   = tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorDarkSlateGray)
	flagStyle     = tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorRed)
	wrongStyle    = tcell.StyleDefault.Foreground(tcell.ColorRed)
	mineStyle     = tcell.StyleDefault.Foreground(tcell.ColorWhite)
	explodedStyle = tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorWhite)
	cursorHidden  = tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack)
	cursorOpen    = tcell.StyleDefault.Background(tcell.ColorOlive).Foreground(tcell.ColorBlack)

	// numberColors follows the classic palette, 1 through 8.
	numberColors = [9]tcell.Color{
		tcell.ColorDefault,
		tcell.ColorBlue,
		tcell.ColorGreen,
		tcell.ColorRed,
		tcell.ColorNavy,
		tcell.ColorMaroon,
		tcell.ColorTeal,
		tcell.ColorWhite,
		tcell.ColorGray,
	}
)

// Draw renders the field, the cursor and the status bar.
func (g *Game) Draw(screen tcell.Screen) {
	screen.Clear()
	g.overlayOn = false

	w, h := screen.Size()
	boardW := g.level.Cols * cellWidth
	if w < boardW || h < g.level.Rows {
		return // terminal too small to draw
	}

	origin := boardOrigin{
		leftX: (w - boardW) / 2,
		topY:  (h - g.level.Rows) / 2,
	}

	g.drawField(screen, origin, time.Now())
	g.drawStatus(screen, origin)

	screen.Show()
}

// drawField paints every cell as it currently shows: a reveal or a planted
// flag whose showAt is still ahead is drawn as it was, so floods ripple,
// mines go off in turn and flags get planted one by one.
func (g *Game) drawField(screen tcell.Screen, origin boardOrigin, now time.Time) {
	for y := 0; y < g.level.Rows; y++ {
		for x := 0; x < g.level.Cols; x++ {
			c := g.cells[y][x]
			shown := !c.showAt.After(now)
			text, style := "  ", hiddenStyle
			switch {
			case c.revealed && shown && c.exploded:
				text, style = "* ", explodedStyle
			case c.revealed && shown && c.mine:
				text, style = "* ", mineStyle
			case c.revealed && shown && c.flagged: // a wrong flag, shown by a hit
				text, style = "X ", wrongStyle
			case c.revealed && shown && c.adjacent > 0:
				text, style = fmt.Sprintf("%d ", c.adjacent), tcell.StyleDefault.Foreground(numberColors[c.adjacent])
			case c.revealed && shown:
				text, style = "  ", tcell.StyleDefault
			case c.flagged && shown:
				text, style = "▲ ", flagStyle
			}
			if x == g.cx && y == g.cy && !g.gameOver {
				if c.revealed && shown {
					style = cursorOpen
				} else {
					style = cursorHidden
				}
			}
			emitStr(screen, origin.leftX+x*cellWidth, origin.topY+y, style, text)
		}
	}
}

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin) {
	statusStyle := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack)

	boardW := g.level.Cols * cellWidth
	centerX := origin.leftX + boardW/2

	pauseText := "PAUSED"
	if g.pauseReason != "" {
		pauseText = g.pauseReason
	}

	status := fmt.Sprintf("%s  SCORE: %d", strings.ToUpper(g.level.Name), g.score)
	if g.store.Persistent() {
		status += fmt.Sprintf("  BEST: %d", g.best)
	}
	status += fmt.Sprintf("  FLAGS: %d/%d  TIME: %s", g.flagCount, g.level.Mines, clock(g.runTime()))
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.won:
		status += " - CLEARED"
	case g.gameOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := g.hint()
	emitStr(screen, centerX-len([]rune(hint))/2, origin.topY+g.level.Rows+1, statusStyle, hint)

	if g.paused {
		g.band(screen, centerX-len(pauseText)/2, origin.topY+g.level.Rows/2, pauseText)
	} else if g.gameOver && !g.won {
		overlay := "GAME OVER"
		g.band(screen, centerX-len(overlay)/2, origin.topY+g.level.Rows/2, overlay)
	}
}

// clock formats a run time as m:ss.
func clock(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
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
	return core.JoinHints("Enter: open", "F: flag", "1-3: level", g.keys.PauseHint(), g.keys.ResetHint("reset"), g.keys.LeaveHint())
}

// NeededSize is the field with the status line two rows above it and the
// hint line one row below (see drawStatus), as wide as the widest of them.
func (g *Game) NeededSize() core.Size {
	status := "INTERMEDIATE  SCORE: 99999  BEST: 99999  FLAGS: 99/99  TIME: 99:59 - GAME OVER"
	return core.Size{
		Cols: core.Widest(g.level.Cols*cellWidth, status, g.hint()),
		Rows: g.level.Rows + 4,
	}
}
