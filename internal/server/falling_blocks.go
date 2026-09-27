package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	fallingBlockScheduleDelay = 2
	fallingBlockGravity       = 0.04
	fallingBlockDrag          = 0.98
	fallingBlockGroundDrag    = 0.7
	fallingBlockGroundBounce  = -0.5
	fallingBlockMaximumTime   = 600
	fallingBlockVoidTime      = 100
	fallingBlockDropDelay     = 10
	fallingBlockWaterRaySpeed = 1
)

type runtimeFallingBlockEntity struct {
	State RuntimeEntityState

	Velocity game.Velocity
	Block    game.Block
	Start    game.BlockPosition

	Time         int32
	FallDistance float32
	OnGround     bool
	DropItem     bool
	Destroyed    bool
}

type concretePowderWaterDirection struct {
	offset game.BlockPosition
	face   game.BlockFace
}

func (entity *runtimeFallingBlockEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeFallingBlockEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: 10, UpdateInterval: 20, TrackDeltas: true}
}

func (entity *runtimeFallingBlockEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeFallingBlockEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Velocity
}

func (entity *runtimeFallingBlockEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	return []protocol.EntityMetadataEntry{{
		Index: protocol.FallingBlockStartMetadataIndex,
		Type:  protocol.MetadataTypeBlockPosition,
		Value: protocol.MetadataBlockPosition(entity.Start),
	}}
}

func (entity *runtimeFallingBlockEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID: snapshot.ID, UUID: snapshot.UUID, Type: int32(game.EntityFallingBlock),
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z,
		VelocityX: snapshot.Velocity.X, VelocityY: snapshot.Velocity.Y, VelocityZ: snapshot.Velocity.Z,
		Data: int32(entity.Block),
	}
}

func (entity *runtimeFallingBlockEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	definition, valid := game.EntityFallingBlock.Definition()
	if !valid {
		return
	}

	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	if entity.Block == game.Air {
		entityID := entity.State.ID
		entity.State.mu.Unlock()

		runtime.removeRuntimeEntity(entityID)

		return
	}

	entity.Time++

	velocity := entity.Velocity
	velocity.Y -= fallingBlockGravity

	previous := entity.State.Position
	movement := runtime.moveGroundEntity(previous, velocity, definition.Width, definition.Height, 0, entity.OnGround)

	entity.State.Position = movement.Position
	entity.OnGround = movement.OnGround

	if movement.HorizontalCollisionX {
		velocity.X = 0
	}

	if movement.HorizontalCollisionZ {
		velocity.Z = 0
	}

	verticalMovement := movement.Position.Y - previous.Y
	if verticalMovement < 0 {
		entity.FallDistance += float32(-verticalMovement)
	}

	falling := entity.Block.FallingDefinition()

	landingPosition := toBlockPosition(movement.Position)
	touchesWater := false

	if falling.Kind == game.FallingBlockKindConcretePowder {
		touchesWater = runtime.World.FluidAt(landingPosition).Type() == game.FluidTypeWater

		speedSquared := velocity.X*velocity.X + velocity.Y*velocity.Y + velocity.Z*velocity.Z
		if !touchesWater && speedSquared > fallingBlockWaterRaySpeed*fallingBlockWaterRaySpeed {
			hit, found := runtime.clipSourceWaterSegment(previous, movement.Position)
			if found {
				landingPosition = hit
				touchesWater = true
			}
		}
	}

	velocity.X *= fallingBlockDrag
	velocity.Y *= fallingBlockDrag
	velocity.Z *= fallingBlockDrag

	entity.Velocity = velocity
	entity.State.movementSyncDirty = true

	entityID := entity.State.ID
	time := entity.Time
	position := entity.State.Position
	onGround := entity.OnGround
	fallDistance := entity.FallDistance
	block := entity.Block
	dropItem := entity.DropItem
	destroyed := entity.Destroyed

	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)

	if onGround || touchesWater {
		runtime.landFallingBlock(entity, entityID, block, landingPosition, position, fallDistance, falling, touchesWater, dropItem, destroyed)

		return
	}

	blockY := int32(math.Floor(position.Y))
	maximumY := int32(protocol.OverworldMinY + protocol.OverworldSectionCount*game.ChunkWidth - 1)

	timedOut := time > fallingBlockMaximumTime || time > fallingBlockVoidTime && (blockY <= protocol.OverworldMinY || blockY > maximumY)
	if timedOut {
		if dropItem {
			runtime.dropFallingBlockItem(block, position)
		}

		runtime.removeRuntimeEntity(entityID)

		return
	}

	runtime.synchronizeRuntimeEntity(entity)
}

