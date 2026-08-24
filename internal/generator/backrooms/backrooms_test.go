package backrooms

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{0, 1, -1, 123456789, -987654321} {
		spawn := generated.Spawn(seed)
		position := game.BlockPosition{
			X: int32(math.Floor(spawn.X)),
			Y: int32(math.Floor(spawn.Y)),
			Z: int32(math.Floor(spawn.Z)),
		}

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

func TestGeneratorHasFiniteVerticalBounds(t *testing.T) {
	generated := Generator{}
	minY, maxY, ok := generated.GenerationBounds(0, game.ChunkPosition{})
	if !ok {
		t.Fatal("backrooms unexpectedly reported an empty chunk")
	}

	if minY != foundationY || maxY != ceilingY {
		t.Fatalf("generation bounds = [%d, %d], want [%d, %d]", minY, maxY, foundationY, ceilingY)
	}

	if normalCeilingY-floorY != 4 {
		t.Fatalf("normal floor-to-ceiling height = %d, want 4", normalCeilingY-floorY)
	}

	if ceilingY-floorY != 5 {
		t.Fatalf("maximum floor-to-ceiling height = %d, want 5", ceilingY-floorY)
	}

	for _, y := range []int32{foundationY - 1, ceilingY + 1, -64, 319} {
		if block := generated.BlockAt(0, game.BlockPosition{Y: y}); block != game.Air {
			t.Fatalf("block at y=%d = %d, want air", y, block)
		}
	}
}

func TestGeneratorProducesVariedZones(t *testing.T) {
	layouts := make(map[layout]struct{})
	palettes := make(map[palette]struct{})

	for zoneZ := int64(-12); zoneZ <= 12; zoneZ++ {
		for zoneX := int64(-12); zoneX <= 12; zoneX++ {
			current := zoneAt(0, zoneX*zoneSize, zoneZ*zoneSize)
			layouts[current.layout] = struct{}{}
			palettes[current.palette] = struct{}{}
		}
	}

	if len(layouts) < 6 {
		t.Fatalf("sampled %d distinct layouts, want at least 6", len(layouts))
	}

	if len(palettes) < 4 {
		t.Fatalf("sampled %d distinct palettes, want all 4", len(palettes))
	}
}

func TestGeneratorUsesNormalAndTallCeilings(t *testing.T) {
	heights := make(map[int32]struct{})

	for zoneZ := int64(-12); zoneZ <= 12; zoneZ++ {
		for zoneX := int64(-12); zoneX <= 12; zoneX++ {
			current := zoneAt(0, zoneX*zoneSize, zoneZ*zoneSize)
			heights[zoneCeilingY(current)] = struct{}{}
		}
	}

	if _, ok := heights[normalCeilingY]; !ok {
		t.Fatal("sample contained no normal-height zones")
	}

	if _, ok := heights[ceilingY]; !ok {
		t.Fatal("sample contained no tall zones")
	}
}

func TestPalettePersistsAcrossRegion(t *testing.T) {
	for _, seed := range []int64{0, 1, -1, 9918273} {
		for regionZ := int64(-2); regionZ <= 2; regionZ++ {
			for regionX := int64(-2); regionX <= 2; regionX++ {
				baseZoneX := regionX * paletteRegionSize
				baseZoneZ := regionZ * paletteRegionSize
				want := zoneAt(seed, baseZoneX*zoneSize, baseZoneZ*zoneSize).palette

				for offsetZ := int64(0); offsetZ < paletteRegionSize; offsetZ++ {
					for offsetX := int64(0); offsetX < paletteRegionSize; offsetX++ {
						current := zoneAt(seed, (baseZoneX+offsetX)*zoneSize, (baseZoneZ+offsetZ)*zoneSize)
						if current.palette != want {
							t.Fatalf(
								"seed %d palette region (%d,%d) changed at offset (%d,%d): got %d, want %d",
								seed,
								regionX,
								regionZ,
								offsetX,
								offsetZ,
								current.palette,
								want,
							)
						}
					}
				}
			}
		}
	}
}

func TestZoneBoundariesAlwaysHavePassages(t *testing.T) {
	for _, seed := range []int64{0, 1, -1, 9918273} {
		for segment := int64(-4); segment <= 4; segment++ {
			for boundary := int64(-4); boundary <= 4; boundary++ {
				for _, vertical := range []bool{false, true} {
					open := 0
					for local := int64(1); local < zoneSize-1; local++ {
						if boundaryOpening(seed, segment, local, boundary, vertical) {
							open++
						}
					}

					if open < 8 {
						t.Fatalf("seed %d boundary (%d,%d) vertical=%v has only %d open blocks", seed, boundary, segment, vertical, open)
					}
				}
			}
		}
	}
}

func TestZoneSpinesConnectPreferredEntrances(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{0, 1, -1, 9918273} {
		for zoneZ := int64(-2); zoneZ <= 2; zoneZ++ {
			for zoneX := int64(-2); zoneX <= 2; zoneX++ {
				entrances := [][2]int64{
					{0, preferredBoundaryCenter(seed, zoneZ, zoneX, true)},
					{zoneSize - 1, preferredBoundaryCenter(seed, zoneZ, zoneX+1, true)},
					{preferredBoundaryCenter(seed, zoneX, zoneZ, false), 0},
					{preferredBoundaryCenter(seed, zoneX, zoneZ+1, false), zoneSize - 1},
				}

				for _, entrance := range entrances {
					if !walkableAtLocal(generated, seed, zoneX, zoneZ, entrance[0], entrance[1]) {
						t.Fatalf("seed %d zone (%d,%d) preferred entrance (%d,%d) is blocked", seed, zoneX, zoneZ, entrance[0], entrance[1])
					}
				}

				reachable := floodZone(generated, seed, zoneX, zoneZ, entrances[0])
				for _, entrance := range entrances[1:] {
					if !reachable[entrance[1]][entrance[0]] {
						t.Fatalf("seed %d zone (%d,%d) entrance (%d,%d) is disconnected", seed, zoneX, zoneZ, entrance[0], entrance[1])
					}
				}
			}
		}
	}
}

func TestDoorwayHasLintel(t *testing.T) {
	current := zoneAt(0, 0, 0)
	blocks := blocksForPalette(current.palette)

	currentCeilingY := zoneCeilingY(current)

	for _, y := range []int64{int64(floorY + 1), int64(floorY + 2)} {
		if block := structureBlock(0, 0, y, 0, current, blocks, structureDoorway); block != game.Air {
			t.Fatalf("doorway block at y=%d = %d, want air", y, block)
		}
	}

	if block := structureBlock(0, 0, int64(currentCeilingY-1), 0, current, blocks, structureDoorway); block == game.Air {
		t.Fatal("doorway lintel is air")
	}
}

func TestCubiclePartitionsStopBelowCeiling(t *testing.T) {
	generated := Generator{}
	found := false

	for z := int32(-384); z <= 384 && !found; z++ {
		for x := int32(-384); x <= 384; x++ {
			current := zoneAt(0, int64(x), int64(z))
			if current.layout != layoutCubicles || structureAt(0, current) != structurePartition {
				continue
			}

			found = true

			for _, y := range []int32{floorY + 1, floorY + 2} {
				if block := generated.BlockAt(0, game.BlockPosition{X: x, Y: y, Z: z}); block == game.Air {
					t.Fatalf("cubicle partition at (%d,%d) y=%d is air", x, z, y)
				}
			}

			if block := generated.BlockAt(0, game.BlockPosition{X: x, Y: zoneCeilingY(current) - 1, Z: z}); block != game.Air {
				t.Fatalf("cubicle partition at (%d,%d) reaches ceiling: block=%d", x, z, block)
			}

			break
		}
	}

	if !found {
		t.Fatal("could not find a generated cubicle partition")
	}
}

func TestInternalDoorWidthsAreNeverOneBlock(t *testing.T) {
	for hash := uint64(0); hash < 4096; hash++ {
		if width := doorWidthForHash(hash, 2, 3); width < 2 {
			t.Fatalf("maze doorway width = %d, want at least 2", width)
		}

		if width := doorWidthForHash(hash, 3, 5); width < 3 {
			t.Fatalf("room doorway width = %d, want at least 3", width)
		}
	}
}

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}

	for _, seed := range []int64{0, 17, -29} {
		for _, chunk := range []game.ChunkPosition{{X: 0, Z: 0}, {X: -3, Z: 5}, {X: 7, Z: -2}} {
			for _, sectionMinY := range []int32{48, 64} {
				var blocks [game.SectionVolume]game.Block
				_, uniform := generated.GenerateSection(seed, chunk, sectionMinY, &blocks)

				allSame := true
				first := blocks[0]

				for localY := range int32(game.ChunkWidth) {
					for localZ := range int32(game.ChunkWidth) {
						for localX := range int32(game.ChunkWidth) {
							index := localY*256 + localZ*16 + localX
							position := game.BlockPosition{
								X: chunk.X*game.ChunkWidth + localX,
								Y: sectionMinY + localY,
								Z: chunk.Z*game.ChunkWidth + localZ,
							}

							want := generated.BlockAt(seed, position)
							if blocks[index] != want {
								t.Fatalf("seed %d chunk %+v section %d block %+v = %d, want %d", seed, chunk, sectionMinY, position, blocks[index], want)
							}

							if blocks[index] != first {
								allSame = false
							}
						}
					}
				}

				if uniform != allSame {
					t.Fatalf("seed %d chunk %+v section %d uniform=%v, actual=%v", seed, chunk, sectionMinY, uniform, allSame)
				}
			}
		}
	}
}

