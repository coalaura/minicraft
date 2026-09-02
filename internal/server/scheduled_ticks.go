package server

import (
	"container/heap"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	scheduledTickPriorityNormal scheduledTickPriority = 0
	scheduledTickDrainLimit     int                   = 65_536
)

type scheduledTickPriority int

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

type scheduledTickHeap[T comparable] []scheduledTick[T]

type scheduledTickReadyHeap[T comparable] []scheduledTick[T]

type scheduledTickChunk[T comparable] struct {
	clock   int64
	pending map[scheduledTickKey[T]]struct{}
	queue   scheduledTickHeap[T]
	ready   scheduledTickReadyHeap[T]
}

type scheduledTickCandidate[T comparable] struct {
	position LoadedChunk
	chunk    *scheduledTickChunk[T]
}

type scheduledTickCandidateHeap[T comparable] []scheduledTickCandidate[T]

// scheduledTickQueue owns work by chunk. Empty chunks are discarded because
// every due time is relative to the same local clock retained with its work.
type scheduledTickQueue[T comparable] struct {
	chunks       map[LoadedChunk]*scheduledTickChunk[T]
	nextSuborder uint64
}

type scheduledBlockTickKey = scheduledTickKey[game.BlockID]
type scheduledFluidTickKey = scheduledTickKey[game.FluidStateType]
type scheduledBlockTicks = scheduledTickQueue[game.BlockID]
type scheduledFluidTicks = scheduledTickQueue[game.FluidStateType]

func (ticks scheduledTickHeap[T]) Len() int {
	return len(ticks)
}

func (ticks scheduledTickHeap[T]) Less(first, second int) bool {
	return scheduledTickBefore(ticks[first], ticks[second])
}

func (ticks scheduledTickHeap[T]) Swap(first, second int) {
	ticks[first], ticks[second] = ticks[second], ticks[first]
}

func (ticks *scheduledTickHeap[T]) Push(value any) {
	tick := value.(scheduledTick[T])

	*ticks = append(*ticks, tick)
}

func (ticks *scheduledTickHeap[T]) Pop() any {
	old := *ticks
	last := len(old) - 1
	tick := old[last]

	old[last] = scheduledTick[T]{}
	*ticks = old[:last]

	return tick
}

func (ticks scheduledTickReadyHeap[T]) Len() int {
	return len(ticks)
}

func (ticks scheduledTickReadyHeap[T]) Less(first, second int) bool {
	return scheduledTickReadyBefore(ticks[first], ticks[second])
}

func (ticks scheduledTickReadyHeap[T]) Swap(first, second int) {
	ticks[first], ticks[second] = ticks[second], ticks[first]
}

func (ticks *scheduledTickReadyHeap[T]) Push(value any) {
	tick := value.(scheduledTick[T])

	*ticks = append(*ticks, tick)
}

func (ticks *scheduledTickReadyHeap[T]) Pop() any {
	old := *ticks
	last := len(old) - 1
	tick := old[last]

	old[last] = scheduledTick[T]{}
	*ticks = old[:last]

	return tick
}

func (candidates scheduledTickCandidateHeap[T]) Len() int {
	return len(candidates)
}

func (candidates scheduledTickCandidateHeap[T]) Less(first, second int) bool {
	return scheduledTickReadyBefore(candidates[first].chunk.ready[0], candidates[second].chunk.ready[0])
}

func (candidates scheduledTickCandidateHeap[T]) Swap(first, second int) {
	candidates[first], candidates[second] = candidates[second], candidates[first]
}

func (candidates *scheduledTickCandidateHeap[T]) Push(value any) {
	candidate := value.(scheduledTickCandidate[T])

	*candidates = append(*candidates, candidate)
}

func (candidates *scheduledTickCandidateHeap[T]) Pop() any {
	old := *candidates
	last := len(old) - 1
	candidate := old[last]

	old[last] = scheduledTickCandidate[T]{}
	*candidates = old[:last]

	return candidate
}

func (ticks *scheduledTickQueue[T]) schedule(position game.BlockPosition, typeID T, delay int64, priorities ...scheduledTickPriority) {
	if delay < 1 {
		delay = 1
	}

	priority := scheduledTickPriorityNormal

	if len(priorities) != 0 {
		priority = priorities[0]
	}

	if ticks.chunks == nil {
		ticks.chunks = make(map[LoadedChunk]*scheduledTickChunk[T])
	}

	positionChunk := blockLoadedChunk(position)
	chunk := ticks.chunks[positionChunk]

	if chunk == nil {
		chunk = &scheduledTickChunk[T]{pending: make(map[scheduledTickKey[T]]struct{})}
		ticks.chunks[positionChunk] = chunk
	}

	key := scheduledTickKey[T]{position: position, typeID: typeID}

	_, present := chunk.pending[key]
	if present {
		return
	}

	ticks.nextSuborder++

	tick := scheduledTick[T]{key: key, due: chunk.clock + delay, priority: priority, suborder: ticks.nextSuborder}

	chunk.pending[key] = struct{}{}
	heap.Push(&chunk.queue, tick)
}

