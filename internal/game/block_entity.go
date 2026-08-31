package game

import "maps"

const (
	BarrelSlotCount  = 27
	ChestSlotCount   = 27
	FurnaceSlotCount = 3
	HopperSlotCount  = 5
)

const (
	BlockEntityTypeNone BlockEntityType = iota
	BlockEntityTypeChest
	BlockEntityTypeTrappedChest
	BlockEntityTypeBarrel
	BlockEntityTypeFurnace
	BlockEntityTypeSmoker
	BlockEntityTypeBlastFurnace
	BlockEntityTypeHopper
)

const (
	BlockEntityRemovalNone BlockEntityRemovalBehavior = iota
	BlockEntityRemovalDropInventory
)

const (
	BlockEntityDataNone BlockEntityDataKind = iota
	BlockEntityDataInventory
	BlockEntityDataFurnace
	BlockEntityDataHopper
)

type BlockEntityType uint8

type BlockEntityRemovalBehavior uint8

type BlockEntityDataKind uint8

type BlockEntityTypeDefinition struct {
	Name                    string
	ProtocolRegistryID12111 int32
	InventorySlots          int
	DataKind                BlockEntityDataKind
	RemovalBehavior         BlockEntityRemovalBehavior
}

type BlockEntityData interface {
	CloneBlockEntityData() BlockEntityData
	EqualBlockEntityData(BlockEntityData) bool
}

type InventoryBlockEntityData struct {
	Items []ItemStack
}

type FurnaceBlockEntityData struct {
	Items            []ItemStack
	LitTimeRemaining int32
	LitTotalTime     int32
	CookingProgress  int32
	CookingTotalTime int32
	RecipesUsed      map[string]int32
}

type HopperBlockEntityData struct {
	Items            []ItemStack
	TransferCooldown int32
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
	BlockEntityTypeNone:  {},
	BlockEntityTypeChest: {Name: "chest", ProtocolRegistryID12111: 1, InventorySlots: ChestSlotCount, DataKind: BlockEntityDataInventory, RemovalBehavior: BlockEntityRemovalDropInventory},
	BlockEntityTypeTrappedChest: {
		Name:                    "trapped_chest",
		ProtocolRegistryID12111: 2,
		InventorySlots:          ChestSlotCount,
		DataKind:                BlockEntityDataInventory,
		RemovalBehavior:         BlockEntityRemovalDropInventory,
	},
	BlockEntityTypeBarrel:       {Name: "barrel", ProtocolRegistryID12111: 27, InventorySlots: BarrelSlotCount, DataKind: BlockEntityDataInventory, RemovalBehavior: BlockEntityRemovalDropInventory},
	BlockEntityTypeFurnace:      {Name: "furnace", ProtocolRegistryID12111: 0, InventorySlots: FurnaceSlotCount, DataKind: BlockEntityDataFurnace, RemovalBehavior: BlockEntityRemovalDropInventory},
	BlockEntityTypeSmoker:       {Name: "smoker", ProtocolRegistryID12111: 28, InventorySlots: FurnaceSlotCount, DataKind: BlockEntityDataFurnace, RemovalBehavior: BlockEntityRemovalDropInventory},
	BlockEntityTypeBlastFurnace: {Name: "blast_furnace", ProtocolRegistryID12111: 29, InventorySlots: FurnaceSlotCount, DataKind: BlockEntityDataFurnace, RemovalBehavior: BlockEntityRemovalDropInventory},
	BlockEntityTypeHopper:       {Name: "hopper", ProtocolRegistryID12111: 18, InventorySlots: HopperSlotCount, DataKind: BlockEntityDataHopper, RemovalBehavior: BlockEntityRemovalDropInventory},
}

func (entityType BlockEntityType) Definition() (BlockEntityTypeDefinition, bool) {
	if entityType == BlockEntityTypeNone || int(entityType) >= len(blockEntityTypeDefinitions) {
		return BlockEntityTypeDefinition{}, false
	}

	return blockEntityTypeDefinitions[entityType], true
}

