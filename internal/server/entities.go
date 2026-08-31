package server

import (
	cryptorand "crypto/rand"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"sync"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	itemEntityWidth              = 0.25
	itemEntityHeight             = 0.25
	itemEntityGravity            = 0.04
	itemEntityVerticalDrag       = 0.98
	itemEntityLifetime           = 6000
	itemEntityMergeRadius        = 0.5
	itemEntitySyncThreshold      = 0.01
	itemEntityWaterDrag          = 0.99
	itemEntityLavaDrag           = 0.95
	itemEntityFluidLift          = 0.0005
	itemEntityFluidRiseMax       = 0.06
	itemEntityWaterPush          = 0.014
	itemEntityLavaPush           = 0.0023333333333333335
	itemEntityFastLavaPush       = 0.007
	itemEntityFluidSyncThreshold = 1e-8
)

type RuntimeEntityState struct {
	mu sync.RWMutex

	ID       int32
	UUID     string
	Position game.Position
	Chunk    LoadedChunk
	Removed  bool

	tracker           runtimeEntityTracker
	movementSyncDirty bool
	metadataDirty     bool
}

type RuntimeEntityTrackingConfig struct {
	ClientRangeChunks int32
	UpdateInterval    int32
	TrackDeltas       bool
}

type runtimeEntityView struct {
	ID       int32
	UUID     string
	Position game.Position
	Chunk    LoadedChunk
	Removed  bool
	Velocity game.Velocity
	Rotation game.Rotation
	OnGround bool
}

type runtimeEntitySpawnSnapshot struct {
	ID       int32
	UUID     string
	Position game.Position
	Velocity game.Velocity
	Yaw      byte
	Pitch    byte
	HeadYaw  byte
}

type RuntimeEntityMetadata interface {
	EntityMetadata() []protocol.EntityMetadataEntry
}

type RuntimeEntityVelocity interface {
	EntityVelocity() game.Velocity
}

type RuntimeEntitySpawner interface {
	AddEntityPacket(runtimeEntitySpawnSnapshot) protocol.AddEntity
}

type RuntimeEntityTracker interface {
	RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig
	RuntimeEntityView() runtimeEntityView
}

type runtimeItemEntity struct {
	State        RuntimeEntityState
	Stack        game.ItemStack
	Velocity     game.Velocity
	Health       int32
	Age          int32
	PickupDelay  int32
	TargetUUID   string
	ThrowerUUID  string
	OnGround     bool
	TickCount    int32
	FluidType    game.FluidType
	FluidImpulse game.Velocity
}

type itemEscapeDirection struct {
	X            int32
	Y            int32
	Z            int32
	Axis         byte
	Positive     bool
	AxisPosition float64
}

type itemEntityMovement struct {
	Position             game.Position
	OnGround             bool
	HorizontalCollisionX bool
	HorizontalCollisionZ bool
	VerticalCollision    bool
	SlimeBounce          bool
}

func (entity *runtimeItemEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeItemEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: 6, UpdateInterval: 20, TrackDeltas: true}
}

func (entity *runtimeItemEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *runtimeItemEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{
		ID:       entity.State.ID,
		UUID:     entity.State.UUID,
		Position: entity.State.Position,
		Chunk:    entity.State.Chunk,
		Removed:  entity.State.Removed,
		Velocity: entity.Velocity,
		OnGround: entity.OnGround,
	}
}

func (entity *runtimeItemEntity) itemMergableLocked() bool {
	definition, valid := entity.Stack.Item.Definition()
	if !valid {
		return false
	}

	return !entity.State.Removed && entity.PickupDelay != 32767 && entity.Age != -32768 && entity.Age < itemEntityLifetime && entity.Stack.Count < definition.StackSize
}

func (entity *runtimeItemEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.entityMetadataLocked()
}

func (entity *runtimeItemEntity) entityMetadataLocked() []protocol.EntityMetadataEntry {
	return []protocol.EntityMetadataEntry{{
		Index: protocol.ItemEntityItemMetadataIndex,
		Type:  protocol.MetadataTypeItemStack,
		Value: protocol.MetadataItemStack{Stack: entity.Stack.Clone()},
	}}
}

func (entity *runtimeItemEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Velocity
}

