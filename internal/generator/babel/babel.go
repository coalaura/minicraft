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
	originX, originZ := cityOrigins(seed)

	return blockAt(seed, position, originX, originZ)
}
func (Generator) GenerateSection(seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < foundationMinY || sectionMinY > maxBuildY {
		return game.Air, true
	}

	originX, originZ := cityOrigins(seed)
	chunkMinX := chunk.X * game.ChunkWidth
	chunkMinZ := chunk.Z * game.ChunkWidth

	first := game.Air
	uniform := true

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			worldZ := chunkMinZ + localZ

			for localX := range int32(game.ChunkWidth) {
				worldX := chunkMinX + localX
				block := blockAt(seed, game.BlockPosition{X: worldX, Y: worldY, Z: worldZ}, originX, originZ)
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

	if block := grandIntersectionBlock(seed, position.Y, relativeX, relativeZ); block != game.Air {
		return block
	}

	if block := elevatedAvenueBlock(seed, position.Y, relativeX, relativeZ, streets); block != game.Air {
		return block
	}

	if block, claimed := localSkybridgeBlock(seed, position.Y, relativeX, relativeZ, streets); claimed {
		return block
	}

	if streets.streetX || streets.streetZ || streets.boulevardX || streets.boulevardZ || streets.grandX || streets.grandZ {
		return game.Air
	}

	cellX := floorDiv(relativeX, lotScale)
	cellZ := floorDiv(relativeZ, lotScale)
	localX := positiveRemainder(relativeX, lotScale)
	localZ := positiveRemainder(relativeZ, lotScale)
	lot := describeLot(seed, cellX, cellZ)

	switch lot.kind {
	case lotPlaza:
		return plazaBlock(position.Y, localX, localZ, lot)
	case lotCourtyard:
		return courtyardBlock(position.Y, localX, localZ, lot)
	default:
		return towerBlock(position.Y, localX, localZ, lot)
	}
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
