package game

import "strings"

const (
	ChunkWidth    = 16
	SectionVolume = ChunkWidth * ChunkWidth * ChunkWidth
)

type Block uint16

type BlockID uint16

type BlockBehavior uint8

type BlockCollision uint8

type BlockSoundType uint8

type BlockTrait uint32

type BlockFace uint8

const (
	BlockFaceDown BlockFace = iota
	BlockFaceUp
	BlockFaceNorth
	BlockFaceSouth
	BlockFaceWest
	BlockFaceEast
)

const (
	BlockTraitReplaceable BlockTrait = 1 << iota
	BlockTraitDirt
	BlockTraitSnowCannotSurviveOn
	BlockTraitSnowCanSurviveOn
	BlockTraitMineablePickaxe
	BlockTraitMineableShovel
	BlockTraitMineableAxe
	BlockTraitMineableHoe
	BlockTraitIncorrectWoodenTool
	BlockTraitIncorrectStoneTool
	BlockTraitIncorrectCopperTool
	BlockTraitIncorrectIronTool
	BlockTraitIncorrectDiamondTool
	BlockTraitIncorrectGoldTool
	BlockTraitIncorrectNetheriteTool
	BlockTraitSwordInstantlyMines
	BlockTraitSwordEfficient
	BlockTraitLeaves
	BlockTraitWool
)

type BlockDropCount struct {
	Value string
	Count int32
}

type BlockDropRule struct {
	Item          Item
	Count         int32
	CountProperty string
	Counts        []BlockDropCount
	GateProperty  string
	GateValue     string
	RequiresActor bool
}

type BlockMining struct {
	Hardness     float32
	DropRules    []BlockDropRule
	RequiresTool bool
	Destroyable  bool
}

const (
	BlockBehaviorNone BlockBehavior = iota
	BlockBehaviorSolid
	BlockBehaviorHorizontalFacing
	BlockBehaviorSlab
	BlockBehaviorStairs
	BlockBehaviorDoor
	BlockBehaviorTrapdoor
	BlockBehaviorFenceGate
	BlockBehaviorFence
	BlockBehaviorPane
	BlockBehaviorWall
	BlockBehaviorSupported
	BlockBehaviorButton
	BlockBehaviorPlant
	BlockBehaviorPointedDripstone
	BlockBehaviorChest
)

const (
	BlockCollisionNone BlockCollision = iota
	BlockCollisionFull
	BlockCollisionSlab
	BlockCollisionStairs
	BlockCollisionDoor
	BlockCollisionTrapdoor
	BlockCollisionFenceGate
	BlockCollisionFence
	BlockCollisionPane
	BlockCollisionWall
	BlockCollisionCarpet
	BlockCollisionSnow
	BlockCollisionPointedDripstone
	BlockCollisionChain
	BlockCollisionCake
	BlockCollisionChest
)

type BlockProperty struct {
	Name   string
	Values []string
}

type BlockPropertyValue struct {
	Name  string
	Value string
}

type BlockDefinition struct {
	ID              BlockID
	Name            string
	DefaultState    Block
	MinState        Block
	MaxState        Block
	Behavior        BlockBehavior
	Collision       BlockCollision
	Emission        uint8
	LightFilter     uint8
	Sound           BlockSoundType
	Traits          BlockTrait
	BlockEntityType BlockEntityType
	Mining          BlockMining
	Properties      []BlockProperty
	Waterloggable   bool
}

type BlockPosition struct {
	X int32
	Y int32
	Z int32
}

type ChunkPosition struct {
	X int32
	Z int32
}

type Generator interface {
	BlockAt(seed int64, position BlockPosition) Block
}

// SectionGenerator is an optional fast path for generators that can reuse
// work while producing a complete section. sectionMinY is a world coordinate.
// When uniform is true, blocks is ignored and the returned block fills it.
type SectionGenerator interface {
	GenerateSection(seed int64, chunk ChunkPosition, sectionMinY int32, blocks *[SectionVolume]Block) (block Block, uniform bool)
}

// ChunkGenerator is an optional fast path for generators that can prepare
// reusable data for every section and biome sample in a chunk. The returned
// generation is ephemeral and must not be used as authoritative world state.
type ChunkGenerator interface {
	GenerateChunk(seed int64, chunk ChunkPosition) GeneratedChunk
}

type GeneratedChunk interface {
	GenerateSection(sectionMinY int32, blocks *[SectionVolume]Block) (block Block, uniform bool)
}

type GeneratedChunkBiomeGenerator interface {
	BiomeAt(x, y, z int32) Biome
}

// BoundedGenerator optionally reports the inclusive vertical range that can
// contain non-air blocks in a chunk. A false return means the chunk is empty.
type BoundedGenerator interface {
	GenerationBounds(seed int64, chunk ChunkPosition) (minY, maxY int32, ok bool)
}

func (block Block) Valid() bool {
	return block <= MaxBlockState
}

func (block Block) Definition() (BlockDefinition, bool) {
	if !block.Valid() {
		return BlockDefinition{}, false
	}

	return blockDefinitions[stateBlockIDs[block]], true
}

func (block Block) MiningProperties() BlockMining {
	definition, valid := block.Definition()
	if !valid {
		return BlockMining{}
	}

	return definition.Mining
}

