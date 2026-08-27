package game

const (
	CraftingSlotCount      = 4
	ArmorSlotCount         = 4
	MainInventorySlotCount = 27
	HotbarSlotCount        = 9
	PlayerInventorySlots   = 46
)

const (
	ItemPlacementUnsupported ItemPlacementRule = iota
	ItemPlacementDefault
	ItemPlacementAxis
	ItemPlacementHorizontalFacing
	ItemPlacementSlab
	ItemPlacementStairs
	ItemPlacementDoor
	ItemPlacementTrapdoor
	ItemPlacementFenceGate
	ItemPlacementFence
	ItemPlacementPane
	ItemPlacementWall
	ItemPlacementLeaves
	ItemPlacementChain
	ItemPlacementSupported
	ItemPlacementButton
	ItemPlacementPressurePlate
	ItemPlacementWeightedPressurePlate
	ItemPlacementPlant
	ItemPlacementSnow
	ItemPlacementCandle
	ItemPlacementPointedDripstone
)

type Item uint16

type ItemPlacementRule uint8

type ItemDefinition struct {
	ID        Item
	Name      string
	StackSize int32
}

type ItemStack struct {
	Item              Item
	Count             int32
	Components        []ItemComponent
	RemovedComponents []int32
}

type ItemComponent struct {
	Type int32
	Data []byte
}

type PlayerInventory struct {
	CraftingResult ItemStack
	Crafting       [CraftingSlotCount]ItemStack
	Armor          [ArmorSlotCount]ItemStack
	Main           [MainInventorySlotCount]ItemStack
	Hotbar         [HotbarSlotCount]ItemStack
	Offhand        ItemStack
}

func (item Item) Valid() bool {
	return item <= MaxItemID
}

func (item Item) Definition() (ItemDefinition, bool) {
	if !item.Valid() {
		return ItemDefinition{}, false
	}

	return itemDefinitions[item], true
}

func (item Item) PlacementBlock() (Block, bool) {
	if !item.Valid() {
		return 0, false
	}

	block := itemPlacementBlocks[item]

	return block, block != Air
}

func (item Item) PlacementRule() ItemPlacementRule {
	if !item.Valid() {
		return ItemPlacementUnsupported
	}

	return itemPlacementRules[item]
}

func ItemForBlock(block Block) (Item, bool) {
	if block == Air {
		return 0, false
	}

	blockDefinition, valid := block.Definition()
	if !valid {
		return 0, false
	}

	for item, itemDefinition := range itemDefinitions {
		if itemDefinition.Name == blockDefinition.Name && Item(item) != ItemAir {
			return Item(item), true
		}
	}

	return 0, false
}

func (stack ItemStack) Empty() bool {
	return stack.Count <= 0 || !stack.Item.Valid() || stack.Item == ItemAir
}

func (stack ItemStack) Clone() ItemStack {
	clone := stack

	clone.Components = make([]ItemComponent, len(stack.Components))

	for index, component := range stack.Components {
		clone.Components[index] = ItemComponent{Type: component.Type, Data: append([]byte(nil), component.Data...)}
	}

	clone.RemovedComponents = append([]int32(nil), stack.RemovedComponents...)

	return clone
}

func (stack ItemStack) Equal(other ItemStack) bool {
	if stack.Item != other.Item || stack.Count != other.Count || len(stack.Components) != len(other.Components) || len(stack.RemovedComponents) != len(other.RemovedComponents) {
		return false
	}

	for index := range stack.Components {
		first := stack.Components[index]
		second := other.Components[index]

		if first.Type != second.Type || string(first.Data) != string(second.Data) {
			return false
		}
	}

	for index := range stack.RemovedComponents {
		if stack.RemovedComponents[index] != other.RemovedComponents[index] {
			return false
		}
	}

	return true
}

func (stack ItemStack) SameItem(other ItemStack) bool {
	first := stack
	second := other

	first.Count = 1
	second.Count = 1

	return first.Equal(second)
}

func (inventory PlayerInventory) Clone() PlayerInventory {
	clone := inventory

	for slot := range PlayerInventorySlots {
		*clone.Slot(slot) = inventory.Slot(slot).Clone()
	}

	return clone
}

func (inventory *PlayerInventory) Slot(slot int) *ItemStack {
	switch {
	case slot == 0:
		return &inventory.CraftingResult
	case slot >= 1 && slot <= 4:
		return &inventory.Crafting[slot-1]
	case slot >= 5 && slot <= 8:
		return &inventory.Armor[slot-5]
	case slot >= 9 && slot <= 35:
		return &inventory.Main[slot-9]
	case slot >= 36 && slot <= 44:
		return &inventory.Hotbar[slot-36]
	case slot == 45:
		return &inventory.Offhand
	default:
		return nil
	}
}

func (inventory *PlayerInventory) Held(selected int) *ItemStack {
	if selected < 0 || selected >= HotbarSlotCount {
		return nil
	}

	return &inventory.Hotbar[selected]
}

func (inventory PlayerInventory) Contents() []ItemStack {
	contents := make([]ItemStack, PlayerInventorySlots)

	for slot := range contents {
		contents[slot] = inventory.Slot(slot).Clone()
	}

	return contents
}

//go:generate go run ../../cmd/generate-items -items ../../data/items.json -blocks ../../data/blocks.json -output items_generated.go
