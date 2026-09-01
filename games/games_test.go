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
		size := sized.NeededSize()

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

// Every game says exactly where its PAUSED band is: nothing when it is not
// up, and when it is, the reported cells - and only those - carry its dark
// red background. Launchers cover it from this, never from the buffer.
func TestEveryGameReportsItsOverlayBand(t *testing.T) {
	registry := WithStore(highscore.NewInMemory())
	for _, name := range registry.Names() {
		game, _ := registry.Get(name)
		reporter, ok := game.(core.OverlayReporter)
		if !ok {
			t.Fatalf("%s does not implement core.OverlayReporter", name)
		}
		size := game.(core.Sized).NeededSize()
		screen := tcell.NewSimulationScreen("UTF-8")
		if err := screen.Init(); err != nil {
			t.Fatal(err)
		}
		screen.SetSize(size.Cols, size.Rows)
		if err := game.Init(screen); err != nil {
			t.Fatalf("%s Init: %v", name, err)
		}

		game.Draw(screen)
		if _, up := reporter.OverlayArea(); up {
			t.Fatalf("%s: reports a band before anything is up", name)
		}

		game.Pause()
		game.Draw(screen)
		screen.Show()
		area, up := reporter.OverlayArea()
		if !game.(core.PauseState).IsPaused() {
			t.Fatalf("%s: refused a pause on a fresh game", name)
		}
		if !up || area.H != 1 || area.W == 0 {
			t.Fatalf("%s: paused but band = %+v, %v", name, area, up)
		}
		for x := area.X - 1; x <= area.X+area.W; x++ {
			_, _, style, _ := screen.GetContent(x, area.Y)
			_, bg, _ := style.Decompose()
			inside := x >= area.X && x < area.X+area.W
			if inside && bg != tcell.ColorDarkRed {
				t.Fatalf("%s: cell (%d,%d) inside the reported band is not the band", name, x, area.Y)
			}
			if !inside && bg == tcell.ColorDarkRed {
				t.Fatalf("%s: cell (%d,%d) outside the reported band is still the band", name, x, area.Y)
			}
		}
	}
}
