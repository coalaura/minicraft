package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type randomTickTestGenerator struct {
	block game.Block
}

type randomTickBoundedGenerator struct {
	base    randomTickTestGenerator
	minY    int32
	maxY    int32
	present bool
	hints   map[int32]randomTickSectionHint
}

type randomTickSectionHint struct {
	mayTick    bool
	definitive bool
}

type randomTickSectionTestGenerator struct {
	bounded      randomTickBoundedGenerator
	uniformBlock game.Block
	blocks       [game.SectionVolume]game.Block
	uniform      bool
	sectionCalls int
}

type countingRandomTickGenerator struct {
	block game.Block
	calls int
}

type generatedSectionInspectionTestCase struct {
	name         string
	uniform      bool
	uniformBlock game.Block
	blockIndex   int
}

type supportedCropRandomTickTestCase struct {
	name string
	crop game.Block
	age  int
}

func (generator randomTickTestGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return generator.block
}

func (generator randomTickBoundedGenerator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return generator.minY, generator.maxY, generator.present
}

func (generator randomTickBoundedGenerator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	return generator.base.BlockAt(seed, position)
}

func (generator randomTickBoundedGenerator) RandomTickSection(_ int64, _ game.ChunkPosition, sectionMinY int32) (bool, bool) {
	hint := generator.hints[sectionMinY]
	return hint.mayTick, hint.definitive
}

func (generator *randomTickSectionTestGenerator) GenerateSection(_ int64, _ game.ChunkPosition, _ int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	generator.sectionCalls++
	copy(blocks[:], generator.blocks[:])

	return generator.uniformBlock, generator.uniform
}

func (generator *randomTickSectionTestGenerator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	return generator.bounded.BlockAt(seed, position)
}

func (generator *randomTickSectionTestGenerator) GenerationBounds(seed int64, chunk game.ChunkPosition) (int32, int32, bool) {
	return generator.bounded.GenerationBounds(seed, chunk)
}

func (generator *randomTickSectionTestGenerator) RandomTickSection(seed int64, chunk game.ChunkPosition, sectionMinY int32) (bool, bool) {
	return generator.bounded.RandomTickSection(seed, chunk, sectionMinY)
}

func (generator *countingRandomTickGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	generator.calls++

	return generator.block
}

func TestRandomTickPositionSequence(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	runtime.SetRandomTickPositionState(12345)

	chunk := LoadedChunk{X: -2, Z: 3}
	expected := []game.BlockPosition{
		{X: -30, Y: -5, Z: 49},
		{X: -17, Y: -1, Z: 63},
		{X: -27, Y: -7, Z: 60},
		{X: -24, Y: -7, Z: 50},
		{X: -32, Y: -9, Z: 51},
		{X: -23, Y: -14, Z: 55},
	}

	for index, want := range expected {
		got := runtime.nextRandomTickPosition(chunk, -16)
		if got != want {
			t.Fatalf("sample %d = %+v, want %+v", index, got, want)
		}
	}
}

func TestRandomTicksUseDefaultSamplingOnlyForActiveChunks(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}
	secondSession := &Session{}

	position := LoadedChunk{X: 1, Z: -1}

	runtime.setSessionActiveChunks(session, []LoadedChunk{position})
	runtime.setSessionActiveChunks(secondSession, []LoadedChunk{position})

	chunk, active := runtime.ActiveChunk(position)
	if !active {
		t.Fatal("chunk is not active")
	}

	chunk.markRandomTickSection(64)

	samples := make([]game.BlockPosition, 0, defaultRandomTickSpeed)

	for range defaultRandomTickSpeed {
		sample := runtime.nextRandomTickPosition(position, 64)

		samples = append(samples, sample)
	}

	runtime.SetRandomTickPositionState(0)

	for _, sample := range samples {
		runtime.World.SetBlock(sample, game.Wheat)
	}

	calls := 0

	runtime.SetRandomTickRandom(func(_ int) int {
		calls++

		return 0
	})

	runtime.tickRandomBlocksLocked()

	if calls != defaultRandomTickSpeed {
		t.Fatalf("random samples = %d, want %d", calls, defaultRandomTickSpeed)
	}

	runtime.setSessionActiveChunks(secondSession, nil)
	runtime.setSessionActiveChunks(session, nil)

	runtime.tickRandomBlocksLocked()

	if calls != defaultRandomTickSpeed {
		t.Fatalf("inactive chunk sampled %d times, want %d", calls, defaultRandomTickSpeed)
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{position})

	chunk, active = runtime.ActiveChunk(position)
	if !active {
		t.Fatal("reactivated chunk is not active")
	}

	chunk.markRandomTickSection(64)

	runtime.RandomTickSpeed = 0

	runtime.tickRandomBlocksLocked()

	if calls != defaultRandomTickSpeed {
		t.Fatalf("zero speed sampled %d times, want %d", calls, defaultRandomTickSpeed)
	}

	runtime.RandomTickSpeed = defaultRandomTickSpeed

	runtime.SetRandomTickRandom(nil)

	runtime.tickRandomBlocksLocked()

	if calls != defaultRandomTickSpeed {
		t.Fatalf("nil sampler sampled %d times, want %d", calls, defaultRandomTickSpeed)
	}
}

