package game

import (
	"bytes"
	"slices"
	"testing"
)

type itemPlacementBlockMappingTestCase struct {
	item  Item
	block Block
}

type copperChestTestCase struct {
	name      string
	block     Block
	item      Item
	oxidation int
	waxed     bool
}

type itemMetadataTestCase struct {
	name                 string
	item                 Item
	maxDurability        int32
	damagePerBlock       int32
	attackDamageModifier float32
	attackSpeedModifier  float32
	damagePerAttack      int32
}

func TestGeneratedItemCatalogueCoversVanillaItems(t *testing.T) {
	if MaxItemID != 1504 {
		t.Fatalf("max item ID = %d, want 1504", MaxItemID)
	}

	for item := Item(0); item <= MaxItemID; item++ {
		definition, ok := item.Definition()
		if !ok {
			t.Fatalf("item %d has no definition", item)
		}

		if definition.ID != item || definition.Name == "" || definition.StackSize < 1 {
			t.Fatalf("item %d definition = %+v", item, definition)
		}
	}

	if (MaxItemID + 1).Valid() {
		t.Fatal("item above catalogue is valid")
	}
}

func TestGeneratedItemMetadata(t *testing.T) {
	tests := []itemMetadataTestCase{
		{
			name:                 "pickaxe",
			item:                 ItemDiamondPickaxe,
			maxDurability:        1561,
			damagePerBlock:       1,
			attackDamageModifier: 4,
			attackSpeedModifier:  -2.8,
			damagePerAttack:      2,
		},
		{
			name:                 "sword",
			item:                 ItemDiamondSword,
			maxDurability:        1561,
			damagePerBlock:       2,
			attackDamageModifier: 6,
			attackSpeedModifier:  -2.4,
			damagePerAttack:      1,
		},
		{
			name:                 "copper axe",
			item:                 ItemCopperAxe,
			maxDurability:        190,
			damagePerBlock:       1,
			attackDamageModifier: 8,
			attackSpeedModifier:  -3.2,
			damagePerAttack:      2,
		},
		{
			name:           "shears",
			item:           ItemShears,
			maxDurability:  238,
			damagePerBlock: 1,
		},
		{
			name:          "chest armor",
			item:          ItemLeatherChestplate,
			maxDurability: 80,
		},
		{
			name: "non-tool",
			item: ItemStone,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition, valid := test.item.Definition()
			if !valid {
				t.Fatal("item definition is missing")
			}

			if definition.MaxDurability != test.maxDurability {
				t.Fatalf("max durability = %d, want %d", definition.MaxDurability, test.maxDurability)
			}

			if definition.Mining.DamagePerBlock != test.damagePerBlock {
				t.Fatalf("damage per block = %d, want %d", definition.Mining.DamagePerBlock, test.damagePerBlock)
			}

			if definition.AttackDamageModifier != test.attackDamageModifier || definition.AttackSpeedModifier != test.attackSpeedModifier {
				t.Fatalf("attack modifiers = damage %v speed %v, want damage %v speed %v", definition.AttackDamageModifier, definition.AttackSpeedModifier, test.attackDamageModifier, test.attackSpeedModifier)
			}

			if definition.DamagePerAttack != test.damagePerAttack {
				t.Fatalf("damage per attack = %d, want %d", definition.DamagePerAttack, test.damagePerAttack)
			}
		})
	}
}