func TestOutOfRangeSectionIsUniformAir(t *testing.T) {
	generated := Generator{}
	var blocks [game.SectionVolume]game.Block

	block, uniform := generated.GenerateSection(0, game.ChunkPosition{}, 80, &blocks)
	if block != game.Air || !uniform {
		t.Fatalf("section above backrooms = (%d, %v), want (air, true)", block, uniform)
	}
}

func floodZone(generated Generator, seed, zoneX, zoneZ int64, start [2]int64) [zoneSize][zoneSize]bool {
	var visited [zoneSize][zoneSize]bool
	queue := make([][2]int64, 0, zoneSize*zoneSize)

	visited[start[1]][start[0]] = true
	queue = append(queue, start)

	directions := [][2]int64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, direction := range directions {
			x := current[0] + direction[0]
			z := current[1] + direction[1]

			if x < 0 || x >= zoneSize || z < 0 || z >= zoneSize || visited[z][x] {
				continue
			}

			if !walkableAtLocal(generated, seed, zoneX, zoneZ, x, z) {
				continue
			}

			visited[z][x] = true
			queue = append(queue, [2]int64{x, z})
		}
	}

	return visited
}

func walkableAtLocal(generated Generator, seed, zoneX, zoneZ, localX, localZ int64) bool {
	worldX := zoneX*zoneSize - zoneSize/2 + localX
	worldZ := zoneZ*zoneSize - zoneSize/2 + localZ

	return generated.BlockAt(seed, game.BlockPosition{
		X: int32(worldX),
		Y: floorY + 1,
		Z: int32(worldZ),
	}) == game.Air
}
