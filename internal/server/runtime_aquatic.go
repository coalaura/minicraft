package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	aquaticMaximumAirSupply   = int32(300)
	aquaticDrowningThreshold  = int32(-20)
	aquaticDrowningDamage     = float32(2)
	aquaticMovementSpeed      = float32(0.7)
	aquaticMovementInputScale = float32(0.01)
	aquaticMoveDistanceScale  = 0.6
	aquaticSwimVolumeScale    = 0.35
	aquaticVelocityDrag       = 0.9
	aquaticIdleSink           = 0.005
	aquaticEyeBuoyancy        = 0.005
	aquaticVerticalSteering   = 0.1
	aquaticSpeedLerp          = float32(0.125)
	aquaticMaximumTurn        = float32(90)
	aquaticGravity            = 0.08
	aquaticAirDrag            = 0.98
	aquaticGroundFriction     = 0.91
	aquaticFlopHorizontal     = 0.05
	aquaticFlopVertical       = 0.4
	aquaticTrackingRange      = 4
	aquaticTrackingInterval   = 3
	aquaticAmbientInterval    = 120
	aquaticFollowRange        = 16
	aquaticBoneMealChance     = 0.05
	aquaticDefaultSchoolSize  = 8
	salmonMaximumSchoolSize   = 5
)

type runtimeAquaticSpec struct {
	EntityType   game.EntityType
	EyeHeight    float64
	AmbientSound game.SoundEvent
	HurtSound    game.SoundEvent
	DeathSound   game.SoundEvent
	FlopSound    game.SoundEvent
	RawDrop      game.Item
	CookedDrop   game.Item
	SchoolSize   int
}

type aquaticMoveControl struct {
	Wanted        game.Position
	SpeedModifier float64
	Speed         float32
	Moving        bool
}

type aquaticSchoolState struct {
	Leader     RuntimeEntity
	SchoolSize int
}

type runtimeAquatic struct {
	State  RuntimeEntityState
	Living RuntimeLivingState
	RuntimeMobState
	Rotation game.Rotation

	Navigation  swimNavigation
	MoveControl aquaticMoveControl
	School      aquaticSchoolState
	Goals       runtimeGoalSelector
	Spec        *runtimeAquaticSpec

	AirSupply         int32
	TickCount         int32
	AmbientSoundTime  int32
	FromBucket        bool
	VerticalCollision bool
	LootDropped       bool
}

type runtimeCodEntity struct {
	runtimeAquatic
}

type runtimeSalmonEntity struct {
	runtimeAquatic
	Variant int32
}

type runtimeTropicalFishEntity struct {
	runtimeAquatic
	Variant int32
}

var codAquaticSpec = runtimeAquaticSpec{
	EntityType:   game.EntityCod,
	EyeHeight:    0.195,
	AmbientSound: game.SoundEntityCodAmbient,
	HurtSound:    game.SoundEntityCodHurt,
	DeathSound:   game.SoundEntityCodDeath,
	FlopSound:    game.SoundEntityCodFlop,
	RawDrop:      game.ItemCod,
	CookedDrop:   game.ItemCookedCod,
	SchoolSize:   aquaticDefaultSchoolSize,
}

var salmonAquaticSpec = runtimeAquaticSpec{
	EntityType:   game.EntitySalmon,
	EyeHeight:    0.26,
	AmbientSound: game.SoundEntitySalmonAmbient,
	HurtSound:    game.SoundEntitySalmonHurt,
	DeathSound:   game.SoundEntitySalmonDeath,
	FlopSound:    game.SoundEntitySalmonFlop,
	RawDrop:      game.ItemSalmon,
	CookedDrop:   game.ItemCookedSalmon,
	SchoolSize:   salmonMaximumSchoolSize,
}

