package natural

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const spawnSearchRadius = int32(192)

func (Generator) Spawn(seed int64) game.Position {
	bestX := int32(0)
	bestZ := int32(0)

	bestScore := math.MaxFloat64

	for radius := int32(0); radius <= spawnSearchRadius; radius += 8 {
		edges := []int32{-radius, radius}

		for z := -radius; z <= radius; z += 8 {
			for _, x := range edges {
				spawnScore := scoreSpawn(seed, x, z)
				if spawnScore < bestScore {
					bestX = x
					bestZ = z
					bestScore = spawnScore
				}
			}
		}

		for x := -radius + 8; x < radius; x += 8 {
			for _, z := range edges {
				spawnScore := scoreSpawn(seed, x, z)
				if spawnScore < bestScore {
					bestX = x
					bestZ = z
					bestScore = spawnScore
				}
			}
		}

		if bestScore < 8 {
			break
		}
	}

	terrain := columnAt(seed, bestX, bestZ)

	return game.Position{
		X: float64(bestX) + 0.5,
		Y: float64(terrain.height + 1),
		Z: float64(bestZ) + 0.5,
	}
}

func scoreSpawn(seed int64, x, z int32) float64 {
	terrain := columnAt(seed, x, z)
	if terrain.height <= seaLevel+1 || terrain.height >= 96 || terrain.beach {
		return math.MaxFloat64
	}

	switch terrain.biome {
	case game.BiomePlains:
	case game.BiomeForest:
	case game.BiomeTaiga:
	default:
		return math.MaxFloat64
	}

	position := game.BlockPosition{X: x, Y: terrain.height + 1, Z: z}

	block, ok := treeFeatureAt(seed, position)
	if ok && block != game.OakLeaves && block != game.SpruceLeaves {
		return math.MaxFloat64
	}

	centerPenalty := math.Hypot(float64(x), float64(z)) / 16
	heightPenalty := math.Abs(float64(terrain.height-70)) * 0.35

	forestPenalty := 0.0

	if terrain.biome != game.BiomePlains {
		forestPenalty = 4
	}

	return centerPenalty + heightPenalty + forestPenalty
}
