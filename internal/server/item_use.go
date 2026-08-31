package server

import (
	"math/rand/v2"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type itemUseHandler func(*Session, game.ItemStack, int32) (bool, error)

var itemUseHandlers = map[game.Item]itemUseHandler{
	game.ItemBucket:      useBucketItem,
	game.ItemWaterBucket: useBucketItem,
	game.ItemLavaBucket:  useBucketItem,
}

func (s *Session) handleUseItem(interaction protocol.UseItem) (err error) {
	defer func() {
		// Use Item is sequenced even when the held stack, hand, or look data is invalid.
		ackErr := s.sendBlockChangedAck(interaction.Sequence)

		if err == nil {
			err = ackErr
		}
	}()

	if !s.playerAlive() {
		return nil
	}

	if !validItemUse(interaction) {
		return nil
	}

	s.Runtime.updatePlayerMovement(s, func(player *game.Player) {
		player.Rotation = game.Rotation{Yaw: interaction.Yaw, Pitch: interaction.Pitch}
	})

	stack, valid := s.heldItem(interaction.Hand)
	if !valid || stack.Empty() {
		return nil
	}

	handler, handled := itemUseHandlers[stack.Item]
	if !handled {
		return nil
	}

	_, err = handler(s, stack, interaction.Hand)
	return err
}

func (r *Runtime) useBucketOn(session *Session, interaction protocol.UseItemOn, stack game.ItemStack) (bool, error) {
	if !worldPositionValid(interaction.Position) || !blockWithinInteractionRange(session.snapshotPlayer(), interaction.Position) {
		return false, nil
	}

	clicked := r.World.BlockAt(interaction.Position)

	if stack.Item == game.ItemBucket {
		if sourceFluid(clicked) {
			return r.fillBucket(session, interaction.Hand, stack, interaction.Position)
		}

		if waterlogged(clicked) {
			return r.fillWaterloggedBucket(session, interaction.Hand, stack, interaction.Position, clicked)
		}

		return false, nil
	}

	fluid, filled := bucketFluid(stack.Item)
	if !filled {
		return false, nil
	}

	if fluid == game.Water && canWaterlog(clicked) {
		return r.waterlogBucket(session, interaction.Hand, stack, interaction.Position, clicked)
	}

	clickedFluid := clicked.FluidState()
	if clickedFluid.IsSource() && clickedFluid.Type() == fluid.FluidState().Type() {
		return r.emptyBucket(session, interaction.Hand, stack, interaction.Position, fluid)
	}

	target := interaction.Position

	if !bucketCanReplace(clicked) {
		var valid bool

		target, valid = placementTarget(interaction.Position, interaction.Face)
		if !valid || !worldPositionValid(target) {
			return false, nil
		}
	}

	return r.emptyBucket(session, interaction.Hand, stack, target, fluid)
}

func (r *Runtime) fillBucket(session *Session, hand int32, stack game.ItemStack, position game.BlockPosition) (bool, error) {
	fluid := r.World.BlockAt(position)
	if waterlogged(fluid) {
		return r.fillWaterloggedBucket(session, hand, stack, position, fluid)
	}

	filledItem, valid := fluidBucket(fluid)

	if !valid || !sourceFluid(fluid) {
		return false, nil
	}

	return r.mutateBucket(session, hand, stack, position, game.Air, filledItem, bucketFillSound(fluid.FluidState()))
}

func (r *Runtime) fillWaterloggedBucket(session *Session, hand int32, stack game.ItemStack, position game.BlockPosition, block game.Block) (bool, error) {
	replacement, valid := block.WithoutContainedFluid()
	if !valid {
		return false, nil
	}

	return r.mutateBucket(session, hand, stack, position, replacement, game.ItemWaterBucket, bucketFillSound(block.FluidState()))
}

func (r *Runtime) waterlogBucket(session *Session, hand int32, stack game.ItemStack, position game.BlockPosition, block game.Block) (bool, error) {
	replacement, valid := block.WithContainedFluid(game.FluidTypeWater)
	if !valid || replacement == block {
		return false, nil
	}

	return r.mutateBucket(session, hand, stack, position, replacement, game.ItemBucket, bucketEmptySound(game.FluidTypeWater))
}

func (r *Runtime) emptyBucket(session *Session, hand int32, stack game.ItemStack, position game.BlockPosition, fluid game.Block) (bool, error) {
	player := session.snapshotPlayer()

	if !worldPositionValid(position) || !blockWithinInteractionRange(player, position) {
		return false, nil
	}

	current := r.World.BlockAt(position)

	currentFluid := current.FluidState()
	placingFluid := fluid.FluidState()

	if currentFluid.IsSource() && currentFluid.Type() == placingFluid.Type() {
		return r.emptyBucketIntoSource(session, hand, stack, position, fluid)
	}

	if !bucketCanReplace(current) {
		return false, nil
	}

	return r.mutateBucket(session, hand, stack, position, fluid, game.ItemBucket, bucketEmptySound(fluid.FluidState().Type()))
}

func (r *Runtime) emptyBucketIntoSource(session *Session, hand int32, stack game.ItemStack, position game.BlockPosition, fluid game.Block) (bool, error) {
	used, err := r.transformBucketHeldStack(session, hand, stack, game.ItemBucket)
	if err != nil || !used {
		return used, err
	}

	sound := bucketSound(position, bucketEmptySound(fluid.FluidState().Type()))

	for _, viewer := range r.snapshotSessions() {
		err := viewer.sendSoundIfLoaded(sound, position)
		if err != nil {
			viewer.Log.Warnf("[play] failed to send bucket sound: %v\n", err)
		}
	}

	return true, nil
}

func (r *Runtime) mutateBucket(session *Session, hand int32, stack game.ItemStack, position game.BlockPosition, replacement game.Block, resultItem game.Item, sound game.SoundEvent) (bool, error) {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	result, delivery, inventoryBefore, inventoryChanged, dropped, player, err := func() (BlockMutationResult, blockMutationDelivery, game.PlayerInventory, bool, game.ItemStack, game.Player, error) {
		defer r.worldMutationMu.Unlock()
		defer r.lifecycleMu.Unlock()

		player := session.snapshotPlayer()
		inventoryBefore := player.Inventory.Clone()

		inventory, dropped, valid := bucketInventoryResult(player, hand, stack, resultItem)
		if !valid {
			return BlockMutationResult{}, blockMutationDelivery{}, inventoryBefore, false, game.ItemStack{}, player, nil
		}

		current := r.World.BlockAt(position)
		if current == replacement {
			return BlockMutationResult{}, blockMutationDelivery{}, inventoryBefore, false, game.ItemStack{}, player, nil
		}

		changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: position, Replacement: replacement}})

		changes = r.withImmediateFluidMixing(changes)

		result, delivery, err := r.mutateBlocksLocked(session, BlockMutationInteract, changes, 1, true, false, false, false, true)

		if !replacement.FluidState().Empty() && current.FluidState().Empty() && current != game.Air && !current.Waterloggable() {
			for index := range delivery.records {
				record := &delivery.records[index]
				if record.change.Position == position && record.previous == current {
					record.lootContext = blockLootNoBreaker
				}
			}
		}

		if result.Changed {
			session.playerMx.Lock()
			session.Player.Inventory = inventory
			player = *session.Player
			session.playerMx.Unlock()

			delivery.runtimeSounds = []positionalBlockSound{{position: position, sound: bucketSound(position, sound)}}

			for _, record := range delivery.records {
				if record.change.Replacement == game.Obsidian || record.change.Replacement == game.Cobblestone || record.change.Replacement == game.Stone || record.change.Replacement == game.Basalt {
					delivery.runtimeEvents = append(delivery.runtimeEvents, protocol.LevelEvent{Event: protocol.LevelEventLavaFizz, Position: record.change.Position})
				}
			}
		}

		return result, delivery, inventoryBefore, result.Changed, dropped, player, err
	}()

	result, err = r.completeBlockMutation(result, delivery, err)
	if err != nil || !result.Changed {
		return false, err
	}

	if !dropped.Empty() {
		r.spawnPlayerDroppedItem(player, dropped, false, true)
	}

	if inventoryChanged {
		err = session.synchronizePlayerInventoryMutation(inventoryBefore)
		if err != nil {
			return true, err
		}
	}

	return true, nil
}

