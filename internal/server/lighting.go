package server

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	lightingHalo   = 15
	lightingWidth  = game.ChunkWidth + lightingHalo*2
	lightingHeight = protocol.OverworldSectionCount * game.ChunkWidth
	lightingVolume = lightingWidth * lightingWidth * lightingHeight
)

type lightingBuffer struct {
	targetBlocks []game.Block
	filter       []byte
	sky          []byte
	block        []byte
	queued       []byte
	queue        []int32
	queueHead    int
	queueTail    int
	queueCount   int
}

type lightingCharacteristics struct {
	hasFilter   bool
	hasEmission bool
}

var lightingBuffers = sync.Pool{
	New: func() any {
		return &lightingBuffer{
			targetBlocks: make([]game.Block, game.ChunkWidth*game.ChunkWidth*lightingHeight),
			filter:       make([]byte, lightingVolume),
			sky:          make([]byte, lightingVolume),
			block:        make([]byte, lightingVolume),
			queued:       make([]byte, lightingVolume),
			queue:        make([]int32, lightingVolume),
		}
	},
}

func buildNormalLevelChunk(world *game.World, chunkX, chunkZ int32) (protocol.LevelChunkWithLight, error) {
	if normalRegionIsOpen(world, chunkX, chunkZ) {
		chunk, err := buildFullbrightLevelChunk(world, chunkX, chunkZ)
		if err != nil {
			return protocol.LevelChunkWithLight{}, err
		}

		light := protocol.NewOpenOverworldLight(chunkX, chunkZ)

		chunk.SkyLightMask = light.SkyLightMask
		chunk.BlockLightMask = light.BlockLightMask
		chunk.EmptySkyLightMask = light.EmptySkyLightMask
		chunk.EmptyBlockLightMask = light.EmptyBlockLightMask
		chunk.SkyLight = light.SkyLight
		chunk.BlockLight = light.BlockLight

		return chunk, nil
	}

	buffer := acquireLightingBuffer()
	defer releaseLightingBuffer(buffer)

	var biomes chunkBiomes

	characteristics, err := generateLightingBlocks(world, chunkX, chunkZ, buffer, &biomes)
	if err != nil {
		return protocol.LevelChunkWithLight{}, err
	}

	calculateLight(buffer, characteristics)

	chunk := protocol.NewEmptyOverworldChunk(chunkX, chunkZ, defaultBiomeID)

	chunk.SkyLightMask = nil
	chunk.SkyLight = nil

	var sectionBlocks protocol.SectionBlocks

	for sectionIndex := range chunk.Sections {
		for localY := range game.ChunkWidth {
			worldY := sectionIndex*game.ChunkWidth + localY

			for localZ := range game.ChunkWidth {
				for localX := range game.ChunkWidth {
					block := buffer.targetBlocks[chunkBlockIndex(localX, worldY, localZ)]

					sectionBlocks.Set(localX, localY, localZ, int32(block))
				}
			}
		}

		section := sectionBlocks.ToSection(defaultBiomeID)

		if biomes.present {
			section.SetBiomes(&biomes.sections[sectionIndex])
		}

		chunk.Sections[sectionIndex] = section
	}

	light := lightUpdateFromBuffer(buffer, chunkX, chunkZ, characteristics)

	chunk.SkyLightMask = light.SkyLightMask
	chunk.BlockLightMask = light.BlockLightMask
	chunk.EmptySkyLightMask = light.EmptySkyLightMask
	chunk.EmptyBlockLightMask = light.EmptyBlockLightMask
	chunk.SkyLight = light.SkyLight
	chunk.BlockLight = light.BlockLight
	chunk.BlockEntities = protocolChunkBlockEntities(world.SnapshotChunkBlockEntities(game.ChunkPosition{X: chunkX, Z: chunkZ}))

	return chunk, nil
}

