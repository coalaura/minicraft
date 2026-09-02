package server

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	fluidStressTicks             = 220
	fluidStressFingerprintOffset = uint64(14_695_981_039_346_656_037)
	fluidStressFingerprintPrime  = uint64(1_099_511_628_211)
)

type fluidStressGenerator struct{}

type fluidStressLightCounter struct {
	mu      sync.Mutex
	builds  map[LoadedChunk]int
	builder chunkLightBuilder
}

type fluidStressFixture struct {
	runtime      *Runtime
	lightCounter *fluidStressLightCounter
}

type fluidStressMetrics struct {
	totalScheduled       uint64
	pending              int
	maximumPending       int
	executed             int
	maximumExecuted      int
	authoritativeChanges int
	deliveries           int
	lightingInvalidated  int
	unbatchedLightBuilds int
	lightBuilds          int
	fluidBlocks          int
	fingerprint          uint64
}

type fluidStressBenchmark struct {
	name     string
	lighting game.LightingMode
}

func (fluidStressGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	source := game.BlockPosition{X: -7, Y: 73}
	if position == source {
		return game.Water
	}

	if position.Y == 63 && position.X >= -16 && position.X <= 47 && position.Z >= -24 && position.Z <= 23 {
		return game.Stone
	}

	ridgeFloor := position.Y == 72 && position.X >= -8 && position.X <= -1 && position.Z == 0
	ridgeSide := position.Y == 73 && position.X >= -8 && position.X <= -1 && (position.Z == -1 || position.Z == 1)
	ridgeRear := position.Y == 73 && position.X == -8 && position.Z == 0

	if ridgeFloor || ridgeSide || ridgeRear {
		return game.Stone
	}

	return game.Air
}

func (counter *fluidStressLightCounter) reset() {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	clear(counter.builds)
}

func (counter *fluidStressLightCounter) build(world *game.World, chunkX, chunkZ int32) (protocol.UpdateLight, error) {
	counter.mu.Lock()
	counter.builds[LoadedChunk{X: chunkX, Z: chunkZ}]++
	counter.mu.Unlock()

	if counter.builder != nil {
		return counter.builder(world, chunkX, chunkZ)
	}

	return protocol.UpdateLight{Position: protocol.ChunkPosition{X: chunkX, Z: chunkZ}}, nil
}

func (counter *fluidStressLightCounter) finish() int {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	for _, builds := range counter.builds {
		if builds != 1 {
			panic("fluid stress lighting chunk rebuilt within one batch")
		}
	}

	return len(counter.builds)
}

func newFluidStressFixture(lighting game.LightingMode, countOnlyLighting bool) *fluidStressFixture {
	world := game.NewOverworld(fluidStressGenerator{}, 42)

	world.SetLightingMode(lighting)

	runtime := NewRuntime(world)

	session := &Session{}

	chunks := make([]LoadedChunk, 0, 16)

	for chunkZ := int32(-2); chunkZ <= 1; chunkZ++ {
		for chunkX := int32(-1); chunkX <= 2; chunkX++ {
			chunks = append(chunks, LoadedChunk{X: chunkX, Z: chunkZ})
		}
	}

	runtime.setSessionActiveChunks(session, chunks)

	fixture := &fluidStressFixture{runtime: runtime}

	counter := &fluidStressLightCounter{builds: make(map[LoadedChunk]int)}

	if !countOnlyLighting {
		counter.builder = buildChunkLight
	}

	runtime.chunkLightBuilder = counter.build
	fixture.lightCounter = counter

	source := game.BlockPosition{X: -7, Y: 73}

	state := world.FluidAt(source)

	runtime.worldMutationMu.Lock()
	runtime.scheduleFluidTickLocked(source, state.StateType(), waterFluidDelay)
	runtime.worldMutationMu.Unlock()

	return fixture
}

