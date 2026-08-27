package server

import (
	"math"
	"strconv"

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

	var runtimeMutations []queuedBlockMutation

	handled, result, affected, delivery, err := func() (bool, BlockMutationResult, []game.BlockPosition, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		block := r.World.BlockAt(position)

		blockDefinition, _ := block.Definition()

		player := session.snapshotPlayer()

		if secondaryUseActive(player) {
			return false, BlockMutationResult{}, nil, blockMutationDelivery{}, nil
		}

		runtimeEntity, active := r.authoritativeRuntimeBlockEntityAt(position, block)
		interaction, interactive := runtimeEntity.(RuntimeBlockEntityInteraction)

		if interactive && active {
			if !session.hasLoadedBlock(position) || !blockWithinInteractionRange(player, position) {
				return true, BlockMutationResult{Block: block}, []game.BlockPosition{position}, blockMutationDelivery{}, nil
			}

			r.lifecycleMu.Lock()
			err := interaction.InteractBlock(r, session)
			r.lifecycleMu.Unlock()

			runtimeMutations = r.takeRuntimeBlockMutationsLocked()

			return true, BlockMutationResult{Block: block, Allowed: true, Changed: true}, []game.BlockPosition{position}, blockMutationDelivery{}, err
		}

		changes := make([]game.BlockChange, 0, 2)

		affected := []game.BlockPosition{position}

		switch block.Behavior() {
		case game.BlockBehaviorDoor:
			if blockDefinition.Name == "iron_door" {
				return false, BlockMutationResult{}, nil, blockMutationDelivery{}, nil
			}

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
			if blockDefinition.Name == "iron_trapdoor" {
				return false, BlockMutationResult{}, nil, blockMutationDelivery{}, nil
			}

			open := blockProperty(block, "open") != "true"

			changes = append(changes, game.BlockChange{Position: position, Replacement: withBlockProperties(block, game.BlockPropertyValue{Name: "open", Value: boolProperty(open)})})
		case game.BlockBehaviorButton:
			if blockProperty(block, "powered") == "true" {
				return true, BlockMutationResult{Block: block, Allowed: true}, affected, blockMutationDelivery{}, nil
			}

			changes = append(changes, game.BlockChange{Position: position, Replacement: withBlockProperties(block, game.BlockPropertyValue{Name: "powered", Value: "true"})})
		default:
			return false, BlockMutationResult{}, nil, blockMutationDelivery{}, nil
		}

		changes = r.withStructuralNeighborChanges(changes)

		result, delivery, err := r.mutateBlocksLocked(session, BlockMutationInteract, changes, len(affected), true, false, false, true)
		return true, result, affected, delivery, err
	}()

	if delivery.waitForDelivery != nil {
		result, err = r.completeBlockMutation(result, delivery, err)
	}

	r.completeRuntimeBlockMutations(runtimeMutations)

	return handled, result, affected, err
}

func secondaryUseActive(player game.Player) bool {
	if !player.Sneaking {
		return false
	}

	selected := player.SelectedHotbarSlot

	mainHandHasItem := selected >= 0 && selected < len(player.Inventory.Hotbar) && !player.Inventory.Hotbar[selected].Empty()
	return mainHandHasItem || !player.Inventory.Offhand.Empty()
}

