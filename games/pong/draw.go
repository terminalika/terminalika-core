package pong

import (
	"fmt"
	core "github.com/terminalika/terminalika-core"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// cellWidth is two columns so a cell is roughly square: the ball moves one
// cell per tick on both axes, like tpong's 2:1 compensation.
const cellWidth = 2

// Paddles are one column thick: the left one hugs the right edge of its
// cell and the right one the left edge, so they face the court.
const paddleWidth = 1

// blinkPeriod is the ball's blink while a serve is pending.
const blinkPeriod = 150 * time.Millisecond

const (
	boardColor  = tcell.ColorDarkSlateGray
	netColor    = tcell.ColorGray
	digitColor  = tcell.ColorSlateGray
	paddleColor = tcell.ColorWhite
	ballColor   = tcell.ColorLime
)

// digitGlyphs is the 3x5 block font for the court's big scores, in the
// spirit of tpong's 7-segment digits but sized for a small court. Every
// pixel is one cell.
var digitGlyphs = map[rune][5]string{
	'0': {"###", "# #", "# #", "# #", "###"},
	'1': {" # ", "## ", " # ", " # ", "###"},
	'2': {"###", "  #", "###", "#  ", "###"},
	'3': {"###", "  #", "###", "  #", "###"},
	'4': {"# #", "# #", "###", "  #", "  #"},
	'5': {"###", "#  ", "###", "  #", "###"},
	'6': {"###", "#  ", "###", "# #", "###"},
	'7': {"###", "  #", "  #", "  #", "  #"},
	'8': {"###", "# #", "###", "# #", "###"},
	'9': {"###", "# #", "###", "  #", "###"},
}

const (
	digitWidth  = 3
	digitHeight = 5
	digitGap    = 1
	digitTop    = 1 // court row the scores hang from
)

type boardOrigin struct {
	leftX int
	topY  int
}

// Draw renders the court, scores, paddles, ball and status bar - or the
// setup screen before a match is set up.
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
	if g.phase == phaseSetup {
		g.drawSetup(screen, origin)
	} else {
		g.drawNet(screen, origin)
		g.drawScores(screen, origin)
		g.drawPaddles(screen, origin)
		g.drawBall(screen, origin)
	}
	g.drawStatus(screen, origin)

	screen.Show()
}

func (g *Game) drawBoard(screen tcell.Screen, origin boardOrigin) {
	emptyStyle := tcell.StyleDefault.
		Background(boardColor).
		Foreground(tcell.ColorBlack)

	for y := 0; y < boardRows; y++ {
		for x := 0; x < boardColumns; x++ {
			fillCell(screen, origin, Point{X: x, Y: y}, emptyStyle)
		}
	}
}

// drawNet draws the dashed centre line.
func (g *Game) drawNet(screen tcell.Screen, origin boardOrigin) {
	style := tcell.StyleDefault.Background(netColor).Foreground(netColor)
	for y := 0; y < boardRows; y += 2 {
		fillCell(screen, origin, Point{X: boardColumns / 2, Y: y}, style)
	}
}

// drawScores draws each side's points in big block digits at the top of
// its half of the court. They sit under the paddles and ball.
func (g *Game) drawScores(screen tcell.Screen, origin boardOrigin) {
	style := tcell.StyleDefault.Background(digitColor).Foreground(digitColor)
	drawDigits(screen, origin, boardColumns/4, g.points[0], style)
	drawDigits(screen, origin, boardColumns-boardColumns/4-1, g.points[1], style)
}

// drawDigits draws n centred on court column centerX.
func drawDigits(screen tcell.Screen, origin boardOrigin, centerX, n int, style tcell.Style) {
	text := strconv.Itoa(n)
	total := len(text)*digitWidth + (len(text)-1)*digitGap
	x := centerX - total/2
	for _, r := range text {
		glyph, ok := digitGlyphs[r]
		if !ok {
			continue
		}
		for row, line := range glyph {
			for col, px := range line {
				if px == '#' {
					fillCell(screen, origin, Point{X: x + col, Y: digitTop + row}, style)
				}
			}
		}
		x += digitWidth + digitGap
	}
}

func (g *Game) drawPaddles(screen tcell.Screen, origin boardOrigin) {
	style := tcell.StyleDefault.Background(paddleColor).Foreground(paddleColor)
	for y := 0; y < paddleHeight; y++ {
		fillCellEdge(screen, origin, Point{X: leftPaddleX, Y: g.paddles[0] + y}, style, true)
		fillCellEdge(screen, origin, Point{X: rightPaddleX, Y: g.paddles[1] + y}, style, false)
	}
}

// drawBall draws the ball; while a serve is pending it blinks at the centre.
func (g *Game) drawBall(screen tcell.Screen, origin boardOrigin) {
	if g.phase == phaseOver {
		return
	}
	if g.phase == phaseServing && !g.paused && (time.Now().UnixMilli()/blinkPeriod.Milliseconds())%2 == 1 {
		return
	}
	style := tcell.StyleDefault.Background(ballColor).Foreground(ballColor)
	fillCell(screen, origin, g.ball, style)
}

