package tetris

import (
	"fmt"

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

	status := fmt.Sprintf("SCORE: %d  BEST: %d  LINES: %d", g.score, g.best, g.lines)
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.gameOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := "Arrows/WASD: move & rotate  X: drop  Space: pause  R: reset  Esc: menu"
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardRows+2, statusStyle, hint)

	if g.paused {
		overlay := pauseText
		overlayStyle := tcell.StyleDefault.
			Foreground(tcell.ColorWhite).
			Background(tcell.ColorDarkRed)
		emitStr(screen, centerX-len(overlay)/2, origin.topY+boardRows/2, overlayStyle, overlay)
	} else if g.gameOver {
		overlay := "GAME OVER"
		overlayStyle := tcell.StyleDefault.
			Foreground(tcell.ColorWhite).
			Background(tcell.ColorDarkRed)
		emitStr(screen, centerX-len(overlay)/2, origin.topY+boardRows/2, overlayStyle, overlay)
	}
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
