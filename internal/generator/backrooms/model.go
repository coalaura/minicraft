package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
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
