package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "backrooms"

	// The base layer is only a coordinate template. The same architectural
	// grammar repeats vertically with a different deterministic seed per layer.
	foundationY    = int32(62)
	floorY         = int32(63)
	normalCeilingY = int32(67)
	ceilingY       = int32(68)

	layerStride       = int32(8)
	lowestLayerIndex  = int64(-15)
	highestLayerIndex = int64(31)
	worldMinY         = int32(-64)
	worldMaxY         = int32(319)

	// Kept as template coordinates for the generic eight-step staircase helpers.
	upperFloorY      = floorY + layerStride
	upperCeilingY    = upperFloorY + 4
	lowerFloorY      = floorY - layerStride
	lowerCeilingY    = lowerFloorY + 4
	lowerFoundationY = lowerFloorY - 1

	zoneSize          = int64(64)
	paletteRegionSize = int64(3)
	paletteLayerSize  = int64(2)
	verticalGroupSize = int64(6)
)

const (
	saltZone      uint64 = 0x8ca19d7a4584f1d7
	saltPalette   uint64 = 0x15f59d0ed05fd4ab
	saltEdge      uint64 = 0xd75a95d152ed83b9
	saltWall      uint64 = 0x4f1bbcdc6768a3d1
	saltFloor     uint64 = 0xc24b8b70d0f89791
	saltLight     uint64 = 0x9a4275c4e7aa8273
	saltDetail    uint64 = 0x72df8e29ec5d4db3
	saltMotif     uint64 = 0xa63c21e6bf41f357
	saltFeature   uint64 = 0x3c8e71ad95f2046b
	saltFurniture uint64 = 0xb4d2097e6ac13f85
	saltVertical  uint64 = 0x61f830e92bd4a7c5
	saltOddity    uint64 = 0x2e74c19af56803bd
	saltLayer     uint64 = 0x94b5172fd36ae801
	saltConnector uint64 = 0x7d3c8af196b24e50
	saltAtrium    uint64 = 0xca61d932f84b705e
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

type zoneFeature uint8

const (
	featureNone zoneFeature = iota
	featureLibrary
	featureArchive
	featureReception
	featureDarkRoom
	featureDoorGallery
	featureServiceRoom
	featureConference
	featureBathroom
	featureRenovation
	featureWindowRoom
	featureStorage
	featureClassroom
	featureMachineRoom
)

type verticalFeature uint8

const (
	verticalNone verticalFeature = iota
	verticalUpperAnnex
	verticalLowerAnnex
	verticalStack
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
	x        int64
	z        int64
	layer    int64
	localX   int64
	localZ   int64
	hash     uint64
	layout   layout
	palette  palette
	feature  zoneFeature
	vertical verticalFeature
}

type openingSpec struct {
	hash    uint64
	centerA int64
	centerB int64
	widthA  int64
	widthB  int64
}

type featureSide uint8

const (
	featureNorth featureSide = iota
	featureEast
	featureSouth
	featureWest
)

type featureRoom struct {
	x0           int64
	x1           int64
	z0           int64
	z1           int64
	entranceSide featureSide
	doorCenter   int64
}

type verticalPlan struct {
	x0     int64
	x1     int64
	z0     int64
	z1     int64
	startX int64
	startZ int64
	stepX  int64
	stepZ  int64
	baseX  int64
	baseZ  int64
	upper  bool
}

type atriumSpec struct {
	enabled     bool
	anchorLayer int64
	span        int64
	x0          int64
	x1          int64
	z0          int64
	z1          int64
	hash        uint64
}

type ambientDoorSpec struct {
	enabled   bool
	vertical  bool
	line      int64
	center    int64
	direction int64
	falseDoor bool
	iron      bool
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
	if block, handled := grandAtriumBlockAt(seed, worldX, worldY, worldZ, current); handled {
		return block
	}

	if block, handled := layerConnectorBlockAt(seed, worldX, worldY, worldZ, current); handled {
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

	if block, ok := featureBlockAt(seed, worldX, templateY, worldZ, current); ok {
		return block
	}

	if block, ok := ambientDoorBlockAt(seed, templateY, current); ok {
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
	layerCount := int(maxLayer - minLayer + 1)
	if layerCount < 0 {
		layerCount = 0
	}

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

func zoneAt(seed, worldX, worldZ int64) zone {
	return zoneAtLayer(seed, worldX, worldZ, 0)
}

func zoneAtLayer(seed, worldX, worldZ, layer int64) zone {
	zoneX, localX := zoneCoordinate(worldX)
	zoneZ, localZ := zoneCoordinate(worldZ)

	hashSalt := saltZone
	if layer != 0 {
		hashSalt ^= mix64(uint64(layer) + saltLayer)
	}
	hash := coordinateHash(seed, zoneX, zoneZ, hashSalt)

	paletteX := floorDiv(zoneX, paletteRegionSize)
	paletteZ := floorDiv(zoneZ, paletteRegionSize)
	paletteSalt := saltPalette
	if layer != 0 {
		paletteLayer := floorDiv(layer, paletteLayerSize)
		paletteSalt ^= mix64(uint64(paletteLayer) + 0x6ac1437e92d5fb08)
	}
	paletteHash := coordinateHash(seed, paletteX, paletteZ, paletteSalt)

	feature := featureForHash(mix64(hash ^ saltFeature))

	return zone{
		x:        zoneX,
		z:        zoneZ,
		layer:    layer,
		localX:   localX,
		localZ:   localZ,
		hash:     hash,
		layout:   layoutForHash(hash),
		palette:  paletteForHash(paletteHash),
		feature:  feature,
		vertical: verticalNone,
	}
}

func layerFloorY(layer int64) int32 {
	return floorY + int32(layer)*layerStride
}

func layerAtY(worldY int32) int64 {
	return floorDiv(int64(worldY-floorY+1), int64(layerStride))
}

func interstitialBlock(selected palette) game.Block {
	if selected == paletteMaintenance {
		return game.StoneBricks
	}

	return game.SmoothStone
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

func featureForHash(hash uint64) zoneFeature {
	bucket := hash % 1000

	switch {
	case bucket < 10:
		return featureLibrary
	case bucket < 19:
		return featureArchive
	case bucket < 31:
		return featureReception
	case bucket < 43:
		return featureDarkRoom
	case bucket < 56:
		return featureDoorGallery
	case bucket < 71:
		return featureServiceRoom
	case bucket < 83:
		return featureConference
	case bucket < 93:
		return featureBathroom
	case bucket < 103:
		return featureRenovation
	case bucket < 112:
		return featureWindowRoom
	case bucket < 122:
		return featureStorage
	case bucket < 131:
		return featureClassroom
	case bucket < 139:
		return featureMachineRoom
	default:
		return featureNone
	}
}

func verticalFeatureForHash(hash uint64, feature zoneFeature) verticalFeature {
	if feature == featureLibrary && (hash>>24)%5 == 0 {
		return verticalUpperAnnex
	}

	if feature != featureNone {
		return verticalNone
	}

	bucket := hash % 2000

	switch {
	case bucket < 22:
		return verticalUpperAnnex
	case bucket < 44:
		return verticalLowerAnnex
	case bucket < 52:
		return verticalStack
	default:
		return verticalNone
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
	if block, ok := featureFloorBlock(current, blocks); ok {
		return block
	}

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
	if darkRoomAt(current) {
		return blocks.ceiling
	}

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

func verticalPlanForZone(current zone, upper bool) verticalPlan {
	salt := saltVertical ^ uint64(0x6c84a9f21d37be05)
	if upper {
		salt ^= 0xb8214f7ca6039de1
	} else {
		salt ^= 0x35d7ec109af84263
	}

	hash := mix64(current.hash ^ salt)
	width := int64(30 + (hash>>8)%5)
	depth := int64(25 + (hash>>16)%5)

	availableX := zoneSize - width - 12
	availableZ := zoneSize - depth - 12
	x0 := int64(6)
	z0 := int64(6)
	if availableX > 0 {
		x0 += int64((hash >> 24) % uint64(availableX+1))
	}
	if availableZ > 0 {
		z0 += int64((hash >> 32) % uint64(availableZ+1))
	}

	x1 := x0 + width - 1
	z1 := z0 + depth - 1

	plan := verticalPlan{x0: x0, x1: x1, z0: z0, z1: z1, upper: upper}
	runAlongX := hash&1 == 0
	positive := hash&(1<<1) == 0

	if runAlongX {
		plan.startZ = (z0+z1)/2 - 1
		if positive {
			plan.startX = x0 + 4
			plan.stepX = 1
		} else {
			plan.startX = x1 - 4
			plan.stepX = -1
		}
	} else {
		plan.startX = (x0+x1)/2 - 1
		if positive {
			plan.startZ = z0 + 4
			plan.stepZ = 1
		} else {
			plan.startZ = z1 - 4
			plan.stepZ = -1
		}
	}

	if upper {
		plan.baseX = plan.startX
		plan.baseZ = plan.startZ
	} else {
		plan.baseX = plan.startX + plan.stepX*7
		plan.baseZ = plan.startZ + plan.stepZ*7
	}

	return plan
}

func layerConnectorEnabled(seed int64, zoneX, zoneZ, lowerLayer int64) bool {
	if lowerLayer < lowestLayerIndex || lowerLayer >= highestLayerIndex {
		return false
	}

	groupSalt := saltConnector ^ mix64(uint64(lowerLayer)+0x43eaf2159b70c68d)
	hash := coordinateHash(seed, zoneX, zoneZ, groupSalt)

	// Roughly one connector per fifteen horizontal zones for each floor boundary.
	return hash%1000 < 66
}

func connectorPlanForZone(current zone) verticalPlan {
	plan := verticalPlanForZone(current, true)
	plan.upper = true
	return plan
}

func layerConnectorBlockAt(seed, worldX, worldY, worldZ int64, current zone) (game.Block, bool) {
	for _, lowerLayer := range []int64{current.layer - 1, current.layer} {
		if !layerConnectorEnabled(seed, current.x, current.z, lowerLayer) {
			continue
		}

		lowerZone := zoneAtLayer(seed, worldX, worldZ, lowerLayer)
		if spec := grandAtriumForZone(seed, lowerZone); spec.enabled && lowerLayer >= spec.anchorLayer && lowerLayer < spec.anchorLayer+spec.span-1 {
			continue
		}

		plan := connectorPlanForZone(lowerZone)
		if !stairEnvelopeXZAt(plan, lowerZone.localX, lowerZone.localZ) {
			continue
		}

		normalizedY := int64(floorY) + (worldY - int64(layerFloorY(lowerLayer)))
		minimumY := int64(floorY + 1)
		maximumY := int64(upperFloorY + 2)
		if normalizedY < minimumY || normalizedY > maximumY {
			continue
		}

		blocks := blocksForPalette(lowerZone.palette)
		if stairSideWallAt(plan, lowerZone.localX, lowerZone.localZ) {
			if normalizedY <= maximumY-1 {
				return blocks.wall, true
			}

			return game.Air, true
		}

		if step, ok := stairStepAt(plan, lowerZone.localX, lowerZone.localZ); ok {
			stairY := int64(floorY+1) + int64(step)
			if normalizedY == stairY {
				return stairBlock(game.OakStairs, stairFacing(plan)), true
			}
		}

		return game.Air, true
	}

	return game.Air, false
}

func verticalBaseOpenAt(seed int64, current zone) bool {
	// Clear a short approach on both floors around ordinary inter-layer stairs.
	if layerConnectorEnabled(seed, current.x, current.z, current.layer) {
		plan := connectorPlanForZone(current)
		if connectorApproachOpenAt(current, plan, false) {
			return true
		}
	}

	if layerConnectorEnabled(seed, current.x, current.z, current.layer-1) {
		lower := current
		lower.layer--
		plan := connectorPlanForZone(lower)
		if connectorApproachOpenAt(current, plan, true) {
			return true
		}
	}

	return false
}

func connectorApproachOpenAt(current zone, plan verticalPlan, upperEnd bool) bool {
	x := plan.startX
	z := plan.startZ
	if upperEnd {
		x += plan.stepX * 7
		z += plan.stepZ * 7
	}

	return abs64(current.localX-x) <= 3 && abs64(current.localZ-z) <= 3
}

func grandAtriumForZone(seed int64, current zone) atriumSpec {
	group := floorDiv(current.layer, verticalGroupSize)
	anchorLayer := group * verticalGroupSize
	hashSalt := saltAtrium ^ mix64(uint64(group)+0x8fd2473cb159e60a)
	hash := coordinateHash(seed, current.x, current.z, hashSalt)

	// Around 1.5% of 64x64 zones per six-floor vertical group.
	if hash%2000 >= 30 {
		return atriumSpec{}
	}

	span := int64(2 + (hash>>12)%3) // two to four floors
	if anchorLayer < lowestLayerIndex || anchorLayer+span-1 > highestLayerIndex {
		return atriumSpec{}
	}

	anchorHashSalt := saltZone
	if anchorLayer != 0 {
		anchorHashSalt ^= mix64(uint64(anchorLayer) + saltLayer)
	}
	anchorHash := coordinateHash(seed, current.x, current.z, anchorHashSalt)
	if featureForHash(mix64(anchorHash^saltFeature)) != featureNone {
		return atriumSpec{}
	}

	width := int64(44 + (hash>>20)%9)
	depth := int64(40 + (hash>>28)%11)

	x0 := int64(5 + (hash>>36)%5)
	z0 := int64(5 + (hash>>44)%5)
	x1 := min(x0+width-1, zoneSize-5)
	z1 := min(z0+depth-1, zoneSize-5)

	return atriumSpec{
		enabled:     true,
		anchorLayer: anchorLayer,
		span:        span,
		x0:          x0,
		x1:          x1,
		z0:          z0,
		z1:          z1,
		hash:        hash,
	}
}

func grandAtriumBlockAt(seed, worldX, worldY, worldZ int64, current zone) (game.Block, bool) {
	spec := grandAtriumForZone(seed, current)
	if !spec.enabled || current.layer < spec.anchorLayer || current.layer >= spec.anchorLayer+spec.span {
		return game.Air, false
	}

	if current.localX < spec.x0 || current.localX > spec.x1 || current.localZ < spec.z0 || current.localZ > spec.z1 {
		return game.Air, false
	}

	anchorZone := zoneAtLayer(seed, worldX, worldZ, spec.anchorLayer)
	blocks := blocksForPalette(anchorZone.palette)
	bottomFloor := int64(layerFloorY(spec.anchorLayer))
	topLayer := spec.anchorLayer + spec.span - 1
	topCeiling := int64(layerFloorY(topLayer) + 5)

	if worldY < bottomFloor-1 || worldY > topCeiling {
		return game.Air, false
	}

	if block, ok := atriumStairBlockAt(spec, worldY, current.localX, current.localZ); ok {
		return block, true
	}

	if atriumColumnAt(spec, current.localX, current.localZ) && worldY > bottomFloor && worldY < topCeiling {
		if worldY == bottomFloor+1 || floorMod(worldY-bottomFloor, int64(layerStride)) == 1 {
			return blocks.trim, true
		}
		return blocks.wall, true
	}

	if atriumPerimeterAt(spec, current.localX, current.localZ) {
		if atriumEntranceAt(spec, current, worldY) {
			return game.Air, true
		}

		return blocks.wall, true
	}

	if worldY == bottomFloor-1 {
		return foundationBlock(anchorZone.palette), true
	}
	if worldY == bottomFloor {
		return blocks.floor, true
	}

	for layer := spec.anchorLayer + 1; layer <= topLayer; layer++ {
		levelFloor := int64(layerFloorY(layer))
		if worldY == levelFloor {
			if atriumBalconyAt(spec, current.localX, current.localZ, layer-spec.anchorLayer) {
				return blocks.floor, true
			}
			return game.Air, true
		}

		if worldY == levelFloor+1 && atriumRailAt(spec, current.localX, current.localZ, layer-spec.anchorLayer) {
			return game.IronBars, true
		}
	}

	if worldY == topCeiling {
		if atriumCeilingLightAt(spec, current.localX, current.localZ) {
			return blocks.light, true
		}
		return blocks.ceiling, true
	}

	return game.Air, true
}

func atriumPerimeterAt(spec atriumSpec, x, z int64) bool {
	return x == spec.x0 || x == spec.x1 || z == spec.z0 || z == spec.z1
}

func atriumEntranceAt(spec atriumSpec, current zone, worldY int64) bool {
	levelFloor := int64(layerFloorY(current.layer))
	if worldY < levelFloor+1 || worldY > levelFloor+3 {
		return false
	}

	centerX := (spec.x0 + spec.x1) / 2
	centerZ := (spec.z0 + spec.z1) / 2
	return ((current.localZ == spec.z0 || current.localZ == spec.z1) && abs64(current.localX-centerX) <= 2) ||
		((current.localX == spec.x0 || current.localX == spec.x1) && abs64(current.localZ-centerZ) <= 2)
}

func atriumBalconyAt(spec atriumSpec, x, z, level int64) bool {
	const width = int64(5)

	nearEdge := x <= spec.x0+width || x >= spec.x1-width || z <= spec.z0+width || z >= spec.z1-width
	if nearEdge {
		return true
	}

	centerX := (spec.x0 + spec.x1) / 2
	centerZ := (spec.z0 + spec.z1) / 2
	if level%2 == 0 {
		return abs64(z-centerZ) <= 1
	}

	return abs64(x-centerX) <= 1
}

func atriumRailAt(spec atriumSpec, x, z, level int64) bool {
	if !atriumBalconyAt(spec, x, z, level) {
		return false
	}

	const width = int64(5)
	innerX0 := spec.x0 + width
	innerX1 := spec.x1 - width
	innerZ0 := spec.z0 + width
	innerZ1 := spec.z1 - width

	centerX := (spec.x0 + spec.x1) / 2
	centerZ := (spec.z0 + spec.z1) / 2

	if level%2 == 0 && abs64(z-centerZ) <= 1 {
		return (z == centerZ-1 || z == centerZ+1) && x > innerX0 && x < innerX1
	}
	if level%2 != 0 && abs64(x-centerX) <= 1 {
		return (x == centerX-1 || x == centerX+1) && z > innerZ0 && z < innerZ1
	}

	return x == innerX0 || x == innerX1 || z == innerZ0 || z == innerZ1
}

func atriumColumnAt(spec atriumSpec, x, z int64) bool {
	positions := [][2]int64{
		{spec.x0 + 7, spec.z0 + 7},
		{spec.x1 - 8, spec.z0 + 7},
		{spec.x0 + 7, spec.z1 - 8},
		{spec.x1 - 8, spec.z1 - 8},
	}

	for _, position := range positions {
		if x >= position[0] && x <= position[0]+1 && z >= position[1] && z <= position[1]+1 {
			return true
		}
	}

	return false
}

func atriumCeilingLightAt(spec atriumSpec, x, z int64) bool {
	if x <= spec.x0+2 || x >= spec.x1-2 || z <= spec.z0+2 || z >= spec.z1-2 {
		return false
	}

	phaseX := int64((spec.hash >> 8) & 3)
	phaseZ := int64((spec.hash >> 12) & 3)
	return floorMod(x+phaseX, 10) >= 4 && floorMod(x+phaseX, 10) <= 6 && floorMod(z+phaseZ, 9) == 4
}

func atriumStairBlockAt(spec atriumSpec, worldY, x, z int64) (game.Block, bool) {
	for gap := int64(0); gap < spec.span-1; gap++ {
		lowerFloor := int64(layerFloorY(spec.anchorLayer + gap))
		upperFloor := int64(layerFloorY(spec.anchorLayer + gap + 1))

		if worldY < lowerFloor+1 || worldY > upperFloor+2 {
			continue
		}

		plan := atriumStairPlan(spec, gap)
		if !stairEnvelopeXZAt(plan, x, z) {
			continue
		}

		if stairSideWallAt(plan, x, z) {
			if worldY <= upperFloor+1 {
				return game.CutSandstone, true
			}
			return game.Air, true
		}

		if step, ok := stairStepAt(plan, x, z); ok && worldY == lowerFloor+1+int64(step) {
			return stairBlock(game.OakStairs, stairFacing(plan)), true
		}

		return game.Air, true
	}

	return game.Air, false
}

func atriumStairPlan(spec atriumSpec, gap int64) verticalPlan {
	plan := verticalPlan{upper: true}

	switch gap % 4 {
	case 0:
		plan.startX = spec.x0 + 7
		plan.startZ = spec.z0 + 4
		plan.stepX = 1
	case 1:
		plan.startX = spec.x1 - 5
		plan.startZ = spec.z0 + 7
		plan.stepZ = 1
	case 2:
		plan.startX = spec.x1 - 7
		plan.startZ = spec.z1 - 5
		plan.stepX = -1
	default:
		plan.startX = spec.x0 + 4
		plan.startZ = spec.z1 - 7
		plan.stepZ = -1
	}

	return plan
}

func verticalRoomContains(plan verticalPlan, x, z int64) bool {
	return x >= plan.x0 && x <= plan.x1 && z >= plan.z0 && z <= plan.z1
}

func verticalRoomPerimeterAt(plan verticalPlan, x, z int64) bool {
	return rectanglePerimeterAt(x, z, plan.x0, plan.x1, plan.z0, plan.z1)
}

func stairEnvelopeXZAt(plan verticalPlan, x, z int64) bool {
	endX := plan.startX + plan.stepX*7
	endZ := plan.startZ + plan.stepZ*7

	if plan.stepX != 0 {
		minimumX := min(plan.startX, endX)
		maximumX := max(plan.startX, endX)
		return x >= minimumX && x <= maximumX && z >= plan.startZ-1 && z <= plan.startZ+2
	}

	minimumZ := min(plan.startZ, endZ)
	maximumZ := max(plan.startZ, endZ)
	return z >= minimumZ && z <= maximumZ && x >= plan.startX-1 && x <= plan.startX+2
}

func stairInteriorXZAt(plan verticalPlan, x, z int64) bool {
	if !stairEnvelopeXZAt(plan, x, z) {
		return false
	}

	return !stairSideWallAt(plan, x, z)
}

func stairSideWallAt(plan verticalPlan, x, z int64) bool {
	if plan.stepX != 0 {
		return z == plan.startZ-1 || z == plan.startZ+2
	}

	return x == plan.startX-1 || x == plan.startX+2
}

func stairStepAt(plan verticalPlan, x, z int64) (int, bool) {
	for step := 0; step < 8; step++ {
		stepX := plan.startX + plan.stepX*int64(step)
		stepZ := plan.startZ + plan.stepZ*int64(step)

		if plan.stepX != 0 {
			if x == stepX && (z == stepZ || z == stepZ+1) {
				return step, true
			}
		} else if z == stepZ && (x == stepX || x == stepX+1) {
			return step, true
		}
	}

	return 0, false
}

func stairFacing(plan verticalPlan) string {
	switch {
	case plan.stepX > 0:
		return "east"
	case plan.stepX < 0:
		return "west"
	case plan.stepZ > 0:
		return "south"
	default:
		return "north"
	}
}

func stairBlock(base game.Block, facing string) game.Block {
	block, ok := base.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: facing},
		game.BlockPropertyValue{Name: "half", Value: "bottom"},
	)
	if !ok {
		return base
	}

	return block
}

func verticalWallBlock(plan verticalPlan, blocks paletteBlocks) game.Block {
	if plan.upper {
		return blocks.wall
	}

	return game.StoneBricks
}

func verticalLightAt(current zone, plan verticalPlan) bool {
	localX := current.localX - plan.x0
	localZ := current.localZ - plan.z0

	if plan.upper {
		return floorMod(localX+2, 8) >= 3 && floorMod(localX+2, 8) <= 4 && floorMod(localZ+3, 7) == 3
	}

	return floorMod(localX+1, 9) == 4 && floorMod(localZ+2, 8) >= 3 && floorMod(localZ+2, 8) <= 4
}

func verticalPartitionAt(current zone, plan verticalPlan) bool {
	if !verticalRoomContains(plan, current.localX, current.localZ) || verticalRoomPerimeterAt(plan, current.localX, current.localZ) {
		return false
	}

	if stairEnvelopeXZAt(plan, current.localX, current.localZ) {
		return false
	}

	hash := mix64(current.hash ^ saltVertical ^ 0x9d74be630af51c28)
	relativeX := current.localX - plan.x0
	relativeZ := current.localZ - plan.z0

	if plan.upper {
		if floorMod(relativeX+int64(hash%4), 8) == 4 && relativeZ >= 4 && relativeZ <= (plan.z1-plan.z0)-4 {
			return floorMod(relativeZ+int64((hash>>8)%9), 11) > 2
		}

		return false
	}

	if floorMod(relativeZ+int64(hash%5), 9) == 4 && relativeX >= 4 && relativeX <= (plan.x1-plan.x0)-4 {
		return floorMod(relativeX+int64((hash>>8)%8), 12) > 3
	}

	return false
}

func verticalFurnitureBlockAt(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks, plan verticalPlan) (game.Block, bool) {
	floor := upperFloorY
	if !plan.upper {
		floor = lowerFloorY
	}

	if int32(worldY) != floor+1 && int32(worldY) != floor+2 {
		return game.Air, false
	}

	if plan.upper && current.feature == featureLibrary {
		if int32(worldY) == floor+1 || int32(worldY) == floor+2 {
			relativeX := current.localX - plan.x0
			relativeZ := current.localZ - plan.z0

			if relativeX >= 3 && floorMod(relativeX-3, 6) == 0 && relativeZ >= 3 && relativeZ <= (plan.z1-plan.z0)-3 {
				if !stairEnvelopeXZAt(plan, current.localX, current.localZ) {
					return bookshelfBlock(seed, worldX, worldZ), true
				}
			}
		}
	}

	if plan.upper {
		if int32(worldY) != floor+1 || stairEnvelopeXZAt(plan, current.localX, current.localZ) {
			return game.Air, false
		}

		hash := coordinateHash(seed, worldX, worldZ, saltFurniture^0x8ef104db6a72c395)
		if hash%67 == 0 {
			return game.OakPlanks, true
		}

		return game.Air, false
	}

	if int32(worldY) == floor+1 {
		relativeX := current.localX - plan.x0
		relativeZ := current.localZ - plan.z0

		if relativeX >= 3 && relativeZ >= 3 && floorMod(relativeX, 7) == 3 && floorMod(relativeZ, 7) == 3 {
			hash := coordinateHash(seed, worldX, worldZ, saltFurniture^0x3a6821d90e4fb7c5)
			if hash&1 == 0 {
				return game.CopperBlock, true
			}

			return game.IronBars, true
		}
	}

	return game.Air, false
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

	if verticalBaseOpenAt(seed, current) {
		return structureOpen
	}

	if zoneSpineOpenAt(seed, current) {
		return structureOpen
	}

	if featured, ok := featureStructureAt(seed, current); ok {
		return featured
	}

	if oddity, ok := ambientDoorStructureAt(seed, current); ok {
		return oddity
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

func featureStructureAt(seed int64, current zone) (structure, bool) {
	if current.feature == featureNone {
		return structureOpen, false
	}

	room := featureRoomForZone(current)

	switch current.feature {
	case featureLibrary:
		return roomFeatureStructureAt(current, room, 2)
	case featureArchive:
		return roomFeatureStructureAt(current, room, 1)
	case featureDarkRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureServiceRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureConference:
		return roomFeatureStructureAt(current, room, 2)
	case featureBathroom:
		return bathroomStructureAt(current, room)
	case featureRenovation:
		return renovationStructureAt(current, room)
	case featureWindowRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureStorage:
		return roomFeatureStructureAt(current, room, 1)
	case featureClassroom:
		return roomFeatureStructureAt(current, room, 2)
	case featureMachineRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureReception:
		return receptionStructureAt(current, room)
	case featureDoorGallery:
		return doorGalleryStructureAt(current, room)
	default:
		return structureOpen, false
	}
}

func featureRoomForZone(current zone) featureRoom {
	hash := mix64(current.hash ^ saltFeature ^ 0x5e21a93fc487d60b)

	width := int64(18 + (hash>>8)%5)
	depth := int64(15 + (hash>>16)%5)

	switch current.feature {
	case featureLibrary:
		width = int64(20 + (hash>>8)%5)
		depth = int64(17 + (hash>>16)%5)
	case featureArchive:
		width = int64(17 + (hash>>8)%4)
		depth = int64(15 + (hash>>16)%4)
	case featureDarkRoom:
		width = int64(17 + (hash>>8)%6)
		depth = int64(15 + (hash>>16)%6)
	case featureDoorGallery:
		width = int64(20 + (hash>>8)%4)
		depth = int64(18 + (hash>>16)%4)
	case featureServiceRoom:
		width = int64(9 + (hash>>8)%4)
		depth = int64(8 + (hash>>16)%4)
	case featureConference:
		width = int64(24 + (hash>>8)%6)
		depth = int64(18 + (hash>>16)%6)
	case featureBathroom:
		width = int64(18 + (hash>>8)%5)
		depth = int64(16 + (hash>>16)%5)
	case featureRenovation:
		width = int64(23 + (hash>>8)%7)
		depth = int64(19 + (hash>>16)%7)
	case featureWindowRoom:
		width = int64(20 + (hash>>8)%5)
		depth = int64(17 + (hash>>16)%5)
	case featureStorage:
		width = int64(16 + (hash>>8)%5)
		depth = int64(14 + (hash>>16)%5)
	case featureClassroom:
		width = int64(24 + (hash>>8)%5)
		depth = int64(20 + (hash>>16)%5)
	case featureMachineRoom:
		width = int64(18 + (hash>>8)%5)
		depth = int64(16 + (hash>>16)%5)
	}

	const margin = int64(6)

	quadrant := (hash >> 24) % 4

	x0 := margin
	z0 := margin
	if quadrant == 1 || quadrant == 3 {
		x0 = zoneSize - margin - width
	}
	if quadrant >= 2 {
		z0 = zoneSize - margin - depth
	}

	x1 := x0 + width - 1
	z1 := z0 + depth - 1

	var entranceSide featureSide
	switch quadrant {
	case 0:
		if hash&(1<<40) == 0 {
			entranceSide = featureEast
		} else {
			entranceSide = featureSouth
		}
	case 1:
		if hash&(1<<40) == 0 {
			entranceSide = featureWest
		} else {
			entranceSide = featureSouth
		}
	case 2:
		if hash&(1<<40) == 0 {
			entranceSide = featureEast
		} else {
			entranceSide = featureNorth
		}
	default:
		if hash&(1<<40) == 0 {
			entranceSide = featureWest
		} else {
			entranceSide = featureNorth
		}
	}

	jitter := int64((hash>>48)%5) - 2
	doorCenter := (z0+z1)/2 + jitter
	if entranceSide == featureNorth || entranceSide == featureSouth {
		doorCenter = (x0+x1)/2 + jitter
	}

	if entranceSide == featureNorth || entranceSide == featureSouth {
		doorCenter = clamp(doorCenter, x0+2, x1-2)
	} else {
		doorCenter = clamp(doorCenter, z0+2, z1-2)
	}

	return featureRoom{
		x0:           x0,
		x1:           x1,
		z0:           z0,
		z1:           z1,
		entranceSide: entranceSide,
		doorCenter:   doorCenter,
	}
}

func roomFeatureStructureAt(current zone, room featureRoom, doorWidth int64) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if !rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		return structureOpen, true
	}

	if roomEntranceAt(current, room, doorWidth) {
		return structureDoorway, true
	}

	return structureWall, true
}

func bathroomStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 1) {
			return structureDoorway, true
		}

		return structureWall, true
	}

	hash := mix64(current.hash ^ saltFurniture ^ 0x41d7c9a2be830f65)
	verticalStalls := hash&1 == 0

	if verticalStalls {
		front := room.z1 - 6
		back := room.z1 - 1

		for x := room.x0 + 3; x <= room.x1-3; x += 4 {
			if current.localX == x && current.localZ >= front && current.localZ <= back {
				return structurePartition, true
			}
		}

		if current.localZ == front && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
			if _, door := bathroomStallDoorIndexAt(current, room, true); door {
				return structureOpen, true
			}

			return structurePartition, true
		}
	} else {
		front := room.x1 - 6
		back := room.x1 - 1

		for z := room.z0 + 3; z <= room.z1-3; z += 4 {
			if current.localZ == z && current.localX >= front && current.localX <= back {
				return structurePartition, true
			}
		}

		if current.localX == front && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
			if _, door := bathroomStallDoorIndexAt(current, room, false); door {
				return structureOpen, true
			}

			return structurePartition, true
		}
	}

	return structureOpen, true
}

