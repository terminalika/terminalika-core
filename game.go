package core

import "github.com/gdamore/tcell/v2"

// Game is the contract every playable game in the Terminalika ecosystem must
// implement.
//
// The launcher/engine owns the global keybindings (ESC, R and SPACE) and only
// forwards the remaining keys to HandleInput. Games therefore must never claim
// those three keys themselves.
type Game interface {
	// Init prepares the game. It is called once before the game loop starts.
	Init(screen tcell.Screen) error

	// Update advances the game state by one frame. Games are responsible for
	// their own tick timing (movement speed, gravity, etc.).
	Update()

	// Draw renders the current state to the screen.
	Draw(screen tcell.Screen)

	// HandleInput handles game-specific keys (arrows, WASD, etc.).
	// It returns true when the event was consumed by the game.
	HandleInput(event *tcell.EventKey) bool

	Pause()
	Resume()
	Reset()
}

// Factory creates a fresh Game instance. Games are stateful, so the registry
// stores factories instead of shared instances.
type Factory func() Game

// PauseState is implemented by games that can report whether they are paused.
// The engine uses it to keep its SPACE toggle in sync when a game is paused or
// resumed through external commands (WebSocket, pi subscription) instead of
// the global SPACE key.
type PauseState interface {
	IsPaused() bool
}

// KeyStateHandler is implemented by games that want to know when a key is
// pressed and when it is released, so that holding a key can drive smooth,
// continuous movement (a Pong paddle, a cannon) instead of one step per
// keypress.
//
// The launcher calls HandleKeyState(ev, true) for every key press before it
// falls back to HandleInput - a press that HandleKeyState consumes is not
// forwarded to HandleInput - and HandleKeyState(ev, false) when the key is
// released. Terminals that report key releases (the kitty keyboard protocol,
// win32-input-mode) deliver real releases; elsewhere the launcher
// synthesises a release shortly after the last press or auto-repeat of the
// key, so games must treat repeated presses of a held key as harmless.
type KeyStateHandler interface {
	HandleKeyState(ev *tcell.EventKey, pressed bool) bool
}
