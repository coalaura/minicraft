package server

import (
	"crypto/rand"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	itemEntityWidth         = 0.25
	itemEntityHeight        = 0.25
	itemEntityGravity       = 0.04
	itemEntityDrag          = 0.98
	itemEntityLifetime      = 6000
	itemEntityTrackingRange = 6 * ChunkWidth
)

type RuntimeEntityState struct {
	ID       int32
	UUID     string
	Position game.Position
	Chunk    LoadedChunk
	Removed  bool
}

type RuntimeEntityMetadata interface {
	EntityMetadata() []protocol.EntityMetadataEntry
}

type RuntimeEntityVelocity interface {
	EntityVelocity() game.Velocity
}

type RuntimeEntitySpawner interface {
	AddEntityPacket() protocol.AddEntity
}

type runtimeItemEntity struct {
	State       RuntimeEntityState
	Stack       game.ItemStack
	Velocity    game.Velocity
	Age         int32
	PickupDelay int32
	TargetUUID  string
	OnGround    bool
	TickCount   int32
}

func (entity *runtimeItemEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *runtimeItemEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	return []protocol.EntityMetadataEntry{{
		Index: protocol.ItemEntityItemMetadataIndex,
		Type:  protocol.MetadataTypeItemStack,
		Value: protocol.MetadataItemStack{Stack: entity.Stack.Clone()},
	}}
}

func (entity *runtimeItemEntity) EntityVelocity() game.Velocity {
	return entity.Velocity
}

func (entity *runtimeItemEntity) AddEntityPacket() protocol.AddEntity {
	return protocol.AddEntity{
		EntityID:  entity.State.ID,
		UUID:      entity.State.UUID,
		Type:      protocol.ItemEntityType,
		X:         entity.State.Position.X,
		Y:         entity.State.Position.Y,
		Z:         entity.State.Position.Z,
		VelocityX: entity.Velocity.X,
		VelocityY: entity.Velocity.Y,
		VelocityZ: entity.Velocity.Z,
	}
}

func (entity *runtimeItemEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	if entity.State.Removed {
		return
	}

	entity.TickCount++
	if entity.PickupDelay > 0 && entity.PickupDelay != 32767 {
		entity.PickupDelay--
	}

	if entity.Age != -32768 {
		entity.Age++
	}

	if entity.Age >= itemEntityLifetime {
		runtime.removeRuntimeEntity(entity.State.ID)
		return
	}

	previous := entity.State.Position

	entity.Velocity.Y -= itemEntityGravity
	entity.State.Position, entity.OnGround = runtime.moveItemEntity(entity.State.Position, entity.Velocity)

	horizontalDrag := itemEntityDrag
	if entity.OnGround {
		horizontalDrag *= runtime.blockFrictionBelow(entity.State.Position)
		if entity.Velocity.Y < 0 {
			entity.Velocity.Y *= -0.5
		}
	}

	entity.Velocity.X *= horizontalDrag
	entity.Velocity.Y *= itemEntityDrag
	entity.Velocity.Z *= horizontalDrag

	runtime.runtimeEntityMoved(entity, previous)

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
		PickupDelay: pickupDelay,
	}

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

func (r *Runtime) spawnPlayerDroppedItem(player game.Player, stack game.ItemStack, randomly bool) *runtimeItemEntity {
	if stack.Empty() {
		return nil
	}

	position := player.EyePosition()

	position.Y -= 0.3

	var velocity game.Velocity
	if randomly {
		power := float64(r.nextEntityRandom()) * 0.5
		direction := float64(r.nextEntityRandom()) * math.Pi * 2

		velocity = game.Velocity{X: -math.Sin(direction) * power, Y: 0.2, Z: math.Cos(direction) * power}
	} else {
		yaw := float64(player.Rotation.Yaw) * math.Pi / 180
		pitch := float64(player.Rotation.Pitch) * math.Pi / 180
		direction := float64(r.nextEntityRandom()) * math.Pi * 2
		power := float64(r.nextEntityRandom()) * 0.02

		velocity.X = -math.Sin(yaw)*math.Cos(pitch)*0.3 + math.Cos(direction)*power
		velocity.Y = -math.Sin(pitch)*0.3 + 0.1 + (float64(r.nextEntityRandom())-float64(r.nextEntityRandom()))*0.1
		velocity.Z = math.Cos(yaw)*math.Cos(pitch)*0.3 + math.Sin(direction)*power
	}

	return r.SpawnItemEntity(stack, position, velocity, 40)
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
	state := entity.RuntimeEntityState()

	indexed := r.entitiesByChunk[state.Chunk]
	if indexed == nil {
		indexed = make(map[int32]RuntimeEntity)

		r.entitiesByChunk[state.Chunk] = indexed
	}

	indexed[state.ID] = entity
}