func bathroomStallDoorIndexAt(current zone, room featureRoom, vertical bool) (int, bool) {
	if vertical {
		front := room.z1 - 6
		if current.localZ != front {
			return 0, false
		}

		for dividerX := room.x0 + 3; dividerX <= room.x1-3; dividerX += 4 {
			doorX := dividerX + 2
			if doorX >= room.x1-1 {
				continue
			}
			if current.localX == doorX {
				return int((dividerX - (room.x0 + 3)) / 4), true
			}
		}

		return 0, false
	}

	front := room.x1 - 6
	if current.localX != front {
		return 0, false
	}

	for dividerZ := room.z0 + 3; dividerZ <= room.z1-3; dividerZ += 4 {
		doorZ := dividerZ + 2
		if doorZ >= room.z1-1 {
			continue
		}
		if current.localZ == doorZ {
			return int((dividerZ - (room.z0 + 3)) / 4), true
		}
	}

	return 0, false
}

func renovationStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 8) {
			return structureOpen, true
		}

		hash := mix64(current.hash ^ saltFeature ^ 0x72e84b15c09f36da)
		if hash%3 == 0 {
			gapCenter := int64(8 + (hash>>12)%48)
			coordinate := current.localX
			if current.localX == room.x0 || current.localX == room.x1 {
				coordinate = current.localZ
			}

			if abs64(coordinate-gapCenter) <= 2 {
				return structureOpen, true
			}
		}

		return structureWall, true
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0
	hash := mix64(current.hash ^ saltFeature ^ 0xd53a106cf8be2479)

	if floorMod(relativeX+int64(hash%7), 9) == 4 && relativeZ >= 3 && relativeZ <= room.z1-room.z0-3 {
		if floorMod(relativeZ+int64((hash>>8)%11), 13) > 4 {
			return structurePartition, true
		}
	}

	if floorMod(relativeZ+int64((hash>>16)%7), 11) == 5 && relativeX >= 3 && relativeX <= room.x1-room.x0-3 {
		if floorMod(relativeX+int64((hash>>24)%9), 14) > 6 {
			return structureWall, true
		}
	}

	return structureOpen, true
}

func receptionStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 7) {
			return structureOpen, true
		}

		return structureWall, true
	}

	return structureOpen, true
}

func doorGalleryStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 5) {
			return structureOpen, true
		}

		return structureWall, true
	}

	vertical, line, direction := doorGalleryWall(current, room)
	if index, ok := doorGalleryDoorIndex(current, room, vertical, line); ok {
		_ = index
		return structureDoorway, true
	}

	if vertical {
		if current.localX == line && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
			return structureWall, true
		}
	} else if current.localZ == line && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
		return structureWall, true
	}

	for index := 0; index < 3; index++ {
		if !doorGalleryDoorIsFalse(current, index) {
			continue
		}

		coordinate := doorGalleryDoorCoordinate(room, vertical, index)
		if vertical {
			if current.localX == line+direction && current.localZ == coordinate {
				return structureWall, true
			}
		} else if current.localZ == line+direction && current.localX == coordinate {
			return structureWall, true
		}
	}

	return structureOpen, true
}

func doorGalleryWall(current zone, room featureRoom) (bool, int64, int64) {
	hash := mix64(current.hash ^ saltFeature ^ 0xf3210bc4795a6de8)
	vertical := hash&1 == 0
	direction := int64(1)
	if hash&(1<<8) != 0 {
		direction = -1
	}

	if vertical {
		return true, (room.x0 + room.x1) / 2, direction
	}

	return false, (room.z0 + room.z1) / 2, direction
}

