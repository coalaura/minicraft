package server

import (
	"math/bits"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestRawBrightnessMatchesChunkLighting(t *testing.T) {
	world := normalLightingTestWorld()

	world.SetBlocks([]game.BlockChange{
		{Position: game.BlockPosition{X: 4, Y: 70, Z: 4}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 8, Y: 70, Z: 8}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 9, Y: 69, Z: 8}, Replacement: game.Torch},
	})

	light, err := buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build chunk light: %v", err)
	}

	positions := []game.BlockPosition{
		{X: 0, Y: 70, Z: 0},
		{X: 4, Y: 70, Z: 4},
		{X: 8, Y: 69, Z: 8},
		{X: 10, Y: 69, Z: 8},
		{X: 15, Y: -1, Z: 15},
	}

	for _, position := range positions {
		actual, queryErr := rawBrightnessAt(world, position)
		if queryErr != nil {
			t.Fatalf("raw brightness at %+v: %v", position, queryErr)
		}

		sky := updateLightLevel(light.SkyLightMask[0], light.SkyLight, position)
		block := updateLightLevel(light.BlockLightMask[0], light.BlockLight, position)

		expected := max(sky, block)
		if actual != expected {
			t.Fatalf("raw brightness at %+v = %d, want %d", position, actual, expected)
		}
	}
}

func TestRawBrightnessFullbrightSkipsGeneration(t *testing.T) {
	generator := &countingLightingGenerator{}

	world := game.NewOverworld(generator)

	brightness, err := rawBrightnessAt(world, game.BlockPosition{Y: 70})
	if err != nil {
		t.Fatalf("raw fullbright: %v", err)
	}

	if brightness != 15 || generator.calls != 0 {
		t.Fatalf("raw fullbright = %d with %d generator calls, want 15 with 0", brightness, generator.calls)
	}
}

func updateLightLevel(mask int64, arrays [][]byte, position game.BlockPosition) byte {
	section := (position.Y-protocol.OverworldMinY)/game.ChunkWidth + 1
	if mask&(1<<section) == 0 {
		return 0
	}

	arrayIndex := bits.OnesCount64(uint64(mask & ((1 << section) - 1)))
	localY := (position.Y - protocol.OverworldMinY) % game.ChunkWidth
	localX := int(position.X) & 15
	localZ := int(position.Z) & 15
	blockIndex := int(localY)*256 + localZ*16 + localX

	return arrays[arrayIndex][blockIndex>>1] >> ((blockIndex & 1) * 4) & 0x0f
}
