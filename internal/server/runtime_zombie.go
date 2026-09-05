package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	zombieMaxHealth                    = 20
	zombieMovementSpeed                = float32(0.23)
	zombieFlyingSpeed                  = float32(0.02)
	zombieAttackDamage                 = float32(3)
	zombieArmor                        = 2
	zombieKnockbackResistance          = 0
	zombieFollowRange                  = 35
	zombieGravity                      = 0.08
	zombieGroundFriction               = float32(0.91)
	zombieAirFriction                  = float32(0.91)
	zombieVerticalDrag                 = 0.98
	zombieJumpStrength                 = 0.42
	zombieStepHeight                   = 0.6
	zombieAttackInterval               = 20
	zombieAttackReachExpansion         = 0.8284271247461903
	zombieMoveMaximumTurn              = float32(90)
	zombieLookMaximumYaw               = float32(30)
	zombieLookMaximumPitch             = float32(30)
	zombieIdleLookMaximumYaw           = float32(10)
	zombieIdleLookMaximumPitch         = float32(40)
	zombieMaximumHeadYaw               = float32(75)
	zombieTargetInterval               = 5
	zombieTargetUnseenTicks            = 30
	zombieStrollInterval               = 60
	zombieIdleLookProbability          = float32(0.02)
	zombieIdleLookDistance             = 8
	zombieRandomPositionAttempts       = 10
	zombieTrackingRangeChunks          = 8
	zombieTrackingInterval             = 3
	zombieLootPickupDelay              = 10
	groundMovementInputScale           = float32(0.21600002)
	groundMovementInputMinimumSquared  = 1e-7
	groundMovementPositionEpsilonSq    = 2.5000003e-7
	groundMovementWaypointVertical     = 1.0
	groundMovementOvershootDistanceSq  = 4.0
	groundMovementOvershootNearNodeSq  = 0.5
	groundMovementWantedMinimumSquared = 2.5000003e-7
)

type groundNavigationState struct {
	Path          []game.Position
	Index         int
	SpeedModifier float64
}

type groundMoveControlState struct {
	Wanted        game.Position
	SpeedModifier float64
	Moving        bool
	ForwardInput  float32
	SidewaysInput float32
	Jump          bool
}

type groundLookControlState struct {
	Wanted       game.Position
	Cooldown     int32
	MaximumYaw   float32
	MaximumPitch float32
}

type groundBodyRotationState struct {
	LastStableHead float32
	StableTicks    int32
}

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

type runtimeZombieEntity struct {
	State    RuntimeEntityState
	Living   RuntimeLivingState
	Rotation game.Rotation

	Navigation  groundNavigationState
	MoveControl groundMoveControlState
	LookControl groundLookControlState
	BodyControl groundBodyRotationState
	Target      zombieTargetState
	Melee       zombieMeleeState
	Idle        zombieIdleState

	TickCount    int32
	NoActionTime int32
	Aggressive   bool
	LootDropped  bool
}

func (navigation *groundNavigationState) Done() bool {
	return navigation.Index >= len(navigation.Path)
}

func (navigation *groundNavigationState) Stop() {
	navigation.Path = nil
	navigation.Index = 0
	navigation.SpeedModifier = 0
}

func (navigation *groundNavigationState) MoveTo(path []game.Position, speedModifier float64) bool {
	if len(path) == 0 {
		navigation.Stop()

		return false
	}

	navigation.Path = path
	navigation.Index = 0
	navigation.SpeedModifier = speedModifier

	return true
}

func (navigation *groundNavigationState) Tick(position game.Position, width float64, control *groundMoveControlState) {
	for !navigation.Done() && groundNavigationNodeReached(position, navigation.Path, navigation.Index, width) {
		navigation.Index++
	}

	if navigation.Done() {
		control.Moving = false

		return
	}

	control.Wanted = navigation.Path[navigation.Index]
	control.SpeedModifier = navigation.SpeedModifier
	control.Moving = true
}

func (control *groundMoveControlState) Tick(position game.Position, rotation *game.Rotation) {
	control.ForwardInput = 0
	control.SidewaysInput = 0
	control.Jump = false

	if !control.Moving {
		return
	}

	deltaX := control.Wanted.X - position.X
	deltaY := control.Wanted.Y - position.Y
	deltaZ := control.Wanted.Z - position.Z
	distanceSquared := deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ

	if distanceSquared < groundMovementWantedMinimumSquared {
		control.Moving = false

		return
	}

	wantedYaw := float32(math.Atan2(deltaZ, deltaX)*180/math.Pi - 90)
	rotation.Yaw = rotateTowards(rotation.Yaw, wantedYaw, zombieMoveMaximumTurn)

	speed := float32(control.SpeedModifier) * zombieMovementSpeed
	control.ForwardInput = speed

	horizontalDistanceSquared := deltaX*deltaX + deltaZ*deltaZ
	if deltaY > zombieStepHeight && horizontalDistanceSquared < max(1, 0.6) {
		control.Jump = true
	}
}