func (r *Runtime) transformBucketHeldStack(session *Session, hand int32, expected game.ItemStack, result game.Item) (bool, error) {
	r.lifecycleMu.Lock()

	player := session.snapshotPlayer()
	before := player.Inventory.Clone()

	inventory, dropped, valid := bucketInventoryResult(player, hand, expected, result)
	if valid {
		player, _ = session.updatePlayerState(func(player *game.Player) bool {
			player.Inventory = inventory

			return true
		})
	}

	r.lifecycleMu.Unlock()

	if !valid {
		return false, nil
	}

	if !dropped.Empty() {
		r.spawnPlayerDroppedItem(player, dropped, false, true)
	}

	err := session.synchronizePlayerInventoryMutation(before)
	if err != nil {
		return false, err
	}

	return true, nil
}

func validItemUse(interaction protocol.UseItem) bool {
	if interaction.Hand != protocol.MainHand && interaction.Hand != protocol.OffHand {
		return false
	}

	return validPlayerRotation(interaction.Yaw, interaction.Pitch)
}

func useBucketItem(session *Session, stack game.ItemStack, hand int32) (bool, error) {
	player := session.snapshotPlayer()

	if stack.Item == game.ItemBucket {
		hit, found := session.Runtime.raycastItemGrid(player, itemUseRange, bucketFillTarget)
		if !found || !sourceFluid(session.Runtime.World.BlockAt(hit.position)) || !blockWithinInteractionRange(player, hit.position) {
			return false, nil
		}

		return session.Runtime.fillBucket(session, hand, stack, hit.position)
	}

	fluid, filled := bucketFluid(stack.Item)
	if !filled {
		return false, nil
	}

	hit, found := session.Runtime.raycastItemGrid(player, itemUseRange, bucketPlacementTarget)
	if !found || !blockWithinInteractionRange(player, hit.position) {
		return false, nil
	}

	target := hit.position

	clicked := session.Runtime.World.BlockAt(hit.position)
	if fluid == game.Water && canWaterlog(clicked) {
		return session.Runtime.waterlogBucket(session, hand, stack, target, clicked)
	}

	if hit.face < protocol.BlockFaceDown || hit.face > protocol.BlockFaceEast {
		return false, nil
	}

	target, found = placementTarget(hit.position, hit.face)
	if !found {
		return false, nil
	}

	return session.Runtime.emptyBucket(session, hand, stack, target, fluid)
}