func buildChunkLight(world *game.World, chunkX, chunkZ int32) (protocol.UpdateLight, error) {
	if normalRegionIsOpen(world, chunkX, chunkZ) {
		return protocol.NewOpenOverworldLight(chunkX, chunkZ), nil
	}

	buffer := acquireLightingBuffer()
	defer releaseLightingBuffer(buffer)

	characteristics, err := generateLightingBlocks(world, chunkX, chunkZ, buffer, nil)
	if err != nil {
		return protocol.UpdateLight{}, err
	}

	calculateLight(buffer, characteristics)

	return lightUpdateFromBuffer(buffer, chunkX, chunkZ, characteristics), nil
}

func buildChangedLightUpdates(world *game.World, changes []game.BlockChange) ([]protocol.UpdateLight, error) {
	affectedSet := make(map[LoadedChunk]struct{}, len(changes)*4)

	for _, change := range changes {
		minChunkX := blockChunkCoordinate(change.Position.X - 14)
		maxChunkX := blockChunkCoordinate(change.Position.X + 14)
		minChunkZ := blockChunkCoordinate(change.Position.Z - 14)
		maxChunkZ := blockChunkCoordinate(change.Position.Z + 14)

		for chunkZ := minChunkZ; chunkZ <= maxChunkZ; chunkZ++ {
			for chunkX := minChunkX; chunkX <= maxChunkX; chunkX++ {
				affectedSet[LoadedChunk{X: chunkX, Z: chunkZ}] = struct{}{}
			}
		}
	}

	affected := make([]LoadedChunk, 0, len(affectedSet))

	for chunk := range affectedSet {
		affected = append(affected, chunk)
	}

	sort.Slice(affected, func(first, second int) bool {
		if affected[first].Z == affected[second].Z {
			return affected[first].X < affected[second].X
		}

		return affected[first].Z < affected[second].Z
	})

	updates := make([]protocol.UpdateLight, len(affected))

	workerCount := min(len(affected), min(runtime.GOMAXPROCS(0), 8))
	if workerCount == 0 {
		return updates, nil
	}

	var (
		nextIndex atomic.Int64
		workers   sync.WaitGroup
		firstErr  error
		errOnce   sync.Once
	)

	workers.Add(workerCount)

	for range workerCount {
		go func() {
			defer workers.Done()

			for {
				index := int(nextIndex.Add(1) - 1)
				if index >= len(affected) {
					return
				}

				chunk := affected[index]

				update, err := buildChunkLight(world, chunk.X, chunk.Z)
				if err != nil {
					errOnce.Do(func() {
						firstErr = err
					})

					return
				}

				updates[index] = update
			}
		}()
	}

	workers.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return updates, nil
}

func acquireLightingBuffer() *lightingBuffer {
	buffer := lightingBuffers.Get().(*lightingBuffer)

	clear(buffer.targetBlocks)
	clear(buffer.filter)
	clear(buffer.block)
	clear(buffer.queued)

	buffer.resetQueue()

	return buffer
}

func releaseLightingBuffer(buffer *lightingBuffer) {
	lightingBuffers.Put(buffer)
}

