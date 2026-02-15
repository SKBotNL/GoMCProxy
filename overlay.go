// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type upgradeData struct {
	text      string
	nextPrice int
}

//go:embed Monocraft.ttf
var monocraftTTF []byte

var upgrades = make(map[string]upgradeData)
var upgradesMutex sync.RWMutex

var traps []string
var trapsMutex sync.RWMutex

var upgradeOrder = [6]string{"sharp", "prot", "haste", "forge", "healpool", "featherfalling"}

func runOverlay() {
	rl.SetTraceLogLevel(rl.LogError)
	rl.SetConfigFlags(rl.FlagWindowTransparent)
	rl.InitWindow(280, 240, "GoMCProxy Overlay")
	rl.SetWindowState(rl.FlagWindowUndecorated | rl.FlagWindowResizable)
	defer rl.CloseWindow()

	rl.SetTargetFPS(5)

	codepoints := []rune{}
	for i := 32; i < 127; i++ {
		codepoints = append(codepoints, rune(i))
	}
	codepoints = append(codepoints, '↑')
	codepoints = append(codepoints, '✔')

	font := rl.LoadFontFromMemory(".ttf", monocraftTTF, 24, codepoints)
	defer rl.UnloadFont(font)

	characterSize := int(rl.MeasureTextEx(font, "a", 24, 0).X)

	for !rl.WindowShouldClose() {
		rl.BeginDrawing()

		width := rl.GetScreenWidth()

		rl.ClearBackground(rl.Color{R: 0, G: 0, B: 0, A: 75})

		rl.DrawTextEx(font, "Upgrades", rl.NewVector2(6, 0), 24, 0, rl.Yellow)

		var y float32 = 20

		upgradesMutex.RLock()
		if len(upgrades) == 0 {
			rl.DrawTextEx(font, "None", rl.NewVector2(6, y), 24, 0, rl.White)
			y += 20
		} else {
			keys := make([]string, 0, len(upgrades))
			for k := range upgrades {
				keys = append(keys, k)
			}
			slices.Sort(keys)

			for _, key := range upgradeOrder {
				data, ok := upgrades[key]
				if !ok {
					continue
				}

				rl.DrawTextEx(font, data.text, rl.NewVector2(6, y), 24, 0, rl.White)
				if data.nextPrice > 0 {
					characters := 1 + len(strconv.Itoa(data.nextPrice))
					rl.DrawTextEx(font, fmt.Sprintf("↑%d", data.nextPrice), rl.NewVector2(float32(width-characterSize*characters-6), float32(y)), 24, 0, color.RGBA{R: 84, G: 255, B: 255, A: 255})
				} else {
					rl.DrawTextEx(font, "✔", rl.NewVector2(float32(width-characterSize-6), float32(y)), 24, 0, rl.Green)
				}
				y += 20
			}
		}
		upgradesMutex.RUnlock()

		y += 8
		rl.DrawTextEx(font, "Traps", rl.NewVector2(6, y), 24, 0, rl.Yellow)
		y += 20

		trapsMutex.RLock()
		if len(traps) == 0 {
			rl.DrawTextEx(font, "None", rl.NewVector2(6, y), 24, 0, rl.White)
		} else {
			for _, trap := range traps {
				rl.DrawTextEx(font, trap, rl.NewVector2(6, y), 24, 0, rl.White)
				y += 20
			}
		}
		trapsMutex.RUnlock()

		rl.EndDrawing()
	}
}
