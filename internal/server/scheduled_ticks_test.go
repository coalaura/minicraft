package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	scheduledTickInactiveChunkCount = 128
	scheduledTicksPerInactiveChunk  = 64
	scheduledTickFutureCount        = 10_000
	scheduledTickBenchmarkFuture    = 100_000
)

func TestScheduledFluidTicksPauseAndResumeExactly(t *testing.T) {
	position := game.BlockPosition{X: 32, Y: 70, Z: -1}

	ticks := scheduledFluidTicks{}

	ticks.schedule(position, game.FluidStateTypeWater, 3)

	for range 20 {
		due := ticks.advance(func(LoadedChunk) bool {
			return false
		})

		if len(due) != 0 {
			t.Fatalf("inactive ticks = %v, want none", due)
		}
	}

	for range 2 {
		due := ticks.advance(func(LoadedChunk) bool {
			return true
		})

		if len(due) != 0 {
			t.Fatalf("resumed ticks before delay elapsed = %v, want none", due)
		}
	}

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 1 || due[0].key.position != position || due[0].key.typeID != game.FluidStateTypeWater {
		t.Fatalf("due ticks = %v, want one fluid tick at %v", due, position)
	}
}

func TestScheduledFluidTicksIgnoreQueuedDuplicates(t *testing.T) {
	position := game.BlockPosition{X: 32, Y: 70}
	ticks := scheduledFluidTicks{}

	ticks.schedule(position, game.FluidStateTypeWater, 5)
	ticks.schedule(position, game.FluidStateTypeWater, 1)

	for range 4 {
		due := ticks.advance(func(LoadedChunk) bool {
			return true
		})

		if len(due) != 0 {
			t.Fatalf("duplicate replaced queued tick: %v", due)
		}
	}

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 1 {
		t.Fatalf("due ticks = %v, want original queued tick", due)
	}

	ticks.schedule(position, game.FluidStateTypeWater, 1)

	due = ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 1 {
		t.Fatalf("self-rescheduled tick = %v, want one tick", due)
	}
}

func TestScheduledTickQueueOrdersDuePriorityAndSuborder(t *testing.T) {
	first := game.BlockPosition{X: 1}
	second := game.BlockPosition{X: 2}
	third := game.BlockPosition{X: 3}

	ticks := scheduledTickQueue[string]{}

	ticks.schedule(first, "first", 2, 1)
	ticks.schedule(second, "second", 2, 0)
	ticks.schedule(third, "third", 2, 1)

	ticks.advance(func(LoadedChunk) bool {
		return true
	})

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 3 {
		t.Fatalf("due ticks = %v, want 3", due)
	}

	if due[0].key.position != second || due[1].key.position != first || due[2].key.position != third {
		t.Fatalf("due order = %v, want priority then insertion order", due)
	}
}

func TestScheduledTickQueueOrdersDivergentChunkClocksByPriority(t *testing.T) {
	highPriorityPosition := game.BlockPosition{X: 1}
	normalPriorityPosition := game.BlockPosition{X: 32}

	highPriorityChunk := blockLoadedChunk(highPriorityPosition)

	ticks := scheduledTickQueue[string]{}

	ticks.schedule(highPriorityPosition, "high", 10, -1)

	for range 9 {
		ticks.advance(func(chunk LoadedChunk) bool {
			return chunk == highPriorityChunk
		})
	}

	ticks.schedule(normalPriorityPosition, "normal", 1)

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 2 {
		t.Fatalf("due ticks = %v, want 2", due)
	}

	if due[0].key.position != highPriorityPosition || due[1].key.position != normalPriorityPosition {
		t.Fatalf("divergent-clock due order = %v, want priority order", due)
	}
}

func TestScheduledBlockTicksUseBlockTypeIdentity(t *testing.T) {
	position := game.BlockPosition{X: 8, Y: 70, Z: 8}

	stoneDefinition, _ := game.StoneButton.Definition()
	oakDefinition, _ := game.OakButton.Definition()

	ticks := scheduledBlockTicks{}

	ticks.schedule(position, stoneDefinition.ID, 2)
	ticks.schedule(position, stoneDefinition.ID, 1)
	ticks.schedule(position, oakDefinition.ID, 2)

	ticks.advance(func(LoadedChunk) bool {
		return true
	})

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 2 {
		t.Fatalf("due ticks = %v, want one tick for each block type", due)
	}

	if due[0].key.typeID != stoneDefinition.ID || due[1].key.typeID != oakDefinition.ID {
		t.Fatalf("due block types = %v, want stone then oak", due)
	}
}

func TestScheduledBlockAndFluidTicksHaveIndependentDomains(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	definition, _ := game.StoneButton.Definition()

	blockTicks := scheduledBlockTicks{}
	fluidTicks := scheduledFluidTicks{}

	blockTicks.schedule(position, definition.ID, 1)
	fluidTicks.schedule(position, game.FluidStateTypeWater, 1)

	blockDue := blockTicks.advance(func(LoadedChunk) bool {
		return true
	})

	fluidDue := fluidTicks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(blockDue) != 1 || len(fluidDue) != 1 {
		t.Fatalf("independent due ticks = blocks %v, fluids %v; want one each", blockDue, fluidDue)
	}
}