func generateLightingBlocks(world *game.World, targetX, targetZ int32, buffer *lightingBuffer, biomes *chunkBiomes) (lightingCharacteristics, error) {
	generator := world.Generator

	var (
		characteristics lightingCharacteristics
		generated       [game.SectionVolume]game.Block
	)

	for chunkZ := targetZ - 1; chunkZ <= targetZ+1; chunkZ++ {
		for chunkX := targetX - 1; chunkX <= targetX+1; chunkX++ {
			chunkPosition := game.ChunkPosition{X: chunkX, Z: chunkZ}

			prepared := prepareChunkGeneration(world, chunkPosition)

			overrides := world.SnapshotChunkOverrides(chunkPosition)

			generationMinY := int32(protocol.OverworldMinY)
			generationMaxY := generationMinY + lightingHeight - 1

			hasGeneration := generator != nil

			if boundedGenerator, bounded := generator.(game.BoundedGenerator); bounded {
				generationMinY, generationMaxY, hasGeneration = boundedGenerator.GenerationBounds(world.Seed, chunkPosition)
			}

			for sectionIndex := range protocol.OverworldSectionCount {
				sectionMinY := int32(protocol.OverworldMinY + sectionIndex*game.ChunkWidth)
				sectionMaxY := sectionMinY + game.ChunkWidth - 1

				if biomes != nil && chunkX == targetX && chunkZ == targetZ {
					sectionBiomes, present, err := buildSectionBiomes(prepared, sectionMinY)
					if err != nil {
						return lightingCharacteristics{}, err
					}

					biomes.sections[sectionIndex] = sectionBiomes
					biomes.present = present
				}

				if !hasGeneration || sectionMaxY < generationMinY || sectionMinY > generationMaxY {
					continue
				}

				var (
					uniformBlock game.Block
					uniform      bool
				)

				uniformBlock, uniform = prepared.GenerateSection(sectionMinY, &generated)

				for localY := range game.ChunkWidth {
					for localZ := range game.ChunkWidth {
						regionZ := int(chunkZ-targetZ)*game.ChunkWidth + localZ + lightingHalo
						if regionZ < 0 || regionZ >= lightingWidth {
							continue
						}

						for localX := range game.ChunkWidth {
							regionX := int(chunkX-targetX)*game.ChunkWidth + localX + lightingHalo
							if regionX < 0 || regionX >= lightingWidth {
								continue
							}

							sectionOffset := localY*256 + localZ*16 + localX
							block := uniformBlock

							if !uniform {
								block = generated[sectionOffset]
							}

							if !block.Valid() {
								return lightingCharacteristics{}, fmt.Errorf("unsupported game block %d in lighting region", block)
							}

							worldY := sectionIndex*game.ChunkWidth + localY

							index := lightingIndex(regionX, worldY, regionZ)

							emission, filter := block.LightProperties()

							buffer.block[index] = emission
							buffer.filter[index] = filter

							characteristics.hasEmission = characteristics.hasEmission || emission != 0
							characteristics.hasFilter = characteristics.hasFilter || filter != 0

							if regionX >= lightingHalo && regionX < lightingHalo+game.ChunkWidth && regionZ >= lightingHalo && regionZ < lightingHalo+game.ChunkWidth {
								buffer.targetBlocks[chunkBlockIndex(regionX-lightingHalo, worldY, regionZ-lightingHalo)] = block
							}
						}
					}
				}
			}

			for position, block := range overrides {
				regionX := int(chunkX-targetX)*game.ChunkWidth + int(position.X) + lightingHalo
				regionZ := int(chunkZ-targetZ)*game.ChunkWidth + int(position.Z) + lightingHalo

				worldY := int(position.Y - protocol.OverworldMinY)

				if regionX < 0 || regionX >= lightingWidth || regionZ < 0 || regionZ >= lightingWidth || worldY < 0 || worldY >= lightingHeight {
					continue
				}

				if !block.Valid() {
					return lightingCharacteristics{}, fmt.Errorf("unsupported game block %d in lighting override", block)
				}

				index := lightingIndex(regionX, worldY, regionZ)

				emission, filter := block.LightProperties()

				buffer.block[index] = emission
				buffer.filter[index] = filter

				characteristics.hasEmission = characteristics.hasEmission || emission != 0
				characteristics.hasFilter = characteristics.hasFilter || filter != 0

				if regionX >= lightingHalo && regionX < lightingHalo+game.ChunkWidth && regionZ >= lightingHalo && regionZ < lightingHalo+game.ChunkWidth {
					buffer.targetBlocks[chunkBlockIndex(regionX-lightingHalo, worldY, regionZ-lightingHalo)] = block
				}
			}
		}
	}

	return characteristics, nil
}

func calculateLight(buffer *lightingBuffer, characteristics lightingCharacteristics) {
	if !characteristics.hasFilter && !characteristics.hasEmission {
		return
	}

	for regionZ := range lightingWidth {
		for regionX := range lightingWidth {
			level := uint8(15)

			for worldY := lightingHeight - 1; worldY >= 0; worldY-- {
				index := lightingIndex(regionX, worldY, regionZ)

				filter := buffer.filter[index]

				if filter >= level {
					level = 0
				} else if filter != 0 {
					level -= filter
				}

				buffer.sky[index] = level
			}
		}
	}

	if characteristics.hasFilter {
		seedSkyPropagation(buffer)

		propagateLight(buffer, buffer.sky)
	}

	if characteristics.hasEmission {
		for index, level := range buffer.block {
			if level > 1 {
				buffer.enqueue(index)
			}
		}

		propagateLight(buffer, buffer.block)
	}
}

