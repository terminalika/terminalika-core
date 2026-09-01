// Package games exposes the games that are built into terminalika-core.
package games

import (
	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/games/g2048"
	"github.com/terminalika/terminalika-core/games/snake"
	"github.com/terminalika/terminalika-core/games/tetris"
	"github.com/terminalika/terminalika-core/highscore"
)

var defaultRegistry = func() *core.Registry {
	r := core.NewRegistry()
	r.Register("2048", func() core.Game { return g2048.New() })
	r.Register("snake", func() core.Game { return snake.New() })
	r.Register("tetris", func() core.Game { return tetris.New() })
	return r
}()

// Default returns the registry containing the built-in games.
func Default() *core.Registry {
	return defaultRegistry
}

// WithStore returns a registry of the built-in games that share the given
// score store instead of the default scores file. Hosts without a filesystem
// (the wasm builds) pass highscore.NewInMemory(); the games then leave the
// best score off the screen.
func WithStore(store *highscore.Store) *core.Registry {
	r := core.NewRegistry()
	r.Register("2048", func() core.Game { return g2048.NewWithStore(store) })
	r.Register("snake", func() core.Game { return snake.NewWithStore(store) })
	r.Register("tetris", func() core.Game { return tetris.NewWithStore(store) })
	return r
}
