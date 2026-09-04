package game

import (
	"bytes"
	"slices"
	"sort"
)

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
	ItemPlacementDirectionalFacing
	ItemPlacementChest
	ItemPlacementFurnace
	ItemPlacementHopper
)

const (
	ItemComponentDamage              int32 = 3
	ItemComponentEnchantments        int32 = 13
	ItemComponentPotionContents      int32 = 49
	ItemComponentPotionDurationScale int32 = 50
	MaxItemComponentType             int32 = 103
)

const (
	ItemUseAnimationNone ItemUseAnimation = iota
	ItemUseAnimationEat
	ItemUseAnimationDrink
)

const (
	ItemOnBlockBehaviorNone ItemOnBlockBehavior = iota
	ItemOnBlockBehaviorHoe
)

const (
	ItemEquipmentSlotNone ItemEquipmentSlot = iota
	ItemEquipmentSlotHead
	ItemEquipmentSlotChest
	ItemEquipmentSlotLegs
	ItemEquipmentSlotFeet
)

type Item uint16

type ItemPlacementRule uint8

type ItemUseAnimation uint8

type ItemOnBlockBehavior uint8

type ItemEquipmentSlot uint8

type ItemMiningRule struct {
	Trait          BlockTrait
	BlockID        BlockID
	Speed          float32
	HasSpeed       bool
	Correct        bool
	HasCorrectness bool
}

type ItemMining struct {
	Rules          []ItemMiningRule
	DefaultSpeed   float32
	DamagePerBlock int32
}

type ItemFood struct {
	Nutrition    int8
	Saturation   float32
	AlwaysEdible bool
}

type ItemUseEffects struct {
	CanSprint          bool
	InteractVibrations bool
	SpeedMultiplier    float32
}

type ItemConsumeEffectType uint8

const (
	ItemConsumeEffectApplyStatusEffects ItemConsumeEffectType = iota + 1
	ItemConsumeEffectRemoveStatusEffects
	ItemConsumeEffectClearAllStatusEffects
	ItemConsumeEffectTeleportRandomly
	ItemConsumeEffectSuspiciousStew
	ItemConsumeEffectPotionContents
)

type ItemConsumeEffect struct {
	Type        ItemConsumeEffectType
	Effects     []MobEffectInstance
	Probability float32
	Remove      []MobEffect
	Diameter    float32
}

type ItemConsumable struct {
	UseEffects     ItemUseEffects
	Particles      bool
	Sound          SoundEvent
	Duration       uint16
	Animation      ItemUseAnimation
	Remainder      Item
	Effects        []ItemConsumeEffect
	DynamicEffects []ItemConsumeEffect
}

type ItemArmor struct {
	Slot                ItemEquipmentSlot
	Defense             int32
	Toughness           float32
	KnockbackResistance float32
}

