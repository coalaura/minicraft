package game

import (
	"maps"
	"sync"
	"sync/atomic"
)

type LightingMode uint8

const (
	LightingFullbright LightingMode = iota
	LightingNormal
)

type TimeState struct {
	Age      int64
	DayTime  int64
	DayCycle bool
}

type LocalBlockPosition struct {
	X int32
	Y int32
	Z int32
}

type World struct {
	Name          string
	DimensionType string
	Seed          int64
	Generator     Generator

	Spawn    Position
	SeaLevel int32
	Lighting LightingMode

	age      atomic.Int64
	dayTime  atomic.Int64
	dayCycle atomic.Bool

	overrideMx    sync.RWMutex
	overrides     map[ChunkPosition]map[LocalBlockPosition]Block
	blockEntities map[ChunkPosition]map[LocalBlockPosition]blockEntityOverride
}

type ChunkOverrides map[LocalBlockPosition]Block

type blockEntityOverride struct {
	entity            BlockEntity
	suppressed        bool
	generatedIdentity bool
}

type BlockChange struct {
	Position    BlockPosition
	Replacement Block
}

type preparedBlockChange struct {
	change                 BlockChange
	generatedBlock         Block
	generatedEntityPresent bool
}

type SpawnGenerator interface {
	Spawn(seed int64) Position
}

type WorldMetadata struct {
	SeaLevel int32
}

type WorldMetadataGenerator interface {
	WorldMetadata(seed int64) WorldMetadata
}

func (w *World) BlockAt(position BlockPosition) Block {
	chunk, local := blockIndex(position)

	w.overrideMx.RLock()
	blocks := w.overrides[chunk]
	block, overridden := blocks[local]
	w.overrideMx.RUnlock()

	if overridden {
		return block
	}

	if w.Generator == nil {
		return Air
	}

	return w.Generator.BlockAt(w.Seed, position)
}

func (w *World) SetBlock(position BlockPosition, block Block) {
	w.SetBlocks([]BlockChange{{Position: position, Replacement: block}})
}

// CompareAndSetBlock changes a block only when it still has the expected state.
func (w *World) CompareAndSetBlock(position BlockPosition, expected, replacement Block) bool {
	generatedBlock := w.generatedBlockAt(position)
	generatedEntityPresent := false

	if blockEntityRemovalNeedsGeneratedEntity(expected, replacement) {
		generatedEntity, present := w.generatedBlockEntityAt(position)
		generatedEntityPresent = present && blockEntityMatchesBlock(generatedEntity, expected)
	}

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

	current := w.blockAtLocked(position, generatedBlock)
	if current != expected {
		return false
	}

	w.updateBlockEntityForBlockChange(position, current, replacement, generatedEntityPresent)
	w.setBlock(position, replacement, generatedBlock)

	return true
}

// SetBlocks applies a prevalidated group while holding the sparse override lock.
func (w *World) SetBlocks(changes []BlockChange) {
	prepared := make([]preparedBlockChange, len(changes))

	for index, change := range changes {
		prepared[index] = preparedBlockChange{change: change, generatedBlock: w.generatedBlockAt(change.Position)}
	}

	w.prepareBlockEntityRemovals(prepared)

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

	for _, preparedChange := range prepared {
		change := preparedChange.change
		current := w.blockAtLocked(change.Position, preparedChange.generatedBlock)

		w.updateBlockEntityForBlockChange(change.Position, current, change.Replacement, preparedChange.generatedEntityPresent)
		w.setBlock(change.Position, change.Replacement, preparedChange.generatedBlock)
	}
}

func (w *World) BlockEntityAt(position BlockPosition) (BlockEntity, bool) {
	chunk, local := blockIndex(position)

	w.overrideMx.RLock()
	override, overridden := w.blockEntities[chunk][local]
	w.overrideMx.RUnlock()

	if overridden {
		return resolvedBlockEntityOverride(override)
	}

	generated, present := w.generatedBlockEntityAt(position)

	// An override may have been committed while procedural generation ran.
	w.overrideMx.RLock()
	override, overridden = w.blockEntities[chunk][local]
	w.overrideMx.RUnlock()

	if overridden {
		return resolvedBlockEntityOverride(override)
	}

	if !present || !blockEntityMatchesBlock(generated, w.BlockAt(position)) {
		return BlockEntity{}, false
	}

	return generated, present
}