func doorGalleryDoorIndex(current zone, room featureRoom, vertical bool, line int64) (int, bool) {
	if vertical {
		if current.localX != line {
			return 0, false
		}
	} else if current.localZ != line {
		return 0, false
	}

	for index := 0; index < 3; index++ {
		coordinate := doorGalleryDoorCoordinate(room, vertical, index)
		if vertical && current.localZ == coordinate {
			return index, true
		}
		if !vertical && current.localX == coordinate {
			return index, true
		}
	}

	return 0, false
}

func doorGalleryDoorCoordinate(room featureRoom, vertical bool, index int) int64 {
	var minimum, maximum int64
	if vertical {
		minimum = room.z0 + 3
		maximum = room.z1 - 3
	} else {
		minimum = room.x0 + 3
		maximum = room.x1 - 3
	}

	switch index {
	case 0:
		return minimum
	case 1:
		return (minimum + maximum) / 2
	default:
		return maximum
	}
}

func doorGalleryDoorIsFalse(current zone, index int) bool {
	if index == 0 {
		return true
	}
	if index == 1 {
		return false
	}

	hash := mix64(current.hash ^ saltFeature ^ 0x861d4e39ba7025cf)
	return hash&1 == 0
}

func featureFloorBlock(current zone, blocks paletteBlocks) (game.Block, bool) {
	if current.feature == featureNone {
		return game.Air, false
	}

	room := featureRoomForZone(current)
	if !roomContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	switch current.feature {
	case featureDarkRoom:
		return blocks.wear, true
	case featureArchive:
		if floorMod(current.localX+current.localZ, 5) == 0 {
			return blocks.wear, true
		}
	case featureBathroom:
		return game.SmoothQuartz, true
	case featureRenovation, featureMachineRoom:
		return game.SmoothStone, true
	case featureStorage:
		return game.GrayWool, true
	case featureWindowRoom:
		return game.LightGrayWool, true
	}

	return game.Air, false
}

