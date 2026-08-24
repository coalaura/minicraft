package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func (s *Session) handleUseItemOn(interaction protocol.UseItemOn) error {
	target, validTarget := placementTarget(interaction.Position, interaction.Face)
	if !validTarget {
		return s.resynchronizePlacement(interaction.Position, interaction.Sequence)
	}

	if !validPlacementInteraction(interaction) {
		return s.resynchronizePlacement(target, interaction.Sequence)
	}

	stack, validHand := s.heldItem(interaction.Hand)
	if !validHand || stack.Empty() {
		return s.resynchronizePlacement(target, interaction.Sequence)
	}

	block, placeable := placementBlock(stack.Item, interaction.Face)
	if !placeable {
		return s.resynchronizePlacement(target, interaction.Sequence)
	}

	result, err := s.Runtime.PlaceBlock(s, interaction.Position, target, block)
	if err != nil {
		return err
	}

	if !result.Allowed || !result.Changed {
		return s.resynchronizePlacement(target, interaction.Sequence)
	}

	return s.sendBlockChangedAck(interaction.Sequence)
}

func placementBlock(item game.Item, face int32) (game.Block, bool) {
	block, placeable := item.PlacementBlock()
	if !placeable {
		return 0, false
	}

	switch item.PlacementRule() {
	case game.ItemPlacementDefault:
		return block, true
	case game.ItemPlacementAxis:
		definition, ok := block.Definition()
		if !ok {
			return 0, false
		}

		axis := 1

		switch face {
		case protocol.BlockFaceWest, protocol.BlockFaceEast:
			axis = 0
		case protocol.BlockFaceNorth, protocol.BlockFaceSouth:
			axis = 2
		case protocol.BlockFaceDown, protocol.BlockFaceUp:
		default:
			return 0, false
		}

		return definition.StateForProperties(axis)
	default:
		return 0, false
	}
}

func (s *Session) resynchronizePlacement(position game.BlockPosition, sequence int32) error {
	s.Runtime.lifecycleMu.Lock()
	defer s.Runtime.lifecycleMu.Unlock()

	state, err := protocolBlockState(s.Runtime.World.BlockAt(position))
	if err != nil {
		return err
	}

	err = s.sendBlockUpdate(position, state)
	if err != nil {
		return err
	}

	return s.sendBlockChangedAck(sequence)
}

func validPlacementInteraction(interaction protocol.UseItemOn) bool {
	if interaction.InsideBlock || interaction.WorldBorderHit {
		return false
	}

	coordinates := [...]float32{interaction.CursorX, interaction.CursorY, interaction.CursorZ}

	for _, coordinate := range coordinates {
		if math.IsNaN(float64(coordinate)) || math.IsInf(float64(coordinate), 0) || coordinate < 0 || coordinate > 1 {
			return false
		}
	}

	return true
}

func placementTarget(clicked game.BlockPosition, face int32) (game.BlockPosition, bool) {
	target := clicked

	switch face {
	case protocol.BlockFaceDown:
		if target.Y == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		target.Y--
	case protocol.BlockFaceUp:
		if target.Y == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		target.Y++
	case protocol.BlockFaceNorth:
		if target.Z == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		target.Z--
	case protocol.BlockFaceSouth:
		if target.Z == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		target.Z++
	case protocol.BlockFaceWest:
		if target.X == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		target.X--
	case protocol.BlockFaceEast:
		if target.X == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		target.X++
	default:
		return game.BlockPosition{}, false
	}

	return target, true
}