func (entity *runtimeItemEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  snapshot.ID,
		UUID:      snapshot.UUID,
		Type:      protocol.ItemEntityType,
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

func (entity *runtimeItemEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	entity.State.mu.Lock()

	if entity.State.Removed {
		entity.State.mu.Unlock()

		return
	}

	entity.TickCount++

	if entity.PickupDelay > 0 && entity.PickupDelay != 32767 {
		entity.PickupDelay--
	}

	previous := entity.State.Position
	previousVelocity := entity.Velocity

	waterContact := runtime.fluidContact(itemEntityBox(entity.State.Position), game.FluidTypeWater, false)
	lavaContact := runtime.fluidContact(itemEntityBox(entity.State.Position), game.FluidTypeLava, false)

	fluidType := game.FluidTypeEmpty

	if waterContact.Depth > fluidContactDepth {
		fluidType = game.FluidTypeWater
	} else if lavaContact.Depth > fluidContactDepth {
		fluidType = game.FluidTypeLava
	}

	switch fluidType {
	case game.FluidTypeEmpty:
		entity.Velocity.Y -= itemEntityGravity
	case game.FluidTypeWater:
		entity.Velocity.X *= itemEntityWaterDrag
		entity.Velocity.Z *= itemEntityWaterDrag

		if entity.Velocity.Y < itemEntityFluidRiseMax {
			entity.Velocity.Y += itemEntityFluidLift
		}
	case game.FluidTypeLava:
		entity.Velocity.X *= itemEntityLavaDrag
		entity.Velocity.Z *= itemEntityLavaDrag

		if entity.Velocity.Y < itemEntityFluidRiseMax {
			entity.Velocity.Y += itemEntityFluidLift
		}
	}

	noPhysics := runtime.itemEntityInsideCollision(entity.State.Position)
	if noPhysics {
		entity.Velocity = runtime.itemVelocityTowardsClosestSpace(entity.State.Position, entity.Velocity)
	}

	horizontalSpeedSquared := entity.Velocity.X*entity.Velocity.X + entity.Velocity.Z*entity.Velocity.Z
	shouldMove := !entity.OnGround || horizontalSpeedSquared > 1e-5 || (entity.TickCount+entity.State.ID)%4 == 0

	if shouldMove {
		if noPhysics {
			entity.State.Position.X += entity.Velocity.X
			entity.State.Position.Y += entity.Velocity.Y
			entity.State.Position.Z += entity.Velocity.Z
			entity.OnGround = false
		} else {
			movement := runtime.moveItemEntity(entity.State.Position, entity.Velocity)

			entity.State.Position = movement.Position
			entity.OnGround = movement.OnGround

			if movement.HorizontalCollisionX {
				entity.Velocity.X = 0
			}

			if movement.HorizontalCollisionZ {
				entity.Velocity.Z = 0
			}

			if movement.VerticalCollision {
				if movement.SlimeBounce && entity.Velocity.Y < 0 {
					entity.Velocity.Y *= -0.8
				} else {
					entity.Velocity.Y = 0
				}
			}
		}

		horizontalDrag := float32(0.98)

		if entity.OnGround {
			horizontalDrag *= runtime.blockFrictionBelow(entity.State.Position)
		}

		entity.Velocity.X *= float64(horizontalDrag)
		entity.Velocity.Y *= itemEntityVerticalDrag
		entity.Velocity.Z *= float64(horizontalDrag)

		if entity.OnGround && entity.Velocity.Y < 0 {
			entity.Velocity.Y *= -0.5
		}
	}

	waterContact = runtime.fluidContact(itemEntityBox(entity.State.Position), game.FluidTypeWater, false)
	lavaContact = runtime.fluidContact(itemEntityBox(entity.State.Position), game.FluidTypeLava, false)

	fluidType = game.FluidTypeEmpty
	fluidContact := entityFluidContact{}

	if waterContact.Depth > 0 {
		fluidType = game.FluidTypeWater
		fluidContact = waterContact
	} else if lavaContact.Depth > 0 {
		fluidType = game.FluidTypeLava
		fluidContact = lavaContact
	}

	if fluidType == game.FluidTypeLava && !entity.Stack.Item.FireResistant() {
		entity.Health -= 4

		if entity.Health <= 0 {
			entityID := entity.State.ID
			position := entity.State.Position

			entity.State.mu.Unlock()

			runtime.playItemBurnSound(position)
			runtime.removeRuntimeEntity(entityID)

			return
		}
	}

	impulse := game.Velocity{}

	switch fluidType {
	case game.FluidTypeWater:
		impulse = fluidCurrentImpulse(entity.Velocity, fluidContact.Flow, itemEntityWaterPush)
	case game.FluidTypeLava:
		push := itemEntityLavaPush

		if runtime.FluidEnvironment.FastLava {
			push = itemEntityFastLavaPush
		}

		impulse = fluidCurrentImpulse(entity.Velocity, fluidContact.Flow, push)
	}

	entity.Velocity.X += impulse.X
	entity.Velocity.Y += impulse.Y
	entity.Velocity.Z += impulse.Z

	fluidStateChanged := entity.FluidType != fluidType

	fluidImpulseChanged := velocityDistanceSquared(entity.FluidImpulse, impulse) > itemEntityFluidSyncThreshold

	entity.FluidType = fluidType
	entity.FluidImpulse = impulse

	movedBlock := itemEntityBlockPosition(previous) != itemEntityBlockPosition(entity.State.Position)

	mergeRate := int32(40)

	if movedBlock {
		mergeRate = 2
	}

	shouldMerge := entity.TickCount%mergeRate == 0 && entity.itemMergableLocked()
	entity.State.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)

	if shouldMerge && runtime.mergeItemEntity(entity) {
		return
	}

	entity.State.mu.Lock()

	if entity.Age != -32768 {
		entity.Age++
	}

	if velocityDistanceSquared(entity.Velocity, previousVelocity) > itemEntitySyncThreshold {
		entity.State.movementSyncDirty = true
	}

	if fluidStateChanged || fluidImpulseChanged {
		entity.State.movementSyncDirty = true
	}

	expired := entity.Age >= itemEntityLifetime
	entityID := entity.State.ID
	entity.State.mu.Unlock()

	if expired {
		runtime.removeRuntimeEntity(entityID)

		return
	}

	if runtime.pickUpItemEntity(entity) {
		return
	}

	runtime.synchronizeRuntimeEntity(entity)
}

