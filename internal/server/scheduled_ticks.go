package server

import (
	"sort"

	"github.com/coalaura/minicraft/internal/game"
)

type scheduledBlockTickType string

const scheduledBlockTickFluid scheduledBlockTickType = "fluid"

type scheduledTickPriority int

const scheduledTickPriorityNormal scheduledTickPriority = 0

type scheduledTickKey[T comparable] struct {
	position game.BlockPosition
	typeID   T
}

type scheduledTick[T comparable] struct {
	key      scheduledTickKey[T]
	due      int64
	priority scheduledTickPriority
	suborder uint64
}

// scheduledTickQueue retains a simulation clock for every chunk that has
// scheduled work. A clock only advances while its chunk is active.
type scheduledTickQueue[T comparable] struct {
	chunkClocks  map[LoadedChunk]int64
	pending      map[scheduledTickKey[T]]scheduledTick[T]
	nextSuborder uint64
}

type scheduledBlockTickKey = scheduledTickKey[scheduledBlockTickType]
type scheduledBlockTicks = scheduledTickQueue[scheduledBlockTickType]

func (ticks *scheduledTickQueue[T]) schedule(position game.BlockPosition, typeID T, delay int64, priorities ...scheduledTickPriority) {
	if delay < 1 {
		delay = 1
	}

	priority := scheduledTickPriorityNormal

	if len(priorities) != 0 {
		priority = priorities[0]
	}

	if ticks.chunkClocks == nil {
		ticks.chunkClocks = make(map[LoadedChunk]int64)
	}

	if ticks.pending == nil {
		ticks.pending = make(map[scheduledTickKey[T]]scheduledTick[T])
	}

	key := scheduledTickKey[T]{position: position, typeID: typeID}

	_, present := ticks.pending[key]
	if present {
		return
	}

	chunk := blockLoadedChunk(position)
	due := ticks.chunkClocks[chunk] + delay

	ticks.nextSuborder++

	ticks.pending[key] = scheduledTick[T]{key: key, due: due, priority: priority, suborder: ticks.nextSuborder}
}

func (ticks *scheduledTickQueue[T]) advance(active func(LoadedChunk) bool) []scheduledTick[T] {
	activeChunks := make([]LoadedChunk, 0)
	seen := make(map[LoadedChunk]struct{})

	for _, tick := range ticks.pending {
		chunk := blockLoadedChunk(tick.key.position)

		_, present := seen[chunk]
		if !present && active(chunk) {
			seen[chunk] = struct{}{}
			activeChunks = append(activeChunks, chunk)
		}
	}

	return ticks.advanceChunks(activeChunks)
}

func (ticks *scheduledTickQueue[T]) advanceChunks(activeChunks []LoadedChunk) []scheduledTick[T] {
	if ticks.chunkClocks == nil {
		ticks.chunkClocks = make(map[LoadedChunk]int64)
	}

	active := make(map[LoadedChunk]struct{}, len(activeChunks))

	for _, chunk := range activeChunks {
		active[chunk] = struct{}{}
		ticks.chunkClocks[chunk]++
	}

	due := make([]scheduledTick[T], 0)

	for key, tick := range ticks.pending {
		chunk := blockLoadedChunk(key.position)

		_, chunkActive := active[chunk]
		if !chunkActive {
			continue
		}

		if tick.due > ticks.chunkClocks[chunk] {
			continue
		}

		delete(ticks.pending, key)
		due = append(due, tick)
	}

	sort.Slice(due, func(first, second int) bool {
		if due[first].due != due[second].due {
			return due[first].due < due[second].due
		}

		if due[first].priority != due[second].priority {
			return due[first].priority < due[second].priority
		}

		return due[first].suborder < due[second].suborder
	})

	return due
}

func (r *Runtime) scheduleBlockTickLocked(position game.BlockPosition, typeID scheduledBlockTickType, delay int64) {
	r.scheduledBlockTicks.schedule(position, typeID, delay)
}

func (r *Runtime) tickScheduledBlocksLocked() {
	ticks := r.scheduledBlockTicks.advanceChunks(r.activeLoadedChunksLocked())

	for _, tick := range ticks {
		switch tick.key.typeID {
		case scheduledBlockTickFluid:
			r.tickFluidLocked(tick.key.position)
		}
	}
}

func (r *Runtime) activeLoadedChunksLocked() []LoadedChunk {
	r.activeChunksMu.RLock()
	defer r.activeChunksMu.RUnlock()

	chunks := make([]LoadedChunk, 0, len(r.activeChunks))

	for chunk := range r.activeChunks {
		chunks = append(chunks, chunk)
	}

	return chunks
}
