package server

import (
	"math"
	"math/rand/v2"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	barrelMenuType              = 2
	barrelBlockEntityRegistryID = 27
	barrelValidityPadding       = 4.0
)

type runtimeBarrel struct {
	position game.BlockPosition
	entity   game.BlockEntity
	viewers  map[*Session]struct{}
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

		switch entity.Type {
		case game.BlockEntityTypeBarrel:
			chunk.SetBlockEntity(blockPosition, &runtimeBarrel{position: blockPosition, entity: entity.Clone()})
		}
	}

	return chunk
}

func (r *Runtime) runtimeBarrelAt(position game.BlockPosition) (*runtimeBarrel, bool) {
	chunkPosition := LoadedChunk{X: chunkCoordinate(float64(position.X)), Z: chunkCoordinate(float64(position.Z))}

	chunk, active := r.ActiveChunk(chunkPosition)
	if !active {
		return nil, false
	}

	entity, present := chunk.BlockEntity(position)
	if !present {
		return nil, false
	}

	barrel, valid := entity.(*runtimeBarrel)
	return barrel, valid
}

func (r *Runtime) openBarrelLocked(session *Session, position game.BlockPosition) error {
	entity, present := r.World.BlockEntityAt(position)
	if !present || entity.Type != game.BlockEntityTypeBarrel {
		return nil
	}

	barrel, active := r.runtimeBarrelAt(position)
	if !active {
		return nil
	}

	r.closeMenuLocked(session, false)

	menu := newNineByThreeMenu(session.allocateWindowID(), &barrel.entity.Items, &session.Player.Inventory)

	menu.barrel = barrel
	session.containerMenu = menu

	if barrel.viewers == nil {
		barrel.viewers = make(map[*Session]struct{})
	}

	firstViewer := len(barrel.viewers) == 0
	barrel.viewers[session] = struct{}{}

	if firstViewer {
		r.setBarrelOpenStateLocked(barrel, true)
	}

	err := session.writePacket(protocol.ClientboundOpenScreenID, protocol.OpenScreen{
		ContainerID: menu.windowID,
		MenuType:    barrelMenuType,
		Title:       game.TranslatableText("container.barrel"),
	})

	if err != nil {
		r.closeMenuLocked(session, false)
		return err
	}

	return session.sendMenuSnapshot(menu.snapshot())
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

	if current.barrel != nil {
		delete(current.barrel.viewers, session)
		if len(current.barrel.viewers) == 0 {
			r.setBarrelOpenStateLocked(current.barrel, false)
		}
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

		candidate := session.inventoryMenu.candidate()

		moveIntoSlots(candidate, &remaining, slotRange(9, 44))

		session.inventoryMenu.commit(candidate)

		return !remaining.Equal(carried)
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

func (r *Runtime) persistAndSynchronizeBarrelLocked(actor *Session, barrel *runtimeBarrel) {
	r.World.SetBlockEntity(barrel.position, barrel.entity)

	for viewer := range barrel.viewers {
		if viewer == actor {
			continue
		}

		current := viewer.activeMenu()
		if current.barrel != barrel {
			continue
		}

		current.incrementStateID()

		err := viewer.sendMenuSnapshot(current.snapshot())
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to synchronize barrel: %v\n", err)
		}
	}
}

func (r *Runtime) setBarrelOpenStateLocked(barrel *runtimeBarrel, open bool) {
	block := r.World.BlockAt(barrel.position)
	if game.BlockEntityTypeForBlock(block) != game.BlockEntityTypeBarrel {
		return
	}

	replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "open", Value: boolProperty(open)})
	if replacement == block {
		return
	}

	if !r.World.CompareAndSetBlock(barrel.position, block, replacement) {
		return
	}

	state, err := protocolBlockState(replacement)
	if err != nil {
		return
	}

	event := game.SoundBlockBarrelClose
	if open {
		event = game.SoundBlockBarrelOpen
	}

	sound := barrelSound(replacement, barrel.position, event)

	for _, viewer := range r.snapshotSessions() {
		err = viewer.sendBlockUpdateIfLoaded(barrel.position, state)
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to update barrel state: %v\n", err)
		}

		err = viewer.sendSoundIfLoaded(sound, barrel.position)
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to play barrel sound: %v\n", err)
		}
	}
}

func barrelSound(block game.Block, position game.BlockPosition, event game.SoundEvent) protocol.Sound {
	directionX, directionY, directionZ := barrelFacingOffset(blockProperty(block, "facing"))

	return protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: string(event)},
		Source: protocol.SoundSourceBlock,
		X:      float64(position.X) + 0.5 + directionX*0.5,
		Y:      float64(position.Y) + 0.5 + directionY*0.5,
		Z:      float64(position.Z) + 0.5 + directionZ*0.5,
		Volume: 0.5,
		Pitch:  0.9 + rand.Float32()*0.1,
		Seed:   rand.Int64(),
	}
}

func barrelFacingOffset(facing string) (float64, float64, float64) {
	switch facing {
	case "east":
		return 1, 0, 0
	case "south":
		return 0, 0, 1
	case "west":
		return -1, 0, 0
	case "up":
		return 0, 1, 0
	case "down":
		return 0, -1, 0
	default:
		return 0, 0, -1
	}
}

func (r *Runtime) tickOpenMenus() {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	for _, session := range r.snapshotSessions() {
		current := session.activeMenu()
		if current.barrel == nil || barrelMenuStillValid(r.World, session.snapshotPlayer(), current.barrel) {
			continue
		}

		r.closeMenuLocked(session, true)
	}
}

func barrelMenuStillValid(world *game.World, player game.Player, barrel *runtimeBarrel) bool {
	entity, present := world.BlockEntityAt(barrel.position)
	if !present || entity.Type != game.BlockEntityTypeBarrel {
		return false
	}

	eye := player.EyePosition()

	distanceX := eye.X - math.Max(float64(barrel.position.X), math.Min(eye.X, float64(barrel.position.X+1)))
	distanceY := eye.Y - math.Max(float64(barrel.position.Y), math.Min(eye.Y, float64(barrel.position.Y+1)))
	distanceZ := eye.Z - math.Max(float64(barrel.position.Z), math.Min(eye.Z, float64(barrel.position.Z+1)))

	maximumDistance := blockInteractionRange + barrelValidityPadding

	return distanceX*distanceX+distanceY*distanceY+distanceZ*distanceZ < maximumDistance*maximumDistance
}

func (r *Runtime) reconcileRuntimeBlockEntities(records []blockMutationRecord) {
	for _, record := range records {
		previousType := game.BlockEntityTypeForBlock(record.previous)

		nextType := game.BlockEntityTypeForBlock(record.change.Replacement)
		if previousType == nextType {
			continue
		}

		chunkPosition := LoadedChunk{X: chunkCoordinate(float64(record.change.Position.X)), Z: chunkCoordinate(float64(record.change.Position.Z))}

		chunk, active := r.ActiveChunk(chunkPosition)
		if !active {
			continue
		}

		if nextType == game.BlockEntityTypeNone {
			chunk.RemoveBlockEntity(record.change.Position)
			continue
		}

		entity, present := r.World.BlockEntityAt(record.change.Position)
		if present && entity.Type == game.BlockEntityTypeBarrel {
			chunk.SetBlockEntity(record.change.Position, &runtimeBarrel{position: record.change.Position, entity: entity.Clone()})
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
		if current.barrel == nil {
			continue
		}

		if _, wasRemoved := removed[current.barrel.position]; wasRemoved {
			r.closeMenuLocked(session, true)
		}
	}
}
