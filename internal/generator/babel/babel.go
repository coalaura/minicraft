package babel

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "babel"

	foundationMinY = int32(62)
	baseFloorY     = int32(64)
	maxBuildY      = int32(264)

	lotScale       = int64(48)
	boulevardScale = int64(96)
	districtScale  = int64(192)

	streetHalfWidth    = int64(3)
	boulevardHalfWidth = int64(5)
	grandHalfWidth     = int64(7)

	walkwayHalfWidth = int64(3)

	streetWalkHalfWidth    = int64(6)
	boulevardWalkHalfWidth = int64(8)
	grandWalkHalfWidth     = int64(10)

	grandPlazaRadius = int64(14)

	skywayFloorY = int32(86)
	skywayBeamY  = int32(91)
)

type lotKind uint8

const (
	lotTower lotKind = iota
	lotCourtyard
	lotPlaza
)

type cityPalette struct {
	wall   game.Block
	wall2  game.Block
	trim   game.Block
	glass  game.Block
	floor  game.Block
	accent game.Block
	light  game.Block
}

type lotDescription struct {
	kind        lotKind
	palette     cityPalette
	hash        uint64
	baseInset   int64
	height      int32
	floorHeight int32
}

type streetState struct {
	grandX     bool
	grandZ     bool
	boulevardX bool
	boulevardZ bool
	streetX    bool
	streetZ    bool
}

type preparedSkybridge struct {
	palette        cityPalette
	bridgeY        int32
	span           int64
	crossingOffset int64
	widthOffset    int64
	valid          bool
}

type preparedColumn struct {
	relativeX  int64
	relativeZ  int64
	localX     int64
	localZ     int64
	streets    streetState
	lot        lotDescription
	xSkybridge preparedSkybridge
	zSkybridge preparedSkybridge
}

type generatedChunk struct {
	seed    int64
	columns [game.ChunkWidth * game.ChunkWidth]preparedColumn
}

type Generator struct{}

var palettes = [...]cityPalette{
	{
		wall:   game.QuartzBlock,
		wall2:  game.SmoothStone,
		trim:   game.QuartzPillar,
		glass:  game.LightBlueStainedGlass,
		floor:  game.PolishedDiorite,
		accent: game.GoldBlock,
		light:  game.SeaLantern,
	},
	{
		wall:   game.StoneBricks,
		wall2:  game.PolishedAndesite,
		trim:   game.IronBlock,
		glass:  game.GrayStainedGlass,
		floor:  game.SmoothStone,
		accent: game.RedstoneBlock,
		light:  game.Glowstone,
	},
	{
		wall:   game.BlackConcrete,
		wall2:  game.PolishedAndesite,
		trim:   game.Obsidian,
		glass:  game.PurpleStainedGlass,
		floor:  game.GrayConcrete,
		accent: game.PurpurBlock,
		light:  game.SeaLantern,
	},
	{
		wall:   game.PrismarineBricks,
		wall2:  game.DarkPrismarine,
		trim:   game.QuartzBlock,
		glass:  game.CyanStainedGlass,
		floor:  game.Prismarine,
		accent: game.SeaLantern,
		light:  game.SeaLantern,
	},
	{
		wall:   game.Bricks,
		wall2:  game.MudBricks,
		trim:   game.StoneBricks,
		glass:  game.OrangeStainedGlass,
		floor:  game.PolishedGranite,
		accent: game.GoldBlock,
		light:  game.Glowstone,
	},
	{
		wall:   game.EndStoneBricks,
		wall2:  game.PurpurBlock,
		trim:   game.QuartzPillar,
		glass:  game.MagentaStainedGlass,
		floor:  game.SmoothQuartz,
		accent: game.SeaLantern,
		light:  game.SeaLantern,
	},
	{
		wall:   game.WhiteConcrete,
		wall2:  game.LightGrayConcrete,
		trim:   game.BlackConcrete,
		glass:  game.BlueStainedGlass,
		floor:  game.SmoothStone,
		accent: game.CyanConcrete,
		light:  game.SeaLantern,
	},
}

var (
	_ game.Generator        = Generator{}
	_ game.SectionGenerator = Generator{}
	_ game.ChunkGenerator   = Generator{}
	_ game.BoundedGenerator = Generator{}
	_ game.SpawnGenerator   = Generator{}
	_ game.GeneratedChunk   = (*generatedChunk)(nil)
)

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New() game.Generator {
	return Generator{}
}

