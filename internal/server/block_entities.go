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

type runtimeBlockEntityFactory func(game.BlockPosition, game.BlockEntity) RuntimeBlockEntity

var runtimeBlockEntityFactories = map[game.BlockEntityType]runtimeBlockEntityFactory{
	game.BlockEntityTypeBarrel: newRuntimeBarrel,
}

func (r *Runtime) newActiveChunk(position LoadedChunk) *ActiveChunk {
	chunk := &ActiveChunk{Position: position}

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
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.closeMenuLocked(session, notify)
}

func (r *Runtime) closeMenuLocked(session *Session, notify bool) {
	if session.Player == nil {
		return
	}

	current := session.containerMenu
	if current == nil || current == session.inventoryMenu {
		session.returnToInventoryMenu()
		return
	}

	windowID := current.windowID

	r.preserveCarriedLocked(session, current.carried)

	current.carried = game.ItemStack{}

	current.resetDrag()

	if current.backing != nil {
		current.backing.Detach(r, session)
	}

	session.returnToInventoryMenu()

	if notify {
		err := session.writePacket(protocol.ClientboundCloseContainerID, protocol.CloseContainer{ContainerID: windowID})
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to close container: %v\n", err)
		}
	}
}

func (r *Runtime) preserveCarriedLocked(session *Session, carried game.ItemStack) {
	if carried.Empty() {
		return
	}

	remaining := carried.Clone()

	session.updatePlayerState(func(player *game.Player) bool {
		if session.inventoryMenu == nil {
			session.inventoryMenu = newPlayerInventoryMenu(&player.Inventory)
		}

		return moveStackIntoPlayerInventory(session.inventoryMenu, &remaining)
	})

	if remaining.Empty() {
		return
	}

	if session.inventoryMenu.carried.Empty() {
		session.inventoryMenu.carried = remaining
		return
	}

	session.preservedCarried = append(session.preservedCarried, remaining)
}

func (s *Session) drainPreservedCarriedLocked(player *game.Player) bool {
	if len(s.preservedCarried) == 0 {
		return false
	}

	if s.inventoryMenu == nil {
		s.inventoryMenu = newPlayerInventoryMenu(&player.Inventory)
	}

	changed := false
	remaining := s.preservedCarried[:0]

	for _, preserved := range s.preservedCarried {
		stack := preserved.Clone()
		if moveStackIntoPlayerInventory(s.inventoryMenu, &stack) {
			changed = true
		}

		if !stack.Empty() {
			remaining = append(remaining, stack)
		}
	}

	s.preservedCarried = remaining
	return changed
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
	removed := make(map[game.BlockPosition]struct{})

	for _, record := range records {
		previousType := game.BlockEntityTypeForBlock(record.previous)
		nextType := game.BlockEntityTypeForBlock(record.change.Replacement)

		if previousType != game.BlockEntityTypeNone && previousType != nextType {
			removed[record.change.Position] = struct{}{}
		}
	}

	if len(removed) == 0 {
		return
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	for _, session := range r.snapshotSessions() {
		current := session.activeMenu()
		if current.backing == nil {
			continue
		}

		if _, wasRemoved := removed[current.backing.Position()]; wasRemoved {
			r.closeMenuLocked(session, true)
		}
	}
}

func (r *Runtime) tickOpenMenus() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	for _, session := range r.snapshotSessions() {
		current := session.activeMenu()
		if current.backing == nil || current.backing.StillValid(r, session) {
			continue
		}

		r.closeMenuLocked(session, true)
	}
}
