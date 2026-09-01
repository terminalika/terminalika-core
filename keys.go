package core

import "strings"

// GlobalKeys names the keys a launcher reserves around every game. The
// launcher handles them itself - see Game: they never reach HandleInput -
// and the games only mention them in their hint line. The wording therefore
// follows the launcher: the CLI's ESC goes back to its menu, pi's parks the
// game. An empty label leaves that key out of the hints, and a game that is
// never given keys draws only its own.
type GlobalKeys struct {
	// Pause is the label of the pause toggle, e.g. "Space".
	Pause string
	// Reset is the label of the reset key, e.g. "R".
	Reset string
	// Leave is the label of the key that leaves the game, e.g. "Esc".
	Leave string
	// LeaveAction says what leaving does in this launcher: "menu", "park".
	LeaveAction string
	// Switch is the label of the key that opens the launcher's game picker
	// from inside a game, e.g. "alt+s"; "" when the launcher has none.
	Switch string
}

// GlobalKeysSetter is implemented by games that draw the launcher's keys in
// their hint line.
type GlobalKeysSetter interface {
	SetGlobalKeys(GlobalKeys)
}

// SetGlobalKeys hands keys to the game when it wants them.
func SetGlobalKeys(g Game, keys GlobalKeys) {
	if s, ok := g.(GlobalKeysSetter); ok {
		s.SetGlobalKeys(keys)
	}
}

// PauseHint is the pause key's hint, "" when there is none.
func (k GlobalKeys) PauseHint() string {
	return hint(k.Pause, "pause/resume")
}

// ResetHint is the reset key's hint with the game's word for what a reset is
// ("reset", "rematch"); "" when there is none.
func (k GlobalKeys) ResetHint(action string) string {
	return hint(k.Reset, action)
}

// LeaveHint is the leave key's hint; "" when there is none.
func (k GlobalKeys) LeaveHint() string {
	action := k.LeaveAction
	if action == "" {
		action = "leave"
	}
	return hint(k.Leave, action)
}

// SwitchHint is the switch key's hint; "" when there is none.
func (k GlobalKeys) SwitchHint() string {
	return hint(k.Switch, "switch game")
}

func hint(key, action string) string {
	if key == "" {
		return ""
	}
	return key + ": " + action
}

// JoinHints joins the non-empty hints with the two spaces the games' hint
// lines use.
func JoinHints(hints ...string) string {
	return strings.Join(nonEmpty(hints), "  ")
}

// HintLines lays the hints out in as few lines of at most width cells as
// they take, in order, never breaking a hint across lines: when the one-line
// hint no longer fits, whole hints move down. A hint wider than width gets a
// line of its own. Empty hints are skipped; no hints means no lines.
func HintLines(width int, hints ...string) []string {
	var lines []string
	line := ""
	for _, h := range nonEmpty(hints) {
		switch {
		case line == "":
			line = h
		case runeLen(line)+2+runeLen(h) <= width:
			line += "  " + h
		default:
			lines = append(lines, line)
			line = h
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// HintLayout sizes a game's surface around its hints. frameCols is what the
// game needs without them - the board, the status line - and availCols is
// what the launcher can give. The result is the one-line hint's width when
// it fits, else as narrow as availCols allows - never narrower than the frame
// or a single hint - and the number of lines the hints take at that width,
// as HintLines would lay them out there.
func HintLayout(availCols, frameCols int, hints ...string) (cols, hintRows int) {
	cols = Widest(frameCols, JoinHints(hints...))
	if availCols < cols {
		cols = max(availCols, Widest(frameCols, hints...))
	}
	return cols, len(HintLines(cols, hints...))
}

func nonEmpty(hints []string) []string {
	parts := hints[:0:0]
	for _, h := range hints {
		if h != "" {
			parts = append(parts, h)
		}
	}
	return parts
}

func runeLen(s string) int { return len([]rune(s)) }
