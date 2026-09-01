package core

import "github.com/gdamore/tcell/v2"

// BoardTint is the slight warm tint the games paint under the parts of a
// board that are "open" - 2048's whole board, Minesweeper's revealed cells -
// so they read as part of the board rather than as holes in it. It is the
// original 2048's board colour, darkened to sit under light tiles and
// coloured numbers alike.
var BoardTint = tcell.NewRGBColor(0x4a, 0x44, 0x3d)
