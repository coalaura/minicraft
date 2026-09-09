package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	creeperMaxHealth           = 20
	creeperMovementSpeed       = float32(0.25)
	creeperEyeHeight           = 1.445
	creeperFollowRange         = 32
	creeperTrackingRangeChunks = 8
	creeperTrackingInterval    = 3
	creeperFuseTime            = 30
	creeperExplosionRadius     = float32(3)
	creeperPoweredRadius       = float32(6)
	creeperTargetInterval      = (10 + 1) / 2
	creeperNearestUnseenChecks = 30
	creeperRetaliateUnseen     = 150
	creeperStrollInterval      = 60
	creeperLookDistance        = 8
)

type creeperTargetState struct {
	Target               runtimeLivingTarget
	UnseenChecks         int32
	Retaliating          bool
	LastRetaliationStamp int64
	HasRetaliationStamp  bool
}

type creeperSwellState struct {
	Target runtimeLivingTarget
}

type creeperMeleeState struct {
	RepathTime int32
}

type creeperStrollState struct {
	Active bool
}

type creeperLookState struct {
	Target runtimeLivingTarget
	Ticks  int32
}

type creeperSwellGoal struct {
	Entity *runtimeCreeperEntity
}

type creeperMeleeGoal struct {
	Entity *runtimeCreeperEntity
}

type creeperStrollGoal struct {
	Entity *runtimeCreeperEntity
}

type creeperLookPlayerGoal struct {
	Entity *runtimeCreeperEntity
}

type creeperRandomLookGoal struct {
	Entity *runtimeCreeperEntity
}

type runtimeCreeperEntity struct {
	State RuntimeEntityState

	Living RuntimeLivingState
	RuntimeMobState
	Rotation game.Rotation

	Navigation  groundNavigationState
	MoveControl groundMoveControlState
	LookControl groundLookControlState
	BodyControl groundBodyRotationState
	Target      creeperTargetState
	Swell       creeperSwellState
	Melee       creeperMeleeState
	Stroll      creeperStrollState
	PlayerLook  creeperLookState
	RandomLook  creeperLookState
	Goals       runtimeGoalSelector
	GoalTarget  runtimeLivingTarget

	TickCount      int32
	FullGoalTick   bool
	SwellOld       int32
	SwellCurrent   int32
	SwellDirection int32
	Powered        bool
	Ignited        bool
	LootDropped    bool
	Exploded       bool
}

func (goal *creeperSwellGoal) CanUse(*Runtime) bool {
	entity := goal.Entity
	return entity.SwellDirection > 0 || entity.GoalTarget.present() && distanceSquared(entity.State.Position, entity.GoalTarget.position()) < 9
}

func (goal *creeperSwellGoal) CanContinue(runtime *Runtime) bool { return goal.CanUse(runtime) }

func (goal *creeperSwellGoal) Start(*Runtime) {
	goal.Entity.Swell.Target = goal.Entity.GoalTarget
	goal.Entity.Navigation.Stop()
}

func (goal *creeperSwellGoal) Stop(*Runtime) { goal.Entity.Swell.Target = runtimeLivingTarget{} }

func (goal *creeperSwellGoal) Tick(runtime *Runtime) {
	entity := goal.Entity
	target := entity.Swell.Target

	if !target.present() {
		entity.setSwellDirection(-1)

		return
	}

	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	if distanceSquared(position, target.position()) > 49 || !target.hasLineOfSight(runtime, position, creeperEyeHeight) {
		entity.setSwellDirection(-1)

		return
	}

	entity.setSwellDirection(1)
}

func (*creeperSwellGoal) RequiresUpdateEveryTick() bool {
	return true
}

func (goal *creeperMeleeGoal) CanUse(*Runtime) bool {
	return goal.Entity.GoalTarget.present()
}

func (goal *creeperMeleeGoal) CanContinue(*Runtime) bool {
	return goal.Entity.GoalTarget.present()
}

func (goal *creeperMeleeGoal) Start(*Runtime) {
	goal.Entity.Stroll.Active = false
}

func (goal *creeperMeleeGoal) Stop(*Runtime) {
	goal.Entity.Navigation.Stop()

	goal.Entity.Melee = creeperMeleeState{}
}

