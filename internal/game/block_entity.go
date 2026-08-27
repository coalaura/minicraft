package game

const BarrelSlotCount = 27

type BlockEntityType uint8

const (
	BlockEntityTypeNone BlockEntityType = iota
	BlockEntityTypeBarrel
)

type BlockEntityTypeDefinition struct {
	Name                    string
	ProtocolRegistryID12111 int32
	InventorySlots          int
}

type BlockEntityData interface {
	CloneBlockEntityData() BlockEntityData
	EqualBlockEntityData(BlockEntityData) bool
}

type InventoryBlockEntityData struct {
	Items []ItemStack
}

type BlockEntity struct {
	Type BlockEntityType
	Data BlockEntityData
}

type ChunkBlockEntities map[LocalBlockPosition]BlockEntity

// BlockEntityGenerator is an optional chunk-level fast path for procedural
// block entities. Positions are local X/Z coordinates and absolute Y values.
type BlockEntityGenerator interface {
	GenerateBlockEntities(seed int64, chunk ChunkPosition) ChunkBlockEntities
}

// BlockEntityPointGenerator avoids generating a complete chunk entity map for
// authoritative point lookups.
type BlockEntityPointGenerator interface {
	GenerateBlockEntity(seed int64, position BlockPosition) (BlockEntity, bool)
}

var blockEntityTypeDefinitions = [...]BlockEntityTypeDefinition{
	BlockEntityTypeNone:   {},
	BlockEntityTypeBarrel: {Name: "barrel", ProtocolRegistryID12111: 27, InventorySlots: BarrelSlotCount},
}

func NewBlockEntity(entityType BlockEntityType) BlockEntity {
	definition, valid := entityType.Definition()
	if !valid || definition.InventorySlots == 0 {
		return BlockEntity{Type: entityType}
	}

	return NewInventoryBlockEntity(entityType, definition.InventorySlots)
}

func NewInventoryBlockEntity(entityType BlockEntityType, slots int) BlockEntity {
	return BlockEntity{
		Type: entityType,
		Data: &InventoryBlockEntityData{Items: make([]ItemStack, slots)},
	}
}

func (entityType BlockEntityType) Definition() (BlockEntityTypeDefinition, bool) {
	if entityType == BlockEntityTypeNone || int(entityType) >= len(blockEntityTypeDefinitions) {
		return BlockEntityTypeDefinition{}, false
	}

	return blockEntityTypeDefinitions[entityType], true
}

func (entity *BlockEntity) Inventory() ([]ItemStack, bool) {
	data, valid := entity.Data.(*InventoryBlockEntityData)
	if !valid {
		return nil, false
	}

	return data.Items, true
}

func (entity BlockEntity) Clone() BlockEntity {
	clone := entity
	if entity.Data != nil {
		clone.Data = entity.Data.CloneBlockEntityData()
	}

	return clone
}

func (entity BlockEntity) Equal(other BlockEntity) bool {
	if entity.Type != other.Type {
		return false
	}

	if entity.Data == nil || other.Data == nil {
		return entity.Data == nil && other.Data == nil
	}

	return entity.Data.EqualBlockEntityData(other.Data)
}

func BlockEntityTypeForBlock(block Block) BlockEntityType {
	definition, valid := block.Definition()
	if !valid {
		return BlockEntityTypeNone
	}

	return definition.BlockEntityType
}

func (data *InventoryBlockEntityData) CloneBlockEntityData() BlockEntityData {
	clone := &InventoryBlockEntityData{Items: make([]ItemStack, len(data.Items))}

	for slot := range data.Items {
		clone.Items[slot] = data.Items[slot].Clone()
	}

	return clone
}

func (data *InventoryBlockEntityData) EqualBlockEntityData(other BlockEntityData) bool {
	otherInventory, valid := other.(*InventoryBlockEntityData)
	if !valid || len(data.Items) != len(otherInventory.Items) {
		return false
	}

	for slot := range data.Items {
		if !data.Items[slot].Equal(otherInventory.Items[slot]) {
			return false
		}
	}

	return true
}
