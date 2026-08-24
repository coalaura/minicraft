package game

const (
	ChunkWidth    = 16
	SectionVolume = ChunkWidth * ChunkWidth * ChunkWidth
)

type Block uint16

type BlockID uint16

type BlockBehavior uint8

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
	ID           BlockID
	Name         string
	DefaultState Block
	MinState     Block
	MaxState     Block
	Behavior     BlockBehavior
	Properties   []BlockProperty
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

func (block Block) Behavior() BlockBehavior {
	if !block.Valid() {
		return BlockBehaviorNone
	}

	return blockDefinitions[stateBlockIDs[block]].Behavior
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

//go:generate go run ../../cmd/generate-blocks -input ../../ref/1.21.11/blocks.json -output blocks_generated.go
