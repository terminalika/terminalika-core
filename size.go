package core

// Size is a screen size in cells.
type Size struct {
	Cols, Rows int
}

// Sized is implemented by games that know how much screen they need: the
// board, the status line above it, the hint line below it, and the widest
// of those lines - with the launcher's key labels in the hint, so set the
// keys first. A launcher that sizes its surface to the game (pi's overlay)
// asks for this and claims no more; the CLI and the website just centre the
// game in the terminal they have.
type Sized interface {
	NeededSize() Size
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
