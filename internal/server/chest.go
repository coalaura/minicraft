package server

import (
	"math/rand/v2"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const chestOpenersEvent byte = 1

type runtimeChest struct {
	position game.BlockPosition
	entity   game.BlockEntity
	viewers  map[*Session]struct{}
}

type runtimeChestBacking struct {
	chests []*runtimeChest
}

func (chest *runtimeChest) BlockEntityType() game.BlockEntityType {
	return chest.entity.Type
}

func (chest *runtimeChest) BlockPosition() game.BlockPosition {
	return chest.position
}

func (chest *runtimeChest) InteractBlock(runtime *Runtime, session *Session) error {
	return runtime.openChestLocked(session, chest)
}

func (backing *runtimeChestBacking) ContainsPosition(position game.BlockPosition) bool {
	for _, chest := range backing.chests {
		if chest.position == position {
			return true
		}
	}

	return false
}

func (backing *runtimeChestBacking) Attach(runtime *Runtime, session *Session) {
	for _, chest := range backing.chests {
		chest.attach(runtime, session)
	}
}

func (backing *runtimeChestBacking) Detach(runtime *Runtime, session *Session) {
	for _, chest := range backing.chests {
		chest.detach(runtime, session)
	}
}

func (backing *runtimeChestBacking) Changed(runtime *Runtime, actor *Session, changed []int) {
	for _, index := range changed {
		if index < 0 || index >= len(backing.chests) {
			continue
		}

		chest := backing.chests[index]
		runtime.World.SetBlockEntity(chest.position, chest.entity)
	}

	viewers := make(map[*Session]struct{})

	for _, chest := range backing.chests {
		for viewer := range chest.viewers {
			viewers[viewer] = struct{}{}
		}
	}

	for viewer := range viewers {
		if viewer == actor {
			continue
		}

		current := viewer.activeMenu()

		currentBacking, valid := current.backing.(*runtimeChestBacking)
		if !valid || !backing.sameChestSet(currentBacking) {
			continue
		}

		current.incrementStateID()

		err := viewer.sendMenuSnapshot(current.snapshot())
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to synchronize chest: %v\n", err)
		}
	}
}

func (backing *runtimeChestBacking) StillValid(runtime *Runtime, session *Session) bool {
	for _, chest := range backing.chests {
		if !containerBlockEntityStillValid(runtime, session, chest) {
			return false
		}
	}

	resolved, valid := runtime.chestBackingAt(backing.chests[0].position)
	return valid && backing.sameChestSet(resolved)
}

func (backing *runtimeChestBacking) sameChestSet(other *runtimeChestBacking) bool {
	if other == nil || len(backing.chests) != len(other.chests) {
		return false
	}

	for index := range backing.chests {
		if backing.chests[index] != other.chests[index] {
			return false
		}
	}

	return true
}

func (chest *runtimeChest) attach(runtime *Runtime, session *Session) {
	if chest.viewers == nil {
		chest.viewers = make(map[*Session]struct{})
	}

	if _, present := chest.viewers[session]; present {
		return
	}

	firstViewer := len(chest.viewers) == 0
	chest.viewers[session] = struct{}{}

	chest.sendOpenersEvent(runtime)

	if firstViewer {
		chest.sendSound(runtime, chestSound(runtime.World.BlockAt(chest.position), true))
	}
}

func (chest *runtimeChest) detach(runtime *Runtime, session *Session) {
	if _, present := chest.viewers[session]; !present {
		return
	}

	delete(chest.viewers, session)

	chest.sendOpenersEvent(runtime)

	if len(chest.viewers) == 0 {
		chest.sendSound(runtime, chestSound(runtime.World.BlockAt(chest.position), false))
	}
}

func (chest *runtimeChest) sendOpenersEvent(runtime *Runtime) {
	block := runtime.World.BlockAt(chest.position)

	definition, valid := block.Definition()
	if !valid || game.BlockEntityTypeForBlock(block) != chest.BlockEntityType() {
		return
	}

	event := protocol.BlockEvent{
		Position: chest.position,
		Event:    chestOpenersEvent,
		Param:    byte(len(chest.viewers)),
		Block:    int32(definition.ID),
	}

	for _, viewer := range runtime.snapshotSessions() {
		err := viewer.sendBlockEventIfLoaded(event)
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to send chest block event: %v\n", err)
		}
	}
}

