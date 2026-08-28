package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type RuntimeBlockEntity interface {
	BlockEntityType() game.BlockEntityType
	BlockPosition() game.BlockPosition
}

type RuntimeBlockEntityTicker interface {
	Tick(*Runtime, *ActiveChunk)
}

type RuntimeBlockEntityInteraction interface {
	InteractBlock(*Runtime, *Session) error
}

func (r *Runtime) commitBlockEntityRemovalEffects(records []blockMutationRecord) {
	for _, record := range records {
		if !record.hadPreviousEntity || record.previousEntity.Type == game.BlockEntityTypeForBlock(record.change.Replacement) {
			continue
		}

		definition, valid := record.previousEntity.Type.Definition()
		if !valid || definition.RemovalBehavior != game.BlockEntityRemovalDropInventory {
			continue
		}

		items, inventory := record.previousEntity.Inventory()
		if inventory {
			r.dropContainerContents(record.change.Position, items)
		}
	}
}

func (r *Runtime) dropContainerContents(position game.BlockPosition, items []game.ItemStack) {
	const (
		itemWidth       = 0.25
		motionDeviation = 0.11485000171139836
	)

	for _, source := range items {
		remaining := source.Clone()

		for !remaining.Empty() {
			count := min(remaining.Count, int32(10+int(r.nextEntityRandom()*21)))
			dropped := remaining.Clone()
			dropped.Count = count
			remaining.Count -= count

			position := game.Position{
				X: float64(position.X) + itemWidth/2 + float64(r.nextEntityRandom())*(1-itemWidth),
				Y: float64(position.Y) + float64(r.nextEntityRandom())*(1-itemWidth),
				Z: float64(position.Z) + itemWidth/2 + float64(r.nextEntityRandom())*(1-itemWidth),
			}

			velocity := game.Velocity{
				X: float64(r.nextEntityRandom()-r.nextEntityRandom()) * motionDeviation,
				Y: 0.2 + float64(r.nextEntityRandom()-r.nextEntityRandom())*motionDeviation,
				Z: float64(r.nextEntityRandom()-r.nextEntityRandom()) * motionDeviation,
			}

			r.SpawnItemEntity(dropped, position, velocity, 10)
		}
	}
}

type runtimeBlockEntityFactory func(game.BlockPosition, game.BlockEntity) RuntimeBlockEntity

var runtimeBlockEntityFactories = map[game.BlockEntityType]runtimeBlockEntityFactory{
	game.BlockEntityTypeChest:        newRuntimeChest,
	game.BlockEntityTypeTrappedChest: newRuntimeChest,
	game.BlockEntityTypeBarrel:       newRuntimeBarrel,
	game.BlockEntityTypeFurnace:      newRuntimeFurnace,
	game.BlockEntityTypeSmoker:       newRuntimeFurnace,
	game.BlockEntityTypeBlastFurnace: newRuntimeFurnace,
}

func (r *Runtime) newActiveChunk(position LoadedChunk) *ActiveChunk {
	chunk := &ActiveChunk{Position: position}

	for id, entity := range r.snapshotEntitiesInChunk(position) {
		chunk.SetEntity(id, entity)
	}

	entities := r.World.SnapshotChunkBlockEntities(game.ChunkPosition{X: position.X, Z: position.Z})

	for local, entity := range entities {
		blockPosition := game.BlockPosition{
			X: position.X*ChunkWidth + local.X,
			Y: local.Y,
			Z: position.Z*ChunkWidth + local.Z,
		}

		runtimeEntity := realizeRuntimeBlockEntity(blockPosition, entity)
		if runtimeEntity != nil {
			chunk.SetBlockEntity(blockPosition, runtimeEntity)
		}
	}

	return chunk
}

func realizeRuntimeBlockEntity(position game.BlockPosition, entity game.BlockEntity) RuntimeBlockEntity {
	factory := runtimeBlockEntityFactories[entity.Type]
	if factory == nil {
		return nil
	}

	return factory(position, entity.Clone())
}

func (r *Runtime) runtimeBlockEntityAt(position game.BlockPosition) (RuntimeBlockEntity, bool) {
	chunkPosition := blockLoadedChunk(position)

	chunk, active := r.ActiveChunk(chunkPosition)
	if !active {
		return nil, false
	}

	return chunk.BlockEntity(position)
}

func (r *Runtime) authoritativeRuntimeBlockEntityAt(position game.BlockPosition, block game.Block) (RuntimeBlockEntity, bool) {
	hostedType := game.BlockEntityTypeForBlock(block)
	if hostedType == game.BlockEntityTypeNone {
		return nil, false
	}

	runtimeEntity, present := r.runtimeBlockEntityAt(position)
	if !present || runtimeEntity.BlockPosition() != position || runtimeEntity.BlockEntityType() != hostedType {
		return nil, false
	}

	authoritativeEntity, present := r.World.BlockEntityAt(position)
	if !present || authoritativeEntity.Type != hostedType {
		return nil, false
	}

	return runtimeEntity, true
}

