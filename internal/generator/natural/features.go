package natural

import "github.com/coalaura/minicraft/internal/game"

func treeFeatureAt(seed int64, position game.BlockPosition) (game.Block, bool) {
	cellX := floorDiv(position.X, treeCellSize)
	cellZ := floorDiv(position.Z, treeCellSize)

	var (
		result  game.Block
		found   bool
		isTrunk bool
	)

	for candidateZ := cellZ - 1; candidateZ <= cellZ+1; candidateZ++ {
		for candidateX := cellX - 1; candidateX <= cellX+1; candidateX++ {
			candidate, ok := treeForCell(seed, candidateX, candidateZ)
			if !ok {
				continue
			}

			block, trunk, matches := treeBlockAt(candidate, position)
			if !matches {
				continue
			}

			if trunk || !isTrunk {
				result = block
				found = true
				isTrunk = trunk
			}
		}
	}

	return result, found
}

func treeForCell(seed int64, cellX, cellZ int32) (tree, bool) {
	hash := coordinateHash(seed, int64(cellX), 0, int64(cellZ), saltTree)

	offsetX := int32(2 + hash%4)
	offsetZ := int32(2 + (hash>>8)%4)

	worldX := cellX*treeCellSize + offsetX
	worldZ := cellZ*treeCellSize + offsetZ

	terrain := columnAt(seed, worldX, worldZ)

	if terrain.height <= seaLevel+1 || terrain.height >= 101 || terrain.beach {
		return tree{}, false
	}

	roll := int((hash >> 16) % 1000)
	kind := treeNone
	threshold := 0

	switch terrain.biome {
	case game.BiomeForest:
		kind = treeOak
		threshold = 590
	case game.BiomeTaiga:
		kind = treeSpruce
		threshold = 640
	case game.BiomeSwamp:
		kind = treeOak
		threshold = 250
	case game.BiomePlains:
		kind = treeOak
		threshold = 65
	default:
		return tree{}, false
	}

	if roll >= threshold {
		return tree{}, false
	}

	height := int32(4 + (hash>>28)%3)

	if kind == treeSpruce {
		height = int32(6 + (hash>>28)%3)
	}

	return tree{
		x:      worldX,
		z:      worldZ,
		baseY:  terrain.height,
		height: height,
		kind:   kind,
	}, true
}

func treeBlockAt(candidate tree, position game.BlockPosition) (game.Block, bool, bool) {
	if position.X == candidate.x && position.Z == candidate.z && position.Y > candidate.baseY && position.Y <= candidate.baseY+candidate.height {
		if candidate.kind == treeSpruce {
			return game.SpruceLog, true, true
		}

		return game.OakLog, true, true
	}

	dx := abs32(position.X - candidate.x)
	dz := abs32(position.Z - candidate.z)

	topY := candidate.baseY + candidate.height

	if candidate.kind == treeSpruce {
		radius, ok := spruceLeafRadius(position.Y - topY)
		if !ok || dx > radius || dz > radius {
			return game.Air, false, false
		}

		if radius == 2 && dx == 2 && dz == 2 {
			return game.Air, false, false
		}

		return generatedLeafState(game.SpruceLeaves, dx, dz, position.Y-topY), false, true
	}

	radius, ok := oakLeafRadius(position.Y - topY)
	if !ok || dx > radius || dz > radius {
		return game.Air, false, false
	}

	if radius == 2 && dx == 2 && dz == 2 {
		hash := coordinateHash(int64(candidate.x), int64(position.X), int64(position.Y), int64(position.Z), uint64(candidate.z)^saltTree)
		if hash&1 == 0 {
			return game.Air, false, false
		}
	}

	return generatedLeafState(game.OakLeaves, dx, dz, position.Y-topY), false, true
}

func generatedLeafState(block game.Block, dx, dz, relativeY int32) game.Block {
	distance := dx + dz

	if relativeY > 0 {
		distance += relativeY
	}

	state, valid := block.WithProperties(game.BlockPropertyValue{Name: "distance", Value: string(rune('0' + distance))})
	if !valid {
		return block
	}

	return state
}

func oakLeafRadius(relativeY int32) (int32, bool) {
	switch relativeY {
	case -2, -1, 0:
		return 2, true
	case 1:
		return 1, true
	default:
		return 0, false
	}
}

func spruceLeafRadius(relativeY int32) (int32, bool) {
	switch relativeY {
	case 1:
		return 0, true
	case 0, -1, -3:
		return 1, true
	case -2, -4:
		return 2, true
	default:
		return 0, false
	}
}

func cactusFeatureAt(seed int64, position game.BlockPosition, terrain column) (game.Block, bool) {
	if terrain.biome != game.BiomeDesert || terrain.beach || terrain.height <= seaLevel+1 {
		return game.Air, false
	}

	hash := coordinateHash(seed, int64(position.X), 0, int64(position.Z), saltDecor)
	if hash%109 != 0 {
		return game.Air, false
	}

	height := int32(2 + (hash>>16)%2)
	if position.Y > terrain.height && position.Y <= terrain.height+height {
		return game.Cactus, true
	}

	return game.Air, false
}

