package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func (s *Session) handleUseItemOn(interaction protocol.UseItemOn) error {
	if !validPlacementInteraction(interaction) {
		return s.resynchronizePlacement(interaction.Position, interaction.Sequence)
	}

	handled, result, affected, err := s.Runtime.InteractBlock(s, interaction.Position)
	if err != nil {
		return err
	}

	if handled {
		if !result.Allowed || !result.Changed {
			return s.resynchronizeBlocks(affected, interaction.Sequence)
		}

		return s.sendBlockChangedAck(interaction.Sequence)
	}

	target, validTarget := placementTarget(interaction.Position, interaction.Face)
	if !validTarget {
		return s.resynchronizePlacement(interaction.Position, interaction.Sequence)
	}

	stack, validHand := s.heldItem(interaction.Hand)
	if !validHand || stack.Empty() {
		return s.resynchronizePlacement(target, interaction.Sequence)
	}

	result, affected, err = s.Runtime.PlaceItem(s, interaction, stack.Item)
	if err != nil {
		return err
	}

	if !result.Allowed || !result.Changed {
		return s.resynchronizeBlocks(affected, interaction.Sequence)
	}

	return s.sendBlockChangedAck(interaction.Sequence)
}

func (r *Runtime) InteractBlock(session *Session, position game.BlockPosition) (bool, BlockMutationResult, []game.BlockPosition, error) {
	r.worldMutationMu.Lock()

	handled, result, affected, delivery, err := func() (bool, BlockMutationResult, []game.BlockPosition, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		block := r.World.BlockAt(position)

		changes := make([]game.BlockChange, 0, 2)

		affected := []game.BlockPosition{position}

		switch block.Behavior() {
		case game.BlockBehaviorDoor:
			otherPosition := position

			if blockProperty(block, "half") == "upper" {
				otherPosition.Y--
			} else {
				otherPosition.Y++
			}

			other := r.World.BlockAt(otherPosition)
			if other.Behavior() != game.BlockBehaviorDoor || !sameBlockType(block, other) || blockProperty(other, "half") == blockProperty(block, "half") {
				return true, BlockMutationResult{Block: block}, affected, blockMutationDelivery{}, nil
			}

			open := blockProperty(block, "open") != "true"

			changes = append(changes,
				game.BlockChange{Position: position, Replacement: withBlockProperties(block, game.BlockPropertyValue{Name: "open", Value: boolProperty(open)})},
				game.BlockChange{Position: otherPosition, Replacement: withBlockProperties(other, game.BlockPropertyValue{Name: "open", Value: boolProperty(open)})},
			)

			affected = append(affected, otherPosition)
		case game.BlockBehaviorTrapdoor, game.BlockBehaviorFenceGate:
			open := blockProperty(block, "open") != "true"

			changes = append(changes, game.BlockChange{Position: position, Replacement: withBlockProperties(block, game.BlockPropertyValue{Name: "open", Value: boolProperty(open)})})
		default:
			return false, BlockMutationResult{}, nil, blockMutationDelivery{}, nil
		}

		changes = r.withStructuralNeighborChanges(changes)

		result, delivery, err := r.mutateBlocksLocked(session, BlockMutationInteract, changes, len(affected), true)
		return true, result, affected, delivery, err
	}()

	result, err = r.completeBlockMutation(result, delivery, err)
	return handled, result, affected, err
}