func (r *Runtime) PlaceItem(session *Session, interaction protocol.UseItemOn, item game.Item) (BlockMutationResult, []game.BlockPosition, error) {
	r.worldMutationMu.Lock()

	result, affected, delivery, err := func() (BlockMutationResult, []game.BlockPosition, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		adjacent, validTarget := placementTarget(interaction.Position, interaction.Face)
		if !validTarget {
			return BlockMutationResult{}, []game.BlockPosition{interaction.Position}, blockMutationDelivery{}, nil
		}

		base, placeable := item.PlacementBlock()
		rule := item.PlacementRule()

		if !placeable || rule == game.ItemPlacementUnsupported {
			return BlockMutationResult{Block: r.World.BlockAt(adjacent)}, []game.BlockPosition{adjacent}, blockMutationDelivery{}, nil
		}

		clicked := r.World.BlockAt(interaction.Position)
		if !session.hasLoadedBlock(interaction.Position) || clicked == game.Air {
			return BlockMutationResult{Block: r.World.BlockAt(adjacent)}, []game.BlockPosition{adjacent}, blockMutationDelivery{}, nil
		}

		if rule == game.ItemPlacementSlab {
			if replacement, merge := slabMerge(base, clicked, interaction.Face, interaction.CursorY); merge {
				changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: interaction.Position, Replacement: replacement}})

				result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true, true, false, true)
				return result, []game.BlockPosition{interaction.Position}, delivery, err
			}

			if replacement, merge := slabMerge(base, r.World.BlockAt(adjacent), oppositeBlockFace(interaction.Face), interaction.CursorY); merge {
				changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: adjacent, Replacement: replacement}})

				result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true, true, false, true)
				return result, []game.BlockPosition{adjacent}, delivery, err
			}
		}

		if replacement, convert := candleCakePlacement(base, rule, clicked, interaction.Face); convert {
			changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: interaction.Position, Replacement: replacement}})

			result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true, true, false, true)
			return result, []game.BlockPosition{interaction.Position}, delivery, err
		}

		canStack := rule != game.ItemPlacementCandle || !secondaryUseActive(session.snapshotPlayer())
		if replacement, stack := stackedPlacement(base, rule, clicked, interaction.Face); canStack && stack {
			changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: interaction.Position, Replacement: replacement}})

			result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true, true, false, true)
			return result, []game.BlockPosition{interaction.Position}, delivery, err
		}

		target := adjacent
		if blockReplaceableBy(clicked, base) {
			target = interaction.Position
		}

		if target != interaction.Position && r.World.BlockAt(target) != game.Air {
			return BlockMutationResult{Block: r.World.BlockAt(target)}, []game.BlockPosition{target}, blockMutationDelivery{}, nil
		}

		player := session.snapshotPlayer()

		state, valid := placementStateWithRotation(base, rule, interaction, player.Rotation)
		if rule == game.ItemPlacementChest {
			state, valid = chestPlacementState(r.World.BlockAt, target, base, interaction, player)
		}

		if !valid {
			return BlockMutationResult{Block: game.Air}, []game.BlockPosition{target}, blockMutationDelivery{}, nil
		}

		if !validPlacementSupport(r.World.BlockAt, target, state, rule) {
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

		result, delivery, err := r.mutateBlocksLocked(session, BlockMutationPlace, changes, requiredChanges, true, true, false, true)
		return result, affected, delivery, err
	}()

	result, err = r.completeBlockMutation(result, delivery, err)
	return result, affected, err
}

func blockReplaceableBy(existing, replacement game.Block) bool {
	if !existing.Replaceable() || sameBlockType(existing, replacement) {
		return false
	}

	definition, _ := existing.Definition()
	if definition.Name == "snow" {
		return blockPropertyInt(existing, "layers") == 1
	}

	return true
}

func placementState(base game.Block, rule game.ItemPlacementRule, interaction protocol.UseItemOn, yaw float32) (game.Block, bool) {
	return placementStateWithRotation(base, rule, interaction, game.Rotation{Yaw: yaw})
}

