package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	skeletonMaxHealth              = 20
	skeletonMovementSpeed          = float32(0.25)
	skeletonAttackDamage           = float32(2)
	skeletonFollowRange            = 16
	skeletonEyeHeight              = 1.74
	skeletonTrackingRangeChunks    = 8
	skeletonTrackingInterval       = 3
	skeletonTargetInterval         = 10
	skeletonNearestUnseenChecks    = 30
	skeletonRetaliateUnseenChecks  = 150
	skeletonBowRangeSquared        = 225
	skeletonBowNearRangeSquared    = 56.25
	skeletonBowFarRangeSquared     = 168.75
	skeletonBowUseTicks            = 20
	skeletonBowHardCooldown        = 20
	skeletonBowNormalCooldown      = 40
	skeletonBowVelocity            = 1.6
	skeletonBowPathSpeed           = 1
	skeletonMeleePathSpeed         = 1.2
	skeletonMeleeAttackInterval    = 20
	skeletonFleeSunSpeed           = 1
	skeletonFleeSunAttempts        = 10
	skeletonAmbientSoundInterval   = 80
	skeletonStrollInterval         = 60
	skeletonRandomPositionAttempts = 10
	skeletonLootPickupDelay        = 10
	skeletonBowDropChance          = float32(0.085)
	skeletonFireDurationTicks      = 8 * 20
)

type skeletonTargetState struct {
	Target               runtimeLivingTarget
	UnseenChecks         int32
	Retaliating          bool
	LastRetaliationStamp int64
	HasRetaliationStamp  bool
}

type skeletonBowState struct {
	AttackTime      int32
	SeeTime         int32
	StrafeTime      int32
	StrafeClockwise bool
	StrafeForward   bool
}

type skeletonMeleeState struct {
	AttackTime int32
	RepathTime int32
}

type skeletonIdleState struct {
	Strolling bool
}

type skeletonSunState struct {
	Restricting bool
	FleePath    []game.Position
}

type skeletonRestrictSunGoal struct {
	Entity *runtimeSkeletonEntity
}

type skeletonFleeSunGoal struct {
	Entity *runtimeSkeletonEntity
}

type skeletonBowGoal struct {
	Entity *runtimeSkeletonEntity
}

type skeletonMeleeGoal struct {
	Entity *runtimeSkeletonEntity
}

type skeletonIdleGoal struct {
	Entity *runtimeSkeletonEntity
}

type runtimeSkeletonEntity struct {
	State RuntimeEntityState

	Living RuntimeLivingState
	RuntimeMobState
	Rotation game.Rotation

	Navigation  groundNavigationState
	MoveControl groundMoveControlState
	LookControl groundLookControlState
	BodyControl groundBodyRotationState
	Target      skeletonTargetState
	Bow         skeletonBowState
	Melee       skeletonMeleeState
	Idle        skeletonIdleState
	Sun         skeletonSunState
	Goals       runtimeGoalSelector
	GoalTarget  runtimeLivingTarget

	MainHand game.ItemStack
	Head     game.ItemStack

	TickCount        int32
	AmbientSoundTime int32
	FullGoalTick     bool
	Aggressive       bool
	LootDropped      bool
}

func (goal *skeletonRestrictSunGoal) CanUse(runtime *Runtime) bool {
	return goal.Entity.shouldRestrictSun(runtime)
}

func (goal *skeletonRestrictSunGoal) CanContinue(runtime *Runtime) bool {
	return goal.Entity.shouldRestrictSun(runtime)
}

func (goal *skeletonRestrictSunGoal) Start(*Runtime) {
	goal.Entity.Sun.Restricting = true
}

func (goal *skeletonRestrictSunGoal) Stop(*Runtime) {
	goal.Entity.Sun.Restricting = false
}

func (*skeletonRestrictSunGoal) Tick(*Runtime) {}

