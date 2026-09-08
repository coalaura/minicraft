package server

import (
	"slices"
	"sort"
	"sync"

	"github.com/coalaura/minicraft/internal/game"
)

type RuntimeEntity interface {
	RuntimeEntityState() *RuntimeEntityState
}

type RuntimeEntityTicker interface {
	Tick(*Runtime, *ActiveChunk)
}

type ActiveChunk struct {
	Position LoadedChunk

	mu             sync.RWMutex
	entities       map[int32]RuntimeEntity
	entityTickers  []runtimeEntitySnapshot
	blockEntities  map[game.BlockPosition]RuntimeBlockEntity
	blockTickers   []runtimeBlockEntitySnapshot
	randomSections map[int32]struct{}
}

type activeChunkReference struct {
	chunk      *ActiveChunk
	references int
}

type runtimeEntitySnapshot struct {
	id     int32
	entity RuntimeEntityTicker
}

type runtimeBlockEntitySnapshot struct {
	position game.BlockPosition
	entity   RuntimeBlockEntityTicker
}

type activeChunkTickSnapshot struct {
	chunk         *ActiveChunk
	entities      []runtimeEntitySnapshot
	blockEntities []runtimeBlockEntitySnapshot
}

func (c *ActiveChunk) SetEntity(id int32, entity RuntimeEntity) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entity == nil {
		delete(c.entities, id)
		c.removeEntityTicker(id)

		return
	}

	if c.entities == nil {
		c.entities = make(map[int32]RuntimeEntity)
	}

	c.entities[id] = entity

	ticker, ticks := entity.(RuntimeEntityTicker)
	if ticks {
		c.setEntityTicker(id, ticker)

		return
	}

	c.removeEntityTicker(id)
}

func (c *ActiveChunk) RemoveEntity(id int32) {
	c.SetEntity(id, nil)
}

func (c *ActiveChunk) EntityCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entities)
}

func (c *ActiveChunk) SetBlockEntity(position game.BlockPosition, entity RuntimeBlockEntity) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entity == nil {
		delete(c.blockEntities, position)

		c.removeBlockTicker(position)

		return
	}

	if c.blockEntities == nil {
		c.blockEntities = make(map[game.BlockPosition]RuntimeBlockEntity)
	}

	c.blockEntities[position] = entity

	ticker, ticks := entity.(RuntimeBlockEntityTicker)
	if ticks {
		c.setBlockTicker(position, ticker)

		return
	}

	c.removeBlockTicker(position)
}

func (c *ActiveChunk) RemoveBlockEntity(position game.BlockPosition) {
	c.SetBlockEntity(position, nil)
}

func (c *ActiveChunk) BlockEntityCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.blockEntities)
}

func (c *ActiveChunk) BlockEntity(position game.BlockPosition) (RuntimeBlockEntity, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entity, present := c.blockEntities[position]
	return entity, present
}

func (c *ActiveChunk) markRandomTickSection(sectionMinY int32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.randomSections == nil {
		c.randomSections = make(map[int32]struct{})
	}

	c.randomSections[sectionMinY] = struct{}{}
}

func (c *ActiveChunk) snapshotRandomTickSections() []int32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sections := make([]int32, 0, len(c.randomSections))

	for sectionMinY := range c.randomSections {
		sections = append(sections, sectionMinY)
	}

	slices.Sort(sections)

	return sections
}

func (c *ActiveChunk) snapshotTickers(entities []runtimeEntitySnapshot, blockEntities []runtimeBlockEntitySnapshot) ([]runtimeEntitySnapshot, []runtimeBlockEntitySnapshot) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entities = copyEntityTickers(entities, c.entityTickers)
	blockEntities = copyBlockTickers(blockEntities, c.blockTickers)

	return entities, blockEntities
}

func (r *Runtime) ActiveChunk(position LoadedChunk) (*ActiveChunk, bool) {
	r.activeChunksMu.RLock()
	defer r.activeChunksMu.RUnlock()

	reference, active := r.activeChunks[position]
	if !active {
		return nil, false
	}

	return reference.chunk, true
}

func (r *Runtime) ActiveChunkCount() int {
	r.activeChunksMu.RLock()
	defer r.activeChunksMu.RUnlock()

	return len(r.activeChunks)
}

