package game

const HotbarSlotCount = 9

const (
	ItemPlacementUnsupported ItemPlacementRule = iota
	ItemPlacementDefault
	ItemPlacementAxis
)

type Item uint16

type ItemPlacementRule uint8

type ItemDefinition struct {
	ID        Item
	Name      string
	StackSize int32
}

type ItemStack struct {
	Item  Item
	Count int32
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

func (stack ItemStack) Empty() bool {
	return stack.Count <= 0 || !stack.Item.Valid() || stack.Item == ItemAir
}

//go:generate go run ../../cmd/generate-items -items ../../ref/1.21.11/items.json -blocks ../../ref/1.21.11/blocks.json -output items_generated.go
