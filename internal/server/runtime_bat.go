package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	batAmbientSoundResetTicks = 80
	batDisturbanceRange       = 4
	batEyeHeight              = 0.45
	batFlightHorizontalSpeed  = 0.5
	batFlightVerticalSpeed    = 0.7
	batForwardInput           = float32(0.5)
	batGravity                = 0.08
	batLavaJumpThreshold      = 0.4
	batMaxHealth              = 6
	batRestingFlag            = byte(0x01)
	batTrackingInterval       = 3
	batTrackingRangeChunks    = 5
	batTravelAcceleration     = float32(0.02)
	batVerticalDamping        = 0.6
)

type runtimeBatEntity struct {
	State  RuntimeEntityState
	Living RuntimeLivingState
	RuntimeMobState
	Rotation game.Rotation

	FlightTarget     game.BlockPosition
	HasFlightTarget  bool
	Resting          bool
	TickCount        int32
	AmbientSoundTime int32
}

func (entity *runtimeBatEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeBatEntity) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (*runtimeBatEntity) RuntimeLivingEyeHeight() float64 {
	return batEyeHeight
}

func (entity *runtimeBatEntity) RuntimeMob() *RuntimeMobState {
	return &entity.RuntimeMobState
}

func (*runtimeBatEntity) RuntimeMobDespawnConfig() RuntimeMobDespawnConfig {
	return RuntimeMobDespawnConfig{
		NoDespawnDistance: runtimeMobNoDespawnDistance,
		DespawnDistance:   runtimeMobDespawnDistance,
		AllowedInPeaceful: true,
	}
}

func (*runtimeBatEntity) RuntimeMobRemoveWhenFarAway(float64) bool {
	return true
}

func (*runtimeBatEntity) RuntimeMobRequiresCustomPersistence() bool {
	return false
}

func (*runtimeBatEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: batTrackingRangeChunks, UpdateInterval: batTrackingInterval, TrackDeltas: false}
}

func (entity *runtimeBatEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeBatEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	metadata := runtimeMobMetadata(entity.Living.EntityFlags(), entity.Living.Health, 0)
	flags := byte(0)

	if entity.Resting {
		flags = batRestingFlag
	}

	return append(metadata, protocol.EntityMetadataEntry{Index: protocol.BatFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(flags)})
}

func (entity *runtimeBatEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (*runtimeBatEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  snapshot.ID,
		UUID:      snapshot.UUID,
		Type:      int32(game.EntityBat),
		X:         snapshot.Position.X,
		Y:         snapshot.Position.Y,
		Z:         snapshot.Position.Z,
		VelocityX: snapshot.Velocity.X,
		VelocityY: snapshot.Velocity.Y,
		VelocityZ: snapshot.Velocity.Z,
		Yaw:       snapshot.Yaw,
		Pitch:     snapshot.Pitch,
		HeadYaw:   snapshot.HeadYaw,
	}
}

func (entity *runtimeBatEntity) RuntimeLivingDamageSound(died bool) (game.SoundEvent, float32, float32) {
	if died {
		return game.SoundEntityBatDeath, 0.1, 0.95
	}

	entity.State.mu.Lock()
	entity.AmbientSoundTime = -batAmbientSoundResetTicks
	entity.State.mu.Unlock()

	return game.SoundEntityBatHurt, 0.1, 0.95
}

func (*runtimeBatEntity) RuntimeEntitySoundSource() int32 {
	return protocol.SoundSourceNeutral
}

func (entity *runtimeBatEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickBat(entity)
}

func (entity *runtimeBatEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{
		ID:       entity.State.ID,
		UUID:     entity.State.UUID,
		Position: entity.State.Position,
		Chunk:    entity.State.Chunk,
		Removed:  entity.State.Removed,
		Velocity: entity.Living.Velocity,
		Rotation: entity.Rotation,
		OnGround: entity.Living.OnGround,
	}
}

func (entity *runtimeBatEntity) runtimeLivingDamageAttemptLocked() {
	entity.setRestingLocked(false)
}

func (entity *runtimeBatEntity) setRestingLocked(resting bool) {
	if entity.Resting == resting {
		return
	}

	entity.Resting = resting
	entity.State.metadataDirty = true
}

func (runtime *Runtime) SpawnBat(position game.Position) *runtimeBatEntity {
	definition, valid := game.EntityBat.Definition()
	if !valid {
		return nil
	}

	entity := &runtimeBatEntity{Resting: true}
	entity.Living = RuntimeLivingState{Width: definition.Width, Height: definition.Height}
	entity.Living.Reset(batMaxHealth)

	runtime.registerRuntimeEntity(entity, game.EntityBat, position)

	return entity
}

