package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	zombieMaxHealth              = 20
	zombieMovementSpeed          = float32(0.23)
	zombieFlyingSpeed            = float32(0.02)
	zombieAttackDamage           = float32(3)
	zombieArmor                  = 2
	zombieKnockbackResistance    = 0
	zombieFollowRange            = 35
	zombieGravity                = 0.08
	zombieGroundFriction         = float32(0.91)
	zombieAirFriction            = float32(0.91)
	zombieVerticalDrag           = 0.98
	zombieJumpStrength           = 0.42
	zombieStepHeight             = 0.6
	zombieAttackInterval         = 20
	zombieAttackReachExpansion   = 0.8284271247461903
	zombieMoveMaximumTurn        = float32(90)
	zombieLookMaximumYaw         = float32(30)
	zombieLookMaximumPitch       = float32(30)
	zombieIdleLookMaximumYaw     = float32(10)
	zombieIdleLookMaximumPitch   = float32(40)
	zombieMaximumHeadYaw         = float32(75)
	zombieTargetInterval         = 5
	zombieTargetUnseenTicks      = 30
	zombieStrollInterval         = 60
	zombieIdleLookProbability    = float32(0.02)
	zombieIdleLookDistance       = 8
	zombieRandomPositionAttempts = 10
	zombieTrackingRangeChunks    = 8
	zombieTrackingInterval       = 3
	zombieLootPickupDelay        = 10
	zombieAmbientSoundInterval   = 80
	zombieFireDurationTicks      = 8 * 20
	zombieLavaFireDurationTicks  = 15 * 20
	zombieBurningDamage          = 1
	zombieLavaDamage             = 4
	zombieStepDistanceScale      = 0.6
	zombieEyeHeight              = 1.74
)

type zombieTargetState struct {
	Session     *Session
	UnseenTicks int32
}

type zombieMeleeState struct {
	Active                          bool
	PathedTarget                    game.Position
	TicksUntilNextPathRecalculation int32
	TicksUntilNextAttack            int32
	LastCanUseCheck                 int64
	RaiseArmTicks                   int32
}

type zombieIdleState struct {
	Strolling       bool
	PlayerLook      *Session
	PlayerLookTicks int32
	RandomLook      bool
	RandomLookTicks int32
	RandomLookX     float64
	RandomLookZ     float64
}

type zombieMeleeGoal struct {
	Entity *runtimeZombieEntity
}

type zombieIdleGoal struct {
	Entity *runtimeZombieEntity
}

type runtimeZombieEntity struct {
	State  RuntimeEntityState
	Living RuntimeLivingState
	RuntimeMobState
	Rotation game.Rotation

	Navigation   groundNavigationState
	MoveControl  groundMoveControlState
	LookControl  groundLookControlState
	BodyControl  groundBodyRotationState
	Target       zombieTargetState
	Melee        zombieMeleeState
	Idle         zombieIdleState
	Goals        runtimeGoalSelector
	GoalTarget   *Session
	FullGoalTick bool

	TickCount        int32
	AmbientSoundTime int32
	Aggressive       bool
	LootDropped      bool
}