func (goal *skeletonFleeSunGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Entity
	if entity.GoalTarget.present() || !entity.shouldFleeSun(runtime) {
		return false
	}

	entity.Sun.FleePath = entity.findSunShelterPath(runtime)

	return len(entity.Sun.FleePath) > 0
}

func (goal *skeletonFleeSunGoal) CanContinue(*Runtime) bool {
	entity := goal.Entity

	return !entity.GoalTarget.present() && !entity.Navigation.Done()
}

func (goal *skeletonFleeSunGoal) Start(*Runtime) {
	entity := goal.Entity
	entity.stopIdle()
	entity.Navigation.MoveTo(entity.Sun.FleePath, skeletonFleeSunSpeed)
}

func (goal *skeletonFleeSunGoal) Stop(*Runtime) {
	goal.Entity.Sun.FleePath = nil
}

func (*skeletonFleeSunGoal) Tick(*Runtime) {}

func (goal *skeletonBowGoal) CanUse(*Runtime) bool {
	return goal.Entity.GoalTarget.present() && goal.Entity.holdingBow()
}

func (goal *skeletonBowGoal) CanContinue(*Runtime) bool {
	entity := goal.Entity

	return entity.GoalTarget.present() && entity.holdingBow()
}

func (goal *skeletonBowGoal) Start(*Runtime) {
	goal.Entity.stopIdle()
	goal.Entity.setAggressive(true)
}

func (goal *skeletonBowGoal) Stop(*Runtime) {
	entity := goal.Entity

	entity.Navigation.Stop()

	entity.stopUsingBow()

	entity.Bow.SeeTime = 0
	entity.Bow.StrafeTime = -1

	entity.setAggressive(false)
}

func (goal *skeletonBowGoal) Tick(runtime *Runtime) {
	goal.Entity.tickBowGoal(runtime)
}

func (goal *skeletonMeleeGoal) CanUse(*Runtime) bool {
	return goal.Entity.GoalTarget.present() && !goal.Entity.holdingBow()
}

func (goal *skeletonMeleeGoal) CanContinue(*Runtime) bool {
	entity := goal.Entity

	return entity.GoalTarget.present() && !entity.holdingBow()
}

func (goal *skeletonMeleeGoal) Start(*Runtime) {
	goal.Entity.stopIdle()
	goal.Entity.setAggressive(true)
}

func (goal *skeletonMeleeGoal) Stop(*Runtime) {
	entity := goal.Entity

	entity.Navigation.Stop()

	entity.Melee = skeletonMeleeState{}

	entity.setAggressive(false)
}

func (goal *skeletonMeleeGoal) Tick(runtime *Runtime) {
	goal.Entity.tickMeleeGoal(runtime)
}

func (goal *skeletonIdleGoal) CanUse(*Runtime) bool {
	return !goal.Entity.GoalTarget.present()
}

func (goal *skeletonIdleGoal) CanContinue(*Runtime) bool {
	return !goal.Entity.GoalTarget.present()
}

func (*skeletonIdleGoal) Start(*Runtime) {}

func (goal *skeletonIdleGoal) Stop(*Runtime) {
	goal.Entity.stopIdle()
}

func (goal *skeletonIdleGoal) Tick(runtime *Runtime) {
	goal.Entity.tickIdle(runtime)
}

func (entity *runtimeSkeletonEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeSkeletonEntity) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (*runtimeSkeletonEntity) RuntimeLivingEyeHeight() float64 {
	return skeletonEyeHeight
}

func (entity *runtimeSkeletonEntity) RuntimeMob() *RuntimeMobState {
	return &entity.RuntimeMobState
}

func (entity *runtimeSkeletonEntity) RuntimeMobDespawnConfig() RuntimeMobDespawnConfig {
	return RuntimeMobDespawnConfig{
		NoDespawnDistance: runtimeMobNoDespawnDistance,
		DespawnDistance:   runtimeMobDespawnDistance,
	}
}

