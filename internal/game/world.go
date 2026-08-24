package game

import "sync"

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

	overrideMx sync.RWMutex
	overrides  map[ChunkPosition]map[LocalBlockPosition]Block
}

type SpawnGenerator interface {
	Spawn(seed int64) Position
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
	chunk, local := blockIndex(position)

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

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

func (w *World) ClearBlockOverride(position BlockPosition) {
	chunk, local := blockIndex(position)

	w.overrideMx.Lock()
	defer w.overrideMx.Unlock()

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

	if spawnGenerator, ok := generator.(SpawnGenerator); ok {
		spawn = spawnGenerator.Spawn(worldSeed)
	}

	return &World{
		Name:          "minecraft:overworld",
		DimensionType: "minecraft:overworld",
		Seed:          worldSeed,
		Generator:     generator,
		Spawn:         spawn,

		SeaLevel: 64,
	}
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