func (r *Runtime) PlaceItem(session *Session, interaction protocol.UseItemOn, item game.Item) (BlockMutationResult, []game.BlockPosition, error) {
	r.worldMutationMu.Lock()

	result, affected, delivery, err := func() (BlockMutationResult, []game.BlockPosition, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		target, validTarget := placementTarget(interaction.Position, interaction.Face)
		if !validTarget {
			return BlockMutationResult{}, []game.BlockPosition{interaction.Position}, blockMutationDelivery{}, nil
		}

		base, placeable := item.PlacementBlock()
		rule := item.PlacementRule()

		if !placeable || rule == game.ItemPlacementUnsupported {
			return BlockMutationResult{Block: r.World.BlockAt(target)}, []game.BlockPosition{target}, blockMutationDelivery{}, nil
		}

		if !session.hasLoadedBlock(interaction.Position) || r.World.BlockAt(interaction.Position) == game.Air {
			return BlockMutationResult{Block: r.World.BlockAt(target)}, []game.BlockPosition{target}, blockMutationDelivery{}, nil
		}

		if rule == game.ItemPlacementSlab {
			if replacement, merge := slabMerge(base, r.World.BlockAt(interaction.Position), interaction.Face, interaction.CursorY); merge {
				changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: interaction.Position, Replacement: replacement}})

				result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true)
				return result, []game.BlockPosition{interaction.Position}, delivery, err
			}

			if replacement, merge := slabMerge(base, r.World.BlockAt(target), oppositeBlockFace(interaction.Face), interaction.CursorY); merge {
				changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: target, Replacement: replacement}})

				result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true)
				return result, []game.BlockPosition{target}, delivery, err
			}
		}

		if r.World.BlockAt(target) != game.Air {
			return BlockMutationResult{Block: r.World.BlockAt(target)}, []game.BlockPosition{target}, blockMutationDelivery{}, nil
		}

		player := session.snapshotPlayer()

		state, valid := placementState(base, rule, interaction, player.Rotation.Yaw)
		if !valid {
			return BlockMutationResult{Block: game.Air}, []game.BlockPosition{target}, blockMutationDelivery{}, nil
		}

		changes := []game.BlockChange{{Position: target, Replacement: state}}

		affected := []game.BlockPosition{target}

		if rule == game.ItemPlacementDoor {
			upper := target
			if upper.Y == math.MaxInt32 || target.Y == math.MinInt32 {
				return BlockMutationResult{Block: game.Air}, affected, blockMutationDelivery{}, nil
			}

			upper.Y++

			below := target

			below.Y--

			if r.World.BlockAt(upper) != game.Air || r.World.BlockAt(below).Behavior() != game.BlockBehaviorSolid {
				return BlockMutationResult{Block: game.Air}, append(affected, upper), blockMutationDelivery{}, nil
			}

			hinge := doorHinge(r.World.BlockAt, target, state, interaction)

			lower := withBlockProperties(state,
				game.BlockPropertyValue{Name: "half", Value: "lower"},
				game.BlockPropertyValue{Name: "hinge", Value: hinge},
			)

			upperState := withBlockProperties(state,
				game.BlockPropertyValue{Name: "half", Value: "upper"},
				game.BlockPropertyValue{Name: "hinge", Value: hinge},
			)

			changes = []game.BlockChange{{Position: target, Replacement: lower}, {Position: upper, Replacement: upperState}}

			affected = append(affected, upper)
		}

		requiredChanges := len(changes)

		changes = r.withStructuralNeighborChanges(changes)

		result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, requiredChanges, true)
		return result, affected, delivery, err
	}()

	result, err = r.completeBlockMutation(result, delivery, err)
	return result, affected, err
}

func placementState(base game.Block, rule game.ItemPlacementRule, interaction protocol.UseItemOn, yaw float32) (game.Block, bool) {
	facing := horizontalFacing(yaw)

	switch rule {
	case game.ItemPlacementDefault:
		return base, true
	case game.ItemPlacementAxis:
		axis := "y"

		switch interaction.Face {
		case protocol.BlockFaceWest, protocol.BlockFaceEast:
			axis = "x"
		case protocol.BlockFaceNorth, protocol.BlockFaceSouth:
			axis = "z"
		case protocol.BlockFaceDown, protocol.BlockFaceUp:
		default:
			return 0, false
		}

		return base.WithProperties(game.BlockPropertyValue{Name: "axis", Value: axis})
	case game.ItemPlacementHorizontalFacing:
		return base.WithProperties(game.BlockPropertyValue{Name: "facing", Value: facing.name()})
	case game.ItemPlacementSlab:
		half := placementHalf(interaction.Face, interaction.CursorY)
		return base.WithProperties(game.BlockPropertyValue{Name: "type", Value: half})
	case game.ItemPlacementStairs:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "facing", Value: facing.name()},
			game.BlockPropertyValue{Name: "half", Value: placementHalf(interaction.Face, interaction.CursorY)},
			game.BlockPropertyValue{Name: "shape", Value: "straight"},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
	case game.ItemPlacementDoor:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "facing", Value: facing.name()},
			game.BlockPropertyValue{Name: "open", Value: "false"},
			game.BlockPropertyValue{Name: "powered", Value: "false"},
		)
	case game.ItemPlacementTrapdoor:
		trapdoorFacing := facing.opposite()
		if clickedDirection, ok := blockFaceDirection(interaction.Face); ok {
			trapdoorFacing = clickedDirection
		}

		return base.WithProperties(
			game.BlockPropertyValue{Name: "facing", Value: trapdoorFacing.name()},
			game.BlockPropertyValue{Name: "half", Value: placementHalf(interaction.Face, interaction.CursorY)},
			game.BlockPropertyValue{Name: "open", Value: "false"},
			game.BlockPropertyValue{Name: "powered", Value: "false"},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
	case game.ItemPlacementFenceGate:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "facing", Value: facing.name()},
			game.BlockPropertyValue{Name: "open", Value: "false"},
			game.BlockPropertyValue{Name: "powered", Value: "false"},
		)
	case game.ItemPlacementFence, game.ItemPlacementPane, game.ItemPlacementWall:
		return base, true
	default:
		return 0, false
	}
}

func placementHalf(face int32, cursorY float32) string {
	if face == protocol.BlockFaceDown || face != protocol.BlockFaceUp && cursorY > 0.5 {
		return "top"
	}

	return "bottom"
}