func (entity *runtimeSkeletonEntity) RuntimeMobRemoveWhenFarAway(float64) bool {
	return true
}

func (entity *runtimeSkeletonEntity) RuntimeMobRequiresCustomPersistence() bool {
	return false
}

func (entity *runtimeSkeletonEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: skeletonTrackingRangeChunks, UpdateInterval: skeletonTrackingInterval, TrackDeltas: true}
}

func (entity *runtimeSkeletonEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeSkeletonEntity) runtimeEntityViewLocked() runtimeEntityView {
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

func (entity *runtimeSkeletonEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	mobFlags := byte(0)

	if entity.Aggressive {
		mobFlags |= protocol.MobFlagAggressive
	}

	return []protocol.EntityMetadataEntry{
		{Index: protocol.EntityFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(entity.Living.EntityFlags())},
		{Index: protocol.LivingFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(entity.Living.ItemUseFlags())},
		{Index: protocol.LivingHealthMetadataIndex, Type: protocol.MetadataTypeFloat, Value: protocol.MetadataFloat(entity.Living.Health)},
		{Index: protocol.MobFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(mobFlags)},
	}
}

func (entity *runtimeSkeletonEntity) EntityEquipment() []protocol.EquipmentEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return []protocol.EquipmentEntry{{Slot: protocol.EquipmentSlotMainHand, Item: entity.MainHand}}
}

func (entity *runtimeSkeletonEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (entity *runtimeSkeletonEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  snapshot.ID,
		UUID:      snapshot.UUID,
		Type:      int32(game.EntitySkeleton),
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

func (entity *runtimeSkeletonEntity) RuntimeEntitySoundSource() int32 {
	return protocol.SoundSourceHostile
}

func (entity *runtimeSkeletonEntity) RuntimeLivingDamageSound(died bool) (game.SoundEvent, float32, float32) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if died {
		return game.SoundEntitySkeletonDeath, 1, 1
	}

	entity.AmbientSoundTime = -skeletonAmbientSoundInterval

	return game.SoundEntitySkeletonHurt, 1, 1
}

func (entity *runtimeSkeletonEntity) RuntimeLivingDied(runtime *Runtime) {
	entity.State.mu.Lock()

	if entity.LootDropped {
		entity.State.mu.Unlock()

		return
	}

	entity.LootDropped = true
	position := entity.State.Position
	lastDamageCauseEntityID := entity.Living.LastDamageCauseEntityID
	entity.State.mu.Unlock()

	items := []game.Item{game.ItemArrow, game.ItemBone}

	for _, item := range items {
		count := int32(skeletonRandomInt(runtime, 3))
		if count <= 0 {
			continue
		}

		stack := game.ItemStack{Item: item, Count: count}

		runtime.SpawnItemEntity(stack, position, game.Velocity{}, skeletonLootPickupDelay)
	}

	killedByPlayer := runtime.runtimeLivingTargetByID(lastDamageCauseEntityID).session != nil

	if killedByPlayer && runtime.nextEntityRandom() < skeletonBowDropChance {
		stack := game.ItemStack{Item: game.ItemBow, Count: 1}

		runtime.SpawnItemEntity(stack, position, game.Velocity{}, skeletonLootPickupDelay)
	}
}

func (entity *runtimeSkeletonEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
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

	entity.State.mu.Lock()
	entity.TickCount++
	entity.NoActionTime++
	position := entity.State.Position
	entity.State.mu.Unlock()

	dayTime := floorMod(runtime.World.Time().DayTime, 24000)
	eyePosition := skeletonEyeBlockPosition(position)

	brightness, _ := zombieDaylightBrightness(runtime.World, eyePosition, dayTime)
	if brightness > 0.5 {
		entity.State.mu.Lock()
		entity.NoActionTime += 2
		entity.State.mu.Unlock()
	}

	runtime.tickRuntimeLivingBaseEnvironment(entity)

	entity.tickAmbientSound(runtime)
	entity.tickDaylightBurning(runtime)

	if entity.dead() {
		runtime.tickRuntimeLivingEntity(entity)
		runtime.synchronizeRuntimeEntity(entity)

		return
	}

	fullGoalTick := entity.TickCount <= 1 || (entity.TickCount+entity.State.ID)%2 == 0

	entity.tickTarget(runtime, fullGoalTick)

	entity.GoalTarget = entity.Target.Target
	entity.FullGoalTick = fullGoalTick

	entity.Goals.Tick(runtime)

	fallDamage, step := runtime.tickGroundMobMovement(entity, &entity.Living, &entity.Rotation, &entity.Navigation, &entity.MoveControl, &entity.LookControl, &entity.BodyControl, skeletonGroundControlConfig(), false)
	if fallDamage > 0 {
		entity.applyEnvironmentDamage(runtime, game.Damage{Type: game.DamageFall, Amount: fallDamage})
	}

	runtime.tickRuntimeLivingBlockEnvironment(entity)

	if step {
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntitySkeletonStep, 0.15, 1)
	}

	runtime.tickRuntimeLivingEntity(entity)
	runtime.synchronizeRuntimeEntity(entity)
}