func bucketInventoryResult(player game.Player, hand int32, expected game.ItemStack, result game.Item) (game.PlayerInventory, game.ItemStack, bool) {
	candidate := player
	candidate.Inventory = player.Inventory.Clone()

	held, valid := heldItemPointer(&candidate, hand)
	if !valid || !held.Equal(expected) {
		return game.PlayerInventory{}, game.ItemStack{}, false
	}

	resultStack := game.ItemStack{Item: result, Count: 1}

	if player.GameMode == game.GameModeCreative {
		if playerInventoryContains(candidate.Inventory, resultStack) {
			return candidate.Inventory, game.ItemStack{}, true
		}

		insertPlayerInventoryStack(&candidate.Inventory, &resultStack, player.SelectedHotbarSlot)

		return candidate.Inventory, resultStack, true
	}

	if held.Count == 1 {
		*held = resultStack

		return candidate.Inventory, game.ItemStack{}, true
	}

	held.Count--

	insertPlayerInventoryStack(&candidate.Inventory, &resultStack, player.SelectedHotbarSlot)

	return candidate.Inventory, resultStack, true
}

func insertPlayerInventoryStack(inventory *game.PlayerInventory, stack *game.ItemStack, selectedHotbarSlot int) {
	slots := make([]int, 0, 36)

	selectedSlot := 36 + selectedHotbarSlot

	if selectedHotbarSlot >= 0 && selectedHotbarSlot < game.HotbarSlotCount {
		slots = append(slots, selectedSlot)
	}

	for slot := 9; slot <= 44; slot++ {
		if slot != selectedSlot {
			slots = append(slots, slot)
		}
	}

	for _, slot := range slots {
		target := inventory.Slot(slot)
		if target == nil || target.Empty() || !target.SameItem(*stack) {
			continue
		}

		definition, valid := target.Item.Definition()
		if !valid {
			continue
		}

		moved := min(definition.StackSize-target.Count, stack.Count)

		target.Count += moved
		stack.Count -= moved
	}

	for _, slot := range slots {
		if stack.Empty() {
			break
		}

		target := inventory.Slot(slot)
		if target == nil || !target.Empty() {
			continue
		}

		definition, valid := stack.Item.Definition()
		if !valid {
			continue
		}

		moved := min(definition.StackSize, stack.Count)
		*target = stack.Clone()
		target.Count = moved
		stack.Count -= moved
	}
}