func (r *Runtime) SpawnItemEntity(stack game.ItemStack, position game.Position, velocity game.Velocity, pickupDelay int32) *runtimeItemEntity {
	if stack.Empty() {
		return nil
	}

	entity := &runtimeItemEntity{
		State: RuntimeEntityState{
			ID:       r.allocateEntityID(),
			UUID:     randomEntityUUID(),
			Position: position,
			Chunk:    positionLoadedChunk(position),
		},
		Stack:       stack.Clone(),
		Velocity:    velocity,
		Health:      5,
		PickupDelay: pickupDelay,
	}
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

func (r *Runtime) playItemBurnSound(position game.Position) {
	sound := protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: "minecraft:entity.generic.burn"},
		Source: protocol.SoundSourceNeutral,
		X:      position.X,
		Y:      position.Y,
		Z:      position.Z,
		Volume: 0.4,
		Pitch:  2 + rand.Float32()*0.4,
		Seed:   rand.Int64(),
	}

	blockPosition := itemEntityBlockPosition(position)

	for _, viewer := range r.snapshotSessions() {
		err := viewer.sendSoundIfLoaded(sound, blockPosition)
		if err != nil && viewer.Log != nil {
			viewer.Log.Warnf("[play] failed to send item burn sound: %v\n", err)
		}
	}
}

func (r *Runtime) spawnPlayerDroppedItem(player game.Player, stack game.ItemStack, randomly, thrownFromHand bool) *runtimeItemEntity {
	if stack.Empty() {
		return nil
	}

	position := player.EyePosition()

	position.Y -= float64(float32(0.3))

	var velocity game.Velocity

	if randomly {
		power := r.nextEntityRandom() * float32(0.5)
		direction := r.nextEntityRandom() * float32(math.Pi*2)

		velocity = game.Velocity{X: float64(-minecraftSin(float64(direction)) * power), Y: float64(float32(0.2)), Z: float64(minecraftCos(float64(direction)) * power)}
	} else {
		pitch := player.Rotation.Pitch * float32(math.Pi/180)
		yaw := player.Rotation.Yaw * float32(math.Pi/180)

		sinPitch := minecraftSin(float64(pitch))
		cosPitch := minecraftCos(float64(pitch))

		sinYaw := minecraftSin(float64(yaw))
		cosYaw := minecraftCos(float64(yaw))

		direction := r.nextEntityRandom() * float32(math.Pi*2)
		randomPower := float32(0.02) * r.nextEntityRandom()

		velocity.X = float64(-sinYaw*cosPitch*float32(0.3)) + math.Cos(float64(direction))*float64(randomPower)
		velocity.Y = float64(-sinPitch*float32(0.3) + float32(0.1) + (r.nextEntityRandom()-r.nextEntityRandom())*float32(0.1))
		velocity.Z = float64(cosYaw*cosPitch*float32(0.3)) + math.Sin(float64(direction))*float64(randomPower)
	}

	entity := r.SpawnItemEntity(stack, position, velocity, 40)

	if thrownFromHand {
		entity.ThrowerUUID = player.UUID
	}

	return entity
}

func (r *Runtime) nextEntityRandom() float32 {
	r.entityRandomMu.Lock()
	defer r.entityRandomMu.Unlock()

	return r.entityRandom()
}

func (r *Runtime) snapshotEntitiesInChunk(chunk LoadedChunk) map[int32]RuntimeEntity {
	r.entityMu.RLock()
	defer r.entityMu.RUnlock()

	source := r.entitiesByChunk[chunk]

	entities := make(map[int32]RuntimeEntity, len(source))

	maps.Copy(entities, source)

	return entities
}

func (r *Runtime) snapshotRuntimeEntities() []RuntimeEntity {
	r.entityMu.RLock()
	defer r.entityMu.RUnlock()

	entities := make([]RuntimeEntity, 0, len(r.entities))

	for _, entity := range r.entities {
		entities = append(entities, entity)
	}

	return entities
}