func TestRandomTickSnapshotExcludesNewSectionsUntilNextTick(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	chunkPosition := LoadedChunk{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunkPosition})

	chunk, active := runtime.ActiveChunk(chunkPosition)
	if !active {
		t.Fatal("chunk is not active")
	}

	chunk.randomSections = make(map[int32]struct{})

	chunk.markRandomTickSection(0)

	runtime.RandomTickSpeed = 1

	runtime.SetRandomTickPositionState(0)

	sampled := runtime.nextRandomTickPosition(chunkPosition, 0)

	runtime.SetRandomTickPositionState(0)

	runtime.World.SetBlock(sampled, game.Wheat)

	calls := 0

	runtime.SetRandomTickRandom(func(_ int) int {
		calls++

		change := game.BlockChange{Position: game.BlockPosition{Y: 20}, Replacement: game.Wheat}

		result, _, err := runtime.mutateBlocksLocked(nil, BlockMutationPlace, []game.BlockChange{change}, 1, true, false, true, false)
		if err != nil || !result.Changed {
			t.Fatalf("promote section during random tick: %+v, %v", result, err)
		}

		return 1
	})

	runtime.tickRandomBlocksLocked()

	if calls != 1 {
		t.Fatalf("gameplay random calls = %d, want 1", calls)
	}

	if runtime.randomTickPositionState != randomTickPositionAdd {
		t.Fatalf("position state = %d, want one advancement %d", runtime.randomTickPositionState, randomTickPositionAdd)
	}

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), []int32{0, 16})
}

func TestGeneratedBaselineFarmlandRandomTicksWithoutOverride(t *testing.T) {
	baseline := randomTickTestGenerator{block: withBlockProperties(game.Farmland, game.BlockPropertyValue{Name: "moisture", Value: "1"})}

	generator := randomTickBoundedGenerator{
		base:    baseline,
		minY:    0,
		maxY:    15,
		present: true,
	}

	world := game.NewOverworld(generator)

	runtime := NewRuntime(world)

	session := &Session{}

	chunkPosition := LoadedChunk{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunkPosition})

	runtime.RandomTickSpeed = 1

	runtime.SetRandomTickPositionState(0)

	sampled := runtime.nextRandomTickPosition(chunkPosition, 0)

	runtime.SetRandomTickPositionState(0)

	runtime.tickRandomBlocksLocked()

	moisture := blockPropertyInt(world.BlockAt(sampled), "moisture")

	if moisture != 0 {
		t.Fatalf("generated farmland moisture = %d, want 0", moisture)
	}
}

func TestRandomTickSectionsSnapshotAndPromotion(t *testing.T) {
	runtime := NewRuntime(game.NewOverworld(nil))

	session := &Session{}

	activePosition := LoadedChunk{X: 1, Z: 0}

	runtime.setSessionActiveChunks(session, []LoadedChunk{activePosition})

	chunk, active := runtime.ActiveChunk(activePosition)
	if !active {
		t.Fatal("chunk is not active")
	}

	chunk.markRandomTickSection(32)
	chunk.markRandomTickSection(-16)

	snapshot := chunk.snapshotRandomTickSections()

	chunk.markRandomTickSection(48)

	expectedSnapshot := []int32{-16, 32}

	assertRandomTickSections(t, snapshot, expectedSnapshot)

	changes := []game.BlockChange{
		{Position: game.BlockPosition{X: 16, Y: 65}, Replacement: game.Farmland},
		{Position: game.BlockPosition{X: 16, Y: 79}, Replacement: game.Wheat},
		{Position: game.BlockPosition{X: 16, Y: 79}, Replacement: game.Beetroots},
		{Position: game.BlockPosition{X: 32, Y: 96}, Replacement: game.Farmland},
		{Position: game.BlockPosition{X: 16, Y: 96}, Replacement: game.Air},
	}

	runtime.promoteRandomTickSections(changes)

	expectedSections := []int32{-16, 32, 48, 64}

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), expectedSections)
}

