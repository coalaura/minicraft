package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "backrooms"

	foundationY    = int32(62)
	floorY         = int32(63)
	normalCeilingY = int32(67)
	ceilingY       = int32(68)

	zoneSize          = int64(64)
	paletteRegionSize = int64(3)
)

const (
	saltZone    uint64 = 0x8ca19d7a4584f1d7
	saltPalette uint64 = 0x15f59d0ed05fd4ab
	saltEdge    uint64 = 0xd75a95d152ed83b9
	saltWall    uint64 = 0x4f1bbcdc6768a3d1
	saltFloor   uint64 = 0xc24b8b70d0f89791
	saltLight   uint64 = 0x9a4275c4e7aa8273
	saltDetail  uint64 = 0x72df8e29ec5d4db3
	saltMotif   uint64 = 0xa63c21e6bf41f357
)

type layout uint8

const (
	layoutClassic layout = iota
	layoutMaze
	layoutLongHalls
	layoutCrossroads
	layoutCubicles
	layoutPillars
	layoutSparse
)

type palette uint8

const (
	paletteClassic palette = iota
	paletteFaded
	paletteOffice
	paletteMaintenance
)

type structure uint8

const (
	structureOpen structure = iota
	structureBulkhead
	structureDoorway
	structurePartition
	structureWall
	structurePillar
)

type paletteBlocks struct {
	wall    game.Block
	trim    game.Block
	accent  game.Block
	floor   game.Block
	wear    game.Block
	stain   game.Block
	ceiling game.Block
	light   game.Block
	broken  game.Block
}

type zone struct {
	x       int64
	z       int64
	localX  int64
	localZ  int64
	hash    uint64
	layout  layout
	palette palette
}

type openingSpec struct {
	hash    uint64
	centerA int64
	centerB int64
	widthA  int64
	widthB  int64
}

type column struct {
	worldX    int64
	worldZ    int64
	zone      zone
	blocks    paletteBlocks
	structure structure
}

type Generator struct{}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	if position.Y < foundationY || position.Y > ceilingY {
		return game.Air
	}

	worldX := int64(position.X)
	worldZ := int64(position.Z)

	current := zoneAt(seed, worldX, worldZ)
	blocks := blocksForPalette(current.palette)
	profile := structureAt(seed, current)

	return blockAtColumn(seed, worldX, int64(position.Y), worldZ, current, blocks, profile)
}

func blockAtColumn(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks, profile structure) game.Block {
	currentCeilingY := zoneCeilingY(current)
	if worldY < int64(foundationY) || worldY > int64(currentCeilingY) {
		return game.Air
	}

	switch int32(worldY) {
	case foundationY:
		return foundationBlock(current.palette)
	case floorY:
		return floorBlock(seed, worldX, worldZ, current, blocks)
	case currentCeilingY:
		return ceilingBlock(seed, worldX, worldZ, current, blocks)
	}

	return structureBlock(seed, worldX, worldY, worldZ, current, blocks, profile)
}