func darkRoomAt(current zone) bool {
	if current.feature != featureDarkRoom {
		return false
	}

	room := featureRoomForZone(current)
	return roomContains(room, current.localX, current.localZ)
}

func featureBlockAt(seed, worldX, worldY, worldZ int64, current zone) (game.Block, bool) {
	if current.feature == featureNone || zoneSpineOpenAt(seed, current) {
		return game.Air, false
	}

	room := featureRoomForZone(current)
	if !roomContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	switch current.feature {
	case featureLibrary:
		return libraryBlockAt(seed, worldX, worldY, worldZ, current, room)
	case featureArchive:
		return archiveBlockAt(seed, worldX, worldY, worldZ, current, room)
	case featureReception:
		return receptionBlockAt(worldY, current, room)
	case featureDarkRoom:
		return roomDoorBlockAt(game.OakDoor, worldY, current, room, 1)
	case featureDoorGallery:
		return doorGalleryBlockAt(worldY, current, room)
	case featureServiceRoom:
		return serviceRoomBlockAt(worldY, current, room)
	case featureConference:
		return conferenceBlockAt(worldY, current, room)
	case featureBathroom:
		return bathroomBlockAt(worldY, current, room)
	case featureRenovation:
		return renovationBlockAt(worldY, current, room)
	case featureWindowRoom:
		return windowRoomBlockAt(worldY, current, room)
	case featureStorage:
		return storageBlockAt(worldY, current, room)
	case featureClassroom:
		return classroomBlockAt(worldY, current, room)
	case featureMachineRoom:
		return machineRoomBlockAt(worldY, current, room)
	default:
		return game.Air, false
	}
}

