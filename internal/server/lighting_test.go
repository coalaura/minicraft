package server

import (
	"math/bits"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type countingLightingGenerator struct {
	calls int
}

func (generator *countingLightingGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	generator.calls++

	return game.Air
}

func TestNormalLightingDirectAndFilteredSkylight(t *testing.T) {
	world := normalLightingTestWorld()

	world.SetBlocks([]game.BlockChange{
		{Position: game.BlockPosition{X: 4, Y: 10, Z: 4}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 6, Y: 10, Z: 4}, Replacement: game.Glass},
	})

	light, err := buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build light: %v", err)
	}

	assertLightLevel(t, light, true, 4, 10, 4, 14)
	assertLightLevel(t, light, true, 4, 9, 4, 14)
	assertLightLevel(t, light, true, 6, 10, 4, 15)
	assertLightLevel(t, light, true, 8, 319, 8, 15)
	assertLightLevel(t, light, false, 8, 0, 8, 0)
}

func TestNormalLightingOpaqueRoofAndSidewaysSkylight(t *testing.T) {
	world := normalLightingTestWorld()

	changes := make([]game.BlockChange, 0, lightingWidth*lightingWidth)

	for z := int32(-lightingHalo); z < game.ChunkWidth+lightingHalo; z++ {
		for x := int32(-lightingHalo); x < game.ChunkWidth+lightingHalo; x++ {
			changes = append(changes, game.BlockChange{
				Position:    game.BlockPosition{X: x, Y: 0, Z: z},
				Replacement: game.Stone,
			})
		}
	}

	world.SetBlocks(changes)

	blocked, err := buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build blocked light: %v", err)
	}

	assertLightLevel(t, blocked, true, 8, -16, 8, 0)

	world.SetBlock(game.BlockPosition{X: 8, Y: 0, Z: 8}, game.Air)

	light, err := buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build opened light: %v", err)
	}

	assertLightLevel(t, light, true, 8, -1, 8, 15)
	assertLightLevel(t, light, true, 9, -1, 8, 14)
}

func TestNormalLightingBlockFalloffAndBoundaries(t *testing.T) {
	world := normalLightingTestWorld()

	world.SetBlocks([]game.BlockChange{
		{Position: game.BlockPosition{X: 8, Y: 15, Z: 8}, Replacement: game.Torch},
		{Position: game.BlockPosition{X: 12, Y: 15, Z: 8}, Replacement: game.Glowstone},
		{Position: game.BlockPosition{X: 9, Y: 15, Z: 8}, Replacement: game.Stone},
		{Position: game.BlockPosition{X: 15, Y: 32, Z: 8}, Replacement: game.Torch},
	})

	first, err := buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build first chunk light: %v", err)
	}

	assertLightLevel(t, first, false, 8, 15, 8, 14)
	assertLightLevel(t, first, false, 9, 15, 8, 0)
	assertLightLevel(t, first, false, 8, 16, 8, 13)
	assertLightLevel(t, first, false, 10, 15, 8, 13)

	second, err := buildChunkLight(world, 1, 0)
	if err != nil {
		t.Fatalf("build neighboring chunk light: %v", err)
	}

	assertLightLevel(t, second, false, 0, 32, 8, 13)
}

func TestNormalOpenLightMasksAndFullbrightBypass(t *testing.T) {
	openWorld := normalLightingTestWorld()

	light, err := buildChunkLight(openWorld, -3, 4)
	if err != nil {
		t.Fatalf("build open light: %v", err)
	}

	fullMask := int64(1<<protocol.OverworldLightSectionCount) - 1
	if light.SkyLightMask[0] != fullMask&^1 || light.EmptySkyLightMask[0] != 1 || light.BlockLightMask[0] != 0 || light.EmptyBlockLightMask[0] != fullMask {
		t.Fatalf("open light masks = sky %x empty sky %x block %x empty block %x", light.SkyLightMask[0], light.EmptySkyLightMask[0], light.BlockLightMask[0], light.EmptyBlockLightMask[0])
	}

	if len(light.SkyLight) != protocol.OverworldLightSectionCount-1 || len(light.BlockLight) != 0 {
		t.Fatalf("open light arrays = sky %d block %d", len(light.SkyLight), len(light.BlockLight))
	}

	generator := &countingLightingGenerator{}

	fullbrightWorld := game.NewOverworld(generator)

	chunk, err := buildLevelChunk(fullbrightWorld, 0, 0)
	if err != nil {
		t.Fatalf("build fullbright chunk: %v", err)
	}

	fullbrightCalls := generator.calls

	if len(chunk.SkyLight) != protocol.OverworldLightSectionCount || &chunk.SkyLight[0][0] != &chunk.SkyLight[1][0] {
		t.Fatal("fullbright chunk did not reuse shared sky arrays")
	}

	generator.calls = 0

	fullbrightWorld.SetLightingMode(game.LightingNormal)

	_, err = buildLevelChunk(fullbrightWorld, 0, 0)
	if err != nil {
		t.Fatalf("build normal chunk: %v", err)
	}

	if generator.calls <= fullbrightCalls {
		t.Fatalf("normal generator calls = %d, fullbright = %d", generator.calls, fullbrightCalls)
	}
}