func (control *groundLookControlState) SetWanted(position game.Position, maximumYaw, maximumPitch float32) {
	control.Wanted = position
	control.Cooldown = 2
	control.MaximumYaw = maximumYaw
	control.MaximumPitch = maximumPitch
}

func (control *groundLookControlState) Tick(position game.Position, rotation *game.Rotation, navigating bool) {
	if control.Cooldown > 0 {
		control.Cooldown--

		deltaX := control.Wanted.X - position.X
		deltaY := control.Wanted.Y - (position.Y + 1.74)
		deltaZ := control.Wanted.Z - position.Z
		horizontalDistance := math.Hypot(deltaX, deltaZ)

		wantedYaw := float32(math.Atan2(deltaZ, deltaX)*180/math.Pi - 90)
		wantedPitch := float32(-math.Atan2(deltaY, horizontalDistance) * 180 / math.Pi)

		rotation.HeadYaw = rotateTowards(rotation.HeadYaw, wantedYaw, control.MaximumYaw)
		rotation.Pitch = rotateTowards(rotation.Pitch, wantedPitch, control.MaximumPitch)
	} else {
		rotation.HeadYaw = rotateTowards(rotation.HeadYaw, rotation.Yaw, zombieIdleLookMaximumYaw)
		rotation.Pitch = rotateTowards(rotation.Pitch, 0, zombieIdleLookMaximumPitch)
	}

	if navigating {
		rotation.HeadYaw = clampAngleAround(rotation.HeadYaw, rotation.Yaw, zombieMaximumHeadYaw)
	}
}

func (control *groundBodyRotationState) Tick(previous, position game.Position, rotation *game.Rotation) {
	deltaX := position.X - previous.X
	deltaZ := position.Z - previous.Z
	moving := deltaX*deltaX+deltaZ*deltaZ > groundMovementPositionEpsilonSq

	if moving {
		rotation.HeadYaw = clampAngleAround(rotation.HeadYaw, rotation.Yaw, zombieMaximumHeadYaw)
		control.LastStableHead = rotation.HeadYaw
		control.StableTicks = 0

		return
	}

	if math.Abs(float64(wrapDegrees(rotation.HeadYaw-control.LastStableHead))) > 15 {
		control.StableTicks = 0
		control.LastStableHead = rotation.HeadYaw
		rotation.Yaw = clampAngleAround(rotation.Yaw, rotation.HeadYaw, zombieMaximumHeadYaw)

		return
	}

	control.StableTicks++

	if control.StableTicks <= 10 {
		return
	}

	progress := min(float32(control.StableTicks-10)/10, 1)
	maximumDifference := zombieMaximumHeadYaw * (1 - progress)
	rotation.Yaw = clampAngleAround(rotation.Yaw, rotation.HeadYaw, maximumDifference)
}