func (r *Runtime) addEntityToChunkIndexLocked(entity RuntimeEntity) {
	view := entity.(RuntimeEntityTracker).RuntimeEntityView()

	indexed := r.entitiesByChunk[view.Chunk]
	if indexed == nil {
		indexed = make(map[int32]RuntimeEntity)

		r.entitiesByChunk[view.Chunk] = indexed
	}

	indexed[view.ID] = entity
}

func (r *Runtime) runtimeEntityMoved(entity RuntimeEntity, previous game.Position) {
	previousChunk := positionLoadedChunk(previous)

	state := entity.RuntimeEntityState()

	state.mu.RLock()
	nextChunk := positionLoadedChunk(state.Position)
	state.mu.RUnlock()

	if previousChunk == nextChunk {
		return
	}

	r.entityMu.Lock()
	state.mu.Lock()

	delete(r.entitiesByChunk[previousChunk], state.ID)

	if len(r.entitiesByChunk[previousChunk]) == 0 {
		delete(r.entitiesByChunk, previousChunk)
	}

	state.Chunk = nextChunk

	indexed := r.entitiesByChunk[nextChunk]
	if indexed == nil {
		indexed = make(map[int32]RuntimeEntity)

		r.entitiesByChunk[nextChunk] = indexed
	}

	indexed[state.ID] = entity

	entityID := state.ID

	state.mu.Unlock()
	r.entityMu.Unlock()

	previousActive, active := r.ActiveChunk(previousChunk)
	if active {
		previousActive.RemoveEntity(entityID)
	}

	nextActive, active := r.ActiveChunk(nextChunk)
	if active {
		nextActive.SetEntity(entityID, entity)
	}

	r.reconcileRuntimeEntityTracking(entity)
}

func (r *Runtime) removeRuntimeEntity(id int32) {
	r.entityMu.Lock()

	entity := r.entities[id]
	if entity == nil {
		r.entityMu.Unlock()

		return
	}

	state := entity.RuntimeEntityState()
	state.mu.Lock()

	state.Removed = true
	chunkPosition := state.Chunk

	delete(r.entities, id)
	delete(r.entitiesByChunk[chunkPosition], id)

	if len(r.entitiesByChunk[chunkPosition]) == 0 {
		delete(r.entitiesByChunk, chunkPosition)
	}

	state.mu.Unlock()
	r.entityMu.Unlock()

	chunk, active := r.ActiveChunk(chunkPosition)
	if active {
		chunk.RemoveEntity(id)
	}

	for _, session := range r.snapshotSessions() {
		session.untrackRuntimeEntity(id)
	}
}

func (r *Runtime) reconcileRuntimeEntityTracking(entity RuntimeEntity) {
	for _, session := range r.snapshotSessions() {
		if session.shouldTrackRuntimeEntity(entity) {
			session.trackRuntimeEntity(entity)
		} else {
			session.untrackRuntimeEntity(entity.(RuntimeEntityTracker).RuntimeEntityView().ID)
		}
	}
}

func (r *Runtime) trackLoadedEntities(session *Session) {
	session.chunkMx.Lock()

	chunks := make([]LoadedChunk, 0, len(session.loadedChunks))

	for chunk := range session.loadedChunks {
		chunks = append(chunks, chunk)
	}

	session.chunkMx.Unlock()

	for _, chunk := range chunks {
		session.trackEntitiesInChunk(chunk)
	}
}