func (entity *runtimeSkeletonEntity) dead() bool {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Dead
}

func (entity *runtimeSkeletonEntity) tickAmbientSound(runtime *Runtime) {
	selection := int32(skeletonRandomInt(runtime, 1000))

	entity.State.mu.Lock()
	ambientSoundTime := entity.AmbientSoundTime
	entity.AmbientSoundTime++

	play := selection < ambientSoundTime
	if play {
		entity.AmbientSoundTime = -skeletonAmbientSoundInterval
	}

	entity.State.mu.Unlock()

	if play {
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntitySkeletonAmbient, 1, 1)
	}
}

func (entity *runtimeSkeletonEntity) tickDaylightBurning(runtime *Runtime) {
	dayTime := floorMod(runtime.World.Time().DayTime, 24000)
	if dayTime > 12541 && dayTime < 23460 {
		return
	}

	entity.State.mu.RLock()
	position := entity.State.Position
	box := entity.Living.CollisionBox(position)

	headEmpty := entity.Head.Empty()
	entity.State.mu.RUnlock()

	eyePosition := skeletonEyeBlockPosition(position)

	brightness, seesSky := zombieDaylightBrightness(runtime.World, eyePosition, dayTime)
	if brightness <= 0.5 || runtime.nextEntityRandom()*30 >= (brightness-0.4)*2 {
		return
	}

	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0
	inPowderSnow := runtime.World.BlockAt(eyePosition) == game.PowderSnow

	if !headEmpty || inWater || inPowderSnow || !seesSky {
		return
	}

	entity.State.mu.Lock()
	wasBurning := entity.Living.RemainingFireTicks > 0
	entity.Living.RemainingFireTicks = max(entity.Living.RemainingFireTicks, skeletonFireDurationTicks)

	if !wasBurning {
		entity.State.metadataDirty = true
	}

	entity.State.mu.Unlock()
}

