package backrooms

import (
	"math"
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestGeneratorSpawnIsOpenAndSupported(t *testing.T) {
	generated := Generator{}

	spawnSeeds := []int64{0, 1, -1, 123456789, -987654321}

	for _, seed := range spawnSeeds {
		spawn := generated.Spawn(seed)

		position := game.BlockPosition{
			X: int32(math.Floor(spawn.X)),
			Y: int32(math.Floor(spawn.Y)),
			Z: int32(math.Floor(spawn.Z)),
		}

		block := generated.BlockAt(seed, position)
		if block != game.Air {
			t.Fatalf("seed %d spawn block = %d, want air", seed, block)
		}

		position.Y++

		block = generated.BlockAt(seed, position)
		if block != game.Air {
			t.Fatalf("seed %d block above spawn = %d, want air", seed, block)
		}

		position.Y -= 2

		block = generated.BlockAt(seed, position)
		if block == game.Air {
			t.Fatalf("seed %d block below spawn is air", seed)
		}
	}
}

func TestGeneratorFillsVerticalWorldWithLayers(t *testing.T) {
	generated := Generator{}

	minY, maxY, ok := generated.GenerationBounds(0, game.ChunkPosition{})
	if !ok {
		t.Fatal("backrooms unexpectedly reported an empty chunk")
	}

	if minY != worldMinY || maxY != worldMaxY {
		t.Fatalf("generation bounds = [%d,%d], want [%d,%d]", minY, maxY, worldMinY, worldMaxY)
	}

	for layer := lowestLayerIndex; layer <= highestLayerIndex; layer += 5 {
		floor := layerFloorY(layer)

		block := generated.BlockAt(17, game.BlockPosition{Y: floor})
		if block == game.Air {
			t.Fatalf("layer %d floor y=%d is air", layer, floor)
		}

		// Every ordinary layer has usable room above its floor somewhere near the origin.
		var foundAir bool

		for z := int32(-8); z <= 8 && !foundAir; z++ {
			for x := int32(-8); x <= 8; x++ {
				if generated.BlockAt(17, game.BlockPosition{X: x, Y: floor + 1, Z: z}) == game.Air {
					foundAir = true

					break
				}
			}
		}

		if !foundAir {
			t.Fatalf("layer %d has no sampled open interior", layer)
		}
	}
}

func TestBreakingCeilingNeverExposesOutsideBetweenLayers(t *testing.T) {
	generated := Generator{}

	ceilingSeeds := []int64{0, 1, 9918273}

	for _, seed := range ceilingSeeds {
		for layer := int64(-8); layer <= 8; layer++ {
			current := zoneAtLayer(seed, 0, 0, layer)

			ceiling := layerFloorY(layer) + (zoneCeilingY(current) - floorY)
			nextFloor := layerFloorY(layer + 1)

			for y := ceiling + 1; y <= nextFloor; y++ {
				block := generated.BlockAt(seed, game.BlockPosition{X: 0, Y: y, Z: 0})
				if block == game.Air {
					t.Fatalf("seed %d layer %d has outside/air gap at y=%d between ceiling=%d and next floor=%d", seed, layer, y, ceiling, nextFloor)
				}
			}
		}
	}
}

func TestLayersUseDifferentDeterministicPlans(t *testing.T) {
	planSeeds := []int64{0, 17, -29}

	for _, seed := range planSeeds {
		seen := make(map[uint64]struct{})

		for layer := int64(-6); layer <= 6; layer++ {
			current := zoneAtLayer(seed, 0, 0, layer)
			seen[current.hash] = struct{}{}
		}

		if len(seen) < 10 {
			t.Fatalf("seed %d sampled only %d distinct vertical zone plans", seed, len(seen))
		}
	}
}

func TestOrdinaryLayerConnectorsContainRealStaircases(t *testing.T) {
	generated := Generator{}
	found := 0

	for layer := int64(-3); layer <= 3 && found < 4; layer++ {
		for zoneZ := int64(-10); zoneZ <= 10 && found < 4; zoneZ++ {
			for zoneX := int64(-10); zoneX <= 10 && found < 4; zoneX++ {
				if !layerConnectorEnabled(0, zoneX, zoneZ, layer) {
					continue
				}

				originX := zoneX*zoneSize - zoneSize/2
				originZ := zoneZ*zoneSize - zoneSize/2

				current := zoneAtLayer(0, originX, originZ, layer)

				plan := connectorPlanForZone(current)

				wantFacing := stairFacing(plan)

				for step := range 8 {
					stepX := plan.startX + plan.stepX*int64(step)
					stepZ := plan.startZ + plan.stepZ*int64(step)

					for lane := range int64(2) {
						localX := stepX
						localZ := stepZ + lane

						if plan.stepZ != 0 {
							localX = stepX + lane
							localZ = stepZ
						}

						block := generated.BlockAt(0, game.BlockPosition{
							X: int32(originX + localX),
							Y: layerFloorY(layer) + 1 + int32(step),
							Z: int32(originZ + localZ),
						})

						if block.Behavior() != game.BlockBehaviorStairs {
							t.Fatalf("layer %d connector stair step %d lane %d = %d", layer, step, lane, block)
						}

						facing, ok := block.Property("facing")

						if ok && facing != wantFacing {
							t.Fatalf("connector facing=%q want=%q", facing, wantFacing)
						}
					}
				}

				found++
			}
		}
	}

	if found < 4 {
		t.Fatalf("found only %d ordinary vertical connectors", found)
	}
}

func TestGrandAtriaAreRareAndSpanMultipleLayers(t *testing.T) {
	generated := Generator{}
	found := 0
	totalGroups := 0

	for group := int64(-2); group <= 4; group++ {
		layer := group * verticalGroupSize

		for zoneZ := int64(-12); zoneZ <= 12; zoneZ++ {
			for zoneX := int64(-12); zoneX <= 12; zoneX++ {
				totalGroups++

				originX := zoneX*zoneSize - zoneSize/2
				originZ := zoneZ*zoneSize - zoneSize/2

				current := zoneAtLayer(0, originX, originZ, layer)

				spec := grandAtriumForZone(0, current)
				if !spec.enabled {
					continue
				}

				found++

				if spec.span < 2 || spec.span > 4 {
					t.Fatalf("atrium span=%d, want 2..4", spec.span)
				}

				centerX := originX + (spec.x0+spec.x1)/2 + 4
				centerZ := originZ + (spec.z0+spec.z1)/2 + 4

				firstUpperFloor := layerFloorY(spec.anchorLayer + 1)

				centerBlock := generated.BlockAt(0, game.BlockPosition{X: int32(centerX), Y: firstUpperFloor, Z: int32(centerZ)})
				if centerBlock != game.Air {
					t.Fatalf("atrium center at intermediate floor is %d, want air", centerBlock)
				}
			}
		}
	}

	if found == 0 {
		t.Fatal("sample contained no grand atria")
	}

	if found*100 < totalGroups*2 {
		t.Fatalf("grand atria still too rare: %d/%d sampled groups", found, totalGroups)
	}

	if found*20 > totalGroups {
		t.Fatalf("grand atria too common: %d/%d sampled groups", found, totalGroups)
	}
}

func TestGrandAtriumContainsIntegratedStairsBalconiesColumnsRailsAndLights(t *testing.T) {
	generated := Generator{}

	for group := int64(-2); group <= 5; group++ {
		layer := group * verticalGroupSize

		for zoneZ := int64(-16); zoneZ <= 16; zoneZ++ {
			for zoneX := int64(-16); zoneX <= 16; zoneX++ {
				originX := zoneX*zoneSize - zoneSize/2
				originZ := zoneZ*zoneSize - zoneSize/2

				current := zoneAtLayer(0, originX, originZ, layer)

				spec := grandAtriumForZone(0, current)
				if !spec.enabled {
					continue
				}

				columnX := originX + spec.x0 + 7
				columnZ := originZ + spec.z0 + 7

				middleY := layerFloorY(spec.anchorLayer) + 3

				columnBlock := generated.BlockAt(0, game.BlockPosition{X: int32(columnX), Y: middleY, Z: int32(columnZ)})
				if columnBlock == game.Air {
					t.Fatal("grand atrium support column is air")
				}

				upperFloor := layerFloorY(spec.anchorLayer + 1)

				balconyX := originX + spec.x0 + 2
				balconyZ := originZ + (spec.z0+spec.z1)/2

				balconyBlock := generated.BlockAt(0, game.BlockPosition{X: int32(balconyX), Y: upperFloor, Z: int32(balconyZ)})
				if balconyBlock == game.Air {
					t.Fatal("grand atrium balcony floor is air")
				}

				innerZ := spec.z0 + 5
				railX := spec.x1 - 7

				rail := generated.BlockAt(0, game.BlockPosition{X: int32(originX + railX), Y: upperFloor + 1, Z: int32(originZ + innerZ)})

				if rail.Behavior() != game.BlockBehaviorPane {
					t.Fatalf("grand atrium rail = %d, want pane", rail)
				}

				railDirections := []string{"east", "west"}

				for _, direction := range railDirections {
					connected, ok := rail.Property(direction)
					if !ok || connected != "true" {
						t.Fatalf("grand atrium rail %s connection = %q, want true", direction, connected)
					}
				}

				plan := atriumStairPlan(spec, 0)

				stairY := layerFloorY(spec.anchorLayer) + 1

				stair := generated.BlockAt(0, game.BlockPosition{X: int32(originX + plan.startX), Y: stairY, Z: int32(originZ + plan.startZ)})
				if stair.Behavior() != game.BlockBehaviorStairs {
					t.Fatalf("grand atrium staircase start = %d, want stairs", stair)
				}

				sideX := plan.startX - plan.stepZ
				sideZ := plan.startZ - plan.stepX

				side := generated.BlockAt(0, game.BlockPosition{X: int32(originX + sideX), Y: stairY + 3, Z: int32(originZ + sideZ)})
				if side != game.Air {
					t.Fatalf("grand atrium staircase side = %d, want open air", side)
				}

				landingRailX := plan.startX + plan.stepX*8
				landingRailZ := plan.startZ + 1

				landingRail := generated.BlockAt(0, game.BlockPosition{X: int32(originX + landingRailX), Y: upperFloor + 1, Z: int32(originZ + landingRailZ)})
				if landingRail.Behavior() != game.BlockBehaviorPane {
					t.Fatalf("grand atrium landing rail = %d, want pane", landingRail)
				}

				connectedToStairs, ok := landingRail.Property("west")
				if !ok || connectedToStairs != "false" {
					t.Fatalf("grand atrium landing rail stair connection = %q, want false", connectedToStairs)
				}

				lights := 0
				stairs := 0

				for y := layerFloorY(spec.anchorLayer); y <= layerFloorY(spec.anchorLayer+spec.span-1)+5; y++ {
					for z := spec.z0; z <= spec.z1; z++ {
						for x := spec.x0; x <= spec.x1; x++ {
							block := generated.BlockAt(0, game.BlockPosition{X: int32(originX + x), Y: y, Z: int32(originZ + z)})

							emission, _ := block.LightProperties()
							if emission > 0 {
								lights++
							}

							if block.Behavior() == game.BlockBehaviorStairs {
								stairs++
							}
						}
					}
				}

				if lights == 0 {
					t.Fatal("grand atrium contains no lights")
				}

				if stairs < 16 {
					t.Fatalf("grand atrium contains %d stairs, want at least 16", stairs)
				}

				return
			}
		}
	}

	t.Fatal("sample contained no grand atrium")
}

func TestOutOfWorldSectionIsUniformAir(t *testing.T) {
	generated := Generator{}

	var blocks [game.SectionVolume]game.Block

	sectionMinYs := []int32{-96, 320, 336}

	for _, sectionMinY := range sectionMinYs {
		block, uniform := generated.GenerateSection(0, game.ChunkPosition{}, sectionMinY, &blocks)
		if block != game.Air || !uniform {
			t.Fatalf("section %d outside world = (%d,%v), want (air,true)", sectionMinY, block, uniform)
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
	paletteSeeds := []int64{0, 1, -1, 9918273}

	for _, seed := range paletteSeeds {
		for regionZ := int64(-2); regionZ <= 2; regionZ++ {
			for regionX := int64(-2); regionX <= 2; regionX++ {
				baseZoneX := regionX * paletteRegionSize
				baseZoneZ := regionZ * paletteRegionSize

				want := zoneAt(seed, baseZoneX*zoneSize, baseZoneZ*zoneSize).palette

				for offsetZ := range int64(paletteRegionSize) {
					for offsetX := range int64(paletteRegionSize) {
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

func TestRareFeaturesArePresentAndSparse(t *testing.T) {
	counts := make(map[zoneFeature]int)
	total := 0

	for zoneZ := int64(-64); zoneZ <= 64; zoneZ++ {
		for zoneX := int64(-64); zoneX <= 64; zoneX++ {
			current := zoneAt(0, zoneX*zoneSize, zoneZ*zoneSize)

			counts[current.feature]++
			total++
		}
	}

	for feature := featureLibrary; feature <= featureMachineRoom; feature++ {
		if counts[feature] == 0 {
			t.Fatalf("sample contained no zones for feature %d", feature)
		}
	}

	featured := total - counts[featureNone]
	if featured*100 < total*10 || featured*100 > total*17 {
		t.Fatalf("featured zones = %d/%d, want roughly 10%%..17%%", featured, total)
	}

	if counts[featureLibrary]*100 >= total*2 {
		t.Fatalf("library zones = %d/%d, expected them to remain rare", counts[featureLibrary], total)
	}
}

func TestAmbientDoorsIncludeFalseAndUsefulDoors(t *testing.T) {
	generated := Generator{}
	seed := int64(0)

	total := 0
	doors := 0
	falseDoors := 0
	usefulDoors := 0
	validatedFalseDoor := false
	validatedUsefulDoor := false

	for zoneZ := int64(-64); zoneZ <= 64; zoneZ++ {
		for zoneX := int64(-64); zoneX <= 64; zoneX++ {
			current := zoneAt(seed, zoneX*zoneSize, zoneZ*zoneSize)

			spec := ambientDoorSpecForZone(seed, current)

			total++

			if !spec.enabled {
				continue
			}

			doors++

			if spec.falseDoor {
				falseDoors++
			} else {
				usefulDoors++
			}

			if (spec.falseDoor && validatedFalseDoor) || (!spec.falseDoor && validatedUsefulDoor) {
				continue
			}

			localX := spec.center
			localZ := spec.line

			backX := localX
			backZ := spec.line + spec.direction

			if spec.vertical {
				localX = spec.line
				localZ = spec.center

				backX = spec.line + spec.direction
				backZ = spec.center
			}

			worldX := zoneX*zoneSize - zoneSize/2 + localX
			worldZ := zoneZ*zoneSize - zoneSize/2 + localZ

			backWorldX := zoneX*zoneSize - zoneSize/2 + backX
			backWorldZ := zoneZ*zoneSize - zoneSize/2 + backZ

			door := generated.BlockAt(seed, game.BlockPosition{X: int32(worldX), Y: floorY + 1, Z: int32(worldZ)})
			if !isDoorBlock(door) {
				t.Fatalf("ambient door block = %d, want door behavior", door)
			}

			behind := generated.BlockAt(seed, game.BlockPosition{X: int32(backWorldX), Y: floorY + 1, Z: int32(backWorldZ)})

			if spec.falseDoor {
				if behind == game.Air {
					t.Fatal("ambient false door unexpectedly leads into air")
				}

				validatedFalseDoor = true
			} else {
				if behind != game.Air {
					t.Fatalf("ambient useful door leads into block %d", behind)
				}

				validatedUsefulDoor = true
			}
		}
	}

	if doors*100 < total || doors*100 > total*6 {
		t.Fatalf("ambient door zones = %d/%d, want roughly 1%%..6%%", doors, total)
	}

	if falseDoors == 0 || usefulDoors == 0 {
		t.Fatalf("ambient doors false=%d useful=%d, want both kinds", falseDoors, usefulDoors)
	}

	if !validatedFalseDoor {
		t.Fatal("could not validate a generated false ambient door")
	}

	if !validatedUsefulDoor {
		t.Fatal("could not validate a generated useful ambient door")
	}
}

func TestLibraryContainsShelvesAndActualDoors(t *testing.T) {
	generated := Generator{}
	seed := int64(0)

	for zoneZ := int64(-64); zoneZ <= 64; zoneZ++ {
		for zoneX := int64(-64); zoneX <= 64; zoneX++ {
			probe := zoneAt(seed, zoneX*zoneSize, zoneZ*zoneSize)
			if probe.feature != featureLibrary {
				continue
			}

			shelves := 0
			doors := 0

			for localZ := range int64(zoneSize) {
				for localX := range int64(zoneSize) {
					worldX := zoneX*zoneSize - zoneSize/2 + localX
					worldZ := zoneZ*zoneSize - zoneSize/2 + localZ

					shelfLevels := []int32{floorY + 1, floorY + 2}

					for _, y := range shelfLevels {
						block := generated.BlockAt(seed, game.BlockPosition{X: int32(worldX), Y: y, Z: int32(worldZ)})
						if block == game.Bookshelf || block == game.ChiseledBookshelf {
							shelves++
						}

						if isDoorBlock(block) {
							doors++
						}
					}
				}
			}

			if shelves < 20 {
				t.Fatalf("library zone (%d,%d) only contains %d bookshelf blocks", zoneX, zoneZ, shelves)
			}

			if doors < 2 {
				t.Fatalf("library zone (%d,%d) contains %d door blocks, want actual doors", zoneX, zoneZ, doors)
			}

			return
		}
	}

	t.Fatal("could not find a library zone")
}

func TestExpandedRareFeaturesHaveSignatureGeometry(t *testing.T) {
	generated := Generator{}
	seed := int64(0)

	wanted := []zoneFeature{
		featureConference,
		featureBathroom,
		featureRenovation,
		featureWindowRoom,
		featureStorage,
		featureClassroom,
		featureMachineRoom,
	}

	found := make(map[zoneFeature][2]int64)

	for zoneZ := int64(-64); zoneZ <= 64 && len(found) < len(wanted); zoneZ++ {
		for zoneX := int64(-64); zoneX <= 64 && len(found) < len(wanted); zoneX++ {
			current := zoneAt(seed, zoneX*zoneSize, zoneZ*zoneSize)

			for _, feature := range wanted {
				if current.feature == feature {
					if _, exists := found[feature]; !exists {
						found[feature] = [2]int64{zoneX, zoneZ}
					}
				}
			}
		}
	}

	for _, feature := range wanted {
		coords, ok := found[feature]
		if !ok {
			t.Fatalf("could not find feature %d", feature)
		}

		counts := make(map[game.Block]int)
		stairs := 0
		doors := 0
		originX := coords[0]*zoneSize - zoneSize/2
		originZ := coords[1]*zoneSize - zoneSize/2

		for localZ := range int64(zoneSize) {
			for localX := range int64(zoneSize) {
				for y := floorY; y <= floorY+2; y++ {
					block := generated.BlockAt(seed, game.BlockPosition{
						X: int32(originX + localX),
						Y: y,
						Z: int32(originZ + localZ),
					})

					counts[block]++

					if block.Behavior() == game.BlockBehaviorStairs {
						stairs++
					}

					if isDoorBlock(block) {
						doors++
					}
				}
			}
		}

		switch feature {
		case featureConference:
			if counts[game.OakSlab] < 10 || stairs < 4 || doors < 2 {
				t.Fatalf("conference signatures slabs=%d stairs=%d doors=%d", counts[game.OakSlab], stairs, doors)
			}
		case featureBathroom:
			if counts[game.SmoothQuartz] < 20 || doors < 4 {
				t.Fatalf("bathroom signatures quartz=%d doors=%d", counts[game.SmoothQuartz], doors)
			}
		case featureRenovation:
			if counts[game.SmoothStone] < 40 || counts[game.OakPlanks]+counts[game.YellowTerracotta] < 4 {
				t.Fatalf("renovation signatures smooth_stone=%d construction=%d", counts[game.SmoothStone], counts[game.OakPlanks]+counts[game.YellowTerracotta])
			}
		case featureWindowRoom:
			if counts[game.TintedGlass] < 8 || doors < 2 {
				t.Fatalf("window-room signatures tinted_glass=%d doors=%d", counts[game.TintedGlass], doors)
			}
		case featureStorage:
			if counts[game.IronBars] < 4 || counts[game.OakPlanks] < 4 || doors < 2 {
				t.Fatalf("storage signatures bars=%d crates=%d doors=%d", counts[game.IronBars], counts[game.OakPlanks], doors)
			}
		case featureClassroom:
			if counts[game.BlackWool] < 8 || counts[game.OakSlab] < 4 || doors < 2 {
				t.Fatalf("classroom signatures board=%d desks=%d doors=%d", counts[game.BlackWool], counts[game.OakSlab], doors)
			}
		case featureMachineRoom:
			if counts[game.CopperBlock] < 4 || counts[game.IronBars] < 2 || doors < 2 {
				t.Fatalf("machine-room signatures copper=%d bars=%d doors=%d", counts[game.CopperBlock], counts[game.IronBars], doors)
			}
		}
	}
}

func TestDoorGalleryContainsDoorToWall(t *testing.T) {
	generated := Generator{}
	seed := int64(0)

	for zoneZ := int64(-64); zoneZ <= 64; zoneZ++ {
		for zoneX := int64(-64); zoneX <= 64; zoneX++ {
			probe := zoneAt(seed, zoneX*zoneSize, zoneZ*zoneSize)
			if probe.feature != featureDoorGallery {
				continue
			}

			room := featureRoomForZone(probe)
			vertical, line, direction := doorGalleryWall(probe, room)
			coordinate := doorGalleryDoorCoordinate(room, vertical, 0)

			doorLocalX := line
			doorLocalZ := coordinate

			backLocalX := line + direction
			backLocalZ := coordinate

			if !vertical {
				doorLocalX = coordinate
				doorLocalZ = line

				backLocalX = coordinate
				backLocalZ = line + direction
			}

			doorZone := zoneAt(seed, zoneX*zoneSize-zoneSize/2+doorLocalX, zoneZ*zoneSize-zoneSize/2+doorLocalZ)
			backZone := zoneAt(seed, zoneX*zoneSize-zoneSize/2+backLocalX, zoneZ*zoneSize-zoneSize/2+backLocalZ)

			if zoneSpineOpenAt(seed, doorZone) || zoneSpineOpenAt(seed, backZone) {
				continue
			}

			doorWorldX := zoneX*zoneSize - zoneSize/2 + doorLocalX
			doorWorldZ := zoneZ*zoneSize - zoneSize/2 + doorLocalZ

			backWorldX := zoneX*zoneSize - zoneSize/2 + backLocalX
			backWorldZ := zoneZ*zoneSize - zoneSize/2 + backLocalZ

			door := generated.BlockAt(seed, game.BlockPosition{X: int32(doorWorldX), Y: floorY + 1, Z: int32(doorWorldZ)})
			if !isDoorBlock(door) {
				t.Fatalf("door gallery block = %d, want a door", door)
			}

			behind := generated.BlockAt(seed, game.BlockPosition{X: int32(backWorldX), Y: floorY + 1, Z: int32(backWorldZ)})
			if behind == game.Air {
				t.Fatal("false door unexpectedly leads into open space")
			}

			return
		}
	}

	t.Fatal("could not find an unobstructed false door gallery")
}

func TestActualDoorUsesMatchingHalves(t *testing.T) {
	lower, ok := actualDoorBlock(game.OakDoor, int64(floorY+1), "east", "left")
	if !ok {
		t.Fatal("lower door block was not generated")
	}

	upper, ok := actualDoorBlock(game.OakDoor, int64(floorY+2), "east", "left")
	if !ok {
		t.Fatal("upper door block was not generated")
	}

	if !isDoorBlock(lower) || !isDoorBlock(upper) {
		t.Fatal("generated door halves are not door states")
	}

	half, found := lower.Property("half")
	if !found || half != "lower" {
		t.Fatalf("lower door half = %q, %v; want lower, true", half, found)
	}

	half, found = upper.Property("half")
	if !found || half != "upper" {
		t.Fatalf("upper door half = %q, %v; want upper, true", half, found)
	}
}

func isDoorBlock(block game.Block) bool {
	definition, ok := block.Definition()
	return ok && strings.HasSuffix(definition.Name, "_door")
}

func TestZoneBoundariesAlwaysHavePassages(t *testing.T) {
	boundarySeeds := []int64{0, 1, -1, 9918273}

	for _, seed := range boundarySeeds {
		for segment := int64(-4); segment <= 4; segment++ {
			for boundary := int64(-4); boundary <= 4; boundary++ {
				orientations := []bool{false, true}

				for _, vertical := range orientations {
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

	spineSeeds := []int64{0, 1, -1, 9918273}

	for _, seed := range spineSeeds {
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

	doorwayLevels := []int64{int64(floorY + 1), int64(floorY + 2)}

	for _, y := range doorwayLevels {
		doorwayBlock := structureBlock(0, 0, y, 0, current, blocks, structureDoorway)
		if doorwayBlock != game.Air {
			t.Fatalf("doorway block at y=%d = %d, want air", y, doorwayBlock)
		}
	}

	lintelBlock := structureBlock(0, 0, int64(currentCeilingY-1), 0, current, blocks, structureDoorway)
	if lintelBlock == game.Air {
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

			partitionLevels := []int32{floorY + 1, floorY + 2}

			for _, y := range partitionLevels {
				partitionBlock := generated.BlockAt(0, game.BlockPosition{X: x, Y: y, Z: z})
				if partitionBlock == game.Air {
					t.Fatalf("cubicle partition at (%d,%d) y=%d is air", x, z, y)
				}
			}

			aboveBlock := generated.BlockAt(0, game.BlockPosition{X: x, Y: zoneCeilingY(current) - 1, Z: z})
			if aboveBlock != game.Air {
				t.Fatalf("cubicle partition at (%d,%d) reaches ceiling: block=%d", x, z, aboveBlock)
			}

			break
		}
	}

	if !found {
		t.Fatal("could not find a generated cubicle partition")
	}
}

func TestInternalDoorWidthsAreNeverOneBlock(t *testing.T) {
	for hash := range uint64(4096) {
		mazeWidth := doorWidthForHash(hash, 2, 3)
		if mazeWidth < 2 {
			t.Fatalf("maze doorway width = %d, want at least 2", mazeWidth)
		}

		roomWidth := doorWidthForHash(hash, 3, 5)
		if roomWidth < 3 {
			t.Fatalf("room doorway width = %d, want at least 3", roomWidth)
		}
	}
}

func TestGenerateSectionMatchesBlockAt(t *testing.T) {
	generated := Generator{}

	sectionSeeds := []int64{0, 17, -29}
	sectionChunks := []game.ChunkPosition{{X: 0, Z: 0}, {X: -3, Z: 5}, {X: 7, Z: -2}}
	sectionMinYs := []int32{-64, 0, 48, 64, 128, 304, 320}

	for _, seed := range sectionSeeds {
		for _, chunk := range sectionChunks {
			for _, sectionMinY := range sectionMinYs {
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
