package quasicrystal

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "quasicrystal"

	foundationMinY = int32(62)
	surfaceY       = int32(64)
	maxBuildY      = int32(112)

	gridSpacing        = 38.0
	pathHalfWidth      = 1.6
	pathCenterWidth    = 0.38
	fieldWavelength    = 31.0
	plazaRadius        = 4.0
	spawnPlazaRadius   = 9.0
	maxStructureHeight = int32(48)

	twoPi = math.Pi * 2
)

type direction struct {
	x float64
	z float64
}

type lineSample struct {
	index    int64
	position float64
	distance float64
	along    float64
}

type nodeDescription struct {
	found    bool
	distance float64
	hash     uint64
	height   int32
}

type columnDescription struct {
	field        float64
	path         bool
	pathFamily   int
	pathDistance float64
	pathAlong    float64
	reliefHeight int32
	spawnPlaza   bool
	node         nodeDescription
}

type Generator struct{}

var directions = [...]direction{
	{x: 1.0, z: 0.0},
	{x: 0.30901699437494745, z: 0.9510565162951535},
	{x: -0.8090169943749473, z: 0.5877852522924732},
	{x: -0.8090169943749476, z: -0.587785252292473},
	{x: 0.30901699437494723, z: -0.9510565162951536},
}

var (
	_ game.Generator        = Generator{}
	_ game.SectionGenerator = Generator{}
	_ game.BoundedGenerator = Generator{}
	_ game.SpawnGenerator   = Generator{}
)

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y < foundationMinY || position.Y > maxBuildY {
		return game.Air
	}

	description := describeColumn(seed, position.X, position.Z)

	return blockForColumn(seed, position.X, position.Y, position.Z, description)
}