func (entity *runtimeSkeletonEntity) tickTarget(runtime *Runtime, fullGoalTick bool) {
	entity.State.mu.Lock()
	position := entity.State.Position
	entityID := entity.State.ID

	damageType, causeID, hurt := entity.Living.LastDamageAt(runtime.World.Time().Age)
	_ = damageType

	damageStamp := entity.Living.LastDamageStamp
	entity.State.mu.Unlock()

	newRetaliation := hurt && causeID != 0 && causeID != entityID && (!entity.Target.HasRetaliationStamp || damageStamp != entity.Target.LastRetaliationStamp)
	if newRetaliation {
		entity.Target.LastRetaliationStamp = damageStamp
		entity.Target.HasRetaliationStamp = true

		candidate := runtime.runtimeLivingTargetByID(causeID)
		if candidate.valid(runtime, position, skeletonFollowRange) {
			entity.Target.Target = candidate
			entity.Target.UnseenChecks = 0
			entity.Target.Retaliating = true
		}
	}

	target := entity.Target.Target
	if target.present() && target.valid(runtime, position, skeletonFollowRange) {
		if fullGoalTick {
			if target.hasLineOfSight(runtime, position, skeletonEyeHeight) {
				entity.Target.UnseenChecks = 0
			} else {
				entity.Target.UnseenChecks++
			}
		}

		maximumUnseen := int32(skeletonNearestUnseenChecks)

		if entity.Target.Retaliating {
			maximumUnseen = skeletonRetaliateUnseenChecks
		}

		if entity.Target.UnseenChecks <= maximumUnseen {
			return
		}
	}

	entity.Target.Target = runtimeLivingTarget{}
	entity.Target.UnseenChecks = 0
	entity.Target.Retaliating = false

	if !fullGoalTick || skeletonRandomInt(runtime, skeletonTargetInterval) != 0 {
		return
	}

	nearest := runtime.nearestValidPlayer(position, skeletonEyeHeight, skeletonFollowRange)
	if nearest != nil {
		entity.Target.Target = runtimeLivingTarget{session: nearest}
	}
}

func (entity *runtimeSkeletonEntity) shouldRestrictSun(runtime *Runtime) bool {
	entity.State.mu.RLock()
	headEmpty := entity.Head.Empty()
	entity.State.mu.RUnlock()

	if !headEmpty {
		return false
	}

	dayTime := floorMod(runtime.World.Time().DayTime, 24000)

	return zombieSkyLightLevel(dayTime) > 11
}

func (entity *runtimeSkeletonEntity) shouldFleeSun(runtime *Runtime) bool {
	entity.State.mu.RLock()
	position := entity.State.Position
	burning := entity.Living.RemainingFireTicks > 0
	headEmpty := entity.Head.Empty()
	entity.State.mu.RUnlock()

	if !burning || !headEmpty {
		return false
	}

	dayTime := floorMod(runtime.World.Time().DayTime, 24000)
	_, seesSky := zombieDaylightBrightness(runtime.World, skeletonEyeBlockPosition(position), dayTime)

	return zombieSkyLightLevel(dayTime) > 11 && seesSky
}

func (entity *runtimeSkeletonEntity) findSunShelterPath(runtime *Runtime) []game.Position {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	dayTime := floorMod(runtime.World.Time().DayTime, 24000)

	for range skeletonFleeSunAttempts {
		goal := game.Position{
			X: position.X + float64(skeletonRandomInt(runtime, 20)-10),
			Y: position.Y + float64(skeletonRandomInt(runtime, 6)-3),
			Z: position.Z + float64(skeletonRandomInt(runtime, 20)-10),
		}

		path := runtime.findGroundPath(position, goal, entity.Living.Width, entity.Living.Height, skeletonFollowRange)
		if len(path) == 0 {
			continue
		}

		end := path[len(path)-1]

		brightness, seesSky := zombieDaylightBrightness(runtime.World, skeletonEyeBlockPosition(end), dayTime)
		if !seesSky && brightness > 0.5 {
			return path
		}
	}

	return nil
}

