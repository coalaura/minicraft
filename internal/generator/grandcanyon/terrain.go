package grandcanyon

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

func columnAt(seed int64, worldX, worldZ int32) column {
	x := float64(worldX)
	z := float64(worldZ)

	warpX := fractalNoise(seed, x/1050, z/1050, 3, saltWarpX) * 220
	warpZ := fractalNoise(seed, x/1050, z/1050, 3, saltWarpZ) * 220

	warpedX := x + warpX
	warpedZ := z + warpZ

	regionalTilt := (warpedZ/2800 - warpedX/4600) * 11
	plateauMacro := fractalNoise(seed, warpedX/1600, warpedZ/1600, 4, saltPlateau) * 13
	plateauDetail := fractalNoise(seed, warpedX/280, warpedZ/280, 3, saltPlateauDetail) * 5

	plateauHeight := 174 + regionalTilt + plateauMacro + plateauDetail
	plateauHeight = math.Max(158, math.Min(194, plateauHeight))

	mainCenter := valueNoise(seed, warpedX/850, warpedZ/850, saltMainCanyon)
	mainCenter += fractalNoise(seed, warpedX/420, warpedZ/420, 2, saltMainDetail) * 0.17

	mainWidthNoise := fractalNoise(seed, warpedX/1100, warpedZ/1100, 2, saltMainWidth)
	mainWidth := 0.205 + (mainWidthNoise+1)*0.032

	promontoryNoise := ridgeNoise(seed, warpedX/95, warpedZ/95, 2, saltPromontory) * 0.015

	mainDistance := (math.Abs(mainCenter) + promontoryNoise) / mainWidth
	mainProfile := terracedProfile(mainDistance)

	floorNoise := fractalNoise(seed, warpedX/650, warpedZ/650, 2, saltFloor) * 3
	riverBed := 38.0 + floorNoise

	mainHeight := riverBed + (plateauHeight-riverBed)*mainProfile

	templeHeight := mainHeight

	if mainDistance >= 0.12 && mainDistance <= 0.78 && mainWidth > 0.20 {
		templeVor := voronoi2D(seed, warpedX/520, warpedZ/520, saltTemple)

		hashFactor := float64(templeVor.featureHash%100) * 0.01
		templeRadius := 0.20 + hashFactor*0.14

		if templeVor.distance < templeRadius {
			templeDist := templeVor.distance / templeRadius
			templeProf := 1.0 - terracedProfile(templeDist)

			templeDetail := fractalNoise(seed, warpedX/75, warpedZ/75, 2, saltTempleDetail) * 0.08
			templeProf = clamp01(templeProf + templeDetail*(1.0-templeDist))

			var summitElevation float64

			switch (templeVor.featureHash >> 8) % 3 {
			case 0:
				summitElevation = math.Min(plateauHeight-6, 172)
			case 1:
				summitElevation = 148
			default:
				summitElevation = 122
			}

			candidateTemple := mainHeight + (summitElevation-mainHeight)*templeProf
			templeHeight = math.Max(mainHeight, candidateTemple)
		}
	}

	mainHeight = math.Max(mainHeight, templeHeight)

	tribX := warpedX*0.88 + warpedZ*0.48
	tribZ := warpedZ*0.88 - warpedX*0.48

	tribCenter := valueNoise(seed, tribX/380, tribZ/380, saltTributary)
	tribCenter += fractalNoise(seed, tribX/190, tribZ/190, 2, saltTributaryDetail) * 0.14

	tribWidthNoise := fractalNoise(seed, warpedX/720, warpedZ/720, 2, saltTributaryWidth)
	tribWidth := 0.095 + (tribWidthNoise+1)*0.019

	tribDistance := math.Abs(tribCenter) / tribWidth
	tribProfile := terracedProfile(tribDistance)

	tribGateNoise := fractalNoise(seed, warpedX/1100, warpedZ/1100, 3, saltTributaryGate)
	tribGate := smoothstep(-0.35, 0.25, tribGateNoise)

	tribReach := 1.0 - smoothstep(1.1, 4.2, mainDistance)
	tribGate *= 0.24 + tribReach*0.76

	tribFloor := 74.0 + floorNoise*4
	tribHeight := tribFloor + (plateauHeight-tribFloor)*tribProfile
	tribHeight = lerp(plateauHeight, tribHeight, tribGate)

	height := math.Min(plateauHeight, math.Min(mainHeight, tribHeight))

	mainStrength := clamp01(1 - mainProfile)
	tribStrength := clamp01((1 - tribProfile) * tribGate)
	canyonStrength := math.Max(mainStrength, tribStrength)

	gullyField := math.Abs(fractalNoise(seed, tribX/130, tribZ/130, 3, saltGully))
	gullyStrength := 1.0 - smoothstep(0.012, 0.065, gullyField)

	gullyBand := smoothstep(0.12, 0.44, canyonStrength)
	gullyBand *= 1.0 - smoothstep(0.78, 0.98, canyonStrength)

	height -= gullyStrength * gullyBand * 20

	riverStrength := 1.0 - smoothstep(0.032, 0.115, mainDistance)
	riverStrength = clamp01(riverStrength)

	height = math.Max(35, math.Min(196, height))
	surfaceHeight := int32(math.Round(height))

	strataDip := int32((warpedX*0.002 - warpedZ*0.0015))
	strataNoise := fractalNoise(seed, warpedX/220, warpedZ/220, 2, saltStrata)
	strataOffset := strataDip + int32(math.Round(strataNoise*4))

	isRiverBed := riverStrength > 0.52 && surfaceHeight <= riverLevel
	terraceBench := (surfaceHeight >= 68 && surfaceHeight <= 78) || (surfaceHeight >= 138 && surfaceHeight <= 148)
	slope := int32(0)
	talusStrength := 0.0

	if canyonStrength > 0.15 && canyonStrength < 0.88 {
		if (surfaceHeight >= 66 && surfaceHeight <= 84) || (surfaceHeight >= 134 && surfaceHeight <= 146) {
			talusStrength = 0.65
		}
	}

	biome := game.BiomeBadlands

	if canyonStrength > 0.22 {
		biome = game.BiomeErodedBadlands
	}

	if riverStrength > 0.55 && surfaceHeight <= riverLevel+1 {
		biome = game.BiomeRiver
	}

	return column{
		height:         surfaceHeight,
		plateauHeight:  int32(math.Round(plateauHeight)),
		strataOffset:   strataOffset,
		slope:          slope,
		terraceBench:   terraceBench,
		isRiverBed:     isRiverBed,
		biome:          biome,
		canyonStrength: canyonStrength,
		riverStrength:  riverStrength,
		talusStrength:  talusStrength,
	}
}

