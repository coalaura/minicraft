package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
)

func zoneAt(seed, worldX, worldZ int64) zone {
	return zoneAtLayer(seed, worldX, worldZ, 0)
}

func zoneAtLayer(seed, worldX, worldZ, layer int64) zone {
	zoneX, localX := zoneCoordinate(worldX)
	zoneZ, localZ := zoneCoordinate(worldZ)

	hashSalt := saltZone

	if layer != 0 {
		hashSalt ^= mix64(uint64(layer) + saltLayer)
	}

	hash := coordinateHash(seed, zoneX, zoneZ, hashSalt)

	paletteX := floorDiv(zoneX, paletteRegionSize)
	paletteZ := floorDiv(zoneZ, paletteRegionSize)

	paletteSalt := saltPalette

	if layer != 0 {
		paletteLayer := floorDiv(layer, paletteLayerSize)
		paletteSalt ^= mix64(uint64(paletteLayer) + 0x6ac1437e92d5fb08)
	}

	paletteHash := coordinateHash(seed, paletteX, paletteZ, paletteSalt)

	feature := featureForHash(mix64(hash ^ saltFeature))

	return zone{
		x:        zoneX,
		z:        zoneZ,
		layer:    layer,
		localX:   localX,
		localZ:   localZ,
		hash:     hash,
		layout:   layoutForHash(hash),
		palette:  paletteForHash(paletteHash),
		feature:  feature,
		vertical: verticalNone,
	}
}

func layerFloorY(layer int64) int32 {
	return floorY + int32(layer)*layerStride
}

func layerAtY(worldY int32) int64 {
	return floorDiv(int64(worldY-floorY+1), int64(layerStride))
}

func interstitialBlock(selected palette) game.Block {
	if selected == paletteMaintenance {
		return game.StoneBricks
	}

	return game.SmoothStone
}

func layoutForHash(hash uint64) layout {
	switch hash % 100 {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
		10, 11, 12, 13, 14, 15, 16, 17,
		18, 19, 20, 21, 22, 23, 24, 25,
		26, 27, 28, 29:
		return layoutClassic
	case 30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
		40, 41, 42, 43, 44:
		return layoutMaze
	case 45, 46, 47, 48, 49, 50, 51, 52, 53, 54,
		55, 56, 57, 58, 59:
		return layoutLongHalls
	case 60, 61, 62, 63, 64, 65, 66, 67, 68, 69:
		return layoutCrossroads
	case 70, 71, 72, 73, 74, 75, 76, 77, 78, 79,
		80, 81:
		return layoutCubicles
	case 82, 83, 84, 85, 86, 87, 88, 89, 90, 91:
		return layoutPillars
	default:
		return layoutSparse
	}
}

func paletteForHash(hash uint64) palette {
	switch hash % 16 {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10:
		return paletteClassic
	case 11, 12, 13:
		return paletteFaded
	case 14:
		return paletteOffice
	default:
		return paletteMaintenance
	}
}

func featureForHash(hash uint64) zoneFeature {
	bucket := hash % 1000

	switch {
	case bucket < 10:
		return featureLibrary
	case bucket < 19:
		return featureArchive
	case bucket < 31:
		return featureReception
	case bucket < 43:
		return featureDarkRoom
	case bucket < 56:
		return featureDoorGallery
	case bucket < 71:
		return featureServiceRoom
	case bucket < 83:
		return featureConference
	case bucket < 93:
		return featureBathroom
	case bucket < 103:
		return featureRenovation
	case bucket < 112:
		return featureWindowRoom
	case bucket < 122:
		return featureStorage
	case bucket < 131:
		return featureClassroom
	case bucket < 139:
		return featureMachineRoom
	default:
		return featureNone
	}
}

