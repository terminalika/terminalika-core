package core

// Rect is a screen rectangle in cells.
type Rect struct {
	X, Y, W, H int
}

// OverlayReporter is implemented by games that draw a band of their own over
// the board - PAUSED, GAME OVER, the winner - and can say where it went. A
// launcher that draws a notice of its own (an agent's pause) puts it on that
// band and makes it at least as wide, so the two never show side by side,
// without going looking for the band in the screen buffer.
type OverlayReporter interface {
	// OverlayArea is the band the last Draw painted; ok is false when there
	// was none.
	OverlayArea() (area Rect, ok bool)
}

// OverlayAreaOf asks g where its band is; ok is false when g has no band
// up or doesn't report one.
func OverlayAreaOf(g Game) (Rect, bool) {
	if r, ok := g.(OverlayReporter); ok {
		return r.OverlayArea()
	}
	return Rect{}, false
}
