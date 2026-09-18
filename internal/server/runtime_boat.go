package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	boatWidth                     = 1.375
	boatHeight                    = 0.5625
	boatGravity                   = -0.04
	boatWaterMomentum             = 0.9
	boatAirMomentum               = 0.9
	boatUnderwaterMomentum        = 0.45
	boatUnderwaterGravity         = 0.01
	boatBuoyancy                  = 0.06153846
	boatPassengerAttachmentHeight = 0.6
	boatMaximumClientMoveSquared  = 100
	boatMovementResidualSquared   = 0.0625
)

type boatStatus uint8

const (
	boatStatusAir boatStatus = iota
	boatStatusWater
	boatStatusUnderFlowingWater
	boatStatusUnderWater
	boatStatusLand
)

type runtimeBoatEntity struct {
	State RuntimeEntityState

	Velocity          game.Velocity
	Rotation          game.Rotation
	DeltaRotation     float32
	WaterLevel        float64
	LandFriction      float64
	Damage            float32
	HurtTime          int32
	HurtDirection     int32
	BubbleTime        int32
	OutOfControlTicks int32
	Status            boatStatus
	Previous          boatStatus
	LeftPaddle        bool
	RightPaddle       bool
	Raft              bool
	Drop              game.Item
}

func (entity *runtimeBoatEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (*runtimeBoatEntity) maximumPassengers() int {
	return maximumVehiclePassengers
}

func (*runtimeBoatEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: 10, UpdateInterval: 3, TrackDeltas: true}
}

func (entity *runtimeBoatEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeBoatEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.entityMetadataLocked()
}

func (entity *runtimeBoatEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Velocity
}

func (entity *runtimeBoatEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{EntityID: snapshot.ID, UUID: snapshot.UUID, Type: int32(entity.State.Type), X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, VelocityX: snapshot.Velocity.X, VelocityY: snapshot.Velocity.Y, VelocityZ: snapshot.Velocity.Z, Yaw: snapshot.Yaw, Pitch: snapshot.Pitch}
}

func (entity *runtimeBoatEntity) RuntimeEntityInteract(runtime *Runtime, session *Session, interaction RuntimeEntityInteraction) bool {
	entity.State.mu.RLock()
	vehicleID := entity.State.ID
	removed := entity.State.Removed
	outOfControlTicks := entity.OutOfControlTicks
	entity.State.mu.RUnlock()

	if removed || interaction.SecondaryAction || outOfControlTicks >= 60 || runtime.boatEyeInWater(entity) {
		return false
	}

	mounted := runtime.mountPassengerLocked(vehicleID, session.passengerID())
	if mounted {
		entity.State.mu.RLock()
		position := entity.State.Position
		yaw := entity.Rotation.Yaw
		raft := entity.Raft
		entity.State.mu.RUnlock()

		runtime.updateBoatPassengerPositionsLocked(vehicleID, position, yaw, raft, true)
	}

	return mounted
}

func (entity *runtimeBoatEntity) RuntimeEntityAttack(runtime *Runtime, attacker *Session) {
	player := attacker.snapshotPlayer()

	strength := player.AttackStrength()
	damage := player.MainHandAttackDamage() * (0.2 + 0.8*strength*strength)

	if strength > playerFullAttackStrength && runtime.playerCanCriticalAttack(player) {
		damage *= playerCriticalDamageMultiplier
	}

	creative := player.GameMode == game.GameModeCreative

	player, _ = attacker.updatePlayerState(func(player *game.Player) bool {
		player.ResetAttackStrength()

		player.AddExhaustion(playerAttackExhaustion)

		return true
	})

	held := player.Inventory.Held(player.SelectedHotbarSlot)

	var heldItem game.ItemStack

	if held != nil {
		heldItem = held.Clone()
	}

	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	entity.HurtDirection = -entity.HurtDirection
	entity.HurtTime = 10
	entity.Damage += damage * 10
	entity.State.metadataDirty = true
	entityID := entity.State.ID
	position := entity.State.Position
	destroyed := creative || entity.Damage > 40
	entity.State.mu.Unlock()

	if destroyed {
		if !creative {
			runtime.SpawnItemEntity(game.ItemStack{Item: entity.Drop, Count: 1}, position, game.Velocity{}, 10)
		}

		runtime.removeRuntimeEntity(entityID)
	} else {
		runtime.synchronizeDirtyRuntimeEntityMetadata(entity)
	}

	if player.GameMode != game.GameModeCreative {
		damagePerAttack := player.MainHandDamagePerAttack()
		if damagePerAttack > 0 {
			before, broke := runtime.damageHeldItem(attacker, protocol.MainHand, heldItem, damagePerAttack)
			if before != nil {
				runtime.sendPlayerAttackInventoryUpdate(attacker, *before, broke)
			}
		}
	}
}