func (Generator) BlockAt(seed int64, position game.BlockPosition) game.Block {
	originX, originZ := cityOrigins(seed)

	return blockAt(seed, position, originX, originZ)
}

func (Generator) GenerateChunk(seed int64, chunk game.ChunkPosition) game.GeneratedChunk {
	generated := prepareChunk(seed, chunk)
	return &generated
}

func (Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	generated := prepareChunk(seed, chunk)
	return generated.GenerateSection(sectionMinY, blocks)
}

func (generated *generatedChunk) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < foundationMinY || sectionMinY > maxBuildY {
		return game.Air, true
	}

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				column := &generated.columns[localZ*game.ChunkWidth+localX]
				block := blockForPreparedColumn(generated.seed, worldY, column)

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

func (Generator) Spawn(seed int64) game.Position {
	originX, originZ := cityOrigins(seed)

	return game.Position{
		X: float64(originX) + 0.5,
		Y: float64(baseFloorY) + 1,
		Z: float64(originZ) + 0.5,
	}
}

func newRegistered() (game.Generator, error) {
	return New(), nil
}

func prepareChunk(seed int64, chunk game.ChunkPosition) generatedChunk {
	originX, originZ := cityOrigins(seed)

	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	generated := generatedChunk{seed: seed}

	for localZ := range int32(game.ChunkWidth) {
		worldZ := chunkMinZ + localZ

		for localX := range int32(game.ChunkWidth) {
			worldX := chunkMinX + localX
			generated.columns[localZ*game.ChunkWidth+localX] = prepareColumn(seed, worldX, worldZ, originX, originZ)
		}
	}

	return generated
}

func prepareColumn(seed int64, worldX, worldZ int32, originX, originZ int64) preparedColumn {
	relativeX := int64(worldX) - originX
	relativeZ := int64(worldZ) - originZ

	cellX := floorDiv(relativeX, lotScale)
	cellZ := floorDiv(relativeZ, lotScale)

	return preparedColumn{
		relativeX:  relativeX,
		relativeZ:  relativeZ,
		localX:     positiveRemainder(relativeX, lotScale),
		localZ:     positiveRemainder(relativeZ, lotScale),
		streets:    classifyStreets(relativeX, relativeZ),
		lot:        describeLot(seed, cellX, cellZ),
		xSkybridge: prepareXSkybridge(seed, relativeX, relativeZ),
		zSkybridge: prepareZSkybridge(seed, relativeX, relativeZ),
	}
}

func prepareXSkybridge(seed int64, relativeX, relativeZ int64) preparedSkybridge {
	distanceX := absolute(gridSignedOffset(relativeX, lotScale))
	if distanceX > 10 {
		return preparedSkybridge{}
	}

	boundaryX := nearestGridIndex(relativeX, lotScale)

	cellZ := floorDiv(relativeZ, lotScale)

	widthOffset := centerDistance(positiveRemainder(relativeZ, lotScale))
	if boundaryX%2 == 0 || widthOffset > 2 {
		return preparedSkybridge{}
	}

	bridgeHash := hashCoordinates(seed, boundaryX, cellZ, 0x6d2b79f5)
	if bridgeHash%4 != 0 {
		return preparedSkybridge{}
	}

	first := describeLot(seed, boundaryX-1, cellZ)
	second := describeLot(seed, boundaryX, cellZ)

	return prepareSkybridge(first, second, bridgeHash, distanceX, widthOffset)
}

func prepareZSkybridge(seed int64, relativeX, relativeZ int64) preparedSkybridge {
	distanceZ := absolute(gridSignedOffset(relativeZ, lotScale))
	if distanceZ > 10 {
		return preparedSkybridge{}
	}

	boundaryZ := nearestGridIndex(relativeZ, lotScale)

	cellX := floorDiv(relativeX, lotScale)

	widthOffset := centerDistance(positiveRemainder(relativeX, lotScale))
	if boundaryZ%2 == 0 || widthOffset > 2 {
		return preparedSkybridge{}
	}

	bridgeHash := hashCoordinates(seed, cellX, boundaryZ, 0x9e3779b9)
	if bridgeHash%4 != 0 {
		return preparedSkybridge{}
	}

	first := describeLot(seed, cellX, boundaryZ-1)
	second := describeLot(seed, cellX, boundaryZ)

	return prepareSkybridge(first, second, bridgeHash, distanceZ, widthOffset)
}

