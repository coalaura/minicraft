package server

import (
	"math/rand/v2"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type runtimeBarrel struct {
	position game.BlockPosition
	entity   game.BlockEntity
	viewers  map[*Session]struct{}
}

func newRuntimeBarrel(position game.BlockPosition, entity game.BlockEntity) RuntimeBlockEntity {
	return &runtimeBarrel{position: position, entity: entity}
}

func (barrel *runtimeBarrel) BlockEntityType() game.BlockEntityType {
	return game.BlockEntityTypeBarrel
}

func (barrel *runtimeBarrel) BlockPosition() game.BlockPosition {
	return barrel.position
}

func (barrel *runtimeBarrel) ContainsPosition(position game.BlockPosition) bool {
	return barrel.position == position
}

func (barrel *runtimeBarrel) InteractBlock(runtime *Runtime, session *Session) error {
	return runtime.openBarrelLocked(session, barrel)
}

func (barrel *runtimeBarrel) Attach(runtime *Runtime, session *Session) {
	if barrel.viewers == nil {
		barrel.viewers = make(map[*Session]struct{})
	}

	firstViewer := len(barrel.viewers) == 0
	barrel.viewers[session] = struct{}{}

	if firstViewer {
		runtime.setBarrelOpenStateLocked(barrel, true)
	}
}

func (barrel *runtimeBarrel) Detach(runtime *Runtime, session *Session) {
	delete(barrel.viewers, session)

	if len(barrel.viewers) == 0 {
		runtime.setBarrelOpenStateLocked(barrel, false)
	}
}

func (barrel *runtimeBarrel) Changed(runtime *Runtime, actor *Session, _ []int) {
	runtime.World.SetBlockEntity(barrel.position, barrel.entity)

	for viewer := range barrel.viewers {
		if viewer == actor {
			continue
		}

		current := viewer.activeMenu()
		if current.backing != barrel {
			continue
		}

		current.incrementStateID()

		err := viewer.sendMenuSnapshot(current.snapshot())
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to synchronize barrel: %v\n", err)
		}
	}
}

func (barrel *runtimeBarrel) StillValid(runtime *Runtime, session *Session) bool {
	return containerBlockEntityStillValid(runtime, session, barrel)
}

func (r *Runtime) openBarrelLocked(session *Session, barrel *runtimeBarrel) error {
	items, inventory := barrel.entity.Inventory()
	if !inventory || len(items) != game.BarrelSlotCount {
		return nil
	}

	r.closeMenuLocked(session, false)

	menu := newGenericContainerMenu(session.allocateWindowID(), 3, items, &session.Player.Inventory)

	menu.backing = barrel
	session.containerMenu = menu

	barrel.Attach(r, session)

	err := session.writePacket(protocol.ClientboundOpenScreenID, protocol.OpenScreen{
		ContainerID: menu.windowID,
		MenuType:    menu.protocolMenuType,
		Title:       game.TranslatableText("container.barrel"),
	})

	if err != nil {
		r.closeMenuLocked(session, false)

		return err
	}

	return session.sendMenuSnapshot(menu.snapshot())
}

func (r *Runtime) runtimeBarrelAt(position game.BlockPosition) (*runtimeBarrel, bool) {
	entity, present := r.runtimeBlockEntityAt(position)
	if !present {
		return nil, false
	}

	barrel, valid := entity.(*runtimeBarrel)
	return barrel, valid
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

	event := game.SoundBlockBarrelClose

	if open {
		event = game.SoundBlockBarrelOpen
	}

	sound := barrelSound(replacement, barrel.position, event)

	change := game.BlockChange{Position: barrel.position, Replacement: replacement}

	result, delivery, err := r.mutateBlocksLocked(nil, BlockMutationInteract, []game.BlockChange{change}, 1, true, false, true, false)
	if err != nil || !result.Changed {
		if err != nil {
			for _, session := range r.snapshotSessions() {
				if session.Log != nil {
					session.Log.Warnf("[play] failed to mutate barrel state: %v\n", err)
				}
			}
		}

		return
	}

	delivery.runtimeSounds = []positionalBlockSound{{position: barrel.position, sound: sound}}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
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