func (r *Runtime) setSessionActiveChunks(session *Session, chunks []LoadedChunk) {
	activated := r.replaceSessionActiveChunks(session, chunks)

	r.resumeActivatedChunks(activated)
}

func (r *Runtime) replaceSessionActiveChunks(session *Session, chunks []LoadedChunk) []LoadedChunk {
	next := make(map[LoadedChunk]struct{}, len(chunks))

	for _, chunk := range chunks {
		next[chunk] = struct{}{}
	}

	r.activeChunksMu.Lock()

	previous := r.sessionActiveChunks[session]

	activated := make([]LoadedChunk, 0)
	topologyChanged := false

	for position := range next {
		if _, retained := previous[position]; retained {
			continue
		}

		reference := r.activeChunks[position]
		if reference == nil {
			reference = &activeChunkReference{chunk: r.newActiveChunk(position)}

			r.activeChunks[position] = reference

			activated = append(activated, position)
			topologyChanged = true
		}

		reference.references++
	}

	for position := range previous {
		if _, retained := next[position]; retained {
			continue
		}

		reference := r.activeChunks[position]

		reference.references--

		if reference.references == 0 {
			delete(r.activeChunks, position)
			topologyChanged = true
		}
	}

	if len(next) == 0 {
		delete(r.sessionActiveChunks, session)
	} else {
		r.sessionActiveChunks[session] = next
	}

	if topologyChanged {
		r.rebuildActiveChunkListLocked()
	}

	r.activeChunksMu.Unlock()

	return activated
}

func (r *Runtime) resumeActivatedChunks(activated []LoadedChunk) {
	if len(activated) == 0 {
		return
	}

	r.worldMutationMu.Lock()
	r.resumeDeferredFluidSourcesLocked(activated)
	r.worldMutationMu.Unlock()
}

func (r *Runtime) releaseSessionActiveChunks(session *Session) {
	session.chunkMx.Lock()

	if session.runtimeChunksReleased {
		session.chunkMx.Unlock()

		return
	}

	session.runtimeChunksReleased = true
	activated := r.replaceSessionActiveChunks(session, nil)

	session.chunkMx.Unlock()

	r.resumeActivatedChunks(activated)
}

func (r *Runtime) tickActiveChunks() {
	r.activeChunkTickMu.Lock()
	clear(r.itemFluidFlowCache)
	r.itemFluidFlowCacheActive = true

	defer func() {
		r.itemFluidFlowCacheActive = false
		r.activeChunkTickMu.Unlock()
	}()

	// Runtime relevance is sampled once per tick. A chunk deactivated during
	// callbacks finishes that snapshot but cannot appear in the next tick.
	r.activeChunksMu.RLock()

	chunkCount := len(r.activeChunkList)

	for len(r.activeChunkTickSnapshots) < chunkCount {
		r.activeChunkTickSnapshots = append(r.activeChunkTickSnapshots, activeChunkTickSnapshot{})
	}

	for index := chunkCount; index < len(r.activeChunkTickSnapshots); index++ {
		snapshot := &r.activeChunkTickSnapshots[index]

		clear(snapshot.entities)
		clear(snapshot.blockEntities)
		snapshot.chunk = nil
	}

	// Snapshot every chunk before ticking so an entity crossing into a later
	// chunk cannot be observed and ticked twice in the same game tick.
	for index := range chunkCount {
		snapshot := &r.activeChunkTickSnapshots[index]
		snapshot.chunk = r.activeChunkList[index]
	}

	r.activeChunksMu.RUnlock()

	for index := range chunkCount {
		snapshot := &r.activeChunkTickSnapshots[index]

		entities, blockEntities := snapshot.chunk.snapshotTickers(snapshot.entities, snapshot.blockEntities)

		snapshot.entities = entities
		snapshot.blockEntities = blockEntities
	}

	for index := range chunkCount {
		snapshot := &r.activeChunkTickSnapshots[index]

		for _, entity := range snapshot.entities {
			entity.entity.Tick(r, snapshot.chunk)
		}

		for _, blockEntity := range snapshot.blockEntities {
			blockEntity.entity.Tick(r, snapshot.chunk)
		}
	}
}