func (entity *runtimeBoatEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	previousPosition := entity.State.Position

	entity.Previous = entity.Status
	entity.Status, entity.WaterLevel, entity.LandFriction = runtime.boatStatus(entity.State.Position)

	if entity.Status == boatStatusUnderWater || entity.Status == boatStatusUnderFlowingWater {
		entity.OutOfControlTicks++
	} else {
		entity.OutOfControlTicks = 0
	}

	ejectPassengers := entity.OutOfControlTicks >= 60

	controlledByPlayer := runtime.boatController(entity.State.ID) != nil
	if controlledByPlayer {
		entity.Velocity = game.Velocity{}
	} else {
		if entity.LeftPaddle || entity.RightPaddle {
			entity.LeftPaddle = false
			entity.RightPaddle = false
			entity.State.metadataDirty = true
		}

		entity.floatBoatLocked(runtime, false)

		movement := runtime.moveGroundEntity(entity.State.Position, entity.Velocity, boatWidth, boatHeight, 0, false)
		entity.State.Position = movement.Position

		if movement.HorizontalCollisionX {
			entity.Velocity.X = 0
		}

		if movement.HorizontalCollisionZ {
			entity.Velocity.Z = 0
		}

		if movement.VerticalCollision {
			entity.Velocity.Y = 0
		}
	}

	if entity.HurtTime > 0 {
		entity.HurtTime--
		entity.State.metadataDirty = true
	}

	if entity.Damage > 0 {
		entity.Damage--
		entity.State.metadataDirty = true
	}

	entity.State.movementSyncDirty = previousPosition != entity.State.Position
	entityID := entity.State.ID
	position := entity.State.Position
	rotation := entity.Rotation
	entity.State.mu.Unlock()

	if ejectPassengers {
		runtime.removePassenger(entityID)
	}

	runtime.runtimeEntityMoved(entity, previousPosition)

	runtime.updateBoatPassengerPositionsLocked(entityID, position, rotation.Yaw, entity.Raft, true)
	runtime.pushBoatEntities(entityID, position)

	runtime.synchronizeRuntimeEntity(entity)
}

func (runtime *Runtime) SpawnBoat(entityType game.EntityType, position game.Position) *runtimeBoatEntity {
	return runtime.spawnBoat(entityType, position, 0)
}

func (runtime *Runtime) spawnBoat(entityType game.EntityType, position game.Position, yaw float32) *runtimeBoatEntity {
	drop, raft, valid := boatVariant(entityType)
	if !valid {
		return nil
	}

	entity := &runtimeBoatEntity{Rotation: game.Rotation{Yaw: yaw}, Drop: drop, Raft: raft, HurtDirection: 1, Status: boatStatusAir, Previous: boatStatusAir}

	runtime.registerRuntimeEntity(entity, entityType, position)

	return entity
}

