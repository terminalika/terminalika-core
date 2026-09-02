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

	w, h := screen.Size()
	boardW := boardColumns * cellWidth
	if w < boardW || h < boardRows {
		return // terminal too small to draw
	}

	// The whole block is centred: status line, blank, board, blank, hint lines.
	hintLines := core.HintLines(w, g.hints()...)
	origin := boardOrigin{
		leftX: (w - boardW) / 2,
		topY:  (h-(boardRows+4+len(hintLines)))/2 + 2,
	}

	g.drawBoard(screen, origin)
	g.drawPiece(screen, origin)
	g.drawStatus(screen, origin, hintLines)
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

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin, hintLines []string) {
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

	for i, line := range hintLines {
		emitStr(screen, centerX-len(line)/2, origin.topY+boardRows+2+i, statusStyle, line)
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

func (g *Game) hints() []string {
	return []string{"Arrows/WASD: move & rotate", "X: drop", g.keys.PauseHint(), g.keys.ResetHint("reset"), g.keys.SwitchHint(), g.keys.LeaveHint()}
}

// NeededSize is the board with the status line two rows above it and the
// hint two rows below (see drawStatus), as wide as the widest of them - or,
// when avail is narrower than the one-line hint, the hint wrapped onto more
// rows (core.HintLayout).
func (g *Game) NeededSize(avail core.Size) core.Size {
	// The longest status drawStatus can produce, with room for the numbers.
	status := "SCORE: 999999  BEST: 999999  LINES: 9999 - GAME OVER"
	cols, hintRows := core.HintLayout(avail.Cols, core.Widest(boardColumns*cellWidth, status), g.hints()...)
	return core.Size{Cols: cols, Rows: boardRows + 4 + hintRows}
}