func (goal *creeperMeleeGoal) Tick(runtime *Runtime) {
	entity := goal.Entity
	target := entity.GoalTarget

	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	entity.LookControl.SetWanted(target.eyePosition(), 30, 30)

	entity.Melee.RepathTime--

	if entity.Melee.RepathTime > 0 {
		return
	}

	path := runtime.findGroundPathInto(entity.Navigation.Path[:0], position, target.position(), entity.Living.Width, entity.Living.Height, creeperFollowRange)

	entity.Navigation.MoveTo(path, 1)

	entity.Melee.RepathTime = int32(4 + creeperRandomInt(runtime, 7))
}

func (*creeperMeleeGoal) RequiresUpdateEveryTick() bool {
	return true
}

func (goal *creeperStrollGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Entity
	if entity.GoalTarget.present() || entity.NoActionTime >= 100 || creeperRandomInt(runtime, creeperStrollInterval) != 0 {
		return false
	}

	path := entity.randomStrollPath(runtime)
	return entity.Navigation.MoveTo(path, 0.8)
}

func (goal *creeperStrollGoal) CanContinue(*Runtime) bool {
	return !goal.Entity.Navigation.Done()
}

func (goal *creeperStrollGoal) Start(*Runtime) {
	goal.Entity.Stroll.Active = true
}

func (goal *creeperStrollGoal) Stop(*Runtime) {
	goal.Entity.Stroll.Active = false
}

func (*creeperStrollGoal) Tick(*Runtime) {}

func (goal *creeperLookPlayerGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Entity

	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	target := runtime.nearestValidPlayer(position, creeperEyeHeight, creeperLookDistance)

	if target == nil {
		return false
	}

	entity.PlayerLook.Target = runtimeLivingTarget{session: target}
	entity.PlayerLook.Ticks = int32(20 + creeperRandomInt(runtime, 20))

	return true
}

func (goal *creeperLookPlayerGoal) CanContinue(*Runtime) bool {
	return goal.Entity.PlayerLook.Ticks > 0
}

func (*creeperLookPlayerGoal) Start(*Runtime) {}

func (goal *creeperLookPlayerGoal) Stop(*Runtime) {
	goal.Entity.PlayerLook = creeperLookState{}
}

func (goal *creeperLookPlayerGoal) Tick(runtime *Runtime) {
	state := &goal.Entity.PlayerLook
	if !state.Target.present() {
		state.Ticks = 0

		return
	}

	goal.Entity.LookControl.SetWanted(state.Target.eyePosition(), 10, 40)

	state.Ticks--
}

func (goal *creeperRandomLookGoal) CanUse(runtime *Runtime) bool {
	if runtime.nextEntityRandom() >= 0.02 {
		return false
	}

	goal.Entity.RandomLook.Ticks = int32(20 + creeperRandomInt(runtime, 20))

	return true
}

func (goal *creeperRandomLookGoal) CanContinue(*Runtime) bool {
	return goal.Entity.RandomLook.Ticks > 0
}

func (*creeperRandomLookGoal) Start(*Runtime) {}

func (goal *creeperRandomLookGoal) Stop(*Runtime) {
	goal.Entity.RandomLook = creeperLookState{}
}

func (goal *creeperRandomLookGoal) Tick(runtime *Runtime) {
	entity := goal.Entity
	angle := float64(runtime.nextEntityRandom()) * math.Pi * 2

	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	position.X += math.Cos(angle)
	position.Y += creeperEyeHeight
	position.Z += math.Sin(angle)

	entity.LookControl.SetWanted(position, 10, 40)

	entity.RandomLook.Ticks--
}

func (entity *runtimeCreeperEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeCreeperEntity) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (*runtimeCreeperEntity) RuntimeLivingEyeHeight() float64 {
	return creeperEyeHeight
}

func (entity *runtimeCreeperEntity) RuntimeMob() *RuntimeMobState {
	return &entity.RuntimeMobState
}

func (*runtimeCreeperEntity) RuntimeMobDespawnConfig() RuntimeMobDespawnConfig {
	return RuntimeMobDespawnConfig{NoDespawnDistance: runtimeMobNoDespawnDistance, DespawnDistance: runtimeMobDespawnDistance}
}

func (*runtimeCreeperEntity) RuntimeMobRemoveWhenFarAway(float64) bool {
	return true
}

func (*runtimeCreeperEntity) RuntimeMobRequiresCustomPersistence() bool {
	return false
}