func (s *Session) trackEntitiesInChunk(chunk LoadedChunk) {
	if s.Runtime == nil {
		return
	}

	entities := s.Runtime.snapshotEntitiesInChunk(chunk)

	ids := make([]int32, 0, len(entities))

	for id := range entities {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids {
		entity := entities[id]
		if s.shouldTrackRuntimeEntity(entity) {
			s.trackRuntimeEntity(entity)
		}
	}
}

func (s *Session) untrackEntitiesInChunk(chunk LoadedChunk) {
	for id := range s.Runtime.snapshotEntitiesInChunk(chunk) {
		s.untrackRuntimeEntity(id)
	}
}

func (s *Session) shouldTrackRuntimeEntity(entity RuntimeEntity) bool {
	tracked, trackable := entity.(RuntimeEntityTracker)
	if !trackable {
		return false
	}

	view := tracked.RuntimeEntityView()
	if view.Removed {
		return false
	}

	s.chunkMx.Lock()
	_, loaded := s.loadedChunks[view.Chunk]
	s.chunkMx.Unlock()

	if !loaded {
		return false
	}

	player := s.snapshotPlayer()

	distanceX := player.Position.X - view.Position.X
	distanceZ := player.Position.Z - view.Position.Z

	configuration := tracked.RuntimeEntityTrackingConfig()

	rangeBlocks := float64(configuration.ClientRangeChunks * ChunkWidth)

	return distanceX*distanceX+distanceZ*distanceZ <= rangeBlocks*rangeBlocks
}

func (s *Session) trackRuntimeEntity(entity RuntimeEntity) {
	_, trackable := entity.(RuntimeEntityTracker)
	lockedViewer, lockable := entity.(runtimeEntityLockedViewer)

	if !trackable || !lockable {
		return
	}

	s.entityTrackMu.Lock()
	defer s.entityTrackMu.Unlock()

	state := entity.RuntimeEntityState()

	state.mu.RLock()
	view := lockedViewer.runtimeEntityViewLocked()

	snapshot := runtimeEntitySpawnSnapshotLocked(view, state.tracker)
	state.mu.RUnlock()

	if view.Removed {
		return
	}

	if s.trackedEntities == nil {
		s.trackedEntities = make(map[int32]struct{})
	}

	if _, present := s.trackedEntities[view.ID]; present {
		return
	}

	s.trackedEntities[view.ID] = struct{}{}

	spawner, spawnable := entity.(RuntimeEntitySpawner)
	if !spawnable {
		delete(s.trackedEntities, view.ID)

		return
	}

	addSent := false

	err := s.writePacket(protocol.ClientboundAddEntityID, spawner.AddEntityPacket(snapshot))
	if err == nil {
		addSent = true

		metadata, present := entity.(RuntimeEntityMetadata)
		if present {
			err = s.writePacket(protocol.ClientboundEntityMetadataID, protocol.EntityMetadata{EntityID: view.ID, Entries: metadata.EntityMetadata()})
		}
	}

	if err != nil && s.Log != nil {
		s.Log.Warnf("[play] failed to track entity: %v\n", err)
	}

	if err != nil {
		delete(s.trackedEntities, view.ID)

		if addSent {
			_ = s.writePacket(protocol.ClientboundRemoveEntitiesID, protocol.RemoveEntities{EntityIDs: []int32{view.ID}})
		}
	}
}

func (s *Session) untrackRuntimeEntity(id int32) {
	s.entityTrackMu.Lock()
	defer s.entityTrackMu.Unlock()

	if _, tracked := s.trackedEntities[id]; !tracked {
		return
	}

	delete(s.trackedEntities, id)

	err := s.writePacket(protocol.ClientboundRemoveEntitiesID, protocol.RemoveEntities{EntityIDs: []int32{id}})
	if err != nil && s.Log != nil {
		s.Log.Warnf("[play] failed to untrack entity: %v\n", err)
	}
}

func (s *Session) tracksRuntimeEntity(id int32) bool {
	s.entityTrackMu.Lock()
	defer s.entityTrackMu.Unlock()

	_, tracked := s.trackedEntities[id]
	return tracked
}

func (s *Session) clearTrackedEntities() {
	s.entityTrackMu.Lock()
	s.trackedEntities = nil
	s.entityTrackMu.Unlock()
}

func (r *Runtime) pickUpItemEntity(entity *runtimeItemEntity) bool {
	entity.State.mu.RLock()

	if entity.PickupDelay != 0 {
		entity.State.mu.RUnlock()

		return false
	}

	itemBox := itemEntityBox(entity.State.Position)

	targetUUID := entity.TargetUUID
	entityID := entity.State.ID

	entity.State.mu.RUnlock()

	pickupBox := game.AABB{MinX: itemBox.MinX - 1, MinY: itemBox.MinY - 0.5, MinZ: itemBox.MinZ - 1, MaxX: itemBox.MaxX + 1, MaxY: itemBox.MaxY + 0.5, MaxZ: itemBox.MaxZ + 1}

	for _, session := range r.snapshotSessions() {
		player := session.snapshotPlayer()
		if player.Dead || targetUUID != "" && targetUUID != player.UUID || !pickupBox.Intersects(player.CollisionBox()) {
			continue
		}

		entity.State.mu.RLock()
		before := player.Inventory.Clone()
		remaining := entity.Stack.Clone()
		originalStack := entity.Stack.Clone()
		entity.State.mu.RUnlock()

		changed := false

		player, changed = session.updatePlayerState(func(current *game.Player) bool {
			playerMenu := newPlayerInventoryMenu(&current.Inventory)

			candidate := playerMenu.candidate()

			moveIntoSlots(candidate, &remaining, slotRange(36, 44))
			moveIntoSlots(candidate, &remaining, slotRange(9, 35))

			if remaining.Equal(originalStack) {
				return false
			}

			playerMenu.commit(candidate)

			return true
		})

		if !changed {
			continue
		}

		originalCount := originalStack.Count

		entity.State.mu.Lock()

		if entity.State.Removed || !entity.Stack.Equal(originalStack) {
			entity.State.mu.Unlock()

			continue
		}

		entity.Stack = remaining
		entity.State.metadataDirty = true
		entity.State.mu.Unlock()

		for _, viewer := range r.snapshotSessions() {
			if viewer.tracksRuntimeEntity(entityID) {
				err := viewer.writePacket(protocol.ClientboundTakeItemEntityID, protocol.TakeItemEntity{ItemEntityID: entityID, PlayerEntityID: player.EntityID, Amount: originalCount})
				if err != nil && viewer.Log != nil {
					viewer.Log.Warnf("[play] failed to animate item pickup: %v\n", err)
				}
			}
		}

		err := session.synchronizePlayerInventoryMutation(before)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize item pickup: %v\n", err)
		}

		entity.State.mu.RLock()
		empty := entity.Stack.Empty()
		entity.State.mu.RUnlock()

		if empty {
			r.removeRuntimeEntity(entityID)
		} else {
			r.synchronizeDirtyRuntimeEntityMetadata(entity)
		}

		return true
	}

	return false
}