func (Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < foundationMinY || sectionMinY > maxBuildY {
		return game.Air, true
	}

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	var columns [game.ChunkWidth * game.ChunkWidth]columnDescription

	for localZ := range int32(game.ChunkWidth) {
		worldZ := chunkMinZ + localZ

		for localX := range int32(game.ChunkWidth) {
			worldX := chunkMinX + localX
			columns[localZ*game.ChunkWidth+localX] = describeColumn(seed, worldX, worldZ)
		}
	}

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			worldZ := chunkMinZ + localZ

			for localX := range int32(game.ChunkWidth) {
				worldX := chunkMinX + localX
				description := columns[localZ*game.ChunkWidth+localX]
				block := blockForColumn(seed, worldX, worldY, worldZ, description)
				index := localY*256 + localZ*16 + localX
				blocks[index] = block

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
	return foundationMinY, maxBuildY, true
}

func (Generator) Spawn(_ int64) game.Position {
	return game.Position{
		X: 0.5,
		Y: float64(surfaceY) + 1,
		Z: 0.5,
	}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func describeColumn(seed int64, worldX, worldZ int32) columnDescription {
	pointX := float64(worldX) + 0.5
	pointZ := float64(worldZ) + 0.5

	if pointX*pointX+pointZ*pointZ <= spawnPlazaRadius*spawnPlazaRadius {
		return columnDescription{spawnPlaza: true}
	}

	var samples [len(directions)]lineSample

	closestFamily := 0
	closestDistance := math.MaxFloat64

	for family, axis := range directions {
		projection := pointX*axis.x + pointZ*axis.z
		phase := gridPhase(seed, family)
		lineIndex := int64(math.Round((projection - phase) / gridSpacing))
		linePosition := float64(lineIndex)*gridSpacing + phase
		distance := math.Abs(projection - linePosition)
		along := -pointX*axis.z + pointZ*axis.x

		samples[family] = lineSample{
			index:    lineIndex,
			position: linePosition,
			distance: distance,
			along:    along,
		}

		if distance < closestDistance {
			closestFamily = family
			closestDistance = distance
		}
	}

	node := nearestNode(seed, pointX, pointZ, samples)
	field := quasicrystalField(seed, pointX, pointZ)

	path := closestDistance <= pathHalfWidth

	description := columnDescription{
		field:        field,
		path:         path,
		pathFamily:   closestFamily,
		pathDistance: closestDistance,
		pathAlong:    samples[closestFamily].along,
		node:         node,
	}

	if !path && !node.found {
		description.reliefHeight = reliefHeight(field)
	}

	return description
}

func nearestNode(seed int64, pointX, pointZ float64, samples [len(directions)]lineSample) nodeDescription {
	closestDistanceSquared := plazaRadius * plazaRadius
	closest := nodeDescription{}

	for firstFamily := 0; firstFamily < len(directions); firstFamily++ {
		firstAxis := directions[firstFamily]
		firstSample := samples[firstFamily]

		for secondFamily := firstFamily + 1; secondFamily < len(directions); secondFamily++ {
			secondAxis := directions[secondFamily]
			secondSample := samples[secondFamily]

			determinant := firstAxis.x*secondAxis.z - firstAxis.z*secondAxis.x

			centerX := (firstSample.position*secondAxis.z - firstAxis.z*secondSample.position) / determinant
			centerZ := (firstAxis.x*secondSample.position - firstSample.position*secondAxis.x) / determinant

			deltaX := pointX - centerX
			deltaZ := pointZ - centerZ

			distanceSquared := deltaX*deltaX + deltaZ*deltaZ
			if distanceSquared > closestDistanceSquared {
				continue
			}

			hash := nodeHash(seed, firstFamily, secondFamily, firstSample.index, secondSample.index)

			closestDistanceSquared = distanceSquared

			closest = nodeDescription{
				found:    true,
				distance: math.Sqrt(distanceSquared),
				hash:     hash,
				height:   structureHeight(hash),
			}
		}
	}

	return closest
}

func blockForColumn(seed int64, worldX, worldY, worldZ int32, description columnDescription) game.Block {
	if worldY < foundationMinY || worldY > maxBuildY {
		return game.Air
	}

	if worldY == foundationMinY {
		return game.Stone
	}

	if worldY == foundationMinY+1 {
		if ((worldX>>2)+(worldZ>>2))&1 == 0 {
			return game.PolishedAndesite
		}

		return game.BlackConcrete
	}

	if worldY == surfaceY {
		return surfaceBlock(seed, worldX, worldZ, description)
	}

	if worldY < surfaceY {
		return game.Air
	}

	if block := nodeStructureBlock(worldY, description.node); block != game.Air {
		return block
	}

	if description.reliefHeight == 0 || worldY > surfaceY+description.reliefHeight {
		return game.Air
	}

	if worldY == surfaceY+description.reliefHeight {
		return fieldSurfaceBlock(description.field)
	}

	if description.field >= 0 {
		return game.PurpurBlock
	}

	return game.DarkPrismarine
}

func surfaceBlock(seed int64, worldX, worldZ int32, description columnDescription) game.Block {
	if description.spawnPlaza {
		return spawnSurfaceBlock(worldX, worldZ)
	}

	if description.node.found {
		if description.node.distance >= plazaRadius-0.8 {
			return game.GoldBlock
		}

		if description.node.distance <= 0.85 {
			return game.SeaLantern
		}

		return game.SmoothQuartz
	}

	if description.path {
		if description.pathDistance <= pathCenterWidth {
			if pathHasLight(seed, description.pathFamily, description.pathAlong) {
				return game.SeaLantern
			}

			return pathAccentBlock(description.pathFamily)
		}

		return game.SmoothQuartz
	}

	return fieldSurfaceBlock(description.field)
}

func spawnSurfaceBlock(worldX, worldZ int32) game.Block {
	pointX := float64(worldX) + 0.5
	pointZ := float64(worldZ) + 0.5

	distance := math.Hypot(pointX, pointZ)

	if distance >= spawnPlazaRadius-0.85 {
		return game.GoldBlock
	}

	if distance <= 1.0 {
		return game.SeaLantern
	}

	if distanceToCentralRay(pointX, pointZ) <= 0.42 {
		return game.GoldBlock
	}

	return game.SmoothQuartz
}

func nodeStructureBlock(worldY int32, node nodeDescription) game.Block {
	if !node.found || node.height == 0 {
		return game.Air
	}

	relativeY := worldY - surfaceY
	if relativeY <= 0 || relativeY > node.height {
		return game.Air
	}

	progress := float64(relativeY-1) / float64(max(node.height-1, 1))

	baseRadius := 2.4
	if node.hash%23 == 0 {
		baseRadius = 3.0
	}

	outerRadius := baseRadius - progress*(baseRadius-0.72)
	if node.distance > outerRadius {
		return game.Air
	}

	if node.distance <= 0.72 {
		if relativeY%5 == 0 || relativeY == node.height {
			return game.SeaLantern
		}

		return game.PurpurBlock
	}

	if relativeY == 1 && node.hash%23 == 0 {
		return game.GoldBlock
	}

	if (relativeY+int32(node.hash&1))&1 == 0 {
		return game.PurpleStainedGlass
	}

	return game.CyanStainedGlass
}

func fieldSurfaceBlock(field float64) game.Block {
	switch {
	case field >= 2.25:
		return game.PurpurBlock
	case field >= 0.85:
		return game.EndStoneBricks
	case field >= -0.65:
		return game.GrayConcrete
	case field >= -2.0:
		return game.BlackConcrete
	default:
		return game.Obsidian
	}
}

func pathAccentBlock(family int) game.Block {
	switch family {
	case 0:
		return game.CyanConcrete
	case 1:
		return game.PurpurBlock
	case 2:
		return game.GoldBlock
	case 3:
		return game.PrismarineBricks
	default:
		return game.LightBlueStainedGlass
	}
}

func distanceToCentralRay(pointX, pointZ float64) float64 {
	closest := math.MaxFloat64

	for _, axis := range directions {
		distance := math.Abs(pointX*axis.x + pointZ*axis.z)
		if distance < closest {
			closest = distance
		}
	}

	return closest
}

func reliefHeight(field float64) int32 {
	strength := math.Abs(field)
	if strength < 2.35 {
		return 0
	}

	height := int32(1 + (strength-2.35)*1.45)
	if height > 4 {
		return 4
	}

	return height
}

func structureHeight(hash uint64) int32 {
	if hash%23 == 0 {
		return maxStructureHeight
	}

	if hash%4 != 0 {
		return 0
	}

	return 18 + int32((hash>>8)%17)
}

func pathHasLight(seed int64, family int, along float64) bool {
	segment := int64(math.Floor(along / 11.0))
	hash := mix64(uint64(seed) ^ uint64(family+1)*0x9e3779b97f4a7c15 ^ uint64(segment))

	return hash%5 == 0
}

func quasicrystalField(seed int64, pointX, pointZ float64) float64 {
	frequency := twoPi / fieldWavelength
	field := 0.0

	for family, axis := range directions {
		projection := pointX*axis.x + pointZ*axis.z
		phase := fieldPhase(seed, family)
		field += math.Cos(projection*frequency + phase)
	}

	return field
}

func gridPhase(seed int64, family int) float64 {
	return unitHash(seed, family, 0x517cc1b727220a95) * gridSpacing
}

func fieldPhase(seed int64, family int) float64 {
	return unitHash(seed, family, 0x94d049bb133111eb) * twoPi
}

func unitHash(seed int64, family int, salt uint64) float64 {
	hash := mix64(uint64(seed) ^ uint64(family+1)*0x9e3779b97f4a7c15 ^ salt)

	return float64(hash>>11) / float64(uint64(1)<<53)
}

func nodeHash(seed int64, firstFamily, secondFamily int, firstLine, secondLine int64) uint64 {
	hash := mix64(uint64(seed) ^ 0x243f6a8885a308d3)
	hash = mix64(hash ^ uint64(firstFamily+1)*0x9e3779b97f4a7c15)
	hash = mix64(hash ^ uint64(secondFamily+1)*0xbf58476d1ce4e5b9)
	hash = mix64(hash ^ uint64(firstLine)*0x94d049bb133111eb)

	return mix64(hash ^ uint64(secondLine)*0xd6e8feb86659fd93)
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}
