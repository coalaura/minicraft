package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func (r *Runtime) UseHeldItemOnBlock(session *Session, interaction protocol.UseItemOn, expected game.Item) (bool, BlockMutationResult, []game.BlockPosition, error) {
	if expected.OnBlockBehavior() != game.ItemOnBlockBehaviorHoe {
		return false, BlockMutationResult{}, nil, nil
	}

	return r.useHoeOnBlock(session, interaction, expected)
}

func (r *Runtime) useHoeOnBlock(session *Session, interaction protocol.UseItemOn, expected game.Item) (bool, BlockMutationResult, []game.BlockPosition, error) {
	r.worldMutationMu.Lock()

	dropRoots := false

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		player := session.snapshotPlayer()

		held, valid := heldItemFromPlayer(player, interaction.Hand)
		if !valid || held.Empty() || held.Item != expected {
			return BlockMutationResult{}, blockMutationDelivery{}, nil
		}

		block := r.World.BlockAt(interaction.Position)

		replacement, tillable, roots := hoeTillingReplacement(r.World.BlockAt, interaction, block)
		if !tillable {
			return BlockMutationResult{Block: block}, blockMutationDelivery{}, nil
		}

		changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: interaction.Position, Replacement: replacement}})

		result, delivery, err := r.mutateBlocksLocked(session, BlockMutationInteract, changes, 1, true, false, false, true)
		if err != nil || !result.Changed {
			return result, delivery, err
		}

		sound := positionalSound(interaction.Position, game.SoundItemHoeTill, 1, 1)

		sound.exclude = session

		delivery.runtimeSounds = append(delivery.runtimeSounds, sound)

		if player.GameMode == game.GameModeSurvival {
			inventoryBefore, toolBroke := r.damageHeldItem(session, interaction.Hand, held, 1)
			delivery.miningInventory = inventoryBefore
			delivery.miningToolBroke = toolBroke
		}

		dropRoots = roots

		return result, delivery, nil
	}()

	result, err = r.completeBlockMutation(result, delivery, err)
	if err == nil && result.Changed && dropRoots {
		r.popResourceFromFace(interaction.Position, interaction.Face, game.ItemStack{Item: game.ItemHangingRoots, Count: 1})
	}

	return true, result, []game.BlockPosition{interaction.Position}, err
}

func (r *Runtime) popResourceFromFace(position game.BlockPosition, face int32, stack game.ItemStack) {
	spawn := game.Position{X: float64(position.X) + 0.5, Y: float64(position.Y) + 0.5, Z: float64(position.Z) + 0.5}
	velocity := game.Velocity{}

	switch face {
	case protocol.BlockFaceDown:
		spawn.Y -= 0.7
		velocity.Y = -0.05
	case protocol.BlockFaceUp:
		spawn.Y += 0.7
		velocity.Y = 0.05
	case protocol.BlockFaceNorth:
		spawn.Z -= 0.7
		velocity.Z = -0.05
	case protocol.BlockFaceSouth:
		spawn.Z += 0.7
		velocity.Z = 0.05
	case protocol.BlockFaceWest:
		spawn.X -= 0.7
		velocity.X = -0.05
	case protocol.BlockFaceEast:
		spawn.X += 0.7
		velocity.X = 0.05
	}

	r.SpawnItemEntity(stack, spawn, velocity, 10)
}

func hoeTillingReplacement(blockAt func(game.BlockPosition) game.Block, interaction protocol.UseItemOn, block game.Block) (game.Block, bool, bool) {
	if sameBlockType(block, game.RootedDirt) {
		return game.Dirt, true, true
	}

	replacement := game.Air

	switch {
	case sameBlockType(block, game.GrassBlock), sameBlockType(block, game.DirtPath), sameBlockType(block, game.Dirt):
		replacement = game.Farmland
	case sameBlockType(block, game.CoarseDirt):
		replacement = game.Dirt
	default:
		return game.Air, false, false
	}

	if interaction.Face == protocol.BlockFaceDown || interaction.Position.Y == math.MaxInt32 {
		return game.Air, false, false
	}

	above := interaction.Position
	above.Y++

	return replacement, blockAt(above) == game.Air, false
}
