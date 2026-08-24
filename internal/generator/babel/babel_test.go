package babel

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

func TestGeneratorRegisters(t *testing.T) {
	generated, err := generator.New(Name)
	if err != nil {
		t.Fatalf("create babel generator: %v", err)
	}

	if generated == nil {
		t.Fatal("babel generator is nil")
	}
}

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{0, 1, -1, 123456789, -987654321} {
		spawn := generated.Spawn(seed)
		position := game.BlockPosition{X: int32(spawn.X), Y: int32(spawn.Y), Z: int32(spawn.Z)}

		if block := generated.BlockAt(seed, position); block != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, block)
		}

		position.Y++
		if block := generated.BlockAt(seed, position); block != game.Air {
			t.Fatalf("seed %d block above spawn = %d, want air", seed, block)
		}

		position.Y -= 2
		if block := generated.BlockAt(seed, position); block == game.Air {
			t.Fatalf("seed %d block below spawn is air", seed)
		}
	}
}

func TestGeneratorHasRecursiveStreetHierarchy(t *testing.T) {
	generated := Generator{}
	seed := int64(42)
	originX, originZ := cityOrigins(seed)

	grand := game.BlockPosition{X: int32(originX), Y: baseFloorY, Z: int32(originZ + 29)}
	boulevard := game.BlockPosition{X: int32(originX + boulevardScale), Y: baseFloorY, Z: int32(originZ + 29)}
	street := game.BlockPosition{X: int32(originX + lotScale), Y: baseFloorY, Z: int32(originZ + 29)}

	if block := generated.BlockAt(seed, grand); block != game.BlackConcrete {
		t.Fatalf("grand avenue block = %d, want black concrete", block)
	}

	if block := generated.BlockAt(seed, boulevard); block != game.GrayConcrete {
		t.Fatalf("boulevard block = %d, want gray concrete", block)
	}

	if block := generated.BlockAt(seed, street); block != game.GrayConcrete {
		t.Fatalf("street block = %d, want gray concrete", block)
	}
}

func TestGeneratorUsesArchitecturalMaterialVariety(t *testing.T) {
	generated := Generator{}
	seed := int64(1337)
	originX, originZ := cityOrigins(seed)
	seen := make(map[game.Block]struct{})

	for y := baseFloorY; y <= 184; y += 4 {
		for z := int64(0); z < districtScale*2; z += 4 {
			for x := int64(0); x < districtScale*2; x += 4 {
				block := generated.BlockAt(seed, game.BlockPosition{
					X: int32(originX + x),
					Y: y,
					Z: int32(originZ + z),
				})
				if block != game.Air {
					seen[block] = struct{}{}
				}
			}
		}
	}

	if len(seen) < 12 {
		t.Fatalf("sampled only %d non-air block types, want at least 12", len(seen))
	}

	glassFound := false
	for _, glass := range []game.Block{
		game.LightBlueStainedGlass,
		game.GrayStainedGlass,
		game.PurpleStainedGlass,
		game.CyanStainedGlass,
		game.OrangeStainedGlass,
		game.MagentaStainedGlass,
		game.BlueStainedGlass,
	} {
		if _, ok := seen[glass]; ok {
			glassFound = true
			break
		}
	}

	if !glassFound {
		t.Fatal("sample contains no stained-glass facade blocks")
	}
}

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}
	seed := int64(-24680)
	originX, originZ := cityOrigins(seed)
	chunk := game.ChunkPosition{
		X: blockChunkCoordinate(int32(originX + lotScale + 12)),
		Z: blockChunkCoordinate(int32(originZ + lotScale + 12)),
	}
	sectionMinY := int32(80)

	var blocks [game.SectionVolume]game.Block
	_, uniform := generated.GenerateSection(seed, chunk, sectionMinY, &blocks)
	if uniform {
		t.Fatal("sample building section unexpectedly reported as uniform")
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localY := range int32(game.ChunkWidth) {
		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				position := game.BlockPosition{
					X: chunkMinX + localX,
					Y: sectionMinY + localY,
					Z: chunkMinZ + localZ,
				}
				index := localY*256 + localZ*16 + localX
				want := generated.BlockAt(seed, position)

				if blocks[index] != want {
					t.Fatalf("section block at %+v = %d, want %d", position, blocks[index], want)
				}
			}
		}
	}
}

func TestGeneratorIsDeterministicAcrossNegativeCoordinates(t *testing.T) {
	generated := Generator{}
	seed := int64(987654321)
	positions := []game.BlockPosition{
		{X: -4096, Y: 64, Z: -4096},
		{X: -1234, Y: 97, Z: 567},
		{X: 812, Y: 143, Z: -991},
		{X: 2048, Y: 86, Z: 2048},
	}

	for _, position := range positions {
		first := generated.BlockAt(seed, position)
		second := generated.BlockAt(seed, position)

		if first != second {
			t.Fatalf("block at %+v changed from %d to %d", position, first, second)
		}
	}
}

func TestGenerationBounds(t *testing.T) {
	generated := Generator{}
	minY, maxY, ok := generated.GenerationBounds(0, game.ChunkPosition{X: -1000, Z: 1000})
	if !ok {
		t.Fatal("babel unexpectedly reported an empty chunk")
	}

	if minY != foundationMinY || maxY != maxBuildY {
		t.Fatalf("bounds = %d..%d, want %d..%d", minY, maxY, foundationMinY, maxBuildY)
	}
}

func blockChunkCoordinate(coordinate int32) int32 {
	chunk := coordinate / game.ChunkWidth
	if coordinate%game.ChunkWidth < 0 {
		chunk--
	}

	return chunk
}
