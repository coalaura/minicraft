package server

import (
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const hopperTransferCooldown int32 = 8

type runtimeHopper struct {
	position       game.BlockPosition
	entity         game.BlockEntity
	viewers        map[*Session]struct{}
	tickedGameTime int64
}

func newRuntimeHopper(position game.BlockPosition, entity game.BlockEntity) RuntimeBlockEntity {
	return &runtimeHopper{position: position, entity: entity}
}

func (hopper *runtimeHopper) BlockEntityType() game.BlockEntityType {
	return game.BlockEntityTypeHopper
}

func (hopper *runtimeHopper) BlockPosition() game.BlockPosition {
	return hopper.position
}

func (hopper *runtimeHopper) ContainsPosition(position game.BlockPosition) bool {
	return hopper.position == position
}

func (hopper *runtimeHopper) InteractBlock(runtime *Runtime, session *Session) error {
	return runtime.openHopperLocked(session, hopper)
}

func (hopper *runtimeHopper) Attach(_ *Runtime, session *Session) {
	if hopper.viewers == nil {
		hopper.viewers = make(map[*Session]struct{})
	}

	hopper.viewers[session] = struct{}{}
}

func (hopper *runtimeHopper) Detach(_ *Runtime, session *Session) {
	delete(hopper.viewers, session)
}

func (hopper *runtimeHopper) Changed(runtime *Runtime, actor *Session, _ []int) {
	runtime.World.SetBlockEntity(hopper.position, hopper.entity)
	hopper.synchronizeInventory(actor)
}

func (hopper *runtimeHopper) StillValid(runtime *Runtime, session *Session) bool {
	return containerBlockEntityStillValid(runtime, session, hopper)
}

func (hopper *runtimeHopper) Tick(runtime *Runtime, _ *ActiveChunk) {
	block := runtime.World.BlockAt(hopper.position)

	entity, authoritative := runtime.authoritativeRuntimeBlockEntityAt(hopper.position, block)
	if !authoritative || entity != hopper {
		return
	}

	data, valid := hopper.entity.Data.(*game.HopperBlockEntityData)
	if !valid || len(data.Items) != game.HopperSlotCount {
		return
	}

	previousCooldown := data.TransferCooldown

	data.TransferCooldown--

	hopper.tickedGameTime = runtime.World.Time().Age

	inventoryChanged := false

	if data.TransferCooldown <= 0 {
		data.TransferCooldown = 0

		if blockProperty(block, "enabled") == "true" {
			inventoryChanged = hopper.tryMoveItems(runtime, data)
		}
	}

	if inventoryChanged {
		data.TransferCooldown = hopperTransferCooldown
		hopper.synchronizeInventory(nil)
	}

	if inventoryChanged || previousCooldown > 0 {
		runtime.World.SetBlockEntity(hopper.position, hopper.entity)
	}
}

func (hopper *runtimeHopper) tryMoveItems(runtime *Runtime, data *game.HopperBlockEntityData) bool {
	moved := false

	if !inventoryEmpty(data.Items) {
		moved = hopper.ejectItem(runtime, data)
	}

	if !inventoryFull(data.Items) && hopper.suckItem(runtime, data) {
		moved = true
	}

	return moved
}

func (hopper *runtimeHopper) ejectItem(runtime *Runtime, data *game.HopperBlockEntityData) bool {
	facing, valid := containerFaceFromName(blockProperty(runtime.World.BlockAt(hopper.position), "facing"))
	if !valid || facing == containerFaceUp {
		return false
	}

	targetPosition, valid := offsetContainerPosition(hopper.position, facing)
	if !valid {
		return false
	}

	target, present := runtime.automatedContainerAt(targetPosition)
	if !present {
		return false
	}

	targetFace := oppositeContainerFace(facing)

	for slot := range data.Items {
		source := &data.Items[slot]
		if source.Empty() {
			continue
		}

		moving := source.Clone()

		moving.Count = 1

		targetWasEmpty := target.hopper != nil && inventoryEmpty(target.hopper.hopperItems())

		changed := target.insert(&moving, targetFace)
		if len(changed) == 0 {
			continue
		}

		source.Count--

		normalizeStack(source)

		if targetWasEmpty {
			hopper.adjustReceivingHopper(target.hopper)
		}

		target.changed(runtime, changed...)

		return true
	}

	return false
}

func (hopper *runtimeHopper) suckItem(runtime *Runtime, data *game.HopperBlockEntityData) bool {
	sourcePosition, valid := offsetContainerPosition(hopper.position, containerFaceUp)
	if !valid {
		return false
	}

	source, present := runtime.automatedContainerAt(sourcePosition)
	if present {
		for _, slot := range source.availableSlots(containerFaceDown) {
			stack := source.slots[slot]
			if !source.canExtract(slot, *stack, containerFaceDown) {
				continue
			}

			moving := stack.Clone()

			moving.Count = 1

			target := &automatedContainer{backing: hopper, hopper: hopper}

			for targetSlot := range data.Items {
				target.slots = append(target.slots, &data.Items[targetSlot])
				target.backingIndices = append(target.backingIndices, 0)
			}

			changed := target.insert(&moving, containerFaceUp)
			if len(changed) == 0 {
				continue
			}

			stack.Count--

			normalizeStack(stack)

			source.changed(runtime, slot)

			return true
		}

		return false
	}

	if runtime.hopperSuctionBlocked(sourcePosition) {
		return false
	}

	return hopper.suckItemEntities(runtime, data)
}

func (hopper *runtimeHopper) adjustReceivingHopper(target *runtimeHopper) {
	data, valid := target.entity.Data.(*game.HopperBlockEntityData)
	if !valid || data.TransferCooldown > hopperTransferCooldown {
		return
	}

	data.TransferCooldown = hopperTransferCooldown

	if target.tickedGameTime >= hopper.tickedGameTime {
		data.TransferCooldown--
	}
}

func (hopper *runtimeHopper) suckItemEntities(runtime *Runtime, data *game.HopperBlockEntityData) bool {
	box := hopperSuctionBox(hopper.position)

	for _, entity := range runtime.itemEntitiesInBox(box) {
		entity.State.mu.Lock()

		if entity.State.Removed || entity.Stack.Empty() || !box.Intersects(itemEntityBox(entity.State.Position)) {
			entity.State.mu.Unlock()

			continue
		}

		remaining := entity.Stack.Clone()

		target := &automatedContainer{backing: hopper, hopper: hopper}

		for slot := range data.Items {
			target.slots = append(target.slots, &data.Items[slot])
			target.backingIndices = append(target.backingIndices, 0)
		}

		changed := target.insert(&remaining, containerFaceUp)
		if len(changed) == 0 {
			entity.State.mu.Unlock()

			continue
		}

		entity.Stack = remaining
		entity.State.metadataDirty = true

		entityID := entity.State.ID

		empty := entity.Stack.Empty()
		entity.State.mu.Unlock()

		if empty {
			runtime.removeRuntimeEntity(entityID)
		} else {
			runtime.synchronizeDirtyRuntimeEntityMetadata(entity)
		}

		return true
	}

	return false
}

func (hopper *runtimeHopper) hopperItems() []game.ItemStack {
	data, valid := hopper.entity.Data.(*game.HopperBlockEntityData)
	if !valid {
		return nil
	}

	return data.Items
}

func (hopper *runtimeHopper) synchronizeInventory(actor *Session) {
	for viewer := range hopper.viewers {
		if viewer == actor {
			continue
		}

		current := viewer.activeMenu()
		if current.backing != hopper {
			continue
		}

		current.incrementStateID()

		err := viewer.sendMenuSnapshot(current.snapshot())
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to synchronize hopper: %v\n", err)
		}
	}
}