func playerInventoryContains(inventory game.PlayerInventory, stack game.ItemStack) bool {
	for slot := 9; slot <= 45; slot++ {
		target := inventory.Slot(slot)
		if target != nil && !target.Empty() && target.SameItem(stack) {
			return true
		}
	}

	return false
}

func sourceFluid(block game.Block) bool {
	return block.FluidState().IsSource()
}

func bucketPlacementTarget(block game.Block) bool {
	return len(block.OutlineBoxes(game.BlockPosition{})) != 0
}

func bucketFillTarget(block game.Block) bool {
	return sourceFluid(block) || len(block.OutlineBoxes(game.BlockPosition{})) != 0
}

func bucketFluid(item game.Item) (game.Block, bool) {
	switch item {
	case game.ItemWaterBucket:
		return game.Water, true
	case game.ItemLavaBucket:
		return game.Lava, true
	default:
		return game.Air, false
	}
}

func fluidBucket(block game.Block) (game.Item, bool) {
	switch block.FluidState().Type() {
	case game.FluidTypeWater:
		return game.ItemWaterBucket, true
	case game.FluidTypeLava:
		return game.ItemLavaBucket, true
	default:
		return game.ItemAir, false
	}
}

func waterlogged(block game.Block) bool {
	value, valid := block.Property("waterlogged")
	return block.FluidState().Type() == game.FluidTypeWater && valid && value == "true"
}

func canWaterlog(block game.Block) bool {
	return block.CanContainFluid(game.FluidTypeWater)
}

func bucketCanReplace(block game.Block) bool {
	fluid := block.FluidState()
	if !fluid.Empty() {
		return !fluid.IsSource()
	}

	return block.Replaceable()
}

func bucketFillSound(fluid game.FluidState) game.SoundEvent {
	if fluid.Type() == game.FluidTypeLava {
		return game.SoundEvent("minecraft:item.bucket.fill_lava")
	}

	return game.SoundEvent("minecraft:item.bucket.fill_water")
}

func bucketEmptySound(fluid game.FluidType) game.SoundEvent {
	if fluid == game.FluidTypeLava {
		return game.SoundEvent("minecraft:item.bucket.empty_lava")
	}

	return game.SoundEvent("minecraft:item.bucket.empty_water")
}

func bucketSound(position game.BlockPosition, event game.SoundEvent) protocol.Sound {
	return protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: string(event)},
		Source: protocol.SoundSourceBlock,
		X:      float64(position.X) + 0.5,
		Y:      float64(position.Y) + 0.5,
		Z:      float64(position.Z) + 0.5,
		Volume: 1,
		Pitch:  1,
		Seed:   rand.Int64(),
	}
}