func (entity *runtimeSkeletonEntity) tickBowGoal(runtime *Runtime) {
	target := entity.GoalTarget

	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	visible := target.hasLineOfSight(runtime, position, skeletonEyeHeight)
	if visible != (entity.Bow.SeeTime > 0) {
		entity.Bow.SeeTime = 0
	}

	if visible {
		entity.Bow.SeeTime++
	} else {
		entity.Bow.SeeTime--
	}

	distance := distanceSquared(position, target.position())
	if distance <= skeletonBowRangeSquared && entity.Bow.SeeTime >= 20 {
		entity.Navigation.Stop()

		entity.Bow.StrafeTime++
	} else {
		entity.moveToTarget(runtime, target.position(), skeletonBowPathSpeed)

		entity.Bow.StrafeTime = -1
	}

	if entity.Bow.StrafeTime >= 20 {
		if runtime.nextEntityRandom() < 0.3 {
			entity.Bow.StrafeClockwise = !entity.Bow.StrafeClockwise
		}

		if runtime.nextEntityRandom() < 0.3 {
			entity.Bow.StrafeForward = !entity.Bow.StrafeForward
		}

		entity.Bow.StrafeTime = 0
	}

	if entity.Bow.StrafeTime > -1 {
		if distance > skeletonBowFarRangeSquared {
			entity.Bow.StrafeForward = true
		} else if distance < skeletonBowNearRangeSquared {
			entity.Bow.StrafeForward = false
		}

		forward := float32(-0.5)

		if entity.Bow.StrafeForward {
			forward = 0.5
		}

		sideways := float32(-0.5)

		if entity.Bow.StrafeClockwise {
			sideways = 0.5
		}

		entity.MoveControl.Strafe(forward, sideways)
	}

	entity.LookControl.SetWanted(target.eyePosition(), 30, 30)

	usingItem, useTicks := entity.tickUsingBow()
	if usingItem {
		if !visible && entity.Bow.SeeTime < -60 {
			entity.stopUsingBow()
		} else if visible && useTicks >= skeletonBowUseTicks {
			entity.stopUsingBow()

			entity.shoot(runtime, target)

			entity.Bow.AttackTime = entity.bowAttackInterval(runtime.Difficulty)
		}

		return
	}

	entity.Bow.AttackTime--

	if entity.Bow.AttackTime <= 0 && entity.Bow.SeeTime >= -60 {
		entity.startUsingBow()
	}
}

func (entity *runtimeSkeletonEntity) tickMeleeGoal(runtime *Runtime) {
	target := entity.GoalTarget

	entity.State.mu.RLock()
	position := entity.State.Position
	entityID := entity.State.ID
	entity.State.mu.RUnlock()

	entity.LookControl.SetWanted(target.eyePosition(), 30, 30)

	entity.Melee.RepathTime--

	if entity.Melee.RepathTime <= 0 {
		entity.moveToTarget(runtime, target.position(), skeletonMeleePathSpeed)

		entity.Melee.RepathTime = int32(4 + skeletonRandomInt(runtime, 7))
	}

	entity.Melee.AttackTime = max(entity.Melee.AttackTime-1, 0)
	if entity.Melee.AttackTime > 0 {
		return
	}

	attackBox := entity.Living.CollisionBox(position)

	attackBox.MinX -= 0.6
	attackBox.MinZ -= 0.6
	attackBox.MaxX += 0.6
	attackBox.MaxZ += 0.6

	if !attackBox.Intersects(target.collisionBox()) || !target.hasLineOfSight(runtime, position, skeletonEyeHeight) {
		return
	}

	entity.Melee.AttackTime = skeletonMeleeAttackInterval

	runtime.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{
		ID:      protocol.ClientboundEntityAnimationID,
		Encoder: protocol.EntityAnimation{EntityID: entityID, Animation: protocol.EntityAnimationSwingMainHand},
	})

	damage := game.Damage{Type: game.DamageMobAttack, Amount: skeletonAttackDamage, CauseEntityID: entityID, DirectEntityID: entityID}

	if target.session != nil {
		damage.Amount = skeletonMeleeDamageForDifficulty(runtime.Difficulty)

		update, applied := runtime.damagePlayerLocked(target.session, damage)
		if applied {
			runtime.sendPlayerSurvivalUpdate(target.session, update)
		}

		return
	}

	update, applied := runtime.damageRuntimeLivingEntityLocked(target.entity, damage)
	if applied {
		runtime.sendRuntimeLivingDamageUpdate(update)
	}
}