type ItemDefinition struct {
	ID                   Item
	Name                 string
	StackSize            int32
	MaxDurability        int32
	AttackDamageModifier float32
	AttackSpeedModifier  float32
	DamagePerAttack      int32
	Armor                ItemArmor
	Mining               ItemMining
	Food                 ItemFood
	Consumable           ItemConsumable
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

func (item Item) FireResistant() bool {
	switch item {
	case ItemAncientDebris,
		ItemNetheriteBlock,
		ItemNetheriteIngot,
		ItemNetheriteScrap,
		ItemNetheriteSword,
		ItemNetheriteShovel,
		ItemNetheritePickaxe,
		ItemNetheriteAxe,
		ItemNetheriteHoe,
		ItemNetheriteHelmet,
		ItemNetheriteChestplate,
		ItemNetheriteLeggings,
		ItemNetheriteBoots,
		ItemNetheriteHorseArmor,
		ItemNetheriteSpear,
		ItemNetheriteNautilusArmor:
		return true
	default:
		return false
	}
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

func (item Item) OnBlockBehavior() ItemOnBlockBehavior {
	if !item.Valid() {
		return ItemOnBlockBehaviorNone
	}

	return itemOnBlockBehaviors[item]
}

func (item Item) MiningProperties() ItemMining {
	definition, valid := item.Definition()
	if !valid {
		return ItemMining{}
	}

	return definition.Mining
}

func (item Item) BaseDestroySpeed(block Block) float32 {
	itemMining := item.MiningProperties()

	for _, rule := range itemMining.Rules {
		if !rule.HasSpeed || !rule.matches(block) {
			continue
		}

		return rule.Speed
	}

	if itemMining.DefaultSpeed > 0 {
		return itemMining.DefaultSpeed
	}

	return 1
}

func (item Item) IsCorrectToolForDrops(block Block) bool {
	blockMining := block.MiningProperties()
	if !blockMining.RequiresTool {
		return true
	}

	itemMining := item.MiningProperties()

	for _, rule := range itemMining.Rules {
		if !rule.HasCorrectness || !rule.matches(block) {
			continue
		}

		return rule.Correct
	}

	return false
}

func (rule ItemMiningRule) matches(block Block) bool {
	if rule.BlockID != AirID && block.Valid() && stateBlockIDs[block] == rule.BlockID {
		return true
	}

	return rule.Trait != 0 && block.HasTrait(rule.Trait)
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
	if stack.Item != other.Item || stack.Count != other.Count {
		return false
	}

	firstComponents, firstRemoved := normalizedComponentPatch(stack.Components, stack.RemovedComponents)
	secondComponents, secondRemoved := normalizedComponentPatch(other.Components, other.RemovedComponents)

	if len(firstComponents) != len(secondComponents) || len(firstRemoved) != len(secondRemoved) {
		return false
	}

	for index := range firstComponents {
		first := firstComponents[index]
		second := secondComponents[index]

		if first.Type != second.Type || !bytes.Equal(first.Data, second.Data) {
			return false
		}
	}

	for index := range firstRemoved {
		if firstRemoved[index] != secondRemoved[index] {
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

func (stack ItemStack) Damage() int32 {
	data, exists := stack.component(ItemComponentDamage)
	if !exists {
		return 0
	}

	value, read, valid := readComponentVarInt(data, 0)
	if !valid || read != len(data) || value < 0 {
		return 0
	}

	return value
}

func (stack *ItemStack) SetDamage(damage int32) {
	if damage <= 0 {
		stack.replaceComponent(ItemComponentDamage, nil)

		return
	}

	stack.replaceComponent(ItemComponentDamage, appendComponentVarInt(nil, damage))
}

func (stack ItemStack) EnchantmentLevel(enchantment Enchantment) int32 {
	return stack.Enchantments()[enchantment]
}

func (stack ItemStack) PreventsEquipmentDrop() bool {
	return stack.EnchantmentLevel(EnchantmentVanishingCurse) > 0
}

func (stack ItemStack) ArmorAttributes(slot ItemEquipmentSlot) (ItemArmor, bool) {
	if stack.Empty() {
		return ItemArmor{}, false
	}

	definition, valid := stack.Item.Definition()
	if !valid || definition.Armor.Slot != slot {
		return ItemArmor{}, false
	}

	return definition.Armor, true
}

func (stack ItemStack) Enchantments() map[Enchantment]int32 {
	data, exists := stack.component(ItemComponentEnchantments)
	if !exists {
		return nil
	}

	count, offset, valid := readComponentVarInt(data, 0)
	if !valid || count < 0 {
		return nil
	}

	enchantments := make(map[Enchantment]int32, count)

	for range count {
		holder, next, holderValid := readComponentVarInt(data, offset)
		if !holderValid {
			return nil
		}

		level, end, levelValid := readComponentVarInt(data, next)
		if !levelValid || level < 0 {
			return nil
		}

		enchantment := Enchantment(holder)
		if enchantment.Valid() {
			enchantments[enchantment] = level
		}

		offset = end
	}

	if offset != len(data) {
		return nil
	}

	return enchantments
}

func (stack *ItemStack) SetEnchantment(enchantment Enchantment, level int32) {
	enchantments := stack.Enchantments()
	if enchantments == nil {
		enchantments = make(map[Enchantment]int32)
	}

	if level <= 0 {
		delete(enchantments, enchantment)
	} else {
		enchantments[enchantment] = min(level, 255)
	}

	stack.SetEnchantments(enchantments)
}

func (stack *ItemStack) SetEnchantments(enchantments map[Enchantment]int32) {
	identities := make([]int, 0, len(enchantments))

	for enchantment, level := range enchantments {
		if enchantment.Valid() && level > 0 {
			identities = append(identities, int(enchantment))
		}
	}

	if len(identities) == 0 {
		stack.replaceComponent(ItemComponentEnchantments, nil)

		return
	}

	sort.Ints(identities)

	data := appendComponentVarInt(nil, int32(len(identities)))

	for _, identity := range identities {
		level := min(enchantments[Enchantment(identity)], 255)

		data = appendComponentVarInt(data, int32(identity))
		data = appendComponentVarInt(data, level)
	}

	stack.replaceComponent(ItemComponentEnchantments, data)
}

func (stack ItemStack) component(componentType int32) ([]byte, bool) {
	if slices.Contains(stack.RemovedComponents, componentType) {
		return nil, false
	}

	for _, component := range slices.Backward(stack.Components) {
		if component.Type == componentType {
			return component.Data, true
		}
	}

	return nil, false
}

func (stack *ItemStack) replaceComponent(componentType int32, data []byte) {
	components := stack.Components[:0]
	inserted := false

	for _, component := range stack.Components {
		if component.Type != componentType {
			components = append(components, component)

			continue
		}

		if data != nil && !inserted {
			components = append(components, ItemComponent{Type: componentType, Data: append([]byte(nil), data...)})
			inserted = true
		}
	}

	if data != nil && !inserted {
		components = append(components, ItemComponent{Type: componentType, Data: append([]byte(nil), data...)})
	}

	stack.Components = components

	removed := stack.RemovedComponents[:0]

	for _, removedType := range stack.RemovedComponents {
		if removedType != componentType {
			removed = append(removed, removedType)
		}
	}

	stack.RemovedComponents = removed
	stack.NormalizeComponents()
}

func (stack *ItemStack) NormalizeComponents() {
	stack.Components, stack.RemovedComponents = normalizedComponentPatch(stack.Components, stack.RemovedComponents)
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

//go:generate go run ../../cmd/generate-items -items ../../data/items.json -blocks ../../data/blocks.json -consumables ../../data/item_consumables.json -attack-attributes ../../data/item_attack_attributes.json -armor-attributes ../../data/item_armor_attributes.json -output items_generated.go
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

func normalizedComponentPatch(components []ItemComponent, removedComponents []int32) ([]ItemComponent, []int32) {
	removedTypes := make(map[int32]struct{}, len(removedComponents))

	for _, componentType := range removedComponents {
		removedTypes[componentType] = struct{}{}
	}

	componentData := make(map[int32][]byte, len(components))

	for _, component := range components {
		if _, removed := removedTypes[component.Type]; removed {
			continue
		}

		componentData[component.Type] = component.Data
	}

	componentTypes := make([]int, 0, len(componentData))

	for componentType := range componentData {
		componentTypes = append(componentTypes, int(componentType))
	}

	sort.Ints(componentTypes)

	normalizedComponents := make([]ItemComponent, 0, len(componentTypes))

	for _, componentType := range componentTypes {
		typed := int32(componentType)
		normalizedComponents = append(normalizedComponents, ItemComponent{Type: typed, Data: componentData[typed]})
	}

	removedTypeList := make([]int, 0, len(removedTypes))

	for componentType := range removedTypes {
		removedTypeList = append(removedTypeList, int(componentType))
	}

	sort.Ints(removedTypeList)

	normalizedRemoved := make([]int32, len(removedTypeList))

	for index, componentType := range removedTypeList {
		normalizedRemoved[index] = int32(componentType)
	}

	return normalizedComponents, normalizedRemoved
}

func appendComponentVarInt(data []byte, value int32) []byte {
	unsigned := uint32(value)

	for {
		current := byte(unsigned & 0x7f)
		unsigned >>= 7

		if unsigned != 0 {
			current |= 0x80
		}

		data = append(data, current)

		if unsigned == 0 {
			return data
		}
	}
}

func readComponentVarInt(data []byte, offset int) (int32, int, bool) {
	var value uint32

	for index := 0; index < 5 && offset+index < len(data); index++ {
		current := data[offset+index]
		value |= uint32(current&0x7f) << (7 * index)

		if current&0x80 == 0 {
			return int32(value), offset + index + 1, true
		}
	}

	return 0, offset, false
}
