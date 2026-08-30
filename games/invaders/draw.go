package invaders

import (
	"fmt"
	core "github.com/terminalika/terminalika-core"
	"time"

	"github.com/gdamore/tcell/v2"
)

// cellWidth is three columns so every cell can hold a small ASCII sprite.
const cellWidth = 3

type boardOrigin struct {
	leftX int
	topY  int
}

// Sprites are exactly cellWidth runes wide and drawn in colour on the board
// background, so the board reads as a ship and invaders rather than blocks.
// They are plain ASCII on purpose: box-drawing and block characters count as
// double width in some terminals and would break the layout.
const (
	cannonSprite     = `/^\`
	playerShotSprite = ` | `
	alienShotSprite  = ` ! `
)

// alienSprites holds two animation frames per formation row, top to bottom.
// The formation alternates frames on every step, like the original's walk.
var alienSprites = [alienRows][2]string{
	{`{@}`, `(@)`},
	{`/o\`, `\o/`},
	{`=o=`, `-o-`},
	{`[o]`, `(o)`},
}

// burstSprites are the two frames of a hit's explosion: a flash, then the
// debris drifting apart. The cannon's wreck is bigger and angrier.
var burstSprites = map[burstKind][2]string{
	burstAlien:  {`\*/`, `- -`},
	burstCannon: {`*^*`, `\*/`},
}

// burstColors colour each kind of burst.
var burstColors = map[burstKind]tcell.Color{
	burstAlien:  tcell.ColorOrange,
	burstCannon: tcell.ColorRed,
}

// rowColors colours each formation row, top to bottom. None of them is close
// to playerColor, so the cannon always stands out from the aliens - even when
// the lowest row is right on top of it.
var rowColors = [alienRows]tcell.Color{
	tcell.ColorFuchsia,
	tcell.ColorPurple,
	tcell.ColorAqua,
	tcell.ColorGreen,
}

// playerColor is the cannon's colour: unique on the board (aliens are
// magenta/purple/aqua/green, shots white/red).
const playerColor = tcell.ColorYellow

// boardColor is the background every sprite is drawn on.
const boardColor = tcell.ColorDarkSlateGray

// Draw renders the board, formation, shots, cannon and status bar.
func (g *Game) Draw(screen tcell.Screen) {
	screen.Clear()

	w, h := screen.Size()
	boardW := boardColumns * cellWidth
	if w < boardW || h < boardRows {
		return // terminal too small to draw
	}

	now := time.Now()
	origin := boardOrigin{
		leftX: (w-boardW)/2 + g.shakeOffset(now),
		topY:  (h - boardRows) / 2,
	}

	g.drawBoard(screen, origin)
	g.drawAliens(screen, origin)
	g.drawBullets(screen, origin)
	g.drawPlayer(screen, origin)
	g.drawEffects(screen, origin, now)
	g.drawStatus(screen, origin)

	screen.Show()
}

// shakeOffset jolts the board one column left and right, alternating every
// few frames, while a cannon hit's shake is in effect.
func (g *Game) shakeOffset(now time.Time) int {
	if !now.Before(g.shakeUntil) {
		return 0
	}
	if (now.UnixMilli()/40)%2 == 0 {
		return 1
	}
	return -1
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

func (g *Game) drawAliens(screen tcell.Screen, origin boardOrigin) {
	for r := 0; r < alienRows; r++ {
		style := tcell.StyleDefault.Background(boardColor).Foreground(rowColors[r]).Bold(true)
		sprite := alienSprites[r][g.frame]
		for c := 0; c < alienCols; c++ {
			if !g.aliens[r][c] {
				continue
			}
			p := g.alienCell(slot{row: r, col: c})
			if p.Y >= 0 && p.Y < boardRows {
				drawSprite(screen, origin, p, style, sprite)
			}
		}
	}
}

func (g *Game) drawBullets(screen tcell.Screen, origin boardOrigin) {
	playerShot := tcell.StyleDefault.Background(boardColor).Foreground(tcell.ColorWhite).Bold(true)
	alienShot := tcell.StyleDefault.Background(boardColor).Foreground(tcell.ColorRed).Bold(true)

	for _, b := range g.bullets {
		if b.fromPlayer() {
			drawSprite(screen, origin, b.pos, playerShot, playerShotSprite)
		} else {
			drawSprite(screen, origin, b.pos, alienShot, alienShotSprite)
		}
	}
}

func (g *Game) drawPlayer(screen tcell.Screen, origin boardOrigin) {
	if g.dying {
		return // the wreck's burst stands in for the cannon until it respawns
	}
	style := tcell.StyleDefault.Background(boardColor).Foreground(playerColor).Bold(true)
	drawSprite(screen, origin, Point{X: g.player, Y: playerRow}, style, cannonSprite)
}

// drawEffects overlays the live bursts and score popups. Bursts show their
// first frame for the first half of their life and the second afterwards;
// popups sit one row above the alien they came from.
func (g *Game) drawEffects(screen tcell.Screen, origin boardOrigin, now time.Time) {
	for _, b := range g.bursts {
		age := now.Sub(b.at)
		if age >= burstDuration {
			continue
		}
		frame := 0
		if age >= burstDuration/2 {
			frame = 1
		}
		style := tcell.StyleDefault.Background(boardColor).Foreground(burstColors[b.kind]).Bold(true)
		drawSprite(screen, origin, b.pos, style, burstSprites[b.kind][frame])
	}

	popupStyle := tcell.StyleDefault.Background(boardColor).Foreground(tcell.ColorWhite).Bold(true)
	for _, p := range g.popups {
		if now.Sub(p.at) >= popupDuration {
			continue
		}
		pos := Point{X: p.pos.X, Y: p.pos.Y - 1}
		if pos.Y < 0 {
			pos.Y = p.pos.Y
		}
		drawSprite(screen, origin, pos, popupStyle, p.text)
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
	status += fmt.Sprintf("  LIVES: %d  WAVE: %d", g.lives, g.wave)
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

// fillCell paints a whole cell with the style's background.
func fillCell(screen tcell.Screen, origin boardOrigin, p Point, style tcell.Style) {
	x := origin.leftX + p.X*cellWidth
	y := origin.topY + p.Y
	for i := 0; i < cellWidth; i++ {
		screen.SetContent(x+i, y, ' ', nil, style)
	}
}

// drawSprite writes a sprite into the cell. Sprites are cellWidth runes;
// a shorter one (a score popup like "+40") is centred and padded, a longer
// one is cut off at the cell edge.
func drawSprite(screen tcell.Screen, origin boardOrigin, p Point, style tcell.Style, sprite string) {
	runes := []rune(sprite)
	if len(runes) > cellWidth {
		runes = runes[:cellWidth]
	}
	pad := (cellWidth - len(runes)) / 2

	x := origin.leftX + p.X*cellWidth
	y := origin.topY + p.Y
	for i := 0; i < cellWidth; i++ {
		r := ' '
		if j := i - pad; j >= 0 && j < len(runes) {
			r = runes[j]
		}
		screen.SetContent(x+i, y, r, nil, style)
	}
}

func emitStr(screen tcell.Screen, x, y int, style tcell.Style, str string) {
	for _, r := range str {
		screen.SetContent(x, y, r, nil, style)
		x++
	}
}

func (g *Game) hint() string {
	return core.JoinHints("Arrows/AD: move", "Up/W/X: fire", g.keys.PauseHint(), g.keys.ResetHint("reset"), g.keys.LeaveHint())
}

// NeededSize is the board with the status line two rows above it and the
// hint line one row below (see drawStatus), as wide as the widest of them.
func (g *Game) NeededSize() core.Size {
	// The longest status drawStatus can produce, with room for the numbers.
	status := "SCORE: 999999  BEST: 999999  LIVES: 9  WAVE: 99 - GAME OVER"
	return core.Size{
		Cols: core.Widest(boardColumns*cellWidth, status, g.hint()),
		Rows: boardRows + 4,
	}
}
