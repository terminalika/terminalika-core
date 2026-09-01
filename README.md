# terminalika-core

Headless Go library that powers the Terminalika terminal games. It defines the
game contract, registry, built-in games, highscore persistence and the event
system. See the [`terminalika`](https://github.com/terminalika/terminalika)
repo for the CLI launcher.

## Packages

- `core.Game` — the contract every game implements:
  `Init`, `Update`, `Draw`, `HandleInput`, `Pause`, `Resume`, `Reset`.
- `core.Registry` — thread-safe registry of game factories by name.
- `games.Default()` — built-in games: `2048`, `snake` and `tetris`.
- `highscore` — persists each game's best score to
  `scores.json` in the user config dir (`~/.config/terminalika/scores.json`
  on Linux, `~/Library/Application Support/terminalika/scores.json` on macOS,
  `%AppData%\terminalika\scores.json` on Windows).
- `core.Event` / `core.Command` / `core.Bus` — the event system. Games publish
  domain events through an `Emitter` and may opt into external commands via
  the optional `core.Commandable` interface.

The library never owns the terminal lifecycle or the network: it only renders
into the `tcell.Screen` it is given and emits transport-agnostic events.

## Local development

To develop this library together with the launcher, clone both repos side by
side and use a Go workspace (see the launcher's README):

```sh
git clone https://github.com/terminalika/terminalika-core.git
git clone https://github.com/terminalika/terminalika.git
cd ..  # into the parent of both
go work init ./terminalika ./terminalika-core
```

The repos are public, so `go get` works out of the box.

## Test

```sh
cd terminalika-core
go test ./...
```
