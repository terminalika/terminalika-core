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
	hiddenStyle = tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorDarkSlateGray)
	flagStyle   = tcell.StyleDefault.Background(tcell.ColorDarkSlateGray).Foreground(tcell.ColorRed)
	// Revealed cells sit on the same slight tint as 2048's board, so the
	// opened part of the field reads as one surface.
	openStyle     = tcell.StyleDefault.Background(core.BoardTint)
	wrongStyle    = openStyle.Foreground(tcell.ColorRed)
	mineStyle     = openStyle.Foreground(tcell.ColorWhite)
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

// Draw renders the picker or the field with its cursor, and the status bar.
//
// The status and hint lines hang on a frame the size of the largest field,
// so they stay put whichever level is up; the picker and smaller fields sit
// centred inside it. A terminal too small for the frame gets the frame
// shrunk to what is actually drawn.
func (g *Game) Draw(screen tcell.Screen) {
	screen.Clear()
	g.overlayOn = false

	w, h := screen.Size()
	areaW, areaH := g.areaSize()
	if w < areaW || h < areaH {
		return // terminal too small to draw
	}
	frameW, frameH := frameSize()
	if w < frameW {
		frameW = areaW
	}
	if h < frameH {
		frameH = areaH
	}

	// The whole block is centred: status line, blank, frame, hint lines.
	hintLines := core.HintLines(w, g.hints()...)
	frame := boardOrigin{
		leftX: (w - frameW) / 2,
		topY:  (h-(frameH+3+len(hintLines)))/2 + 2,
	}
	area := boardOrigin{
		leftX: frame.leftX + (frameW-areaW)/2,
		topY:  frame.topY + (frameH-areaH)/2,
	}

	if g.choosing {
		g.drawPicker(screen, area)
	} else {
		g.drawField(screen, area, time.Now())
	}
	g.drawStatus(screen, frame, frameW, frameH, hintLines)

	screen.Show()
}

// frameSize is the largest field, and so the picker's and every level's
// common frame.
func frameSize() (w, h int) {
	w, h = pickerWidth, pickerRows
	for _, l := range Levels {
		if lw := l.Cols * cellWidth; lw > w {
			w = lw
		}
		if l.Rows > h {
			h = l.Rows
		}
	}
	return w, h
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
				text, style = fmt.Sprintf("%d ", c.adjacent), openStyle.Foreground(numberColors[c.adjacent])
			case c.revealed && shown:
				text, style = "  ", openStyle
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

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin, areaW, areaH int, hintLines []string) {
	statusStyle := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack)

	centerX := origin.leftX + areaW/2

	pauseText := "PAUSED"
	if g.pauseReason != "" {
		pauseText = g.pauseReason
	}

	status := fmt.Sprintf("%s  SCORE: %d", strings.ToUpper(g.level.Name), g.score)
	if g.store.Persistent() {
		status += fmt.Sprintf("  BEST: %d", g.best)
	}
	if !g.choosing {
		status += fmt.Sprintf("  FLAGS: %d/%d  TIME: %s", g.flagCount, g.level.Mines, clock(g.runTime()))
	}
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.won:
		status += " - CLEARED"
	case g.gameOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	for i, line := range hintLines {
		emitStr(screen, centerX-len([]rune(line))/2, origin.topY+areaH+1+i, statusStyle, line)
	}

	if g.paused {
		g.band(screen, centerX-len(pauseText)/2, origin.topY+areaH/2, pauseText)
	} else if g.gameOver && !g.won {
		overlay := "GAME OVER"
		g.band(screen, centerX-len(overlay)/2, origin.topY+areaH/2, overlay)
	}
}

// pickerRows is the picker's height: a title, a blank line and the levels.
const pickerRows = 2 + 3

// drawPicker lists the levels with their best scores, the current one
// highlighted.
func (g *Game) drawPicker(screen tcell.Screen, origin boardOrigin) {
	areaW, _ := g.areaSize()
	title := tcell.StyleDefault.Foreground(tcell.ColorWhite).Bold(true)
	plain := tcell.StyleDefault.Foreground(tcell.ColorWhite)
	highlight := tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlack)

	emitStr(screen, origin.leftX+(areaW-len("MINESWEEPER"))/2, origin.topY, title, "MINESWEEPER")
	for i, l := range Levels {
		line := fmt.Sprintf("  %-13s %2dx%-2d  %2d mines   best %d", l.Name, l.Cols, l.Rows, l.Mines, g.store.Best(l.scoreName))
		style := plain
		if l.Name == g.level.Name {
			line = "▸" + line[1:]
			style = highlight
		}
		line += strings.Repeat(" ", areaW-len([]rune(line)))
		emitStr(screen, origin.leftX, origin.topY+2+i, style, line)
	}
}

// areaSize is what Draw centres and hangs the status and hint lines on:
// the picker or the field.
func (g *Game) areaSize() (w, h int) {
	if g.choosing {
		return pickerWidth, pickerRows
	}
	return g.level.Cols * cellWidth, g.level.Rows
}

// pickerWidth fits the longest picker line with room for a best score.
const pickerWidth = 44

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

func (g *Game) hints() []string {
	if g.choosing {
		return g.pickerHints()
	}
	return g.fieldHints()
}

func (g *Game) pickerHints() []string {
	return []string{"Up/Down: choose", "Enter: start", g.keys.PauseHint(), g.keys.SwitchHint(), g.keys.LeaveHint()}
}

func (g *Game) fieldHints() []string {
	return []string{"Enter: open", "F: flag", g.keys.PauseHint(), g.keys.ResetHint("levels"), g.keys.SwitchHint(), g.keys.LeaveHint()}
}

// NeededSize is the largest field - the level is picked after the size is
// asked for - with the status line two rows above it and the hint one row
// below (see drawStatus), as wide as the widest of them; when avail is
// narrower than the one-line hint, the hint wraps onto more rows instead
// (core.HintLayout). Both the picker's and the field's hints must fit, so
// the size is the larger of the two layouts.
func (g *Game) NeededSize(avail core.Size) core.Size {
	frameW, frameH := frameSize()
	status := "INTERMEDIATE  SCORE: 99999  BEST: 99999  FLAGS: 99/99  TIME: 99:59 - GAME OVER"
	frameW = core.Widest(frameW, status)
	pickerCols, pickerRows := core.HintLayout(avail.Cols, frameW, g.pickerHints()...)
	fieldCols, fieldRows := core.HintLayout(avail.Cols, frameW, g.fieldHints()...)
	return core.Size{
		Cols: max(pickerCols, fieldCols),
		Rows: frameH + 3 + max(pickerRows, fieldRows),
	}
}