func (entity *runtimeZombieEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (goal *zombieMeleeGoal) CanUse(*Runtime) bool {
	return goal.Entity.GoalTarget != nil
}

func (goal *zombieMeleeGoal) CanContinue(*Runtime) bool {
	return goal.Entity.GoalTarget != nil
}

func (goal *zombieMeleeGoal) Start(*Runtime) {
	goal.Entity.stopIdleGoals()
}

func (goal *zombieMeleeGoal) Stop(*Runtime) {
	goal.Entity.stopMelee()
}

func (goal *zombieMeleeGoal) Tick(runtime *Runtime) {
	goal.Entity.tickMeleeGoal(runtime, goal.Entity.GoalTarget, goal.Entity.FullGoalTick)
}

func (*zombieMeleeGoal) RequiresUpdateEveryTick() bool {
	return true
}

func (goal *zombieIdleGoal) CanUse(*Runtime) bool {
	return goal.Entity.GoalTarget == nil
}

func (goal *zombieIdleGoal) CanContinue(*Runtime) bool {
	return goal.Entity.GoalTarget == nil
}

func (goal *zombieIdleGoal) Start(*Runtime) {
	goal.Entity.stopMelee()
}

func (goal *zombieIdleGoal) Stop(*Runtime) {
	goal.Entity.stopIdleGoals()
}

func (goal *zombieIdleGoal) Tick(runtime *Runtime) {
	goal.Entity.tickIdleGoals(runtime, goal.Entity.FullGoalTick)
}

func (*zombieIdleGoal) RequiresUpdateEveryTick() bool {
	return true
}

func (entity *runtimeZombieEntity) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (*runtimeZombieEntity) RuntimeLivingEyeHeight() float64 {
	return zombieEyeHeight
}

func (entity *runtimeZombieEntity) RuntimeMob() *RuntimeMobState {
	return &entity.RuntimeMobState
}

func (entity *runtimeZombieEntity) RuntimeMobDespawnConfig() RuntimeMobDespawnConfig {
	return RuntimeMobDespawnConfig{
		NoDespawnDistance: runtimeMobNoDespawnDistance,
		DespawnDistance:   runtimeMobDespawnDistance,
	}
}

func (entity *runtimeZombieEntity) RuntimeMobRemoveWhenFarAway(float64) bool {
	return true
}

func (entity *runtimeZombieEntity) RuntimeMobRequiresCustomPersistence() bool {
	return false
}

func (entity *runtimeZombieEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: zombieTrackingRangeChunks, UpdateInterval: zombieTrackingInterval, TrackDeltas: true}
}

func (entity *runtimeZombieEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeZombieEntity) runtimeEntityViewLocked() runtimeEntityView {
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

func (entity *runtimeZombieEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	mobFlags := byte(0)

	if entity.Aggressive {
		mobFlags |= protocol.MobFlagAggressive
	}

	return []protocol.EntityMetadataEntry{
		{Index: protocol.EntityFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(entity.Living.EntityFlags())},
		{Index: protocol.LivingHealthMetadataIndex, Type: protocol.MetadataTypeFloat, Value: protocol.MetadataFloat(entity.Living.Health)},
		{Index: protocol.MobFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(mobFlags)},
	}
}

func (entity *runtimeZombieEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (entity *runtimeZombieEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  snapshot.ID,
		UUID:      snapshot.UUID,
		Type:      int32(game.EntityZombie),
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

func (entity *runtimeZombieEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
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
	eyePosition := game.BlockPosition{
		X: int32(math.Floor(position.X)),
		Y: int32(math.Floor(position.Y + zombieEyeHeight)),
		Z: int32(math.Floor(position.Z)),
	}
	brightness, _ := zombieDaylightBrightness(runtime.World, eyePosition, dayTime)

	if brightness > 0.5 {
		entity.State.mu.Lock()
		entity.NoActionTime += 2
		entity.State.mu.Unlock()
	}

	entity.tickBaseEnvironment(runtime)
	entity.tickAmbientSound(runtime)
	entity.tickDaylightBurning(runtime)

	if entity.dead() {
		runtime.tickRuntimeLivingEntity(entity)
		runtime.synchronizeRuntimeEntity(entity)

		return
	}

	fullGoalTick := entity.TickCount <= 1 || (entity.TickCount+entity.State.ID)%2 == 0
	target := entity.tickTarget(runtime, fullGoalTick)

	entity.tickGoals(runtime, target, fullGoalTick)

	fallDamage, step := entity.tickControlsAndMovement(runtime)
	if fallDamage > 0 {
		entity.applyEnvironmentDamage(runtime, game.Damage{Type: game.DamageFall, Amount: fallDamage})
	}

	entity.tickBlockEnvironment(runtime)

	if step {
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityZombieStep, 0.15, 1)
	}

	if !entity.dead() {
		entity.tickAttack(runtime, target)
	}

	runtime.tickRuntimeLivingEntity(entity)
	runtime.synchronizeRuntimeEntity(entity)
}

func (entity *runtimeZombieEntity) RuntimeLivingDamageSound(died bool) (game.SoundEvent, float32, float32) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if died {
		return game.SoundEntityZombieDeath, 1, 1
	}

	entity.AmbientSoundTime = -zombieAmbientSoundInterval

	return game.SoundEntityZombieHurt, 1, 1
}

func (entity *runtimeZombieEntity) RuntimeEntitySoundSource() int32 {
	return protocol.SoundSourceHostile
}

func (entity *runtimeZombieEntity) dead() bool {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Dead
}

func (entity *runtimeZombieEntity) tickBaseEnvironment(runtime *Runtime) {
	runtime.tickRuntimeLivingBaseEnvironment(entity)
}

func (entity *runtimeZombieEntity) tickAmbientSound(runtime *Runtime) {
	entity.State.mu.RLock()
	alive := !entity.Living.Dead
	entity.State.mu.RUnlock()

	if !alive {
		return
	}

	selection := int32(zombieRandomInt(runtime, 1000))

	entity.State.mu.Lock()
	ambientSoundTime := entity.AmbientSoundTime
	entity.AmbientSoundTime++
	play := selection < ambientSoundTime

	if play {
		entity.AmbientSoundTime = -zombieAmbientSoundInterval
	}

	entity.State.mu.Unlock()

	if play {
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityZombieAmbient, 1, 1)
	}
}