func (r *Runtime) openHopperLocked(session *Session, hopper *runtimeHopper) error {
	data, valid := hopper.entity.Data.(*game.HopperBlockEntityData)
	if !valid || len(data.Items) != game.HopperSlotCount {
		return nil
	}

	r.closeMenuLocked(session, false)

	menu := newHopperMenu(session.allocateWindowID(), data.Items, &session.Player.Inventory)

	menu.backing = hopper
	session.containerMenu = menu

	hopper.Attach(r, session)

	err := session.writePacket(protocol.ClientboundOpenScreenID, protocol.OpenScreen{
		ContainerID: menu.windowID,
		MenuType:    menu.protocolMenuType,
		Title:       game.TranslatableText("container.hopper"),
	})

	if err != nil {
		r.closeMenuLocked(session, false)

		return err
	}

	return session.sendMenuSnapshot(menu.snapshot())
}

func newHopperMenu(windowID int32, items []game.ItemStack, inventory *game.PlayerInventory) *menu {
	slots := make([]menuSlot, 0, game.HopperSlotCount+36)

	for slot := range items {
		slots = append(slots, menuSlot{stack: &items[slot], limit: 64, storage: menuStorageBacking})
	}

	for playerSlot := 9; playerSlot <= 44; playerSlot++ {
		slots = append(slots, menuSlot{
			stack: inventory.Slot(playerSlot), limit: 64, playerSlot: playerSlot,
			hasPlayerSlot: true, storage: menuStoragePlayer,
		})
	}

	current := &menu{
		windowID: windowID, protocolMenuType: protocol.MenuHopper, slots: slots,
		hiddenOffhand: inventory.Slot(45), quickMove: quickMoveGenericContainer,
		containerSlots: game.HopperSlotCount,
	}

	for hotbar := range game.HotbarSlotCount {
		current.hotbarSlots[hotbar] = game.HopperSlotCount + 27 + hotbar
		current.hasHotbarSlots[hotbar] = true
	}

	return current
}