func (*runtimeCreeperEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: creeperTrackingRangeChunks, UpdateInterval: creeperTrackingInterval, TrackDeltas: true}
}

func (entity *runtimeCreeperEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeCreeperEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{ID: entity.State.ID, UUID: entity.State.UUID, Position: entity.State.Position, Chunk: entity.State.Chunk, Removed: entity.State.Removed, Velocity: entity.Living.Velocity, Rotation: entity.Rotation, OnGround: entity.Living.OnGround}
}

func (entity *runtimeCreeperEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return []protocol.EntityMetadataEntry{
		{Index: protocol.EntityFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(entity.Living.EntityFlags())},
		{Index: protocol.LivingHealthMetadataIndex, Type: protocol.MetadataTypeFloat, Value: protocol.MetadataFloat(entity.Living.Health)},
		{Index: protocol.MobFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(0)},
		{Index: protocol.CreeperSwellMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.SwellDirection)},
		{Index: protocol.CreeperPoweredMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(entity.Powered)},
		{Index: protocol.CreeperIgnitedMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(entity.Ignited)},
	}
}

func (entity *runtimeCreeperEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (entity *runtimeCreeperEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{EntityID: snapshot.ID, UUID: snapshot.UUID, Type: int32(game.EntityCreeper), X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, VelocityX: snapshot.Velocity.X, VelocityY: snapshot.Velocity.Y, VelocityZ: snapshot.Velocity.Z, Yaw: snapshot.Yaw, Pitch: snapshot.Pitch, HeadYaw: snapshot.HeadYaw}
}

func (*runtimeCreeperEntity) RuntimeEntitySoundSource() int32 {
	return protocol.SoundSourceHostile
}

func (entity *runtimeCreeperEntity) RuntimeLivingDamageSound(died bool) (game.SoundEvent, float32, float32) {
	if died {
		return game.SoundEntityCreeperDeath, 1, 1
	}

	return game.SoundEntityCreeperHurt, 1, 1
}

func (entity *runtimeCreeperEntity) RuntimeLivingDied(runtime *Runtime) {
	entity.State.mu.Lock()

	if entity.LootDropped || entity.Exploded {
		entity.State.mu.Unlock()

		return
	}

	entity.LootDropped = true
	position := entity.State.Position
	entity.State.mu.Unlock()

	count := int32(creeperRandomInt(runtime, 3))

	if count > 0 {
		runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemGunpowder, Count: count}, position, game.Velocity{}, 10)
	}
}

func (entity *runtimeCreeperEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	entity.State.mu.RLock()
	removed := entity.State.Removed
	dead := entity.Living.Dead
	entity.State.mu.RUnlock()

	if removed {
		return
	}

	if dead && runtime.Difficulty != game.DifficultyPeaceful {
		runtime.tickRuntimeLivingEntity(entity)

		return
	}

	if runtime.checkRuntimeMobDespawn(entity) {
		return
	}

	entity.tickFuse(runtime)

	entity.State.mu.RLock()
	removed = entity.State.Removed
	entity.State.mu.RUnlock()

	if removed {
		return
	}

	entity.State.mu.Lock()
	entity.TickCount++
	entity.NoActionTime++

	tickCount := entity.TickCount
	entityID := entity.State.ID
	entity.State.mu.Unlock()

	runtime.tickRuntimeLivingBaseEnvironment(entity)

	if entity.dead() {
		runtime.tickRuntimeLivingEntity(entity)
		runtime.synchronizeRuntimeEntity(entity)

		return
	}

	fullGoalTick := tickCount <= 1 || (tickCount+entityID)%2 == 0

	entity.tickTarget(runtime, fullGoalTick)

	entity.GoalTarget = entity.Target.Target
	entity.FullGoalTick = fullGoalTick

	entity.Goals.Tick(runtime, fullGoalTick)

	entity.State.mu.RLock()
	fallDistance := entity.Living.FallDistance
	entity.State.mu.RUnlock()

	fallDamage, _ := runtime.tickGroundMobMovement(entity, &entity.Living, &entity.Rotation, &entity.Navigation, &entity.MoveControl, &entity.LookControl, &entity.BodyControl, creeperGroundControlConfig(), false)

	if fallDamage > 0 {
		entity.setSwellCurrent(min(creeperFuseTime-5, entity.SwellCurrent+int32(fallDistance*1.5)))
		entity.applyEnvironmentDamage(runtime, game.Damage{Type: game.DamageFall, Amount: fallDamage})
	}

	runtime.tickRuntimeLivingBlockEnvironment(entity)

	runtime.tickRuntimeLivingEntity(entity)
	runtime.synchronizeRuntimeEntity(entity)
}