func (entity *runtimeZombieEntity) tickDaylightBurning(runtime *Runtime) {
	dayTime := floorMod(runtime.World.Time().DayTime, 24000)
	if dayTime > 12541 && dayTime < 23460 {
		return
	}

	entity.State.mu.RLock()
	position := entity.State.Position

	box := entity.Living.CollisionBox(position)

	alive := !entity.Living.Dead
	entity.State.mu.RUnlock()

	if !alive {
		return
	}

	eyePosition := game.BlockPosition{
		X: int32(math.Floor(position.X)),
		Y: int32(math.Floor(position.Y + zombieEyeHeight)),
		Z: int32(math.Floor(position.Z)),
	}

	brightness, seesSky := zombieDaylightBrightness(runtime.World, eyePosition, dayTime)
	if brightness <= 0.5 || runtime.nextEntityRandom()*30 >= (brightness-0.4)*2 {
		return
	}

	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0
	inPowderSnow := runtime.World.BlockAt(eyePosition) == game.PowderSnow

	if inWater || inPowderSnow || !seesSky {
		return
	}

	entity.State.mu.Lock()

	wasBurning := entity.Living.RemainingFireTicks > 0
	entity.Living.RemainingFireTicks = max(entity.Living.RemainingFireTicks, zombieFireDurationTicks)

	if !wasBurning {
		entity.State.metadataDirty = true
	}

	entity.State.mu.Unlock()
}

func (entity *runtimeZombieEntity) tickBlockEnvironment(runtime *Runtime) {
	runtime.tickRuntimeLivingBlockEnvironment(entity)
}

func (entity *runtimeZombieEntity) applyEnvironmentDamage(runtime *Runtime, damage game.Damage) bool {
	update, applied := runtime.damageRuntimeLivingEntityLocked(entity, damage)
	if applied {
		runtime.sendRuntimeLivingDamageUpdate(update)
	}

	return applied
}

func (r *Runtime) runtimeLivingTouchesFallResettingBlock(box game.AABB) bool {
	minX := int32(math.Floor(box.MinX))
	minY := int32(math.Floor(box.MinY))
	minZ := int32(math.Floor(box.MinZ))
	maxX := int32(math.Ceil(box.MaxX)) - 1
	maxY := int32(math.Ceil(box.MaxY)) - 1
	maxZ := int32(math.Ceil(box.MaxZ)) - 1

	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			for z := minZ; z <= maxZ; z++ {
				block := r.World.BlockAt(game.BlockPosition{X: x, Y: y, Z: z})
				if block.HasTrait(game.BlockTraitFallDamageResetting) {
					return true
				}
			}
		}
	}

	return false
}

func (entity *runtimeZombieEntity) RuntimeLivingDied(runtime *Runtime) {
	entity.State.mu.Lock()

	if entity.LootDropped {
		entity.State.mu.Unlock()

		return
	}

	entity.LootDropped = true
	position := entity.State.Position

	entity.State.mu.Unlock()

	count := int32(runtime.nextEntityRandom() * 3)
	if count <= 0 {
		return
	}

	stack := game.ItemStack{Item: game.ItemRottenFlesh, Count: count}
	runtime.SpawnItemEntity(stack, position, game.Velocity{}, zombieLootPickupDelay)
}

