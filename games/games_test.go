package games

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	core "github.com/terminalika/terminalika-core"
	"github.com/terminalika/terminalika-core/highscore"
)

func TestDefaultRegistryListsBuiltInGames(t *testing.T) {
	want := []string{"2048", "mines", "snake", "tetris"}

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

// On a screen of exactly NeededSize every game draws its status line on the
// top row and its whole hint line, launcher key labels included, on the
// bottom row: nothing hangs off the edge, and no row is wasted.
func TestEveryGameFitsItsNeededSize(t *testing.T) {
	keys := core.GlobalKeys{Pause: "Space", Reset: "R", Leave: "Esc/ctrl+]", LeaveAction: "return to menu"}
	registry := WithStore(highscore.NewInMemory())
	for _, name := range registry.Names() {
		game, _ := registry.Get(name)
		sized, ok := game.(core.Sized)
		if !ok {
			t.Fatalf("%s does not implement core.Sized", name)
		}
		core.SetGlobalKeys(game, keys)
		size := sized.NeededSize(core.Size{Cols: 200, Rows: 100})

		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatal(err)
		}
		screen.SetSize(size.Cols, size.Rows)
		if err := game.Init(screen); err != nil {
			t.Fatalf("%s Init: %v", name, err)
		}
		game.Draw(screen)
		screen.Show()

		rows := screenRows(screen)
		if len(rows) != size.Rows {
			t.Fatalf("%s: %d rows drawn, NeededSize says %d", name, len(rows), size.Rows)
		}
		if !strings.Contains(rows[len(rows)-1], keys.LeaveHint()) {
			t.Fatalf("%s: bottom row %q should carry the whole hint", name, rows[len(rows)-1])
		}
		if !strings.Contains(rows[0], "SCORE") {
			t.Fatalf("%s: top row %q should be the status line", name, rows[0])
		}
	}
}

// Given less width than its one-line hint, every game wraps the hint onto
// more rows - whole hints at a time, in order - and asks for exactly those
// rows: the board and status still fit, the last hint ends the last row, and
// no hint is cut.
func TestEveryGameWrapsItsHintWhenNarrow(t *testing.T) {
	keys := core.GlobalKeys{Pause: "Space", Reset: "R", Leave: "Esc/alt+g", LeaveAction: "return to menu", Switch: "alt+s"}
	registry := WithStore(highscore.NewInMemory())
	for _, name := range registry.Names() {
		game, _ := registry.Get(name)
		sized := game.(core.Sized)
		core.SetGlobalKeys(game, keys)
		oneLine := sized.NeededSize(core.Size{Cols: 200, Rows: 100})
		// Narrow enough to force a wrap, wide enough for the board and status.
		avail := core.Size{Cols: oneLine.Cols - 10, Rows: 100}
		size := sized.NeededSize(avail)
		if size.Cols != avail.Cols {
			t.Fatalf("%s: asked for %d cols, want the %d available", name, size.Cols, avail.Cols)
		}
		if size.Rows <= oneLine.Rows {
			t.Fatalf("%s: %d rows when narrow, want more than the one-line %d", name, size.Rows, oneLine.Rows)
		}

		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatal(err)
		}
		screen.SetSize(size.Cols, size.Rows)
		if err := game.Init(screen); err != nil {
			t.Fatalf("%s Init: %v", name, err)
		}
		game.Draw(screen)
		screen.Show()

		rows := screenRows(screen)
		if !strings.Contains(rows[0], "SCORE") {
			t.Fatalf("%s: top row %q should be the status line", name, rows[0])
		}
		if !strings.Contains(rows[len(rows)-1], keys.LeaveHint()) {
			t.Fatalf("%s: bottom row %q should end the hints", name, rows[len(rows)-1])
		}
		extra := size.Rows - oneLine.Rows
		tail := strings.Join(rows[len(rows)-1-extra:], "  ")
		for _, h := range []string{keys.PauseHint(), keys.SwitchHint(), keys.LeaveHint()} {
			if !strings.Contains(tail, h) {
				t.Fatalf("%s: hint %q missing or split across the wrapped rows %q", name, h, rows[len(rows)-1-extra:])
			}
		}
	}
}

func screenRows(screen tcell.SimulationScreen) []string {
	cells, w, h := screen.GetContents()
	rows := make([]string, h)
	for y := 0; y < h; y++ {
		var row strings.Builder
		for x := 0; x < w; x++ {
			if c := cells[y*w+x]; len(c.Runes) > 0 {
				row.WriteRune(c.Runes[0])
			} else {
				row.WriteByte(' ')
			}
		}
		rows[y] = row.String()
	}
	return rows
}