func (c *ActiveChunk) setEntityTicker(id int32, ticker RuntimeEntityTicker) {
	index := sort.Search(len(c.entityTickers), func(index int) bool {
		return c.entityTickers[index].id >= id
	})

	if index < len(c.entityTickers) && c.entityTickers[index].id == id {
		c.entityTickers[index].entity = ticker

		return
	}

	c.entityTickers = slices.Insert(c.entityTickers, index, runtimeEntitySnapshot{id: id, entity: ticker})
}

func (c *ActiveChunk) removeEntityTicker(id int32) {
	index := sort.Search(len(c.entityTickers), func(index int) bool {
		return c.entityTickers[index].id >= id
	})

	if index >= len(c.entityTickers) || c.entityTickers[index].id != id {
		return
	}

	c.entityTickers = slices.Delete(c.entityTickers, index, index+1)
}

func (c *ActiveChunk) setBlockTicker(position game.BlockPosition, ticker RuntimeBlockEntityTicker) {
	index := sort.Search(len(c.blockTickers), func(index int) bool {
		return blockPositionCompare(c.blockTickers[index].position, position) >= 0
	})

	if index < len(c.blockTickers) && c.blockTickers[index].position == position {
		c.blockTickers[index].entity = ticker

		return
	}

	c.blockTickers = slices.Insert(c.blockTickers, index, runtimeBlockEntitySnapshot{position: position, entity: ticker})
}

func (c *ActiveChunk) removeBlockTicker(position game.BlockPosition) {
	index := sort.Search(len(c.blockTickers), func(index int) bool {
		return blockPositionCompare(c.blockTickers[index].position, position) >= 0
	})

	if index >= len(c.blockTickers) || c.blockTickers[index].position != position {
		return
	}

	c.blockTickers = slices.Delete(c.blockTickers, index, index+1)
}

func (r *Runtime) rebuildActiveChunkListLocked() {
	previousCount := len(r.activeChunkList)
	r.activeChunkList = r.activeChunkList[:0]

	for _, reference := range r.activeChunks {
		r.activeChunkList = append(r.activeChunkList, reference.chunk)
	}

	if len(r.activeChunkList) < previousCount {
		clear(r.activeChunkList[len(r.activeChunkList):previousCount])
	}

	sort.Slice(r.activeChunkList, func(first, second int) bool {
		firstPosition := r.activeChunkList[first].Position
		secondPosition := r.activeChunkList[second].Position

		if firstPosition.X != secondPosition.X {
			return firstPosition.X < secondPosition.X
		}

		return firstPosition.Z < secondPosition.Z
	})
}

func copyEntityTickers(destination, source []runtimeEntitySnapshot) []runtimeEntitySnapshot {
	previousCount := len(destination)
	destination = append(destination[:0], source...)

	if len(source) < previousCount {
		clear(destination[len(source):previousCount])
	}

	return destination
}

func copyBlockTickers(destination, source []runtimeBlockEntitySnapshot) []runtimeBlockEntitySnapshot {
	previousCount := len(destination)
	destination = append(destination[:0], source...)

	if len(source) < previousCount {
		clear(destination[len(source):previousCount])
	}

	return destination
}

func blockPositionCompare(first, second game.BlockPosition) int {
	if first.X != second.X {
		if first.X < second.X {
			return -1
		}

		return 1
	}

	if first.Y != second.Y {
		if first.Y < second.Y {
			return -1
		}

		return 1
	}

	if first.Z < second.Z {
		return -1
	}

	if first.Z > second.Z {
		return 1
	}

	return 0
}

func (r *Runtime) snapshotActiveChunks() []*ActiveChunk {
	r.activeChunksMu.RLock()
	defer r.activeChunksMu.RUnlock()

	chunks := make([]*ActiveChunk, 0, len(r.activeChunks))

	for _, reference := range r.activeChunks {
		chunks = append(chunks, reference.chunk)
	}

	sort.Slice(chunks, func(first, second int) bool {
		if chunks[first].Position.X != chunks[second].Position.X {
			return chunks[first].Position.X < chunks[second].Position.X
		}

		return chunks[first].Position.Z < chunks[second].Position.Z
	})

	return chunks
}
