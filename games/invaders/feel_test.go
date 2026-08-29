package invaders

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	core "github.com/terminalika/terminalika-core"
)

// killOneAlien leaves two aliens, shoots one of them and returns its cell.
func killOneAlien(t *testing.T, g *Game) Point {
	t.Helper()
	clearAliens(g)
	g.aliens[0][0] = true
	g.aliens[1][3] = true
	g.alive = 2

	target := g.alienCell(slot{row: 1, col: 3})
	g.bullets = []bullet{{pos: Point{X: target.X, Y: target.Y + 1}, dy: -1}}
	g.stepBullets()

	if g.aliens[1][3] {
		t.Fatal("alien should be destroyed")
	}
	return target
}

func TestAlienKillPlaysBurstPopupAndHitStop(t *testing.T) {
	g := newTestGame()
	before := time.Now()

	target := killOneAlien(t, g)

	if len(g.bursts) != 1 || g.bursts[0].pos != target || g.bursts[0].kind != burstAlien {
		t.Fatalf("bursts = %+v, want one alien burst at %v", g.bursts, target)
	}
	if len(g.popups) != 1 || g.popups[0].pos != target || g.popups[0].text != "+30" {
		t.Fatalf("popups = %+v, want one \"+30\" popup at %v", g.popups, target)
	}
	if !g.freezeUntil.After(before) {
		t.Fatal("an alien kill should freeze the action for a beat")
	}
	if g.freezeUntil.Sub(before) > alienHitStop+time.Second {
		t.Fatalf("alien hit-stop is way too long: %v", g.freezeUntil.Sub(before))
	}
	if g.dying {
		t.Fatal("killing an alien must not put the cannon in its dying state")
	}
}

func TestUpdateHoldsStillDuringHitStop(t *testing.T) {
	g := newTestGame()
	g.freezeUntil = time.Now().Add(time.Hour)
	g.lastAlienTick = time.Now().Add(-time.Hour) // formation is long overdue
	g.lastPlayerShotTick = g.lastAlienTick
	g.lastAlienShotTick = g.lastAlienTick
	g.lastAlienFire = g.lastAlienTick
	g.bullets = []bullet{{pos: Point{X: 3, Y: 10}, dy: -1}}
	formation := g.formation

	g.Update()

	if g.formation != formation {
		t.Fatalf("formation moved to %v during hit-stop, want it held at %v", g.formation, formation)
	}
	if g.bullets[0].pos != (Point{X: 3, Y: 10}) {
		t.Fatalf("shot moved to %v during hit-stop, want it held", g.bullets[0].pos)
	}
	if countBullets(g, false) != 0 {
		t.Fatal("aliens must not fire during hit-stop")
	}
}

func TestCannonHitWrecksShakesAndRespawns(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)
	g.player = 3
	g.bullets = []bullet{{pos: Point{X: 3, Y: playerRow - 1}, dy: 1}}
	before := time.Now()

	g.stepBullets()

	if !g.dying {
		t.Fatal("a hit cannon should be dying until it respawns")
	}
	if g.player != 3 {
		t.Fatalf("player = %d right after the hit, want it left at 3 for the wreck", g.player)
	}
	if len(g.bursts) != 1 || g.bursts[0].kind != burstCannon || g.bursts[0].pos != (Point{X: 3, Y: playerRow}) {
		t.Fatalf("bursts = %+v, want one cannon burst at (3,%d)", g.bursts, playerRow)
	}
	if !g.shakeUntil.After(before) {
		t.Fatal("a cannon hit should shake the board")
	}
	if !g.freezeUntil.After(before.Add(alienHitStop)) {
		t.Fatal("a cannon hit should hold the action longer than an alien kill")
	}

	// The wreck replaces the cannon on screen; the burst shows instead.
	g.Draw(screen)
	if screenContains(screen, cannonSprite) {
		t.Fatal("cannon sprite should be hidden while dying")
	}
	if !screenContains(screen, burstSprites[burstCannon][0]) {
		t.Fatal("cannon burst should be on screen right after the hit")
	}

	// Keys and commands are swallowed/rejected while dying.
	if !g.HandleInput(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("keys should be swallowed while dying")
	}
	if g.player != 3 {
		t.Fatal("wreck must not move")
	}
	if g.fire() {
		t.Fatal("wreck must not fire")
	}
	if err := g.HandleCommand(core.Command{Type: cmdFire}); err == nil {
		t.Fatal("fire command should be rejected while dying")
	}
	if err := g.HandleCommand(core.Command{Type: cmdMove, Payload: core.MustJSON(movePayload{DX: 1})}); err == nil {
		t.Fatal("move command should be rejected while dying")
	}

	// Once the hold is over, Update respawns a fresh cannon in the middle.
	g.freezeUntil = time.Now().Add(-time.Millisecond)
	g.Update()
	if g.dying {
		t.Fatal("cannon should have respawned after the hold")
	}
	if g.player != boardColumns/2 {
		t.Fatalf("player = %d after respawn, want %d", g.player, boardColumns/2)
	}
	g.bursts = nil
	g.Draw(screen)
	if !screenContains(screen, cannonSprite) {
		t.Fatal("cannon sprite should be back after respawn")
	}
}