func (chest *runtimeChest) sendSound(runtime *Runtime, event game.SoundEvent) {
	block := runtime.World.BlockAt(chest.position)
	if game.BlockEntityTypeForBlock(block) != chest.BlockEntityType() || blockProperty(block, "type") == "left" {
		return
	}

	soundPosition := chest.position

	directionX := 0.0
	directionZ := 0.0

	if blockProperty(block, "type") == "right" {
		connected, valid := chestBlockConnectedDirection(block)
		if !valid {
			return
		}

		directionX, directionZ = horizontalSoundOffset(connected)
	}

	sound := protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: string(event)},
		Source: protocol.SoundSourceBlock,
		X:      float64(soundPosition.X) + 0.5 + directionX*0.5,
		Y:      float64(soundPosition.Y) + 0.5,
		Z:      float64(soundPosition.Z) + 0.5 + directionZ*0.5,
		Volume: 0.5,
		Pitch:  0.9 + rand.Float32()*0.1,
		Seed:   rand.Int64(),
	}

	for _, viewer := range runtime.snapshotSessions() {
		err := viewer.sendSoundIfLoaded(sound, soundPosition)
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to play chest sound: %v\n", err)
		}
	}
}

func (r *Runtime) openChestLocked(session *Session, clicked *runtimeChest) error {
	backing, valid := r.chestBackingAt(clicked.position)
	if !valid || r.chestBackingBlocked(backing) {
		return nil
	}

	rows := len(backing.chests) * 3
	inventories := make([]menuInventoryBacking, 0, len(backing.chests))

	for index, chest := range backing.chests {
		items, inventory := chest.entity.Inventory()
		if !inventory || len(items) != game.ChestSlotCount {
			return nil
		}

		inventories = append(inventories, menuInventoryBacking{items: items, index: index})
	}

	r.closeMenuLocked(session, false)

	menu := newComposedGenericContainerMenu(session.allocateWindowID(), rows, inventories, &session.Player.Inventory)
	if menu == nil {
		return nil
	}

	menu.backing = backing
	session.containerMenu = menu

	backing.Attach(r, session)

	title := "container.chest"

	if len(backing.chests) == 2 {
		title = "container.chestDouble"
	}

	err := session.writePacket(protocol.ClientboundOpenScreenID, protocol.OpenScreen{
		ContainerID: menu.windowID,
		MenuType:    menu.protocolMenuType,
		Title:       game.TranslatableText(title),
	})

	if err != nil {
		r.closeMenuLocked(session, false)

		return err
	}

	return session.sendMenuSnapshot(menu.snapshot())
}

func (r *Runtime) chestBackingAt(position game.BlockPosition) (*runtimeChestBacking, bool) {
	block := r.World.BlockAt(position)
	if block.Behavior() != game.BlockBehaviorChest {
		return nil, false
	}

	entity, present := r.runtimeBlockEntityAt(position)

	chest, valid := entity.(*runtimeChest)
	if !present || !valid || chest.BlockEntityType() != game.BlockEntityTypeForBlock(block) {
		return nil, false
	}

	chestType := blockProperty(block, "type")
	if chestType == "single" {
		return &runtimeChestBacking{chests: []*runtimeChest{chest}}, true
	}

	connected, valid := chestBlockConnectedDirection(block)
	if !valid {
		return nil, false
	}

	partnerPosition, valid := connected.offset(position)
	if !valid {
		return nil, false
	}

	partnerBlock := r.World.BlockAt(partnerPosition)
	if !chestBlocksCanConnect(block, partnerBlock) || blockProperty(partnerBlock, "facing") != blockProperty(block, "facing") || blockProperty(partnerBlock, "type") != oppositeChestType(chestType) {
		return nil, false
	}

	partnerEntity, present := r.runtimeBlockEntityAt(partnerPosition)

	partner, valid := partnerEntity.(*runtimeChest)
	if !present || !valid || partner.BlockEntityType() != chest.BlockEntityType() {
		return nil, false
	}

	if chestType == "right" {
		return &runtimeChestBacking{chests: []*runtimeChest{chest, partner}}, true
	}

	return &runtimeChestBacking{chests: []*runtimeChest{partner, chest}}, true
}