func prepareSkybridge(first, second lotDescription, bridgeHash uint64, crossingOffset, widthOffset int64) preparedSkybridge {
	if first.kind == lotPlaza || second.kind == lotPlaza {
		return preparedSkybridge{}
	}

	bridgeY := baseFloorY + 28 + int32((bridgeHash>>8)%7)*6
	if first.height < bridgeY-baseFloorY+6 || second.height < bridgeY-baseFloorY+6 {
		return preparedSkybridge{}
	}

	bridgeRelativeY := bridgeY - baseFloorY

	return preparedSkybridge{
		palette:        first.palette,
		bridgeY:        bridgeY,
		span:           max(lotOuterInsetAt(first, bridgeRelativeY), lotOuterInsetAt(second, bridgeRelativeY)) + 1,
		crossingOffset: crossingOffset,
		widthOffset:    widthOffset,
		valid:          true,
	}
}

func blockForPreparedColumn(seed int64, worldY int32, column *preparedColumn) game.Block {
	if worldY < foundationMinY || worldY > maxBuildY {
		return game.Air
	}

	if worldY == foundationMinY {
		return game.Stone
	}

	if worldY == foundationMinY+1 {
		if ((column.relativeX>>3)+(column.relativeZ>>3))&1 == 0 {
			return game.StoneBricks
		}

		return game.PolishedAndesite
	}

	if worldY == baseFloorY {
		return preparedSurfaceBlock(column)
	}

	grandBlock := grandIntersectionBlock(seed, worldY, column.relativeX, column.relativeZ)
	if grandBlock != game.Air {
		return grandBlock
	}

	avenueBlock := elevatedAvenueBlock(seed, worldY, column.relativeX, column.relativeZ, column.streets)
	if avenueBlock != game.Air {
		return avenueBlock
	}

	skybridgeBlock, claimed := preparedLocalSkybridgeBlock(worldY, column)
	if claimed {
		return skybridgeBlock
	}

	if column.streets.streetX || column.streets.streetZ || column.streets.boulevardX || column.streets.boulevardZ || column.streets.grandX || column.streets.grandZ {
		return game.Air
	}

	return lotBlock(worldY, column.localX, column.localZ, column.lot)
}

func preparedLocalSkybridgeBlock(worldY int32, column *preparedColumn) (game.Block, bool) {
	if column.xSkybridge.valid {
		bridge := &column.xSkybridge

		block, claimed := preparedSkybridgeBlock(worldY, bridge)
		if claimed {
			return block, true
		}
	}

	if column.zSkybridge.valid {
		bridge := &column.zSkybridge

		block, claimed := preparedSkybridgeBlock(worldY, bridge)
		if claimed {
			return block, true
		}
	}

	return game.Air, false
}

func preparedSkybridgeBlock(worldY int32, bridge *preparedSkybridge) (game.Block, bool) {
	if bridge.crossingOffset > bridge.span || worldY < bridge.bridgeY || worldY > bridge.bridgeY+4 {
		return game.Air, false
	}

	if worldY == bridge.bridgeY {
		if bridge.widthOffset == 0 && bridge.crossingOffset%5 == 0 {
			return bridge.palette.light, true
		}

		return bridge.palette.floor, true
	}

	if worldY < bridge.bridgeY+4 && bridge.widthOffset == 2 {
		return bridge.palette.glass, true
	}

	if worldY == bridge.bridgeY+4 {
		if bridge.crossingOffset%4 == 0 {
			return bridge.palette.trim, true
		}

		return bridge.palette.wall2, true
	}

	return game.Air, true
}

func lotBlock(worldY int32, localX, localZ int64, lot lotDescription) game.Block {
	switch lot.kind {
	case lotPlaza:
		return plazaBlock(worldY, localX, localZ, lot)
	case lotCourtyard:
		return courtyardBlock(worldY, localX, localZ, lot)
	default:
		return towerBlock(worldY, localX, localZ, lot)
	}
}