func TestScheduledFluidTicksUseIndependentChunkClocks(t *testing.T) {
	first := game.BlockPosition{X: 0, Y: 70}
	second := game.BlockPosition{X: 32, Y: 70}

	firstChunk := blockLoadedChunk(first)

	ticks := scheduledFluidTicks{}

	ticks.schedule(first, game.FluidStateTypeWater, 2)
	ticks.schedule(second, game.FluidStateTypeWater, 2)

	for range 2 {
		ticks.advance(func(chunk LoadedChunk) bool {
			return chunk == firstChunk
		})
	}

	if ticks.len() != 1 {
		t.Fatalf("pending ticks after first chunk advances = %d, want 1", ticks.len())
	}

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 0 {
		t.Fatalf("second chunk caught up immediately: %v", due)
	}

	due = ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 1 || due[0].key.position != second {
		t.Fatalf("due ticks after second chunk delay = %v, want second chunk tick", due)
	}
}

func TestScheduledFluidTicksSurviveActiveChunkDestruction(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	position := game.BlockPosition{X: 32, Y: 70}

	chunk := blockLoadedChunk(position)

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunk})
	runtime.scheduleFluidTickLocked(position, game.FluidStateTypeWater, 1)
	runtime.setSessionActiveChunks(session, nil)
	runtime.tickScheduledFluidsLocked()

	if runtime.scheduledFluidTicks.len() != 1 {
		t.Fatalf("pending ticks after chunk destruction = %d, want 1", runtime.scheduledFluidTicks.len())
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunk})
	runtime.tickScheduledFluidsLocked()

	if runtime.scheduledFluidTicks.len() != 0 {
		t.Fatalf("pending ticks after reactivation = %d, want 0", runtime.scheduledFluidTicks.len())
	}
}

func TestScheduledTickQueueChecksInactiveContainersNotPositions(t *testing.T) {
	ticks := scheduledTickQueue[int]{}

	for chunkX := range int32(scheduledTickInactiveChunkCount) {
		for typeID := range scheduledTicksPerInactiveChunk {
			position := game.BlockPosition{X: chunkX * game.ChunkWidth, Y: int32(typeID)}

			ticks.schedule(position, typeID, 20)
		}
	}

	checks := 0

	due := ticks.advance(func(LoadedChunk) bool {
		checks++

		return false
	})

	if len(due) != 0 {
		t.Fatalf("inactive due ticks = %d, want 0", len(due))
	}

	if checks != scheduledTickInactiveChunkCount {
		t.Fatalf("inactive membership checks = %d, want one per chunk (%d)", checks, scheduledTickInactiveChunkCount)
	}

	wantPending := scheduledTickInactiveChunkCount * scheduledTicksPerInactiveChunk
	if ticks.len() != wantPending {
		t.Fatalf("pending inactive ticks = %d, want %d", ticks.len(), wantPending)
	}
}

func TestScheduledTickQueueLeavesFutureHeapEntriesUninspected(t *testing.T) {
	ticks := scheduledTickQueue[int]{}

	for typeID := range scheduledTickFutureCount {
		position := game.BlockPosition{Y: int32(typeID)}

		ticks.schedule(position, typeID, 100)
	}

	duePosition := game.BlockPosition{X: 1}

	ticks.schedule(duePosition, scheduledTickFutureCount, 1)

	due := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(due) != 1 || due[0].key.position != duePosition {
		t.Fatalf("due ticks = %v, want only the early tick", due)
	}

	if ticks.len() != scheduledTickFutureCount {
		t.Fatalf("future pending ticks = %d, want %d", ticks.len(), scheduledTickFutureCount)
	}
}

func TestScheduledTickQueueCapsDrainAndRetainsOrder(t *testing.T) {
	ticks := scheduledTickQueue[int]{}
	total := scheduledTickDrainLimit + 257

	for typeID := range total {
		position := game.BlockPosition{Y: int32(typeID)}

		ticks.schedule(position, typeID, 1)
	}

	first := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(first) != scheduledTickDrainLimit {
		t.Fatalf("first drain = %d, want cap %d", len(first), scheduledTickDrainLimit)
	}

	for index, tick := range first {
		if tick.key.typeID != index {
			t.Fatalf("first drain tick %d type = %d, want insertion order", index, tick.key.typeID)
		}
	}

	if ticks.len() != total-scheduledTickDrainLimit {
		t.Fatalf("leftover ticks = %d, want %d", ticks.len(), total-scheduledTickDrainLimit)
	}

	second := ticks.advance(func(LoadedChunk) bool {
		return true
	})

	if len(second) != total-scheduledTickDrainLimit {
		t.Fatalf("second drain = %d, want %d", len(second), total-scheduledTickDrainLimit)
	}

	for index, tick := range second {
		want := scheduledTickDrainLimit + index
		if tick.key.typeID != want {
			t.Fatalf("leftover tick %d type = %d, want %d", index, tick.key.typeID, want)
		}
	}

	if ticks.len() != 0 || ticks.chunkCount() != 0 {
		t.Fatalf("drained queue retained %d ticks in %d chunks", ticks.len(), ticks.chunkCount())
	}
}

func BenchmarkScheduledTickQueueFuturePopulation(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		b.StopTimer()

		ticks := scheduledTickQueue[int]{}

		for typeID := range scheduledTickBenchmarkFuture {
			position := game.BlockPosition{Y: int32(typeID)}

			ticks.schedule(position, typeID, 1_000_000)
		}

		ticks.schedule(game.BlockPosition{X: 1}, scheduledTickBenchmarkFuture, 1)

		b.StartTimer()

		due := ticks.advance(func(LoadedChunk) bool {
			return true
		})

		if len(due) != 1 {
			b.Fatalf("due ticks = %d, want 1", len(due))
		}
	}
}
