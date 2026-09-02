package server

import (
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	pointLightRadius = 15
	pointLightWidth  = pointLightRadius*2 + 1
	pointLightArea   = pointLightWidth * pointLightWidth
	pointLightVolume = pointLightArea * pointLightWidth
)

type pointLightBuffer struct {
	filter []byte
	sky    []byte
	block  []byte
	queued []bool
	queue  []int
}

func (buffer *pointLightBuffer) enqueue(index int) {
	if buffer.queued[index] {
		return
	}

	buffer.queued[index] = true
	buffer.queue = append(buffer.queue, index)
}

func (buffer *pointLightBuffer) propagate(light []byte) {
	for len(buffer.queue) != 0 {
		index := buffer.queue[0]
		buffer.queue = buffer.queue[1:]
		buffer.queued[index] = false

		level := light[index]
		if level <= 1 {
			continue
		}

		x := index % pointLightWidth
		yz := index / pointLightWidth
		z := yz % pointLightWidth
		y := yz / pointLightWidth

		if x > 0 {
			buffer.spread(light, index-1, level)
		}

		if x+1 < pointLightWidth {
			buffer.spread(light, index+1, level)
		}

		if z > 0 {
			buffer.spread(light, index-pointLightWidth, level)
		}

		if z+1 < pointLightWidth {
			buffer.spread(light, index+pointLightWidth, level)
		}

		if y > 0 {
			buffer.spread(light, index-pointLightArea, level)
		}

		if y+1 < pointLightWidth {
			buffer.spread(light, index+pointLightArea, level)
		}
	}
}

func (buffer *pointLightBuffer) spread(light []byte, target int, sourceLevel byte) {
	attenuation := max(buffer.filter[target], byte(1))
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

func rawBrightnessAt(world *game.World, position game.BlockPosition) (uint8, error) {
	if world.Lighting == game.LightingFullbright {
		return 15, nil
	}

	buffer := pointLightBuffer{
		filter: make([]byte, pointLightVolume),
		sky:    make([]byte, pointLightVolume),
		block:  make([]byte, pointLightVolume),
		queued: make([]bool, pointLightVolume),
		queue:  make([]int, 0, pointLightVolume),
	}

	err := populatePointLight(world, position, &buffer)
	if err != nil {
		return 0, err
	}

	propagatePointLight(&buffer)

	center := pointLightIndex(pointLightRadius, pointLightRadius, pointLightRadius)
	return max(buffer.sky[center], buffer.block[center]), nil
}

func populatePointLight(world *game.World, position game.BlockPosition, buffer *pointLightBuffer) error {
	minX := position.X - pointLightRadius
	maxX := position.X + pointLightRadius
	minZ := position.Z - pointLightRadius
	maxZ := position.Z + pointLightRadius
	minY := position.Y - pointLightRadius
	maxY := position.Y + pointLightRadius

	levels := make([]byte, pointLightArea)

	for index := range levels {
		levels[index] = 15
	}

	minChunkX := blockChunkCoordinate(minX)
	maxChunkX := blockChunkCoordinate(maxX)
	minChunkZ := blockChunkCoordinate(minZ)
	maxChunkZ := blockChunkCoordinate(maxZ)

	for chunkZ := minChunkZ; chunkZ <= maxChunkZ; chunkZ++ {
		for chunkX := minChunkX; chunkX <= maxChunkX; chunkX++ {
			chunk := game.ChunkPosition{X: chunkX, Z: chunkZ}

			prepared := prepareChunkGeneration(world, chunk)

			overrides := world.SnapshotChunkOverrides(chunk)

			var generated [game.SectionVolume]game.Block

			generationMinY := int32(protocol.OverworldMinY)
			generationMaxY := int32(protocol.OverworldMinY + protocol.OverworldSectionCount*game.ChunkWidth - 1)
			hasGeneration := world.Generator != nil

			boundedGenerator, bounded := world.Generator.(game.BoundedGenerator)
			if bounded {
				generationMinY, generationMaxY, hasGeneration = boundedGenerator.GenerationBounds(world.Seed, chunk)
			}

			for sectionIndex := protocol.OverworldSectionCount - 1; sectionIndex >= 0; sectionIndex-- {
				sectionMinY := int32(protocol.OverworldMinY + sectionIndex*game.ChunkWidth)
				sectionMaxY := sectionMinY + game.ChunkWidth - 1
				uniformBlock := game.Air
				uniform := true

				if hasGeneration && sectionMaxY >= generationMinY && sectionMinY <= generationMaxY {
					uniformBlock, uniform = prepared.GenerateSection(sectionMinY, &generated)
				}

				for localY := game.ChunkWidth - 1; localY >= 0; localY-- {
					worldY := sectionMinY + int32(localY)

					for localZ := range game.ChunkWidth {
						worldZ := chunkZ*game.ChunkWidth + int32(localZ)

						if worldZ < minZ || worldZ > maxZ {
							continue
						}

						for localX := range game.ChunkWidth {
							worldX := chunkX*game.ChunkWidth + int32(localX)

							if worldX < minX || worldX > maxX {
								continue
							}

							sectionOffset := localY*256 + localZ*16 + localX
							block := uniformBlock

							if !uniform {
								block = generated[sectionOffset]
							}

							local := game.LocalBlockPosition{X: int32(localX), Y: worldY, Z: int32(localZ)}

							override, overridden := overrides[local]
							if overridden {
								block = override
							}

							if !block.Valid() {
								return fmt.Errorf("unsupported game block %d in point lighting region", block)
							}

							x := int(worldX - minX)
							z := int(worldZ - minZ)

							column := z*pointLightWidth + x

							emission, filter := block.LightProperties()

							level := levels[column]
							if filter >= level {
								level = 0
							} else if filter != 0 {
								level -= filter
							}

							levels[column] = level

							if worldY < minY || worldY > maxY {
								continue
							}

							y := int(worldY - minY)

							index := pointLightIndex(x, y, z)

							buffer.filter[index] = filter
							buffer.sky[index] = level
							buffer.block[index] = emission
						}
					}
				}
			}
		}
	}

	return nil
}

func propagatePointLight(buffer *pointLightBuffer) {
	for y := 1; y < pointLightWidth-1; y++ {
		for z := 1; z < pointLightWidth-1; z++ {
			for x := 1; x < pointLightWidth-1; x++ {
				index := pointLightIndex(x, y, z)
				if buffer.sky[index] > 1 {
					buffer.enqueue(index)
				}
			}
		}
	}

	buffer.propagate(buffer.sky)

	for index, level := range buffer.block {
		if level > 1 {
			buffer.enqueue(index)
		}
	}

	buffer.propagate(buffer.block)
}

func pointLightIndex(x, y, z int) int {
	return y*pointLightArea + z*pointLightWidth + x
}