func TestWreckCannotBeHitAgain(t *testing.T) {
	g := newTestGame()
	g.bullets = []bullet{{pos: Point{X: g.player, Y: playerRow - 1}, dy: 1}}
	g.stepBullets()
	if g.lives != startLives-1 {
		t.Fatalf("lives = %d, want %d", g.lives, startLives-1)
	}

	// Another shot on the wreck while it's dying must not cost a second life.
	g.bullets = []bullet{{pos: Point{X: g.player, Y: playerRow - 1}, dy: 1}}
	g.stepBullets()
	if g.lives != startLives-1 {
		t.Fatalf("lives = %d after hitting the wreck, want %d unchanged", g.lives, startLives-1)
	}
}

func TestEffectsExpire(t *testing.T) {
	g := newTestGame()
	old := time.Now().Add(-time.Hour)
	g.bursts = []burst{{pos: Point{X: 1, Y: 1}, at: old}, {pos: Point{X: 2, Y: 2}, at: time.Now()}}
	g.popups = []popup{{pos: Point{X: 1, Y: 1}, text: "+10", at: old}}

	g.expireEffects(time.Now())

	if len(g.bursts) != 1 || g.bursts[0].pos != (Point{X: 2, Y: 2}) {
		t.Fatalf("bursts = %+v, want only the fresh one", g.bursts)
	}
	if len(g.popups) != 0 {
		t.Fatalf("popups = %+v, want the stale one gone", g.popups)
	}
}

func TestResetClearsFeelState(t *testing.T) {
	g := newTestGame()
	g.bullets = []bullet{{pos: Point{X: g.player, Y: playerRow - 1}, dy: 1}}
	g.stepBullets() // wreck the cannon: dying, burst, freeze, shake
	killOneAlien(t, g)

	g.Reset()

	if g.dying || len(g.bursts) != 0 || len(g.popups) != 0 {
		t.Fatalf("after Reset dying/bursts/popups = %v/%d/%d, want false/0/0", g.dying, len(g.bursts), len(g.popups))
	}
	if time.Now().Before(g.freezeUntil) || time.Now().Before(g.shakeUntil) {
		t.Fatal("Reset must lift any freeze or shake")
	}
}

func TestShotsMoveOnSeparateClocks(t *testing.T) {
	g := newTestGame()
	clearAliens(g)
	g.aliens[0][0] = true
	g.alive = 1
	g.formation = Point{X: 20, Y: 1}
	g.bullets = []bullet{
		{pos: Point{X: 2, Y: 10}, dy: -1},
		{pos: Point{X: 4, Y: 5}, dy: 1},
	}

	g.stepShots(true)
	if g.bullets[0].pos.Y != 9 || g.bullets[1].pos.Y != 5 {
		t.Fatalf("after player step shots at y=%d,%d; want 9,5", g.bullets[0].pos.Y, g.bullets[1].pos.Y)
	}

	g.stepShots(false)
	if g.bullets[0].pos.Y != 9 || g.bullets[1].pos.Y != 6 {
		t.Fatalf("after alien step shots at y=%d,%d; want 9,6", g.bullets[0].pos.Y, g.bullets[1].pos.Y)
	}
}

func TestPlayerShotIsFasterThanAlienShot(t *testing.T) {
	if playerShotPeriod >= alienShotPeriod {
		t.Fatalf("player shot period %v should be shorter than alien shot period %v", playerShotPeriod, alienShotPeriod)
	}
}

func TestDrawShowsBurstAndPopupAfterKill(t *testing.T) {
	g := newTestGame()
	screen := newSimScreen(t, 80, 24)

	killOneAlien(t, g)
	g.Draw(screen)

	if !screenContains(screen, burstSprites[burstAlien][0]) {
		t.Fatal("alien burst should be on screen right after the kill")
	}
	if !screenContains(screen, "+30") {
		t.Fatal("score popup should be on screen right after the kill")
	}
}

func TestShakeOffsetOnlyWhileShaking(t *testing.T) {
	g := newTestGame()
	now := time.Now()

	if got := g.shakeOffset(now); got != 0 {
		t.Fatalf("shakeOffset = %d with no shake, want 0", got)
	}
	g.shakeUntil = now.Add(time.Hour)
	if got := g.shakeOffset(now); got != 1 && got != -1 {
		t.Fatalf("shakeOffset = %d while shaking, want +/-1", got)
	}
}
