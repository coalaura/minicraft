package backrooms

import (
	"strconv"

	"github.com/coalaura/minicraft/internal/game"
)

const atriumBalconyWidth = int64(5)

func grandAtriumForZone(seed int64, current zone) atriumSpec {
	group := floorDiv(current.layer, verticalGroupSize)

	anchorLayer := group * verticalGroupSize

	hashSalt := saltAtrium ^ mix64(uint64(group)+0x8fd2473cb159e60a)

	hash := coordinateHash(seed, current.x, current.z, hashSalt)

	// Around 3% of 64x64 zones per six-floor vertical group.
	if hash%2000 >= 60 {
		return atriumSpec{}
	}

	span := int64(2 + (hash>>12)%3) // two to four floors
	if anchorLayer < lowestLayerIndex || anchorLayer+span-1 > highestLayerIndex {
		return atriumSpec{}
	}

	anchorHashSalt := saltZone

	if anchorLayer != 0 {
		anchorHashSalt ^= mix64(uint64(anchorLayer) + saltLayer)
	}

	anchorHash := coordinateHash(seed, current.x, current.z, anchorHashSalt)
	if featureForHash(mix64(anchorHash^saltFeature)) != featureNone {
		return atriumSpec{}
	}

	width := int64(44 + (hash>>20)%9)
	depth := int64(40 + (hash>>28)%11)

	x0 := int64(5 + (hash>>36)%5)
	z0 := int64(5 + (hash>>44)%5)

	x1 := min(x0+width-1, zoneSize-5)
	z1 := min(z0+depth-1, zoneSize-5)

	return atriumSpec{
		enabled:     true,
		anchorLayer: anchorLayer,
		span:        span,
		x0:          x0,
		x1:          x1,
		z0:          z0,
		z1:          z1,
		hash:        hash,
	}
}

func grandAtriumBlockAt(seed, worldX, worldY, worldZ int64, current zone) (game.Block, bool) {
	spec := grandAtriumForZone(seed, current)
	if !spec.enabled || current.layer < spec.anchorLayer || current.layer >= spec.anchorLayer+spec.span {
		return game.Air, false
	}

	if current.localX < spec.x0 || current.localX > spec.x1 || current.localZ < spec.z0 || current.localZ > spec.z1 {
		return game.Air, false
	}

	anchorZone := zoneAtLayer(seed, worldX, worldZ, spec.anchorLayer)
	blocks := blocksForPalette(anchorZone.palette)

	bottomFloor := int64(layerFloorY(spec.anchorLayer))
	topLayer := spec.anchorLayer + spec.span - 1
	topCeiling := int64(layerFloorY(topLayer) + 5)

	if worldY < bottomFloor-1 || worldY > topCeiling {
		return game.Air, false
	}

	block, ok := atriumStairBlockAt(spec, worldY, current.localX, current.localZ)
	if ok {
		return block, true
	}

	if atriumColumnAt(spec, current.localX, current.localZ) && worldY > bottomFloor && worldY < topCeiling {
		if worldY == bottomFloor+1 || floorMod(worldY-bottomFloor, int64(layerStride)) == 1 {
			return blocks.trim, true
		}

		return blocks.wall, true
	}

	if atriumPerimeterAt(spec, current.localX, current.localZ) {
		if atriumEntranceAt(spec, current, worldY) {
			return game.Air, true
		}

		return blocks.wall, true
	}

	if worldY == bottomFloor-1 {
		return foundationBlock(anchorZone.palette), true
	}

	if worldY == bottomFloor {
		return blocks.floor, true
	}

	for layer := spec.anchorLayer + 1; layer <= topLayer; layer++ {
		levelFloor := int64(layerFloorY(layer))
		if worldY == levelFloor {
			if atriumBalconyAt(spec, current.localX, current.localZ, layer-spec.anchorLayer) {
				if atriumBalconyLightAt(spec, current.localX, current.localZ, layer-spec.anchorLayer) {
					return blocks.light, true
				}

				return blocks.floor, true
			}

			return game.Air, true
		}

		if worldY == levelFloor+1 && atriumRailVisibleAt(spec, current.localX, current.localZ, layer-spec.anchorLayer) {
			return atriumRailBlock(spec, current.localX, current.localZ, layer-spec.anchorLayer), true
		}
	}

	if worldY == topCeiling {
		if atriumCeilingLightAt(spec, current.localX, current.localZ) {
			return blocks.light, true
		}

		return blocks.ceiling, true
	}

	return game.Air, true
}

func atriumPerimeterAt(spec atriumSpec, x, z int64) bool {
	return x == spec.x0 || x == spec.x1 || z == spec.z0 || z == spec.z1
}

func atriumEntranceAt(spec atriumSpec, current zone, worldY int64) bool {
	levelFloor := int64(layerFloorY(current.layer))
	if worldY < levelFloor+1 || worldY > levelFloor+3 {
		return false
	}

	centerX := (spec.x0 + spec.x1) / 2
	centerZ := (spec.z0 + spec.z1) / 2

	return ((current.localZ == spec.z0 || current.localZ == spec.z1) && abs64(current.localX-centerX) <= 2) || ((current.localX == spec.x0 || current.localX == spec.x1) && abs64(current.localZ-centerZ) <= 2)
}