func (r *Runtime) closeMenu(session *Session, notify bool) {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	r.closeMenuLocked(session, notify)

	deliveries := r.takeRuntimeBlockMutationsLocked()

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

	r.completeRuntimeBlockMutations(deliveries)
}

func (r *Runtime) closeMenuLocked(session *Session, notify bool) {
	r.closeMenuWithRemovalStateLocked(session, notify, false)
}

func (r *Runtime) closeMenuWithRemovalStateLocked(session *Session, notify, disconnected bool) {
	if session.Player == nil {
		return
	}

	current := session.containerMenu
	if current == nil || current == session.inventoryMenu {
		session.returnToInventoryMenu()
		return
	}

	windowID := current.windowID
	carried := current.carried.Clone()
	before := session.snapshotPlayer().Inventory

	var removedDrops []game.ItemStack

	removedInventoryChanged := false

	if current.removed != nil {
		_, removedInventoryChanged = session.updatePlayerState(func(player *game.Player) bool {
			candidate := current.candidate()

			candidate.selected = player.SelectedHotbarSlot

			current.removed(candidate, disconnected)

			changedSlots := candidate.changedSlots()

			removedDrops = cloneItemStacks(candidate.dropped)

			current.commit(candidate)

			return current.exposesPlayerSlots(changedSlots)
		})
	}

	current.carried = game.ItemStack{}

	current.resetDrag()

	if current.backing != nil {
		current.backing.Detach(r, session)
	}

	session.returnToInventoryMenu()

	inventoryChanged := false

	if !carried.Empty() {
		player := session.snapshotPlayer()

		if !disconnected {
			_, inventoryChanged = session.updatePlayerState(func(currentPlayer *game.Player) bool {
				return moveStackIntoPlayerInventory(session.inventoryMenu, &carried)
			})

			player = session.snapshotPlayer()
		}

		if !carried.Empty() {
			r.spawnPlayerDroppedItem(player, carried, false, false)
		}
	}

	player := session.snapshotPlayer()

	for _, stack := range removedDrops {
		r.spawnPlayerDroppedItem(player, stack, false, false)
	}

	if notify {
		err := session.writePacket(protocol.ClientboundCloseContainerID, protocol.CloseContainer{ContainerID: windowID})
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to close container: %v\n", err)
		}
	}

	if !disconnected && (removedInventoryChanged || inventoryChanged) {
		err := session.synchronizePlayerInventoryMutation(before)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize returned carried item: %v\n", err)
		}
	}
}

func moveStackIntoPlayerInventory(playerMenu *menu, stack *game.ItemStack) bool {
	before := stack.Clone()

	candidate := playerMenu.candidate()

	moveIntoSlots(candidate, stack, slotRange(9, 44))

	playerMenu.commit(candidate)

	return !stack.Equal(before)
}

func (r *Runtime) reconcileRuntimeBlockEntities(records []blockMutationRecord) {
	for _, record := range records {
		previousType := game.BlockEntityTypeForBlock(record.previous)
		nextType := game.BlockEntityTypeForBlock(record.change.Replacement)

		if previousType == nextType {
			continue
		}

		chunk, active := r.ActiveChunk(blockLoadedChunk(record.change.Position))
		if !active {
			continue
		}

		if nextType == game.BlockEntityTypeNone {
			chunk.RemoveBlockEntity(record.change.Position)

			continue
		}

		entity, present := r.World.BlockEntityAt(record.change.Position)
		if !present {
			continue
		}

		runtimeEntity := realizeRuntimeBlockEntity(record.change.Position, entity)
		if runtimeEntity != nil {
			chunk.SetBlockEntity(record.change.Position, runtimeEntity)
		}
	}
}

func (r *Runtime) closeRemovedBlockEntityMenus(records []blockMutationRecord) {
	changed := make(map[game.BlockPosition]struct{})

	for _, record := range records {
		if !sameBlockType(record.previous, record.change.Replacement) {
			changed[record.change.Position] = struct{}{}
		}
	}

	if len(changed) == 0 {
		return
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	for _, session := range r.snapshotSessions() {
		current := session.activeMenu()
		if current.backing == nil {
			continue
		}

		for position := range changed {
			if current.backing.ContainsPosition(position) && !current.backing.StillValid(r, session) {
				r.closeMenuLocked(session, true)

				break
			}
		}
	}
}

func (r *Runtime) tickOpenMenus() {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	for _, session := range r.snapshotSessions() {
		current := session.activeMenu()
		if current.backing == nil {
			continue
		}

		if current.backing.StillValid(r, session) {
			err := session.sendChangedMenuData(current, false)
			if err != nil && session.Log != nil {
				session.Log.Warnf("[play] failed to synchronize menu data: %v\n", err)
			}

			continue
		}

		r.closeMenuLocked(session, true)
	}

	deliveries := r.takeRuntimeBlockMutationsLocked()

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

	r.completeRuntimeBlockMutations(deliveries)
}