func (r *Runtime) synchronizeDirtyRuntimeEntityMetadata(entity RuntimeEntityMetadata) {
	stateful := entity.(RuntimeEntity)
	state := stateful.RuntimeEntityState()

	state.mu.Lock()

	if !state.metadataDirty || state.Removed {
		state.mu.Unlock()

		return
	}

	item, itemEntity := entity.(*runtimeItemEntity)

	var entries []protocol.EntityMetadataEntry

	if itemEntity {
		entries = item.entityMetadataLocked()
	} else {
		state.mu.Unlock()
		entries = entity.EntityMetadata()
		state.mu.Lock()

		if !state.metadataDirty || state.Removed {
			state.mu.Unlock()

			return
		}
	}

	id := state.ID

	state.metadataDirty = false
	state.mu.Unlock()

	packet := protocol.EntityMetadata{EntityID: id, Entries: entries}

	for _, session := range r.snapshotSessions() {
		if session.tracksRuntimeEntity(id) {
			_ = session.writePacket(protocol.ClientboundEntityMetadataID, packet)
		}
	}
}

func (r *Runtime) mergeItemEntity(entity *runtimeItemEntity) bool {
	entity.State.mu.RLock()
	view := entity.runtimeEntityViewLocked()
	entity.State.mu.RUnlock()

	candidates := r.itemMergeCandidates(view)

	consumedIDs := make([]int32, 0, len(candidates))
	dirty := false

	for _, candidate := range candidates {
		if candidate == entity {
			continue
		}

		removed, receiver, consumed := mergeItemEntities(entity, candidate)
		if receiver == nil {
			continue
		}

		if removed {
			r.synchronizeDirtyRuntimeEntityMetadata(receiver)
			r.removeRuntimeEntity(consumed.State.ID)

			return true
		}

		dirty = true
		consumedIDs = append(consumedIDs, consumed.State.ID)
	}

	if dirty {
		r.synchronizeDirtyRuntimeEntityMetadata(entity)

		for _, consumedID := range consumedIDs {
			r.removeRuntimeEntity(consumedID)
		}
	}

	return false
}

func (r *Runtime) itemMergeCandidates(view runtimeEntityView) []*runtimeItemEntity {
	minChunk := positionLoadedChunk(game.Position{X: view.Position.X - itemEntityMergeRadius - itemEntityWidth, Z: view.Position.Z - itemEntityMergeRadius - itemEntityWidth})
	maxChunk := positionLoadedChunk(game.Position{X: view.Position.X + itemEntityMergeRadius + itemEntityWidth, Z: view.Position.Z + itemEntityMergeRadius + itemEntityWidth})

	r.entityMu.RLock()
	var candidates []*runtimeItemEntity

	for chunkX := minChunk.X; chunkX <= maxChunk.X; chunkX++ {
		for chunkZ := minChunk.Z; chunkZ <= maxChunk.Z; chunkZ++ {
			for _, candidate := range r.entitiesByChunk[LoadedChunk{X: chunkX, Z: chunkZ}] {
				item, itemEntity := candidate.(*runtimeItemEntity)
				if itemEntity {
					candidates = append(candidates, item)
				}
			}
		}
	}

	r.entityMu.RUnlock()

	slices.SortFunc(candidates, func(first, second *runtimeItemEntity) int {
		return int(first.State.ID - second.State.ID)
	})

	return candidates
}

func (r *Runtime) itemEntityInsideCollision(position game.Position) bool {
	box := itemEntityBox(position)

	box.MinX += 1e-7
	box.MinY += 1e-7
	box.MinZ += 1e-7
	box.MaxX -= 1e-7
	box.MaxY -= 1e-7
	box.MaxZ -= 1e-7

	return slices.ContainsFunc(r.itemCollisionBoxes(box, game.Velocity{}), box.Intersects)
}

