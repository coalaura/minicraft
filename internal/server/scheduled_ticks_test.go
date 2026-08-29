package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestScheduledBlockTicksPauseAndResumeExactly(t *testing.T) {
	position := game.BlockPosition{X: 32, Y: 70, Z: -1}

	ticks := scheduledBlockTicks{}

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

func TestScheduledBlockTicksIgnoreQueuedDuplicates(t *testing.T) {
	position := game.BlockPosition{X: 32, Y: 70}
	ticks := scheduledBlockTicks{}

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

func TestScheduledBlockTicksUseIndependentChunkClocks(t *testing.T) {
	first := game.BlockPosition{X: 0, Y: 70}
	second := game.BlockPosition{X: 32, Y: 70}

	firstChunk := blockLoadedChunk(first)

	ticks := scheduledBlockTicks{}

	ticks.schedule(first, game.FluidStateTypeWater, 2)
	ticks.schedule(second, game.FluidStateTypeWater, 2)

	for range 2 {
		ticks.advance(func(chunk LoadedChunk) bool {
			return chunk == firstChunk
		})
	}

	if len(ticks.pending) != 1 {
		t.Fatalf("pending ticks after first chunk advances = %d, want 1", len(ticks.pending))
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

func TestScheduledBlockTicksSurviveActiveChunkDestruction(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	position := game.BlockPosition{X: 32, Y: 70}

	chunk := blockLoadedChunk(position)

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunk})
	runtime.scheduleBlockTickLocked(position, game.FluidStateTypeWater, 1)
	runtime.setSessionActiveChunks(session, nil)
	runtime.tickScheduledBlocksLocked()

	if len(runtime.scheduledBlockTicks.pending) != 1 {
		t.Fatalf("pending ticks after chunk destruction = %d, want 1", len(runtime.scheduledBlockTicks.pending))
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunk})
	runtime.tickScheduledBlocksLocked()

	if len(runtime.scheduledBlockTicks.pending) != 0 {
		t.Fatalf("pending ticks after reactivation = %d, want 0", len(runtime.scheduledBlockTicks.pending))
	}
}