func (r *Runtime) chestBackingBlocked(backing *runtimeChestBacking) bool {
	for _, chest := range backing.chests {
		above := chest.position

		above.Y++

		block := r.World.BlockAt(above)

		if block.IsRedstoneConductor() {
			return true
		}
	}

	return false
}

func newRuntimeChest(position game.BlockPosition, entity game.BlockEntity) RuntimeBlockEntity {
	return &runtimeChest{position: position, entity: entity}
}

func horizontalSoundOffset(direction horizontalDirection) (float64, float64) {
	switch direction {
	case directionSouth:
		return 0, 1
	case directionWest:
		return -1, 0
	case directionEast:
		return 1, 0
	default:
		return 0, -1
	}
}

func chestBlockConnectedDirection(block game.Block) (horizontalDirection, bool) {
	facing, valid := directionFromName(blockProperty(block, "facing"))
	if !valid || blockProperty(block, "type") == "single" {
		return 0, false
	}

	return chestConnectedDirection(facing, blockProperty(block, "type")), true
}

func chestBlocksCanConnect(first, second game.Block) bool {
	if sameBlockType(first, second) {
		return true
	}

	_, _, firstCopper := copperChestProperties(first)
	_, _, secondCopper := copperChestProperties(second)

	return firstCopper && secondCopper
}

func copperChestProperties(block game.Block) (weathering int, waxed, valid bool) {
	definition, defined := block.Definition()
	if !defined {
		return 0, false, false
	}

	switch definition.ID {
	case game.CopperChestID:
		return 0, false, true
	case game.ExposedCopperChestID:
		return 1, false, true
	case game.WeatheredCopperChestID:
		return 2, false, true
	case game.OxidizedCopperChestID:
		return 3, false, true
	case game.WaxedCopperChestID:
		return 0, true, true
	case game.WaxedExposedCopperChestID:
		return 1, true, true
	case game.WaxedWeatheredCopperChestID:
		return 2, true, true
	case game.WaxedOxidizedCopperChestID:
		return 3, true, true
	default:
		return 0, false, false
	}
}

func normalizedCopperChestBlock(first, second game.Block) (game.Block, bool) {
	firstWeathering, firstWaxed, firstCopper := copperChestProperties(first)
	secondWeathering, secondWaxed, secondCopper := copperChestProperties(second)

	if !firstCopper || !secondCopper {
		return first, false
	}

	weathering := min(firstWeathering, secondWeathering)
	waxed := firstWaxed && secondWaxed

	switch {
	case waxed && weathering == 0:
		return game.WaxedCopperChest, true
	case waxed && weathering == 1:
		return game.WaxedExposedCopperChest, true
	case waxed && weathering == 2:
		return game.WaxedWeatheredCopperChest, true
	case waxed:
		return game.WaxedOxidizedCopperChest, true
	case weathering == 0:
		return game.CopperChest, true
	case weathering == 1:
		return game.ExposedCopperChest, true
	case weathering == 2:
		return game.WeatheredCopperChest, true
	default:
		return game.OxidizedCopperChest, true
	}
}

func copyChestProperties(destination, source game.Block) game.Block {
	return withBlockProperties(destination,
		game.BlockPropertyValue{Name: "facing", Value: blockProperty(source, "facing")},
		game.BlockPropertyValue{Name: "type", Value: blockProperty(source, "type")},
		game.BlockPropertyValue{Name: "waterlogged", Value: blockProperty(source, "waterlogged")},
	)
}

func chestSound(block game.Block, open bool) game.SoundEvent {
	weathering, _, copper := copperChestProperties(block)
	if !copper {
		if open {
			return game.SoundBlockChestOpen
		}

		return game.SoundBlockChestClose
	}

	if weathering >= 3 {
		if open {
			return game.SoundBlockCopperChestOxidizedOpen
		}

		return game.SoundBlockCopperChestOxidizedClose
	}

	if weathering == 2 {
		if open {
			return game.SoundBlockCopperChestWeatheredOpen
		}

		return game.SoundBlockCopperChestWeatheredClose
	}

	if open {
		return game.SoundBlockCopperChestOpen
	}

	return game.SoundBlockCopperChestClose
}
