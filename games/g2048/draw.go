package g2048

import (
	"fmt"
	"strconv"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
)

// Tiles are tileW cells wide and tileH tall and sit flush against each
// other, so the board is boardW x boardH.
const (
	tileW  = 7
	tileH  = 3
	boardW = size * tileW
	boardH = size * tileH
)

type boardOrigin struct {
	leftX int
	topY  int
}

// tileStyles is the original game's palette, one entry per power of two; a
// tile past the table gets beyondStyle. Empty cells are not in it: they are
// left in the terminal's own colours.
var tileStyles = map[int]tcell.Style{
	2:    tcell.StyleDefault.Background(tcell.NewRGBColor(0xee, 0xe4, 0xda)).Foreground(tcell.NewRGBColor(0x77, 0x6e, 0x65)),
	4:    tcell.StyleDefault.Background(tcell.NewRGBColor(0xed, 0xe0, 0xc8)).Foreground(tcell.NewRGBColor(0x77, 0x6e, 0x65)),
	8:    tcell.StyleDefault.Background(tcell.NewRGBColor(0xf2, 0xb1, 0x79)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	16:   tcell.StyleDefault.Background(tcell.NewRGBColor(0xf5, 0x95, 0x63)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	32:   tcell.StyleDefault.Background(tcell.NewRGBColor(0xf6, 0x7c, 0x5f)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	64:   tcell.StyleDefault.Background(tcell.NewRGBColor(0xf6, 0x5e, 0x3b)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	128:  tcell.StyleDefault.Background(tcell.NewRGBColor(0xed, 0xcf, 0x72)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	256:  tcell.StyleDefault.Background(tcell.NewRGBColor(0xed, 0xcc, 0x61)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	512:  tcell.StyleDefault.Background(tcell.NewRGBColor(0xed, 0xc8, 0x50)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	1024: tcell.StyleDefault.Background(tcell.NewRGBColor(0xed, 0xc5, 0x3f)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
	2048: tcell.StyleDefault.Background(tcell.NewRGBColor(0xed, 0xc2, 0x2e)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2)),
}

var beyondStyle = tcell.StyleDefault.Background(tcell.NewRGBColor(0x3c, 0x3a, 0x32)).Foreground(tcell.NewRGBColor(0xf9, 0xf6, 0xf2))

func styleFor(value int) tcell.Style {
	if s, ok := tileStyles[value]; ok {
		return s
	}
	return beyondStyle
}

// Draw renders the board, the tiles and the status bar.
func (g *Game) Draw(screen tcell.Screen) {
	screen.Clear()
	g.overlayOn = false

	w, h := screen.Size()
	if w < boardW || h < boardH {
		return // terminal too small to draw
	}

	origin := boardOrigin{
		leftX: (w - boardW) / 2,
		topY:  (h - boardH) / 2,
	}

	g.drawTiles(screen, origin)
	g.drawStatus(screen, origin)

	screen.Show()
}

func (g *Game) drawTiles(screen tcell.Screen, origin boardOrigin) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			v := g.board[y][x]
			if v == 0 {
				continue // the terminal's own background shows through
			}
			style := styleFor(v)
			left := origin.leftX + x*tileW
			top := origin.topY + y*tileH
			for dy := 0; dy < tileH; dy++ {
				for dx := 0; dx < tileW; dx++ {
					screen.SetContent(left+dx, top+dy, ' ', nil, style)
				}
			}
			label := strconv.Itoa(v)
			if v >= winTile {
				style = style.Bold(true)
			}
			emitStr(screen, left+(tileW-len(label))/2, top+tileH/2, style, label)
		}
	}
}

func (g *Game) drawStatus(screen tcell.Screen, origin boardOrigin) {
	statusStyle := tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack)

	centerX := origin.leftX + boardW/2

	pauseText := "PAUSED"
	if g.pauseReason != "" {
		pauseText = g.pauseReason
	}

	status := fmt.Sprintf("SCORE: %d", g.score)
	if g.store.Persistent() {
		status += fmt.Sprintf("  BEST: %d", g.best)
	}
	switch {
	case g.paused:
		status += " - " + pauseText
	case g.gameOver:
		status += " - GAME OVER"
	case g.won:
		status += " - 2048!"
	}
	emitStr(screen, centerX-len(status)/2, origin.topY-2, statusStyle, status)

	hint := g.hint()
	emitStr(screen, centerX-len(hint)/2, origin.topY+boardH+1, statusStyle, hint)

	if g.paused {
		g.band(screen, centerX-len(pauseText)/2, origin.topY+boardH/2, pauseText)
	} else if g.gameOver {
		overlay := "GAME OVER"
		g.band(screen, centerX-len(overlay)/2, origin.topY+boardH/2, overlay)
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
	return core.JoinHints("Arrows/WASD/HJKL: slide", g.keys.PauseHint(), g.keys.ResetHint("new game"), g.keys.LeaveHint())
}

// NeededSize is the board with the status line two rows above it and the
// hint line one row below (see drawStatus), as wide as the widest of them.
func (g *Game) NeededSize() core.Size {
	status := "SCORE: 999999  BEST: 999999 - GAME OVER"
	return core.Size{
		Cols: core.Widest(boardW, status, g.hint()),
		Rows: boardH + 4,
	}
}