func blockAt(seed int64, position game.BlockPosition, originX, originZ int64) game.Block {
	if position.Y < foundationMinY || position.Y > maxBuildY {
		return game.Air
	}

	relativeX := int64(position.X) - originX
	relativeZ := int64(position.Z) - originZ

	if position.Y == foundationMinY {
		return game.Stone
	}

	if position.Y == foundationMinY+1 {
		if ((relativeX>>3)+(relativeZ>>3))&1 == 0 {
			return game.StoneBricks
		}

		return game.PolishedAndesite
	}

	streets := classifyStreets(relativeX, relativeZ)

	if position.Y == baseFloorY {
		return surfaceBlock(seed, relativeX, relativeZ, streets)
	}

	grandBlock := grandIntersectionBlock(seed, position.Y, relativeX, relativeZ)
	if grandBlock != game.Air {
		return grandBlock
	}

	avenueBlock := elevatedAvenueBlock(seed, position.Y, relativeX, relativeZ, streets)
	if avenueBlock != game.Air {
		return avenueBlock
	}

	skybridgeBlock, claimed := localSkybridgeBlock(seed, position.Y, relativeX, relativeZ, streets)
	if claimed {
		return skybridgeBlock
	}

	if streets.streetX || streets.streetZ || streets.boulevardX || streets.boulevardZ || streets.grandX || streets.grandZ {
		return game.Air
	}

	cellX := floorDiv(relativeX, lotScale)
	cellZ := floorDiv(relativeZ, lotScale)

	localX := positiveRemainder(relativeX, lotScale)
	localZ := positiveRemainder(relativeZ, lotScale)

	lot := describeLot(seed, cellX, cellZ)

	return lotBlock(position.Y, localX, localZ, lot)
}

func preparedSurfaceBlock(column *preparedColumn) game.Block {
	relativeX := column.relativeX
	relativeZ := column.relativeZ

	streets := column.streets

	grandOffsetX := absolute(gridSignedOffset(relativeX, districtScale))
	grandOffsetZ := absolute(gridSignedOffset(relativeZ, districtScale))

	if grandOffsetX <= grandPlazaRadius && grandOffsetZ <= grandPlazaRadius {
		return grandPlazaSurface(relativeX, relativeZ)
	}

	if streets.grandX || streets.grandZ || streets.boulevardX || streets.boulevardZ || streets.streetX || streets.streetZ {
		return roadSurface(relativeX, relativeZ, streets)
	}

	if nearStreet(relativeX, relativeZ) {
		if ((relativeX>>2)+(relativeZ>>2))&1 == 0 {
			return game.SmoothStone
		}

		return game.StoneBricks
	}

	return lotSurfaceBlock(column.localX, column.localZ, column.lot)
}

func surfaceBlock(seed int64, relativeX, relativeZ int64, streets streetState) game.Block {
	grandOffsetX := absolute(gridSignedOffset(relativeX, districtScale))
	grandOffsetZ := absolute(gridSignedOffset(relativeZ, districtScale))

	if grandOffsetX <= grandPlazaRadius && grandOffsetZ <= grandPlazaRadius {
		return grandPlazaSurface(relativeX, relativeZ)
	}

	if streets.grandX || streets.grandZ || streets.boulevardX || streets.boulevardZ || streets.streetX || streets.streetZ {
		return roadSurface(relativeX, relativeZ, streets)
	}

	if nearStreet(relativeX, relativeZ) {
		if ((relativeX>>2)+(relativeZ>>2))&1 == 0 {
			return game.SmoothStone
		}

		return game.StoneBricks
	}

	cellX := floorDiv(relativeX, lotScale)
	cellZ := floorDiv(relativeZ, lotScale)

	localX := positiveRemainder(relativeX, lotScale)
	localZ := positiveRemainder(relativeZ, lotScale)

	lot := describeLot(seed, cellX, cellZ)

	return lotSurfaceBlock(localX, localZ, lot)
}

func lotSurfaceBlock(localX, localZ int64, lot lotDescription) game.Block {
	if lot.kind == lotPlaza {
		return plazaSurface(localX, localZ, lot)
	}

	if lot.kind == lotCourtyard && insideCourtyard(localX, localZ, lot) {
		centerX := localX - 23
		centerZ := localZ - 24
		radiusSquared := centerX*centerX + centerZ*centerZ

		if radiusSquared <= 36 {
			if radiusSquared >= 25 {
				return lot.palette.trim
			}

			return game.PrismarineBricks
		}

		if centerDistance(localX) <= 2 || centerDistance(localZ) <= 2 {
			return lot.palette.floor
		}

		return game.GrassBlock
	}

	if insideRect(localX, localZ, lot.baseInset) {
		return lot.palette.floor
	}

	if (localX+localZ+int64(lot.hash&7))%7 == 0 {
		return lot.palette.wall2
	}

	return game.SmoothStone
}
