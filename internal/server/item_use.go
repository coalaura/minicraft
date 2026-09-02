package server

import (
	"math/rand/v2"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	consumeEffectsTickInterval  = 4
	consumeEffectsStartFraction = 0.21875
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
	if handled {
		s.Runtime.stopUsingItem(s)

		_, err = handler(s, stack, interaction.Hand)
		return err
	}

	definition, valid := stack.Item.Definition()
	if !valid || definition.Consumable.Duration == 0 {
		return nil
	}

	return s.Runtime.startUsingConsumable(s, interaction.Hand, stack, definition.Food, definition.Consumable)
}

func (r *Runtime) startUsingConsumable(session *Session, hand int32, stack game.ItemStack, food game.ItemFood, consumable game.ItemConsumable) error {
	r.lifecycleMu.Lock()

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		if player.UsingItem || player.Dead || player.GameMode == game.GameModeSpectator {
			return false
		}

		if player.GameMode != game.GameModeCreative && food.Nutrition > 0 && !food.AlwaysEdible && player.FoodLevel >= game.DefaultPlayerFoodLevel {
			return false
		}

		held, valid := heldItemPointer(player, hand)
		if !valid || !held.Equal(stack) {
			return false
		}

		player.UsingItem = true
		player.UsingOffhand = hand == protocol.OffHand
		player.UseRemainingTicks = consumable.Duration
		player.UseAnimation = consumable.Animation
		player.UseStack = stack.Clone()

		return true
	})

	r.lifecycleMu.Unlock()

	if changed {
		r.sendPlayerMetadataUpdates([]game.Player{player})
	}

	return nil
}

func (r *Runtime) stopUsingItem(session *Session) {
	r.lifecycleMu.Lock()

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		return player.StopUsingItem()
	})

	r.lifecycleMu.Unlock()

	if changed {
		r.sendPlayerMetadataUpdates([]game.Player{player})
	}
}

func (r *Runtime) tickUsingItemLocked(session *Session) ([]playerSurvivalUpdate, bool) {
	var (
		before            game.PlayerInventory
		dropped           game.ItemStack
		sounds            []protocol.Sound
		useEnded          bool
		inventoryChanged  bool
		absorptionChanged bool
		effectChanges     []playerMobEffectChange
		instantEffects    []game.MobEffectInstance
	)

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		if !player.UsingItem {
			return false
		}

		hand := int32(protocol.MainHand)

		if player.UsingOffhand {
			hand = protocol.OffHand
		}

		held, valid := heldItemPointer(player, hand)
		if !valid || held.Empty() || held.Item != player.UseStack.Item {
			player.StopUsingItem()

			useEnded = true

			return true
		}

		player.UseStack = held.Clone()

		definition, valid := held.Item.Definition()
		if !valid || definition.Consumable.Duration == 0 {
			player.StopUsingItem()

			useEnded = true

			return true
		}

		if player.UseRemainingTicks > 1 {
			usedTicks := definition.Consumable.Duration - player.UseRemainingTicks
			waitTicks := uint16(float32(definition.Consumable.Duration) * consumeEffectsStartFraction)

			// Vanilla clients derive consume particles from synchronized using-item state; only sound is server-broadcast.
			if usedTicks > waitTicks && player.UseRemainingTicks%consumeEffectsTickInterval == 0 {
				sounds = append(sounds, r.consumableSound(*player, definition.Consumable, protocol.SoundSourcePlayer))
			}

			player.UseRemainingTicks--

			return true
		}

		sounds = append(sounds, r.consumableSound(*player, definition.Consumable, protocol.SoundSourcePlayer))

		if definition.Food.Nutrition > 0 {
			sounds = append(sounds, r.foodCompletionSounds(*player, definition.Consumable)...)
		}

		before = player.Inventory.Clone()

		inventoryChanged = true

		if definition.Food.Nutrition > 0 {
			player.FoodLevel = min(game.DefaultPlayerFoodLevel, player.FoodLevel+int32(definition.Food.Nutrition))
			player.Saturation = min(float32(player.FoodLevel), player.Saturation+definition.Food.Saturation)
		}

		effectChanges, absorptionChanged = r.applyConsumableMobEffects(player, definition.Consumable.Effects)

		for _, effect := range definition.Consumable.DynamicEffects {
			if effect.Type != game.ItemConsumeEffectPotionContents {
				continue
			}

			contents, exists := held.PotionContents()
			if !exists {
				continue
			}

			for _, instance := range contents.Effects(held.PotionDurationScale()) {
				if instance.Effect == game.MobEffectInstantHealth || instance.Effect == game.MobEffectInstantDamage {
					instantEffects = append(instantEffects, instance)

					continue
				}

				change, effectChanged, absorptionUpdated := addPlayerMobEffect(player, instance)
				absorptionChanged = absorptionChanged || absorptionUpdated

				if effectChanged {
					effectChanges = append(effectChanges, change)
				}
			}
		}

		if player.GameMode != game.GameModeCreative {
			held.Count--

			if definition.Consumable.Remainder != game.ItemAir {
				remainder := game.ItemStack{Item: definition.Consumable.Remainder, Count: 1}

				if held.Empty() {
					*held = remainder
				} else {
					insertPlayerInventoryStack(&player.Inventory, &remainder, player.SelectedHotbarSlot)

					dropped = remainder
				}
			} else if held.Empty() {
				*held = game.ItemStack{}
			}
		}

		player.StopUsingItem()

		useEnded = true

		return true
	})

	if !changed || (!useEnded && len(sounds) == 0) {
		return nil, false
	}

	if !dropped.Empty() {
		r.spawnPlayerDroppedItem(player, dropped, false, true)
	}

	update := playerSurvivalUpdate{player: player, metadataChanged: useEnded || absorptionChanged, sounds: sounds, effectChanges: effectChanges}

	if inventoryChanged {
		update.healthChanged = true
		update.inventoryBefore = &before
	}

	updates := []playerSurvivalUpdate{update}

	for _, instance := range instantEffects {
		if session.snapshotPlayer().Dead {
			break
		}

		amount := instantMobEffectAmount(instance)

		var (
			instantUpdate playerSurvivalUpdate
			applied       bool
		)

		if instance.Effect == game.MobEffectInstantHealth {
			instantUpdate, applied = r.healPlayerLocked(session, amount)
		} else {
			instantUpdate, applied = r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageMagic, Amount: amount})
		}

		if applied {
			updates = append(updates, instantUpdate)
		}
	}

	return updates, true
}