func slabMerge(base, existing game.Block, face int32, cursorY float32) (game.Block, bool) {
	if existing.Behavior() != game.BlockBehaviorSlab || !sameBlockType(base, existing) || blockProperty(existing, "type") == "double" {
		return 0, false
	}

	typeName := blockProperty(existing, "type")

	compatible := typeName == "bottom" && (face == protocol.BlockFaceUp || face != protocol.BlockFaceDown && cursorY > 0.5)
	compatible = compatible || typeName == "top" && (face == protocol.BlockFaceDown || face != protocol.BlockFaceUp && cursorY <= 0.5)

	if !compatible {
		return 0, false
	}

	return existing.WithProperties(
		game.BlockPropertyValue{Name: "type", Value: "double"},
		game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
	)
}

func doorHinge(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, door game.Block, interaction protocol.UseItemOn) string {
	facing, _ := directionFromName(blockProperty(door, "facing"))

	leftPosition, leftValid := facing.left().offset(position)
	if !leftValid {
		leftPosition = position
	}

	rightPosition, rightValid := facing.right().offset(position)
	if !rightValid {
		rightPosition = position
	}

	leftUpper := leftPosition

	leftUpper.Y++

	rightUpper := rightPosition

	rightUpper.Y++

	leftSolids := boolInt(leftValid && blockAt(leftPosition).Behavior() == game.BlockBehaviorSolid) + boolInt(leftValid && blockAt(leftUpper).Behavior() == game.BlockBehaviorSolid)
	rightSolids := boolInt(rightValid && blockAt(rightPosition).Behavior() == game.BlockBehaviorSolid) + boolInt(rightValid && blockAt(rightUpper).Behavior() == game.BlockBehaviorSolid)

	if leftSolids != rightSolids {
		if leftSolids > rightSolids {
			return "right"
		}

		return "left"
	}

	leftDoor := game.Air
	if leftValid {
		leftDoor = blockAt(leftPosition)
	}

	rightDoor := game.Air
	if rightValid {
		rightDoor = blockAt(rightPosition)
	}

	leftDoorPresent := sameBlockType(door, leftDoor) && blockProperty(leftDoor, "half") == "lower"
	rightDoorPresent := sameBlockType(door, rightDoor) && blockProperty(rightDoor, "half") == "lower"

	if leftDoorPresent && !rightDoorPresent {
		return "right"
	}

	if rightDoorPresent && !leftDoorPresent {
		return "left"
	}

	var rightClick bool

	switch facing {
	case directionNorth:
		rightClick = interaction.CursorX > 0.5
	case directionSouth:
		rightClick = interaction.CursorX < 0.5
	case directionWest:
		rightClick = interaction.CursorZ < 0.5
	case directionEast:
		rightClick = interaction.CursorZ > 0.5
	}

	if rightClick {
		return "right"
	}

	return "left"
}

func boolInt(value bool) int {
	if value {
		return 1
	}

	return 0
}

func blockFaceDirection(face int32) (horizontalDirection, bool) {
	switch face {
	case protocol.BlockFaceNorth:
		return directionNorth, true
	case protocol.BlockFaceSouth:
		return directionSouth, true
	case protocol.BlockFaceWest:
		return directionWest, true
	case protocol.BlockFaceEast:
		return directionEast, true
	default:
		return 0, false
	}
}

func oppositeBlockFace(face int32) int32 {
	return [...]int32{protocol.BlockFaceUp, protocol.BlockFaceDown, protocol.BlockFaceSouth, protocol.BlockFaceNorth, protocol.BlockFaceEast, protocol.BlockFaceWest}[face]
}

func (s *Session) resynchronizePlacement(position game.BlockPosition, sequence int32) error {
	return s.resynchronizeBlocks([]game.BlockPosition{position}, sequence)
}

func (s *Session) resynchronizeBlocks(positions []game.BlockPosition, sequence int32) error {
	states := make([]int32, len(positions))

	s.Runtime.worldMutationMu.Lock()
	for index, position := range positions {
		state, err := protocolBlockState(s.Runtime.World.BlockAt(position))
		if err != nil {
			s.Runtime.worldMutationMu.Unlock()

			return err
		}

		states[index] = state
	}

	waitForDelivery := s.Runtime.blockMutationDeliveryTail

	deliveryComplete := make(chan struct{})

	s.Runtime.blockMutationDeliveryTail = deliveryComplete
	s.Runtime.worldMutationMu.Unlock()

	<-waitForDelivery
	defer close(deliveryComplete)

	for index, position := range positions {
		err := s.sendBlockUpdate(position, states[index])
		if err != nil {
			return err
		}
	}

	return s.sendBlockChangedAck(sequence)
}

func validPlacementInteraction(interaction protocol.UseItemOn) bool {
	if interaction.InsideBlock || interaction.WorldBorderHit {
		return false
	}

	if interaction.Hand != protocol.MainHand && interaction.Hand != protocol.OffHand {
		return false
	}

	if interaction.Face < protocol.BlockFaceDown || interaction.Face > protocol.BlockFaceEast {
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
