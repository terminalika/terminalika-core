package core

import (
	"strings"
	"testing"
)

func TestGlobalKeysHints(t *testing.T) {
	keys := GlobalKeys{Pause: "Space", Reset: "R", Leave: "Esc", LeaveAction: "menu"}
	got := JoinHints("Arrows: move", keys.PauseHint(), keys.ResetHint("rematch"), keys.LeaveHint())
	if want := "Arrows: move  Space: pause  R: rematch  Esc: menu"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	var none GlobalKeys
	got = JoinHints("Arrows: move", none.PauseHint(), none.ResetHint("reset"), none.LeaveHint())
	if want := "Arrows: move"; got != want {
		t.Fatalf("unset keys should draw only the game's own hint, got %q", got)
	}

	park := GlobalKeys{Leave: "Esc", LeaveAction: "park"}
	if got := park.LeaveHint(); got != "Esc: park" {
		t.Fatalf("got %q", got)
	}
	if got := (GlobalKeys{Leave: "Esc"}).LeaveHint(); got != "Esc: leave" {
		t.Fatalf("a leave key without an action should still read, got %q", got)
	}
}

func TestHintLines(t *testing.T) {
	hints := []string{"Arrows: move", "Space: pause", "", "R: reset", "Esc: menu"}
	if got := HintLines(80, hints...); len(got) != 1 || got[0] != "Arrows: move  Space: pause  R: reset  Esc: menu" {
		t.Fatalf("wide: %q", got)
	}
	got := HintLines(30, hints...)
	want := []string{"Arrows: move  Space: pause", "R: reset  Esc: menu"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("30 wide: got %q, want %q", got, want)
	}
	got = HintLines(5, hints...)
	if len(got) != 4 || got[0] != "Arrows: move" {
		t.Fatalf("a hint wider than the line gets a line of its own, got %q", got)
	}
	if got := HintLines(10); got != nil {
		t.Fatalf("no hints, no lines: %q", got)
	}
}

func TestHintLayout(t *testing.T) {
	hints := []string{"Arrows: move", "Space: pause", "R: reset", "Esc: menu"} // 47 in one line
	if cols, rows := HintLayout(80, 20, hints...); cols != 47 || rows != 1 {
		t.Fatalf("room for one line: got %d cols, %d rows", cols, rows)
	}
	if cols, rows := HintLayout(30, 20, hints...); cols != 30 || rows != 2 {
		t.Fatalf("narrow: got %d cols, %d rows, want 30 and 2", cols, rows)
	}
	if cols, _ := HintLayout(30, 40, hints...); cols != 40 {
		t.Fatalf("never narrower than the frame: got %d", cols)
	}
	if cols, rows := HintLayout(5, 4, hints...); cols != 12 || rows != 4 {
		t.Fatalf("never narrower than a hint: got %d cols, %d rows", cols, rows)
	}
	if got := (GlobalKeys{Switch: "alt+s"}).SwitchHint(); got != "alt+s: switch game" {
		t.Fatalf("got %q", got)
	}
	if got := (GlobalKeys{}).SwitchHint(); got != "" {
		t.Fatalf("no switch key, no hint: %q", got)
	}
}

type keyed struct {
	Game
	keys GlobalKeys
}

func (k *keyed) SetGlobalKeys(keys GlobalKeys) { k.keys = keys }

func TestSetGlobalKeysOnlyWhenWanted(t *testing.T) {
	k := &keyed{}
	SetGlobalKeys(k, GlobalKeys{Leave: "Esc"})
	if k.keys.Leave != "Esc" {
		t.Fatal("setter should have received the keys")
	}
	SetGlobalKeys(Game(nil), GlobalKeys{}) // a game without the setter is fine
}