func (entity *runtimeSkeletonEntity) tickIdle(runtime *Runtime) {
	if entity.Idle.Strolling && entity.Navigation.Done() {
		entity.Idle.Strolling = false
	}

	if !entity.FullGoalTick || entity.Idle.Strolling || entity.NoActionTime >= 100 || skeletonRandomInt(runtime, skeletonStrollInterval) != 0 {
		return
	}

	path := entity.randomStrollPath(runtime)
	path = entity.restrictSunPath(runtime, path)

	if entity.Navigation.MoveTo(path, 1) {
		entity.Idle.Strolling = true
	}
}

func (entity *runtimeSkeletonEntity) randomStrollPath(runtime *Runtime) []game.Position {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	var best []game.Position

	bestDistance := -1.0

	for range skeletonRandomPositionAttempts {
		goal := game.Position{
			X: position.X + float64(skeletonRandomInt(runtime, 21)-10),
			Y: position.Y + float64(skeletonRandomInt(runtime, 15)-7),
			Z: position.Z + float64(skeletonRandomInt(runtime, 21)-10),
		}

		path := runtime.findGroundPath(position, goal, entity.Living.Width, entity.Living.Height, skeletonFollowRange)
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

func (entity *runtimeSkeletonEntity) restrictSunPath(runtime *Runtime, path []game.Position) []game.Position {
	if !entity.Sun.Restricting {
		return path
	}

	for index, position := range path {
		_, seesSky := zombieDaylightBrightness(runtime.World, skeletonEyeBlockPosition(position), floorMod(runtime.World.Time().DayTime, 24000))
		if seesSky {
			return path[:index]
		}
	}

	return path
}

func (entity *runtimeSkeletonEntity) moveToTarget(runtime *Runtime, target game.Position, speed float64) {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	path := runtime.findGroundPath(position, target, entity.Living.Width, entity.Living.Height, skeletonFollowRange)
	path = entity.restrictSunPath(runtime, path)

	entity.Navigation.MoveTo(path, speed)
}

func (entity *runtimeSkeletonEntity) shoot(runtime *Runtime, target runtimeLivingTarget) {
	entity.State.mu.RLock()
	position := entity.State.Position
	ownerID := entity.State.ID
	entity.State.mu.RUnlock()

	baseDamage := 2 + float64(runtime.Difficulty)*0.11 + 0.57425*skeletonTriangle(runtime)
	targetPosition := target.position()
	deltaX := targetPosition.X - position.X
	deltaZ := targetPosition.Z - position.Z
	horizontalDistance := math.Hypot(deltaX, deltaZ)
	arrowY := position.Y + skeletonEyeHeight - 0.1
	deltaY := targetPosition.Y + target.height()/3 - arrowY + horizontalDistance*0.2

	length := math.Sqrt(deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ)
	if length == 0 {
		return
	}

	uncertainty := float64(14 - int(runtime.Difficulty)*4)
	spread := 0.0172275 * uncertainty

	velocity := game.Velocity{
		X: (deltaX/length + skeletonTriangle(runtime)*spread) * skeletonBowVelocity,
		Y: (deltaY/length + skeletonTriangle(runtime)*spread) * skeletonBowVelocity,
		Z: (deltaZ/length + skeletonTriangle(runtime)*spread) * skeletonBowVelocity,
	}

	arrow := runtime.SpawnArrow(game.Position{X: position.X, Y: arrowY, Z: position.Z}, velocity, ownerID)

	arrow.SetBaseDamage(baseDamage)

	pitch := 1 / (runtime.nextEntityRandom()*0.4 + 0.8)

	runtime.broadcastRuntimeEntitySound(entity, game.SoundEntitySkeletonShoot, 1, pitch)
}

func (entity *runtimeSkeletonEntity) holdingBow() bool {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return !entity.MainHand.Empty() && entity.MainHand.Item == game.ItemBow
}

func (entity *runtimeSkeletonEntity) tickUsingBow() (bool, int32) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if !entity.Living.UsingItem {
		return false, 0
	}

	return true, entity.Living.TickUsingItem()
}

func (entity *runtimeSkeletonEntity) startUsingBow() {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	entity.Living.StartUsingItem(false)

	entity.State.metadataDirty = true
}

func (entity *runtimeSkeletonEntity) stopUsingBow() {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if !entity.Living.UsingItem {
		return
	}

	entity.Living.StopUsingItem()

	entity.State.metadataDirty = true
}

func (entity *runtimeSkeletonEntity) bowAttackInterval(difficulty game.Difficulty) int32 {
	if difficulty == game.DifficultyHard {
		return skeletonBowHardCooldown
	}

	return skeletonBowNormalCooldown
}

func (entity *runtimeSkeletonEntity) stopIdle() {
	entity.Idle = skeletonIdleState{}

	entity.Navigation.Stop()
}

func (entity *runtimeSkeletonEntity) setAggressive(aggressive bool) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if entity.Aggressive == aggressive {
		return
	}

	entity.Aggressive = aggressive
	entity.State.metadataDirty = true
}

