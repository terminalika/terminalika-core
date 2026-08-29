// Package games exposes the games that are built into terminalika-core.
package games

import (
	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/games/invaders"
	"github.com/terminalika/terminalika-core/games/pong"
	"github.com/terminalika/terminalika-core/games/snake"
	"github.com/terminalika/terminalika-core/games/tetris"
)

var defaultRegistry = func() *core.Registry {
	r := core.NewRegistry()
	r.Register("invaders", func() core.Game { return invaders.New() })
	r.Register("pong", func() core.Game { return pong.New() })
	r.Register("snake", func() core.Game { return snake.New() })
	r.Register("tetris", func() core.Game { return tetris.New() })
	return r
}()

// Default returns the registry containing the built-in games.
func Default() *core.Registry {
	return defaultRegistry
}
