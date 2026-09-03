package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const leafUpdateDelay = 1

var leafNeighborOffsets = [...]game.BlockPosition{
	{Y: -1},
	{Y: 1},
	{Z: -1},
	{Z: 1},
	{X: -1},
	{X: 1},
}

func (r *Runtime) scheduleLeafNeighborsLocked(changes []game.BlockChange) {
	for _, change := range changes {
		for _, offset := range leafNeighborOffsets {
			neighborPosition, valid := offsetBlockPosition(change.Position, offset)
			if !valid {
				continue
			}

			neighbor := r.World.BlockAt(neighborPosition)
			if !neighbor.HasTrait(game.BlockTraitLeaves) {
				continue
			}

			neighborDistance := leafDistanceFromBlock(change.Replacement) + 1
			if neighborDistance == 1 && blockPropertyInt(neighbor, "distance") == 1 {
				continue
			}

			r.scheduleBlockTickLocked(neighborPosition, neighbor, leafUpdateDelay)
		}
	}
}

func (r *Runtime) tickLeafLocked(position game.BlockPosition, block game.Block) {
	distance := calculateLeafDistance(r.World.BlockAt, position)
	if distance == blockPropertyInt(block, "distance") {
		return
	}

	replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "distance", Value: decimalBlockPropertyValue(distance)})

	r.queueTickMutationLocked([]game.BlockChange{{Position: position, Replacement: replacement}}, false)
}

func (r *Runtime) randomTickLeafLocked(position game.BlockPosition, block game.Block) {
	if blockPropertyInt(block, "distance") != 7 || blockProperty(block, "persistent") != "false" {
		return
	}

	result, delivery, err := r.mutateBlocksLocked(nil, BlockMutationPlace, []game.BlockChange{{Position: position, Replacement: game.Air}}, 1, true, false, true, false)
	if err != nil || !result.Changed {
		return
	}

	for index := range delivery.records {
		delivery.records[index].lootContext = blockLootNoBreaker
	}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
}

func calculateLeafDistance(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition) int {
	distance := 7

	for _, offset := range leafNeighborOffsets {
		neighborPosition, valid := offsetBlockPosition(position, offset)
		if !valid {
			continue
		}

		neighborDistance := leafDistanceFromBlock(blockAt(neighborPosition)) + 1
		distance = min(distance, neighborDistance)

		if distance == 1 {
			return distance
		}
	}

	return distance
}

func leafDistanceFromBlock(block game.Block) int {
	if block.HasTrait(game.BlockTraitLogs) {
		return 0
	}

	distance, present := block.Property("distance")
	if !present || len(distance) != 1 || distance[0] < '1' || distance[0] > '7' {
		return 7
	}

	return int(distance[0] - '0')
}

func offsetBlockPosition(position, offset game.BlockPosition) (game.BlockPosition, bool) {
	if offset.X < 0 && position.X == math.MinInt32 || offset.X > 0 && position.X == math.MaxInt32 {
		return game.BlockPosition{}, false
	}

	if offset.Y < 0 && position.Y == math.MinInt32 || offset.Y > 0 && position.Y == math.MaxInt32 {
		return game.BlockPosition{}, false
	}

	if offset.Z < 0 && position.Z == math.MinInt32 || offset.Z > 0 && position.Z == math.MaxInt32 {
		return game.BlockPosition{}, false
	}

	position.X += offset.X
	position.Y += offset.Y
	position.Z += offset.Z

	return position, true
}
