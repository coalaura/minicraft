package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	zombieMaxHealth             = 20
	zombieMovementSpeed         = 0.23
	zombieMovementAcceleration  = 0.08
	zombieAttackDamage          = 3
	zombieArmor                 = 2
	zombieKnockbackResistance   = 0
	zombieFollowRange           = 35
	zombieGravity               = 0.08
	zombieGroundFriction        = 0.91
	zombieAirFriction           = 0.91
	zombieVerticalDrag          = 0.98
	zombieJumpStrength          = 0.42
	zombieStepHeight            = 0.6
	zombieAttackInterval        = 20
	zombieAttackReachExpansion  = 0.8284271247461903
	zombiePathRefreshTicks      = 10
	zombiePathWaypointTolerance = 0.2
	zombieTrackingRangeChunks   = 8
	zombieTrackingInterval      = 3
	zombieLootPickupDelay       = 10
)

type runtimeZombieEntity struct {
	State    RuntimeEntityState
	Living   RuntimeLivingState
	Rotation game.Rotation

	Target             *Session
	Path               []game.Position
	PathIndex          int
	PathRefresh        int32
	AttackCooldown     int32
	TickCount          int32
	Aggressive         bool
	LootDropped        bool
	LastPathTargetNode game.BlockPosition
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

	target := runtime.zombieTarget(entity)

	entity.updatePath(runtime, target)
	entity.tickMovement(runtime, target)
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

func (entity *runtimeZombieEntity) updatePath(runtime *Runtime, target *Session) {
	entity.State.mu.Lock()

	entity.TickCount++

	if entity.AttackCooldown > 0 {
		entity.AttackCooldown--
	}

	aggressive := target != nil
	if entity.Aggressive != aggressive {
		entity.Aggressive = aggressive
		entity.State.metadataDirty = true
	}

	if target == nil {
		entity.Path = nil
		entity.PathIndex = 0
		entity.PathRefresh = 0

		entity.State.mu.Unlock()

		return
	}

	targetPlayer := target.snapshotPlayer()

	targetNode := game.BlockPosition{X: int32(math.Floor(targetPlayer.Position.X)), Y: int32(math.Floor(targetPlayer.Position.Y)), Z: int32(math.Floor(targetPlayer.Position.Z))}

	refresh := entity.PathRefresh <= 0 || targetNode != entity.LastPathTargetNode || entity.PathIndex >= len(entity.Path)
	start := entity.State.Position

	if entity.PathRefresh > 0 {
		entity.PathRefresh--
	}

	entity.State.mu.Unlock()

	if !refresh {
		return
	}

	path := runtime.findGroundPath(start, targetPlayer.Position, entity.Living.Width, entity.Living.Height, zombieFollowRange)

	entity.State.mu.Lock()
	entity.Path = path
	entity.PathIndex = 0
	entity.PathRefresh = zombiePathRefreshTicks
	entity.LastPathTargetNode = targetNode
	entity.State.mu.Unlock()
}

func (entity *runtimeZombieEntity) tickMovement(runtime *Runtime, target *Session) {
	entity.State.mu.Lock()

	previous := entity.State.Position
	directionX := 0.0
	directionZ := 0.0
	nextY := entity.State.Position.Y

	for entity.PathIndex < len(entity.Path) {
		waypoint := entity.Path[entity.PathIndex]

		deltaX := waypoint.X - entity.State.Position.X
		deltaZ := waypoint.Z - entity.State.Position.Z

		if deltaX*deltaX+deltaZ*deltaZ > zombiePathWaypointTolerance*zombiePathWaypointTolerance {
			directionX = deltaX
			directionZ = deltaZ
			nextY = waypoint.Y

			break
		}

		entity.PathIndex++
	}

	if directionX == 0 && directionZ == 0 && target != nil {
		player := target.snapshotPlayer()

		directionX = player.Position.X - entity.State.Position.X
		directionZ = player.Position.Z - entity.State.Position.Z
	}

	length := math.Hypot(directionX, directionZ)

	if length > 0 {
		acceleration := zombieMovementAcceleration

		if entity.Living.OnGround {
			friction := float64(runtime.blockFrictionBelow(entity.State.Position) * zombieGroundFriction)
			acceleration = zombieMovementSpeed * (0.16277137 / (friction * friction * friction))
		}

		entity.Living.Velocity.X += directionX / length * acceleration
		entity.Living.Velocity.Z += directionZ / length * acceleration
		entity.Rotation.Yaw = float32(math.Atan2(-directionX, directionZ) * 180 / math.Pi)
		entity.Rotation.HeadYaw = entity.Rotation.Yaw
	}

	if entity.Living.OnGround && nextY > entity.State.Position.Y+0.25 {
		entity.Living.Velocity.Y = zombieJumpStrength
	}

	movement := runtime.moveGroundEntity(entity.State.Position, entity.Living.Velocity, entity.Living.Width, entity.Living.Height, zombieStepHeight, entity.Living.OnGround)

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

	if entity.Living.OnGround {
		horizontalDrag = float64(runtime.blockFrictionBelow(entity.State.Position) * zombieGroundFriction)
	}

	entity.Living.Velocity.X *= horizontalDrag
	entity.Living.Velocity.Y *= zombieVerticalDrag
	entity.Living.Velocity.Z *= horizontalDrag

	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)
}