func seedSkyPropagation(buffer *lightingBuffer) {
	for worldY := range lightingHeight {
		for regionZ := 1; regionZ < lightingWidth-1; regionZ++ {
			for regionX := 1; regionX < lightingWidth-1; regionX++ {
				index := lightingIndex(regionX, worldY, regionZ)

				level := buffer.sky[index]
				if level <= 1 {
					continue
				}

				if lightCanImprove(buffer, buffer.sky, index-1, level) || lightCanImprove(buffer, buffer.sky, index+1, level) || lightCanImprove(buffer, buffer.sky, index-lightingWidth, level) || lightCanImprove(buffer, buffer.sky, index+lightingWidth, level) {
					buffer.enqueue(index)
				}
			}
		}
	}
}

func propagateLight(buffer *lightingBuffer, light []byte) {
	for buffer.queueCount != 0 {
		index := buffer.dequeue()

		level := light[index]
		if level <= 1 {
			continue
		}

		x := index % lightingWidth
		yz := index / lightingWidth

		z := yz % lightingWidth
		y := yz / lightingWidth

		if x > 0 {
			spreadLight(buffer, light, index-1, level)
		}

		if x+1 < lightingWidth {
			spreadLight(buffer, light, index+1, level)
		}

		if z > 0 {
			spreadLight(buffer, light, index-lightingWidth, level)
		}

		if z+1 < lightingWidth {
			spreadLight(buffer, light, index+lightingWidth, level)
		}

		if y > 0 {
			spreadLight(buffer, light, index-lightingWidth*lightingWidth, level)
		}

		if y+1 < lightingHeight {
			spreadLight(buffer, light, index+lightingWidth*lightingWidth, level)
		}
	}
}

func spreadLight(buffer *lightingBuffer, light []byte, target int, sourceLevel uint8) {
	attenuation := max(buffer.filter[target], uint8(1))
	if attenuation >= sourceLevel {
		return
	}

	candidate := sourceLevel - attenuation
	if candidate <= light[target] {
		return
	}

	light[target] = candidate

	buffer.enqueue(target)
}

func lightCanImprove(buffer *lightingBuffer, light []byte, target int, sourceLevel uint8) bool {
	attenuation := max(buffer.filter[target], uint8(1))

	return attenuation < sourceLevel && sourceLevel-attenuation > light[target]
}

