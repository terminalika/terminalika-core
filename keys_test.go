package core

import "testing"

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