func (entity *runtimeSkeletonEntity) applyEnvironmentDamage(runtime *Runtime, damage game.Damage) bool {
	update, applied := runtime.damageRuntimeLivingEntityLocked(entity, damage)
	if applied {
		runtime.sendRuntimeLivingDamageUpdate(update)
	}

	return applied
}

func (runtime *Runtime) SpawnSkeleton(position game.Position) *runtimeSkeletonEntity {
	definition, valid := game.EntitySkeleton.Definition()
	if !valid {
		return nil
	}

	entity := &runtimeSkeletonEntity{
		Living: RuntimeLivingState{
			NextStepDistance: 1,
			Width:            definition.Width,
			Height:           definition.Height,
		},
		Bow:      skeletonBowState{AttackTime: -1, StrafeTime: -1},
		MainHand: game.ItemStack{Item: game.ItemBow, Count: 1},
	}

	entity.Living.Reset(skeletonMaxHealth)

	entity.Goals.Add(2, 0, &skeletonRestrictSunGoal{Entity: entity})
	entity.Goals.Add(3, runtimeGoalMove, &skeletonFleeSunGoal{Entity: entity})
	entity.Goals.Add(4, runtimeGoalMove|runtimeGoalLook, &skeletonBowGoal{Entity: entity})
	entity.Goals.Add(4, runtimeGoalMove|runtimeGoalLook, &skeletonMeleeGoal{Entity: entity})
	entity.Goals.Add(5, runtimeGoalMove, &skeletonIdleGoal{Entity: entity})

	runtime.registerRuntimeEntity(entity, position)

	return entity
}

func skeletonEyeBlockPosition(position game.Position) game.BlockPosition {
	return game.BlockPosition{
		X: int32(math.Floor(position.X)),
		Y: int32(math.Floor(position.Y + skeletonEyeHeight)),
		Z: int32(math.Floor(position.Z)),
	}
}

func skeletonRandomInt(runtime *Runtime, bound int) int {
	if bound <= 1 {
		return 0
	}

	value := int(runtime.nextEntityRandom() * float32(bound))

	return min(value, bound-1)
}

func skeletonTriangle(runtime *Runtime) float64 {
	return float64(runtime.nextEntityRandom() - runtime.nextEntityRandom())
}

func skeletonMeleeDamageForDifficulty(difficulty game.Difficulty) float32 {
	switch difficulty {
	case game.DifficultyPeaceful:
		return 0
	case game.DifficultyEasy:
		return min(skeletonAttackDamage/2+1, skeletonAttackDamage)
	case game.DifficultyHard:
		return skeletonAttackDamage * 1.5
	default:
		return skeletonAttackDamage
	}
}

func skeletonGroundControlConfig() groundMobControlConfig {
	configuration := zombieGroundControlConfig()

	configuration.MovementSpeed = skeletonMovementSpeed
	configuration.EyeHeight = skeletonEyeHeight

	return configuration
}