func libraryBlockAt(seed, worldX, worldY, worldZ int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 2); ok {
		return block, true
	}

	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2
	if worldY == int64(floorY+1) && current.localX == centerX && current.localZ == centerZ {
		return game.Lectern, true
	}

	hash := mix64(current.hash ^ saltFurniture)
	verticalRows := hash&1 == 0

	if verticalRows {
		relativeX := current.localX - room.x0
		if relativeX >= 3 && floorMod(relativeX-3, 5) == 0 && current.localZ >= room.z0+3 && current.localZ <= room.z1-3 {
			return bookshelfBlock(seed, worldX, worldZ), true
		}
	} else {
		relativeZ := current.localZ - room.z0
		if relativeZ >= 3 && floorMod(relativeZ-3, 5) == 0 && current.localX >= room.x0+3 && current.localX <= room.x1-3 {
			return bookshelfBlock(seed, worldX, worldZ), true
		}
	}

	if current.localZ == room.z0+1 && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
		return bookshelfBlock(seed, worldX, worldZ), true
	}

	return game.Air, false
}

func archiveBlockAt(seed, worldX, worldY, worldZ int64, current zone, room featureRoom) (game.Block, bool) {
	door := game.OakDoor
	if mix64(current.hash^saltFurniture)%4 == 0 {
		door = game.IronDoor
	}

	if block, ok := roomDoorBlockAt(door, worldY, current, room, 1); ok {
		return block, true
	}

	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	hash := mix64(current.hash ^ saltFurniture ^ 0x247ebca5d109836f)
	verticalRows := hash&1 == 0

	if verticalRows {
		relativeX := current.localX - room.x0
		if relativeX >= 2 && floorMod(relativeX-2, 4) == 0 && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
			return archiveShelfBlock(seed, worldX, worldZ), true
		}
	} else {
		relativeZ := current.localZ - room.z0
		if relativeZ >= 2 && floorMod(relativeZ-2, 4) == 0 && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
			return archiveShelfBlock(seed, worldX, worldZ), true
		}
	}

	return game.Air, false
}

func receptionBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	counter := false
	lectern := false

	switch room.entranceSide {
	case featureNorth:
		line := room.z0 + 5
		counter = current.localZ == line && abs64(current.localX-centerX) <= 5
		counter = counter || current.localX == centerX+5 && current.localZ >= line && current.localZ <= line+4
		lectern = current.localX == centerX && current.localZ == line
	case featureSouth:
		line := room.z1 - 5
		counter = current.localZ == line && abs64(current.localX-centerX) <= 5
		counter = counter || current.localX == centerX-5 && current.localZ <= line && current.localZ >= line-4
		lectern = current.localX == centerX && current.localZ == line
	case featureEast:
		line := room.x1 - 5
		counter = current.localX == line && abs64(current.localZ-centerZ) <= 5
		counter = counter || current.localZ == centerZ+5 && current.localX <= line && current.localX >= line-4
		lectern = current.localX == line && current.localZ == centerZ
	case featureWest:
		line := room.x0 + 5
		counter = current.localX == line && abs64(current.localZ-centerZ) <= 5
		counter = counter || current.localZ == centerZ-5 && current.localX >= line && current.localX <= line+4
		lectern = current.localX == line && current.localZ == centerZ
	}

	if lectern && worldY == int64(floorY+2) {
		return game.Lectern, true
	}

	if counter && worldY == int64(floorY+1) {
		return game.OakPlanks, true
	}

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if receptionBackdropAt(current, room) {
			return game.Bookshelf, true
		}
	}

	return game.Air, false
}

func receptionBackdropAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1-1 && abs64(current.localX-centerX) <= 4
	case featureSouth:
		return current.localZ == room.z0+1 && abs64(current.localX-centerX) <= 4
	case featureEast:
		return current.localX == room.x0+1 && abs64(current.localZ-centerZ) <= 4
	default:
		return current.localX == room.x1-1 && abs64(current.localZ-centerZ) <= 4
	}
}

func serviceRoomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	door := game.OakDoor
	if mix64(current.hash^saltFurniture)&1 == 0 {
		door = game.IronDoor
	}

	if block, ok := roomDoorBlockAt(door, worldY, current, room, 1); ok {
		return block, true
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	if serviceShelfAt(current, room) {
		return game.OakPlanks, true
	}

	return game.Air, false
}

func serviceShelfAt(current zone, room featureRoom) bool {
	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1-1 && current.localX >= room.x0+2 && current.localX <= room.x1-2
	case featureSouth:
		return current.localZ == room.z0+1 && current.localX >= room.x0+2 && current.localX <= room.x1-2
	case featureEast:
		return current.localX == room.x0+1 && current.localZ >= room.z0+2 && current.localZ <= room.z1-2
	default:
		return current.localX == room.x1-1 && current.localZ >= room.z0+2 && current.localZ <= room.z1-2
	}
}

func conferenceBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 2); ok {
		return block, true
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2
	horizontal := room.x1-room.x0 >= room.z1-room.z0

	if horizontal {
		tableHalf := min(int64(7), (room.x1-room.x0)/2-4)
		if abs64(current.localX-centerX) <= tableHalf && abs64(current.localZ-centerZ) <= 1 {
			if current.localX == centerX-tableHalf && current.localZ == centerZ {
				return game.Lectern, true
			}

			return game.OakSlab, true
		}

		if abs64(current.localZ-centerZ) == 3 && abs64(current.localX-centerX) <= tableHalf && floorMod(current.localX-centerX, 3) == 0 {
			facing := "south"
			if current.localZ > centerZ {
				facing = "north"
			}

			return stairBlock(game.OakStairs, facing), true
		}
	} else {
		tableHalf := min(int64(7), (room.z1-room.z0)/2-4)
		if abs64(current.localZ-centerZ) <= tableHalf && abs64(current.localX-centerX) <= 1 {
			if current.localZ == centerZ-tableHalf && current.localX == centerX {
				return game.Lectern, true
			}

			return game.OakSlab, true
		}

		if abs64(current.localX-centerX) == 3 && abs64(current.localZ-centerZ) <= tableHalf && floorMod(current.localZ-centerZ, 3) == 0 {
			facing := "east"
			if current.localX > centerX {
				facing = "west"
			}

			return stairBlock(game.OakStairs, facing), true
		}
	}

	return game.Air, false
}

func bathroomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.IronDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	hash := mix64(current.hash ^ saltFurniture ^ 0x41d7c9a2be830f65)
	verticalStalls := hash&1 == 0

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if index, door := bathroomStallDoorIndexAt(current, room, verticalStalls); door {
			if index == 1 && hash&(1<<12) != 0 {
				return game.Air, true
			}

			hinge := "left"
			if index%2 != 0 {
				hinge = "right"
			}

			facing := "north"
			if !verticalStalls {
				facing = "west"
			}

			return actualDoorBlock(game.OakDoor, worldY, facing, hinge)
		}
	}

	if worldY != int64(floorY+1) {
		return game.Air, false
	}

	if verticalStalls {
		if current.localZ == room.z0+2 && current.localX >= room.x0+3 && current.localX <= room.x1-3 && floorMod(current.localX-room.x0, 3) == 0 {
			return game.SmoothQuartz, true
		}
	} else if current.localX == room.x0+2 && current.localZ >= room.z0+3 && current.localZ <= room.z1-3 && floorMod(current.localZ-room.z0, 3) == 0 {
		return game.SmoothQuartz, true
	}

	return game.Air, false
}

func renovationBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0
	hash := mix64(current.hash ^ saltFurniture ^ 0x1c97e406b53af82d)

	post := floorMod(relativeX+int64(hash%5), 8) == 3 && floorMod(relativeZ+int64((hash>>8)%5), 8) == 3
	if post {
		if worldY == int64(floorY+1) && (relativeX+relativeZ)%3 == 0 {
			return game.YellowTerracotta, true
		}

		return game.OakPlanks, true
	}

	if worldY == int64(floorY+1) && floorMod(relativeX+relativeZ+int64(hash%7), 23) == 0 {
		return game.OakPlanks, true
	}

	return game.Air, false
}

func windowRoomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if windowWallAt(current, room) {
			return game.TintedGlass, true
		}
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		if current.localZ == room.z1-3 && abs64(current.localX-centerX) <= 3 {
			return game.OakSlab, true
		}
	case featureSouth:
		if current.localZ == room.z0+3 && abs64(current.localX-centerX) <= 3 {
			return game.OakSlab, true
		}
	case featureEast:
		if current.localX == room.x0+3 && abs64(current.localZ-centerZ) <= 3 {
			return game.OakSlab, true
		}
	case featureWest:
		if current.localX == room.x1-3 && abs64(current.localZ-centerZ) <= 3 {
			return game.OakSlab, true
		}
	}

	return game.Air, false
}

func windowWallAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1 && abs64(current.localX-centerX) <= 5
	case featureSouth:
		return current.localZ == room.z0 && abs64(current.localX-centerX) <= 5
	case featureEast:
		return current.localX == room.x0 && abs64(current.localZ-centerZ) <= 5
	default:
		return current.localX == room.x1 && abs64(current.localZ-centerZ) <= 5
	}
}

func storageBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.IronDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	if current.localX == centerX && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
		openingCenter := (room.z0 + room.z1) / 2
		if abs64(current.localZ-openingCenter) > 1 && (worldY == int64(floorY+1) || worldY == int64(floorY+2)) {
			return game.IronBars, true
		}
	}

	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0
	crate := floorMod(relativeX, 6) <= 1 && floorMod(relativeZ, 6) <= 1 && relativeX >= 3 && relativeZ >= 3
	if !crate {
		return game.Air, false
	}

	hash := mix64(current.hash ^ uint64(relativeX*131+relativeZ*719) ^ saltFurniture)
	if worldY == int64(floorY+2) && hash%3 != 0 {
		return game.Air, false
	}

	return game.OakPlanks, true
}

func classroomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 2); ok {
		return block, true
	}

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if classroomBoardAt(current, room) {
			return game.BlackWool, true
		}
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	if classroomTeacherPositionAt(current, room) {
		return game.Lectern, true
	}

	horizontalRows := room.entranceSide == featureNorth || room.entranceSide == featureSouth
	if horizontalRows {
		relativeX := current.localX - room.x0
		relativeZ := current.localZ - room.z0
		if relativeX >= 4 && relativeX <= room.x1-room.x0-4 && relativeZ >= 6 && relativeZ <= room.z1-room.z0-5 {
			if floorMod(relativeX, 4) == 1 && floorMod(relativeZ, 4) == 1 {
				return game.OakSlab, true
			}
			if floorMod(relativeX, 4) == 1 && floorMod(relativeZ, 4) == 2 {
				facing := "north"
				if room.entranceSide == featureNorth {
					facing = "south"
				}
				return stairBlock(game.OakStairs, facing), true
			}
		}
	} else {
		relativeX := current.localX - room.x0
		relativeZ := current.localZ - room.z0
		if relativeZ >= 4 && relativeZ <= room.z1-room.z0-4 && relativeX >= 6 && relativeX <= room.x1-room.x0-5 {
			if floorMod(relativeZ, 4) == 1 && floorMod(relativeX, 4) == 1 {
				return game.OakSlab, true
			}
			if floorMod(relativeZ, 4) == 1 && floorMod(relativeX, 4) == 2 {
				facing := "west"
				if room.entranceSide == featureWest {
					facing = "east"
				}
				return stairBlock(game.OakStairs, facing), true
			}
		}
	}

	_ = centerX
	_ = centerZ
	return game.Air, false
}

func classroomBoardAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1 && abs64(current.localX-centerX) <= 4
	case featureSouth:
		return current.localZ == room.z0 && abs64(current.localX-centerX) <= 4
	case featureEast:
		return current.localX == room.x0 && abs64(current.localZ-centerZ) <= 4
	default:
		return current.localX == room.x1 && abs64(current.localZ-centerZ) <= 4
	}
}

func classroomTeacherPositionAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localX == centerX && current.localZ == room.z1-3
	case featureSouth:
		return current.localX == centerX && current.localZ == room.z0+3
	case featureEast:
		return current.localX == room.x0+3 && current.localZ == centerZ
	default:
		return current.localX == room.x1-3 && current.localZ == centerZ
	}
}

func machineRoomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.IronDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0
	machine := relativeX >= 3 && relativeZ >= 3 && floorMod(relativeX, 6) <= 1 && floorMod(relativeZ, 6) <= 1

	if machine && (worldY == int64(floorY+1) || worldY == int64(floorY+2)) {
		if worldY == int64(floorY+2) && (relativeX+relativeZ)%3 == 0 {
			return game.IronBars, true
		}

		return game.CopperBlock, true
	}

	if worldY == int64(floorY+2) && floorMod(relativeX+2, 7) == 3 && floorMod(relativeZ+1, 5) == 2 {
		return game.IronBars, true
	}

	return game.Air, false
}

func doorGalleryBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	vertical, line, direction := doorGalleryWall(current, room)
	index, ok := doorGalleryDoorIndex(current, room, vertical, line)
	if !ok {
		return game.Air, false
	}

	facing := "south"
	if vertical {
		if direction > 0 {
			facing = "east"
		} else {
			facing = "west"
		}
	} else if direction < 0 {
		facing = "north"
	}

	door := game.OakDoor
	if index == 2 && mix64(current.hash^saltFurniture)&1 == 0 {
		door = game.IronDoor
	}

	hinge := "left"
	if index%2 != 0 {
		hinge = "right"
	}

	return actualDoorBlock(door, worldY, facing, hinge)
}

func ambientDoorSpecForZone(seed int64, current zone) ambientDoorSpec {
	if current.feature != featureNone || current.vertical != verticalNone {
		return ambientDoorSpec{}
	}

	hash := mix64(current.hash ^ saltOddity)
	if hash%100 >= 4 {
		return ambientDoorSpec{}
	}

	for attempt := uint64(0); attempt < 6; attempt++ {
		candidate := mix64(hash ^ (attempt+1)*0x9e3779b97f4a7c15)
		vertical := candidate&1 == 0
		line := int64(11 + (candidate>>8)%42)
		center := int64(11 + (candidate>>16)%42)
		direction := int64(1)
		if candidate&(1<<24) != 0 {
			direction = -1
		}

		falseDoor := (candidate>>32)%100 < 62
		iron := (candidate>>40)%100 < 18

		doorProbe := current
		backProbe := current
		if vertical {
			doorProbe.localX = line
			doorProbe.localZ = center
			backProbe.localX = line + direction
			backProbe.localZ = center
		} else {
			doorProbe.localX = center
			doorProbe.localZ = line
			backProbe.localX = center
			backProbe.localZ = line + direction
		}

		if zoneSpineOpenAt(seed, doorProbe) || zoneSpineOpenAt(seed, backProbe) {
			continue
		}

		if !falseDoor {
			clear := true
			for distance := int64(1); distance <= 2; distance++ {
				probe := current
				if vertical {
					probe.localX = line + direction*distance
					probe.localZ = center
				} else {
					probe.localX = center
					probe.localZ = line + direction*distance
				}

				if zoneSpineOpenAt(seed, probe) {
					continue
				}

				profile := mergeStructure(layoutStructureAt(seed, probe), motifStructureAt(seed, probe))
				if profile == structureWall || profile == structurePillar || profile == structurePartition {
					clear = false
					break
				}
			}

			if !clear {
				continue
			}
		}

		return ambientDoorSpec{
			enabled:   true,
			vertical:  vertical,
			line:      line,
			center:    center,
			direction: direction,
			falseDoor: falseDoor,
			iron:      iron,
		}
	}

	return ambientDoorSpec{}
}

func ambientDoorStructureAt(seed int64, current zone) (structure, bool) {
	spec := ambientDoorSpecForZone(seed, current)
	if !spec.enabled {
		return structureOpen, false
	}

	if spec.vertical {
		if current.localX == spec.line && abs64(current.localZ-spec.center) <= 5 {
			if current.localZ == spec.center {
				return structureDoorway, true
			}

			return structureWall, true
		}

		if spec.falseDoor && current.localX == spec.line+spec.direction && current.localZ == spec.center {
			return structureWall, true
		}
	} else {
		if current.localZ == spec.line && abs64(current.localX-spec.center) <= 5 {
			if current.localX == spec.center {
				return structureDoorway, true
			}

			return structureWall, true
		}

		if spec.falseDoor && current.localZ == spec.line+spec.direction && current.localX == spec.center {
			return structureWall, true
		}
	}

	return structureOpen, false
}

func ambientDoorBlockAt(seed, worldY int64, current zone) (game.Block, bool) {
	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	spec := ambientDoorSpecForZone(seed, current)
	if !spec.enabled {
		return game.Air, false
	}

	onDoor := false
	facing := "south"
	if spec.vertical {
		onDoor = current.localX == spec.line && current.localZ == spec.center
		if spec.direction > 0 {
			facing = "east"
		} else {
			facing = "west"
		}
	} else {
		onDoor = current.localZ == spec.line && current.localX == spec.center
		if spec.direction < 0 {
			facing = "north"
		}
	}

	if !onDoor {
		return game.Air, false
	}

	door := game.OakDoor
	if spec.iron {
		door = game.IronDoor
	}

	hinge := "left"
	if mix64(current.hash^saltOddity^0x75be29c104f8da63)&1 != 0 {
		hinge = "right"
	}

	return actualDoorBlock(door, worldY, facing, hinge)
}

func roomDoorBlockAt(base game.Block, worldY int64, current zone, room featureRoom, width int64) (game.Block, bool) {
	index, ok := roomEntranceIndex(current, room, width)
	if !ok {
		return game.Air, false
	}

	hinge := "left"
	if width > 1 && index%2 != 0 {
		hinge = "right"
	} else if width == 1 && mix64(current.hash^saltFurniture)&1 != 0 {
		hinge = "right"
	}

	return actualDoorBlock(base, worldY, doorFacingForSide(room.entranceSide), hinge)
}

func actualDoorBlock(base game.Block, worldY int64, facing, hinge string) (game.Block, bool) {
	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	half := "lower"
	if worldY == int64(floorY+2) {
		half = "upper"
	}

	block, ok := base.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: facing},
		game.BlockPropertyValue{Name: "half", Value: half},
		game.BlockPropertyValue{Name: "hinge", Value: hinge},
		game.BlockPropertyValue{Name: "open", Value: "false"},
	)
	if !ok {
		return base, true
	}

	return block, true
}

func roomEntranceIndex(current zone, room featureRoom, width int64) (int64, bool) {
	if !roomEntranceAt(current, room, width) {
		return 0, false
	}

	coordinate := current.localZ
	if room.entranceSide == featureNorth || room.entranceSide == featureSouth {
		coordinate = current.localX
	}

	start := room.doorCenter - width/2
	return coordinate - start, true
}

func roomEntranceAt(current zone, room featureRoom, width int64) bool {
	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z0 && withinOpening(current.localX, room.doorCenter, width)
	case featureEast:
		return current.localX == room.x1 && withinOpening(current.localZ, room.doorCenter, width)
	case featureSouth:
		return current.localZ == room.z1 && withinOpening(current.localX, room.doorCenter, width)
	default:
		return current.localX == room.x0 && withinOpening(current.localZ, room.doorCenter, width)
	}
}

func doorFacingForSide(side featureSide) string {
	switch side {
	case featureNorth:
		return "south"
	case featureEast:
		return "west"
	case featureSouth:
		return "north"
	default:
		return "east"
	}
}

func bookshelfBlock(seed, worldX, worldZ int64) game.Block {
	hash := coordinateHash(seed, worldX, worldZ, saltFurniture)
	if hash%5 == 0 {
		return game.ChiseledBookshelf
	}

	return game.Bookshelf
}

func archiveShelfBlock(seed, worldX, worldZ int64) game.Block {
	hash := coordinateHash(seed, worldX, worldZ, saltFurniture^0x5f208ca4d16e739b)
	if hash%3 != 0 {
		return game.ChiseledBookshelf
	}

	return game.Bookshelf
}

func roomContains(room featureRoom, x, z int64) bool {
	return x >= room.x0 && x <= room.x1 && z >= room.z0 && z <= room.z1
}

func roomInteriorContains(room featureRoom, x, z int64) bool {
	return x > room.x0 && x < room.x1 && z > room.z0 && z < room.z1
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
