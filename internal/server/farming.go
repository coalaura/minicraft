package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	farmlandMaximumMoisture = 7
	cropMaximumStandardAge  = 7
	cropMaximumBeetrootAge  = 3
)

func (r *Runtime) randomTickFarmlandLocked(position game.BlockPosition, block game.Block) {
	moisture := blockPropertyInt(block, "moisture")

	hydrated := r.farmlandHasWater(position)

	if hydrated && moisture < farmlandMaximumMoisture {
		replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "moisture", Value: "7"})

		r.queueTickMutationLocked([]game.BlockChange{{Position: position, Replacement: replacement}}, false)

		return
	}

	if hydrated {
		return
	}

	if moisture > 0 {
		replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "moisture", Value: decimalBlockPropertyValue(moisture - 1)})

		r.queueTickMutationLocked([]game.BlockChange{{Position: position, Replacement: replacement}}, false)

		return
	}

	above := position
	above.Y++

	if r.World.BlockAt(above).HasTrait(game.BlockTraitMaintainsFarmland) {
		return
	}

	r.queueTickMutationLocked([]game.BlockChange{{Position: position, Replacement: game.Dirt}}, true)
}

func (r *Runtime) randomTickCropLocked(position game.BlockPosition, block game.Block) {
	if sameBlockType(block, game.Beetroots) && r.randomTickRandom(3) == 0 {
		return
	}

	brightness, err := rawBrightnessAt(r.World, position)
	if err != nil || brightness < 9 {
		return
	}

	speed := cropGrowthSpeed(r.World.BlockAt, position, block)
	bound := int(25/speed) + 1

	if r.randomTickRandom(bound) != 0 {
		return
	}

	age := blockPropertyInt(block, "age")
	replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "age", Value: decimalBlockPropertyValue(age + 1)})

	r.queueTickMutationLocked([]game.BlockChange{{Position: position, Replacement: replacement}}, false)
}

func (r *Runtime) farmlandHasWater(position game.BlockPosition) bool {
	for yOffset := int32(0); yOffset <= 1; yOffset++ {
		for zOffset := int32(-4); zOffset <= 4; zOffset++ {
			for xOffset := int32(-4); xOffset <= 4; xOffset++ {
				candidate := game.BlockPosition{
					X: position.X + xOffset,
					Y: position.Y + yOffset,
					Z: position.Z + zOffset,
				}

				if r.World.FluidAt(candidate).Type() == game.FluidTypeWater {
					return true
				}
			}
		}
	}

	return false
}

func (r *Runtime) scheduleFarmlandSurvivalChecksLocked(changes []game.BlockChange) {
	for _, change := range changes {
		if change.Position.Y == math.MinInt32 {
			continue
		}

		farmlandPosition := change.Position
		farmlandPosition.Y--

		farmland := r.World.BlockAt(farmlandPosition)
		if !sameBlockType(farmland, game.Farmland) || farmlandSurvivesBelow(change.Replacement) {
			continue
		}

		r.scheduleBlockTickLocked(farmlandPosition, farmland, 1)
	}
}

func (r *Runtime) tickFarmlandSurvivalLocked(position game.BlockPosition, block game.Block) {
	if position.Y == math.MaxInt32 {
		return
	}

	above := position
	above.Y++

	if farmlandSurvivesBelow(r.World.BlockAt(above)) {
		return
	}

	r.queueTickMutationLocked([]game.BlockChange{{Position: position, Replacement: game.Dirt}}, true)
}

func (r *Runtime) queueTickMutationLocked(changes []game.BlockChange, structural bool) {
	requiredChanges := len(changes)

	if structural {
		changes = r.withStructuralNeighborChanges(changes)
	}

	result, delivery, err := r.mutateBlocksLocked(nil, BlockMutationPlace, changes, requiredChanges, true, false, true, false)
	if err != nil || !result.Changed {
		return
	}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
}

func cropGrowthSpeed(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, crop game.Block) float32 {
	speed := float32(1)
	belowY := position.Y - 1

	for zOffset := int32(-1); zOffset <= 1; zOffset++ {
		for xOffset := int32(-1); xOffset <= 1; xOffset++ {
			below := game.BlockPosition{X: position.X + xOffset, Y: belowY, Z: position.Z + zOffset}

			farmland := blockAt(below)
			if !sameBlockType(farmland, game.Farmland) {
				continue
			}

			contribution := float32(1)

			if blockPropertyInt(farmland, "moisture") > 0 {
				contribution = 3
			}

			if xOffset != 0 || zOffset != 0 {
				contribution /= 4
			}

			speed += contribution
		}
	}

	westEast := sameCrop(blockAt, crop, position.X-1, position.Y, position.Z) || sameCrop(blockAt, crop, position.X+1, position.Y, position.Z)
	northSouth := sameCrop(blockAt, crop, position.X, position.Y, position.Z-1) || sameCrop(blockAt, crop, position.X, position.Y, position.Z+1)

	if westEast && northSouth {
		return speed / 2
	}

	diagonal := sameCrop(blockAt, crop, position.X-1, position.Y, position.Z-1) ||
		sameCrop(blockAt, crop, position.X+1, position.Y, position.Z-1) ||
		sameCrop(blockAt, crop, position.X-1, position.Y, position.Z+1) ||
		sameCrop(blockAt, crop, position.X+1, position.Y, position.Z+1)

	if diagonal {
		return speed / 2
	}

	return speed
}

func farmlandSurvivesBelow(above game.Block) bool {
	if above.Behavior() == game.BlockBehaviorFenceGate {
		return true
	}

	if sameBlockType(above, game.MovingPiston) {
		return true
	}

	boxes := above.CollisionBoxes(game.BlockPosition{})

	if len(boxes) == 0 {
		return true
	}

	bounds := boxes[0]

	for _, box := range boxes[1:] {
		bounds.MinX = min(bounds.MinX, box.MinX)
		bounds.MinY = min(bounds.MinY, box.MinY)
		bounds.MinZ = min(bounds.MinZ, box.MinZ)
		bounds.MaxX = max(bounds.MaxX, box.MaxX)
		bounds.MaxY = max(bounds.MaxY, box.MaxY)
		bounds.MaxZ = max(bounds.MaxZ, box.MaxZ)
	}

	xSize := bounds.MaxX - bounds.MinX
	ySize := bounds.MaxY - bounds.MinY
	zSize := bounds.MaxZ - bounds.MinZ
	averageSize := (xSize + ySize + zSize) / 3

	return averageSize < 0.7291666666666666 && ySize < 1
}

func cropMaximumAge(block game.Block) int {
	switch {
	case sameBlockType(block, game.Wheat), sameBlockType(block, game.Carrots), sameBlockType(block, game.Potatoes):
		return cropMaximumStandardAge
	case sameBlockType(block, game.Beetroots):
		return cropMaximumBeetrootAge
	default:
		return 0
	}
}

func sameCrop(blockAt func(game.BlockPosition) game.Block, crop game.Block, x, y, z int32) bool {
	return sameBlockType(blockAt(game.BlockPosition{X: x, Y: y, Z: z}), crop)
}

func decimalBlockPropertyValue(value int) string {
	return string(rune('0' + value))
}
