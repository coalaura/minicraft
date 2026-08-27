package game

const BarrelSlotCount = 27

type BlockEntityType uint8

const (
	BlockEntityTypeNone BlockEntityType = iota
	BlockEntityTypeBarrel
)

type BlockEntity struct {
	Type  BlockEntityType
	Items [BarrelSlotCount]ItemStack
}

type ChunkBlockEntities map[LocalBlockPosition]BlockEntity

// BlockEntityGenerator is an optional chunk-level fast path for procedural
// block entities. Positions are local X/Z coordinates and absolute Y values.
type BlockEntityGenerator interface {
	GenerateBlockEntities(seed int64, chunk ChunkPosition) ChunkBlockEntities
}

func (entity BlockEntity) Clone() BlockEntity {
	clone := entity

	for slot := range entity.Items {
		clone.Items[slot] = entity.Items[slot].Clone()
	}

	return clone
}

func (entity BlockEntity) Equal(other BlockEntity) bool {
	if entity.Type != other.Type {
		return false
	}

	for slot := range entity.Items {
		if !entity.Items[slot].Equal(other.Items[slot]) {
			return false
		}
	}

	return true
}

func BlockEntityTypeForBlock(block Block) BlockEntityType {
	definition, valid := block.Definition()
	if !valid {
		return BlockEntityTypeNone
	}

	switch definition.Name {
	case "barrel":
		return BlockEntityTypeBarrel
	default:
		return BlockEntityTypeNone
	}
}