// SetBlockEntity stores authoritative content using copy-on-write semantics.
// It returns false when the hosting block does not support the entity type.
func (w *World) SetBlockEntity(position BlockPosition, entity BlockEntity) bool {
	chunk, local := blockIndex(position)

	generated, generatedPresent := w.generatedBlockEntityAt(position)
	generatedBlock := w.generatedBlockAt(position)

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

	if BlockEntityTypeForBlock(w.blockAtLocked(position, generatedBlock)) != entity.Type || entity.Type == BlockEntityTypeNone {
		return false
	}

	override, overridden := w.blockEntities[chunk][local]

	if overridden && override.generatedIdentity && generatedPresent && entity.Equal(generated) {
		w.clearBlockEntityOverride(chunk, local)

		return true
	}

	if !overridden && generatedPresent && entity.Equal(generated) {
		return true
	}

	generatedIdentity := !overridden && generatedPresent || overridden && override.generatedIdentity

	w.setBlockEntityOverride(chunk, local, blockEntityOverride{entity: entity.Clone(), generatedIdentity: generatedIdentity})

	return true
}

// SnapshotChunkBlockEntities resolves procedural entities and sparse
// overrides without scanning block positions.
func (w *World) SnapshotChunkBlockEntities(chunk ChunkPosition) ChunkBlockEntities {
	entities := w.generatedBlockEntities(chunk)

	w.overrideMx.RLock()
	blocks := maps.Clone(w.overrides[chunk])
	entityOverrides := maps.Clone(w.blockEntities[chunk])
	w.overrideMx.RUnlock()

	for position, override := range entityOverrides {
		if override.suppressed {
			delete(entities, position)

			continue
		}

		if entities == nil {
			entities = make(ChunkBlockEntities)
		}

		entities[position] = override.entity.Clone()
	}

	for position, entity := range entities {
		if position.X < 0 || position.X >= ChunkWidth || position.Z < 0 || position.Z >= ChunkWidth {
			delete(entities, position)

			continue
		}

		block, overridden := blocks[position]
		if !overridden {
			worldPosition := BlockPosition{X: chunk.X*ChunkWidth + position.X, Y: position.Y, Z: chunk.Z*ChunkWidth + position.Z}

			block = w.generatedBlockAt(worldPosition)
		}

		if !blockEntityMatchesBlock(entity, block) {
			delete(entities, position)
		}
	}

	if len(entities) == 0 {
		return nil
	}

	return entities
}

func (w *World) BlockEntityOverrideCount() int {
	w.overrideMx.RLock()
	defer w.overrideMx.RUnlock()

	count := 0

	for _, entities := range w.blockEntities {
		count += len(entities)
	}

	return count
}

func (w *World) ClearBlockOverride(position BlockPosition) {
	chunk, local := blockIndex(position)

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

	w.clearBlockOverride(chunk, local)
	w.clearBlockEntityOverride(chunk, local)
}

func (w *World) SetLightingMode(mode LightingMode) {
	w.Lighting = mode
}

func (w *World) SetTime(dayTime int64, dayCycle bool) {
	w.age.Store(0)
	w.dayTime.Store(dayTime)
	w.dayCycle.Store(dayCycle)
}

func (w *World) SetDayTime(dayTime int64) {
	w.dayTime.Store(dayTime)
}

func (w *World) Time() TimeState {
	return TimeState{
		Age:      w.age.Load(),
		DayTime:  w.dayTime.Load(),
		DayCycle: w.dayCycle.Load(),
	}
}

func (w *World) AdvanceTime() TimeState {
	w.age.Add(1)

	if w.dayCycle.Load() {
		w.dayTime.Add(1)
	}

	return w.Time()
}