func (ticks *scheduledTickQueue[T]) advance(active func(LoadedChunk) bool, limits ...int) []scheduledTick[T] {
	limit := scheduledTickDrainLimit

	if len(limits) != 0 {
		limit = limits[0]
	}

	activeChunks := make(map[LoadedChunk]*activeChunkReference)

	for position := range ticks.chunks {
		if active(position) {
			activeChunks[position] = nil
		}
	}

	return ticks.advanceChunks(activeChunks, limit)
}

func (ticks *scheduledTickQueue[T]) advanceChunks(activeChunks map[LoadedChunk]*activeChunkReference, limit int) []scheduledTick[T] {
	if limit <= 0 || len(ticks.chunks) == 0 {
		return nil
	}

	candidates := make(scheduledTickCandidateHeap[T], 0, min(len(activeChunks), len(ticks.chunks)))

	for position := range activeChunks {
		chunk := ticks.chunks[position]
		if chunk == nil {
			continue
		}

		chunk.clock++

		for len(chunk.queue) != 0 && chunk.queue[0].due <= chunk.clock {
			tick := heap.Pop(&chunk.queue).(scheduledTick[T])
			heap.Push(&chunk.ready, tick)
		}

		if len(chunk.ready) != 0 {
			candidates = append(candidates, scheduledTickCandidate[T]{position: position, chunk: chunk})
		}
	}

	heap.Init(&candidates)

	due := make([]scheduledTick[T], 0, min(limit, len(candidates)))

	for len(candidates) != 0 && len(due) < limit {
		candidate := heap.Pop(&candidates).(scheduledTickCandidate[T])
		tick := heap.Pop(&candidate.chunk.ready).(scheduledTick[T])

		delete(candidate.chunk.pending, tick.key)

		due = append(due, tick)

		if len(candidate.chunk.queue) == 0 && len(candidate.chunk.ready) == 0 {
			delete(ticks.chunks, candidate.position)

			continue
		}

		if len(candidate.chunk.ready) != 0 {
			heap.Push(&candidates, candidate)
		}
	}

	return due
}

func (ticks *scheduledTickQueue[T]) contains(key scheduledTickKey[T]) bool {
	chunk := ticks.chunks[blockLoadedChunk(key.position)]
	if chunk == nil {
		return false
	}

	_, present := chunk.pending[key]
	return present
}

func (ticks *scheduledTickQueue[T]) len() int {
	total := 0

	for _, chunk := range ticks.chunks {
		total += len(chunk.pending)
	}

	return total
}

func (ticks *scheduledTickQueue[T]) chunkCount() int {
	return len(ticks.chunks)
}

func (r *Runtime) scheduleBlockTickLocked(position game.BlockPosition, block game.Block, delay int64) {
	definition, valid := block.Definition()
	if !valid {
		return
	}

	r.scheduledBlockTicks.schedule(position, definition.ID, delay)
}

func (r *Runtime) tickScheduledBlocksLocked() {
	ticks := r.advanceScheduledTicksLocked(&r.scheduledBlockTicks)

	for _, tick := range ticks {
		block := r.World.BlockAt(tick.key.position)

		definition, valid := block.Definition()
		if !valid || definition.ID != tick.key.typeID {
			continue
		}

		r.tickBlockLocked(tick.key.position, block)
	}
}

func (r *Runtime) scheduleFluidTickLocked(position game.BlockPosition, typeID game.FluidStateType, delay int64) {
	r.scheduledFluidTicks.schedule(position, typeID, delay)
}

func (r *Runtime) tickScheduledFluidsLocked() int {
	ticks := r.advanceScheduledTicksLocked(&r.scheduledFluidTicks)
	executed := 0

	for _, tick := range ticks {
		state := r.World.FluidAt(tick.key.position)
		if state.StateType() != tick.key.typeID {
			continue
		}

		r.tickFluidLocked(tick.key.position)
		executed++
	}

	return executed
}

func (r *Runtime) advanceScheduledTicksLocked[T comparable](ticks *scheduledTickQueue[T]) []scheduledTick[T] {
	r.activeChunksMu.RLock()
	due := ticks.advanceChunks(r.activeChunks, scheduledTickDrainLimit)
	r.activeChunksMu.RUnlock()

	return due
}

func scheduledTickBefore[T comparable](first, second scheduledTick[T]) bool {
	if first.due != second.due {
		return first.due < second.due
	}

	if first.priority != second.priority {
		return first.priority < second.priority
	}

	return first.suborder < second.suborder
}

func scheduledTickReadyBefore[T comparable](first, second scheduledTick[T]) bool {
	if first.priority != second.priority {
		return first.priority < second.priority
	}

	return first.suborder < second.suborder
}