func (r *Runtime) itemVelocityTowardsClosestSpace(position game.Position, velocity game.Velocity) game.Velocity {
	block := itemEntityBlockPosition(game.Position{X: position.X, Y: position.Y + itemEntityHeight/2, Z: position.Z})

	fractionX := position.X - math.Floor(position.X)
	fractionY := position.Y + itemEntityHeight/2 - math.Floor(position.Y+itemEntityHeight/2)
	fractionZ := position.Z - math.Floor(position.Z)

	directions := [...]itemEscapeDirection{
		{Z: -1, Axis: 'z', AxisPosition: fractionZ},
		{Z: 1, Axis: 'z', Positive: true, AxisPosition: fractionZ},
		{X: -1, Axis: 'x', AxisPosition: fractionX},
		{X: 1, Axis: 'x', Positive: true, AxisPosition: fractionX},
		{Y: 1, Axis: 'y', Positive: true, AxisPosition: fractionY},
	}

	closest := math.MaxFloat64
	closestDirection := directions[len(directions)-1]

	for _, direction := range directions {
		neighbor := game.BlockPosition{X: block.X + direction.X, Y: block.Y + direction.Y, Z: block.Z + direction.Z}

		if blockCollisionShapeFull(r.World.BlockAt(neighbor), neighbor) {
			continue
		}

		distance := direction.AxisPosition

		if direction.Positive {
			distance = 1 - distance
		}

		if distance < closest {
			closest = distance
			closestDirection = direction
		}
	}

	velocity.X *= 0.75
	velocity.Y *= 0.75
	velocity.Z *= 0.75

	speed := float64(r.nextEntityRandom()*float32(0.2) + float32(0.1))

	if !closestDirection.Positive {
		speed = -speed
	}

	switch closestDirection.Axis {
	case 'x':
		velocity.X = speed
	case 'y':
		velocity.Y = speed
	case 'z':
		velocity.Z = speed
	}

	return velocity
}

func (r *Runtime) moveItemEntity(position game.Position, velocity game.Velocity) itemEntityMovement {
	box := itemEntityBox(position)

	blocks := r.itemCollisionBoxes(box, velocity)

	resolved := collideAABBWithBlocks(box, blocks, velocity)
	deltaX := resolved.X
	deltaY := resolved.Y
	deltaZ := resolved.Z

	position.X += deltaX
	position.Y += deltaY
	position.Z += deltaZ

	verticalCollision := deltaY != velocity.Y
	onGround := velocity.Y < 0 && verticalCollision

	return itemEntityMovement{
		Position:             position,
		OnGround:             onGround,
		HorizontalCollisionX: deltaX != velocity.X,
		HorizontalCollisionZ: deltaZ != velocity.Z,
		VerticalCollision:    verticalCollision,
		SlimeBounce:          onGround && r.blockBelowItem(position) == game.SlimeBlock,
	}
}

func (r *Runtime) itemCollisionBoxes(box game.AABB, velocity game.Velocity) []game.AABB {
	minX := int32(math.Floor(min(box.MinX, box.MinX+velocity.X)))
	minY := int32(math.Floor(min(box.MinY, box.MinY+velocity.Y)))
	minZ := int32(math.Floor(min(box.MinZ, box.MinZ+velocity.Z)))

	maxX := int32(math.Floor(max(box.MaxX, box.MaxX+velocity.X) - 1e-7))
	maxY := int32(math.Floor(max(box.MaxY, box.MaxY+velocity.Y) - 1e-7))
	maxZ := int32(math.Floor(max(box.MaxZ, box.MaxZ+velocity.Z) - 1e-7))

	var boxes []game.AABB

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}

				boxes = append(boxes, r.World.BlockAt(position).CollisionBoxes(position)...)
			}
		}
	}

	return boxes
}

