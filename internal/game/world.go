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

	overrideMx sync.RWMutex
	overrides  map[ChunkPosition]map[LocalBlockPosition]Block
}

type ChunkOverrides map[LocalBlockPosition]Block

type BlockChange struct {
	Position    BlockPosition
	Replacement Block
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

// SetBlocks applies a prevalidated group while holding the sparse override lock.
func (w *World) SetBlocks(changes []BlockChange) {
	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

	for _, change := range changes {
		w.setBlock(change.Position, change.Replacement)
	}
}

func (w *World) ClearBlockOverride(position BlockPosition) {
	chunk, local := blockIndex(position)

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

	w.clearBlockOverride(chunk, local)
}

func (w *World) SetLightingMode(mode LightingMode) {
	w.Lighting = mode
}

func (w *World) SetTime(dayTime int64, dayCycle bool) {
	w.age.Store(0)
	w.dayTime.Store(dayTime)
	w.dayCycle.Store(dayCycle)
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

func (w *World) setBlock(position BlockPosition, block Block) {
	chunk, local := blockIndex(position)

	if block == w.generatedBlockAt(position) {
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