func (entity *runtimeCreeperEntity) tickFuse(runtime *Runtime) {
	entity.State.mu.Lock()

	if entity.Ignited && entity.SwellDirection != 1 {
		entity.SwellDirection = 1
		entity.State.metadataDirty = true
	}

	entity.SwellOld = entity.SwellCurrent

	direction := entity.SwellDirection
	current := entity.SwellCurrent

	if current == 0 && direction > 0 {
		entity.State.mu.Unlock()

		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityCreeperPrimed, 1, 0.5)

		entity.State.mu.Lock()
	}

	entity.SwellCurrent = max(0, min(creeperFuseTime, current+direction))
	explode := entity.SwellCurrent >= creeperFuseTime && !entity.Exploded

	if explode {
		entity.Exploded = true
	}

	position := entity.State.Position
	entityID := entity.State.ID
	powered := entity.Powered
	entity.State.mu.Unlock()

	if !explode {
		return
	}

	radius := creeperExplosionRadius

	if powered {
		radius = creeperPoweredRadius
	}

	_, players, living := runtime.explodeLocked(RuntimeExplosion{Position: position, Radius: radius, DirectEntityID: entityID, CauseEntityID: entityID, BlockInteraction: ExplosionDestroyBlocksWithDecay})

	runtime.sendExplosionEntityUpdates(players, living)
	runtime.removeRuntimeEntity(entityID)
}

func (entity *runtimeCreeperEntity) dead() bool {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Dead
}

func (entity *runtimeCreeperEntity) setSwellDirection(direction int32) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if entity.SwellDirection != direction {
		entity.SwellDirection = direction
		entity.State.metadataDirty = true
	}
}

func (entity *runtimeCreeperEntity) setSwellCurrent(current int32) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	current = max(0, min(creeperFuseTime, current))
	if entity.SwellCurrent == current {
		return
	}

	entity.SwellCurrent = current
}

func (entity *runtimeCreeperEntity) applyEnvironmentDamage(runtime *Runtime, damage game.Damage) bool {
	update, applied := runtime.damageRuntimeLivingEntityLocked(entity, damage)

	if applied {
		runtime.sendRuntimeLivingDamageUpdate(update)
	}

	return applied
}

func (entity *runtimeCreeperEntity) tickTarget(runtime *Runtime, fullGoalTick bool) {
	entity.State.mu.RLock()
	position := entity.State.Position
	entityID := entity.State.ID

	damageType, causeID, hurt := entity.Living.LastDamageAt(runtime.World.Time().Age)

	_ = damageType

	damageStamp := entity.Living.LastDamageStamp
	entity.State.mu.RUnlock()

	newRetaliation := hurt && causeID != 0 && causeID != entityID && (!entity.Target.HasRetaliationStamp || damageStamp != entity.Target.LastRetaliationStamp)

	if newRetaliation {
		entity.Target.LastRetaliationStamp = damageStamp
		entity.Target.HasRetaliationStamp = true
		candidate := runtime.runtimeLivingTargetByID(causeID)

		if candidate.valid(runtime, position, creeperFollowRange) {
			entity.Target.Target = candidate
			entity.Target.UnseenChecks = 0
			entity.Target.Retaliating = true
		}
	}

	target := entity.Target.Target
	if target.present() && target.valid(runtime, position, creeperFollowRange) {
		if fullGoalTick {
			if target.hasLineOfSight(runtime, position, creeperEyeHeight) {
				entity.Target.UnseenChecks = 0
			} else {
				entity.Target.UnseenChecks++
			}
		}

		maximum := int32(creeperNearestUnseenChecks)

		if entity.Target.Retaliating {
			maximum = creeperRetaliateUnseen
		}

		if entity.Target.UnseenChecks <= maximum {
			return
		}
	}

	entity.Target.Target = runtimeLivingTarget{}
	entity.Target.UnseenChecks = 0
	entity.Target.Retaliating = false

	if !fullGoalTick || creeperRandomInt(runtime, creeperTargetInterval) != 0 {
		return
	}

	nearest := runtime.nearestValidPlayer(position, creeperEyeHeight, creeperFollowRange)
	if nearest != nil {
		entity.Target.Target = runtimeLivingTarget{session: nearest}
	}
}

