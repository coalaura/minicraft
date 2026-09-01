package grandcanyon

import "github.com/coalaura/minicraft/internal/game"

const (
	saltFeatureCactus  uint64 = 0xa4093822299f31d0
	saltFeatureDecor   uint64 = 0x589965cc75374cc3
	saltFeatureBoulder uint64 = 0xeb44accab455d165
)

func cactusFeatureAt(seed int64, position game.BlockPosition, terrain column) (game.Block, bool) {
	if terrain.height < riverLevel+4 {
		return game.Air, false
	}

	if terrain.riverStrength > 0.20 {
		return game.Air, false
	}

	if terrain.slope > 2 {
		return game.Air, false
	}

	hash := coordinateHash(
		seed,
		int64(position.X),
		0,
		int64(position.Z),
		saltFeatureCactus,
	)

	isPlateau := terrain.canyonStrength < 0.22
	isBench := terrain.terraceBench && terrain.canyonStrength < 0.75

	if !isPlateau && !isBench {
		return game.Air, false
	}

	chance := uint64(140)

	if isBench {
		chance = 260
	}

	if hash%chance != 0 {
		return game.Air, false
	}

	cactusHeight := int32(1 + (hash>>8)%3)
	if position.Y > terrain.height && position.Y <= terrain.height+cactusHeight {
		return palette.cactus, true
	}

	return game.Air, false
}

func surfaceDecorationAt(seed int64, position game.BlockPosition, terrain column) (game.Block, bool) {
	if position.Y != terrain.height+1 {
		return game.Air, false
	}

	if terrain.height < riverLevel+1 {
		return game.Air, false
	}

	if terrain.slope > 3 {
		return game.Air, false
	}

	hash := coordinateHash(
		seed,
		int64(position.X),
		int64(terrain.height),
		int64(position.Z),
		saltFeatureDecor,
	)

	isPlateau := terrain.canyonStrength < 0.24
	isBench := terrain.terraceBench && terrain.canyonStrength < 0.85

	if isPlateau {
		if hash%48 == 0 {
			return palette.deadBush, true
		}

		return game.Air, false
	}

	if isBench {
		if hash%64 == 0 {
			return palette.deadBush, true
		}

		return game.Air, false
	}

	return game.Air, false
}

func riverBoulderAt(seed int64, position game.BlockPosition, terrain column) (game.Block, bool) {
	if position.Y != terrain.height+1 {
		return game.Air, false
	}

	if terrain.riverStrength < 0.35 || terrain.riverStrength > 0.85 {
		return game.Air, false
	}

	if terrain.height < riverLevel-1 || terrain.height > riverLevel+2 {
		return game.Air, false
	}

	hash := coordinateHash(
		seed,
		int64(position.X),
		int64(position.Y),
		int64(position.Z),
		saltFeatureBoulder,
	)

	if hash%37 != 0 {
		return game.Air, false
	}

	switch hash % 4 {
	case 0:
		return palette.granite, true
	case 1:
		return palette.andesite, true
	case 2:
		return palette.cobblestone, true
	default:
		return game.Stone, true
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

			minY := max(sectionMinY, terrain.height+1)
			maxY := min(sectionMaxY, terrain.height+4)

			for worldY := minY; worldY <= maxY; worldY++ {
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

				block, ok = riverBoulderAt(seed, position, terrain)
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