func (entity *runtimeZombieEntity) tickTarget(runtime *Runtime, fullGoalTick bool) *Session {
	entity.State.mu.Lock()
	current := entity.Target.Session
	position := entity.State.Position
	entity.State.mu.Unlock()

	if current != nil {
		if !fullGoalTick {
			return current
		}

		player := current.playerView()

		if runtime.playerTargetValid(current, position, zombieFollowRange) {
			if runtime.playerTargetHasLineOfSight(position, zombieEyeHeight, player.Position) {
				entity.Target.UnseenTicks = 0
			} else {
				entity.Target.UnseenTicks++
			}

			if entity.Target.UnseenTicks <= zombieTargetUnseenTicks {
				return current
			}
		}

		entity.stopMelee()
		entity.Target = zombieTargetState{}
		current = nil
	}

	if !fullGoalTick || zombieRandomInt(runtime, zombieTargetInterval) != 0 {
		return nil
	}

	nearest := runtime.nearestZombieTarget(position)
	if nearest == nil {
		return nil
	}

	entity.Target.Session = nearest
	entity.Target.UnseenTicks = 0

	return nearest
}

func (entity *runtimeZombieEntity) tickGoals(runtime *Runtime, target *Session, fullGoalTick bool) {
	entity.GoalTarget = target
	entity.FullGoalTick = fullGoalTick
	entity.Goals.Tick(runtime, fullGoalTick)
}

func (entity *runtimeZombieEntity) tickMeleeGoal(runtime *Runtime, target *Session, fullGoalTick bool) {
	player := target.playerView()

	if entity.Melee.Active && fullGoalTick && entity.Navigation.Done() {
		entity.stopMelee()
	}

	if !entity.Melee.Active {
		gameTime := runtime.World.Time().Age
		if gameTime-entity.Melee.LastCanUseCheck < 20 {
			return
		}

		entity.Melee.LastCanUseCheck = gameTime

		path := runtime.findGroundPath(entity.State.Position, player.Position, entity.Living.Width, entity.Living.Height, zombieFollowRange)
		withinReach := zombieMeleeBoxesIntersect(entity.Living.CollisionBox(entity.State.Position), player.collisionBox())

		if len(path) == 0 && !withinReach {
			return
		}

		entity.Navigation.MoveTo(path, 1)

		entity.Melee.Active = true
		entity.Melee.TicksUntilNextPathRecalculation = 0
		entity.Melee.TicksUntilNextAttack = 0
		entity.Melee.RaiseArmTicks = 0
	}

	entity.LookControl.SetWanted(player.eyePosition(), zombieLookMaximumYaw, zombieLookMaximumPitch)

	entity.Melee.TicksUntilNextPathRecalculation = max(entity.Melee.TicksUntilNextPathRecalculation-1, 0)

	lineOfSight := runtime.playerTargetHasLineOfSight(entity.State.Position, zombieEyeHeight, player.Position)

	pathedTargetUnset := entity.Melee.PathedTarget == (game.Position{})

	targetMoved := distanceSquared(player.Position, entity.Melee.PathedTarget) >= 1

	shouldRecalculate := pathedTargetUnset || targetMoved
	if !shouldRecalculate && lineOfSight && entity.Melee.TicksUntilNextPathRecalculation <= 0 {
		shouldRecalculate = runtime.nextEntityRandom() < 0.05
	}

	if lineOfSight && entity.Melee.TicksUntilNextPathRecalculation <= 0 && shouldRecalculate {
		entity.Melee.PathedTarget = player.Position
		entity.Melee.TicksUntilNextPathRecalculation = int32(4 + zombieRandomInt(runtime, 7))

		distance := distanceSquared(entity.State.Position, player.Position)
		if distance > 1024 {
			entity.Melee.TicksUntilNextPathRecalculation += 10
		} else if distance > 256 {
			entity.Melee.TicksUntilNextPathRecalculation += 5
		}

		path := runtime.findGroundPath(entity.State.Position, player.Position, entity.Living.Width, entity.Living.Height, zombieFollowRange)
		if !entity.Navigation.MoveTo(path, 1) {
			entity.Melee.TicksUntilNextPathRecalculation += 15
		}
	}

	entity.Melee.TicksUntilNextAttack = max(entity.Melee.TicksUntilNextAttack-1, 0)
	entity.Melee.RaiseArmTicks++

	entity.setAggressive(entity.Melee.RaiseArmTicks >= 5 && entity.Melee.TicksUntilNextAttack < zombieAttackInterval/2)
}

