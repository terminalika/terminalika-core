package games

import (
	"testing"

	core "github.com/terminalika/terminalika-core"
)

func TestDefaultRegistryListsBuiltInGames(t *testing.T) {
	want := []string{"invaders", "pong", "snake", "tetris"}

	got := Default().Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
}

func TestDefaultRegistryBuildsEveryGame(t *testing.T) {
	for _, name := range Default().Names() {
		game, ok := Default().Get(name)
		if !ok || game == nil {
			t.Fatalf("Get(%q) = %v, %v; want a game", name, game, ok)
		}
		// Every built-in game supports external commands and reports its pause
		// state, so the launcher's sidecar and agent subscriptions work for all
		// of them.
		if _, ok := game.(core.Commandable); !ok {
			t.Fatalf("%s does not implement core.Commandable", name)
		}
		if _, ok := game.(core.PauseState); !ok {
			t.Fatalf("%s does not implement core.PauseState", name)
		}
		if _, ok := game.(core.EmitterSetter); !ok {
			t.Fatalf("%s does not implement core.EmitterSetter", name)
		}
	}
}
