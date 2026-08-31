package game

import "testing"

type scriptedBlockLootRandom struct {
	integers   []int
	floats     []float32
	intIndex   int
	floatIndex int
}

type blockLootChanceTest struct {
	name   string
	level  int32
	chance float32
}

func (random *scriptedBlockLootRandom) IntN(bound int) int {
	if random.intIndex >= len(random.integers) {
		panic("unexpected block loot integer draw")
	}

	value := random.integers[random.intIndex]
	random.intIndex++

	if value < 0 || value >= bound {
		panic("scripted block loot integer is outside bound")
	}

	return value
}

func (random *scriptedBlockLootRandom) Float32() float32 {
	if random.floatIndex >= len(random.floats) {
		panic("unexpected block loot float draw")
	}

	value := random.floats[random.floatIndex]
	random.floatIndex++

	return value
}

func TestDiamondOreCanonicalLoot(t *testing.T) {
	for level := int32(1); level <= 3; level++ {
		t.Run("fortune minimum", func(t *testing.T) {
			tool := enchantedLootTool(ItemIronPickaxe, EnchantmentFortune, level)
			random := &scriptedBlockLootRandom{integers: []int{0}}
			assertBlockLoot(t, DiamondOre, tool, random, ItemStack{Item: ItemDiamond, Count: 1})
		})

		t.Run("fortune maximum", func(t *testing.T) {
			tool := enchantedLootTool(ItemIronPickaxe, EnchantmentFortune, level)
			random := &scriptedBlockLootRandom{integers: []int{int(level) + 1}}
			assertBlockLoot(t, DiamondOre, tool, random, ItemStack{Item: ItemDiamond, Count: level + 1})
		})
	}

	assertBlockLoot(t, DiamondOre, ItemStack{Item: ItemIronPickaxe, Count: 1}, &scriptedBlockLootRandom{}, ItemStack{Item: ItemDiamond, Count: 1})

	silkAndFortune := enchantedLootTool(ItemIronPickaxe, EnchantmentSilkTouch, 1)

	silkAndFortune.SetEnchantment(EnchantmentFortune, 3)

	assertBlockLoot(t, DiamondOre, silkAndFortune, &scriptedBlockLootRandom{}, ItemStack{Item: ItemDiamondOre, Count: 1})

	if ItemWoodenPickaxe.IsCorrectToolForDrops(DiamondOre) {
		t.Fatal("wooden pickaxe unexpectedly passes diamond ore harvest gating")
	}

	if !ItemIronPickaxe.IsCorrectToolForDrops(DiamondOre) {
		t.Fatal("iron pickaxe does not pass diamond ore harvest gating")
	}
}

func TestGravelCanonicalLoot(t *testing.T) {
	silk := enchantedLootTool(ItemIronShovel, EnchantmentSilkTouch, 1)

	assertBlockLoot(t, Gravel, silk, &scriptedBlockLootRandom{}, ItemStack{Item: ItemGravel, Count: 1})

	tests := []blockLootChanceTest{
		{name: "unenchanted", chance: 0.1},
		{name: "fortune one", level: 1, chance: 0.14285715},
		{name: "fortune two", level: 2, chance: 0.25},
		{name: "fortune three", level: 3, chance: 1},
	}

	for _, test := range tests {
		t.Run(test.name+" succeeds", func(t *testing.T) {
			tool := enchantedLootTool(ItemIronShovel, EnchantmentFortune, test.level)

			random := &scriptedBlockLootRandom{floats: []float32{test.chance - 0.000001}}

			assertBlockLoot(t, Gravel, tool, random, ItemStack{Item: ItemFlint, Count: 1})
		})

		if test.chance < 1 {

			t.Run(test.name+" falls through", func(t *testing.T) {
				tool := enchantedLootTool(ItemIronShovel, EnchantmentFortune, test.level)

				random := &scriptedBlockLootRandom{floats: []float32{test.chance}}

				assertBlockLoot(t, Gravel, tool, random, ItemStack{Item: ItemGravel, Count: 1})
			})
		}
	}

	overLevel := enchantedLootTool(ItemIronShovel, EnchantmentFortune, 8)

	assertBlockLoot(t, Gravel, overLevel, &scriptedBlockLootRandom{floats: []float32{0.99}}, ItemStack{Item: ItemFlint, Count: 1})
}

func TestOakLeavesCanonicalLootAndDrawOrder(t *testing.T) {
	tools := []ItemStack{
		{Item: ItemShears, Count: 1},
		enchantedLootTool(ItemIronHoe, EnchantmentSilkTouch, 1),
	}

	for _, tool := range tools {
		random := &scriptedBlockLootRandom{}
		assertBlockLoot(t, OakLeaves, tool, random, ItemStack{Item: ItemOakLeaves, Count: 1})
	}

	random := &scriptedBlockLootRandom{
		floats:   []float32{0, 0, 0},
		integers: []int{1},
	}

	assertBlockLoot(
		t,
		OakLeaves,
		ItemStack{Item: ItemAir},
		random,
		ItemStack{Item: ItemOakSapling, Count: 1},
		ItemStack{Item: ItemStick, Count: 2},
		ItemStack{Item: ItemApple, Count: 1},
	)

	if random.floatIndex != 3 || random.intIndex != 1 {
		t.Fatalf("leaf random draws = floats %d, integers %d, want 3 and 1", random.floatIndex, random.intIndex)
	}

	fortune := enchantedLootTool(ItemIronHoe, EnchantmentFortune, 3)

	random = &scriptedBlockLootRandom{floats: []float32{0.09, 1, 1}}

	assertBlockLoot(t, OakLeaves, fortune, random, ItemStack{Item: ItemOakSapling, Count: 1})

	random = &scriptedBlockLootRandom{floats: []float32{0.06, 1, 1}}

	assertBlockLoot(t, OakLeaves, ItemStack{Item: ItemAir}, random)
}

func TestCanonicalStateDependentCounts(t *testing.T) {
	doubleSlab := blockWithLootProperties(t, OakSlab, BlockPropertyValue{Name: "type", Value: "double"})

	assertBlockLoot(t, doubleSlab, ItemStack{Item: ItemAir}, &scriptedBlockLootRandom{}, ItemStack{Item: ItemOakSlab, Count: 2})

	fourCandles := blockWithLootProperties(t, Candle, BlockPropertyValue{Name: "candles", Value: "4"})

	assertBlockLoot(t, fourCandles, ItemStack{Item: ItemAir}, &scriptedBlockLootRandom{}, ItemStack{Item: ItemCandle, Count: 4})
}

func enchantedLootTool(item Item, enchantment Enchantment, level int32) ItemStack {
	stack := ItemStack{Item: item, Count: 1}

	if level > 0 {
		stack.SetEnchantment(enchantment, level)
	}

	return stack
}

func blockWithLootProperties(t *testing.T, block Block, properties ...BlockPropertyValue) Block {
	t.Helper()

	result, valid := block.WithProperties(properties...)
	if !valid {
		t.Fatalf("properties %+v are invalid for block %d", properties, block)
	}

	return result
}

func assertBlockLoot(t *testing.T, block Block, tool ItemStack, random BlockLootRandom, expected ...ItemStack) {
	t.Helper()

	actual := EvaluateBlockLoot(BlockLootContext{Block: block, Tool: tool, HasActor: true}, random)
	if len(actual) != len(expected) {
		t.Fatalf("loot = %+v, want %+v", actual, expected)
	}

	for index := range expected {
		if !actual[index].Equal(expected[index]) {
			t.Fatalf("loot = %+v, want %+v", actual, expected)
		}
	}
}