func (r *Runtime) runtimeEntityMoved(entity RuntimeEntity, previous game.Position) {
	state := entity.RuntimeEntityState()

	previousChunk := positionLoadedChunk(previous)
	nextChunk := positionLoadedChunk(state.Position)

	if previousChunk == nextChunk {
		return
	}

	r.entityMu.Lock()

	delete(r.entitiesByChunk[previousChunk], state.ID)

	if len(r.entitiesByChunk[previousChunk]) == 0 {
		delete(r.entitiesByChunk, previousChunk)
	}

	state.Chunk = nextChunk

	r.addEntityToChunkIndexLocked(entity)

	r.entityMu.Unlock()

	previousActive, active := r.ActiveChunk(previousChunk)
	if active {
		previousActive.RemoveEntity(state.ID)
	}

	nextActive, active := r.ActiveChunk(nextChunk)
	if active {
		nextActive.SetEntity(state.ID, entity)
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

	state.Removed = true

	delete(r.entities, id)
	delete(r.entitiesByChunk[state.Chunk], id)

	if len(r.entitiesByChunk[state.Chunk]) == 0 {
		delete(r.entitiesByChunk, state.Chunk)
	}

	r.entityMu.Unlock()

	chunk, active := r.ActiveChunk(state.Chunk)
	if active {
		chunk.RemoveEntity(id)
	}

	for _, session := range r.snapshotSessions() {
		session.untrackRuntimeEntity(id)
	}
}

func (r *Runtime) synchronizeRuntimeEntity(entity RuntimeEntity) {
	state := entity.RuntimeEntityState()

	r.reconcileRuntimeEntityTracking(entity)

	velocity := game.Velocity{}

	moving, moves := entity.(RuntimeEntityVelocity)
	if moves {
		velocity = moving.EntityVelocity()
	}

	packet := protocol.SynchronizeEntityPosition{
		EntityID:  state.ID,
		X:         state.Position.X,
		Y:         state.Position.Y,
		Z:         state.Position.Z,
		VelocityX: velocity.X,
		VelocityY: velocity.Y,
		VelocityZ: velocity.Z,
	}

	for _, session := range r.snapshotSessions() {
		if !session.tracksRuntimeEntity(state.ID) {
			continue
		}

		err := session.writePacket(protocol.ClientboundSynchronizeEntityPositionID, packet)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize entity: %v\n", err)
		}
	}
}