func TestRandomTickSectionInitializationUsesBoundsHintsAndOverrides(t *testing.T) {
	baseGenerator := randomTickTestGenerator{block: game.Stone}

	generator := randomTickBoundedGenerator{
		base:    baseGenerator,
		minY:    -80,
		maxY:    -33,
		present: true,
		hints: map[int32]randomTickSectionHint{
			-64: {mayTick: false, definitive: true},
			-48: {mayTick: false, definitive: false},
		},
	}

	world := game.NewOverworld(generator)

	override := game.BlockPosition{X: 0, Y: 80, Z: 0}

	world.SetBlock(override, game.Farmland)

	runtime := NewRuntime(world)

	chunk := runtime.newActiveChunk(LoadedChunk{})

	expected := []int32{80}

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), expected)
}

func TestRandomTickSectionInitializationInspectsGeneratedContents(t *testing.T) {
	tests := []generatedSectionInspectionTestCase{
		{name: "uniform", uniform: true, uniformBlock: game.Farmland},
		{name: "non-uniform", blockIndex: 37},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			generator := &randomTickSectionTestGenerator{
				bounded: randomTickBoundedGenerator{
					base:    randomTickTestGenerator{block: game.Stone},
					minY:    0,
					maxY:    15,
					present: true,
				},
				uniformBlock: test.uniformBlock,
				uniform:      test.uniform,
			}

			for index := range generator.blocks {
				generator.blocks[index] = game.Stone
			}

			if !test.uniform {
				generator.blocks[test.blockIndex] = game.Farmland
			}

			runtime := NewRuntime(game.NewOverworld(generator))

			chunk := runtime.newActiveChunk(LoadedChunk{})

			assertRandomTickSections(t, chunk.snapshotRandomTickSections(), []int32{0})

			if generator.sectionCalls != 1 {
				t.Fatalf("section generation calls = %d, want 1", generator.sectionCalls)
			}
		})
	}
}

func TestDefinitiveRandomTickHintAvoidsSectionGeneration(t *testing.T) {
	generator := &randomTickSectionTestGenerator{
		bounded: randomTickBoundedGenerator{
			base:    randomTickTestGenerator{block: game.Stone},
			minY:    0,
			maxY:    31,
			present: true,
			hints: map[int32]randomTickSectionHint{
				0:  {mayTick: false, definitive: true},
				16: {mayTick: true, definitive: true},
			},
		},
	}

	runtime := NewRuntime(game.NewOverworld(generator))

	chunk := runtime.newActiveChunk(LoadedChunk{})

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), []int32{16})

	if generator.sectionCalls != 0 {
		t.Fatalf("section generation calls = %d, want 0", generator.sectionCalls)
	}
}

func TestStaticGeneratedWorldDoesNotSampleEmptySections(t *testing.T) {
	generator := &countingRandomTickGenerator{block: game.Stone}

	runtime := NewRuntime(game.NewOverworld(generator))

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	activationCalls := generator.calls
	if activationCalls == 0 {
		t.Fatal("activation did not inspect generated contents")
	}

	chunk, active := runtime.ActiveChunk(LoadedChunk{})
	if !active {
		t.Fatal("chunk is not active")
	}

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), nil)

	runtime.tickRandomBlocksLocked()

	if generator.calls != activationCalls {
		t.Fatalf("procedural calls after random tick = %d, want %d", generator.calls, activationCalls)
	}
}

func TestRandomTickSectionEligibilityRebuildsOnReactivation(t *testing.T) {
	generator := &randomTickSectionTestGenerator{
		bounded: randomTickBoundedGenerator{
			base:    randomTickTestGenerator{block: game.Stone},
			minY:    0,
			maxY:    15,
			present: true,
		},
		uniformBlock: game.Stone,
		uniform:      true,
	}

	world := game.NewOverworld(generator)

	runtime := NewRuntime(world)

	session := &Session{}

	chunkPosition := LoadedChunk{}

	override := game.BlockPosition{Y: 32}

	world.SetBlock(override, game.Farmland)

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunkPosition})

	chunk, _ := runtime.ActiveChunk(chunkPosition)

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), []int32{32})

	runtime.setSessionActiveChunks(session, nil)

	world.SetBlock(override, game.Stone)

	generator.uniformBlock = game.Farmland
	generator.bounded.base.block = game.Farmland

	runtime.setSessionActiveChunks(session, []LoadedChunk{chunkPosition})

	chunk, _ = runtime.ActiveChunk(chunkPosition)

	assertRandomTickSections(t, chunk.snapshotRandomTickSections(), []int32{0})
}

