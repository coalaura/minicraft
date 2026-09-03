package server

import "github.com/coalaura/minicraft/internal/game"

const (
	stoneButtonPressTicks  = 20
	woodenButtonPressTicks = 30
)

func (r *Runtime) tickBlockLocked(position game.BlockPosition, block game.Block) {
	if block.HasTrait(game.BlockTraitLeaves) {
		r.tickLeafLocked(position, block)

		return
	}

	if sameBlockType(block, game.Farmland) {
		r.tickFarmlandSurvivalLocked(position, block)

		return
	}

	if block.Behavior() != game.BlockBehaviorButton || blockProperty(block, "powered") != "true" {
		return
	}

	if buttonProjectileSensitive(block) && r.buttonHasProjectileLocked(position, block) {
		r.scheduleBlockTickLocked(position, block, buttonPressTicks(block))

		return
	}

	replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "powered", Value: "false"})

	result, delivery, err := r.mutateBlocksLocked(nil, BlockMutationPlace, []game.BlockChange{{Position: position, Replacement: replacement}}, 1, true, false, true, false)
	if err != nil || !result.Changed {
		return
	}

	event, valid := blockButtonSound(replacement, false)
	if valid {
		delivery.runtimeSounds = append(delivery.runtimeSounds, positionalSound(position, event, 1, 1))
	}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
}

func (r *Runtime) buttonHasProjectileLocked(game.BlockPosition, game.Block) bool {
	// No currently implemented runtime entity is a button-activating projectile.
	return false
}
func buttonPressTicks(block game.Block) int64 {
	if block.SoundType().Break == game.SoundBlockStoneBreak {
		return stoneButtonPressTicks
	}

	return woodenButtonPressTicks
}

func buttonProjectileSensitive(block game.Block) bool {
	return block.Behavior() == game.BlockBehaviorButton && buttonPressTicks(block) == woodenButtonPressTicks
}