// SnapshotChunkOverrides returns a point-in-time copy of the sparse overrides
// for chunk. Chunk generation can then release the world lock before doing work.
func (w *World) SnapshotChunkOverrides(chunk ChunkPosition) ChunkOverrides {
	w.overrideMx.RLock()
	defer w.overrideMx.RUnlock()

	blocks := w.overrides[chunk]
	if len(blocks) == 0 {
		return nil
	}

	snapshot := make(ChunkOverrides, len(blocks))

	maps.Copy(snapshot, blocks)

	return snapshot
}

func (w *World) generatedBlockAt(position BlockPosition) Block {
	if w.Generator == nil {
		return Air
	}

	return w.Generator.BlockAt(w.Seed, position)
}

func (w *World) blockAtLocked(position BlockPosition, generated Block) Block {
	chunk, local := blockIndex(position)

	blocks := w.overrides[chunk]

	block, overridden := blocks[local]
	if overridden {
		return block
	}

	return generated
}

func (w *World) prepareBlockEntityRemovals(prepared []preparedBlockChange) {
	currentBlocks := make([]Block, len(prepared))

	w.overrideMx.RLock()

	for index, preparedChange := range prepared {
		currentBlocks[index] = w.blockAtLocked(preparedChange.change.Position, preparedChange.generatedBlock)
	}

	w.overrideMx.RUnlock()

	pointGenerator, hasPointGenerator := w.Generator.(BlockEntityPointGenerator)
	chunkGenerator, hasChunkGenerator := w.Generator.(BlockEntityGenerator)

	generatedChunks := make(map[ChunkPosition]ChunkBlockEntities)

	for index := range prepared {
		change := prepared[index].change
		current := currentBlocks[index]

		if !blockEntityRemovalNeedsGeneratedEntity(current, change.Replacement) {
			continue
		}

		var (
			entity  BlockEntity
			present bool
		)

		if hasPointGenerator {
			entity, present = pointGenerator.GenerateBlockEntity(w.Seed, change.Position)
		} else if hasChunkGenerator {
			chunk, local := blockIndex(change.Position)

			entities, generated := generatedChunks[chunk]
			if !generated {
				entities = chunkGenerator.GenerateBlockEntities(w.Seed, chunk)
				generatedChunks[chunk] = entities
			}

			entity, present = entities[local]
		}

		prepared[index].generatedEntityPresent = present && blockEntityMatchesBlock(entity, current)
	}
}

func (w *World) generatedBlockEntities(chunk ChunkPosition) ChunkBlockEntities {
	generator, supported := w.Generator.(BlockEntityGenerator)
	if !supported {
		return nil
	}

	generated := generator.GenerateBlockEntities(w.Seed, chunk)
	if len(generated) == 0 {
		return nil
	}

	entities := make(ChunkBlockEntities, len(generated))

	for position, entity := range generated {
		entities[position] = entity.Clone()
	}

	return entities
}

func (w *World) generatedBlockEntityAt(position BlockPosition) (BlockEntity, bool) {
	pointGenerator, supported := w.Generator.(BlockEntityPointGenerator)
	if supported {
		entity, present := pointGenerator.GenerateBlockEntity(w.Seed, position)
		return entity.Clone(), present
	}

	chunk, local := blockIndex(position)

	entities := w.generatedBlockEntities(chunk)

	entity, present := entities[local]
	return entity, present
}

func resolvedBlockEntityOverride(override blockEntityOverride) (BlockEntity, bool) {
	if override.suppressed {
		return BlockEntity{}, false
	}

	return override.entity.Clone(), true
}

func (w *World) updateBlockEntityForBlockChange(position BlockPosition, current, replacement Block, generated bool) {
	currentType := BlockEntityTypeForBlock(current)
	replacementType := BlockEntityTypeForBlock(replacement)

	if currentType == replacementType {
		return
	}

	chunk, local := blockIndex(position)

	if replacementType == BlockEntityTypeNone {
		if generated {
			w.setBlockEntityOverride(chunk, local, blockEntityOverride{suppressed: true})
		} else {
			w.clearBlockEntityOverride(chunk, local)
		}

		return
	}

	w.setBlockEntityOverride(chunk, local, blockEntityOverride{entity: NewBlockEntity(replacementType)})
}