func (runtime *Runtime) boatStatus(position game.Position) (boatStatus, float64, float64) {
	box := entityBox(position, boatWidth, boatHeight)

	minimumX := int32(math.Floor(box.MinX + 0.001))
	maximumX := int32(math.Floor(box.MaxX - 0.001))
	minimumZ := int32(math.Floor(box.MinZ + 0.001))
	maximumZ := int32(math.Floor(box.MaxZ - 0.001))

	topY := int32(math.Floor(box.MaxY + 0.001))

	underwater := false

	for y := int32(math.Floor(box.MaxY)); y <= topY; y++ {
		for x := minimumX; x <= maximumX; x++ {
			for z := minimumZ; z <= maximumZ; z++ {
				block := game.BlockPosition{X: x, Y: y, Z: z}

				fluid := runtime.World.FluidAt(block)
				if fluid.Type() != game.FluidTypeWater {
					continue
				}

				level := float64(y) + fluid.Height(runtime.World, block)
				if level < box.MaxY+0.001 {
					continue
				}

				if !fluid.IsSource() {
					return boatStatusUnderFlowingWater, 0, 0
				}

				underwater = true
			}
		}
	}

	if underwater {
		return boatStatusUnderWater, 0, 0
	}

	waterLevel := -math.MaxFloat64

	minimumY := int32(math.Floor(box.MinY))
	maximumY := int32(math.Floor(box.MinY + 0.001))

	for y := minimumY; y <= maximumY; y++ {
		for x := minimumX; x <= maximumX; x++ {
			for z := minimumZ; z <= maximumZ; z++ {
				block := game.BlockPosition{X: x, Y: y, Z: z}

				fluid := runtime.World.FluidAt(block)
				if fluid.Type() == game.FluidTypeWater {
					waterLevel = max(waterLevel, float64(y)+fluid.Height(runtime.World, block))
				}
			}
		}
	}

	if waterLevel > box.MinY {
		return boatStatusWater, waterLevel, 0
	}

	friction := runtime.boatLandFriction(box)
	if friction > 0 {
		return boatStatusLand, 0, friction
	}

	return boatStatusAir, 0, 0
}

func (runtime *Runtime) boatEyeInWater(entity *runtimeBoatEntity) bool {
	entity.State.mu.RLock()
	eye := entity.State.Position
	eye.Y += boatHeight
	entity.State.mu.RUnlock()

	block := game.BlockPosition{X: int32(math.Floor(eye.X)), Y: int32(math.Floor(eye.Y)), Z: int32(math.Floor(eye.Z))}

	fluid := runtime.World.FluidAt(block)

	return fluid.Type() == game.FluidTypeWater && eye.Y < float64(block.Y)+fluid.Height(runtime.World, block)
}

func (entity *runtimeBoatEntity) floatBoatLocked(runtime *Runtime, controlledByPlayer bool) {
	gravity := boatGravity
	momentum := boatAirMomentum
	buoyancy := 0.0

	switch entity.Status {
	case boatStatusWater:
		buoyancy = (entity.WaterLevel - entity.State.Position.Y) / boatHeight
		momentum = boatWaterMomentum
	case boatStatusUnderFlowingWater:
		gravity = -0.0007
		momentum = boatWaterMomentum
	case boatStatusUnderWater:
		buoyancy = boatUnderwaterGravity
		momentum = boatUnderwaterMomentum
	case boatStatusLand:
		momentum = entity.LandFriction

		if controlledByPlayer {
			entity.LandFriction *= 0.5
		}
	}

	// Vanilla snaps an airboat entering any water state to a collision-free surface position.
	if entity.Previous == boatStatusAir && entity.Status != boatStatusAir && entity.Status != boatStatusLand {
		entity.WaterLevel = entity.State.Position.Y + boatHeight
		targetY := entity.waterLevelAboveLocked(runtime) - boatHeight + 0.101
		target := entity.State.Position
		target.Y = targetY

		if runtime.boatPositionClear(target) {
			entity.State.Position.Y = targetY
			entity.Velocity.Y = 0
		}

		entity.Status = boatStatusWater

		return
	}

	entity.Velocity.X *= momentum
	entity.Velocity.Z *= momentum
	entity.Velocity.Y += gravity
	entity.DeltaRotation *= float32(momentum)

	if buoyancy > 0 {
		entity.Velocity.Y = (entity.Velocity.Y + buoyancy*boatBuoyancy) * 0.75
	}
}

func (entity *runtimeBoatEntity) waterLevelAboveLocked(runtime *Runtime) float64 {
	box := entityBox(entity.State.Position, boatWidth, boatHeight)

	minimumX := int32(math.Floor(box.MinX))
	maximumX := int32(math.Ceil(box.MaxX))
	minimumY := int32(math.Floor(box.MaxY))
	maximumY := int32(math.Ceil(box.MaxY - entity.Velocity.Y))
	minimumZ := int32(math.Floor(box.MinZ))
	maximumZ := int32(math.Ceil(box.MaxZ))

	for y := minimumY; y < maximumY; y++ {
		blockHeight := 0.0

		for x := minimumX; x < maximumX; x++ {
			for z := minimumZ; z < maximumZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}
				fluid := runtime.World.FluidAt(position)

				if fluid.Type() == game.FluidTypeWater {
					blockHeight = max(blockHeight, fluid.Height(runtime.World, position))
				}
			}
		}

		if blockHeight < 1 {
			return float64(y) + blockHeight
		}
	}

	return float64(maximumY + 1)
}