func (entity *BlockEntity) Inventory() ([]ItemStack, bool) {
	switch data := entity.Data.(type) {
	case *InventoryBlockEntityData:
		return data.Items, true
	case *FurnaceBlockEntityData:
		return data.Items, true
	case *HopperBlockEntityData:
		return data.Items, true
	default:
		return nil, false
	}
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

func (data *FurnaceBlockEntityData) CloneBlockEntityData() BlockEntityData {
	clone := &FurnaceBlockEntityData{
		Items:            make([]ItemStack, len(data.Items)),
		LitTimeRemaining: data.LitTimeRemaining,
		LitTotalTime:     data.LitTotalTime,
		CookingProgress:  data.CookingProgress,
		CookingTotalTime: data.CookingTotalTime,
	}

	for slot := range data.Items {
		clone.Items[slot] = data.Items[slot].Clone()
	}

	if data.RecipesUsed != nil {
		clone.RecipesUsed = make(map[string]int32, len(data.RecipesUsed))
		maps.Copy(clone.RecipesUsed, data.RecipesUsed)
	}

	return clone
}

func (data *FurnaceBlockEntityData) EqualBlockEntityData(other BlockEntityData) bool {
	otherFurnace, valid := other.(*FurnaceBlockEntityData)
	if !valid || len(data.Items) != len(otherFurnace.Items) ||
		data.LitTimeRemaining != otherFurnace.LitTimeRemaining ||
		data.LitTotalTime != otherFurnace.LitTotalTime ||
		data.CookingProgress != otherFurnace.CookingProgress ||
		data.CookingTotalTime != otherFurnace.CookingTotalTime ||
		len(data.RecipesUsed) != len(otherFurnace.RecipesUsed) {
		return false
	}

	for slot := range data.Items {
		if !data.Items[slot].Equal(otherFurnace.Items[slot]) {
			return false
		}
	}

	for name, uses := range data.RecipesUsed {
		if otherFurnace.RecipesUsed[name] != uses {
			return false
		}
	}

	return true
}

func (data *HopperBlockEntityData) CloneBlockEntityData() BlockEntityData {
	clone := &HopperBlockEntityData{
		Items:            make([]ItemStack, len(data.Items)),
		TransferCooldown: data.TransferCooldown,
	}

	for slot := range data.Items {
		clone.Items[slot] = data.Items[slot].Clone()
	}

	return clone
}

func (data *HopperBlockEntityData) EqualBlockEntityData(other BlockEntityData) bool {
	otherHopper, valid := other.(*HopperBlockEntityData)
	if !valid || len(data.Items) != len(otherHopper.Items) || data.TransferCooldown != otherHopper.TransferCooldown {
		return false
	}

	for slot := range data.Items {
		if !data.Items[slot].Equal(otherHopper.Items[slot]) {
			return false
		}
	}

	return true
}
func NewBlockEntity(entityType BlockEntityType) BlockEntity {
	definition, valid := entityType.Definition()
	if !valid {
		return BlockEntity{Type: entityType}
	}

	switch definition.DataKind {
	case BlockEntityDataInventory:
		return NewInventoryBlockEntity(entityType, definition.InventorySlots)
	case BlockEntityDataFurnace:
		return BlockEntity{
			Type: entityType,
			Data: &FurnaceBlockEntityData{Items: make([]ItemStack, definition.InventorySlots)},
		}
	case BlockEntityDataHopper:
		return BlockEntity{
			Type: entityType,
			Data: &HopperBlockEntityData{Items: make([]ItemStack, definition.InventorySlots), TransferCooldown: -1},
		}
	default:
		return BlockEntity{Type: entityType}
	}
}

func NewInventoryBlockEntity(entityType BlockEntityType, slots int) BlockEntity {
	return BlockEntity{
		Type: entityType,
		Data: &InventoryBlockEntityData{Items: make([]ItemStack, slots)},
	}
}

func BlockEntityTypeForBlock(block Block) BlockEntityType {
	definition, valid := block.Definition()
	if !valid {
		return BlockEntityTypeNone
	}

	return definition.BlockEntityType
}