func instantMobEffectAmount(instance game.MobEffectInstance) float32 {
	base := int32(4)

	if instance.Effect == game.MobEffectInstantDamage {
		base = 6
	}

	shift := uint32(instance.Amplifier) & 31
	amount := int32(uint32(base) << shift)

	if instance.Effect == game.MobEffectInstantHealth {
		amount = max(amount, 0)
	}

	return float32(amount)
}

func (r *Runtime) consumableSound(player game.Player, consumable game.ItemConsumable, source int32) protocol.Sound {
	eatVolume := float32(1)

	if r.nextEntityRandom() < 0.5 {
		eatVolume = 0.5
	}

	eatPitch := 1 + (r.nextEntityRandom()-r.nextEntityRandom())*0.2
	drinkPitch := 0.9 + r.nextEntityRandom()*0.1

	volume := eatVolume
	pitch := eatPitch

	if consumable.Animation == game.ItemUseAnimationDrink {
		volume = 0.5
		pitch = drinkPitch
	}

	return playerConsumptionSound(player, consumable.Sound, source, volume, pitch)
}

func (r *Runtime) foodCompletionSounds(player game.Player, consumable game.ItemConsumable) []protocol.Sound {
	consumePitch := 1 + (r.nextEntityRandom()-r.nextEntityRandom())*0.4
	burpPitch := 0.9 + r.nextEntityRandom()*0.1

	return []protocol.Sound{
		playerConsumptionSound(player, consumable.Sound, protocol.SoundSourceNeutral, 1, consumePitch),
		playerConsumptionSound(player, game.SoundEntityPlayerBurp, protocol.SoundSourcePlayer, 0.5, burpPitch),
	}
}

func playerConsumptionSound(player game.Player, event game.SoundEvent, source int32, volume, pitch float32) protocol.Sound {
	return protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: string(event)},
		Source: source,
		X:      player.Position.X,
		Y:      player.Position.Y,
		Z:      player.Position.Z,
		Volume: volume,
		Pitch:  pitch,
		Seed:   rand.Int64(),
	}
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