func (fixture *fluidStressFixture) tick(metrics *fluidStressMetrics) {
	if fixture.lightCounter != nil {
		fixture.lightCounter.reset()
	}

	fixture.runtime.World.AdvanceTime()

	fixture.runtime.worldMutationMu.Lock()
	executed := fixture.runtime.tickScheduledFluidsLocked()

	deliveries := fixture.runtime.takeRuntimeBlockMutationsLocked()

	pending := fixture.runtime.scheduledFluidTicks.len()
	fixture.runtime.worldMutationMu.Unlock()

	metrics.executed += executed
	metrics.maximumExecuted = max(metrics.maximumExecuted, executed)
	metrics.maximumPending = max(metrics.maximumPending, pending)
	metrics.deliveries += len(deliveries)

	affected := make(map[LoadedChunk]struct{})

	for _, delivery := range deliveries {
		metrics.authoritativeChanges += len(delivery.result.Changes)
		deliveryAffected := make(map[LoadedChunk]struct{})

		for _, change := range delivery.delivery.lightingChanges {
			addFluidStressLightingChunks(affected, change.Position)
			addFluidStressLightingChunks(deliveryAffected, change.Position)
		}

		metrics.unbatchedLightBuilds += len(deliveryAffected)
	}

	metrics.lightingInvalidated += len(affected)

	fixture.runtime.completeRuntimeBlockMutations(deliveries)

	if fixture.lightCounter != nil {
		metrics.lightBuilds += fixture.lightCounter.finish()
	}
}

func (fixture *fluidStressFixture) finish(metrics *fluidStressMetrics) {
	metrics.totalScheduled = fixture.runtime.scheduledFluidTicks.nextSuborder
	metrics.pending = fixture.runtime.scheduledFluidTicks.len()

	fingerprint := fluidStressFingerprintOffset

	for blockY := int32(64); blockY <= 73; blockY++ {
		for blockZ := int32(-24); blockZ <= 23; blockZ++ {
			for blockX := int32(-16); blockX <= 47; blockX++ {
				position := game.BlockPosition{X: blockX, Y: blockY, Z: blockZ}

				block := fixture.runtime.World.BlockAt(position)
				if block.FluidState().Empty() {
					continue
				}

				metrics.fluidBlocks++

				fingerprint ^= uint64(uint32(blockX))
				fingerprint *= fluidStressFingerprintPrime
				fingerprint ^= uint64(uint32(blockY))
				fingerprint *= fluidStressFingerprintPrime
				fingerprint ^= uint64(uint32(blockZ))
				fingerprint *= fluidStressFingerprintPrime
				fingerprint ^= uint64(block)
				fingerprint *= fluidStressFingerprintPrime
			}
		}
	}

	metrics.fingerprint = fingerprint
}

func runFluidStressFixture(fixture *fluidStressFixture, ticks int) fluidStressMetrics {
	metrics := fluidStressMetrics{}

	for range ticks {
		fixture.tick(&metrics)
	}

	fixture.finish(&metrics)

	return metrics
}

func addFluidStressLightingChunks(chunks map[LoadedChunk]struct{}, position game.BlockPosition) {
	minimumX := blockChunkCoordinate(position.X - 14)
	maximumX := blockChunkCoordinate(position.X + 14)
	minimumZ := blockChunkCoordinate(position.Z - 14)
	maximumZ := blockChunkCoordinate(position.Z + 14)

	for chunkZ := minimumZ; chunkZ <= maximumZ; chunkZ++ {
		for chunkX := minimumX; chunkX <= maximumX; chunkX++ {
			chunks[LoadedChunk{X: chunkX, Z: chunkZ}] = struct{}{}
		}
	}
}