func surfaceDecorationAt(seed int64, position game.BlockPosition, terrain column) (game.Block, bool) {
	if position.Y != terrain.height+1 || terrain.height <= seaLevel || terrain.beach {
		return game.Air, false
	}

	if terrain.biome == game.BiomeSnowyPlains {
		return game.Snow, true
	}

	hash := coordinateHash(seed, int64(position.X), int64(position.Y), int64(position.Z), saltDecor)
	roll := hash % 1000

	switch terrain.biome {
	case game.BiomeForest:
		switch {
		case roll < 18:
			return flowerFor(hash), true
		case roll < 65:
			return game.Fern, true
		case roll < 285:
			return game.ShortGrass, true
		}
	case game.BiomeTaiga:
		switch {
		case roll < 125:
			return game.Fern, true
		case roll < 205:
			return game.ShortGrass, true
		}
	case game.BiomeSwamp:
		switch {
		case roll < 25:
			return game.BlueOrchid, true
		case roll < 155:
			return game.ShortGrass, true
		}
	case game.BiomePlains:
		switch {
		case roll < 42:
			return flowerFor(hash), true
		case roll < 225:
			return game.ShortGrass, true
		}
	}

	return game.Air, false
}

func flowerFor(hash uint64) game.Block {
	switch (hash >> 20) % 5 {
	case 0:
		return game.Dandelion
	case 1:
		return game.Poppy
	case 2:
		return game.Cornflower
	case 3:
		return game.OxeyeDaisy
	default:
		return game.AzureBluet
	}
}

func treesForChunk(seed int64, chunkPosition game.ChunkPosition) []tree {
	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth

	chunkMaxX := chunkMinX + game.ChunkWidth - 1
	chunkMaxZ := chunkMinZ + game.ChunkWidth - 1

	minCellX := floorDiv(chunkMinX-maxTreeRadius, treeCellSize)
	maxCellX := floorDiv(chunkMaxX+maxTreeRadius, treeCellSize)

	minCellZ := floorDiv(chunkMinZ-maxTreeRadius, treeCellSize)
	maxCellZ := floorDiv(chunkMaxZ+maxTreeRadius, treeCellSize)

	trees := make([]tree, 0, 16)

	for cellZ := minCellZ; cellZ <= maxCellZ; cellZ++ {
		for cellX := minCellX; cellX <= maxCellX; cellX++ {
			candidate, ok := treeForCell(seed, cellX, cellZ)
			if !ok {
				continue
			}

			trees = append(trees, candidate)
		}
	}

	return trees
}

func applyPreparedTrees(chunkPosition game.ChunkPosition, sectionMinY int32, trees []tree, blocks *[game.SectionVolume]game.Block) {
	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth

	chunkMaxX := chunkMinX + game.ChunkWidth - 1
	chunkMaxZ := chunkMinZ + game.ChunkWidth - 1

	sectionMaxY := sectionMinY + game.ChunkWidth - 1

	for _, candidate := range trees {

		minTreeY := candidate.baseY + 1
		maxTreeY := candidate.baseY + candidate.height + 1

		if maxTreeY < sectionMinY || minTreeY > sectionMaxY {
			continue
		}

		for y := max(sectionMinY, candidate.baseY+1); y <= min(sectionMaxY, candidate.baseY+candidate.height+1); y++ {
			for z := max(chunkMinZ, candidate.z-maxTreeRadius); z <= min(chunkMaxZ, candidate.z+maxTreeRadius); z++ {
				for x := max(chunkMinX, candidate.x-maxTreeRadius); x <= min(chunkMaxX, candidate.x+maxTreeRadius); x++ {
					position := game.BlockPosition{X: x, Y: y, Z: z}

					block, trunk, matches := treeBlockAt(candidate, position)
					if !matches {
						continue
					}

					index := (y-sectionMinY)*256 + (z-chunkMinZ)*16 + (x - chunkMinX)
					existing := blocks[index]

					if trunk {
						if existing == game.Air || existing == game.OakLeaves || existing == game.SpruceLeaves || existing == game.OakLog || existing == game.SpruceLog {
							blocks[index] = block
						}

						continue
					}

					if existing == game.Air || existing == game.OakLeaves || existing == game.SpruceLeaves {
						blocks[index] = block
					}
				}
			}
		}
	}
}

func applyColumnFeatures(seed int64, chunkPosition game.ChunkPosition, sectionMinY int32, columns *[game.ChunkWidth * game.ChunkWidth]column, blocks *[game.SectionVolume]game.Block) {
	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth

	sectionMaxY := sectionMinY + game.ChunkWidth - 1

	for localZ := range int32(game.ChunkWidth) {
		for localX := range int32(game.ChunkWidth) {
			terrain := columns[localZ*game.ChunkWidth+localX]

			worldX := chunkMinX + localX
			worldZ := chunkMinZ + localZ

			for worldY := max(sectionMinY, terrain.height+1); worldY <= min(sectionMaxY, terrain.height+3); worldY++ {
				position := game.BlockPosition{X: worldX, Y: worldY, Z: worldZ}

				index := (worldY-sectionMinY)*256 + localZ*16 + localX

				if blocks[index] != game.Air {
					continue
				}

				block, ok := cactusFeatureAt(seed, position, terrain)
				if ok {
					blocks[index] = block

					continue
				}

				block, ok = surfaceDecorationAt(seed, position, terrain)
				if ok {
					blocks[index] = block
				}
			}
		}
	}
}
