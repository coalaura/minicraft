package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	arrowBaseDamage                   = 2.0
	arrowGravity                      = 0.05
	arrowAirDrag                      = 0.99
	arrowWaterDrag                    = 0.6
	arrowLifetime                     = 1200
	arrowOwnerMargin                  = 1.0
	arrowCollisionMargin              = 0.3
	arrowEmbeddedBackoff              = 0.05
	arrowShakeTicks             int32 = 7
	arrowRotationSmoothing            = 0.2
	arrowFailedHitSpeed               = 0.001
	arrowEmbeddedPointInflation       = 0.06
)

type runtimeProjectileState struct {
	Velocity  game.Velocity
	OwnerID   int32
	TickCount int32
	LeftOwner bool
}

type runtimeArrowEntity struct {
	State RuntimeEntityState
	runtimeProjectileState

	Rotation      game.Rotation
	Life          int32
	ShakeTime     int32
	PierceLevel   byte
	Critical      bool
	baseDamage    float64
	InGround      bool
	EmbeddedAt    game.BlockPosition
	EmbeddedBlock game.Block
}

type arrowTarget struct {
	entity  RuntimeLivingEntity
	session *Session
	box     game.AABB
	id      int32
}

func (entity *runtimeArrowEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeArrowEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: 4, UpdateInterval: 1, TrackDeltas: true}
}

func (entity *runtimeArrowEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeArrowEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{ID: entity.State.ID, UUID: entity.State.UUID, Position: entity.State.Position, Chunk: entity.State.Chunk, Removed: entity.State.Removed, Velocity: entity.Velocity, Rotation: entity.Rotation}
}

func (entity *runtimeArrowEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	flags := byte(0)

	if entity.Critical {
		flags |= protocol.ArrowFlagCritical
	}

	return []protocol.EntityMetadataEntry{
		{Index: protocol.ArrowFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(flags)},
		{Index: protocol.ArrowPierceLevelMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(entity.PierceLevel)},
		{Index: protocol.ArrowInGroundMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(entity.InGround)},
	}
}

func (entity *runtimeArrowEntity) SetBaseDamage(baseDamage float64) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	entity.baseDamage = baseDamage
}

func (entity *runtimeArrowEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Velocity
}

func (entity *runtimeArrowEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{EntityID: snapshot.ID, UUID: snapshot.UUID, Type: int32(game.EntityArrow), X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, VelocityX: snapshot.Velocity.X, VelocityY: snapshot.Velocity.Y, VelocityZ: snapshot.Velocity.Z, Yaw: snapshot.Yaw, Pitch: snapshot.Pitch, HeadYaw: snapshot.HeadYaw, Data: entity.OwnerID}
}

func (entity *runtimeArrowEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	entity.TickCount++

	if entity.ShakeTime > 0 {
		entity.ShakeTime--
	}

	if entity.InGround {
		currentBlock := runtime.World.BlockAt(entity.EmbeddedAt)
		if currentBlock == entity.EmbeddedBlock || arrowEmbeddedPointCollides(currentBlock, entity.EmbeddedAt, entity.State.Position) {
			entity.Life++

			remove := entity.Life >= arrowLifetime
			entityID := entity.State.ID

			entity.State.mu.Unlock()

			if remove {
				runtime.removeRuntimeEntity(entityID)
			}

			return
		}

		arrowSetInGround(entity, false)

		entity.Life = 0
		entity.Velocity.X *= float64(runtime.nextEntityRandom()) * .2
		entity.Velocity.Y *= float64(runtime.nextEntityRandom()) * .2
		entity.Velocity.Z *= float64(runtime.nextEntityRandom()) * .2
		entity.State.metadataDirty = true
	} else {
		entity.Life = 0
	}

	from := entity.State.Position
	velocity := entity.Velocity
	ownerID := entity.OwnerID
	leftOwner := entity.LeftOwner
	tickCount := entity.TickCount

	entity.State.mu.Unlock()

	targets := runtime.arrowTargets()

	if !leftOwner && !arrowOwnerInSweep(from, velocity, ownerID, targets) {
		leftOwner = true
	}

	to := game.Position{X: from.X + velocity.X, Y: from.Y + velocity.Y, Z: from.Z + velocity.Z}

	blockHit, hitBlock := runtime.sweepWorldBlockSegment(from, to)

	target, entityFraction, hitEntity := arrowNearestTarget(from, to, targets, ownerID, leftOwner, tickCount)

	if hitEntity && (!hitBlock || entityFraction < blockHit.Fraction) {
		if runtime.damageArrowTarget(target, entity.State.ID, ownerID, velocity) {
			runtime.removeRuntimeEntity(entity.State.ID)

			return
		}

		velocity.X *= -0.2
		velocity.Y *= -0.2
		velocity.Z *= -0.2

		if arrowVelocityLength(velocity) < arrowFailedHitSpeed {
			runtime.removeRuntimeEntity(entity.State.ID)

			return
		}

		to = from
	}

	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	previous := entity.State.Position
	entity.LeftOwner = leftOwner

	if hitBlock && (!hitEntity || blockHit.Fraction <= entityFraction) {
		entity.State.Position = arrowBackedOffPosition(from, blockHit.Position, velocity)

		arrowSetInGround(entity, true)

		entity.Life = 0
		entity.EmbeddedAt = blockHit.BlockPosition
		entity.EmbeddedBlock = blockHit.Block
		entity.ShakeTime = arrowShakeTicks
	} else {
		entity.State.Position = to
		entity.Velocity = velocity

		arrowApplyPhysics(runtime, entity)
	}

	entity.State.movementSyncDirty = true

	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)

	runtime.synchronizeRuntimeEntity(entity)
}