func blocksForPalette(selected palette) paletteBlocks {
	switch selected {
	case paletteFaded:
		return paletteBlocks{
			wall:    game.SmoothSandstone,
			trim:    game.Sandstone,
			accent:  game.BrownTerracotta,
			floor:   game.LightGrayWool,
			wear:    game.GrayWool,
			stain:   game.BrownWool,
			ceiling: game.LightGrayTerracotta,
			light:   game.OchreFroglight,
			broken:  game.BrownTerracotta,
		}
	case paletteOffice:
		return paletteBlocks{
			wall:    game.WhiteTerracotta,
			trim:    game.LightGrayTerracotta,
			accent:  game.SmoothQuartz,
			floor:   game.GrayWool,
			wear:    game.LightGrayWool,
			stain:   game.BrownWool,
			ceiling: game.SmoothQuartz,
			light:   game.SeaLantern,
			broken:  game.LightGrayTerracotta,
		}
	case paletteMaintenance:
		return paletteBlocks{
			wall:    game.LightGrayTerracotta,
			trim:    game.StoneBricks,
			accent:  game.SmoothStone,
			floor:   game.GrayWool,
			wear:    game.LightGrayWool,
			stain:   game.BlackWool,
			ceiling: game.SmoothStone,
			light:   game.SeaLantern,
			broken:  game.GrayWool,
		}
	default:
		return paletteBlocks{
			wall:    game.SmoothSandstone,
			trim:    game.CutSandstone,
			accent:  game.Sandstone,
			floor:   game.LightGrayWool,
			wear:    game.GrayWool,
			stain:   game.BrownWool,
			ceiling: game.WhiteTerracotta,
			light:   game.OchreFroglight,
			broken:  game.LightGrayTerracotta,
		}
	}
}

func zoneCeilingY(current zone) int32 {
	switch current.layout {
	case layoutPillars:
		return ceilingY
	case layoutCrossroads, layoutSparse:
		if current.hash&(1<<52) != 0 {
			return ceilingY
		}
	}

	return normalCeilingY
}

func foundationBlock(selected palette) game.Block {
	if selected == paletteMaintenance {
		return game.StoneBricks
	}

	return game.SmoothStone
}

func floorBlock(seed, worldX, worldZ int64, current zone, blocks paletteBlocks) game.Block {
	block, ok := featureFloorBlock(current, blocks)
	if ok {
		return block
	}

	patchX := floorDiv(worldX, 8)
	patchZ := floorDiv(worldZ, 8)

	patchHash := coordinateHash(seed, patchX, patchZ, saltFloor^uint64(current.palette))

	if patchHash%17 == 0 {
		localX := floorMod(worldX, 8)
		localZ := floorMod(worldZ, 8)

		centerX := int64(1 + (patchHash>>8)%6)
		centerZ := int64(1 + (patchHash>>16)%6)

		radius := int64(2 + (patchHash>>24)%2)

		deltaX := localX - centerX
		deltaZ := localZ - centerZ

		edgeHash := coordinateHash(seed, worldX, worldZ, saltFloor^0x62f4e987ac113da5)

		edgeJitter := int64(edgeHash%5) - 2

		if deltaX*deltaX+deltaZ*deltaZ <= radius*radius+edgeJitter {
			return blocks.stain
		}
	}

	wearHash := coordinateHash(seed, floorDiv(worldX, 2), floorDiv(worldZ, 2), saltFloor^0xe4fb9c31a7695d27)

	if wearHash%41 == 0 {
		return blocks.wear
	}

	return blocks.floor
}

func ceilingBlock(seed, worldX, worldZ int64, current zone, blocks paletteBlocks) game.Block {
	if darkRoomAt(current) {
		return blocks.ceiling
	}

	if !lightFixtureAt(current) {
		return blocks.ceiling
	}

	fixtureX := floorDiv(worldX, 4)
	fixtureZ := floorDiv(worldZ, 4)

	hash := coordinateHash(seed, fixtureX, fixtureZ, saltLight^uint64(current.layout))

	if hash%17 == 0 {
		return blocks.broken
	}

	return blocks.light
}

func lightFixtureAt(current zone) bool {
	phaseX := int64((current.hash >> 8) & 7)
	phaseZ := int64((current.hash >> 16) & 7)

	switch current.layout {
	case layoutLongHalls:
		vertical, corridorCenter, _ := longHallParameters(current)

		if vertical {
			return abs64(current.localX-corridorCenter) <= 1 && floorMod(current.localZ+phaseZ, 9) == 4
		}

		return abs64(current.localZ-corridorCenter) <= 1 && floorMod(current.localX+phaseX, 9) == 4
	case layoutCrossroads:
		return floorMod(current.localX+phaseX, 10) >= 4 && floorMod(current.localX+phaseX, 10) <= 6 && floorMod(current.localZ+phaseZ, 8) == 4
	case layoutPillars, layoutSparse:
		return floorMod(current.localX+phaseX, 12) >= 5 && floorMod(current.localX+phaseX, 12) <= 6 && floorMod(current.localZ+phaseZ, 11) == 5
	default:
		return floorMod(current.localX+phaseX, 8) >= 3 && floorMod(current.localX+phaseX, 8) <= 5 && floorMod(current.localZ+phaseZ, 8) == 4
	}
}