func (Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, output *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < foundationY || sectionMinY > ceilingY {
		return game.Air, true
	}

	var columns [game.ChunkWidth * game.ChunkWidth]column

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	for localZ := range int32(game.ChunkWidth) {
		for localX := range int32(game.ChunkWidth) {
			worldX := int64(chunkMinX + localX)
			worldZ := int64(chunkMinZ + localZ)
			current := zoneAt(seed, worldX, worldZ)

			columns[localZ*game.ChunkWidth+localX] = column{
				worldX:    worldX,
				worldZ:    worldZ,
				zone:      current,
				blocks:    blocksForPalette(current.palette),
				structure: structureAt(seed, current),
			}
		}
	}

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		worldY := int64(sectionMinY + localY)

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				column := columns[localZ*game.ChunkWidth+localX]
				block := blockAtColumn(
					seed,
					column.worldX,
					worldY,
					column.worldZ,
					column.zone,
					column.blocks,
					column.structure,
				)

				index := localY*256 + localZ*16 + localX
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
	return foundationY, ceilingY, true
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

func zoneAt(seed, worldX, worldZ int64) zone {
	zoneX, localX := zoneCoordinate(worldX)
	zoneZ, localZ := zoneCoordinate(worldZ)
	hash := coordinateHash(seed, zoneX, zoneZ, saltZone)

	paletteX := floorDiv(zoneX, paletteRegionSize)
	paletteZ := floorDiv(zoneZ, paletteRegionSize)
	paletteHash := coordinateHash(seed, paletteX, paletteZ, saltPalette)

	return zone{
		x:       zoneX,
		z:       zoneZ,
		localX:  localX,
		localZ:  localZ,
		hash:    hash,
		layout:  layoutForHash(hash),
		palette: paletteForHash(paletteHash),
	}
}

func layoutForHash(hash uint64) layout {
	switch hash % 100 {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
		10, 11, 12, 13, 14, 15, 16, 17,
		18, 19, 20, 21, 22, 23, 24, 25,
		26, 27, 28, 29:
		return layoutClassic
	case 30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
		40, 41, 42, 43, 44:
		return layoutMaze
	case 45, 46, 47, 48, 49, 50, 51, 52, 53, 54,
		55, 56, 57, 58, 59:
		return layoutLongHalls
	case 60, 61, 62, 63, 64, 65, 66, 67, 68, 69:
		return layoutCrossroads
	case 70, 71, 72, 73, 74, 75, 76, 77, 78, 79,
		80, 81:
		return layoutCubicles
	case 82, 83, 84, 85, 86, 87, 88, 89, 90, 91:
		return layoutPillars
	default:
		return layoutSparse
	}
}

func paletteForHash(hash uint64) palette {
	switch hash % 16 {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10:
		return paletteClassic
	case 11, 12, 13:
		return paletteFaded
	case 14:
		return paletteOffice
	default:
		return paletteMaintenance
	}
}

func blocksForPalette(selected palette) paletteBlocks {
	switch selected {
	case paletteFaded:
		return paletteBlocks{
			wall:    game.SmoothSandstone,
			trim:    game.Sandstone,
			accent:  game.BrownTerracotta,
			floor:   game.LightGrayWool,
			wear:    game.GrayWool,
			stain:   game.BrownWool,
			ceiling: game.LightGrayTerracotta,
			light:   game.OchreFroglight,
			broken:  game.BrownTerracotta,
		}
	case paletteOffice:
		return paletteBlocks{
			wall:    game.WhiteTerracotta,
			trim:    game.LightGrayTerracotta,
			accent:  game.SmoothQuartz,
			floor:   game.GrayWool,
			wear:    game.LightGrayWool,
			stain:   game.BrownWool,
			ceiling: game.SmoothQuartz,
			light:   game.SeaLantern,
			broken:  game.LightGrayTerracotta,
		}
	case paletteMaintenance:
		return paletteBlocks{
			wall:    game.LightGrayTerracotta,
			trim:    game.StoneBricks,
			accent:  game.SmoothStone,
			floor:   game.GrayWool,
			wear:    game.LightGrayWool,
			stain:   game.BlackWool,
			ceiling: game.SmoothStone,
			light:   game.SeaLantern,
			broken:  game.GrayWool,
		}
	default:
		return paletteBlocks{
			wall:    game.SmoothSandstone,
			trim:    game.CutSandstone,
			accent:  game.Sandstone,
			floor:   game.LightGrayWool,
			wear:    game.GrayWool,
			stain:   game.BrownWool,
			ceiling: game.WhiteTerracotta,
			light:   game.OchreFroglight,
			broken:  game.LightGrayTerracotta,
		}
	}
}

func zoneCeilingY(current zone) int32 {
	switch current.layout {
	case layoutPillars:
		return ceilingY
	case layoutCrossroads, layoutSparse:
		if current.hash&(1<<52) != 0 {
			return ceilingY
		}
	}

	return normalCeilingY
}

func foundationBlock(selected palette) game.Block {
	if selected == paletteMaintenance {
		return game.StoneBricks
	}

	return game.SmoothStone
}

func floorBlock(seed, worldX, worldZ int64, current zone, blocks paletteBlocks) game.Block {
	patchX := floorDiv(worldX, 8)
	patchZ := floorDiv(worldZ, 8)
	patchHash := coordinateHash(seed, patchX, patchZ, saltFloor^uint64(current.palette))

	if patchHash%17 == 0 {
		localX := floorMod(worldX, 8)
		localZ := floorMod(worldZ, 8)
		centerX := int64(1 + (patchHash>>8)%6)
		centerZ := int64(1 + (patchHash>>16)%6)
		radius := int64(2 + (patchHash>>24)%2)

		deltaX := localX - centerX
		deltaZ := localZ - centerZ
		edgeHash := coordinateHash(seed, worldX, worldZ, saltFloor^0x62f4e987ac113da5)
		edgeJitter := int64(edgeHash%5) - 2

		if deltaX*deltaX+deltaZ*deltaZ <= radius*radius+edgeJitter {
			return blocks.stain
		}
	}

	wearHash := coordinateHash(seed, floorDiv(worldX, 2), floorDiv(worldZ, 2), saltFloor^0xe4fb9c31a7695d27)
	if wearHash%41 == 0 {
		return blocks.wear
	}

	return blocks.floor
}

func ceilingBlock(seed, worldX, worldZ int64, current zone, blocks paletteBlocks) game.Block {
	if !lightFixtureAt(current) {
		return blocks.ceiling
	}

	fixtureX := floorDiv(worldX, 4)
	fixtureZ := floorDiv(worldZ, 4)
	hash := coordinateHash(seed, fixtureX, fixtureZ, saltLight^uint64(current.layout))

	if hash%17 == 0 {
		return blocks.broken
	}

	return blocks.light
}

func lightFixtureAt(current zone) bool {
	phaseX := int64((current.hash >> 8) & 7)
	phaseZ := int64((current.hash >> 16) & 7)

	switch current.layout {
	case layoutLongHalls:
		vertical, corridorCenter, _ := longHallParameters(current)
		if vertical {
			return abs64(current.localX-corridorCenter) <= 1 && floorMod(current.localZ+phaseZ, 9) == 4
		}

		return abs64(current.localZ-corridorCenter) <= 1 && floorMod(current.localX+phaseX, 9) == 4
	case layoutCrossroads:
		return floorMod(current.localX+phaseX, 10) >= 4 &&
			floorMod(current.localX+phaseX, 10) <= 6 &&
			floorMod(current.localZ+phaseZ, 8) == 4
	case layoutPillars, layoutSparse:
		return floorMod(current.localX+phaseX, 12) >= 5 &&
			floorMod(current.localX+phaseX, 12) <= 6 &&
			floorMod(current.localZ+phaseZ, 11) == 5
	default:
		return floorMod(current.localX+phaseX, 8) >= 3 &&
			floorMod(current.localX+phaseX, 8) <= 5 &&
			floorMod(current.localZ+phaseZ, 8) == 4
	}
}

func structureBlock(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks, profile structure) game.Block {
	currentCeilingY := zoneCeilingY(current)

	switch profile {
	case structureWall, structurePillar:
		return wallBlock(seed, worldX, worldY, worldZ, current, blocks)
	case structureDoorway:
		if int32(worldY) == currentCeilingY-1 {
			return wallBlock(seed, worldX, worldY, worldZ, current, blocks)
		}
	case structurePartition:
		if int32(worldY) == floorY+1 {
			return blocks.wall
		}

		if int32(worldY) == floorY+2 {
			return blocks.trim
		}
	case structureBulkhead:
		if int32(worldY) == currentCeilingY-1 {
			return blocks.trim
		}
	}

	return game.Air
}

func wallBlock(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks) game.Block {
	if worldY == int64(floorY+1) {
		return blocks.trim
	}

	patchX := floorDiv(worldX, 3)
	patchZ := floorDiv(worldZ, 3)
	hash := coordinateHash(seed, patchX, patchZ, saltDetail^uint64(worldY))

	if hash%149 == 0 {
		return blocks.accent
	}

	return blocks.wall
}

func structureAt(seed int64, current zone) structure {
	if boundary, found := zoneBoundaryStructureAt(seed, current); found {
		return boundary
	}

	if zoneSpineOpenAt(seed, current) {
		return structureOpen
	}

	base := layoutStructureAt(seed, current)
	motif := motifStructureAt(seed, current)

	return mergeStructure(base, motif)
}

func layoutStructureAt(seed int64, current zone) structure {
	switch current.layout {
	case layoutMaze:
		return mazeStructureAt(seed, current)
	case layoutLongHalls:
		return longHallStructureAt(seed, current)
	case layoutCrossroads:
		return crossroadsStructureAt(seed, current)
	case layoutCubicles:
		return cubicleStructureAt(seed, current)
	case layoutPillars:
		return pillarStructureAt(seed, current)
	case layoutSparse:
		return sparseStructureAt(seed, current)
	default:
		return classicStructureAt(seed, current)
	}
}

func zoneBoundaryStructureAt(seed int64, current zone) (structure, bool) {
	xBoundary := current.localX == 0
	zBoundary := current.localZ == 0

	if !xBoundary && !zBoundary {
		return structureOpen, false
	}

	if xBoundary && zBoundary {
		return structureWall, true
	}

	if xBoundary {
		spec := boundaryOpeningSpec(seed, current.z, current.x, true)
		return boundaryPointStructure(current.localZ, spec), true
	}

	spec := boundaryOpeningSpec(seed, current.x, current.z, false)
	return boundaryPointStructure(current.localX, spec), true
}

func boundaryOpening(seed, segment, local, boundary int64, vertical bool) bool {
	spec := boundaryOpeningSpec(seed, segment, boundary, vertical)
	return withinOpening(local, spec.centerA, spec.widthA) || withinOpening(local, spec.centerB, spec.widthB)
}

func boundaryOpeningSpec(seed, segment, boundary int64, vertical bool) openingSpec {
	salt := saltEdge
	if vertical {
		salt ^= 0xe9b91f1b9de263c7
	}

	hash := coordinateHash(seed, boundary, segment, salt)

	return openingSpec{
		hash:    hash,
		centerA: int64(14 + hash%9),
		centerB: int64(41 + (hash>>8)%9),
		widthA:  int64(4 + (hash>>16)%4),
		widthB:  int64(4 + (hash>>24)%4),
	}
}

func boundaryPointStructure(local int64, spec openingSpec) structure {
	if withinOpening(local, spec.centerA, spec.widthA) {
		return openingStructure(spec.widthA, mix64(spec.hash^0x75bf8d2e90c64a13))
	}

	if withinOpening(local, spec.centerB, spec.widthB) {
		return openingStructure(spec.widthB, mix64(spec.hash^0x23617dbe5f09a4c7))
	}

	return structureWall
}

func preferredBoundaryCenter(seed, segment, boundary int64, vertical bool) int64 {
	spec := boundaryOpeningSpec(seed, segment, boundary, vertical)
	if spec.hash&(1<<40) == 0 {
		return spec.centerA
	}

	return spec.centerB
}

func withinOpening(local, center, width int64) bool {
	half := width / 2
	return local >= center-half && local <= center+(width-1)/2
}

func openingStructure(width int64, hash uint64) structure {
	if width >= 5 && hash%4 == 0 {
		return structureOpen
	}

	return structureDoorway
}

func zoneSpineOpenAt(seed int64, current zone) bool {
	hubX := int64(24 + (current.hash>>32)%17)
	hubZ := int64(24 + (current.hash>>40)%17)
	width := int64(3)

	if current.localX <= hubX+width/2 {
		westZ := preferredBoundaryCenter(seed, current.z, current.x, true)
		if horizontalPathAt(current.localX, current.localZ, 0, hubX, westZ, width) ||
			(withinOpening(current.localX, hubX, width) && between(current.localZ, westZ, hubZ)) {
			return true
		}
	}

	if current.localX >= hubX-width/2 {
		eastZ := preferredBoundaryCenter(seed, current.z, current.x+1, true)
		if horizontalPathAt(current.localX, current.localZ, hubX, zoneSize-1, eastZ, width) ||
			(withinOpening(current.localX, hubX, width) && between(current.localZ, eastZ, hubZ)) {
			return true
		}
	}

	if current.localZ <= hubZ+width/2 {
		northX := preferredBoundaryCenter(seed, current.x, current.z, false)
		if verticalPathAt(current.localX, current.localZ, 0, hubZ, northX, width) ||
			(withinOpening(current.localZ, hubZ, width) && between(current.localX, northX, hubX)) {
			return true
		}
	}

	if current.localZ >= hubZ-width/2 {
		southX := preferredBoundaryCenter(seed, current.x, current.z+1, false)
		if verticalPathAt(current.localX, current.localZ, hubZ, zoneSize-1, southX, width) ||
			(withinOpening(current.localZ, hubZ, width) && between(current.localX, southX, hubX)) {
			return true
		}
	}

	return withinOpening(current.localX, hubX, width) && withinOpening(current.localZ, hubZ, width)
}

func horizontalPathAt(x, z, startX, endX, centerZ, width int64) bool {
	minimum := min(startX, endX)
	maximum := max(startX, endX)

	return x >= minimum && x <= maximum && withinOpening(z, centerZ, width)
}

func verticalPathAt(x, z, startZ, endZ, centerX, width int64) bool {
	minimum := min(startZ, endZ)
	maximum := max(startZ, endZ)

	return z >= minimum && z <= maximum && withinOpening(x, centerX, width)
}

func classicStructureAt(seed int64, current zone) structure {
	base := gridStructureAt(seed, current, 16, 3, 5, 35, 0x39a45bb4829f2e1d, 0xb1af2e88384f7cc5)
	divider := classicDividerStructureAt(seed, current)

	return mergeStructure(base, divider)
}

func classicDividerStructureAt(seed int64, current zone) structure {
	const cellSize = int64(16)

	cellX := current.localX / cellSize
	cellZ := current.localZ / cellSize
	withinX := current.localX % cellSize
	withinZ := current.localZ % cellSize

	hash := coordinateHash(seed, current.x*4+cellX, current.z*4+cellZ, saltWall^0x8f0b72d3be9c1465)
	if hash%100 >= 68 {
		return structureOpen
	}

	wallX := int64(5 + (hash>>8)%6)
	wallZ := int64(5 + (hash>>16)%6)
	doorX := int64(4 + (hash>>24)%7)
	doorZ := int64(4 + (hash>>32)%7)

	switch (hash >> 40) % 6 {
	case 0:
		return verticalSegmentStructure(withinX, withinZ, wallX, 2, 13, doorZ, 3, hash)
	case 1:
		return horizontalSegmentStructure(withinX, withinZ, wallZ, 2, 13, doorX, 3, hash)
	case 2:
		vertical := verticalSegmentStructure(withinX, withinZ, wallX, 2, 13, doorZ, 3, hash)
		horizontal := horizontalSegmentStructure(withinX, withinZ, wallZ, wallX, 13, doorX, 3, hash>>1)
		return mergeStructure(vertical, horizontal)
	case 3:
		vertical := verticalSegmentStructure(withinX, withinZ, wallX, wallZ, 13, doorZ, 3, hash)
		horizontal := horizontalSegmentStructure(withinX, withinZ, wallZ, 2, wallX, doorX, 3, hash>>1)
		return mergeStructure(vertical, horizontal)
	case 4:
		if withinZ == wallZ && withinX >= 3 && withinX <= 12 {
			return structureWall
		}
	case 5:
		if (withinX == wallX || withinX == wallX+3) && withinZ >= 4 && withinZ <= 11 {
			if withinOpening(withinZ, doorZ, 3) {
				return structureDoorway
			}

			return structureWall
		}
	}

	return structureOpen
}

func mazeStructureAt(seed int64, current zone) structure {
	base := gridStructureAt(seed, current, 8, 2, 3, 6, 0x0c62b5949e3d81f7, 0x6db18fe3a45c27d9)
	if base != structureOpen {
		return base
	}

	cellX := current.localX / 8
	cellZ := current.localZ / 8
	withinX := current.localX % 8
	withinZ := current.localZ % 8
	hash := coordinateHash(seed, current.x*8+cellX, current.z*8+cellZ, saltWall^0x146bed3fa5c89270)

	if hash%100 >= 24 {
		return structureOpen
	}

	if hash&1 == 0 {
		wallX := int64(2 + (hash>>8)%4)
		if withinX == wallX && withinZ >= 1 && withinZ <= 5 {
			return structureWall
		}

		return structureOpen
	}

	wallZ := int64(2 + (hash>>8)%4)
	if withinZ == wallZ && withinX >= 2 && withinX <= 6 {
		return structureWall
	}

	return structureOpen
}

func gridStructureAt(seed int64, current zone, cellSize, minDoorWidth, maxDoorWidth, missingPercent int64, xSalt, zSalt uint64) structure {
	result := structureOpen

	if current.localX%cellSize == 0 {
		segment := current.localZ / cellSize
		wall := current.localX / cellSize
		hash := coordinateHash(seed, current.x*16+wall, current.z*16+segment, saltWall^xSalt)

		if hash%100 >= uint64(missingPercent) {
			width := doorWidthForHash(hash, minDoorWidth, maxDoorWidth)
			if gridDoorOpen(current.localZ, cellSize, width, hash) {
				result = mergeStructure(result, openingStructure(width, hash>>7))
			} else {
				return structureWall
			}
		}
	}

	if current.localZ%cellSize == 0 {
		segment := current.localX / cellSize
		wall := current.localZ / cellSize
		hash := coordinateHash(seed, current.x*16+segment, current.z*16+wall, saltWall^zSalt)

		if hash%100 >= uint64(missingPercent) {
			width := doorWidthForHash(hash, minDoorWidth, maxDoorWidth)
			if gridDoorOpen(current.localX, cellSize, width, hash) {
				result = mergeStructure(result, openingStructure(width, hash>>7))
			} else {
				return structureWall
			}
		}
	}

	return result
}

func doorWidthForHash(hash uint64, minimum, maximum int64) int64 {
	if maximum <= minimum {
		return minimum
	}

	return minimum + int64((hash>>20)%uint64(maximum-minimum+1))
}

func gridDoorOpen(local, cellSize, width int64, hash uint64) bool {
	position := floorMod(local, cellSize)
	margin := int64(1)
	span := max(cellSize-width-2*margin+1, 1)
	start := margin + int64((hash>>12)%uint64(span))

	return position >= start && position < start+width
}

func longHallParameters(current zone) (bool, int64, int64) {
	vertical := current.hash&1 == 0
	center := int64(28 + (current.hash>>8)%9)
	halfWidth := int64(2 + (current.hash>>16)%2)

	return vertical, center, halfWidth
}

func longHallStructureAt(seed int64, current zone) structure {
	vertical, corridorCenter, halfWidth := longHallParameters(current)

	across := current.localX
	along := current.localZ
	if !vertical {
		across, along = along, across
	}

	if abs64(across-corridorCenter) <= halfWidth {
		return structureOpen
	}

	leftWall := corridorCenter - halfWidth - 1
	rightWall := corridorCenter + halfWidth + 1
	if across == leftWall || across == rightWall {
		segment := along / 12
		side := int64(0)
		if across == rightWall {
			side = 1
		}

		hash := coordinateHash(seed, current.x*16+segment, current.z*16+side, saltWall^0xc28fd4d4604d39c7)
		width := doorWidthForHash(hash, 3, 5)
		if gridDoorOpen(along, 12, width, hash) {
			return openingStructure(width, hash>>11)
		}

		return structureWall
	}

	phase := int64((current.hash >> 24) % 6)
	if floorMod(along+phase, 12) == 0 {
		hash := coordinateHash(seed, current.x*8+across/8, current.z*8+along/12, saltWall^0x4ef37cd47f93d123)
		if hash%100 < 78 {
			width := doorWidthForHash(hash, 3, 4)
			if gridDoorOpen(across, 16, width, hash) {
				return openingStructure(width, hash>>9)
			}

			return structureWall
		}
	}

	return structureOpen
}

func crossroadsStructureAt(seed int64, current zone) structure {
	centerX := int64(29 + (current.hash>>8)%7)
	centerZ := int64(29 + (current.hash>>16)%7)
	halfX := int64(6 + (current.hash>>24)%3)
	halfZ := int64(6 + (current.hash>>32)%3)

	deltaX := abs64(current.localX - centerX)
	deltaZ := abs64(current.localZ - centerZ)

	if deltaX <= halfX && deltaZ <= halfZ {
		return structureOpen
	}

	if deltaX == halfX+1 && deltaZ <= halfZ+1 {
		if withinOpening(current.localZ, centerZ, 5) {
			return structureDoorway
		}

		return structureWall
	}

	if deltaZ == halfZ+1 && deltaX <= halfX+1 {
		if withinOpening(current.localX, centerX, 5) {
			return structureDoorway
		}

		return structureWall
	}

	return gridStructureAt(seed, current, 12, 3, 5, 24, 0x947e2a5db6813fc0, 0x2a1dc4b9538e76f1)
}

func cubicleStructureAt(seed int64, current zone) structure {
	cellX := current.localX / 8
	cellZ := current.localZ / 8
	withinX := current.localX % 8
	withinZ := current.localZ % 8
	hash := coordinateHash(seed, current.x*8+cellX, current.z*8+cellZ, saltWall^0x71873ff6f885b9d1)

	if hash%100 < 18 {
		return structureOpen
	}

	if hash%19 == 0 {
		pillarX := int64(3 + (hash>>8)%2)
		pillarZ := int64(3 + (hash>>12)%2)
		if withinX == pillarX && withinZ == pillarZ {
			return structurePillar
		}
	}

	arm := int64(3 + (hash>>16)%3)
	orientation := (hash >> 24) % 4

	switch orientation {
	case 0:
		if (withinX == 1 && withinZ <= arm) || (withinZ == 1 && withinX <= arm) {
			return structurePartition
		}
	case 1:
		if (withinX == 6 && withinZ <= arm) || (withinZ == 1 && withinX >= 7-arm) {
			return structurePartition
		}
	case 2:
		if (withinX == 6 && withinZ >= 7-arm) || (withinZ == 6 && withinX >= 7-arm) {
			return structurePartition
		}
	default:
		if (withinX == 1 && withinZ >= 7-arm) || (withinZ == 6 && withinX <= arm) {
			return structurePartition
		}
	}

	return structureOpen
}

func pillarStructureAt(seed int64, current zone) structure {
	const cellSize = int64(8)

	cellX := current.localX / cellSize
	cellZ := current.localZ / cellSize
	withinX := current.localX % cellSize
	withinZ := current.localZ % cellSize
	hash := coordinateHash(seed, current.x*8+cellX, current.z*8+cellZ, saltWall^0x52f01d1fa98ddfed)

	if hash%100 < 24 {
		return structureOpen
	}

	size := int64(1)
	if (hash>>8)%100 < 34 {
		size = 2
	}
	if (hash>>16)%100 < 5 {
		size = 3
	}

	span := max(cellSize-size-2, 1)
	startX := int64(1 + (hash>>24)%uint64(span))
	startZ := int64(1 + (hash>>32)%uint64(span))

	if withinX >= startX && withinX < startX+size && withinZ >= startZ && withinZ < startZ+size {
		return structurePillar
	}

	if hash%13 == 0 {
		if withinZ == startZ && withinX >= startX && withinX <= min(startX+5, cellSize-2) {
			return structureWall
		}
	}

	return structureOpen
}

func sparseStructureAt(seed int64, current zone) structure {
	const cellSize = int64(16)

	cellX := current.localX / cellSize
	cellZ := current.localZ / cellSize
	withinX := current.localX % cellSize
	withinZ := current.localZ % cellSize
	hash := coordinateHash(seed, current.x*4+cellX, current.z*4+cellZ, saltWall^0xd21fb6417307e88f)

	switch hash % 8 {
	case 0, 1:
		return structureOpen
	case 2:
		if withinZ == 6 && withinX >= 2 && withinX <= 13 {
			if withinOpening(withinX, 9, 4) {
				return structureDoorway
			}

			return structureWall
		}
	case 3:
		if withinX == 8 && withinZ >= 2 && withinZ <= 13 {
			if withinOpening(withinZ, 6, 3) {
				return structureDoorway
			}

			return structureWall
		}
	case 4:
		if (withinX == 4 && withinZ >= 4 && withinZ <= 12) ||
			(withinZ == 4 && withinX >= 4 && withinX <= 11) {
			return structureWall
		}
	case 5:
		if rectanglePerimeterAt(withinX, withinZ, 3, 12, 4, 11) {
			if withinZ == 11 && withinOpening(withinX, 8, 4) {
				return structureDoorway
			}

			return structureWall
		}
	case 6:
		if withinZ == 5 && withinX >= 3 && withinX <= 12 {
			return structurePartition
		}
	case 7:
		if withinX >= 6 && withinX <= 8 && withinZ >= 5 && withinZ <= 10 {
			return structurePillar
		}
	}

	return structureOpen
}

func motifStructureAt(seed int64, current zone) structure {
	firstHash := mix64(current.hash ^ saltMotif)
	first := singleMotifStructureAt(seed, current, firstHash, 52)

	secondHash := mix64(firstHash ^ 0x7d816a3cf205e94b)
	second := singleMotifStructureAt(seed, current, secondHash, 24)

	return mergeStructure(first, second)
}

func singleMotifStructureAt(seed int64, current zone, hash uint64, chance uint64) structure {
	if hash%100 >= chance {
		return structureOpen
	}

	switch (hash >> 8) % 5 {
	case 0:
		return freestandingWallMotif(current, hash)
	case 1:
		return islandRoomMotif(current, hash)
	case 2:
		return cornerMotif(current, hash)
	case 3:
		return bulkheadMotif(current, hash)
	default:
		return partitionMotif(current, hash)
	}
}

func freestandingWallMotif(current zone, hash uint64) structure {
	vertical := hash&(1<<16) == 0
	line := int64(12 + (hash>>20)%40)
	start := int64(8 + (hash>>28)%20)
	length := int64(14 + (hash>>36)%15)
	end := min(start+length, zoneSize-8)
	door := start + length/2

	if vertical {
		return verticalSegmentStructure(current.localX, current.localZ, line, start, end, door, 3, hash)
	}

	return horizontalSegmentStructure(current.localX, current.localZ, line, start, end, door, 3, hash)
}

func islandRoomMotif(current zone, hash uint64) structure {
	x0 := int64(8 + (hash>>16)%29)
	z0 := int64(8 + (hash>>24)%29)
	width := int64(9 + (hash>>32)%8)
	depth := int64(9 + (hash>>40)%8)
	x1 := min(x0+width, zoneSize-7)
	z1 := min(z0+depth, zoneSize-7)

	if !rectanglePerimeterAt(current.localX, current.localZ, x0, x1, z0, z1) {
		return structureOpen
	}

	side := (hash >> 48) % 4
	switch side {
	case 0:
		if current.localZ == z0 && withinOpening(current.localX, (x0+x1)/2, 3) {
			return structureDoorway
		}
	case 1:
		if current.localX == x1 && withinOpening(current.localZ, (z0+z1)/2, 3) {
			return structureDoorway
		}
	case 2:
		if current.localZ == z1 && withinOpening(current.localX, (x0+x1)/2, 3) {
			return structureDoorway
		}
	default:
		if current.localX == x0 && withinOpening(current.localZ, (z0+z1)/2, 3) {
			return structureDoorway
		}
	}

	return structureWall
}

func cornerMotif(current zone, hash uint64) structure {
	cornerX := int64(12 + (hash>>16)%40)
	cornerZ := int64(12 + (hash>>24)%40)
	lengthX := int64(7 + (hash>>32)%10)
	lengthZ := int64(7 + (hash>>40)%10)
	directionX := int64(1)
	directionZ := int64(1)

	if hash&(1<<50) != 0 {
		directionX = -1
	}
	if hash&(1<<51) != 0 {
		directionZ = -1
	}

	xEnd := clamp(cornerX+directionX*lengthX, 5, zoneSize-6)
	zEnd := clamp(cornerZ+directionZ*lengthZ, 5, zoneSize-6)

	horizontal := current.localZ == cornerZ && between(current.localX, cornerX, xEnd)
	vertical := current.localX == cornerX && between(current.localZ, cornerZ, zEnd)
	if horizontal || vertical {
		return structureWall
	}

	return structureOpen
}

func bulkheadMotif(current zone, hash uint64) structure {
	vertical := hash&(1<<16) == 0
	line := int64(10 + (hash>>20)%44)
	start := int64(8 + (hash>>28)%18)
	end := min(start+int64(18+(hash>>36)%20), zoneSize-7)

	if vertical {
		if current.localX == line && current.localZ >= start && current.localZ <= end {
			return structureBulkhead
		}

		return structureOpen
	}

	if current.localZ == line && current.localX >= start && current.localX <= end {
		return structureBulkhead
	}

	return structureOpen
}

func partitionMotif(current zone, hash uint64) structure {
	vertical := hash&(1<<16) == 0
	line := int64(10 + (hash>>20)%44)
	start := int64(8 + (hash>>28)%22)
	end := min(start+int64(10+(hash>>36)%14), zoneSize-7)

	if vertical {
		if current.localX == line && current.localZ >= start && current.localZ <= end {
			return structurePartition
		}

		return structureOpen
	}

	if current.localZ == line && current.localX >= start && current.localX <= end {
		return structurePartition
	}

	return structureOpen
}

func verticalSegmentStructure(x, z, wallX, startZ, endZ, doorCenter, doorWidth int64, hash uint64) structure {
	if x != wallX || !between(z, startZ, endZ) {
		return structureOpen
	}

	if withinOpening(z, doorCenter, doorWidth) {
		return openingStructure(doorWidth, hash)
	}

	return structureWall
}

func horizontalSegmentStructure(x, z, wallZ, startX, endX, doorCenter, doorWidth int64, hash uint64) structure {
	if z != wallZ || !between(x, startX, endX) {
		return structureOpen
	}

	if withinOpening(x, doorCenter, doorWidth) {
		return openingStructure(doorWidth, hash)
	}

	return structureWall
}

func rectanglePerimeterAt(x, z, x0, x1, z0, z1 int64) bool {
	if x < x0 || x > x1 || z < z0 || z > z1 {
		return false
	}

	return x == x0 || x == x1 || z == z0 || z == z1
}

func mergeStructure(first, second structure) structure {
	if first > second {
		return first
	}

	return second
}

func zoneCoordinate(value int64) (int64, int64) {
	shifted := value + zoneSize/2
	zone := floorDiv(shifted, zoneSize)
	local := shifted - zone*zoneSize

	return zone, local
}

func floorDiv(value, divisor int64) int64 {
	quotient := value / divisor
	if value%divisor < 0 {
		quotient--
	}

	return quotient
}

func floorMod(value, divisor int64) int64 {
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}

	return remainder
}

func coordinateHash(seed, x, z int64, salt uint64) uint64 {
	hash := uint64(seed) ^ salt
	hash ^= mix64(uint64(x) + 0x9e3779b97f4a7c15)
	hash ^= mix64(uint64(z) + 0xbf58476d1ce4e5b9)

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

func between(value, first, second int64) bool {
	minimum := min(first, second)
	maximum := max(first, second)

	return value >= minimum && value <= maximum
}

func clamp(value, minimum, maximum int64) int64 {
	return max(minimum, min(value, maximum))
}

func abs32(value int32) int32 {
	if value < 0 {
		return -value
	}

	return value
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