func TestFarmlandRandomTickHydrationAndDecay(t *testing.T) {
	position := game.BlockPosition{X: 10, Y: 70, Z: -4}

	world := game.NewOverworld(nil)

	runtime := NewRuntime(world)

	waterloggedFence := withBlockProperties(game.OakFence, game.BlockPropertyValue{Name: "waterlogged", Value: "true"})

	world.SetBlock(game.BlockPosition{X: position.X + 4, Y: position.Y + 1, Z: position.Z}, waterloggedFence)

	if !runtime.farmlandHasWater(position) {
		t.Fatal("waterlogged block at hydration boundary was not detected")
	}

	world.SetBlock(game.BlockPosition{X: position.X + 4, Y: position.Y + 1, Z: position.Z}, game.Air)
	world.SetBlock(game.BlockPosition{X: position.X + 5, Y: position.Y, Z: position.Z}, game.Water)

	if runtime.farmlandHasWater(position) {
		t.Fatal("water outside hydration boundary was detected")
	}

	hydrated := withBlockProperties(game.Farmland, game.BlockPropertyValue{Name: "moisture", Value: "2"})

	world.SetBlock(position, hydrated)
	world.SetBlock(game.BlockPosition{X: position.X, Y: position.Y, Z: position.Z + 4}, game.Water)

	runtime.randomTickFarmlandLocked(position, hydrated)

	moisture := blockPropertyInt(world.BlockAt(position), "moisture")

	if moisture != farmlandMaximumMoisture {
		t.Fatalf("hydrated moisture = %d, want %d", moisture, farmlandMaximumMoisture)
	}

	world.SetBlock(game.BlockPosition{X: position.X, Y: position.Y, Z: position.Z + 4}, game.Air)

	dry := withBlockProperties(game.Farmland, game.BlockPropertyValue{Name: "moisture", Value: "2"})

	world.SetBlock(position, dry)

	runtime.randomTickFarmlandLocked(position, dry)

	moisture = blockPropertyInt(world.BlockAt(position), "moisture")

	if moisture != 1 {
		t.Fatalf("dry moisture = %d, want 1", moisture)
	}

	dry = withBlockProperties(game.Farmland, game.BlockPropertyValue{Name: "moisture", Value: "0"})

	world.SetBlock(position, dry)

	above := game.BlockPosition{X: position.X, Y: position.Y + 1, Z: position.Z}

	world.SetBlock(above, game.Wheat)

	runtime.randomTickFarmlandLocked(position, dry)

	if !sameBlockType(world.BlockAt(position), game.Farmland) {
		t.Fatal("crop did not maintain dry farmland")
	}

	world.SetBlock(above, game.Air)

	runtime.randomTickFarmlandLocked(position, dry)

	if world.BlockAt(position) != game.Dirt {
		t.Fatalf("unmaintained dry farmland = %v, want dirt", world.BlockAt(position))
	}
}

func TestFarmlandSurvivalUsesScheduledTick(t *testing.T) {
	position := game.BlockPosition{Y: 70}
	above := game.BlockPosition{Y: 71}

	world := game.NewOverworld(nil)

	runtime := NewRuntime(world)

	session := &Session{}

	runtime.setSessionActiveChunks(session, []LoadedChunk{blockLoadedChunk(position)})

	world.SetBlock(position, game.Farmland)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: above, Replacement: game.Stone}})
	if err != nil || !result.Changed {
		t.Fatalf("place farmland cover: %+v, %v", result, err)
	}

	if !sameBlockType(world.BlockAt(position), game.Farmland) {
		t.Fatal("farmland changed before its scheduled tick")
	}

	runtime.worldMutationMu.Lock()
	runtime.tickScheduledBlocksLocked()
	runtime.worldMutationMu.Unlock()

	if world.BlockAt(position) != game.Dirt {
		t.Fatalf("covered farmland = %d after scheduled tick, want dirt", world.BlockAt(position))
	}

	world.SetBlocks([]game.BlockChange{
		{Position: position, Replacement: game.Farmland},
		{Position: above, Replacement: game.OakFenceGate},
	})

	if !farmlandSurvivesBelow(game.OakFenceGate) {
		t.Fatal("fence gate did not preserve farmland")
	}

	bottomSlab := withBlockProperties(game.StoneSlab, game.BlockPropertyValue{Name: "type", Value: "bottom"})
	if farmlandSurvivesBelow(bottomSlab) {
		t.Fatal("solid bottom slab preserved farmland")
	}
}