func (r *Runtime) reconcileRuntimeEntityTracking(entity RuntimeEntity) {
	for _, session := range r.snapshotSessions() {
		if session.shouldTrackRuntimeEntity(entity) {
			session.trackRuntimeEntity(entity)
		} else {
			session.untrackRuntimeEntity(entity.RuntimeEntityState().ID)
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
	state := entity.RuntimeEntityState()
	if state.Removed {
		return false
	}

	s.chunkMx.Lock()
	_, loaded := s.loadedChunks[state.Chunk]
	s.chunkMx.Unlock()

	if !loaded {
		return false
	}

	player := s.snapshotPlayer()

	distanceX := player.Position.X - state.Position.X
	distanceZ := player.Position.Z - state.Position.Z

	return distanceX*distanceX+distanceZ*distanceZ <= itemEntityTrackingRange*itemEntityTrackingRange
}

func (s *Session) trackRuntimeEntity(entity RuntimeEntity) {
	state := entity.RuntimeEntityState()

	s.entityTrackMu.Lock()
	if s.trackedEntities == nil {
		s.trackedEntities = make(map[int32]struct{})
	}

	if _, tracked := s.trackedEntities[state.ID]; tracked {
		s.entityTrackMu.Unlock()
		return
	}

	s.trackedEntities[state.ID] = struct{}{}
	s.entityTrackMu.Unlock()

	spawner, spawnable := entity.(RuntimeEntitySpawner)
	if !spawnable {
		s.untrackRuntimeEntity(state.ID)

		return
	}

	err := s.writePacket(protocol.ClientboundAddEntityID, spawner.AddEntityPacket())
	if err == nil {
		metadata, present := entity.(RuntimeEntityMetadata)
		if present {
			err = s.writePacket(protocol.ClientboundEntityMetadataID, protocol.EntityMetadata{EntityID: state.ID, Entries: metadata.EntityMetadata()})
		}
	}

	if err != nil && s.Log != nil {
		s.Log.Warnf("[play] failed to track entity: %v\n", err)
	}

	if err != nil {
		s.untrackRuntimeEntity(state.ID)
	}
}

func (s *Session) untrackRuntimeEntity(id int32) {
	s.entityTrackMu.Lock()
	if _, tracked := s.trackedEntities[id]; !tracked {
		s.entityTrackMu.Unlock()

		return
	}

	delete(s.trackedEntities, id)
	s.entityTrackMu.Unlock()

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
	if entity.PickupDelay != 0 {
		return false
	}

	itemBox := itemEntityBox(entity.State.Position)

	pickupBox := game.AABB{MinX: itemBox.MinX - 1, MinY: itemBox.MinY - 0.5, MinZ: itemBox.MinZ - 1, MaxX: itemBox.MaxX + 1, MaxY: itemBox.MaxY + 0.5, MaxZ: itemBox.MaxZ + 1}

	for _, session := range r.snapshotSessions() {
		player := session.snapshotPlayer()
		if entity.TargetUUID != "" && entity.TargetUUID != player.UUID || !pickupBox.Intersects(player.CollisionBox()) {
			continue
		}

		before := player.Inventory.Clone()
		remaining := entity.Stack.Clone()

		changed := false

		player, changed = session.updatePlayerState(func(current *game.Player) bool {
			playerMenu := newPlayerInventoryMenu(&current.Inventory)

			candidate := playerMenu.candidate()

			moveIntoSlots(candidate, &remaining, slotRange(36, 44))
			moveIntoSlots(candidate, &remaining, slotRange(9, 35))

			if remaining.Equal(entity.Stack) {
				return false
			}

			playerMenu.commit(candidate)

			return true
		})

		if !changed {
			continue
		}

		originalCount := entity.Stack.Count
		entity.Stack = remaining

		for _, viewer := range r.snapshotSessions() {
			if viewer.tracksRuntimeEntity(entity.State.ID) {
				err := viewer.writePacket(protocol.ClientboundTakeItemEntityID, protocol.TakeItemEntity{ItemEntityID: entity.State.ID, PlayerEntityID: player.EntityID, Amount: originalCount})
				if err != nil && viewer.Log != nil {
					viewer.Log.Warnf("[play] failed to animate item pickup: %v\n", err)
				}
			}
		}

		err := session.synchronizePlayerInventoryMutation(before)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize item pickup: %v\n", err)
		}

		if entity.Stack.Empty() {
			r.removeRuntimeEntity(entity.State.ID)
		} else {
			r.synchronizeRuntimeEntityMetadata(entity)
		}

		return true
	}

	return false
}

func (r *Runtime) synchronizeRuntimeEntityMetadata(entity RuntimeEntityMetadata) {
	stateful := entity.(RuntimeEntity)

	state := stateful.RuntimeEntityState()

	packet := protocol.EntityMetadata{EntityID: state.ID, Entries: entity.EntityMetadata()}

	for _, session := range r.snapshotSessions() {
		if session.tracksRuntimeEntity(state.ID) {
			_ = session.writePacket(protocol.ClientboundEntityMetadataID, packet)
		}
	}
}

func (r *Runtime) moveItemEntity(position game.Position, velocity game.Velocity) (game.Position, bool) {
	box := itemEntityBox(position)

	blocks := r.itemCollisionBoxes(box, velocity)

	deltaY := collideY(box, blocks, velocity.Y)
	box = box.Translate(0, deltaY, 0)

	deltaX := collideX(box, blocks, velocity.X)
	box = box.Translate(deltaX, 0, 0)

	deltaZ := collideZ(box, blocks, velocity.Z)

	position.X += deltaX
	position.Y += deltaY
	position.Z += deltaZ

	return position, velocity.Y < 0 && deltaY != velocity.Y
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

func (r *Runtime) blockFrictionBelow(position game.Position) float64 {
	block := r.World.BlockAt(game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y - 0.01)), Z: int32(math.Floor(position.Z))})

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

	_, err := rand.Read(uuid[:])
	if err != nil {
		panic(fmt.Sprintf("generate entity UUID: %v", err))
	}

	uuid[6] = uuid[6]&0x0f | 0x40
	uuid[8] = uuid[8]&0x3f | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
