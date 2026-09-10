package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	tntDefaultFuse    = 80
	tntExplosionPower = 4
	tntGravity        = 0.04
	tntDrag           = 0.98
	tntGroundDrag     = 0.7
	tntGroundBounce   = -0.5
)

type runtimeTntEntity struct {
	State RuntimeEntityState

	Velocity game.Velocity
	OwnerID  int32
	Fuse     int32
	OnGround bool

	OwnerIsPlayer bool
}

func (entity *runtimeTntEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeTntEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: 10, UpdateInterval: 10, TrackDeltas: true}
}

func (entity *runtimeTntEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeTntEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return []protocol.EntityMetadataEntry{
		{Index: protocol.TntFuseMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.Fuse)},
		{Index: protocol.TntBlockStateMetadataIndex, Type: protocol.MetadataTypeBlockState, Value: protocol.MetadataVarInt(game.Tnt)},
	}
}

func (entity *runtimeTntEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Velocity
}

func (entity *runtimeTntEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{EntityID: snapshot.ID, UUID: snapshot.UUID, Type: int32(game.EntityTnt), X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, VelocityX: snapshot.Velocity.X, VelocityY: snapshot.Velocity.Y, VelocityZ: snapshot.Velocity.Z}
}

func (entity *runtimeTntEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	definition, valid := game.EntityTnt.Definition()
	if !valid {
		return
	}

	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	runtime.applyPhysicalEntityFluidCurrents(entityTntBox(entity.State.Position, definition), &entity.Velocity)

	velocity := entity.Velocity
	velocity.Y -= tntGravity

	movement := runtime.moveGroundEntity(entity.State.Position, velocity, definition.Width, definition.Height, 0, entity.OnGround)

	previous := entity.State.Position
	entity.State.Position = movement.Position
	entity.OnGround = movement.OnGround

	velocity.X *= tntDrag
	velocity.Y *= tntDrag
	velocity.Z *= tntDrag

	if movement.OnGround {
		velocity.X *= tntGroundDrag
		velocity.Y *= tntGroundBounce
		velocity.Z *= tntGroundDrag
	}

	entity.Velocity = velocity
	entity.Fuse--
	entity.State.metadataDirty = true
	entity.State.movementSyncDirty = true

	entityID := entity.State.ID
	ownerID := entity.OwnerID
	ownerIsPlayer := entity.OwnerIsPlayer
	position := entity.State.Position
	fuse := entity.Fuse

	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)

	if fuse > 0 {
		runtime.synchronizeRuntimeEntity(entity)

		return
	}

	runtime.removeRuntimeEntity(entityID)

	position.Y += definition.Height * 0.0625

	_, players, living := runtime.explodeLocked(RuntimeExplosion{Position: position, Radius: tntExplosionPower, DirectEntityID: entityID, CauseEntityID: ownerID, CauseIsPlayer: ownerIsPlayer, BlockInteraction: ExplosionDestroyBlocksWithDecay})

	runtime.sendExplosionEntityUpdates(players, living)
}

func (runtime *Runtime) SpawnTnt(position game.Position, velocity game.Velocity, ownerID int32, ownerIsPlayer bool) *runtimeTntEntity {
	entity := &runtimeTntEntity{Velocity: velocity, OwnerID: ownerID, Fuse: tntDefaultFuse, OwnerIsPlayer: ownerIsPlayer}

	runtime.registerRuntimeEntity(entity, game.EntityTnt, position)

	return entity
}

func (runtime *Runtime) primeTnt(position game.BlockPosition, ownerID int32, ownerIsPlayer bool, fuse int32) *runtimeTntEntity {
	angle := float64(runtime.nextEntityRandom()) * math.Pi * 2
	spawn := game.Position{X: float64(position.X) + 0.5, Y: float64(position.Y), Z: float64(position.Z) + 0.5}
	velocity := game.Velocity{X: -math.Sin(angle) * 0.02, Y: 0.2, Z: -math.Cos(angle) * 0.02}

	entity := runtime.SpawnTnt(spawn, velocity, ownerID, ownerIsPlayer)

	entity.State.mu.Lock()
	entity.Fuse = fuse
	entity.State.metadataDirty = true
	entity.State.mu.Unlock()

	return entity
}

func (entity *runtimeTntEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{ID: entity.State.ID, UUID: entity.State.UUID, Position: entity.State.Position, Chunk: entity.State.Chunk, Removed: entity.State.Removed, Velocity: entity.Velocity, OnGround: entity.OnGround}
}

func entityTntBox(position game.Position, definition game.EntityDefinition) game.AABB {
	halfWidth := definition.Width / 2

	return game.AABB{MinX: position.X - halfWidth, MinY: position.Y, MinZ: position.Z - halfWidth, MaxX: position.X + halfWidth, MaxY: position.Y + definition.Height, MaxZ: position.Z + halfWidth}
}
