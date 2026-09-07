package server

import (
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

type runtimeEntityConstructor func(*Runtime, game.Position) RuntimeEntity

var runtimeEntityConstructors = map[game.EntityType]runtimeEntityConstructor{
	game.EntityArrow: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnArrow(position, game.Velocity{}, 0)
	},
	game.EntityChicken: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnChicken(position)
	},
	game.EntityCow: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnCow(position)
	},
	game.EntityCreeper: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnCreeper(position)
	},
	game.EntitySheep: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnSheep(position)
	},
	game.EntitySkeleton: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnSkeleton(position)
	},
	game.EntityTnt: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnTnt(position, game.Velocity{}, 0, false)
	},
	game.EntityZombie: func(runtime *Runtime, position game.Position) RuntimeEntity {
		return runtime.SpawnZombie(position)
	},
}

func (r *Runtime) SpawnEntity(entityType game.EntityType, position game.Position) (RuntimeEntity, bool) {
	constructor, implemented := runtimeEntityConstructors[entityType]
	if !implemented {
		return nil, false
	}

	entity := constructor(r, position)

	return entity, entity != nil
}

func (r *Runtime) registerRuntimeEntity(entity RuntimeEntity, position game.Position) {
	state := entity.RuntimeEntityState()

	state.ID = r.allocateEntityID()
	state.UUID = randomEntityUUID()
	state.Position = position
	state.Chunk = positionLoadedChunk(position)

	tracked := entity.(RuntimeEntityTracker)

	state.tracker = newRuntimeEntityTracker(tracked.RuntimeEntityView())

	r.entityMu.Lock()
	r.entities[state.ID] = entity

	r.addEntityToChunkIndexLocked(entity)
	r.entityMu.Unlock()

	chunk, active := r.ActiveChunk(state.Chunk)
	if active {
		chunk.SetEntity(state.ID, entity)
	}

	r.reconcileRuntimeEntityTracking(entity)
}

func runtimeEntityImplemented(entityType game.EntityType) bool {
	_, implemented := runtimeEntityConstructors[entityType]

	return implemented
}

func runtimeEntityImplementationNames() []string {
	names := make([]string, 0, len(runtimeEntityConstructors))

	for entityType := range runtimeEntityConstructors {
		definition, valid := entityType.Definition()
		if valid {
			names = append(names, "minecraft:"+definition.Name)
		}
	}

	slices.Sort(names)

	return names
}