func (entity *runtimeBoatEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{ID: entity.State.ID, UUID: entity.State.UUID, Position: entity.State.Position, Chunk: entity.State.Chunk, Removed: entity.State.Removed, Velocity: entity.Velocity, Rotation: entity.Rotation}
}

func (entity *runtimeBoatEntity) entityMetadataLocked() []protocol.EntityMetadataEntry {
	return []protocol.EntityMetadataEntry{
		{Index: 8, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.HurtTime)},
		{Index: 9, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.HurtDirection)},
		{Index: 10, Type: protocol.MetadataTypeFloat, Value: protocol.MetadataFloat(entity.Damage)},
		{Index: 11, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(entity.LeftPaddle)},
		{Index: 12, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(entity.RightPaddle)},
		{Index: 13, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.BubbleTime)},
	}
}

func (entity *runtimeBoatEntity) safeDismountPosition(runtime *Runtime, passenger any) game.Position {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	passengerWidth, passengerHeight, yaw := boatDismountParameters(passenger)
	distance := (boatWidth*math.Sqrt2 + passengerWidth + 0.00001) / 2
	angle := float64(yaw) * math.Pi / 180
	directionX := -math.Sin(angle)
	directionZ := math.Cos(angle)
	maximum := max(math.Abs(directionX), math.Abs(directionZ))

	if maximum > 0 {
		directionX /= maximum
		directionZ /= maximum
	}

	targetX := position.X + directionX*distance
	targetZ := position.Z + directionZ*distance

	targetBlock := game.BlockPosition{X: int32(math.Floor(targetX)), Y: int32(math.Floor(position.Y + boatHeight)), Z: int32(math.Floor(targetZ))}

	belowBlock := targetBlock
	belowBlock.Y--

	if runtime.World.FluidAt(belowBlock).Type() != game.FluidTypeWater {
		blocks := [...]game.BlockPosition{targetBlock, belowBlock}

		for _, block := range blocks {
			floorHeight, valid := runtime.boatDismountFloorHeight(block)
			if !valid {
				continue
			}

			candidate := game.Position{X: targetX, Y: float64(block.Y) + floorHeight, Z: targetZ}
			if runtime.boatDismountPositionClear(candidate, passengerWidth, passengerHeight) {
				return candidate
			}
		}
	}

	return game.Position{X: position.X, Y: position.Y + boatHeight, Z: position.Z}
}

func (runtime *Runtime) boatLandFriction(box game.AABB) float64 {
	contact := game.AABB{MinX: box.MinX, MinY: box.MinY - 0.001, MinZ: box.MinZ, MaxX: box.MaxX, MaxY: box.MinY, MaxZ: box.MaxZ}

	minimumX := int32(math.Floor(contact.MinX)) - 1
	maximumX := int32(math.Ceil(contact.MaxX)) + 1
	minimumY := int32(math.Floor(contact.MinY)) - 1
	maximumY := int32(math.Ceil(contact.MaxY)) + 1
	minimumZ := int32(math.Floor(contact.MinZ)) - 1
	maximumZ := int32(math.Ceil(contact.MaxZ)) + 1

	var (
		collisionBuffer [7]game.AABB
		friction        float64
		contacts        int
	)

	for x := minimumX; x < maximumX; x++ {
		for z := minimumZ; z < maximumZ; z++ {
			edges := 0

			if x == minimumX || x == maximumX-1 {
				edges++
			}

			if z == minimumZ || z == maximumZ-1 {
				edges++
			}

			if edges == 2 {
				continue
			}

			for y := minimumY; y < maximumY; y++ {
				if edges > 0 && (y == minimumY || y == maximumY-1) {
					continue
				}

				position := game.BlockPosition{X: x, Y: y, Z: z}

				block := runtime.World.BlockAt(position)
				if block == game.LilyPad {
					continue
				}

				boxes := block.AppendCollisionBoxes(collisionBuffer[:0], position)

				collides := slices.ContainsFunc(boxes, contact.Intersects)
				if collides {
					friction += boatBlockFriction(block)
					contacts++
				}
			}
		}
	}

	if contacts == 0 {
		return 0
	}

	return friction / float64(contacts)
}

