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
	return hint(k.Pause, "pause")
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

func hint(key, action string) string {
	if key == "" {
		return ""
	}
	return key + ": " + action
}

// JoinHints joins the non-empty hints with the two spaces the games' hint
// lines use.
func JoinHints(hints ...string) string {
	parts := hints[:0:0]
	for _, h := range hints {
		if h != "" {
			parts = append(parts, h)
		}
	}
	return strings.Join(parts, "  ")
}
