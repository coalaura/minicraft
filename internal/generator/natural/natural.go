package natural

import (
	"math"
	"math/bits"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "natural"

	seaLevel      = int32(63)
	minimumY      = int32(-64)
	maximumY      = int32(175)
	treeCellSize  = int32(8)
	maxTreeRadius = int32(2)
	maxTreeHeight = int32(9)
)

const (
	treeNone treeKind = iota
	treeOak
	treeSpruce
)

const (
	saltWarpX       uint64 = 0x9e3779b97f4a7c15
	saltWarpZ       uint64 = 0xbf58476d1ce4e5b9
	saltContinents  uint64 = 0x94d049bb133111eb
	saltElevation   uint64 = 0xd6e8feb86659fd93
	saltMountains   uint64 = 0xa0761d6478bd642f
	saltRidges      uint64 = 0xe7037ed1a0b428db
	saltRivers      uint64 = 0x8ebc6af09c88c6e3
	saltTemperature uint64 = 0x589965cc75374cc3
	saltHumidity    uint64 = 0x1d8e4e27c47d124f
	saltSurface     uint64 = 0xeb44accab455d165
	saltDecor       uint64 = 0x9c06faf4d023e3ab
	saltTree        uint64 = 0xc2b2ae3d27d4eb4f
	saltBedrock     uint64 = 0x165667b19e3779f9
)

type Generator struct{}

type column struct {
	height        int32
	biome         game.Biome
	temperature   float64
	humidity      float64
	riverStrength float64
	beach         bool
}

type treeKind uint8

type tree struct {
	x      int32
	z      int32
	baseY  int32
	height int32
	kind   treeKind
}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y < minimumY || position.Y > maximumY {
		return game.Air
	}

	terrain := columnAt(seed, position.X, position.Z)
	block := terrainBlockAt(seed, position, terrain)
	if block != game.Air {
		return block
	}

	if feature, ok := treeFeatureAt(seed, position); ok {
		return feature
	}

	if feature, ok := cactusFeatureAt(seed, position, terrain); ok {
		return feature
	}

	if feature, ok := surfaceDecorationAt(seed, position, terrain); ok {
		return feature
	}

	return game.Air
}

func (Generator) BiomeAt(seed int64, x, z int32) game.Biome {
	return columnAt(seed, x, z).biome
}

func (Generator) GenerateSection(seed int64, chunkPosition game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < minimumY || sectionMinY > maximumY {
		return game.Air, true
	}

	if sectionMinY >= minimumY+game.ChunkWidth && sectionMaxY <= 31 {
		return game.Stone, true
	}

	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth

	var columns [game.ChunkWidth * game.ChunkWidth]column

	minHeight := int32(math.MaxInt32)
	maxHeight := int32(math.MinInt32)

	for localZ := range int32(game.ChunkWidth) {
		for localX := range int32(game.ChunkWidth) {
			terrain := columnAt(seed, chunkMinX+localX, chunkMinZ+localZ)
			columns[localZ*game.ChunkWidth+localX] = terrain
			minHeight = min(minHeight, terrain.height)
			maxHeight = max(maxHeight, terrain.height)
		}
	}

	if sectionMinY > max(maxHeight, seaLevel)+maxTreeHeight+2 {
		return game.Air, true
	}

	if sectionMaxY < minHeight-7 && sectionMinY > minimumY+4 {
		return game.Stone, true
	}

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				index := localY*256 + localZ*16 + localX
				position := game.BlockPosition{
					X: chunkMinX + localX,
					Y: worldY,
					Z: chunkMinZ + localZ,
				}

				blocks[index] = terrainBlockAt(seed, position, columns[localZ*game.ChunkWidth+localX])
			}
		}
	}

	applyTrees(seed, chunkPosition, sectionMinY, blocks)
	applyColumnFeatures(seed, chunkPosition, sectionMinY, &columns, blocks)

	first := blocks[0]
	for _, block := range blocks[1:] {
		if block != first {
			return game.Air, false
		}
	}

	return first, true
}

func (Generator) GenerationBounds(_ int64, _ game.ChunkPosition) (int32, int32, bool) {
	return minimumY, maximumY, true
}