func (entity *runtimeCreeperEntity) randomStrollPath(runtime *Runtime) []game.Position {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	var best []game.Position

	bestDistance := -1.0

	for range 10 {
		goal := game.Position{X: position.X + float64(creeperRandomInt(runtime, 21)-10), Y: position.Y + float64(creeperRandomInt(runtime, 15)-7), Z: position.Z + float64(creeperRandomInt(runtime, 21)-10)}

		path := runtime.findGroundPath(position, goal, entity.Living.Width, entity.Living.Height, creeperFollowRange)
		if len(path) == 0 {
			continue
		}

		distance := distanceSquared(position, path[len(path)-1])
		if distance > bestDistance {
			best = path
			bestDistance = distance
		}
	}

	return best
}

func (entity *runtimeCreeperEntity) RuntimeEntityInteract(runtime *Runtime, session *Session, interaction RuntimeEntityInteraction) bool {
	player := session.snapshotPlayer()

	held, valid := heldItemPointer(&player, interaction.Hand)
	if !valid || held.Empty() || (held.Item != game.ItemFlintAndSteel && held.Item != game.ItemFireCharge) {
		return false
	}

	sound := game.SoundItemFireChargeUse

	if held.Item == game.ItemFlintAndSteel {
		sound = game.SoundItemFlintAndSteelUse
	}

	entity.State.mu.Lock()

	if entity.State.Removed || entity.Living.Dead {
		entity.State.mu.Unlock()

		return false
	}

	entity.Ignited = true
	entity.State.metadataDirty = true
	entity.State.mu.Unlock()

	runtime.broadcastRuntimeEntitySoundFromSource(entity, sound, protocol.SoundSourcePlayer, 1, 0.8+runtime.nextEntityRandom()*0.4)

	if player.GameMode == game.GameModeCreative {
		return false
	}

	changed := false

	session.mutatePlayer(func(current *game.Player) bool {
		stack, found := heldItemPointer(current, interaction.Hand)

		if !found || !stack.SameItem(*held) {
			return false
		}

		if stack.Item == game.ItemFireCharge {
			stack.Count--

			if stack.Count == 0 {
				*stack = game.ItemStack{}
			}

			changed = true

			return true
		}

		definition, defined := stack.Item.Definition()

		if !defined {
			return false
		}

		_, changed = runtime.damageItemStack(stack, 1, definition.MaxDurability)
		return changed
	})

	return changed
}

func (runtime *Runtime) SpawnCreeper(position game.Position) *runtimeCreeperEntity {
	definition, valid := game.EntityCreeper.Definition()
	if !valid {
		return nil
	}

	entity := &runtimeCreeperEntity{Living: RuntimeLivingState{NextStepDistance: 1, Width: definition.Width, Height: definition.Height}}

	entity.Living.Reset(creeperMaxHealth)

	entity.SwellDirection = -1

	entity.Goals.Add(1, runtimeGoalJump, &groundMobFloatGoal{Entity: entity, EyeHeight: creeperEyeHeight, MoveControl: &entity.MoveControl})
	entity.Goals.Add(2, runtimeGoalMove, &creeperSwellGoal{Entity: entity})
	entity.Goals.Add(4, runtimeGoalMove|runtimeGoalLook, &creeperMeleeGoal{Entity: entity})
	entity.Goals.Add(5, runtimeGoalMove, &creeperStrollGoal{Entity: entity})
	entity.Goals.Add(6, runtimeGoalLook, &creeperLookPlayerGoal{Entity: entity})
	entity.Goals.Add(6, runtimeGoalLook, &creeperRandomLookGoal{Entity: entity})

	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func creeperRandomInt(runtime *Runtime, bound int) int {
	if bound <= 1 {
		return 0
	}

	return min(int(runtime.nextEntityRandom()*float32(bound)), bound-1)
}

func creeperGroundControlConfig() groundMobControlConfig {
	configuration := zombieGroundControlConfig()

	configuration.MovementSpeed = creeperMovementSpeed
	configuration.EyeHeight = creeperEyeHeight

	return configuration
}