func (entity *runtimeZombieEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeZombieEntity) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
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

	if runtime.Difficulty == game.DifficultyPeaceful {
		runtime.removeRuntimeEntity(entity.State.ID)

		return
	}

	if dead {
		runtime.tickRuntimeLivingEntity(entity)

		return
	}

	entity.State.mu.Lock()
	entity.TickCount++
	entity.NoActionTime++
	entity.State.mu.Unlock()

	fullGoalTick := entity.TickCount <= 1 || (entity.TickCount+entity.State.ID)%2 == 0
	target := entity.tickTarget(runtime, fullGoalTick)

	entity.tickGoals(runtime, target, fullGoalTick)
	entity.tickControlsAndMovement(runtime)
	entity.tickAttack(runtime, target)

	runtime.tickRuntimeLivingEntity(entity)
	runtime.synchronizeRuntimeEntity(entity)
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

	for _, session := range runtime.snapshotSessions() {
		player := session.snapshotPlayer()
		if distanceSquared(position, player.Position) <= 32*32 {
			entity.NoActionTime = 0

			break
		}
	}

	if current != nil {
		if !fullGoalTick {
			return current
		}

		player := current.snapshotPlayer()
		connected := slices.Contains(runtime.snapshotSessions(), current)

		if connected && zombieTargetRetained(player, position) {
			if runtime.zombieHasLineOfSight(position, player.Position) {
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
	entity.NoActionTime = 0

	return nearest
}

func (entity *runtimeZombieEntity) tickGoals(runtime *Runtime, target *Session, fullGoalTick bool) {
	if target != nil {
		entity.stopIdleGoals()

		entity.tickMeleeGoal(runtime, target, fullGoalTick)

		return
	}

	entity.stopMelee()

	entity.tickIdleGoals(runtime, fullGoalTick)
}

func (entity *runtimeZombieEntity) tickMeleeGoal(runtime *Runtime, target *Session, fullGoalTick bool) {
	player := target.snapshotPlayer()

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
		withinReach := zombieMeleeBoxesIntersect(entity.Living.CollisionBox(entity.State.Position), player.CollisionBox())

		if len(path) == 0 && !withinReach {
			return
		}

		entity.Navigation.MoveTo(path, 1)

		entity.Melee.Active = true
		entity.Melee.TicksUntilNextPathRecalculation = 0
		entity.Melee.TicksUntilNextAttack = 0
		entity.Melee.RaiseArmTicks = 0
	}

	entity.LookControl.SetWanted(player.EyePosition(), zombieLookMaximumYaw, zombieLookMaximumPitch)

	entity.Melee.TicksUntilNextPathRecalculation = max(entity.Melee.TicksUntilNextPathRecalculation-1, 0)

	lineOfSight := runtime.zombieHasLineOfSight(entity.State.Position, player.Position)

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
		player := entity.Idle.PlayerLook.snapshotPlayer()
		valid := !player.Dead && distanceSquared(entity.State.Position, player.Position) <= zombieIdleLookDistance*zombieIdleLookDistance

		if fullGoalTick && (!valid || entity.Idle.PlayerLookTicks <= 0) {
			entity.Idle.PlayerLook = nil
		} else if valid && fullGoalTick {
			entity.LookControl.SetWanted(player.EyePosition(), zombieIdleLookMaximumYaw, zombieIdleLookMaximumPitch)
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

func (entity *runtimeZombieEntity) tickControlsAndMovement(runtime *Runtime) {
	entity.State.mu.Lock()

	previous := entity.State.Position

	entity.Navigation.Tick(entity.State.Position, entity.Living.Width, &entity.MoveControl)
	entity.MoveControl.Tick(entity.State.Position, &entity.Rotation)
	entity.LookControl.Tick(entity.State.Position, &entity.Rotation, !entity.Navigation.Done())

	if entity.MoveControl.Jump && entity.Living.OnGround {
		entity.Living.Velocity.Y = max(entity.Living.Velocity.Y, zombieJumpStrength)
	}

	entity.applyGroundLivingPhysics(runtime)
	entity.BodyControl.Tick(previous, entity.State.Position, &entity.Rotation)

	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)
}

func (entity *runtimeZombieEntity) applyGroundLivingPhysics(runtime *Runtime) {
	wasOnGround := entity.Living.OnGround
	blockFriction := float32(1)
	acceleration := zombieFlyingSpeed

	if wasOnGround {
		blockFriction = runtime.blockFrictionBelow(entity.State.Position)
		acceleration = zombieMovementSpeed * (groundMovementInputScale / (blockFriction * blockFriction * blockFriction))
	}

	inputX := entity.MoveControl.SidewaysInput
	inputZ := entity.MoveControl.ForwardInput
	inputLengthSquared := inputX*inputX + inputZ*inputZ

	if inputLengthSquared >= groundMovementInputMinimumSquared {
		if inputLengthSquared > 1 {
			inverseLength := float32(1 / math.Sqrt(float64(inputLengthSquared)))
			inputX *= inverseLength
			inputZ *= inverseLength
		}

		inputX *= acceleration
		inputZ *= acceleration

		yaw := float64(entity.Rotation.Yaw) * math.Pi / 180
		sine := float64(minecraftSin(yaw))
		cosine := float64(minecraftCos(yaw))

		entity.Living.Velocity.X += float64(inputX)*cosine - float64(inputZ)*sine
		entity.Living.Velocity.Z += float64(inputZ)*cosine + float64(inputX)*sine
	}

	movement := runtime.moveGroundEntity(entity.State.Position, entity.Living.Velocity, entity.Living.Width, entity.Living.Height, zombieStepHeight, wasOnGround)
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

	entity.Living.Velocity.Y -= zombieGravity

	horizontalDrag := zombieAirFriction

	if wasOnGround {
		horizontalDrag = blockFriction * zombieGroundFriction
	}

	entity.Living.Velocity.X *= float64(horizontalDrag)
	entity.Living.Velocity.Y *= zombieVerticalDrag
	entity.Living.Velocity.Z *= float64(horizontalDrag)
}

func (entity *runtimeZombieEntity) tickAttack(runtime *Runtime, target *Session) {
	if target == nil || !entity.Melee.Active || entity.Melee.TicksUntilNextAttack > 0 {
		return
	}

	player := target.snapshotPlayer()

	entity.State.mu.Lock()

	if !zombieMeleeBoxesIntersect(entity.Living.CollisionBox(entity.State.Position), player.CollisionBox()) {
		entity.State.mu.Unlock()

		return
	}

	position := entity.State.Position
	entityID := entity.State.ID
	entity.State.mu.Unlock()

	if !runtime.zombieHasLineOfSight(position, player.Position) {
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

	if update.fullHurt {
		originalVelocity := player.Velocity

		player, _ = target.updatePlayerState(func(current *game.Player) bool {
			directionX := position.X - current.Position.X
			directionZ := position.Z - current.Position.Z

			runtime.applyPlayerKnockback(current, directionX, directionZ, playerHurtKnockback)

			return true
		})

		update.player = player

		target.updatePlayerState(func(current *game.Player) bool {
			current.Velocity = originalVelocity

			return true
		})
	}

	runtime.sendPlayerSurvivalUpdate(target, update)

	if update.fullHurt {
		runtime.sendPlayerKnockback(player)
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
		State: RuntimeEntityState{
			ID:       r.allocateEntityID(),
			UUID:     randomEntityUUID(),
			Position: position,
			Chunk:    positionLoadedChunk(position),
		},
		Living: RuntimeLivingState{
			KnockbackResistance: zombieKnockbackResistance,
			Width:               definition.Width,
			Height:              definition.Height,
			Armor:               zombieArmor,
		},
		Melee: zombieMeleeState{LastCanUseCheck: -20},
	}

	entity.Living.Reset(zombieMaxHealth)
	entity.State.tracker = newRuntimeEntityTracker(entity.runtimeEntityViewLocked())

	r.entityMu.Lock()
	r.entities[entity.State.ID] = entity
	r.addEntityToChunkIndexLocked(entity)
	r.entityMu.Unlock()

	chunk, active := r.ActiveChunk(entity.State.Chunk)
	if active {
		chunk.SetEntity(entity.State.ID, entity)
	}

	r.reconcileRuntimeEntityTracking(entity)

	return entity
}

func (r *Runtime) nearestZombieTarget(position game.Position) *Session {
	var nearest *Session

	nearestDistance := float64(zombieFollowRange * zombieFollowRange)

	for _, session := range r.snapshotSessions() {
		player := session.snapshotPlayer()
		if !zombieTargetAcquirable(player, position) || !r.zombieHasLineOfSight(position, player.Position) {
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

func (r *Runtime) zombieTarget(entity *runtimeZombieEntity) *Session {
	position := entity.State.Position
	current := entity.Target.Session

	if current != nil {
		connected := slices.Contains(r.snapshotSessions(), current)
		player := current.snapshotPlayer()

		if connected && zombieTargetRetained(player, position) {
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

	for _, session := range r.snapshotSessions() {
		player := session.snapshotPlayer()
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
	from.Y += 1.74
	to.Y += 1.62

	deltaX := to.X - from.X
	deltaY := to.Y - from.Y
	deltaZ := to.Z - from.Z

	minimumX := int32(math.Floor(min(from.X, to.X)))
	minimumY := int32(math.Floor(min(from.Y, to.Y)))
	minimumZ := int32(math.Floor(min(from.Z, to.Z)))
	maximumX := int32(math.Floor(max(from.X, to.X)))
	maximumY := int32(math.Floor(max(from.Y, to.Y)))
	maximumZ := int32(math.Floor(max(from.Z, to.Z)))

	for x := minimumX; x <= maximumX; x++ {
		for y := minimumY; y <= maximumY; y++ {
			for z := minimumZ; z <= maximumZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}
				block := r.World.BlockAt(position)

				for _, box := range block.CollisionBoxes(position) {
					distance, _, intersects := raycastAABB(from, deltaX, deltaY, deltaZ, box)
					if intersects && distance >= 0 && distance <= 1 {
						return false
					}
				}
			}
		}
	}

	return true
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

func zombieTargetAcquirable(player game.Player, zombiePosition game.Position) bool {
	return zombieTargetRetained(player, zombiePosition)
}

func zombieTargetRetained(player game.Player, zombiePosition game.Position) bool {
	if player.Dead || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator {
		return false
	}

	return distanceSquared(zombiePosition, player.Position) <= zombieFollowRange*zombieFollowRange
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