func (runtime *Runtime) tickBat(entity *runtimeBatEntity) {
	entity.State.mu.RLock()
	removed := entity.State.Removed
	dead := entity.Living.Dead
	entity.State.mu.RUnlock()

	if removed {
		return
	}

	if dead {
		runtime.tickRuntimeLivingEntity(entity)

		return
	}

	if runtime.checkRuntimeMobDespawn(entity) {
		return
	}

	entity.State.mu.Lock()
	entity.TickCount++
	entity.NoActionTime++
	entity.State.mu.Unlock()

	runtime.tickRuntimeLivingBaseEnvironment(entity)

	if runtimeLivingDead(entity) {
		runtime.tickRuntimeLivingEntity(entity)
		runtime.synchronizeRuntimeEntity(entity)

		return
	}

	forwardInput, takeoffPosition, takeoff := runtime.tickBatAI(entity)
	if takeoff {
		runtime.broadcastBatTakeoff(takeoffPosition)
	}

	runtime.tickBatMovement(entity, forwardInput)
	runtime.tickRuntimeLivingBlockEnvironment(entity)
	runtime.tickBatAmbientSound(entity)
	runtime.tickRuntimeLivingEntity(entity)
	runtime.synchronizeRuntimeEntity(entity)
}

func (runtime *Runtime) tickBatAI(entity *runtimeBatEntity) (float32, game.BlockPosition, bool) {
	entity.State.mu.Lock()

	position := entity.State.Position
	blockPosition := batBlockPosition(position)
	above := blockPosition
	above.Y++
	takeoff := false

	if entity.Resting {
		if runtime.World.BlockAt(above).IsRedstoneConductor() {
			if runtimeMobRandomInt(runtime, 200) == 0 {
				entity.Rotation.HeadYaw = float32(runtimeMobRandomInt(runtime, 360))
			}

			if runtime.batPlayerNearby(position) {
				entity.setRestingLocked(false)
				takeoff = true
			}
		} else {
			entity.setRestingLocked(false)
			takeoff = true
		}

		entity.State.mu.Unlock()

		return 0, blockPosition, takeoff
	}

	if entity.HasFlightTarget {
		targetBlock := runtime.World.BlockAt(entity.FlightTarget)
		if !batTargetBlockEmpty(targetBlock) || entity.FlightTarget.Y <= protocol.OverworldMinY {
			entity.HasFlightTarget = false
		}
	}

	chooseTarget := !entity.HasFlightTarget
	if !chooseTarget {
		chooseTarget = runtimeMobRandomInt(runtime, 30) == 0 || batTargetNearPosition(entity.FlightTarget, position)
	}

	if chooseTarget {
		targetOffsetX := runtimeMobRandomInt(runtime, 7)
		targetOffsetX -= runtimeMobRandomInt(runtime, 7)

		targetOffsetY := runtimeMobRandomInt(runtime, 6) - 2

		targetOffsetZ := runtimeMobRandomInt(runtime, 7)
		targetOffsetZ -= runtimeMobRandomInt(runtime, 7)

		entity.FlightTarget = game.BlockPosition{
			X: blockPosition.X + int32(targetOffsetX),
			Y: blockPosition.Y + int32(targetOffsetY),
			Z: blockPosition.Z + int32(targetOffsetZ),
		}
		entity.HasFlightTarget = true
	}

	deltaX := float64(entity.FlightTarget.X) + 0.5 - position.X
	deltaY := float64(entity.FlightTarget.Y) + 0.1 - position.Y
	deltaZ := float64(entity.FlightTarget.Z) + 0.5 - position.Z

	entity.Living.Velocity.X += (batSign(deltaX)*batFlightHorizontalSpeed - entity.Living.Velocity.X) * 0.1
	entity.Living.Velocity.Y += (batSign(deltaY)*batFlightVerticalSpeed - entity.Living.Velocity.Y) * 0.1
	entity.Living.Velocity.Z += (batSign(deltaZ)*batFlightHorizontalSpeed - entity.Living.Velocity.Z) * 0.1

	desiredYaw := float32(math.Atan2(entity.Living.Velocity.Z, entity.Living.Velocity.X)*180/math.Pi - 90)
	entity.Rotation.Yaw += wrapDegrees(desiredYaw - entity.Rotation.Yaw)

	if runtimeMobRandomInt(runtime, 100) == 0 && runtime.World.BlockAt(above).IsRedstoneConductor() {
		entity.setRestingLocked(true)
	}

	entity.State.mu.Unlock()

	return batForwardInput, game.BlockPosition{}, false
}

