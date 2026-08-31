package tetris

import (
	"fmt"
	core "github.com/terminalika/terminalika-core"

	"github.com/gdamore/tcell/v2"
)

const cellWidth = 2

type boardOrigin struct {
	leftX int
	topY  int
}

// Draw renders the board, active piece and status bar.
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

	g.drawBoard(screen, origin)
	g.drawPiece(screen, origin)
	g.drawStatus(screen, origin)

	screen.Show()
}

func (g *Game) drawBoard(screen tcell.Screen, origin boardOrigin) {
	emptyStyle := tcell.StyleDefault.
		Background(tcell.ColorDarkSlateGray).
		Foreground(tcell.ColorBlack)

	for y := 0; y < boardRows; y++ {
		for x := 0; x < boardColumns; x++ {
			style := emptyStyle
			if g.board[y][x].filled {
				color := g.board[y][x].color
				style = tcell.StyleDefault.Background(color).Foreground(color)
			}
			fillCell(screen, origin, Point{X: x, Y: y}, style)
		}
	}
}

func (g *Game) drawPiece(screen tcell.Screen, origin boardOrigin) {
	if g.current == nil {
		return
	}

	color := defs[g.current.kind].color
	style := tcell.StyleDefault.Background(color).Foreground(color)

	for _, p := range g.current.cells() {
		if p.Y >= 0 {
			fillCell(screen, origin, p, style)
		}
	}
}

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin) {
	statusStyle := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack)

	boardW := boardColumns * cellWidth
	centerX := origin.leftX + boardW/2

	pauseText := "PAUSED"
	if g.pauseReason != "" {
		pauseText = g.pauseReason
	}

	status := fmt.Sprintf("SCORE: %d", g.score)
	if g.store.Persistent() {
		status += fmt.Sprintf("  BEST: %d", g.best)
	}
	status += fmt.Sprintf("  LINES: %d", g.lines)
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.gameOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := g.hint()
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardRows+2, statusStyle, hint)

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

func fillCell(screen tcell.Screen, origin boardOrigin, p Point, style tcell.Style) {
	x := origin.leftX + p.X*cellWidth
	y := origin.topY + p.Y
	for i := 0; i < cellWidth; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}
}

func emitStr(screen tcell.Screen, x, y int, style tcell.Style, str string) {
	for _, r := range str {
		screen.SetContent(x, y, r, nil, style)
		x++
	}
}

func (g *Game) hint() string {
	return core.JoinHints("Arrows/WASD: move & rotate", "X: drop", g.keys.PauseHint(), g.keys.ResetHint("reset"), g.keys.LeaveHint())
}

// NeededSize is the board with the status line two rows above it and the
// hint line two rows below (see drawStatus), as wide as the widest of them.
func (g *Game) NeededSize() core.Size {
	// The longest status drawStatus can produce, with room for the numbers.
	status := "SCORE: 999999  BEST: 999999  LINES: 9999 - GAME OVER"
	return core.Size{
		Cols: core.Widest(boardColumns*cellWidth, status, g.hint()),
		Rows: boardRows + 5,
	}
}