func (runtime *Runtime) boatDismountPositionClear(position game.Position, width, height float64) bool {
	box := entityBox(position, width, height)

	var collisionBuffer [128]game.AABB

	boxes := runtime.appendEntityCollisionBoxes(collisionBuffer[:0], box, game.Velocity{})

	return !slices.ContainsFunc(boxes, box.Intersects)
}

func (runtime *Runtime) boatPositionClear(position game.Position) bool {
	box := entityBox(position, boatWidth, boatHeight)

	var collisionBuffer [128]game.AABB

	boxes := runtime.appendEntityCollisionBoxes(collisionBuffer[:0], box, game.Velocity{})

	return !slices.ContainsFunc(boxes, box.Intersects)
}

func (runtime *Runtime) boatPlacementClear(position game.Position) bool {
	if !runtime.boatPositionClear(position) {
		return false
	}

	box := entityBox(position, boatWidth, boatHeight)
	iterator := runtime.runtimeEntitiesInBox(box)

	for {
		candidate, present := iterator.Next()
		if !present {
			break
		}

		state := candidate.RuntimeEntityState()

		state.mu.RLock()
		candidatePosition := state.Position
		removed := state.Removed
		state.mu.RUnlock()

		if removed {
			continue
		}

		switch entity := candidate.(type) {
		case *runtimeBoatEntity:
			if box.Intersects(entityBox(candidatePosition, boatWidth, boatHeight)) {
				return false
			}
		case RuntimeLivingEntity:
			if box.Intersects(entity.RuntimeLivingState().CollisionBox(candidatePosition)) {
				return false
			}
		}
	}

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if player.GameMode != game.GameModeSpectator && box.Intersects(player.collisionBox()) {
			return false
		}
	}

	return true
}

func (runtime *Runtime) boatDismountFloorHeight(position game.BlockPosition) (float64, bool) {
	var collisionBuffer [7]game.AABB

	boxes := runtime.World.BlockAt(position).AppendCollisionBoxes(collisionBuffer[:0], position)
	if len(boxes) != 0 {
		height := -math.MaxFloat64

		for _, box := range boxes {
			height = max(height, box.MaxY-float64(position.Y))
		}

		return height, height < 1
	}

	below := position
	below.Y--

	boxes = runtime.World.BlockAt(below).AppendCollisionBoxes(collisionBuffer[:0], below)
	if len(boxes) == 0 {
		return 0, false
	}

	height := -math.MaxFloat64

	for _, box := range boxes {
		height = max(height, box.MaxY-float64(position.Y))
	}

	return height, height < 1
}

func boatVariant(entityType game.EntityType) (game.Item, bool, bool) {
	switch entityType {
	case game.EntityAcaciaBoat:
		return game.ItemAcaciaBoat, false, true
	case game.EntityBirchBoat:
		return game.ItemBirchBoat, false, true
	case game.EntityCherryBoat:
		return game.ItemCherryBoat, false, true
	case game.EntityDarkOakBoat:
		return game.ItemDarkOakBoat, false, true
	case game.EntityJungleBoat:
		return game.ItemJungleBoat, false, true
	case game.EntityMangroveBoat:
		return game.ItemMangroveBoat, false, true
	case game.EntityOakBoat:
		return game.ItemOakBoat, false, true
	case game.EntityPaleOakBoat:
		return game.ItemPaleOakBoat, false, true
	case game.EntitySpruceBoat:
		return game.ItemSpruceBoat, false, true
	case game.EntityBambooRaft:
		return game.ItemBambooRaft, true, true
	default:
		return game.ItemAir, false, false
	}
}

func boatBlockFriction(block game.Block) float64 {
	definition, valid := block.Definition()
	if !valid {
		return 0.6
	}

	switch definition.Name {
	case "blue_ice":
		return 0.989
	case "ice", "packed_ice", "frosted_ice":
		return 0.98
	case "slime_block":
		return 0.8
	default:
		return 0.6
	}
}