func TestCropGrowthSpeedAndRandomTickGuards(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	crop := game.Wheat

	blocks := make(map[game.BlockPosition]game.Block)

	blockAt := func(position game.BlockPosition) game.Block {
		return blocks[position]
	}

	blocks[game.BlockPosition{Y: 69}] = withBlockProperties(game.Farmland, game.BlockPropertyValue{Name: "moisture", Value: "7"})

	speed := cropGrowthSpeed(blockAt, position, crop)
	if speed != 4 {
		t.Fatalf("hydrated center crop speed = %v, want 4", speed)
	}

	blocks[game.BlockPosition{X: 1, Y: 70}] = crop
	blocks[game.BlockPosition{Z: 1, Y: 70}] = crop

	speed = cropGrowthSpeed(blockAt, position, crop)
	if speed != 2 {
		t.Fatalf("row crop speed = %v, want 2", speed)
	}

	world := game.NewOverworld(nil)

	world.Lighting = game.LightingFullbright

	runtime := NewRuntime(world)

	world.SetBlock(game.BlockPosition{Y: 69}, blocks[game.BlockPosition{Y: 69}])
	world.SetBlock(position, crop)

	bounds := make([]int, 0, 2)

	runtime.SetRandomTickRandom(func(bound int) int {
		bounds = append(bounds, bound)

		return 0
	})

	runtime.randomTickCropLocked(position, crop)

	age := blockPropertyInt(world.BlockAt(position), "age")

	if age != 1 {
		t.Fatalf("grown wheat age = %d, want 1", age)
	}

	if len(bounds) != 1 || bounds[0] != 7 {
		t.Fatalf("wheat random bounds = %v, want [7]", bounds)
	}

	beetroots := game.Beetroots

	world.SetBlock(position, beetroots)

	runtime.SetRandomTickRandom(func(bound int) int {
		bounds = append(bounds, bound)

		return 0
	})

	runtime.randomTickCropLocked(position, beetroots)

	age = blockPropertyInt(world.BlockAt(position), "age")

	if age != 0 {
		t.Fatalf("beetroot age = %d, want 0", age)
	}

	if len(bounds) != 2 || bounds[1] != 3 {
		t.Fatalf("beetroot random bounds = %v, want [7 3]", bounds)
	}

	darkWorld := game.NewOverworld(randomTickTestGenerator{block: game.Stone})

	darkWorld.Lighting = game.LightingNormal

	darkRuntime := NewRuntime(darkWorld)

	darkWorld.SetBlock(game.BlockPosition{Y: 69}, blocks[game.BlockPosition{Y: 69}])
	darkWorld.SetBlock(position, crop)

	darkCalls := 0

	darkRuntime.SetRandomTickRandom(func(_ int) int {
		darkCalls++

		return 0
	})

	darkRuntime.randomTickCropLocked(position, crop)

	age = blockPropertyInt(darkWorld.BlockAt(position), "age")

	if age != 0 {
		t.Fatalf("dark wheat age = %d, want 0", age)
	}

	if darkCalls != 0 {
		t.Fatalf("dark wheat random calls = %d, want 0", darkCalls)
	}
}

func TestSupportedCropRandomTickMetadataStopsAtMaximumAge(t *testing.T) {
	tests := []supportedCropRandomTickTestCase{
		{name: "wheat", crop: game.Wheat, age: 7},
		{name: "carrots", crop: game.Carrots, age: 7},
		{name: "potatoes", crop: game.Potatoes, age: 7},
		{name: "beetroots", crop: game.Beetroots, age: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			immature := withBlockProperties(test.crop, game.BlockPropertyValue{Name: "age", Value: decimalBlockPropertyValue(test.age - 1)})
			mature := withBlockProperties(test.crop, game.BlockPropertyValue{Name: "age", Value: decimalBlockPropertyValue(test.age)})

			if !immature.RandomlyTicks() || mature.RandomlyTicks() {
				t.Fatalf("random ticking = immature %t mature %t, want true false", immature.RandomlyTicks(), mature.RandomlyTicks())
			}
		})
	}
}

func assertRandomTickSections(t *testing.T, actual, expected []int32) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("random tick sections = %v, want %v", actual, expected)
	}

	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("random tick sections = %v, want %v", actual, expected)
		}
	}
}
