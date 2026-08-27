package server

import (
	"sort"
	"sync"

	"github.com/coalaura/minicraft/internal/game"
)

type RuntimeEntity interface {
	Tick(*Runtime, *ActiveChunk)
}

type RuntimeBlockEntity interface {
}

type RuntimeBlockEntityTicker interface {
	Tick(*Runtime, *ActiveChunk)
}

type ActiveChunk struct {
	Position LoadedChunk

	mu            sync.RWMutex
	entities      map[uint64]RuntimeEntity
	blockEntities map[game.BlockPosition]RuntimeBlockEntity
}

type activeChunkReference struct {
	chunk      *ActiveChunk
	references int
}

type runtimeEntitySnapshot struct {
	id     uint64
	entity RuntimeEntity
}

type runtimeBlockEntitySnapshot struct {
	position game.BlockPosition
	entity   RuntimeBlockEntityTicker
}

func (c *ActiveChunk) SetEntity(id uint64, entity RuntimeEntity) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entity == nil {
		delete(c.entities, id)

		return
	}

	if c.entities == nil {
		c.entities = make(map[uint64]RuntimeEntity)
	}

	c.entities[id] = entity
}

func (c *ActiveChunk) RemoveEntity(id uint64) {
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

func (c *ActiveChunk) tick(runtime *Runtime) {
	entities, blockEntities := c.snapshotTickers()

	for _, snapshot := range entities {
		snapshot.entity.Tick(runtime, c)
	}

	for _, snapshot := range blockEntities {
		snapshot.entity.Tick(runtime, c)
	}
}

func (c *ActiveChunk) snapshotTickers() ([]runtimeEntitySnapshot, []runtimeBlockEntitySnapshot) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entities := make([]runtimeEntitySnapshot, 0, len(c.entities))

	for id, entity := range c.entities {
		entities = append(entities, runtimeEntitySnapshot{id: id, entity: entity})
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
	defer r.activeChunksMu.Unlock()

	previous := r.sessionActiveChunks[session]

	for position := range next {
		if _, retained := previous[position]; retained {
			continue
		}

		reference := r.activeChunks[position]
		if reference == nil {
			reference = &activeChunkReference{chunk: r.newActiveChunk(position)}

			r.activeChunks[position] = reference
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

		return
	}

	r.sessionActiveChunks[session] = next
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

	for _, chunk := range chunks {
		chunk.tick(r)
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