func (runtime *Runtime) SpawnFallingBlock(position game.Position, block game.Block, start game.BlockPosition) *runtimeFallingBlockEntity {
	entity := &runtimeFallingBlockEntity{Block: block, Start: start, DropItem: true}

	runtime.registerRuntimeEntity(entity, game.EntityFallingBlock, position)

	return entity
}

func (runtime *Runtime) tickFallingBlockLocked(position game.BlockPosition, block game.Block) {
	if position.Y < protocol.OverworldMinY {
		return
	}

	below := position
	below.Y--

	if !fallingBlockFree(runtime.World.BlockAt(below)) {
		return
	}

	carried := block

	withoutFluid, hadFluid := block.WithoutContainedFluid()
	if hadFluid {
		carried = withoutFluid
	}

	replacement := block.FluidState().LegacyBlock()

	changes := runtime.withStructuralNeighborChanges([]game.BlockChange{{Position: position, Replacement: replacement}})

	result, delivery, err := runtime.mutateBlocksLocked(nil, BlockMutationPlace, changes, 1, true, false, true, false)
	if err != nil || !result.Changed || runtime.World.BlockAt(position) != replacement {
		return
	}

	spawn := game.Position{X: float64(position.X) + 0.5, Y: float64(position.Y), Z: float64(position.Z) + 0.5}

	entity := &runtimeFallingBlockEntity{Block: carried, Start: position, DropItem: true}

	delivery.runtimeAfterDelivery = func() {
		runtime.registerRuntimeEntity(entity, game.EntityFallingBlock, spawn)
	}

	runtime.runtimeBlockMutations = append(runtime.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
}

func (runtime *Runtime) landFallingBlock(entity *runtimeFallingBlockEntity, entityID int32, block game.Block, landingPosition game.BlockPosition, position game.Position, fallDistance float32, falling game.FallingBlockDefinition, touchesWater, dropItem, destroyed bool) {
	velocity := entity.Velocity

	velocity.X *= fallingBlockGroundDrag
	velocity.Y *= fallingBlockGroundBounce
	velocity.Z *= fallingBlockGroundDrag

	entity.State.mu.Lock()
	entity.Velocity = velocity
	entity.State.mu.Unlock()

	if falling.HurtAmount > 0 {
		fallTicks := int32(math.Ceil(float64(fallDistance - 1)))
		if fallTicks >= 0 {
			damage := min(int32(math.Floor(float64(float32(fallTicks)*falling.HurtAmount))), falling.HurtMaximum)
			if damage > 0 {
				runtime.damageFallingBlockEntities(entityID, position, damage)

				if runtime.nextEntityRandom() < 0.05+float32(fallTicks)*0.05 {
					if falling.DamagedState == game.Air {
						destroyed = true
					} else {
						block = preserveFallingBlockFacing(block, falling.DamagedState)
						falling = block.FallingDefinition()
					}
				}
			}
		}
	}

	if destroyed {
		runtime.removeRuntimeEntity(entityID)
		runtime.sendFallingBlockEvent(protocol.LevelEventAnvilBroken, landingPosition)

		return
	}

	target := runtime.World.BlockAt(landingPosition)
	if sameBlockType(target, game.MovingPiston) {
		runtime.synchronizeRuntimeEntity(entity)

		return
	}

	below := landingPosition

	below.Y--

	canPlace := fallingBlockMayReplace(target) && (!fallingBlockFree(runtime.World.BlockAt(below)) || falling.Kind == game.FallingBlockKindConcretePowder && touchesWater)
	if canPlace {
		replacement := block

		if falling.Kind == game.FallingBlockKindConcretePowder && runtime.concretePowderSolidifies(landingPosition, runtime.World.BlockAt) {
			replacement = falling.HardenedState
		} else if target.FluidState().Type() == game.FluidTypeWater {
			waterlogged, valid := replacement.WithContainedFluid(game.FluidTypeWater)
			if valid {
				replacement = waterlogged
			}
		}

		changes := runtime.withStructuralNeighborChanges([]game.BlockChange{{Position: landingPosition, Replacement: replacement}})

		result, delivery, err := runtime.mutateBlocksLocked(nil, BlockMutationPlace, changes, 1, true, false, true, false, true)
		if err == nil && result.Changed && runtime.World.BlockAt(landingPosition) == replacement {
			viewers := runtime.unregisterRuntimeEntity(entityID)

			delivery.runtimeAfterDelivery = func() {
				runtime.untrackRemovedRuntimeEntity(entityID, viewers)

				if falling.Kind == game.FallingBlockKindAnvil {
					runtime.sendFallingBlockEvent(protocol.LevelEventAnvilLand, landingPosition)
				}
			}

			runtime.runtimeBlockMutations = append(runtime.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})

			return
		}
	}

	runtime.removeRuntimeEntity(entityID)

	if dropItem {
		if falling.Kind == game.FallingBlockKindAnvil {
			runtime.sendFallingBlockEvent(protocol.LevelEventAnvilBroken, landingPosition)
		}

		runtime.dropFallingBlockItem(block, position)
	}
}