func (entity *runtimeZombieEntity) tickIdleGoals(runtime *Runtime, fullGoalTick bool) {
	if entity.Idle.Strolling && entity.Navigation.Done() {
		entity.Idle.Strolling = false
	}

	if entity.Idle.PlayerLook != nil {
		player := entity.Idle.PlayerLook.playerView()
		valid := !player.Dead && distanceSquared(entity.State.Position, player.Position) <= zombieIdleLookDistance*zombieIdleLookDistance

		if fullGoalTick && (!valid || entity.Idle.PlayerLookTicks <= 0) {
			entity.Idle.PlayerLook = nil
		} else if valid && fullGoalTick {
			entity.LookControl.SetWanted(player.eyePosition(), zombieIdleLookMaximumYaw, zombieIdleLookMaximumPitch)
			entity.Idle.PlayerLookTicks--
		}
	}

	if entity.Idle.RandomLook {
		entity.Idle.RandomLookTicks--

		if entity.Idle.RandomLookTicks < 0 {
			entity.Idle.RandomLook = false
		} else {
			wanted := entity.State.Position
			wanted.X += entity.Idle.RandomLookX
			wanted.Y += 1.74
			wanted.Z += entity.Idle.RandomLookZ
			entity.LookControl.SetWanted(wanted, zombieIdleLookMaximumYaw, zombieIdleLookMaximumPitch)
		}
	}

	if !fullGoalTick {
		return
	}

	if !entity.Idle.Strolling && !entity.Idle.RandomLook && entity.NoActionTime < 100 && zombieRandomInt(runtime, zombieStrollInterval) == 0 {
		path := entity.randomStrollPath(runtime)
		if entity.Navigation.MoveTo(path, 1) {
			entity.Idle.Strolling = true
		}
	}

	if entity.Idle.PlayerLook == nil && !entity.Idle.RandomLook && runtime.nextEntityRandom() < zombieIdleLookProbability {
		entity.Idle.PlayerLook = runtime.nearestZombieLookTarget(entity.State.Position)
		if entity.Idle.PlayerLook != nil {
			entity.Idle.PlayerLookTicks = int32(20 + zombieRandomInt(runtime, 20))
		}
	}

	if entity.Idle.PlayerLook == nil && !entity.Idle.Strolling && !entity.Idle.RandomLook && runtime.nextEntityRandom() < zombieIdleLookProbability {
		angle := float64(runtime.nextEntityRandom()) * math.Pi * 2
		entity.Idle.RandomLook = true
		entity.Idle.RandomLookTicks = int32(20 + zombieRandomInt(runtime, 20))
		entity.Idle.RandomLookX = math.Cos(angle)
		entity.Idle.RandomLookZ = math.Sin(angle)
	}
}

func (entity *runtimeZombieEntity) randomStrollPath(runtime *Runtime) []game.Position {
	position := entity.State.Position
	blockPosition := game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
	inWater := runtime.World.FluidAt(blockPosition).Type() == game.FluidTypeWater
	horizontalRange := 10
	avoidWater := runtime.nextEntityRandom() >= 0.001

	if inWater {
		horizontalRange = 15
		avoidWater = true
	}

	path := entity.randomStrollPathCandidates(runtime, horizontalRange, avoidWater)
	if len(path) == 0 && inWater {
		path = entity.randomStrollPathCandidates(runtime, 10, false)
	}

	return path
}

