package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

type Generator struct{}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y < worldMinY || position.Y > worldMaxY {
		return game.Air
	}

	worldX := int64(position.X)
	worldZ := int64(position.Z)
	worldY := int64(position.Y)

	layer := layerAtY(position.Y)
	if layer < lowestLayerIndex {
		return game.SmoothStone
	}

	if layer > highestLayerIndex {
		return game.SmoothStone
	}

	current := zoneAtLayer(seed, worldX, worldZ, layer)
	blocks := blocksForPalette(current.palette)
	profile := structureAt(seed, current)

	return blockAtLayerColumn(seed, worldX, worldY, worldZ, current, blocks, profile)
}

func blockAtLayerColumn(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks, profile structure) game.Block {
	block, handled := grandAtriumBlockAt(seed, worldX, worldY, worldZ, current)
	if handled {
		return block
	}

	block, handled = layerConnectorBlockAt(seed, worldX, worldY, worldZ, current)
	if handled {
		return block
	}

	layerFloor := int64(layerFloorY(current.layer))

	templateY := int64(floorY) + (worldY - layerFloor)
	currentCeilingY := int64(zoneCeilingY(current))

	if templateY > currentCeilingY {
		return interstitialBlock(current.palette)
	}

	if templateY < int64(foundationY) {
		return game.Air
	}

	switch int32(templateY) {
	case foundationY:
		return foundationBlock(current.palette)
	case floorY:
		return floorBlock(seed, worldX, worldZ, current, blocks)
	case int32(currentCeilingY):
		return ceilingBlock(seed, worldX, worldZ, current, blocks)
	}

	block, ok := featureBlockAt(seed, worldX, templateY, worldZ, current)
	if ok {
		return block
	}

	block, ok = ambientDoorBlockAt(seed, templateY, current)
	if ok {
		return block
	}

	return structureBlock(seed, worldX, templateY, worldZ, current, blocks, profile)
}

func (generated Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, output *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < worldMinY || sectionMinY > worldMaxY {
		return game.Air, true
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	minLayer := layerAtY(max(sectionMinY, worldMinY))
	maxLayer := layerAtY(min(sectionMaxY, worldMaxY))

	minLayer = max(minLayer, lowestLayerIndex)
	maxLayer = min(maxLayer, highestLayerIndex)

	var layers [3][game.ChunkWidth * game.ChunkWidth]column

	layerCount := max(int(maxLayer-minLayer+1), 0)

	for layerOffset := 0; layerOffset < layerCount && layerOffset < len(layers); layerOffset++ {
		layer := minLayer + int64(layerOffset)

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				worldX := int64(chunkMinX + localX)
				worldZ := int64(chunkMinZ + localZ)

				current := zoneAtLayer(seed, worldX, worldZ, layer)

				layers[layerOffset][localZ*game.ChunkWidth+localX] = column{
					worldX:    worldX,
					worldZ:    worldZ,
					zone:      current,
					blocks:    blocksForPalette(current.palette),
					structure: structureAt(seed, current),
				}
			}
		}
	}

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				index := localY*256 + localZ*16 + localX
				block := game.Air

				switch {
				case worldY < worldMinY || worldY > worldMaxY:
					block = game.Air
				default:
					layer := layerAtY(worldY)
					if layer < lowestLayerIndex {
						block = game.SmoothStone
					} else if layer > highestLayerIndex {
						block = game.SmoothStone
					} else {
						layerOffset := int(layer - minLayer)
						currentColumn := layers[layerOffset][localZ*game.ChunkWidth+localX]

						block = blockAtLayerColumn(
							seed,
							currentColumn.worldX,
							int64(worldY),
							currentColumn.worldZ,
							currentColumn.zone,
							currentColumn.blocks,
							currentColumn.structure,
						)
					}
				}

				output[index] = block

				if index == 0 {
					first = block
				} else if block != first {
					uniform = false
				}
			}
		}
	}

	return first, uniform
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return worldMinY, worldMaxY, true
}

func (generated Generator) Spawn(seed int64) game.Position {
	for radius := int32(0); radius <= 30; radius++ {
		for z := -radius; z <= radius; z++ {
			for x := -radius; x <= radius; x++ {
				if radius != 0 && abs32(x) != radius && abs32(z) != radius {
					continue
				}

				if !generated.spawnOpen(seed, x, z) {
					continue
				}

				return game.Position{
					X: float64(x) + 0.5,
					Y: float64(floorY + 1),
					Z: float64(z) + 0.5,
				}
			}
		}
	}

	return game.Position{X: 0.5, Y: float64(floorY + 1), Z: 0.5}
}

func (generated Generator) spawnOpen(seed int64, x, z int32) bool {
	for y := floorY + 1; y <= floorY+2; y++ {
		if generated.BlockAt(seed, game.BlockPosition{X: x, Y: y, Z: z}) != game.Air {
			return false
		}
	}

	return generated.BlockAt(seed, game.BlockPosition{X: x, Y: floorY, Z: z}) != game.Air
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}