func atriumBalconyAt(spec atriumSpec, x, z, level int64) bool {
	nearEdge := x <= spec.x0+atriumBalconyWidth || x >= spec.x1-atriumBalconyWidth || z <= spec.z0+atriumBalconyWidth || z >= spec.z1-atriumBalconyWidth
	if nearEdge {
		return true
	}

	centerX := (spec.x0 + spec.x1) / 2
	centerZ := (spec.z0 + spec.z1) / 2

	if level%2 == 0 {
		return abs64(z-centerZ) <= 1
	}

	return abs64(x-centerX) <= 1
}

func atriumRailAt(spec atriumSpec, x, z, level int64) bool {
	if !atriumBalconyAt(spec, x, z, level) {
		return false
	}

	innerX0 := spec.x0 + atriumBalconyWidth
	innerX1 := spec.x1 - atriumBalconyWidth
	innerZ0 := spec.z0 + atriumBalconyWidth
	innerZ1 := spec.z1 - atriumBalconyWidth

	centerX := (spec.x0 + spec.x1) / 2
	centerZ := (spec.z0 + spec.z1) / 2

	if level%2 == 0 && abs64(z-centerZ) <= 1 {
		return (z == centerZ-1 || z == centerZ+1) && x > innerX0 && x < innerX1
	}

	if level%2 != 0 && abs64(x-centerX) <= 1 {
		return (x == centerX-1 || x == centerX+1) && z > innerZ0 && z < innerZ1
	}

	return x == innerX0 || x == innerX1 || z == innerZ0 || z == innerZ1
}

func atriumRailBlock(spec atriumSpec, x, z, level int64) game.Block {
	block, ok := game.IronBars.WithProperties(
		game.BlockPropertyValue{Name: "east", Value: strconv.FormatBool(atriumRailVisibleAt(spec, x+1, z, level))},
		game.BlockPropertyValue{Name: "north", Value: strconv.FormatBool(atriumRailVisibleAt(spec, x, z-1, level))},
		game.BlockPropertyValue{Name: "south", Value: strconv.FormatBool(atriumRailVisibleAt(spec, x, z+1, level))},
		game.BlockPropertyValue{Name: "west", Value: strconv.FormatBool(atriumRailVisibleAt(spec, x-1, z, level))},
	)

	if !ok {
		return game.IronBars
	}

	return block
}

func atriumRailVisibleAt(spec atriumSpec, x, z, level int64) bool {
	if !atriumRailAt(spec, x, z, level) {
		return false
	}

	railY := int64(layerFloorY(spec.anchorLayer+level) + 1)
	_, staircase := atriumStairBlockAt(spec, railY, x, z)

	return !staircase
}

func atriumColumnAt(spec atriumSpec, x, z int64) bool {
	positions := [][2]int64{
		{spec.x0 + 7, spec.z0 + 7},
		{spec.x1 - 8, spec.z0 + 7},
		{spec.x0 + 7, spec.z1 - 8},
		{spec.x1 - 8, spec.z1 - 8},
	}

	for _, position := range positions {
		if x >= position[0] && x <= position[0]+1 && z >= position[1] && z <= position[1]+1 {
			return true
		}
	}

	return false
}

func atriumCeilingLightAt(spec atriumSpec, x, z int64) bool {
	if x <= spec.x0+2 || x >= spec.x1-2 || z <= spec.z0+2 || z >= spec.z1-2 {
		return false
	}

	phaseX := int64((spec.hash >> 8) & 3)
	phaseZ := int64((spec.hash >> 12) & 3)

	return floorMod(x+phaseX, 10) >= 4 && floorMod(x+phaseX, 10) <= 6 && floorMod(z+phaseZ, 9) == 4
}

func atriumStairBlockAt(spec atriumSpec, worldY, x, z int64) (game.Block, bool) {
	for gap := int64(0); gap < spec.span-1; gap++ {
		lowerFloor := int64(layerFloorY(spec.anchorLayer + gap))
		upperFloor := int64(layerFloorY(spec.anchorLayer + gap + 1))

		if worldY < lowerFloor+1 || worldY > upperFloor+2 {
			continue
		}

		plan := atriumStairPlan(spec, gap)
		if !stairEnvelopeXZAt(plan, x, z) {
			continue
		}

		step, ok := stairStepAt(plan, x, z)

		if ok && worldY == lowerFloor+1+int64(step) {
			return stairBlock(game.OakStairs, stairFacing(plan)), true
		}

		return game.Air, true
	}

	return game.Air, false
}

func atriumStairPlan(spec atriumSpec, gap int64) verticalPlan {
	plan := verticalPlan{upper: true}

	switch gap % 4 {
	case 0:
		plan.startX = spec.x0 + 7
		plan.startZ = spec.z0 + 4
		plan.stepX = 1
	case 1:
		plan.startX = spec.x1 - 5
		plan.startZ = spec.z0 + 7
		plan.stepZ = 1
	case 2:
		plan.startX = spec.x1 - 7
		plan.startZ = spec.z1 - 5
		plan.stepX = -1
	default:
		plan.startX = spec.x0 + 4
		plan.startZ = spec.z1 - 7
		plan.stepZ = -1
	}

	return plan
}

func atriumBalconyLightAt(spec atriumSpec, x, z, level int64) bool {
	if !atriumBalconyAt(spec, x, z, level) {
		return false
	}

	phase := int64(mix64(spec.hash^uint64(level)) % 9)

	if z == spec.z0+2 || z == spec.z1-2 {
		return floorMod(x+phase, 9) == 0
	}

	if x == spec.x0+2 || x == spec.x1-2 {
		return floorMod(z+phase, 9) == 0
	}

	return false
}