func (runtime *Runtime) damageFallingBlockEntities(entityID int32, position game.Position, amount int32) {
	definition, valid := game.EntityFallingBlock.Definition()
	if !valid {
		return
	}

	box := entityBox(position, definition.Width, definition.Height)

	damage := game.Damage{Type: game.DamageFallingAnvil, Amount: float32(amount), CauseEntityID: entityID, DirectEntityID: entityID}

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator || !box.Intersects(player.collisionBox()) {
			continue
		}

		update, applied := runtime.damagePlayerLocked(session, damage)
		if applied {
			runtime.sendPlayerSurvivalUpdate(session, update)
		}
	}

	iterator := runtime.runtimeLivingEntitiesInBox(box)

	for {
		living, found := iterator.Next()
		if !found {
			break
		}

		update, applied := runtime.damageRuntimeLivingEntityLocked(living, damage)
		if applied {
			runtime.sendRuntimeLivingDamageUpdate(update)
		}
	}
}

func (runtime *Runtime) dropFallingBlockItem(block game.Block, position game.Position) {
	item, valid := game.ItemForBlock(block)
	if !valid {
		return
	}

	runtime.SpawnItemEntity(game.ItemStack{Item: item, Count: 1}, position, game.Velocity{}, fallingBlockDropDelay)
}

func (runtime *Runtime) sendFallingBlockEvent(event int32, position game.BlockPosition) {
	levelEvent := protocol.LevelEvent{Event: event, Position: position}

	for _, session := range runtime.sessionView() {
		err := session.sendLevelEventIfLoaded(levelEvent)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to send falling block event: %v\n", err)
		}
	}
}

func (runtime *Runtime) scheduleFallingBlockNeighborsLocked(changes []game.BlockChange) {
	for _, change := range changes {
		enqueueStructuralNeighborhood(change.Position, func(position game.BlockPosition) {
			block := runtime.World.BlockAt(position)
			if block.FallingDefinition().Kind != game.FallingBlockKindNone {
				runtime.scheduleBlockTickLocked(position, block, fallingBlockScheduleDelay)
			}
		})
	}
}

func (runtime *Runtime) concretePowderSolidifies(position game.BlockPosition, blockAt func(game.BlockPosition) game.Block) bool {
	return concretePowderSolidifies(blockAt, position)
}

func (runtime *Runtime) clipSourceWaterSegment(from, to game.Position) (game.BlockPosition, bool) {
	if from == to {
		return game.BlockPosition{}, false
	}

	// BlockGetter.traverseBlocks nudges both endpoints inward before stepping the grid.
	start := game.Position{
		X: math.FMA(-1e-7, from.X-to.X, from.X),
		Y: math.FMA(-1e-7, from.Y-to.Y, from.Y),
		Z: math.FMA(-1e-7, from.Z-to.Z, from.Z),
	}

	end := game.Position{
		X: math.FMA(-1e-7, to.X-from.X, to.X),
		Y: math.FMA(-1e-7, to.Y-from.Y, to.Y),
		Z: math.FMA(-1e-7, to.Z-from.Z, to.Z),
	}

	position := toBlockPosition(start)

	hit, water := runtime.sourceWaterClipCell(from, to, position)
	if hit {
		return position, water
	}

	stepX, distanceX, deltaX := rayStep(start.X, end.X-start.X)
	stepY, distanceY, deltaY := rayStep(start.Y, end.Y-start.Y)
	stepZ, distanceZ, deltaZ := rayStep(start.Z, end.Z-start.Z)

	for distanceX <= 1 || distanceY <= 1 || distanceZ <= 1 {
		if distanceX < distanceY && distanceX < distanceZ {
			position.X += stepX
			distanceX += deltaX
		} else if distanceY < distanceZ {
			position.Y += stepY
			distanceY += deltaY
		} else {
			position.Z += stepZ
			distanceZ += deltaZ
		}

		hit, water = runtime.sourceWaterClipCell(from, to, position)
		if hit {
			return position, water
		}
	}

	return game.BlockPosition{}, false
}