func (runtime *Runtime) SpawnArrow(position game.Position, velocity game.Velocity, ownerID int32) *runtimeArrowEntity {
	projectile := runtimeProjectileState{Velocity: velocity, OwnerID: ownerID}

	entity := &runtimeArrowEntity{runtimeProjectileState: projectile, Rotation: arrowRotation(velocity)}

	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) arrowTargets() []arrowTarget {
	targets := make([]arrowTarget, 0)

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if !player.Dead {
			targets = append(targets, arrowTarget{session: session, box: player.collisionBox(), id: player.EntityID})
		}
	}

	entities := runtime.appendRuntimeEntities(nil)

	for _, entity := range entities {
		living, valid := entity.(RuntimeLivingEntity)
		if !valid {
			continue
		}

		state := living.RuntimeEntityState()

		state.mu.RLock()
		position := state.Position
		removed := state.Removed
		id := state.ID

		dead := living.RuntimeLivingState().Dead

		box := living.RuntimeLivingState().CollisionBox(position)
		state.mu.RUnlock()

		if !removed && !dead {
			targets = append(targets, arrowTarget{entity: living, box: box, id: id})
		}
	}

	return targets
}

func (runtime *Runtime) damageArrowTarget(target arrowTarget, arrowID, ownerID int32, velocity game.Velocity) bool {
	speed := math.Sqrt(velocity.X*velocity.X + velocity.Y*velocity.Y + velocity.Z*velocity.Z)

	baseDamage := arrowBaseDamage

	runtime.entityMu.RLock()
	entity := runtime.entities[arrowID]
	runtime.entityMu.RUnlock()

	if arrow, isArrow := entity.(*runtimeArrowEntity); isArrow {
		arrow.State.mu.RLock()
		baseDamage = arrow.baseDamage
		arrow.State.mu.RUnlock()

		if baseDamage <= 0 {
			baseDamage = arrowBaseDamage
		}
	}

	causeID := ownerID

	if causeID == 0 {
		causeID = arrowID
	}

	damage := game.Damage{Type: game.DamageArrow, Amount: float32(math.Ceil(speed * baseDamage)), CauseEntityID: causeID, DirectEntityID: arrowID}

	if target.session != nil {
		if runtime.arrowOwnedByRuntimeLiving(ownerID) {
			damage.Amount = arrowPlayerDamageForDifficulty(runtime.Difficulty, damage.Amount)
		}

		update, applied := runtime.damagePlayerLocked(target.session, damage)
		if applied {
			runtime.sendPlayerSurvivalUpdate(target.session, update)
		}

		return applied
	}

	update, applied := runtime.damageRuntimeLivingEntityLocked(target.entity, damage)
	if applied {
		runtime.sendRuntimeLivingDamageUpdate(update)
	}

	return applied
}

func arrowNearestTarget(from, to game.Position, targets []arrowTarget, ownerID int32, leftOwner bool, tickCount int32) (arrowTarget, float64, bool) {
	deltaX := to.X - from.X
	deltaY := to.Y - from.Y
	deltaZ := to.Z - from.Z

	nearestFraction := math.Inf(1)
	nearest := arrowTarget{}

	for _, target := range targets {
		if target.id == ownerID && !leftOwner {
			continue
		}

		fraction, _, intersects := raycastAABB(from, deltaX, deltaY, deltaZ, arrowInflate(target.box, arrowCollisionMarginForTick(tickCount)))
		if intersects && fraction >= 0 && fraction <= 1 && fraction < nearestFraction {
			nearestFraction = fraction
			nearest = target
		}
	}

	return nearest, nearestFraction, nearestFraction != math.Inf(1)
}

func (runtime *Runtime) arrowOwnedByRuntimeLiving(ownerID int32) bool {
	runtime.entityMu.RLock()
	owner := runtime.entities[ownerID]
	runtime.entityMu.RUnlock()

	_, living := owner.(RuntimeLivingEntity)

	return living
}