var tropicalFishAquaticSpec = runtimeAquaticSpec{
	EntityType:   game.EntityTropicalFish,
	EyeHeight:    0.26,
	AmbientSound: game.SoundEntityTropicalFishAmbient,
	HurtSound:    game.SoundEntityTropicalFishHurt,
	DeathSound:   game.SoundEntityTropicalFishDeath,
	FlopSound:    game.SoundEntityTropicalFishFlop,
	RawDrop:      game.ItemTropicalFish,
	SchoolSize:   aquaticDefaultSchoolSize,
}

func (entity *runtimeAquatic) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeAquatic) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (entity *runtimeAquatic) RuntimeLivingEyeHeight() float64 {
	return entity.Spec.EyeHeight
}

func (entity *runtimeAquatic) RuntimeMob() *RuntimeMobState {
	return &entity.RuntimeMobState
}

func (entity *runtimeAquatic) RuntimeMobDespawnConfig() RuntimeMobDespawnConfig {
	return RuntimeMobDespawnConfig{
		NoDespawnDistance: runtimeMobNoDespawnDistance,
		DespawnDistance:   runtimeMobDespawnDistance,
		AllowedInPeaceful: true,
	}
}

func (entity *runtimeAquatic) RuntimeMobRemoveWhenFarAway(float64) bool {
	return !entity.FromBucket
}

func (entity *runtimeAquatic) RuntimeMobRequiresCustomPersistence() bool {
	return entity.FromBucket
}

func (entity *runtimeAquatic) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: aquaticTrackingRange, UpdateInterval: aquaticTrackingInterval, TrackDeltas: true}
}

func (entity *runtimeAquatic) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeAquatic) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.entityMetadataLocked()
}

func (entity *runtimeAquatic) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (entity *runtimeAquatic) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  snapshot.ID,
		UUID:      snapshot.UUID,
		Type:      int32(entity.Spec.EntityType),
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

func (entity *runtimeAquatic) RuntimeLivingDamageSound(died bool) (game.SoundEvent, float32, float32) {
	if died {
		return entity.Spec.DeathSound, 1, 1
	}

	entity.State.mu.Lock()
	entity.AmbientSoundTime = -aquaticAmbientInterval
	entity.State.mu.Unlock()

	return entity.Spec.HurtSound, 1, 1
}

func (entity *runtimeAquatic) RuntimeEntitySoundSource() int32 {
	return protocol.SoundSourceNeutral
}

func (entity *runtimeAquatic) RuntimeLivingDied(runtime *Runtime) {
	entity.State.mu.Lock()

	if entity.LootDropped {
		entity.State.mu.Unlock()

		return
	}

	entity.LootDropped = true
	position := entity.State.Position
	burning := entity.Living.RemainingFireTicks > 0
	entity.State.mu.Unlock()

	drop := entity.Spec.RawDrop

	if burning && entity.Spec.CookedDrop != game.ItemAir {
		drop = entity.Spec.CookedDrop
	}

	spawnAnimalDrop(runtime, position, drop, 1)

	if runtime.nextEntityRandom() < aquaticBoneMealChance {
		spawnAnimalDrop(runtime, position, game.ItemBoneMeal, 1)
	}
}

func (entity *runtimeCodEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickAquatic(entity, &entity.runtimeAquatic)
}

func (entity *runtimeSalmonEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickAquatic(entity, &entity.runtimeAquatic)
}

func (entity *runtimeSalmonEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	metadata := entity.entityMetadataLocked()

	return append(metadata, protocol.EntityMetadataEntry{Index: protocol.FishVariantMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.Variant)})
}

func (entity *runtimeTropicalFishEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickAquatic(entity, &entity.runtimeAquatic)
}

func (entity *runtimeTropicalFishEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	metadata := entity.entityMetadataLocked()

	return append(metadata, protocol.EntityMetadataEntry{Index: protocol.FishVariantMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.Variant)})
}