func TestItemStackTypedComponents(t *testing.T) {
	stack := ItemStack{
		Components: []ItemComponent{
			{Type: ItemComponentDamage, Data: []byte{0xAC, 0x02}},
			{Type: ItemComponentEnchantments, Data: []byte{0x02, 0x17, 0x03, 0x14, 0x05}},
		},
	}

	if stack.Damage() != 300 {
		t.Fatalf("damage = %d, want 300", stack.Damage())
	}

	if stack.EnchantmentLevel(EnchantmentEfficiency) != 5 || stack.EnchantmentLevel(EnchantmentFortune) != 3 {
		t.Fatalf("enchantments = %v, want efficiency 5 and fortune 3", stack.Enchantments())
	}

	stack.SetEnchantment(EnchantmentEfficiency, 7)

	expectedEnchantments := []byte{0x02, 0x14, 0x07, 0x17, 0x03}
	data, exists := stack.component(ItemComponentEnchantments)

	if !exists || !bytes.Equal(data, expectedEnchantments) {
		t.Fatalf("upserted enchantments = %x, want %x", data, expectedEnchantments)
	}

	stack.SetEnchantment(EnchantmentEfficiency, 0)
	stack.SetEnchantment(EnchantmentFortune, 0)
	stack.SetDamage(0)

	_, hasDamage := stack.component(ItemComponentDamage)
	_, hasEnchantments := stack.component(ItemComponentEnchantments)

	if hasDamage || hasEnchantments {
		t.Fatalf("default components remain: %+v", stack.Components)
	}
}