func (runtime *Runtime) sourceWaterClipCell(from, to game.Position, position game.BlockPosition) (bool, bool) {
	block := runtime.World.BlockAt(position)

	directionX := to.X - from.X
	directionY := to.Y - from.Y
	directionZ := to.Z - from.Z

	nearestBlock := math.Inf(1)

	var collisionBuffer [7]game.AABB

	for _, box := range block.AppendCollisionBoxes(collisionBuffer[:0], position) {
		fraction, _, hit := raycastAABB(from, directionX, directionY, directionZ, box)
		if hit && fraction <= 1 && fraction < nearestBlock {
			nearestBlock = fraction
		}
	}

	nearestWater := math.Inf(1)

	fluid := runtime.World.FluidAt(position)
	if fluid.Type() == game.FluidTypeWater && fluid.IsSource() {
		box := fluidRaycastBox(runtime.World, block, position)

		fraction, _, hit := raycastAABB(from, directionX, directionY, directionZ, box)
		if hit && fraction <= 1 {
			nearestWater = fraction
		}
	}

	if nearestBlock <= nearestWater && !math.IsInf(nearestBlock, 1) {
		return true, false
	}

	return !math.IsInf(nearestWater, 1), !math.IsInf(nearestWater, 1)
}

func (entity *runtimeFallingBlockEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{
		ID: entity.State.ID, UUID: entity.State.UUID, Position: entity.State.Position, Chunk: entity.State.Chunk,
		Removed: entity.State.Removed, Velocity: entity.Velocity, OnGround: entity.OnGround,
	}
}

func concretePowderSolidifies(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition) bool {
	if blockAt(position).FluidState().Type() == game.FluidTypeWater {
		return true
	}

	directions := [...]concretePowderWaterDirection{
		{offset: game.BlockPosition{Y: 1}, face: game.BlockFaceDown},
		{offset: game.BlockPosition{Z: -1}, face: game.BlockFaceSouth},
		{offset: game.BlockPosition{Z: 1}, face: game.BlockFaceNorth},
		{offset: game.BlockPosition{X: -1}, face: game.BlockFaceEast},
		{offset: game.BlockPosition{X: 1}, face: game.BlockFaceWest},
	}

	for _, direction := range directions {
		neighborPosition := game.BlockPosition{X: position.X + direction.offset.X, Y: position.Y + direction.offset.Y, Z: position.Z + direction.offset.Z}

		neighbor := blockAt(neighborPosition)
		if neighbor.FluidState().Type() == game.FluidTypeWater && !neighbor.FullFace(direction.face) {
			return true
		}
	}

	return false
}

func fallingBlockFree(block game.Block) bool {
	return block == game.Air || !block.FluidState().Empty() || block.Replaceable()
}

func fallingBlockMayReplace(target game.Block) bool {
	// DirectionalPlaceContext carries an empty item stack: block-specific stacking
	// rules for slabs, candles, snow and scaffolding cannot use the falling block.
	definition, valid := target.Definition()
	if !valid {
		return false
	}

	switch definition.ID {
	case game.ScaffoldingID:
		return false
	case game.SculkVeinID:
		return true
	case game.GlowLichenID:
		faces := [...]string{"down", "up", "north", "south", "east", "west"}

		for _, face := range faces {
			value, _ := target.Property(face)
			if value != "true" {
				return true
			}
		}

		return false
	case game.VineID:
		faces := [...]string{"up", "north", "south", "east", "west"}

		for _, face := range faces {
			value, _ := target.Property(face)
			if value != "true" {
				return true
			}
		}

		return false
	case game.SnowID:
		return blockPropertyInt(target, "layers") == 1
	default:
		if definition.Behavior == game.BlockBehaviorSlab {
			return false
		}

		return target.Replaceable()
	}
}

func preserveFallingBlockFacing(block, replacement game.Block) game.Block {
	facing, valid := block.Property("facing")
	if !valid {
		return replacement
	}

	state, valid := replacement.WithProperties(game.BlockPropertyValue{Name: "facing", Value: facing})
	if !valid {
		return replacement
	}

	return state
}