func (entity *runtimeAquatic) runtimeEntityViewLocked() runtimeEntityView {
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

func (entity *runtimeAquatic) entityMetadataLocked() []protocol.EntityMetadataEntry {
	metadata := runtimeMobMetadata(entity.Living.EntityFlags(), entity.Living.Health, 0)
	metadata = append(metadata,
		protocol.EntityMetadataEntry{Index: protocol.EntityAirMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.AirSupply)},
		protocol.EntityMetadataEntry{Index: protocol.FishFromBucketMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(entity.FromBucket)},
	)

	return metadata
}

func (runtime *Runtime) SpawnCod(position game.Position) *runtimeCodEntity {
	entity := &runtimeCodEntity{}
	if !runtime.initializeAquatic(&entity.runtimeAquatic, &codAquaticSpec) {
		return nil
	}

	runtime.configureAquaticGoals(entity, &entity.runtimeAquatic)
	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) SpawnSalmon(position game.Position) *runtimeSalmonEntity {
	entity := &runtimeSalmonEntity{Variant: 1}
	if !runtime.initializeAquatic(&entity.runtimeAquatic, &salmonAquaticSpec) {
		return nil
	}

	runtime.configureAquaticGoals(entity, &entity.runtimeAquatic)
	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) SpawnTropicalFish(position game.Position) *runtimeTropicalFishEntity {
	entity := &runtimeTropicalFishEntity{}
	if !runtime.initializeAquatic(&entity.runtimeAquatic, &tropicalFishAquaticSpec) {
		return nil
	}

	runtime.configureAquaticGoals(entity, &entity.runtimeAquatic)
	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func (runtime *Runtime) initializeAquatic(entity *runtimeAquatic, specification *runtimeAquaticSpec) bool {
	definition, valid := specification.EntityType.Definition()
	if !valid {
		return false
	}

	entity.Spec = specification
	entity.AirSupply = aquaticMaximumAirSupply
	entity.School.SchoolSize = 1
	entity.Living = RuntimeLivingState{Width: definition.Width, Height: definition.Height, NextStepDistance: 1}
	entity.Living.Reset(3)

	return true
}

func (runtime *Runtime) tickAquatic(concrete RuntimeEntity, entity *runtimeAquatic) {
	livingEntity := concrete.(RuntimeLivingEntity)

	entity.State.mu.RLock()
	removed := entity.State.Removed
	dead := entity.Living.Dead
	entity.State.mu.RUnlock()

	if removed {
		return
	}

	if dead {
		runtime.tickRuntimeLivingEntity(livingEntity)

		return
	}

	mob := concrete.(RuntimeMobEntity)
	if runtime.checkRuntimeMobDespawn(mob) {
		return
	}

	entity.State.mu.Lock()
	entity.TickCount++
	entity.NoActionTime++
	tickCount := entity.TickCount
	entityID := entity.State.ID
	entity.State.mu.Unlock()

	runtime.tickRuntimeLivingBaseEnvironment(livingEntity)
	runtime.tickAquaticAir(livingEntity, entity)

	if runtimeLivingDead(livingEntity) {
		runtime.tickRuntimeLivingEntity(livingEntity)
		runtime.synchronizeRuntimeEntity(concrete)

		return
	}

	fullGoalTick := tickCount <= 1 || (tickCount+entityID)%2 == 0
	entity.Goals.Tick(runtime, fullGoalTick)

	runtime.tickAquaticMovement(concrete, entity)
	runtime.tickRuntimeLivingBlockEnvironment(livingEntity)
	runtime.tickAquaticAmbientSound(concrete, entity)
	runtime.tickRuntimeLivingEntity(livingEntity)
	runtime.synchronizeRuntimeEntity(concrete)
}

func (runtime *Runtime) tickAquaticAir(concrete RuntimeLivingEntity, entity *runtimeAquatic) {
	entity.State.mu.Lock()
	box := entity.Living.CollisionBox(entity.State.Position)
	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0
	drown := false

	if inWater {
		if entity.AirSupply != aquaticMaximumAirSupply {
			entity.State.metadataDirty = true
		}

		entity.AirSupply = aquaticMaximumAirSupply
	} else {
		entity.AirSupply--
		entity.State.metadataDirty = true

		if entity.AirSupply == aquaticDrowningThreshold {
			entity.AirSupply = 0
			drown = true
		}
	}

	entity.State.mu.Unlock()

	if drown {
		runtime.applyRuntimeLivingEnvironmentDamage(concrete, game.Damage{Type: game.DamageDrown, Amount: aquaticDrowningDamage})
	}
}

func (runtime *Runtime) tickAquaticMovement(concrete RuntimeEntity, entity *runtimeAquatic) {
	entity.State.mu.Lock()
	previous := entity.State.Position
	box := entity.Living.CollisionBox(previous)
	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0

	if !inWater && entity.Living.OnGround && entity.VerticalCollision {
		entity.Living.Velocity.X += (float64(runtime.nextEntityRandom())*2 - 1) * aquaticFlopHorizontal
		entity.Living.Velocity.Y += aquaticFlopVertical
		entity.Living.Velocity.Z += (float64(runtime.nextEntityRandom())*2 - 1) * aquaticFlopHorizontal
		entity.Living.OnGround = false
		entity.State.movementSyncDirty = true

		entity.State.mu.Unlock()

		runtime.broadcastRuntimeEntitySound(concrete, entity.Spec.FlopSound, 1, 1)

		entity.State.mu.Lock()
	}

	if inWater {
		runtime.applyAquaticMoveControl(entity)
		runtime.applyAquaticWaterPhysics(entity)
	} else {
		runtime.applyAquaticLandPhysics(entity)
	}

	deltaX := entity.State.Position.X - previous.X
	deltaZ := entity.State.Position.Z - previous.Z
	entity.Living.MoveDistance += float32(math.Hypot(deltaX, deltaZ) * aquaticMoveDistanceScale)

	playSwimSound := inWater && entity.Living.MoveDistance > float32(entity.Living.NextStepDistance)
	if playSwimSound {
		entity.Living.NextStepDistance = int32(entity.Living.MoveDistance) + 1
	}

	velocity := entity.Living.Velocity
	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(concrete, previous)

	if playSwimSound {
		volume := float32(math.Sqrt(velocity.X*velocity.X*0.2+velocity.Y*velocity.Y+velocity.Z*velocity.Z*0.2)) * aquaticSwimVolumeScale
		volume = min(volume, 1)
		pitch := 1 + (runtime.nextEntityRandom()-runtime.nextEntityRandom())*0.4
		runtime.broadcastRuntimeEntitySound(concrete, game.SoundEntityFishSwim, volume, pitch)
	}
}

func (runtime *Runtime) applyAquaticMoveControl(entity *runtimeAquatic) {
	eye := entity.State.Position
	eye.Y += entity.Spec.EyeHeight

	if runtime.positionInWater(eye) {
		entity.Living.Velocity.Y += aquaticEyeBuoyancy
	}

	wanted, moving := runtime.advanceSwimNavigation(entity.State.Position, entity.Living.Width, entity.Living.Height, &entity.Navigation)
	entity.MoveControl.Moving = moving

	if !moving {
		entity.MoveControl.Speed = 0

		return
	}

	entity.MoveControl.Wanted = wanted
	entity.MoveControl.SpeedModifier = entity.Navigation.Speed
	targetSpeed := float32(entity.Navigation.Speed) * aquaticMovementSpeed
	entity.MoveControl.Speed += (targetSpeed - entity.MoveControl.Speed) * aquaticSpeedLerp

	deltaX := wanted.X - entity.State.Position.X
	deltaY := wanted.Y - entity.State.Position.Y
	deltaZ := wanted.Z - entity.State.Position.Z
	distance := math.Sqrt(deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ)

	if deltaY != 0 && distance != 0 {
		entity.Living.Velocity.Y += float64(entity.MoveControl.Speed) * deltaY / distance * aquaticVerticalSteering
	}

	if deltaX != 0 || deltaZ != 0 {
		wantedYaw := float32(math.Atan2(deltaZ, deltaX)*180/math.Pi - 90)
		entity.Rotation.Yaw = rotateTowards(entity.Rotation.Yaw, wantedYaw, aquaticMaximumTurn)
		entity.Rotation.HeadYaw = entity.Rotation.Yaw
	}
}

func (runtime *Runtime) applyAquaticWaterPhysics(entity *runtimeAquatic) {
	if entity.MoveControl.Speed != 0 {
		yaw := float64(entity.Rotation.Yaw) * math.Pi / 180
		acceleration := float64(aquaticMovementInputScale * entity.MoveControl.Speed)

		entity.Living.Velocity.X += -float64(minecraftSin(yaw)) * acceleration
		entity.Living.Velocity.Z += float64(minecraftCos(yaw)) * acceleration
	}

	movement := runtime.moveGroundEntity(entity.State.Position, entity.Living.Velocity, entity.Living.Width, entity.Living.Height, 0, entity.Living.OnGround)

	entity.State.Position = movement.Position
	entity.Living.OnGround = movement.OnGround
	entity.VerticalCollision = movement.VerticalCollision

	if movement.HorizontalCollisionX {
		entity.Living.Velocity.X = 0
	}

	if movement.HorizontalCollisionZ {
		entity.Living.Velocity.Z = 0
	}

	if movement.VerticalCollision {
		entity.Living.Velocity.Y = 0
	}

	entity.Living.Velocity.X *= aquaticVelocityDrag
	entity.Living.Velocity.Y *= aquaticVelocityDrag
	entity.Living.Velocity.Z *= aquaticVelocityDrag

	if entity.Navigation.Done() {
		entity.Living.Velocity.Y -= aquaticIdleSink
	}
}

func (runtime *Runtime) applyAquaticLandPhysics(entity *runtimeAquatic) {
	movement := runtime.moveGroundEntity(entity.State.Position, entity.Living.Velocity, entity.Living.Width, entity.Living.Height, 0, entity.Living.OnGround)

	entity.State.Position = movement.Position
	entity.Living.OnGround = movement.OnGround
	entity.VerticalCollision = movement.VerticalCollision

	if movement.HorizontalCollisionX {
		entity.Living.Velocity.X = 0
	}

	if movement.HorizontalCollisionZ {
		entity.Living.Velocity.Z = 0
	}

	if movement.VerticalCollision {
		entity.Living.Velocity.Y = 0
	}

	entity.Living.Velocity.X *= aquaticGroundFriction
	entity.Living.Velocity.Y = (entity.Living.Velocity.Y - aquaticGravity) * aquaticAirDrag
	entity.Living.Velocity.Z *= aquaticGroundFriction
}

func (runtime *Runtime) tickAquaticAmbientSound(concrete RuntimeEntity, entity *runtimeAquatic) {
	selection := runtimeMobRandomInt(runtime, 1000)

	entity.State.mu.Lock()
	ambientSoundTime := entity.AmbientSoundTime
	entity.AmbientSoundTime++
	play := selection < int(ambientSoundTime)

	if play {
		entity.AmbientSoundTime = -aquaticAmbientInterval
	}

	entity.State.mu.Unlock()

	if play {
		runtime.broadcastRuntimeEntitySound(concrete, entity.Spec.AmbientSound, 1, 1)
	}
}

func (runtime *Runtime) positionInWater(position game.Position) bool {
	blockPosition := game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
	state := runtime.World.FluidAt(blockPosition)

	return state.Type() == game.FluidTypeWater && position.Y < float64(blockPosition.Y)+state.Height(runtime.World, blockPosition)
}