func placementStateWithRotation(base game.Block, rule game.ItemPlacementRule, interaction protocol.UseItemOn, rotation game.Rotation) (game.Block, bool) {
	facing := horizontalFacing(rotation.Yaw)

	switch rule {
	case game.ItemPlacementDefault:
		return base, true
	case game.ItemPlacementAxis:
		axis, valid := placementAxis(interaction.Face)
		if !valid {
			return 0, false
		}

		return base.WithProperties(game.BlockPropertyValue{Name: "axis", Value: axis})
	case game.ItemPlacementHorizontalFacing:
		return base.WithProperties(game.BlockPropertyValue{Name: "facing", Value: facing.name()})
	case game.ItemPlacementDirectionalFacing:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "facing", Value: directionalPlacementFacing(rotation)},
			game.BlockPropertyValue{Name: "open", Value: "false"},
		)
	case game.ItemPlacementChest:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "facing", Value: facing.opposite().name()},
			game.BlockPropertyValue{Name: "type", Value: "single"},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
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
	case game.ItemPlacementLeaves:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "distance", Value: "7"},
			game.BlockPropertyValue{Name: "persistent", Value: "true"},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
	case game.ItemPlacementChain:
		axis, valid := placementAxis(interaction.Face)
		if !valid {
			return 0, false
		}

		return base.WithProperties(
			game.BlockPropertyValue{Name: "axis", Value: axis},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
	case game.ItemPlacementButton:
		face := "wall"
		buttonFacing := facing

		switch interaction.Face {
		case protocol.BlockFaceUp:
			face = "floor"
		case protocol.BlockFaceDown:
			face = "ceiling"
		default:
			clickedDirection, valid := blockFaceDirection(interaction.Face)
			if !valid {
				return 0, false
			}

			buttonFacing = clickedDirection
		}

		return base.WithProperties(
			game.BlockPropertyValue{Name: "face", Value: face},
			game.BlockPropertyValue{Name: "facing", Value: buttonFacing.name()},
			game.BlockPropertyValue{Name: "powered", Value: "false"},
		)
	case game.ItemPlacementPressurePlate:
		return base.WithProperties(game.BlockPropertyValue{Name: "powered", Value: "false"})
	case game.ItemPlacementWeightedPressurePlate:
		return base.WithProperties(game.BlockPropertyValue{Name: "power", Value: "0"})
	case game.ItemPlacementSnow:
		return base.WithProperties(game.BlockPropertyValue{Name: "layers", Value: "1"})
	case game.ItemPlacementCandle:
		return base.WithProperties(
			game.BlockPropertyValue{Name: "candles", Value: "1"},
			game.BlockPropertyValue{Name: "lit", Value: "false"},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
	case game.ItemPlacementPointedDripstone:
		direction := ""
		if interaction.Face == protocol.BlockFaceUp {
			direction = "up"
		}

		if interaction.Face == protocol.BlockFaceDown {
			direction = "down"
		}

		if direction == "" {
			return 0, false
		}

		return base.WithProperties(
			game.BlockPropertyValue{Name: "thickness", Value: "tip"},
			game.BlockPropertyValue{Name: "vertical_direction", Value: direction},
			game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
		)
	case game.ItemPlacementFence, game.ItemPlacementPane, game.ItemPlacementWall, game.ItemPlacementSupported, game.ItemPlacementPlant:
		return base, true
	default:
		return 0, false
	}
}

func chestPlacementState(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, base game.Block, interaction protocol.UseItemOn, player game.Player) (game.Block, bool) {
	facing := horizontalFacing(player.Rotation.Yaw).opposite()
	chestType := "single"
	secondaryUse := secondaryUseActive(player)
	connectedPartner := game.Air

	if secondaryUse {
		clickedDirection, horizontal := blockFaceDirection(interaction.Face)
		if horizontal {
			partnerDirection := clickedDirection.opposite()
			partner := blockInDirection(blockAt, position, partnerDirection)

			partnerFacing, compatible := chestPlacementPartner(base, partner)
			if compatible && horizontalDirectionAxis(partnerFacing) != horizontalDirectionAxis(clickedDirection) {
				facing = partnerFacing
				connectedPartner = partner

				chestType = "left"
				if facing.left() == partnerDirection {
					chestType = "right"
				}
			}
		}
	} else {
		partner := blockInDirection(blockAt, position, facing.right())

		partnerFacing, compatible := chestPlacementPartner(base, partner)
		if compatible && partnerFacing == facing {
			chestType = "left"
			connectedPartner = partner
		} else {
			partner = blockInDirection(blockAt, position, facing.left())

			partnerFacing, compatible = chestPlacementPartner(base, partner)
			if compatible && partnerFacing == facing {
				chestType = "right"
				connectedPartner = partner
			}
		}
	}

	state, valid := base.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: facing.name()},
		game.BlockPropertyValue{Name: "type", Value: chestType},
		game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
	)
	if !valid || chestType == "single" {
		return state, valid
	}

	normalized, copperPair := normalizedCopperChestBlock(base, connectedPartner)
	if !copperPair {
		return state, true
	}

	return copyChestProperties(normalized, state), true
}