func boatPassengerPosition(position game.Position, yaw float32, index, count int, raft bool) game.Position {
	return boatPassengerPositionFor(position, yaw, index, count, raft, false)
}

func boatPassengerPositionFor(position game.Position, yaw float32, index, count int, raft, animal bool) game.Position {
	offset := 0.0

	if count > 1 {
		if index == 0 {
			offset = 0.2
		} else {
			offset = -0.6
		}

		if animal {
			offset += 0.2
		}
	}

	attachmentHeight := boatHeight / 3

	if raft {
		attachmentHeight = boatHeight * 0.8888889
	}

	angle := float64(yaw) * math.Pi / 180

	return game.Position{
		X: position.X - math.Sin(angle)*offset,
		Y: position.Y + attachmentHeight - boatPassengerAttachmentHeight,
		Z: position.Z + math.Cos(angle)*offset,
	}
}

func boatDismountParameters(passenger any) (float64, float64, float32) {
	switch entity := passenger.(type) {
	case *Session:
		player := entity.playerView()

		return 0.6, 1.8, player.Rotation.Yaw
	case RuntimeLivingEntity:
		state := entity.RuntimeEntityState()

		state.mu.RLock()
		width := entity.RuntimeLivingState().Width
		height := entity.RuntimeLivingState().Height
		state.mu.RUnlock()

		tracker, tracked := entity.(RuntimeEntityTracker)
		if !tracked {
			return width, height, 0
		}

		view := tracker.RuntimeEntityView()

		return width, height, view.Rotation.Yaw
	default:
		return 0.6, 1.8, 0
	}
}

func (runtime *Runtime) boatPassengersChangedLocked(vehicleID int32, worldLocked bool) {
	runtime.entityMu.RLock()
	boat, valid := runtime.entities[vehicleID].(*runtimeBoatEntity)
	runtime.entityMu.RUnlock()

	if !valid {
		return
	}

	boat.State.mu.Lock()
	position := boat.State.Position
	yaw := boat.Rotation.Yaw
	raft := boat.Raft

	if boat.LeftPaddle || boat.RightPaddle {
		boat.LeftPaddle = false
		boat.RightPaddle = false
		boat.State.metadataDirty = true
	}

	boat.State.mu.Unlock()

	runtime.updateBoatPassengerPositionsLocked(vehicleID, position, yaw, raft, worldLocked)
	runtime.synchronizeDirtyRuntimeEntityMetadata(boat)
}

func (runtime *Runtime) updateBoatPassengerPositionsLocked(vehicleID int32, position game.Position, yaw float32, raft, worldLocked bool) {
	runtime.passengerMu.RLock()
	passengers := runtime.vehiclePassengers[vehicleID]
	runtime.passengerMu.RUnlock()

	for index := 0; index < passengers.count; index++ {
		passenger := runtime.passengerForID(passengers.ids[index])
		if passenger == nil {
			continue
		}

		_, animal := passenger.(interface {
			runtimeBoatAnimal()
		})

		passengerPosition := boatPassengerPositionFor(position, yaw, index, passengers.count, raft, animal)

		runtime.updatePassengerPositionLocked(passenger, passengerPosition)

		session, playerPassenger := passenger.(*Session)
		if playerPassenger {
			session.playerMx.Lock()
			session.Player.Rotation.Yaw = clampAngleAround(session.Player.Rotation.Yaw, yaw, 105)
			session.playerMx.Unlock()

			runtime.updateMountedPlayerChunks(session, worldLocked)
		}
	}
}

func (runtime *Runtime) updateMountedPlayerChunks(session *Session, worldLocked bool) {
	session.chunkMx.Lock()
	initialized := session.hasChunkCenter
	center := session.centerChunk
	session.chunkMx.Unlock()

	if !initialized || center == positionLoadedChunk(session.playerView().Position) {
		return
	}

	err := session.updatePlayerChunksWithWorldLock(worldLocked)
	if err != nil && session.Log != nil {
		session.Log.Warnf("[play] failed to update mounted player chunks: %v\n", err)
	}
}