func (entity *runtimeZombieEntity) randomStrollPathCandidates(runtime *Runtime, horizontalRange int, avoidWater bool) []game.Position {
	position := entity.State.Position

	var best []game.Position

	bestDistance := -1.0

	for range zombieRandomPositionAttempts {
		offsetX := zombieRandomInt(runtime, horizontalRange*2+1) - horizontalRange
		offsetY := zombieRandomInt(runtime, 15) - 7
		offsetZ := zombieRandomInt(runtime, horizontalRange*2+1) - horizontalRange

		goal := game.Position{X: position.X + float64(offsetX), Y: position.Y + float64(offsetY), Z: position.Z + float64(offsetZ)}
		path := runtime.findGroundPath(position, goal, entity.Living.Width, entity.Living.Height, zombieFollowRange)

		if len(path) == 0 {
			continue
		}

		end := path[len(path)-1]
		endBlock := game.BlockPosition{X: int32(math.Floor(end.X)), Y: int32(math.Floor(end.Y)), Z: int32(math.Floor(end.Z))}

		if avoidWater && runtime.World.FluidAt(endBlock).Type() == game.FluidTypeWater {
			continue
		}

		distance := distanceSquared(position, end)
		if distance > bestDistance {
			best = path
			bestDistance = distance
		}
	}

	return best
}

func (entity *runtimeZombieEntity) tickControlsAndMovement(runtime *Runtime) (float32, bool) {
	configuration := zombieGroundControlConfig()

	return runtime.tickGroundMobMovement(entity, &entity.Living, &entity.Rotation, &entity.Navigation, &entity.MoveControl, &entity.LookControl, &entity.BodyControl, configuration, false)
}

func (entity *runtimeZombieEntity) applyGroundLivingPhysics(runtime *Runtime) {
	configuration := zombieGroundControlConfig()

	entity.State.mu.Lock()
	runtime.applyGroundLivingPhysics(&entity.State, &entity.Living, &entity.Rotation, &entity.MoveControl, configuration)
	entity.State.mu.Unlock()
}

func (entity *runtimeZombieEntity) tickAttack(runtime *Runtime, target *Session) {
	if target == nil || !entity.Melee.Active || entity.Melee.TicksUntilNextAttack > 0 {
		return
	}

	targetView := target.playerView()

	entity.State.mu.Lock()

	if !zombieMeleeBoxesIntersect(entity.Living.CollisionBox(entity.State.Position), targetView.collisionBox()) {
		entity.State.mu.Unlock()

		return
	}

	position := entity.State.Position
	entityID := entity.State.ID
	entity.State.mu.Unlock()

	if !runtime.playerTargetHasLineOfSight(position, zombieEyeHeight, targetView.Position) {
		return
	}

	entity.Melee.TicksUntilNextAttack = zombieAttackInterval

	runtime.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{
		ID:      protocol.ClientboundEntityAnimationID,
		Encoder: protocol.EntityAnimation{EntityID: entityID, Animation: protocol.EntityAnimationSwingMainHand},
	})

	damageAmount := zombieDamageForDifficulty(runtime.Difficulty)

	damage := game.Damage{Type: game.DamageMobAttack, Amount: damageAmount, CauseEntityID: entityID, DirectEntityID: entityID}

	update, applied := runtime.damagePlayerLocked(target, damage)
	if !applied {
		return
	}

	var knockbackPlayer game.Player

	if update.fullHurt {
		originalVelocity := targetView.Velocity

		knockbackPlayer, _ = target.updatePlayerState(func(current *game.Player) bool {
			directionX := position.X - current.Position.X
			directionZ := position.Z - current.Position.Z

			runtime.applyPlayerKnockback(current, directionX, directionZ, playerHurtKnockback)

			return true
		})

		update.player = knockbackPlayer

		target.mutatePlayer(func(current *game.Player) bool {
			current.Velocity = originalVelocity

			return true
		})
	}

	runtime.sendPlayerSurvivalUpdate(target, update)

	if update.fullHurt {
		runtime.sendPlayerKnockback(knockbackPlayer)
	}
}

func (entity *runtimeZombieEntity) stopMelee() {
	if !entity.Melee.Active {
		return
	}

	entity.Melee.Active = false

	entity.Navigation.Stop()

	entity.setAggressive(false)
}

func (entity *runtimeZombieEntity) stopIdleGoals() {
	entity.Idle = zombieIdleState{}
	if !entity.Melee.Active {
		entity.Navigation.Stop()
	}
}

func (entity *runtimeZombieEntity) setAggressive(aggressive bool) {
	entity.State.mu.Lock()
	defer entity.State.mu.Unlock()

	if entity.Aggressive == aggressive {
		return
	}

	entity.Aggressive = aggressive
	entity.State.metadataDirty = true
}