func TestFluidStressFixtureIsDeterministicAndBatchesLighting(t *testing.T) {
	first := runFluidStressFixture(newFluidStressFixture(game.LightingNormal, true), fluidStressTicks)
	second := runFluidStressFixture(newFluidStressFixture(game.LightingNormal, true), fluidStressTicks)

	if first != second {
		t.Fatalf("fluid stress metrics differ between deterministic runs: first %+v, second %+v", first, second)
	}

	if first.fluidBlocks < 128 {
		t.Fatalf("fluid stress spread reached %d blocks, want a broad lower sheet", first.fluidBlocks)
	}

	if first.executed == 0 || first.authoritativeChanges == 0 || first.deliveries == 0 {
		t.Fatalf("fluid stress did not exercise scheduled runtime mutations: %+v", first)
	}

	if first.lightBuilds != first.lightingInvalidated {
		t.Fatalf("batched light builds = %d, invalidated chunks = %d", first.lightBuilds, first.lightingInvalidated)
	}

	if first.lightBuilds >= first.unbatchedLightBuilds {
		t.Fatalf("batched light builds = %d, want fewer than unbatched %d", first.lightBuilds, first.unbatchedLightBuilds)
	}

	t.Logf("deterministic fluid stress metrics: %+v", first)
}

func BenchmarkFluidStress(b *testing.B) {
	benchmarks := []fluidStressBenchmark{
		{name: "Fullbright", lighting: game.LightingFullbright},
		{name: "NormalLighting", lighting: game.LightingNormal},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()

			var metrics fluidStressMetrics

			for b.Loop() {
				b.StopTimer()
				fixture := newFluidStressFixture(benchmark.lighting, false)
				b.StartTimer()

				metrics = runFluidStressFixture(fixture, fluidStressTicks)
			}

			b.ReportMetric(float64(metrics.executed), "fluid-ticks/run")
			b.ReportMetric(float64(metrics.totalScheduled), "scheduled/run")
			b.ReportMetric(float64(metrics.authoritativeChanges), "block-changes/run")
			b.ReportMetric(float64(metrics.deliveries), "deliveries/run")
			b.ReportMetric(float64(metrics.lightingInvalidated), "invalidated-chunks/run")
			b.ReportMetric(float64(metrics.lightBuilds), "chunk-light-builds/run")
			b.ReportMetric(float64(metrics.unbatchedLightBuilds), "unbatched-light-builds/run")
			b.ReportMetric(float64(metrics.fluidBlocks), "fluid-blocks/run")
			b.ReportMetric(float64(metrics.pending), "pending/run")
			b.ReportMetric(float64(metrics.maximumPending), "max-pending/run")
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*fluidStressTicks), "ns/runtime-tick")
		})
	}
}

func BenchmarkRuntimeLightingBatchOverlappingMutations(b *testing.B) {
	for b.Loop() {
		b.StopTimer()

		runtime := NewRuntime(normalLightingTestWorld())
		var builds atomic.Int64

		runtime.chunkLightBuilder = func(world *game.World, chunkX, chunkZ int32) (protocol.UpdateLight, error) {
			builds.Add(1)

			return buildChunkLight(world, chunkX, chunkZ)
		}

		runtime.worldMutationMu.Lock()

		for offset := range int32(48) {
			change := game.BlockChange{Position: game.BlockPosition{X: 1, Y: 40 + offset, Z: 1}, Replacement: game.Torch}

			result, delivery, err := runtime.mutateBlocksLocked(nil, blockMutationLiteral, []game.BlockChange{change}, 1, true, false, true, false)
			if err != nil || !result.Changed {
				b.Fatalf("prepare runtime lighting mutation %d: result %+v, err %v", offset, result, err)
			}

			runtime.runtimeBlockMutations = append(runtime.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
		}

		deliveries := runtime.takeRuntimeBlockMutationsLocked()
		runtime.worldMutationMu.Unlock()

		b.StartTimer()
		runtime.completeRuntimeBlockMutations(deliveries)
		b.StopTimer()

		b.ReportMetric(float64(builds.Load()), "chunk-light-builds/run")
		b.StartTimer()
	}
}