func (w *World) setBlockEntityOverride(chunk ChunkPosition, local LocalBlockPosition, override blockEntityOverride) {
	if w.blockEntities == nil {
		w.blockEntities = make(map[ChunkPosition]map[LocalBlockPosition]blockEntityOverride)
	}

	entities := w.blockEntities[chunk]
	if entities == nil {
		entities = make(map[LocalBlockPosition]blockEntityOverride)

		w.blockEntities[chunk] = entities
	}

	entities[local] = override
}

func (w *World) clearBlockEntityOverride(chunk ChunkPosition, local LocalBlockPosition) {
	entities := w.blockEntities[chunk]
	delete(entities, local)

	if len(entities) == 0 {
		delete(w.blockEntities, chunk)
	}
}

func (w *World) setBlock(position BlockPosition, block, generated Block) {
	chunk, local := blockIndex(position)

	if block == generated {
		w.clearBlockOverride(chunk, local)

		return
	}

	if w.overrides == nil {
		w.overrides = make(map[ChunkPosition]map[LocalBlockPosition]Block)
	}

	blocks := w.overrides[chunk]
	if blocks == nil {
		blocks = make(map[LocalBlockPosition]Block)
		w.overrides[chunk] = blocks
	}

	blocks[local] = block
}

func blockEntityRemovalNeedsGeneratedEntity(current, replacement Block) bool {
	return BlockEntityTypeForBlock(current) != BlockEntityTypeNone && BlockEntityTypeForBlock(replacement) == BlockEntityTypeNone
}

func blockEntityMatchesBlock(entity BlockEntity, block Block) bool {
	return entity.Type != BlockEntityTypeNone && entity.Type == BlockEntityTypeForBlock(block)
}

func (w *World) clearBlockOverride(chunk ChunkPosition, local LocalBlockPosition) {
	blocks := w.overrides[chunk]
	delete(blocks, local)

	if len(blocks) == 0 {
		delete(w.overrides, chunk)
	}
}

func NewOverworld(generator Generator, seed ...int64) *World {
	var worldSeed int64

	if len(seed) > 0 {
		worldSeed = seed[0]
	}

	spawn := Position{X: 0.5, Y: 70, Z: 0.5}

	metadata := WorldMetadata{SeaLevel: 63}

	if spawnGenerator, ok := generator.(SpawnGenerator); ok {
		spawn = spawnGenerator.Spawn(worldSeed)
	}

	if metadataGenerator, ok := generator.(WorldMetadataGenerator); ok {
		metadata = metadataGenerator.WorldMetadata(worldSeed)
	}

	world := &World{
		Name:          "minecraft:overworld",
		DimensionType: "minecraft:overworld",
		Seed:          worldSeed,
		Generator:     generator,
		Spawn:         spawn,

		SeaLevel: metadata.SeaLevel,
		Lighting: LightingFullbright,
	}

	world.SetTime(6000, true)

	return world
}

func ParseLightingMode(mode string) LightingMode {
	if mode == "fullbright" {
		return LightingFullbright
	}

	return LightingNormal
}

func blockIndex(position BlockPosition) (ChunkPosition, LocalBlockPosition) {
	chunkX := blockChunkCoordinate(position.X)
	chunkZ := blockChunkCoordinate(position.Z)

	return ChunkPosition{X: chunkX, Z: chunkZ}, LocalBlockPosition{
		X: position.X - chunkX*ChunkWidth,
		Y: position.Y,
		Z: position.Z - chunkZ*ChunkWidth,
	}
}

func blockChunkCoordinate(coordinate int32) int32 {
	chunk := coordinate / ChunkWidth

	if coordinate%ChunkWidth < 0 {
		chunk--
	}

	return chunk
}
