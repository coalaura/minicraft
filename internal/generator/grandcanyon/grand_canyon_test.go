package grandcanyon

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestGrandCanyonDeterminism(t *testing.T) {
	generator := Generator{}
	seed := int64(123456789)

	for z := int32(-32); z <= 32; z += 8 {
		for x := int32(-32); x <= 32; x += 8 {
			col1 := columnAt(seed, x, z)
			col2 := columnAt(seed, x, z)

			if col1 != col2 {
				t.Fatalf("columnAt non-deterministic at (%d, %d): %+v vs %+v", x, z, col1, col2)
			}

			for y := int32(30); y <= 190; y += 10 {
				pos := game.BlockPosition{X: x, Y: y, Z: z}

				b1 := generator.BlockAt(seed, pos)
				b2 := generator.BlockAt(seed, pos)

				if b1 != b2 {
					t.Fatalf("BlockAt non-deterministic at (%d, %d, %d): %v vs %v", x, y, z, b1, b2)
				}
			}
		}
	}
}

func TestGrandCanyonElevationRange(t *testing.T) {
	seed := int64(987654321)

	minObserved := int32(1000)
	maxObserved := int32(-1000)

	for z := int32(-1000); z <= 1000; z += 25 {
		for x := int32(-1000); x <= 1000; x += 25 {
			col := columnAt(seed, x, z)

			if col.height < minObserved {
				minObserved = col.height
			}

			if col.height > maxObserved {
				maxObserved = col.height
			}

			if col.height < 34 || col.height > 200 {
				t.Fatalf("Elevation out of expected range at (%d, %d): %d", x, z, col.height)
			}
		}
	}

	if minObserved > riverLevel {
		t.Fatalf("Expected canyon floor below riverLevel (%d), got min %d", riverLevel, minObserved)
	}

	if maxObserved < 165 {
		t.Fatalf("Expected plateau rim above 165, got max %d", maxObserved)
	}
}

func TestGrandCanyonStratigraphy(t *testing.T) {
	generator := Generator{}
	seed := int64(42)

	blocksSeen := make(map[game.Block]bool)

	for z := int32(-500); z <= 500; z += 20 {
		for x := int32(-500); x <= 500; x += 20 {
			col := columnAt(seed, x, z)

			for y := int32(10); y <= col.height; y += 4 {
				pos := game.BlockPosition{X: x, Y: y, Z: z}
				block := generator.BlockAt(seed, pos)
				blocksSeen[block] = true
			}
		}
	}

	expectedBlocks := []game.Block{
		palette.deepslate,
		palette.sandstone,
		palette.redSandstone,
		palette.terracotta,
		palette.orangeTerracotta,
		palette.yellowTerracotta,
		palette.redTerracotta,
		palette.brownTerracotta,
		palette.whiteTerracotta,
	}

	for _, expected := range expectedBlocks {
		if !blocksSeen[expected] {
			t.Errorf("Expected strata block %v to appear in world", expected)
		}
	}
}

func TestGrandCanyonRiverCorridor(t *testing.T) {
	generator := Generator{}
	seed := int64(42)

	foundWater := false
	foundGravelOrMud := false

	for z := int32(-1000); z <= 1000; z += 10 {
		for x := int32(-1000); x <= 1000; x += 10 {
			col := columnAt(seed, x, z)
			if col.isRiverBed && col.height < riverLevel {
				posWater := game.BlockPosition{X: x, Y: riverLevel, Z: z}

				block := generator.BlockAt(seed, posWater)
				if block == game.Water {
					foundWater = true
				}

				posBed := game.BlockPosition{X: x, Y: col.height, Z: z}

				bedBlock := generator.BlockAt(seed, posBed)
				if bedBlock == palette.gravel || bedBlock == palette.mud || bedBlock == palette.redSand {
					foundGravelOrMud = true
				}
			}
		}
	}

	if !foundWater {
		t.Fatalf("Expected river water to be generated at river level")
	}

	if !foundGravelOrMud {
		t.Fatalf("Expected riverbed gravel/mud/sand to be generated")
	}
}

func TestGrandCanyonTemplesAndButtes(t *testing.T) {
	seed := int64(42)

	foundElevatedFormation := false

	for z := int32(-800); z <= 800; z += 15 {
		for x := int32(-800); x <= 800; x += 15 {
			col := columnAt(seed, x, z)

			if col.canyonStrength > 0.30 && col.canyonStrength < 0.70 && col.height > 115 && col.height < 185 {
				foundElevatedFormation = true

				break
			}
		}

		if foundElevatedFormation {
			break
		}
	}

	if !foundElevatedFormation {
		t.Fatalf("Expected isolated temple/butte formation within canyon amphitheater")
	}
}

