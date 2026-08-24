package game

const (
	ChunkWidth    = 16
	SectionVolume = ChunkWidth * ChunkWidth * ChunkWidth
)

type Block uint16

type BlockID uint16

type BlockProperty struct {
	Name   string
	Values []string
}

type BlockDefinition struct {
	ID           BlockID
	Name         string
	DefaultState Block
	MinState     Block
	MaxState     Block
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

func BlockByID(id BlockID) (BlockDefinition, bool) {
	if id > MaxBlockID {
		return BlockDefinition{}, false
	}

	return blockDefinitions[id], true
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

//go:generate go run ../../cmd/generate-blocks -input ../../ref/1.21.11/blocks.json -output blocks_generated.go