func (r *Runtime) blockFrictionBelow(position game.Position) float32 {
	block := r.blockBelowItem(position)

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

func (r *Runtime) blockBelowItem(position game.Position) game.Block {
	blockPosition := game.BlockPosition{
		X: int32(math.Floor(position.X)),
		Y: int32(math.Floor(position.Y - 0.999999)),
		Z: int32(math.Floor(position.Z)),
	}

	return r.World.BlockAt(blockPosition)
}

func minecraftSin(value float64) float32 {
	index := int64(value*10430.378350470453) & 65535
	return float32(math.Sin(float64(index) / 10430.378350470453))
}

func minecraftCos(value float64) float32 {
	index := int64(value*10430.378350470453+16384) & 65535
	return float32(math.Sin(float64(index) / 10430.378350470453))
}

func mergeItemEntities(entity, other *runtimeItemEntity) (bool, *runtimeItemEntity, *runtimeItemEntity) {
	first := entity
	second := other

	if first.State.ID > second.State.ID {
		swap := first
		first = second
		second = swap
	}

	first.State.mu.Lock()
	second.State.mu.Lock()

	defer first.State.mu.Unlock()
	defer second.State.mu.Unlock()

	if !entity.itemMergableLocked() || !other.itemMergableLocked() || entity.TargetUUID != other.TargetUUID {
		return false, nil, nil
	}

	searchBox := itemEntityBox(entity.State.Position)

	searchBox.MinX -= itemEntityMergeRadius
	searchBox.MaxX += itemEntityMergeRadius
	searchBox.MinZ -= itemEntityMergeRadius
	searchBox.MaxZ += itemEntityMergeRadius

	if !searchBox.Intersects(itemEntityBox(other.State.Position)) || !entity.Stack.SameItem(other.Stack) {
		return false, nil, nil
	}

	definition, valid := entity.Stack.Item.Definition()
	if !valid || entity.Stack.Count+other.Stack.Count > definition.StackSize {
		return false, nil, nil
	}

	receiver := other
	consumed := entity

	if other.Stack.Count < entity.Stack.Count {
		receiver = entity
		consumed = other
	}

	maximum := min(definition.StackSize, int32(64))

	transfer := min(maximum-receiver.Stack.Count, consumed.Stack.Count)
	if transfer <= 0 {
		return false, nil, nil
	}

	receiver.Stack.Count += transfer
	consumed.Stack.Count -= transfer

	receiver.PickupDelay = max(receiver.PickupDelay, consumed.PickupDelay)
	receiver.Age = min(receiver.Age, consumed.Age)
	receiver.State.metadataDirty = true

	return consumed == entity, receiver, consumed
}

func blockCollisionShapeFull(block game.Block, position game.BlockPosition) bool {
	boxes := block.CollisionBoxes(position)
	if len(boxes) != 1 {
		return false
	}

	box := boxes[0]
	return box.MinX == float64(position.X) && box.MinY == float64(position.Y) && box.MinZ == float64(position.Z) &&
		box.MaxX == float64(position.X+1) && box.MaxY == float64(position.Y+1) && box.MaxZ == float64(position.Z+1)
}

func itemEntityBlockPosition(position game.Position) game.BlockPosition {
	return game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
}

func collideAABBWithBlocks(box game.AABB, blocks []game.AABB, movement game.Velocity) game.Velocity {
	movement.Y = collideY(box, blocks, movement.Y)
	box = box.Translate(0, movement.Y, 0)

	if math.Abs(movement.X) < math.Abs(movement.Z) {
		movement.Z = collideZ(box, blocks, movement.Z)
		box = box.Translate(0, 0, movement.Z)
		movement.X = collideX(box, blocks, movement.X)

		return movement
	}

	movement.X = collideX(box, blocks, movement.X)
	box = box.Translate(movement.X, 0, 0)
	movement.Z = collideZ(box, blocks, movement.Z)

	return movement
}

func itemEntityBox(position game.Position) game.AABB {
	halfWidth := itemEntityWidth / 2
	return game.AABB{MinX: position.X - halfWidth, MinY: position.Y, MinZ: position.Z - halfWidth, MaxX: position.X + halfWidth, MaxY: position.Y + itemEntityHeight, MaxZ: position.Z + halfWidth}
}

func collideX(box game.AABB, blocks []game.AABB, delta float64) float64 {
	for _, block := range blocks {
		if block.MaxY <= box.MinY || block.MinY >= box.MaxY || block.MaxZ <= box.MinZ || block.MinZ >= box.MaxZ {
			continue
		}

		if delta > 0 && box.MaxX <= block.MinX {
			delta = min(delta, block.MinX-box.MaxX)
		} else if delta < 0 && box.MinX >= block.MaxX {
			delta = max(delta, block.MaxX-box.MinX)
		}
	}

	return delta
}

func collideY(box game.AABB, blocks []game.AABB, delta float64) float64 {
	for _, block := range blocks {
		if block.MaxX <= box.MinX || block.MinX >= box.MaxX || block.MaxZ <= box.MinZ || block.MinZ >= box.MaxZ {
			continue
		}

		if delta > 0 && box.MaxY <= block.MinY {
			delta = min(delta, block.MinY-box.MaxY)
		} else if delta < 0 && box.MinY >= block.MaxY {
			delta = max(delta, block.MaxY-box.MinY)
		}
	}

	return delta
}

func collideZ(box game.AABB, blocks []game.AABB, delta float64) float64 {
	for _, block := range blocks {
		if block.MaxX <= box.MinX || block.MinX >= box.MaxX || block.MaxY <= box.MinY || block.MinY >= box.MaxY {
			continue
		}

		if delta > 0 && box.MaxZ <= block.MinZ {
			delta = min(delta, block.MinZ-box.MaxZ)
		} else if delta < 0 && box.MinZ >= block.MaxZ {
			delta = max(delta, block.MaxZ-box.MinZ)
		}
	}

	return delta
}

func positionLoadedChunk(position game.Position) LoadedChunk {
	return LoadedChunk{X: chunkCoordinate(position.X), Z: chunkCoordinate(position.Z)}
}

func randomEntityUUID() string {
	var uuid [16]byte

	_, err := cryptorand.Read(uuid[:])
	if err != nil {
		panic(fmt.Sprintf("generate entity UUID: %v", err))
	}

	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
