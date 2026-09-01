package core

// Size is a screen size in cells.
type Size struct {
	Cols, Rows int
}

// Sized is implemented by games that know how much screen they need: the
// board, the status line above it, the hint lines below it, and the widest
// of those - with the launcher's key labels in the hint, so set the keys
// first. avail is the most the launcher can give; a game whose one-line hint
// is wider than that wraps it (see HintLayout) and asks for the extra rows
// instead. A launcher that sizes its surface to the game (pi's overlay) asks
// for this and claims no more; the CLI and the website just centre the game
// in the terminal they have.
type Sized interface {
	NeededSize(avail Size) Size
}

// Widest returns the larger of min and the longest of lines, in cells.
func Widest(min int, lines ...string) int {
	for _, l := range lines {
		if n := len([]rune(l)); n > min {
			min = n
		}
	}
	return min
}