func (runtime *Runtime) pushBoatEntities(vehicleID int32, position game.Position) {
	boatBox := entityBox(position, boatWidth, boatHeight)

	queryBox := game.AABB{
		MinX: boatBox.MinX - 0.2,
		MinY: boatBox.MinY + 0.01,
		MinZ: boatBox.MinZ - 0.2,
		MaxX: boatBox.MaxX + 0.2,
		MaxY: boatBox.MaxY - 0.01,
		MaxZ: boatBox.MaxZ + 0.2,
	}

	addPassengers := runtime.boatController(vehicleID) == nil

	runtime.entityMu.RLock()
	boat, _ := runtime.entities[vehicleID].(*runtimeBoatEntity)
	runtime.entityMu.RUnlock()

	if boat == nil || runtime.boatEyeInWater(boat) {
		addPassengers = false
	}

	for iterator := runtime.runtimeEntitiesInBox(queryBox); ; {
		candidate, present := iterator.Next()
		if !present {
			break
		}

		state := candidate.RuntimeEntityState()

		state.mu.RLock()
		candidateID := state.ID
		state.mu.RUnlock()

		if candidateID == vehicleID || runtime.passengersShareRootVehicle(vehicleID, candidateID) {
			continue
		}

		living, livingEntity := candidate.(RuntimeLivingEntity)
		if !livingEntity {
			continue
		}

		state.mu.Lock()
		livingState := living.RuntimeLivingState()
		candidatePosition := state.Position
		candidateBox := livingState.CollisionBox(candidatePosition)

		if state.Removed || livingState.Dead || !queryBox.Intersects(candidateBox) {
			state.mu.Unlock()

			continue
		}

		canBoard := addPassengers && livingState.Width < boatWidth && !state.Type.CannotBePushedOntoBoats()
		state.mu.Unlock()

		if canBoard && runtime.mountPassengerLocked(vehicleID, candidateID) {
			continue
		}

		if candidateBox.MinY > boatBox.MinY {
			continue
		}

		state.mu.Lock()
		livingState = living.RuntimeLivingState()
		candidatePosition = state.Position

		if state.Removed || livingState.Dead {
			state.mu.Unlock()

			continue
		}

		impulse, applied := boatPushImpulse(position, candidatePosition)
		if applied {
			livingState.Velocity.X += impulse.X
			livingState.Velocity.Z += impulse.Z
			state.movementSyncDirty = true
		}

		state.mu.Unlock()

		if applied {
			runtime.synchronizeRuntimeEntity(candidate)
		}
	}

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if player.Dead || player.GameMode == game.GameModeSpectator || runtime.passengersShareRootVehicle(vehicleID, player.EntityID) {
			continue
		}

		playerBox := player.collisionBox()
		if playerBox.MinY > boatBox.MinY || !queryBox.Intersects(playerBox) {
			continue
		}

		impulse, applied := boatPushImpulse(position, player.Position)
		if !applied {
			continue
		}

		updated, changed := session.updatePlayerState(func(target *game.Player) bool {
			target.Velocity.X += impulse.X
			target.Velocity.Z += impulse.Z

			return true
		})

		if changed {
			runtime.sendPlayerKnockback(updated)
		}
	}
}

func (runtime *Runtime) passengersShareRootVehicle(firstID, secondID int32) bool {
	runtime.passengerMu.RLock()
	defer runtime.passengerMu.RUnlock()

	firstRoot := firstID

	for runtime.passengerVehicles[firstRoot] != 0 {
		firstRoot = runtime.passengerVehicles[firstRoot]
	}

	secondRoot := secondID

	for runtime.passengerVehicles[secondRoot] != 0 {
		secondRoot = runtime.passengerVehicles[secondRoot]
	}

	return firstRoot == secondRoot
}

func boatPushImpulse(source, target game.Position) (game.Velocity, bool) {
	directionX := target.X - source.X
	directionZ := target.Z - source.Z

	maximum := max(math.Abs(directionX), math.Abs(directionZ))
	if maximum < 0.01 || math.IsNaN(maximum) || math.IsInf(maximum, 0) {
		return game.Velocity{}, false
	}

	scale := math.Sqrt(maximum)
	directionX /= scale
	directionZ /= scale
	power := min(1/scale, 1) * 0.05

	impulse := game.Velocity{X: directionX * power, Z: directionZ * power}
	if math.IsNaN(impulse.X) || math.IsNaN(impulse.Z) || math.IsInf(impulse.X, 0) || math.IsInf(impulse.Z, 0) {
		return game.Velocity{}, false
	}

	return impulse, true
}