func TestGrandCanyonFeatures(t *testing.T) {
	generator := Generator{}
	seed := int64(42)

	foundCactus := false
	foundDeadBush := false
	foundBoulder := false

	for z := int32(-600); z <= 600; z += 5 {
		for x := int32(-600); x <= 600; x += 5 {
			col := columnAt(seed, x, z)

			for y := col.height + 1; y <= col.height+3; y++ {
				pos := game.BlockPosition{X: x, Y: y, Z: z}
				b := generator.BlockAt(seed, pos)

				if b == palette.cactus {
					foundCactus = true
				}

				if b == palette.deadBush {
					foundDeadBush = true
				}

				if b == palette.granite || b == palette.andesite || b == palette.cobblestone {
					foundBoulder = true
				}
			}
		}
	}

	if !foundCactus {
		t.Errorf("Expected cactus feature to generate")
	}

	if !foundDeadBush {
		t.Errorf("Expected dead bush feature to generate")
	}

	if !foundBoulder {
		t.Errorf("Expected river boulder feature to generate")
	}
}

func TestGrandCanyonSpawn(t *testing.T) {
	generator := Generator{}
	seeds := [...]int64{1, 42, 100, 999999}

	for _, seed := range seeds {
		spawn := generator.Spawn(seed)

		if spawn.Y < 155 || spawn.Y > 200 {
			t.Fatalf("Spawn Y=%v out of expected rim plateau range for seed %d", spawn.Y, seed)
		}

		footPos := game.BlockPosition{
			X: int32(spawn.X),
			Y: int32(spawn.Y) - 1,
			Z: int32(spawn.Z),
		}

		footBlock := generator.BlockAt(seed, footPos)
		if footBlock == game.Air || footBlock == game.Water {
			t.Fatalf("Spawn footing at %v is not solid ground: %v", footPos, footBlock)
		}

		abovePos := game.BlockPosition{
			X: int32(spawn.X),
			Y: int32(spawn.Y),
			Z: int32(spawn.Z),
		}

		aboveBlock := generator.BlockAt(seed, abovePos)
		if aboveBlock != game.Air {
			t.Fatalf("Spawn position at %v is obstructed by %v", abovePos, aboveBlock)
		}
	}
}

func TestGenerateSectionConsistency(t *testing.T) {
	generator := Generator{}
	seed := int64(42)
	chunkPos := game.ChunkPosition{X: 2, Z: -3}

	prepared := generator.GenerateChunk(seed, chunkPos)

	for sectionY := int32(-64); sectionY <= 192; sectionY += 16 {
		var blocks [game.SectionVolume]game.Block

		uniformBlock, uniform := prepared.GenerateSection(sectionY, &blocks)

		for ly := range int32(16) {
			for lz := range int32(16) {
				for lx := range int32(16) {
					worldPos := game.BlockPosition{
						X: chunkPos.X*16 + lx,
						Y: sectionY + ly,
						Z: chunkPos.Z*16 + lz,
					}

					expected := generator.BlockAt(seed, worldPos)
					actual := blocks[ly*256+lz*16+lx]

					if uniform {
						actual = uniformBlock
					}

					if expected != actual {
						t.Fatalf("Mismatch at (%d, %d, %d): BlockAt=%v, GenerateSection=%v", worldPos.X, worldPos.Y, worldPos.Z, expected, actual)
					}
				}
			}
		}
	}
}

func BenchmarkBlockAt(b *testing.B) {
	generator := Generator{}

	positions := [...]game.BlockPosition{
		{X: 0, Y: 64, Z: 0},
		{X: 128, Y: 150, Z: -256},
		{X: 512, Y: 40, Z: 512},
		{X: -1024, Y: 180, Z: 2048},
	}

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		_ = generator.BlockAt(42, positions[index%len(positions)])
	}
}

func BenchmarkGenerateChunk(b *testing.B) {
	generator := Generator{}
	chunk := game.ChunkPosition{X: 12, Z: -8}

	b.ReportAllocs()

	for b.Loop() {
		_ = generator.GenerateChunk(42, chunk)
	}
}

func BenchmarkGenerateSection(b *testing.B) {
	generator := Generator{}
	chunk := game.ChunkPosition{X: 12, Z: -8}
	prepared := generator.GenerateChunk(42, chunk)

	sections := [...]int32{-64, -48, -32, 0, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192}

	var blocks [game.SectionVolume]game.Block

	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		_, _ = prepared.GenerateSection(sections[index%len(sections)], &blocks)
	}
}

func BenchmarkChunkSections(b *testing.B) {
	generator := Generator{}
	chunk := game.ChunkPosition{X: 12, Z: -8}
	sections := [...]int32{-64, -48, -32, -16, 0, 16, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192}

	b.Run("prepared_chunk", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			prepared := generator.GenerateChunk(42, chunk)

			var blocks [game.SectionVolume]game.Block

			for _, sectionMinY := range sections {
				_, _ = prepared.GenerateSection(sectionMinY, &blocks)
			}
		}
	})

	b.Run("repeated_section_preparation", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			var blocks [game.SectionVolume]game.Block

			for _, sectionMinY := range sections {
				_, _ = generator.GenerateSection(42, chunk, sectionMinY, &blocks)
			}
		}
	})
}