// drawSetup renders the pre-match screen: the opponent and the match length,
// one row selected.
func (g *Game) drawSetup(screen tcell.Screen, origin boardOrigin) {
	boardW := boardColumns * cellWidth
	centerX := origin.leftX + boardW/2

	titleStyle := tcell.StyleDefault.Background(boardColor).Foreground(tcell.ColorAqua).Bold(true)
	subtitleStyle := tcell.StyleDefault.Background(boardColor).Foreground(tcell.ColorSilver)
	itemStyle := tcell.StyleDefault.Background(boardColor).Foreground(tcell.ColorWhite)
	selectedStyle := tcell.StyleDefault.Background(tcell.ColorAqua).Foreground(tcell.ColorBlack)

	emitStr(screen, centerX-2, origin.topY+2, titleStyle, "PONG")
	emitStr(screen, centerX-len("choose your challenge")/2, origin.topY+3, subtitleStyle, "choose your challenge")

	rows := [setupRows]string{
		setupLine("MODE", modeNames[g.mode]),
		setupLine("FIRST TO", strconv.Itoa(g.target)),
	}
	startX := centerX - len(rows[0])/2
	for i, line := range rows {
		style := itemStyle
		if i == g.setupRow {
			style = selectedStyle
		}
		emitStr(screen, startX, origin.topY+6+i*2, style, line)
	}

	hint := "Up/Down: select  Left/Right: change  Enter: play"
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardRows-3, subtitleStyle, hint)
}

// setupLine formats one setup row as "LABEL     < value >" with the value
// centred in a fixed field so the rows line up.
func setupLine(label, value string) string {
	const field = 10
	pad := field - len(value)
	if pad < 0 {
		pad = 0
	}
	left := pad / 2
	return fmt.Sprintf("%-8s  < %s%s%s >", label, strings.Repeat(" ", left), value, strings.Repeat(" ", pad-left))
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

	var status string
	switch {
	case g.phase == phaseSetup:
		if g.store.Persistent() {
			status = fmt.Sprintf("BEST: %d", g.best)
		}
	case g.mode.bot():
		status = fmt.Sprintf("YOU %d : %d BOT  SCORE: %d", g.points[0], g.points[1], g.score)
		if g.store.Persistent() {
			status += fmt.Sprintf("  BEST: %d", g.best)
		}
	default:
		status = fmt.Sprintf("P1 %d : %d P2", g.points[0], g.points[1])
	}
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.phase == phaseOver:
		status += " - GAME OVER"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := g.hint()
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardRows+1, statusStyle, hint)

	if g.paused {
		g.band(screen, centerX-len(pauseText)/2, origin.topY+boardRows/2, pauseText)
	} else if g.phase == phaseOver {
		overlay := g.winnerText()
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

// OverlayArea reports the PAUSED / winner band the last Draw painted.
func (g *Game) OverlayArea() (core.Rect, bool) {
	return g.overlay, g.overlayOn
}

// winnerText names the winner of the finished match.
func (g *Game) winnerText() string {
	if g.mode.bot() {
		if g.winner == 0 {
			return "YOU WIN"
		}
		return "BOT WINS"
	}
	return fmt.Sprintf("P%d WINS", g.winner+1)
}

// fillCell paints a whole cell with the style's background.
func fillCell(screen tcell.Screen, origin boardOrigin, p Point, style tcell.Style) {
	x := origin.leftX + p.X*cellWidth
	y := origin.topY + p.Y
	for i := 0; i < cellWidth; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}
}

// fillCellEdge paints paddleWidth columns at the right (or left) edge of a
// cell, for the one-column-thick paddles.
func fillCellEdge(screen tcell.Screen, origin boardOrigin, p Point, style tcell.Style, rightEdge bool) {
	x := origin.leftX + p.X*cellWidth
	if rightEdge {
		x += cellWidth - paddleWidth
	}
	y := origin.topY + p.Y
	for i := 0; i < paddleWidth; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}
}

func emitStr(screen tcell.Screen, x, y int, style tcell.Style, str string) {
	for _, r := range str {
		screen.SetContent(x, y, r, nil, style)
		x++
	}
}

// hint is the hint line for the current phase and mode.
func (g *Game) hint() string {
	switch {
	case g.phase == phaseSetup:
		return g.hintSetup()
	case g.phase == phaseOver:
		return g.hintOver()
	case g.mode.bot():
		return g.hintBot()
	default:
		return g.hintTwoPlayer()
	}
}

func (g *Game) hintSetup() string {
	return core.JoinHints("Arrows/WASD: choose", "Enter: play", g.keys.LeaveHint())
}

func (g *Game) hintOver() string {
	// Enter is the game's own rematch key; the launcher's reset does the same.
	rematch := "Enter"
	if g.keys.Reset != "" {
		rematch += "/" + g.keys.Reset
	}
	return core.JoinHints(rematch+": rematch", "M: setup", g.keys.LeaveHint())
}

func (g *Game) hintBot() string {
	return core.JoinHints("W/S or Up/Down: move", g.keys.PauseHint(), g.keys.ResetHint("rematch"), g.keys.LeaveHint())
}

func (g *Game) hintTwoPlayer() string {
	return core.JoinHints("W/S: left", "Up/Down: right", g.keys.PauseHint(), g.keys.ResetHint("reset"), g.keys.LeaveHint())
}

// NeededSize is the court with the status line two rows above it and the
// hint line one row below (see drawStatus), as wide as the widest line any
// phase draws there.
func (g *Game) NeededSize() core.Size {
	// The longest status drawStatus can produce, with room for the numbers.
	status := "YOU 99 : 99 BOT  SCORE: 99999  BEST: 99999 - GAME OVER"
	return core.Size{
		Cols: core.Widest(boardColumns*cellWidth, status, g.hintSetup(), g.hintOver(), g.hintBot(), g.hintTwoPlayer()),
		Rows: boardRows + 4,
	}
}