func (runtime *Runtime) tickBatMovement(entity *runtimeBatEntity, forwardInput float32) {
	entity.State.mu.Lock()

	previous := entity.State.Position
	box := entity.Living.CollisionBox(previous)

	runtime.applyPhysicalEntityFluidCurrents(box, &entity.Living.Velocity)

	water := runtime.fluidContact(box, game.FluidTypeWater, false)
	lava := runtime.fluidContact(box, game.FluidTypeLava, false)

	if forwardInput != 0 {
		yaw := float64(entity.Rotation.Yaw) * math.Pi / 180
		acceleration := float64(forwardInput * batTravelAcceleration)

		entity.Living.Velocity.X -= float64(minecraftSin(yaw)) * acceleration
		entity.Living.Velocity.Z += float64(minecraftCos(yaw)) * acceleration
	}

	movement := runtime.moveFreeFlightEntity(entity.State.Position, entity.Living.Velocity, entity.Living.Width, entity.Living.Height)
	entity.State.Position = movement.Position
	entity.Living.OnGround = movement.OnGround

	if movement.HorizontalCollisionX {
		entity.Living.Velocity.X = 0
	}

	if movement.HorizontalCollisionZ {
		entity.Living.Velocity.Z = 0
	}

	if movement.VerticalCollision {
		entity.Living.Velocity.Y = 0
	}

	switch {
	case water.Depth > 0:
		entity.Living.Velocity.X *= 0.8
		entity.Living.Velocity.Y *= 0.8
		entity.Living.Velocity.Z *= 0.8

		if entity.Living.Velocity.Y <= 0 {
			entity.Living.Velocity.Y -= batGravity / 16
		}
	case lava.Depth > 0:
		if lava.Depth <= batLavaJumpThreshold {
			entity.Living.Velocity.X *= 0.5
			entity.Living.Velocity.Y *= 0.8
			entity.Living.Velocity.Z *= 0.5

			if entity.Living.Velocity.Y <= 0 {
				entity.Living.Velocity.Y -= batGravity / 16
			}
		} else {
			entity.Living.Velocity.X *= 0.5
			entity.Living.Velocity.Y *= 0.5
			entity.Living.Velocity.Z *= 0.5
		}

		entity.Living.Velocity.Y -= batGravity / 4
	default:
		entity.Living.Velocity.Y -= batGravity
		entity.Living.Velocity.X *= 0.91
		entity.Living.Velocity.Y *= 0.98
		entity.Living.Velocity.Z *= 0.91
	}

	if entity.Resting {
		entity.Living.Velocity = game.Velocity{}
		entity.State.Position.Y = math.Floor(entity.State.Position.Y) + 1 - entity.Living.Height
	} else {
		entity.Living.Velocity.Y *= batVerticalDamping
	}

	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)
}

func (runtime *Runtime) tickBatAmbientSound(entity *runtimeBatEntity) {
	selection := runtimeMobRandomInt(runtime, 1000)

	entity.State.mu.Lock()
	ambientSoundTime := entity.AmbientSoundTime
	entity.AmbientSoundTime++
	play := selection < int(ambientSoundTime)

	if play {
		entity.AmbientSoundTime = -batAmbientSoundResetTicks
		play = !entity.Resting || runtimeMobRandomInt(runtime, 4) == 0
	}

	entity.State.mu.Unlock()

	if play {
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityBatAmbient, 0.1, 0.95)
	}
}

func (runtime *Runtime) batPlayerNearby(position game.Position) bool {
	maximumDistanceSquared := float64(batDisturbanceRange * batDisturbanceRange)

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if player.Dead || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator {
			continue
		}

		if distanceSquared(position, player.Position) <= maximumDistanceSquared {
			return true
		}
	}

	return false
}

func (runtime *Runtime) broadcastBatTakeoff(position game.BlockPosition) {
	event := protocol.LevelEvent{Event: protocol.LevelEventBatTakeoff, Position: position}

	for _, session := range runtime.sessionView() {
		err := session.sendLevelEventIfLoaded(event)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to send bat takeoff event: %v\n", err)
		}
	}
}

func batBlockPosition(position game.Position) game.BlockPosition {
	return game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
}

func batTargetBlockEmpty(block game.Block) bool {
	return block == game.Air || block == game.CaveAir || block == game.VoidAir
}

func batTargetNearPosition(target game.BlockPosition, position game.Position) bool {
	deltaX := float64(target.X) + 0.5 - position.X
	deltaY := float64(target.Y) + 0.5 - position.Y
	deltaZ := float64(target.Z) + 0.5 - position.Z

	return deltaX*deltaX+deltaY*deltaY+deltaZ*deltaZ < 4
}

func batSign(value float64) float64 {
	switch {
	case value < 0:
		return -1
	case value > 0:
		return 1
	default:
		return 0
	}
}