func chestPlacementPartner(base, candidate game.Block) (horizontalDirection, bool) {
	if !chestBlocksCanConnect(base, candidate) || blockProperty(candidate, "type") != "single" {
		return 0, false
	}

	return directionFromName(blockProperty(candidate, "facing"))
}

func horizontalDirectionAxis(direction horizontalDirection) byte {
	if direction == directionWest || direction == directionEast {
		return 'x'
	}

	return 'z'
}

func directionalPlacementFacing(rotation game.Rotation) string {
	yaw := float64(rotation.Yaw) * math.Pi / 180
	pitch := float64(rotation.Pitch) * math.Pi / 180

	lookX := -math.Sin(yaw) * math.Cos(pitch)
	lookY := -math.Sin(pitch)
	lookZ := math.Cos(yaw) * math.Cos(pitch)

	absX := math.Abs(lookX)
	absY := math.Abs(lookY)
	absZ := math.Abs(lookZ)

	if absY >= absX && absY >= absZ {
		if lookY > 0 {
			return "down"
		}

		return "up"
	}

	if absX >= absZ {
		if lookX > 0 {
			return "west"
		}

		return "east"
	}

	if lookZ > 0 {
		return "north"
	}

	return "south"
}

func placementAxis(face int32) (string, bool) {
	switch face {
	case protocol.BlockFaceWest, protocol.BlockFaceEast:
		return "x", true
	case protocol.BlockFaceNorth, protocol.BlockFaceSouth:
		return "z", true
	case protocol.BlockFaceDown, protocol.BlockFaceUp:
		return "y", true
	default:
		return "", false
	}
}

func stackedPlacement(base game.Block, rule game.ItemPlacementRule, existing game.Block, face int32) (game.Block, bool) {
	if !sameBlockType(base, existing) {
		return 0, false
	}

	property := ""
	maximum := 0

	switch rule {
	case game.ItemPlacementSnow:
		if face != protocol.BlockFaceUp {
			return 0, false
		}

		property = "layers"
		maximum = 8
	case game.ItemPlacementCandle:
		property = "candles"
		maximum = 4
	default:
		return 0, false
	}

	count := blockPropertyInt(existing, property)
	if count <= 0 || count >= maximum {
		return 0, false
	}

	replacement, valid := existing.WithProperties(game.BlockPropertyValue{Name: property, Value: strconv.Itoa(count + 1)})
	return replacement, valid
}

func candleCakePlacement(base game.Block, rule game.ItemPlacementRule, existing game.Block, face int32) (game.Block, bool) {
	if rule != game.ItemPlacementCandle || face != protocol.BlockFaceUp {
		return 0, false
	}

	existingDefinition, valid := existing.Definition()
	if !valid || existingDefinition.Name != "cake" || blockProperty(existing, "bites") != "0" {
		return 0, false
	}

	baseDefinition, _ := base.Definition()

	candleCake, valid := game.BlockByName(baseDefinition.Name + "_cake")
	if !valid {
		return 0, false
	}

	return candleCake.WithProperties(game.BlockPropertyValue{Name: "lit", Value: "false"})
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