func arrowPlayerDamageForDifficulty(difficulty game.Difficulty, damage float32) float32 {
	switch difficulty {
	case game.DifficultyPeaceful:
		return 0
	case game.DifficultyEasy:
		return min(damage/2+1, damage)
	case game.DifficultyHard:
		return damage * 1.5
	default:
		return damage
	}
}

func arrowOwnerInSweep(from game.Position, velocity game.Velocity, ownerID int32, targets []arrowTarget) bool {
	if ownerID == 0 {
		return false
	}

	for _, target := range targets {
		if target.id != ownerID {
			continue
		}

		fraction, _, intersects := raycastAABB(from, velocity.X, velocity.Y, velocity.Z, arrowInflate(target.box, arrowOwnerMargin))
		return intersects && fraction <= 1
	}

	return false
}

func arrowApplyPhysics(runtime *Runtime, entity *runtimeArrowEntity) {
	drag := arrowAirDrag

	if runtime.fluidContact(arrowBox(entity.State.Position), game.FluidTypeWater, false).Depth > fluidContactDepth {
		drag = arrowWaterDrag
	}

	entity.Velocity.X *= drag
	entity.Velocity.Y *= drag
	entity.Velocity.Z *= drag
	entity.Velocity.Y -= arrowGravity

	rotation := arrowRotation(entity.Velocity)

	entity.Rotation.Yaw = arrowLerpRotation(entity.Rotation.Yaw, rotation.Yaw)
	entity.Rotation.Pitch = arrowLerpRotation(entity.Rotation.Pitch, rotation.Pitch)
	entity.Rotation.HeadYaw = entity.Rotation.Yaw
}

func arrowBackedOffPosition(from, hit game.Position, velocity game.Velocity) game.Position {
	length := math.Sqrt(velocity.X*velocity.X + velocity.Y*velocity.Y + velocity.Z*velocity.Z)
	if length == 0 {
		return hit
	}

	return game.Position{X: hit.X - velocity.X/length*arrowEmbeddedBackoff, Y: hit.Y - velocity.Y/length*arrowEmbeddedBackoff, Z: hit.Z - velocity.Z/length*arrowEmbeddedBackoff}
}

func arrowBox(position game.Position) game.AABB {
	return game.AABB{MinX: position.X - 0.25, MinY: position.Y - 0.25, MinZ: position.Z - 0.25, MaxX: position.X + 0.25, MaxY: position.Y + 0.25, MaxZ: position.Z + 0.25}
}

func arrowInflate(box game.AABB, margin float64) game.AABB {
	return game.AABB{MinX: box.MinX - margin, MinY: box.MinY - margin, MinZ: box.MinZ - margin, MaxX: box.MaxX + margin, MaxY: box.MaxY + margin, MaxZ: box.MaxZ + margin}
}

func arrowEmbeddedPointCollides(block game.Block, blockPosition game.BlockPosition, position game.Position) bool {
	point := game.AABB{
		MinX: position.X - arrowEmbeddedPointInflation,
		MinY: position.Y - arrowEmbeddedPointInflation,
		MinZ: position.Z - arrowEmbeddedPointInflation,
		MaxX: position.X + arrowEmbeddedPointInflation,
		MaxY: position.Y + arrowEmbeddedPointInflation,
		MaxZ: position.Z + arrowEmbeddedPointInflation,
	}

	var boxBuffer [7]game.AABB

	return slices.ContainsFunc(block.AppendCollisionBoxes(boxBuffer[:0], blockPosition), point.Intersects)
}

func arrowCollisionMarginForTick(tickCount int32) float64 {
	return max(0, min(arrowCollisionMargin, float64(tickCount-2)/20))
}

func arrowVelocityLength(velocity game.Velocity) float64 {
	return math.Sqrt(velocity.X*velocity.X + velocity.Y*velocity.Y + velocity.Z*velocity.Z)
}

// arrowSetInGround records the metadata change while the entity state lock is held.
func arrowSetInGround(entity *runtimeArrowEntity, inGround bool) {
	if entity.InGround == inGround {
		return
	}

	entity.InGround = inGround
	entity.State.metadataDirty = true
}

func arrowRotation(velocity game.Velocity) game.Rotation {
	horizontal := math.Hypot(velocity.X, velocity.Z)
	if horizontal == 0 && velocity.Y == 0 {
		return game.Rotation{}
	}

	yaw := math.Atan2(velocity.X, velocity.Z) * 180 / math.Pi
	pitch := math.Atan2(velocity.Y, horizontal) * 180 / math.Pi

	return game.Rotation{Yaw: float32(yaw), Pitch: float32(pitch), HeadYaw: float32(yaw)}
}

func arrowLerpRotation(from, to float32) float32 {
	for to-from < -180 {
		to += 360
	}

	for to-from >= 180 {
		to -= 360
	}

	return from + (to-from)*arrowRotationSmoothing
}