func lightUpdateFromBuffer(buffer *lightingBuffer, chunkX, chunkZ int32, characteristics lightingCharacteristics) protocol.UpdateLight {
	if !characteristics.hasFilter && !characteristics.hasEmission {
		return protocol.NewOpenOverworldLight(chunkX, chunkZ)
	}

	update := protocol.UpdateLight{Position: protocol.ChunkPosition{X: chunkX, Z: chunkZ}}

	var (
		skyMask        int64
		blockMask      int64
		emptySkyMask   int64 = 1
		emptyBlockMask int64 = 1 | 1<<(protocol.OverworldLightSectionCount-1)
		skyBacking     []byte
		blockBacking   []byte
	)

	update.SkyLight = make([][]byte, 0, protocol.OverworldLightSectionCount-1)
	update.BlockLight = make([][]byte, 0, protocol.OverworldSectionCount)

	for sectionIndex := range protocol.OverworldSectionCount {
		lightSection := sectionIndex + 1

		skyArray, nonzero := packLightSection(buffer.sky, sectionIndex)
		if nonzero {
			if skyBacking == nil {
				skyBacking = make([]byte, protocol.SkyLightArrayLength*(protocol.OverworldSectionCount+1))
			}

			offset := len(update.SkyLight) * protocol.SkyLightArrayLength
			copy(skyBacking[offset:], skyArray[:])

			update.SkyLight = append(update.SkyLight, skyBacking[offset:offset+protocol.SkyLightArrayLength])

			skyMask |= 1 << lightSection
		} else {
			emptySkyMask |= 1 << lightSection
		}

		blockArray, nonzero := packLightSection(buffer.block, sectionIndex)
		if nonzero {
			if blockBacking == nil {
				blockBacking = make([]byte, protocol.SkyLightArrayLength*protocol.OverworldSectionCount)
			}

			offset := len(update.BlockLight) * protocol.SkyLightArrayLength
			copy(blockBacking[offset:], blockArray[:])

			update.BlockLight = append(update.BlockLight, blockBacking[offset:offset+protocol.SkyLightArrayLength])

			blockMask |= 1 << lightSection
		} else {
			emptyBlockMask |= 1 << lightSection
		}
	}

	if skyBacking == nil {
		skyBacking = make([]byte, protocol.SkyLightArrayLength)
	}

	topOffset := len(update.SkyLight) * protocol.SkyLightArrayLength

	for index := range skyBacking[topOffset : topOffset+protocol.SkyLightArrayLength] {
		skyBacking[topOffset+index] = 0xff
	}

	update.SkyLight = append(update.SkyLight, skyBacking[topOffset:topOffset+protocol.SkyLightArrayLength])

	skyMask |= 1 << (protocol.OverworldLightSectionCount - 1)

	update.SkyLightMask = []int64{skyMask}
	update.BlockLightMask = []int64{blockMask}
	update.EmptySkyLightMask = []int64{emptySkyMask}
	update.EmptyBlockLightMask = []int64{emptyBlockMask}

	return update
}

func normalRegionIsOpen(world *game.World, targetX, targetZ int32) bool {
	if world.Generator != nil {
		return false
	}

	for chunkZ := targetZ - 1; chunkZ <= targetZ+1; chunkZ++ {
		for chunkX := targetX - 1; chunkX <= targetX+1; chunkX++ {
			if len(world.SnapshotChunkOverrides(game.ChunkPosition{X: chunkX, Z: chunkZ})) != 0 {
				return false
			}
		}
	}

	return true
}

func packLightSection(light []byte, sectionIndex int) ([protocol.SkyLightArrayLength]byte, bool) {
	var (
		packed  [protocol.SkyLightArrayLength]byte
		nonzero bool
	)

	for localY := range game.ChunkWidth {
		worldY := sectionIndex*game.ChunkWidth + localY

		for localZ := range game.ChunkWidth {
			for localX := range game.ChunkWidth {
				level := light[lightingIndex(localX+lightingHalo, worldY, localZ+lightingHalo)]

				blockIndex := localY*256 + localZ*16 + localX
				packed[blockIndex>>1] |= level << ((blockIndex & 1) * 4)

				nonzero = nonzero || level != 0
			}
		}
	}

	return packed, nonzero
}

func lightingIndex(x, y, z int) int {
	return (y*lightingWidth+z)*lightingWidth + x
}

func chunkBlockIndex(x, y, z int) int {
	return (y*game.ChunkWidth+z)*game.ChunkWidth + x
}

func (buffer *lightingBuffer) resetQueue() {
	buffer.queueHead = 0
	buffer.queueTail = 0
	buffer.queueCount = 0
}

func (buffer *lightingBuffer) enqueue(index int) {
	if buffer.queued[index] != 0 {
		return
	}

	buffer.queued[index] = 1
	buffer.queue[buffer.queueTail] = int32(index)

	buffer.queueTail++

	if buffer.queueTail == len(buffer.queue) {
		buffer.queueTail = 0
	}

	buffer.queueCount++
}

func (buffer *lightingBuffer) dequeue() int {
	index := int(buffer.queue[buffer.queueHead])

	buffer.queueHead++

	if buffer.queueHead == len(buffer.queue) {
		buffer.queueHead = 0
	}

	buffer.queueCount--
	buffer.queued[index] = 0

	return index
}
