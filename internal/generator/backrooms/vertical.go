package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
)

func verticalPlanForZone(current zone, upper bool) verticalPlan {
	salt := saltVertical ^ uint64(0x6c84a9f21d37be05)
	if upper {
		salt ^= 0xb8214f7ca6039de1
	} else {
		salt ^= 0x35d7ec109af84263
	}

	hash := mix64(current.hash ^ salt)

	width := int64(30 + (hash>>8)%5)
	depth := int64(25 + (hash>>16)%5)

	availableX := zoneSize - width - 12
	availableZ := zoneSize - depth - 12

	x0 := int64(6)
	z0 := int64(6)

	if availableX > 0 {
		x0 += int64((hash >> 24) % uint64(availableX+1))
	}

	if availableZ > 0 {
		z0 += int64((hash >> 32) % uint64(availableZ+1))
	}

	x1 := x0 + width - 1
	z1 := z0 + depth - 1

	plan := verticalPlan{x0: x0, x1: x1, z0: z0, z1: z1, upper: upper}

	runAlongX := hash&1 == 0
	positive := hash&(1<<1) == 0

	if runAlongX {
		plan.startZ = (z0+z1)/2 - 1

		if positive {
			plan.startX = x0 + 4
			plan.stepX = 1
		} else {
			plan.startX = x1 - 4
			plan.stepX = -1
		}
	} else {
		plan.startX = (x0+x1)/2 - 1

		if positive {
			plan.startZ = z0 + 4
			plan.stepZ = 1
		} else {
			plan.startZ = z1 - 4
			plan.stepZ = -1
		}
	}

	if upper {
		plan.baseX = plan.startX
		plan.baseZ = plan.startZ
	} else {
		plan.baseX = plan.startX + plan.stepX*7
		plan.baseZ = plan.startZ + plan.stepZ*7
	}

	return plan
}

func layerConnectorEnabled(seed int64, zoneX, zoneZ, lowerLayer int64) bool {
	if lowerLayer < lowestLayerIndex || lowerLayer >= highestLayerIndex {
		return false
	}

	groupSalt := saltConnector ^ mix64(uint64(lowerLayer)+0x43eaf2159b70c68d)

	hash := coordinateHash(seed, zoneX, zoneZ, groupSalt)

	// Roughly one connector per fifteen horizontal zones for each floor boundary.
	return hash%1000 < 66
}

func connectorPlanForZone(current zone) verticalPlan {
	plan := verticalPlanForZone(current, true)

	plan.upper = true

	return plan
}

func layerConnectorBlockAt(seed, worldX, worldY, worldZ int64, current zone) (game.Block, bool) {
	for _, lowerLayer := range []int64{current.layer - 1, current.layer} {
		if !layerConnectorEnabled(seed, current.x, current.z, lowerLayer) {
			continue
		}

		lowerZone := zoneAtLayer(seed, worldX, worldZ, lowerLayer)

		spec := grandAtriumForZone(seed, lowerZone)
		if spec.enabled && lowerLayer >= spec.anchorLayer && lowerLayer < spec.anchorLayer+spec.span-1 {
			continue
		}

		plan := connectorPlanForZone(lowerZone)

		if !stairEnvelopeXZAt(plan, lowerZone.localX, lowerZone.localZ) {
			continue
		}

		normalizedY := int64(floorY) + (worldY - int64(layerFloorY(lowerLayer)))

		minimumY := int64(floorY + 1)
		maximumY := int64(upperFloorY + 2)

		if normalizedY < minimumY || normalizedY > maximumY {
			continue
		}

		blocks := blocksForPalette(lowerZone.palette)

		if stairSideWallAt(plan, lowerZone.localX, lowerZone.localZ) {
			if normalizedY <= maximumY-1 {
				return blocks.wall, true
			}

			return game.Air, true
		}

		if step, ok := stairStepAt(plan, lowerZone.localX, lowerZone.localZ); ok {
			stairY := int64(floorY+1) + int64(step)
			if normalizedY == stairY {
				return stairBlock(game.OakStairs, stairFacing(plan)), true
			}
		}

		return game.Air, true
	}

	return game.Air, false
}

func verticalBaseOpenAt(seed int64, current zone) bool {
	// Clear a short approach on both floors around ordinary inter-layer stairs.
	if layerConnectorEnabled(seed, current.x, current.z, current.layer) {
		plan := connectorPlanForZone(current)

		if connectorApproachOpenAt(current, plan, false) {
			return true
		}
	}

	if layerConnectorEnabled(seed, current.x, current.z, current.layer-1) {
		lower := current

		lower.layer--

		plan := connectorPlanForZone(lower)

		if connectorApproachOpenAt(current, plan, true) {
			return true
		}
	}

	return false
}

func connectorApproachOpenAt(current zone, plan verticalPlan, upperEnd bool) bool {
	x := plan.startX
	z := plan.startZ

	if upperEnd {
		x += plan.stepX * 7
		z += plan.stepZ * 7
	}

	return abs64(current.localX-x) <= 3 && abs64(current.localZ-z) <= 3
}

func stairEnvelopeXZAt(plan verticalPlan, x, z int64) bool {
	endX := plan.startX + plan.stepX*7
	endZ := plan.startZ + plan.stepZ*7

	if plan.stepX != 0 {
		minimumX := min(plan.startX, endX)
		maximumX := max(plan.startX, endX)

		return x >= minimumX && x <= maximumX && z >= plan.startZ-1 && z <= plan.startZ+2
	}

	minimumZ := min(plan.startZ, endZ)
	maximumZ := max(plan.startZ, endZ)

	return z >= minimumZ && z <= maximumZ && x >= plan.startX-1 && x <= plan.startX+2
}

func stairSideWallAt(plan verticalPlan, x, z int64) bool {
	if plan.stepX != 0 {
		return z == plan.startZ-1 || z == plan.startZ+2
	}

	return x == plan.startX-1 || x == plan.startX+2
}

func stairStepAt(plan verticalPlan, x, z int64) (int, bool) {
	for step := range 8 {
		stepX := plan.startX + plan.stepX*int64(step)
		stepZ := plan.startZ + plan.stepZ*int64(step)

		if plan.stepX != 0 {
			if x == stepX && (z == stepZ || z == stepZ+1) {
				return step, true
			}
		} else if z == stepZ && (x == stepX || x == stepX+1) {
			return step, true
		}
	}

	return 0, false
}

func stairFacing(plan verticalPlan) string {
	switch {
	case plan.stepX > 0:
		return "east"
	case plan.stepX < 0:
		return "west"
	case plan.stepZ > 0:
		return "south"
	default:
		return "north"
	}
}

func stairBlock(base game.Block, facing string) game.Block {
	block, ok := base.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: facing},
		game.BlockPropertyValue{Name: "half", Value: "bottom"},
	)

	if !ok {
		return base
	}

	return block
}