func (block Block) IsRedstoneConductor() bool {
	definition, valid := block.Definition()
	if !valid {
		return false
	}

	name := definition.Name
	if name == "soul_sand" || name == "mud" {
		return true
	}

	if redstoneConductorNever(name) {
		return false
	}

	boxes := block.CollisionBoxes(BlockPosition{})
	if len(boxes) != 1 {
		return false
	}

	box := boxes[0]
	return box.MinX == 0 && box.MinY == 0 && box.MinZ == 0 && box.MaxX == 1 && box.MaxY == 1 && box.MaxZ == 1
}

func redstoneConductorNever(name string) bool {
	if strings.HasSuffix(name, "_leaves") || strings.HasSuffix(name, "_stained_glass") {
		return true
	}

	if strings.HasSuffix(name, "copper_grate") || strings.HasSuffix(name, "copper_bulb") {
		return true
	}

	switch name {
	case "glass", "sticky_piston", "piston", "moving_piston", "tnt", "ice", "glowstone", "beacon", "redstone_block", "sea_lantern", "chorus_flower", "frosted_ice", "observer", "bamboo", "scaffolding", "tinted_glass", "powder_snow", "pointed_dripstone":
		return true
	default:
		return false
	}
}

func (block Block) Behavior() BlockBehavior {
	if !block.Valid() {
		return BlockBehaviorNone
	}

	return blockDefinitions[stateBlockIDs[block]].Behavior
}

func (block Block) LightProperties() (emission, filter uint8) {
	if !block.Valid() {
		return 0, 15
	}

	properties := stateLightProperties[block]

	return properties & 0x0f, properties >> 4
}

func (block Block) SoundType() BlockSound {
	if !block.Valid() {
		return BlockSound{}
	}

	return blockSounds[blockDefinitions[stateBlockIDs[block]].Sound]
}

func (block Block) Replaceable() bool {
	if !block.Valid() {
		return false
	}

	return block.HasTrait(BlockTraitReplaceable)
}

func (block Block) HasTrait(trait BlockTrait) bool {
	if !block.Valid() {
		return false
	}

	return blockDefinitions[stateBlockIDs[block]].Traits&trait != 0
}

func (block Block) SameLightProperties(other Block) bool {
	if !block.Valid() || !other.Valid() {
		return block == other
	}

	return stateLightProperties[block] == stateLightProperties[other]
}

func (block Block) Waterloggable() bool {
	definition, valid := block.Definition()
	if !valid {
		return false
	}

	return definition.Waterloggable
}

func (block Block) Property(name string) (string, bool) {
	definition, ok := block.Definition()
	if !ok {
		return "", false
	}

	propertyIndex, ok := definition.PropertyIndex(name)
	if !ok {
		return "", false
	}

	indices := definition.propertyIndices(block)
	return definition.Properties[propertyIndex].Values[indices[propertyIndex]], true
}

func (block Block) WithProperties(values ...BlockPropertyValue) (Block, bool) {
	definition, ok := block.Definition()
	if !ok {
		return 0, false
	}

	indices := definition.propertyIndices(block)

	changed := make([]bool, len(definition.Properties))

	for _, value := range values {
		propertyIndex, found := definition.PropertyIndex(value.Name)
		if !found || changed[propertyIndex] {
			return 0, false
		}

		valueIndex := definition.Properties[propertyIndex].ValueIndex(value.Value)
		if valueIndex < 0 {
			return 0, false
		}

		indices[propertyIndex] = valueIndex
		changed[propertyIndex] = true
	}

	return definition.StateForProperties(indices...)
}

func BlockByID(id BlockID) (BlockDefinition, bool) {
	if id > MaxBlockID {
		return BlockDefinition{}, false
	}

	return blockDefinitions[id], true
}

func (property BlockProperty) ValueIndex(value string) int {
	for index, candidate := range property.Values {
		if candidate == value {
			return index
		}
	}

	return -1
}

func (definition BlockDefinition) PropertyIndex(name string) (int, bool) {
	for index, property := range definition.Properties {
		if property.Name == name {
			return index, true
		}
	}

	return 0, false
}

func (definition BlockDefinition) State(offset uint16) (Block, bool) {
	state := uint32(definition.MinState) + uint32(offset)
	if state > uint32(definition.MaxState) {
		return 0, false
	}

	return Block(state), true
}

// StateForProperties resolves ordered property value indices using the same
// mixed-radix order as the vanilla registry.
func (definition BlockDefinition) StateForProperties(indices ...int) (Block, bool) {
	if len(indices) != len(definition.Properties) {
		return 0, false
	}

	var offset int

	for index, valueIndex := range indices {
		valueCount := len(definition.Properties[index].Values)
		if valueIndex < 0 || valueIndex >= valueCount {
			return 0, false
		}

		offset = offset*valueCount + valueIndex
	}

	return definition.State(uint16(offset))
}

func (definition BlockDefinition) StateForPropertyValues(values ...BlockPropertyValue) (Block, bool) {
	return definition.DefaultState.WithProperties(values...)
}

func (definition BlockDefinition) propertyIndices(block Block) []int {
	indices := make([]int, len(definition.Properties))

	offset := int(block - definition.MinState)

	for index := len(definition.Properties) - 1; index >= 0; index-- {
		valueCount := len(definition.Properties[index].Values)
		indices[index] = offset % valueCount
		offset /= valueCount
	}

	return indices
}

//go:generate go run ../../cmd/generate-blocks -input ../../data/blocks.json -items ../../data/items.json -tags ../../data/block_tags -loot ../../data/block_loot -output blocks_generated.go -protocol-output ../protocol/block_tags_generated.go