func (r *Runtime) SpawnZombie(position game.Position) *runtimeZombieEntity {
	definition, valid := game.EntityZombie.Definition()
	if !valid {
		return nil
	}

	entity := &runtimeZombieEntity{
		Living: RuntimeLivingState{
			NextStepDistance:    1,
			KnockbackResistance: zombieKnockbackResistance,
			Width:               definition.Width,
			Height:              definition.Height,
			Armor:               zombieArmor,
		},
		Melee: zombieMeleeState{LastCanUseCheck: -20},
	}

	entity.Living.Reset(zombieMaxHealth)

	entity.Goals.Add(2, runtimeGoalMove|runtimeGoalLook, &zombieMeleeGoal{Entity: entity})
	entity.Goals.Add(7, runtimeGoalMove|runtimeGoalLook, &zombieIdleGoal{Entity: entity})

	r.registerRuntimeEntity(entity, position)

	return entity
}

func (r *Runtime) nearestZombieTarget(position game.Position) *Session {
	return r.nearestValidPlayer(position, zombieEyeHeight, zombieFollowRange)
}

func (r *Runtime) zombieTarget(entity *runtimeZombieEntity) *Session {
	position := entity.State.Position
	current := entity.Target.Session

	if current != nil {
		if r.playerTargetValid(current, position, zombieFollowRange) {
			return current
		}
	}

	entity.Target = zombieTargetState{}
	entity.Target.Session = r.nearestZombieTarget(position)

	return entity.Target.Session
}

func (r *Runtime) nearestZombieLookTarget(position game.Position) *Session {
	var nearest *Session

	nearestDistance := float64(zombieIdleLookDistance * zombieIdleLookDistance)

	for _, session := range r.sessionView() {
		player := session.playerView()
		if player.Dead || player.GameMode == game.GameModeSpectator {
			continue
		}

		distance := distanceSquared(position, player.Position)
		if distance < nearestDistance {
			nearest = session
			nearestDistance = distance
		}
	}

	return nearest
}

func (r *Runtime) zombieHasLineOfSight(from, to game.Position) bool {
	return r.playerTargetHasLineOfSight(from, zombieEyeHeight, to)
}

func groundNavigationNodeReached(position game.Position, path []game.Position, index int, width float64) bool {
	waypoint := path[index]
	maximumDistance := width / 2

	if width <= 0.75 {
		maximumDistance = 0.75 - width/2
	}

	deltaX := math.Abs(position.X - waypoint.X)
	deltaY := math.Abs(position.Y - waypoint.Y)
	deltaZ := math.Abs(position.Z - waypoint.Z)

	if deltaX < maximumDistance && deltaZ < maximumDistance && deltaY < groundMovementWaypointVertical {
		return true
	}

	if index+1 >= len(path) {
		return false
	}

	toCurrentX := waypoint.X - position.X
	toCurrentY := waypoint.Y - position.Y
	toCurrentZ := waypoint.Z - position.Z
	distanceCurrentSquared := toCurrentX*toCurrentX + toCurrentY*toCurrentY + toCurrentZ*toCurrentZ

	if distanceCurrentSquared >= groundMovementOvershootDistanceSq {
		return false
	}

	next := path[index+1]
	toNextX := next.X - position.X
	toNextY := next.Y - position.Y
	toNextZ := next.Z - position.Z
	distanceNextSquared := toNextX*toNextX + toNextY*toNextY + toNextZ*toNextZ

	if distanceNextSquared >= distanceCurrentSquared && distanceCurrentSquared >= groundMovementOvershootNearNodeSq {
		return false
	}

	currentLength := math.Sqrt(distanceCurrentSquared)
	nextLength := math.Sqrt(distanceNextSquared)

	if currentLength == 0 || nextLength == 0 {
		return true
	}

	dot := (toCurrentX*toNextX + toCurrentY*toNextY + toCurrentZ*toNextZ) / (currentLength * nextLength)

	return dot < 0
}

func zombieMeleeBoxesIntersect(zombie, target game.AABB) bool {
	zombie.MinX -= zombieAttackReachExpansion
	zombie.MinZ -= zombieAttackReachExpansion
	zombie.MaxX += zombieAttackReachExpansion
	zombie.MaxZ += zombieAttackReachExpansion

	return zombie.Intersects(target)
}