func terracedProfile(distance float64) float64 {
	if distance <= 0 {
		return 0
	}

	if distance >= 1 {
		return 1
	}

	profile := 0.12 * smoothstep(0.040, 0.080, distance)
	profile += 0.08 * smoothstep(0.120, 0.210, distance)
	profile += 0.26 * smoothstep(0.240, 0.360, distance)
	profile += 0.14 * smoothstep(0.420, 0.530, distance)
	profile += 0.09 * smoothstep(0.560, 0.670, distance)
	profile += 0.21 * smoothstep(0.700, 0.860, distance)
	profile += 0.10 * smoothstep(0.880, 0.980, distance)

	return clamp01(profile)
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
		hash := coordinateHash(
			seed,
			int64(position.X),
			int64(position.Y),
			int64(position.Z),
			saltBedrock,
		)

		if hash%5 >= bedrockChance {
			return game.Bedrock
		}
	}

	if position.Y > terrain.height {
		if terrain.riverStrength > 0.46 && terrain.height < riverLevel && position.Y <= riverLevel {
			return game.Water
		}

		return game.Air
	}

	depth := terrain.height - position.Y

	if terrain.isRiverBed || (terrain.riverStrength > 0.48 && terrain.height <= riverLevel+1) {
		if depth == 0 {
			hash := coordinateHash(
				seed,
				int64(position.X),
				0,
				int64(position.Z),
				saltSurface,
			)

			switch hash % 6 {
			case 0, 1:
				return palette.redSand
			case 2:
				return palette.mud
			default:
				return palette.gravel
			}
		}

		if depth <= 2 {
			return palette.gravel
		}
	}

	if terrain.talusStrength > 0.4 && depth <= 2 {
		hash := coordinateHash(
			seed,
			int64(position.X),
			int64(position.Y),
			int64(position.Z),
			saltSurface,
		)

		if hash%4 == 0 {
			return palette.gravel
		}

		if hash%3 == 0 {
			return palette.coarseDirt
		}

		if hash%5 == 0 {
			return palette.redSandstone
		}

		return palette.redSand
	}

	if terrain.canyonStrength < 0.18 && depth <= 2 {
		hash := coordinateHash(
			seed,
			int64(position.X),
			0,
			int64(position.Z),
			saltSurface,
		)

		if hash%17 == 0 {
			return palette.coarseDirt
		}

		if hash%23 == 0 {
			return palette.sandstone
		}

		return palette.redSand
	}

	if depth == 0 && terrain.canyonStrength < 0.35 {
		hash := coordinateHash(
			seed,
			int64(position.X),
			0,
			int64(position.Z),
			saltSurface,
		)

		if hash%11 == 0 {
			return palette.redSandstone
		}

		if hash%19 == 0 {
			return palette.coarseDirt
		}
	}

	sublayerHash := coordinateHash(
		seed,
		int64(position.X),
		int64(position.Y),
		int64(position.Z),
		saltStrata,
	)

	return strataBlockAt(
		position.Y+terrain.strataOffset,
		sublayerHash,
	)
}