func (r *Runtime) hopperSuctionBlocked(position game.BlockPosition) bool {
	block := r.World.BlockAt(position)

	definition, valid := block.Definition()
	if !valid || definition.Collision != game.BlockCollisionFull {
		return false
	}

	return definition.ID != game.BeeNestID && definition.ID != game.BeehiveID
}

func (r *Runtime) itemEntitiesInBox(box game.AABB) []*runtimeItemEntity {
	minChunk := positionLoadedChunk(game.Position{X: box.MinX, Z: box.MinZ})
	maxChunk := positionLoadedChunk(game.Position{X: box.MaxX, Z: box.MaxZ})

	r.activeChunksMu.RLock()

	activeChunks := make([]LoadedChunk, 0, (maxChunk.X-minChunk.X+1)*(maxChunk.Z-minChunk.Z+1))

	for chunkX := minChunk.X; chunkX <= maxChunk.X; chunkX++ {
		for chunkZ := minChunk.Z; chunkZ <= maxChunk.Z; chunkZ++ {
			chunkPosition := LoadedChunk{X: chunkX, Z: chunkZ}

			_, active := r.activeChunks[chunkPosition]
			if !active {
				continue
			}

			activeChunks = append(activeChunks, chunkPosition)
		}
	}

	r.activeChunksMu.RUnlock()

	r.entityMu.RLock()

	var items []*runtimeItemEntity

	for _, chunkPosition := range activeChunks {
		for _, entity := range r.entitiesByChunk[chunkPosition] {
			item, itemEntity := entity.(*runtimeItemEntity)
			if itemEntity {
				items = append(items, item)
			}
		}
	}

	r.entityMu.RUnlock()

	slices.SortFunc(items, func(first, second *runtimeItemEntity) int {
		return int(first.State.ID - second.State.ID)
	})

	return items
}

func hopperSuctionBox(position game.BlockPosition) game.AABB {
	return game.AABB{
		MinX: float64(position.X),
		MinY: float64(position.Y) + 11.0/16.0,
		MinZ: float64(position.Z),
		MaxX: float64(position.X) + 1,
		MaxY: float64(position.Y) + 2,
		MaxZ: float64(position.Z) + 1,
	}
}

func inventoryEmpty(items []game.ItemStack) bool {
	return !slices.ContainsFunc(items, func(stack game.ItemStack) bool {
		return !stack.Empty()
	})
}

func inventoryFull(items []game.ItemStack) bool {
	for _, stack := range items {
		if stack.Empty() || stack.Count < stackLimit(stack) {
			return false
		}
	}

	return true
}
