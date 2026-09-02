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
	blockEntities  map[game.BlockPosition]RuntimeBlockEntity
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

		return
	}

	if c.entities == nil {
		c.entities = make(map[int32]RuntimeEntity)
	}

	c.entities[id] = entity
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

		return
	}

	if c.blockEntities == nil {
		c.blockEntities = make(map[game.BlockPosition]RuntimeBlockEntity)
	}

	c.blockEntities[position] = entity
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

func (c *ActiveChunk) snapshotTickers() ([]runtimeEntitySnapshot, []runtimeBlockEntitySnapshot) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entities := make([]runtimeEntitySnapshot, 0, len(c.entities))

	for id, entity := range c.entities {
		ticker, ticks := entity.(RuntimeEntityTicker)
		if ticks {
			entities = append(entities, runtimeEntitySnapshot{id: id, entity: ticker})
		}
	}

	sort.Slice(entities, func(first, second int) bool {
		return entities[first].id < entities[second].id
	})

	blockEntities := make([]runtimeBlockEntitySnapshot, 0, len(c.blockEntities))

	for position, entity := range c.blockEntities {
		ticker, ticks := entity.(RuntimeBlockEntityTicker)
		if ticks {
			blockEntities = append(blockEntities, runtimeBlockEntitySnapshot{position: position, entity: ticker})
		}
	}

	sort.Slice(blockEntities, func(first, second int) bool {
		firstPosition := blockEntities[first].position
		secondPosition := blockEntities[second].position

		if firstPosition.X != secondPosition.X {
			return firstPosition.X < secondPosition.X
		}

		if firstPosition.Y != secondPosition.Y {
			return firstPosition.Y < secondPosition.Y
		}

		return firstPosition.Z < secondPosition.Z
	})

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
	next := make(map[LoadedChunk]struct{}, len(chunks))

	for _, chunk := range chunks {
		next[chunk] = struct{}{}
	}

	r.activeChunksMu.Lock()

	previous := r.sessionActiveChunks[session]

	activated := make([]LoadedChunk, 0)

	for position := range next {
		if _, retained := previous[position]; retained {
			continue
		}

		reference := r.activeChunks[position]
		if reference == nil {
			reference = &activeChunkReference{chunk: r.newActiveChunk(position)}

			r.activeChunks[position] = reference

			activated = append(activated, position)
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
		}
	}

	if len(next) == 0 {
		delete(r.sessionActiveChunks, session)
	} else {
		r.sessionActiveChunks[session] = next
	}

	r.activeChunksMu.Unlock()

	if len(activated) == 0 {
		return
	}

	r.worldMutationMu.Lock()
	r.resumeDeferredFluidSourcesLocked(activated)
	r.worldMutationMu.Unlock()
}

func (r *Runtime) releaseSessionActiveChunks(session *Session) {
	session.chunkMx.Lock()
	defer session.chunkMx.Unlock()

	if session.runtimeChunksReleased {
		return
	}

	session.runtimeChunksReleased = true

	r.setSessionActiveChunks(session, nil)
}

func (r *Runtime) tickActiveChunks() {
	// Runtime relevance is sampled once per tick. A chunk deactivated during
	// callbacks finishes that snapshot but cannot appear in the next tick.
	chunks := r.snapshotActiveChunks()

	snapshots := make([]activeChunkTickSnapshot, len(chunks))

	// Snapshot every chunk before ticking so an entity crossing into a later
	// chunk cannot be observed and ticked twice in the same game tick.
	for index, chunk := range chunks {
		entities, blockEntities := chunk.snapshotTickers()

		snapshots[index] = activeChunkTickSnapshot{chunk: chunk, entities: entities, blockEntities: blockEntities}
	}

	for _, snapshot := range snapshots {
		for _, entity := range snapshot.entities {
			entity.entity.Tick(r, snapshot.chunk)
		}

		for _, blockEntity := range snapshot.blockEntities {
			blockEntity.entity.Tick(r, snapshot.chunk)
		}
	}
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