func (entity *runtimeZombieEntity) tickAttack(runtime *Runtime, target *Session) {
	if target == nil {
		return
	}

	player := target.snapshotPlayer()

	entity.State.mu.Lock()

	if entity.AttackCooldown > 0 || !zombieMeleeBoxesIntersect(entity.Living.CollisionBox(entity.State.Position), player.CollisionBox()) {
		entity.State.mu.Unlock()

		return
	}

	position := entity.State.Position
	entityID := entity.State.ID

	entity.State.mu.Unlock()

	if !runtime.zombieHasLineOfSight(position, player.Position) {
		return
	}

	entity.State.mu.Lock()
	entity.AttackCooldown = zombieAttackInterval
	entity.State.mu.Unlock()

	runtime.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{
		ID:      protocol.ClientboundEntityAnimationID,
		Encoder: protocol.EntityAnimation{EntityID: entityID, Animation: protocol.EntityAnimationSwingMainHand},
	})

	damage := game.Damage{Type: game.DamageMobAttack, Amount: zombieAttackDamage, CauseEntityID: entityID, DirectEntityID: entityID}

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

func (r *Runtime) zombieTarget(entity *runtimeZombieEntity) *Session {
	entity.State.mu.RLock()
	current := entity.Target
	position := entity.State.Position
	entity.State.mu.RUnlock()

	sessions := r.snapshotSessions()
	if current != nil && slices.Contains(sessions, current) && zombieTargetValid(current.snapshotPlayer(), position) {
		return current
	}

	var nearest *Session

	nearestDistance := float64(zombieFollowRange * zombieFollowRange)

	for _, session := range sessions {
		player := session.snapshotPlayer()
		if !zombieTargetValid(player, position) {
			continue
		}

		distance := horizontalDistanceSquared(position, player.Position)
		if distance < nearestDistance {
			nearest = session
			nearestDistance = distance
		}
	}

	entity.State.mu.Lock()
	entity.Target = nearest
	entity.State.mu.Unlock()

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

func zombieTargetValid(player game.Player, zombiePosition game.Position) bool {
	if player.Dead || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator {
		return false
	}

	return horizontalDistanceSquared(zombiePosition, player.Position) <= zombieFollowRange*zombieFollowRange
}

func zombieMeleeBoxesIntersect(zombie, target game.AABB) bool {
	zombie.MinX -= zombieAttackReachExpansion
	zombie.MinZ -= zombieAttackReachExpansion
	zombie.MaxX += zombieAttackReachExpansion
	zombie.MaxZ += zombieAttackReachExpansion

	return zombie.Intersects(target)
}

func horizontalDistanceSquared(first, second game.Position) float64 {
	deltaX := first.X - second.X
	deltaZ := first.Z - second.Z

	return deltaX*deltaX + deltaZ*deltaZ
}
