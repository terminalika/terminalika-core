package snake

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
)

const cellWidth = 2

type boardOrigin struct {
	leftX int
	topY  int
}

// Draw renders the board, food, snake and status bar.
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
	g.drawFood(screen, origin)
	g.drawSnake(screen, origin)
	g.drawStatus(screen, origin)

	screen.Show()
}

func (g *Game) drawBoard(screen tcell.Screen, origin boardOrigin) {
	emptyStyle := tcell.StyleDefault.
		Background(tcell.ColorDarkSlateGray).
		Foreground(tcell.ColorBlack)

	for y := 0; y < boardRows; y++ {
		for x := 0; x < boardColumns; x++ {
			fillCell(screen, origin, Point{X: x, Y: y}, emptyStyle)
		}
	}
}

func (g *Game) drawSnake(screen tcell.Screen, origin boardOrigin) {
	bodyStyle := tcell.StyleDefault.Background(tcell.ColorGreen).Foreground(tcell.ColorGreen)
	headStyle := tcell.StyleDefault.Background(tcell.ColorLime).Foreground(tcell.ColorLime)

	for i, p := range g.snake {
		style := bodyStyle
		if i == 0 {
			style = headStyle
		}
		fillCell(screen, origin, p, style)
	}
}

func (g *Game) drawFood(screen tcell.Screen, origin boardOrigin) {
	if !g.hasFood {
		return
	}
	style := tcell.StyleDefault.Background(tcell.ColorRed).Foreground(tcell.ColorRed)
	fillCell(screen, origin, g.food, style)
}

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin) {
	statusStyle := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack)

	boardW := boardColumns * cellWidth
	centerX := origin.leftX + boardW/2

	status := fmt.Sprintf("SCORE: %d  BEST: %d", g.score, g.best)
	switch {
	case g.paused:
		status += " - PAUSED"
	case g.gameOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := "Arrows/WASD: move  Space: pause  R: reset  Esc: menu"
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardRows+1, statusStyle, hint)

	if g.paused {
		overlay := "PAUSED"
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