func TestNormalMutationRelightsLoadedChunksOnly(t *testing.T) {
	world := normalLightingTestWorld()

	runtime := NewRuntime(world)

	position := game.BlockPosition{X: 8, Y: 70, Z: 8}

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)
	unloaded, unloadedConnection := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Unloaded", game.GameModeCreative)

	actor.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(actor, position)

	unloaded.loadedChunks = map[LoadedChunk]struct{}{{X: 20, Z: 20}: {}}

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, unloaded)

	actorConnection.reset()
	unloadedConnection.reset()

	result, err := runtime.MutateBlocks(actor, BlockMutationPlace, []game.BlockChange{{Position: position, Replacement: game.Torch}})
	if err != nil || !result.Changed {
		t.Fatalf("place emitter result = %+v, error = %v", result, err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundUpdateLightID})
	assertPacketIDs(t, unloadedConnection.packetIDs(t), nil)

	actorConnection.reset()

	result, err = runtime.MutateBlocks(actor, BlockMutationBreak, []game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("remove emitter result = %+v, error = %v", result, err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundUpdateLightID})

	world.SetBlock(position, game.Stone)

	actorConnection.reset()

	result, err = runtime.MutateBlocks(actor, BlockMutationInteract, []game.BlockChange{{Position: position, Replacement: game.Dirt}})
	if err != nil || !result.Changed {
		t.Fatalf("replace equivalent block result = %+v, error = %v", result, err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID})
}

func TestNormalMutationClosesAndReopensSkylight(t *testing.T) {
	world := normalLightingTestWorld()

	changes := make([]game.BlockChange, 0, lightingWidth*lightingWidth)

	opening := game.BlockPosition{X: 8, Y: 70, Z: 8}

	for z := int32(-lightingHalo); z < game.ChunkWidth+lightingHalo; z++ {
		for x := int32(-lightingHalo); x < game.ChunkWidth+lightingHalo; x++ {
			if x == opening.X && z == opening.Z {
				continue
			}

			changes = append(changes, game.BlockChange{
				Position:    game.BlockPosition{X: x, Y: opening.Y, Z: z},
				Replacement: game.Stone,
			})
		}
	}

	world.SetBlocks(changes)

	runtime := NewRuntime(world)

	actor, connection := newBlockMutationTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "Actor", game.GameModeCreative)
	actor.Player.Position = blockMutationTestPlayerPosition(opening)

	markChunkLoaded(actor, opening)

	joinTestSession(t, runtime, actor)

	connection.reset()

	light, err := buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build open light: %v", err)
	}

	assertLightLevel(t, light, true, 8, 69, 8, 15)

	result, err := runtime.MutateBlocks(actor, BlockMutationPlace, []game.BlockChange{{Position: opening, Replacement: game.Stone}})
	if err != nil || !result.Changed {
		t.Fatalf("close opening result = %+v, error = %v", result, err)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundUpdateLightID})

	light, err = buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build closed light: %v", err)
	}

	assertLightLevel(t, light, true, 8, 69, 8, 0)

	connection.reset()

	result, err = runtime.MutateBlocks(actor, BlockMutationBreak, []game.BlockChange{{Position: opening, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("reopen skylight result = %+v, error = %v", result, err)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundUpdateLightID})

	light, err = buildChunkLight(world, 0, 0)
	if err != nil {
		t.Fatalf("build reopened light: %v", err)
	}

	assertLightLevel(t, light, true, 8, 69, 8, 15)
}

func TestChangedLightUpdatesCrossNegativeChunkBoundaries(t *testing.T) {
	world := normalLightingTestWorld()

	updates, err := buildChangedLightUpdates(world, []game.BlockChange{{
		Position:    game.BlockPosition{X: -1, Y: 70, Z: -1},
		Replacement: game.Torch,
	}})

	if err != nil {
		t.Fatalf("build changed light updates: %v", err)
	}

	expected := []LoadedChunk{
		{X: -1, Z: -1},
		{X: 0, Z: -1},
		{X: -1, Z: 0},
		{X: 0, Z: 0},
	}

	if len(updates) != len(expected) {
		t.Fatalf("update count = %d, want %d", len(updates), len(expected))
	}

	for index, update := range updates {
		actual := LoadedChunk{X: update.Position.X, Z: update.Position.Z}
		if actual != expected[index] {
			t.Fatalf("update %d = %+v, want %+v", index, actual, expected[index])
		}
	}
}

func normalLightingTestWorld() *game.World {
	world := game.NewOverworld(nil)

	world.SetLightingMode(game.LightingNormal)

	return world
}

func assertLightLevel(t *testing.T, update protocol.UpdateLight, sky bool, localX, worldY, localZ int, expected byte) {
	t.Helper()

	section := (worldY-protocol.OverworldMinY)/game.ChunkWidth + 1
	localY := (worldY - protocol.OverworldMinY) % game.ChunkWidth

	mask := update.BlockLightMask[0]
	arrays := update.BlockLight

	if sky {
		mask = update.SkyLightMask[0]
		arrays = update.SkyLight
	}

	var actual byte

	if mask&(1<<section) != 0 {
		arrayIndex := bits.OnesCount64(uint64(mask & ((1 << section) - 1)))
		blockIndex := localY*256 + localZ*16 + localX

		actual = arrays[arrayIndex][blockIndex>>1] >> ((blockIndex & 1) * 4) & 0x0f
	}

	if actual != expected {
		t.Fatalf("light at (%d,%d,%d) = %d, want %d", localX, worldY, localZ, actual, expected)
	}
}