func (runtime *Runtime) boatController(vehicleID int32) *Session {
	runtime.passengerMu.RLock()
	passengers := runtime.vehiclePassengers[vehicleID]
	runtime.passengerMu.RUnlock()

	if passengers.count == 0 {
		return nil
	}

	for _, session := range runtime.sessionView() {
		if session.passengerID() == passengers.ids[0] {
			return session
		}
	}

	return nil
}

func (session *Session) handleMoveVehicle(move protocol.MoveVehicle) {
	if !validPlayerPosition(move.X, move.Y, move.Z) || !validPlayerRotation(move.Yaw, move.Pitch) {
		return
	}

	runtime := session.Runtime

	runtime.worldMutationMu.Lock()
	runtime.lifecycleMu.Lock()

	boat := runtime.controlledBoat(session)
	if boat == nil {
		runtime.lifecycleMu.Unlock()
		runtime.worldMutationMu.Unlock()

		return
	}

	boat.State.mu.Lock()
	previousPosition := boat.State.Position
	previousRotation := boat.Rotation

	delta := game.Velocity{X: move.X - previousPosition.X, Y: move.Y - previousPosition.Y, Z: move.Z - previousPosition.Z}

	movedDistanceSquared := delta.X*delta.X + delta.Y*delta.Y + delta.Z*delta.Z
	expectedDistanceSquared := boat.Velocity.X*boat.Velocity.X + boat.Velocity.Y*boat.Velocity.Y + boat.Velocity.Z*boat.Velocity.Z

	rejected := movedDistanceSquared-expectedDistanceSquared > boatMaximumClientMoveSquared
	if !rejected {
		movement := runtime.moveGroundEntity(previousPosition, delta, boatWidth, boatHeight, 0, false)
		residualX := move.X - movement.Position.X
		residualY := move.Y - movement.Position.Y
		residualZ := move.Z - movement.Position.Z
		residualSquared := residualX*residualX + residualY*residualY + residualZ*residualZ
		rejected = residualSquared > boatMovementResidualSquared
	}

	if !rejected {
		boat.State.Position = game.Position{X: move.X, Y: move.Y, Z: move.Z}
		boat.Rotation = game.Rotation{Yaw: move.Yaw, Pitch: move.Pitch}
		boat.State.movementSyncDirty = previousPosition != boat.State.Position
	} else {
		boat.Rotation = previousRotation
	}

	position := boat.State.Position
	rotation := boat.Rotation
	entityID := boat.State.ID
	boat.State.mu.Unlock()

	if !rejected {
		runtime.runtimeEntityMoved(boat, previousPosition)
		runtime.updateBoatPassengerPositionsLocked(entityID, position, rotation.Yaw, boat.Raft, true)
		runtime.pushBoatEntities(entityID, position)
		runtime.synchronizeRuntimeEntity(boat)
	}

	runtime.lifecycleMu.Unlock()
	runtime.worldMutationMu.Unlock()

	if rejected {
		err := session.writePacket(protocol.ClientboundMoveVehicleID, protocol.MoveVehicleCorrection{X: position.X, Y: position.Y, Z: position.Z, Yaw: rotation.Yaw, Pitch: rotation.Pitch})
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to correct vehicle movement: %v\n", err)
		}
	}
}

func (session *Session) handlePaddleBoat(paddle protocol.PaddleBoat) {
	boat := session.Runtime.controlledBoat(session)
	if boat == nil {
		return
	}

	boat.State.mu.Lock()
	boat.LeftPaddle = paddle.LeftPaddle
	boat.RightPaddle = paddle.RightPaddle
	boat.State.metadataDirty = true
	boat.State.mu.Unlock()
}

func (runtime *Runtime) controlledBoat(session *Session) *runtimeBoatEntity {
	vehicleID := session.VehicleID()
	if vehicleID == 0 || runtime.boatController(vehicleID) != session {
		return nil
	}

	runtime.entityMu.RLock()
	entity, _ := runtime.entities[vehicleID].(*runtimeBoatEntity)
	runtime.entityMu.RUnlock()

	return entity
}
