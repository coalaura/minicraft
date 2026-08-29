package natural

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

func columnAt(seed int64, worldX, worldZ int32) column {
	x := float64(worldX)
	z := float64(worldZ)

	warpX := fractalNoise(seed, x/560, z/560, 3, saltWarpX) * 58
	warpZ := fractalNoise(seed, x/560, z/560, 3, saltWarpZ) * 58

	warpedX := x + warpX
	warpedZ := z + warpZ

	continentalness := fractalNoise(seed, warpedX/920, warpedZ/920, 4, saltContinents)
	elevation := fractalNoise(seed, warpedX/230, warpedZ/230, 4, saltElevation)

	mountainField := fractalNoise(seed, warpedX/690, warpedZ/690, 3, saltMountains)
	mountainMask := smoothstep(0.12, 0.66, mountainField)

	ridgeField := 1 - math.Abs(fractalNoise(seed, warpedX/175, warpedZ/175, 4, saltRidges))
	ridge := smoothstep(0.32, 0.86, ridgeField)
	ridge *= ridge

	height := 64.5 + continentalness*27 + elevation*9
	mountainHeight := mountainMask * ridge * (29 + 27*smoothstep(0.18, 0.82, continentalness))
	height += mountainHeight

	riverField := fractalNoise(seed, (warpedX+warpZ*0.08)/470, (warpedZ-warpedX*0.06)/470, 3, saltRivers)

	riverStrength := 1 - smoothstep(0.015, 0.072, math.Abs(riverField))
	riverStrength *= smoothstep(-0.12, 0.18, continentalness)
	riverStrength *= 1 - mountainMask*0.76
	riverStrength = clamp01(riverStrength)

	if height > float64(seaLevel-2) && riverStrength > 0 {
		riverTarget := float64(seaLevel - 2)
		height = lerp(height, riverTarget, riverStrength*0.92)
	}

	height = math.Max(36, math.Min(157, height))
	surfaceHeight := int32(math.Round(height))

	temperature := 0.5 + fractalNoise(seed, warpedX/1180, warpedZ/1180, 3, saltTemperature)*0.5
	humidity := 0.5 + fractalNoise(seed, warpedX/1040, warpedZ/1040, 3, saltHumidity)*0.5

	if surfaceHeight > 88 {
		temperature -= float64(surfaceHeight-88) / 150
	}

	temperature = clamp01(temperature)
	humidity = clamp01(humidity)

	biome := chooseBiome(surfaceHeight, temperature, humidity, riverStrength)
	beach := surfaceHeight >= seaLevel-1 && surfaceHeight <= seaLevel+2 && riverStrength < 0.56 && biome != game.BiomeSwamp

	return column{
		height:        surfaceHeight,
		biome:         biome,
		temperature:   temperature,
		humidity:      humidity,
		riverStrength: riverStrength,
		beach:         beach,
	}
}

func chooseBiome(height int32, temperature, humidity, riverStrength float64) game.Biome {
	if height <= seaLevel-3 {
		return game.BiomeOcean
	}

	if riverStrength > 0.68 && height <= seaLevel+1 {
		return game.BiomeRiver
	}

	if height >= 108 {
		if temperature < 0.58 {
			return game.BiomeSnowyPlains
		}

		return game.BiomePlains
	}

	if temperature < 0.19 {
		return game.BiomeSnowyPlains
	}

	if temperature < 0.37 {
		return game.BiomeTaiga
	}

	if humidity > 0.76 && temperature > 0.4 && height <= seaLevel+6 {
		return game.BiomeSwamp
	}

	if temperature > 0.65 && humidity < 0.47 {
		return game.BiomeDesert
	}

	if humidity > 0.57 {
		return game.BiomeForest
	}

	return game.BiomePlains
}

func terrainBlockAt(seed int64, position game.BlockPosition, terrain column) game.Block {
	if position.Y < minimumY || position.Y > maximumY {
		return game.Air
	}

	if position.Y == minimumY {
		return game.Bedrock
	}

	if position.Y < minimumY+5 {
		bedrockChance := uint64(position.Y - minimumY)
		if coordinateHash(seed, int64(position.X), int64(position.Y), int64(position.Z), saltBedrock)%5 >= bedrockChance {
			return game.Bedrock
		}
	}

	if position.Y > terrain.height {
		if position.Y <= seaLevel {
			return game.Water
		}

		return game.Air
	}

	depth := terrain.height - position.Y

	if terrain.biome == game.BiomeOcean {
		if depth == 0 {
			return oceanFloorBlock(seed, position.X, position.Z)
		}

		if depth <= 3 {
			return game.Dirt
		}

		return game.Stone
	}

	if terrain.biome == game.BiomeRiver {
		if depth == 0 {
			if coordinateHash(seed, int64(position.X), 0, int64(position.Z), saltSurface)%5 == 0 {
				return game.Sand
			}

			return game.Gravel
		}

		if depth <= 3 {
			return game.Dirt
		}

		return game.Stone
	}

	if terrain.beach {
		if depth <= 3 {
			return game.Sand
		}

		if depth <= 6 {
			return game.Sandstone
		}

		return game.Stone
	}

	if terrain.biome == game.BiomeDesert {
		if depth <= 3 {
			return game.Sand
		}

		if depth <= 7 {
			return game.Sandstone
		}

		return game.Stone
	}

	if terrain.height >= 103 {
		if depth == 0 && coordinateHash(seed, int64(position.X), 0, int64(position.Z), saltSurface)%7 == 0 {
			return game.Gravel
		}

		return game.Stone
	}

	if depth == 0 {
		return game.GrassBlock
	}

	if depth <= 3 {
		return game.Dirt
	}

	return game.Stone
}

func oceanFloorBlock(seed int64, x, z int32) game.Block {
	choice := coordinateHash(seed, int64(x), 0, int64(z), saltSurface) % 12

	switch {
	case choice < 4:
		return game.Gravel
	case choice < 7:
		return game.Sand
	default:
		return game.Dirt
	}
}