func zombieDamageForDifficulty(difficulty game.Difficulty) float32 {
	switch difficulty {
	case game.DifficultyPeaceful:
		return 0
	case game.DifficultyEasy:
		return min(zombieAttackDamage/2+1, zombieAttackDamage)
	case game.DifficultyHard:
		return zombieAttackDamage * 1.5
	default:
		return zombieAttackDamage
	}
}

func zombieRandomInt(runtime *Runtime, bound int) int {
	if bound <= 1 {
		return 0
	}

	value := int(runtime.nextEntityRandom() * float32(bound))

	return min(value, bound-1)
}

func zombieGroundControlConfig() groundMobControlConfig {
	return groundMobControlConfig{
		MovementSpeed:        zombieMovementSpeed,
		FlyingSpeed:          zombieFlyingSpeed,
		GroundFriction:       zombieGroundFriction,
		AirFriction:          zombieAirFriction,
		Gravity:              zombieGravity,
		VerticalDrag:         zombieVerticalDrag,
		JumpStrength:         zombieJumpStrength,
		StepHeight:           zombieStepHeight,
		StepDistanceScale:    zombieStepDistanceScale,
		EyeHeight:            zombieEyeHeight,
		MoveMaximumTurn:      zombieMoveMaximumTurn,
		IdleLookMaximumYaw:   zombieIdleLookMaximumYaw,
		IdleLookMaximumPitch: zombieIdleLookMaximumPitch,
		MaximumHeadYaw:       zombieMaximumHeadYaw,
	}
}

func zombieDaylightBrightness(world *game.World, position game.BlockPosition, dayTime int64) (float32, bool) {
	sky, block, err := rawLightLevelsAt(world, position)
	if err != nil {
		return 0, false
	}

	skyLevel := zombieSkyLightLevel(dayTime)
	skyDarkening := uint8(15 - skyLevel)
	localSky := byte(0)

	if sky > skyDarkening {
		localSky = sky - skyDarkening
	}

	raw := max(localSky, block)
	value := float32(raw) / 15
	brightness := value / (4 - 3*value)

	seesSky := sky == 15

	if world.Lighting == game.LightingFullbright {
		seesSky = zombieCanSeeSky(world, position)
	}

	return brightness, seesSky
}

func zombieCanSeeSky(world *game.World, position game.BlockPosition) bool {
	maximumY := int32(protocol.OverworldMinY + protocol.OverworldSectionCount*game.ChunkWidth - 1)

	for y := position.Y + 1; y <= maximumY; y++ {
		block := world.BlockAt(game.BlockPosition{X: position.X, Y: y, Z: position.Z})

		_, filter := block.LightProperties()
		if filter != 0 {
			return false
		}
	}

	return true
}

func zombieSkyLightLevel(dayTime int64) float32 {
	switch {
	case dayTime >= 133 && dayTime <= 11867:
		return 15
	case dayTime > 11867 && dayTime < 13670:
		progress := float32(dayTime-11867) / float32(13670-11867)
		return 15 - 11*progress
	case dayTime >= 13670 && dayTime <= 22330:
		return 4
	default:
		adjusted := dayTime
		if adjusted < 133 {
			adjusted += 24000
		}

		progress := float32(adjusted-22330) / float32(24000+133-22330)

		return 4 + 11*progress
	}
}

func rotateTowards(current, wanted, maximum float32) float32 {
	delta := wrapDegrees(wanted - current)
	delta = max(-maximum, min(delta, maximum))

	return current + delta
}

func clampAngleAround(angle, center, maximumDifference float32) float32 {
	difference := wrapDegrees(angle - center)
	difference = max(-maximumDifference, min(difference, maximumDifference))

	return center + difference
}

func wrapDegrees(angle float32) float32 {
	wrapped := float32(math.Mod(float64(angle), 360))
	if wrapped >= 180 {
		wrapped -= 360
	}

	if wrapped < -180 {
		wrapped += 360
	}

	return wrapped
}

func distanceSquared(first, second game.Position) float64 {
	deltaX := first.X - second.X
	deltaY := first.Y - second.Y
	deltaZ := first.Z - second.Z

	return deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ
}