func (Generator) Spawn(seed int64) game.Position {
	const searchRadius = int32(192)

	bestX := int32(0)
	bestZ := int32(0)
	bestScore := math.MaxFloat64

	for radius := int32(0); radius <= searchRadius; radius += 8 {
		for z := -radius; z <= radius; z += 8 {
			for _, x := range []int32{-radius, radius} {
				if spawnScore := scoreSpawn(seed, x, z); spawnScore < bestScore {
					bestX = x
					bestZ = z
					bestScore = spawnScore
				}
			}
		}

		for x := -radius + 8; x < radius; x += 8 {
			for _, z := range []int32{-radius, radius} {
				if spawnScore := scoreSpawn(seed, x, z); spawnScore < bestScore {
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

func newRegistered() (game.Generator, error) {
	return New(), nil
}

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

		return game.SpruceLeaves, false, true
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

	return game.OakLeaves, false, true
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

func applyTrees(seed int64, chunkPosition game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) {
	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth
	chunkMaxX := chunkMinX + game.ChunkWidth - 1
	chunkMaxZ := chunkMinZ + game.ChunkWidth - 1
	sectionMaxY := sectionMinY + game.ChunkWidth - 1

	minCellX := floorDiv(chunkMinX-maxTreeRadius, treeCellSize)
	maxCellX := floorDiv(chunkMaxX+maxTreeRadius, treeCellSize)
	minCellZ := floorDiv(chunkMinZ-maxTreeRadius, treeCellSize)
	maxCellZ := floorDiv(chunkMaxZ+maxTreeRadius, treeCellSize)

	for cellZ := minCellZ; cellZ <= maxCellZ; cellZ++ {
		for cellX := minCellX; cellX <= maxCellX; cellX++ {
			candidate, ok := treeForCell(seed, cellX, cellZ)
			if !ok {
				continue
			}

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

				if block, ok := cactusFeatureAt(seed, position, terrain); ok {
					blocks[index] = block
					continue
				}

				if block, ok := surfaceDecorationAt(seed, position, terrain); ok {
					blocks[index] = block
				}
			}
		}
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
	if block, ok := treeFeatureAt(seed, position); ok && block != game.OakLeaves && block != game.SpruceLeaves {
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

func fractalNoise(seed int64, x, z float64, octaves int, salt uint64) float64 {
	amplitude := 1.0
	frequency := 1.0
	total := 0.0
	weight := 0.0

	for octave := 0; octave < octaves; octave++ {
		octaveSalt := salt + uint64(octave)*0x9e3779b97f4a7c15
		total += valueNoise(seed, x*frequency, z*frequency, octaveSalt) * amplitude
		weight += amplitude
		frequency *= 2
		amplitude *= 0.5
	}

	return total / weight
}

func valueNoise(seed int64, x, z float64, salt uint64) float64 {
	x0 := int64(math.Floor(x))
	z0 := int64(math.Floor(z))
	x1 := x0 + 1
	z1 := z0 + 1

	tx := fade(x - float64(x0))
	tz := fade(z - float64(z0))

	a := latticeValue(seed, x0, z0, salt)
	b := latticeValue(seed, x1, z0, salt)
	c := latticeValue(seed, x0, z1, salt)
	d := latticeValue(seed, x1, z1, salt)

	top := lerp(a, b, tx)
	bottom := lerp(c, d, tx)

	return lerp(top, bottom, tz)
}

func latticeValue(seed int64, x, z int64, salt uint64) float64 {
	hash := coordinateHash(seed, x, 0, z, salt)
	unit := float64(hash>>11) * (1.0 / (1 << 53))

	return unit*2 - 1
}

func coordinateHash(seed int64, x, y, z int64, salt uint64) uint64 {
	hash := uint64(seed) ^ salt
	hash ^= uint64(x) * 0x9e3779b97f4a7c15
	hash = bits.RotateLeft64(hash, 21)
	hash ^= uint64(y) * 0xbf58476d1ce4e5b9
	hash = bits.RotateLeft64(hash, 17)
	hash ^= uint64(z) * 0x94d049bb133111eb

	return mix64(hash)
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}

func fade(value float64) float64 {
	return value * value * value * (value*(value*6-15) + 10)
}

func smoothstep(edge0, edge1, value float64) float64 {
	if edge0 == edge1 {
		return 0
	}

	t := clamp01((value - edge0) / (edge1 - edge0))

	return t * t * (3 - 2*t)
}

func lerp(first, second, amount float64) float64 {
	return first + (second-first)*amount
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor
	if value%divisor < 0 {
		quotient--
	}

	return quotient
}

func abs32(value int32) int32 {
	if value < 0 {
		return -value
	}

	return value
}
