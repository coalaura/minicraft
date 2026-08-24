package server

import (
	"reflect"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator/spawnplatform"
	"github.com/coalaura/minicraft/internal/protocol"
)

type unsupportedBlockGenerator struct{}

func (unsupportedBlockGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.Block(100)
}

func TestLevelChunksQueryWorldAcrossBoundaries(t *testing.T) {
	world := game.NewOverworld(spawnplatform.New())

	right, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("build right chunk: %v", err)
	}

	left, err := buildLevelChunk(world, -1, 0)
	if err != nil {
		t.Fatalf("build left chunk: %v", err)
	}

	rightAgain, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("rebuild right chunk: %v", err)
	}

	if !reflect.DeepEqual(right, rightAgain) {
		t.Fatal("chunk changed when generated in a different order")
	}

	assertChunkBlockState(t, left, 15, 69, 0, protocol.StoneBlockState)
	assertChunkBlockState(t, right, 0, 69, 0, protocol.StoneBlockState)
	assertChunkBlockState(t, right, 4, 69, 0, protocol.StoneBlockState)
	assertChunkBlockState(t, right, 5, 69, 0, protocol.AirBlockState)
}

func TestLevelChunksIncludeWorldOverrides(t *testing.T) {
	world := game.NewOverworld(spawnplatform.New())

	world.SetBlock(game.BlockPosition{X: 0, Y: 69, Z: 0}, game.Air)
	world.SetBlock(game.BlockPosition{X: 5, Y: 69, Z: 0}, game.Stone)

	chunk, err := buildLevelChunk(world, 0, 0)
	if err != nil {
		t.Fatalf("build overridden chunk: %v", err)
	}

	assertChunkBlockState(t, chunk, 0, 69, 0, protocol.AirBlockState)
	assertChunkBlockState(t, chunk, 5, 69, 0, protocol.StoneBlockState)
}

func TestChunkBatchValidatesGeneratedBlocksBeforeWriting(t *testing.T) {
	session, connection := newChunkTestSession(game.Position{})

	session.Runtime.World = &game.World{Generator: unsupportedBlockGenerator{}}

	sent, err := session.sendChunkBatch([]LoadedChunk{{}})
	if err == nil {
		t.Fatal("send chunk batch with unsupported block succeeded")
	}

	if sent != 0 {
		t.Fatalf("sent chunk count = %d, want 0", sent)
	}

	if packets := connection.packets(t); len(packets) != 0 {
		t.Fatalf("invalid chunk wrote packets: %v", connection.packetIDs(t))
	}
}

func assertChunkBlockState(t *testing.T, chunk protocol.LevelChunkWithLight, localX int, worldY int, localZ int, expected int32) {
	t.Helper()

	sectionIndex := (worldY - protocol.OverworldMinY) / ChunkWidth
	localY := (worldY - protocol.OverworldMinY) % ChunkWidth
	section := chunk.Sections[sectionIndex]

	actual := section.BlockState

	if len(section.Palette) != 0 {
		blockIndex := localY*256 + localZ*16 + localX
		packed := section.Data[blockIndex/16]
		paletteIndex := packed >> uint(blockIndex%16*protocol.MinBitsPerEntry) & 0x0F
		actual = section.Palette[paletteIndex]
	}

	if actual != expected {
		t.Fatalf(
			"block (%d, %d, %d) state = %d, want %d",
			localX,
			worldY,
			localZ,
			actual,
			expected,
		)
	}
}