func TestItemStackTypedComponentsRejectInvalidPayloads(t *testing.T) {
	invalidDamagePayloads := [][]byte{{0x80}, {0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, {0x01, 0x00}}

	for _, data := range invalidDamagePayloads {
		stack := ItemStack{Components: []ItemComponent{{Type: ItemComponentDamage, Data: data}}}
		if stack.Damage() != 0 {
			t.Fatalf("damage from invalid payload %x = %d, want 0", data, stack.Damage())
		}
	}

	invalidEnchantmentPayloads := [][]byte{{0x80}, {0x01, 0x14}, {0x01, 0x14, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, {0x00, 0x00}}

	for _, data := range invalidEnchantmentPayloads {
		stack := ItemStack{Components: []ItemComponent{{Type: ItemComponentEnchantments, Data: data}}}
		if stack.Enchantments() != nil {
			t.Fatalf("enchantments from invalid payload %x = %v, want nil", data, stack.Enchantments())
		}
	}
}

func TestItemStackComponentReplacementPreservesUnknownComponents(t *testing.T) {
	stack := ItemStack{
		Components: []ItemComponent{
			{Type: 99, Data: []byte{0xAA}},
			{Type: ItemComponentDamage, Data: []byte{0x01}},
			{Type: 100, Data: []byte{0xBB}},
			{Type: ItemComponentDamage, Data: []byte{0x02}},
		},
		RemovedComponents: []int32{99, ItemComponentDamage},
	}

	stack.SetDamage(5)

	expectedComponents := []ItemComponent{
		{Type: ItemComponentDamage, Data: []byte{0x05}},
		{Type: 100, Data: []byte{0xBB}},
	}

	if len(stack.Components) != len(expectedComponents) {
		t.Fatalf("component count = %d, want %d", len(stack.Components), len(expectedComponents))
	}

	for index, expected := range expectedComponents {
		actual := stack.Components[index]
		if actual.Type != expected.Type || !bytes.Equal(actual.Data, expected.Data) {
			t.Fatalf("component %d = %+v, want %+v", index, actual, expected)
		}
	}

	if !slices.Equal(stack.RemovedComponents, []int32{99}) {
		t.Fatalf("removed components = %v, want [99]", stack.RemovedComponents)
	}
}

func TestItemStackComponentPatchEqualityUsesMapSemantics(t *testing.T) {
	first := ItemStack{
		Item:  ItemDiamondPickaxe,
		Count: 1,
		Components: []ItemComponent{
			{Type: 99, Data: []byte{0x01}},
			{Type: ItemComponentDamage, Data: []byte{0x02}},
			{Type: 99, Data: []byte{0x03}},
			{Type: ItemComponentEnchantments, Data: []byte{0x01, 0x14, 0x05}},
		},
		RemovedComponents: []int32{100, ItemComponentDamage, 100},
	}

	second := ItemStack{
		Item:  ItemDiamondPickaxe,
		Count: 1,
		Components: []ItemComponent{
			{Type: ItemComponentEnchantments, Data: []byte{0x01, 0x14, 0x05}},
			{Type: 99, Data: []byte{0x03}},
		},
		RemovedComponents: []int32{ItemComponentDamage, 100},
	}

	if !first.Equal(second) || !first.SameItem(second) {
		t.Fatal("semantically identical component patches compare different")
	}

	second.Components[1].Data = []byte{0x04}

	if first.Equal(second) {
		t.Fatal("component patches with different opaque payloads compare equal")
	}
}

func TestItemStackNormalizeComponentsUsesLastAdditionAndRemovalWins(t *testing.T) {
	stack := ItemStack{
		Components: []ItemComponent{
			{Type: ItemComponentDamage, Data: []byte{0x01}},
			{Type: 99, Data: []byte{0xAA, 0xBB}},
			{Type: ItemComponentDamage, Data: []byte{0x02}},
		},
		RemovedComponents: []int32{99, 99},
	}

	stack.NormalizeComponents()

	expected := []ItemComponent{{Type: ItemComponentDamage, Data: []byte{0x02}}}
	if len(stack.Components) != 1 || stack.Components[0].Type != expected[0].Type || !bytes.Equal(stack.Components[0].Data, expected[0].Data) {
		t.Fatalf("normalized components = %v, want %v", stack.Components, expected)
	}

	if !slices.Equal(stack.RemovedComponents, []int32{99}) {
		t.Fatalf("normalized removals = %v, want [99]", stack.RemovedComponents)
	}
}

func TestItemStackEnchantmentsUseDeterministicHolderOrder(t *testing.T) {
	stack := ItemStack{}

	stack.SetEnchantments(map[Enchantment]int32{
		EnchantmentFortune:    3,
		EnchantmentEfficiency: 5,
	})

	data, exists := stack.component(ItemComponentEnchantments)
	expected := []byte{0x02, 0x14, 0x05, 0x17, 0x03}

	if !exists || !bytes.Equal(data, expected) {
		t.Fatalf("enchantment holders = %x, want %x", data, expected)
	}
}

func TestItemPlacementBlockMapping(t *testing.T) {
	placementBlockCases := map[string]itemPlacementBlockMappingTestCase{
		"stone": {item: ItemStone, block: Stone},
		"dirt":  {item: ItemDirt, block: Dirt},
		"glass": {item: ItemGlass, block: Glass},
		"log":   {item: ItemOakLog, block: OakLog},
	}

	for name, test := range placementBlockCases {
		t.Run(name, func(t *testing.T) {
			block, ok := test.item.PlacementBlock()
			if !ok || block != test.block {
				t.Fatalf("placement block = %d, %v; want %d, true", block, ok, test.block)
			}
		})
	}

	rule := ItemOakLog.PlacementRule()
	if rule != ItemPlacementAxis {
		t.Fatalf("oak log placement rule = %d, want axis", rule)
	}

	for _, test := range copperChestBlocksForTest() {
		block, ok := test.item.PlacementBlock()
		if !ok || block != test.block {
			t.Errorf("%s placement block = %d, %v; want %d, true", test.name, block, ok, test.block)
		}
	}

	placementRuleCases := map[Item]ItemPlacementRule{
		ItemOakSlab:                   ItemPlacementSlab,
		ItemOakStairs:                 ItemPlacementStairs,
		ItemOakDoor:                   ItemPlacementDoor,
		ItemOakTrapdoor:               ItemPlacementTrapdoor,
		ItemOakFenceGate:              ItemPlacementFenceGate,
		ItemOakFence:                  ItemPlacementFence,
		ItemGlassPane:                 ItemPlacementPane,
		ItemIronBars:                  ItemPlacementPane,
		ItemCobblestoneWall:           ItemPlacementWall,
		ItemIronDoor:                  ItemPlacementDoor,
		ItemIronTrapdoor:              ItemPlacementTrapdoor,
		ItemOakLeaves:                 ItemPlacementLeaves,
		ItemIronChain:                 ItemPlacementChain,
		ItemStoneButton:               ItemPlacementButton,
		ItemSnow:                      ItemPlacementSnow,
		ItemCandle:                    ItemPlacementCandle,
		ItemPointedDripstone:          ItemPlacementPointedDripstone,
		ItemFern:                      ItemPlacementPlant,
		ItemPoppy:                     ItemPlacementPlant,
		ItemCopperBars:                ItemPlacementPane,
		ItemCobweb:                    ItemPlacementDefault,
		ItemChest:                     ItemPlacementChest,
		ItemCopperChest:               ItemPlacementChest,
		ItemExposedCopperChest:        ItemPlacementChest,
		ItemWeatheredCopperChest:      ItemPlacementChest,
		ItemOxidizedCopperChest:       ItemPlacementChest,
		ItemWaxedCopperChest:          ItemPlacementChest,
		ItemWaxedExposedCopperChest:   ItemPlacementChest,
		ItemWaxedWeatheredCopperChest: ItemPlacementChest,
		ItemWaxedOxidizedCopperChest:  ItemPlacementChest,
	}

	for item, rule := range placementRuleCases {
		actual := item.PlacementRule()
		if actual != rule {
			t.Errorf("item %d placement rule = %d, want %d", item, actual, rule)
		}
	}
}

func copperChestBlocksForTest() []copperChestTestCase {
	return []copperChestTestCase{
		{name: "copper", block: CopperChest, item: ItemCopperChest},
		{name: "exposed", block: ExposedCopperChest, item: ItemExposedCopperChest, oxidation: 1},
		{name: "weathered", block: WeatheredCopperChest, item: ItemWeatheredCopperChest, oxidation: 2},
		{name: "oxidized", block: OxidizedCopperChest, item: ItemOxidizedCopperChest, oxidation: 3},
		{name: "waxed_copper", block: WaxedCopperChest, item: ItemWaxedCopperChest, waxed: true},
		{name: "waxed_exposed", block: WaxedExposedCopperChest, item: ItemWaxedExposedCopperChest, oxidation: 1, waxed: true},
		{name: "waxed_weathered", block: WaxedWeatheredCopperChest, item: ItemWaxedWeatheredCopperChest, oxidation: 2, waxed: true},
		{name: "waxed_oxidized", block: WaxedOxidizedCopperChest, item: ItemWaxedOxidizedCopperChest, oxidation: 3, waxed: true},
	}
}

func TestUnsupportedItemsAreNotMappedToPlacementStates(t *testing.T) {
	items := []Item{
		ItemWhiteBed,
		ItemTorch,
		ItemRedstone,
		ItemWaterBucket,
	}

	for _, item := range items {
		block, ok := item.PlacementBlock()
		if ok {
			t.Errorf("item %d maps to block %d", item, block)
		}
	}

	block, ok := MaxItemID.PlacementBlock()
	if ok {
		t.Errorf("last catalogue item maps to block %d", block)
	}
}

func TestItemMiningRuleMatchesBlockIDAcrossStates(t *testing.T) {
	vineWithNorthFace, valid := Vine.WithProperties(BlockPropertyValue{Name: "north", Value: "true"})
	if !valid {
		t.Fatal("north-facing vine state is invalid")
	}

	vineWithSouthFace, valid := Vine.WithProperties(BlockPropertyValue{Name: "south", Value: "true"})
	if !valid {
		t.Fatal("south-facing vine state is invalid")
	}

	rule := ItemMiningRule{BlockID: VineID}
	if !rule.matches(vineWithNorthFace) {
		t.Fatal("rule does not match north-facing vine")
	}

	if !rule.matches(vineWithSouthFace) {
		t.Fatal("rule does not match south-facing vine")
	}

	if rule.matches(GlowLichen) {
		t.Fatal("vine rule matches glow lichen")
	}
}
