package game

import "testing"

type craftingRemainderTestCase struct {
	input  Item
	output Item
}

type cookingRecipeTestCase struct {
	name       string
	recipeType CookingRecipeType
	input      Item
	result     Item
	time       int32
}

func TestMatchShapedSupportsOffsetAndMirror(t *testing.T) {
	recipe := ShapedRecipe{
		width:  2,
		height: 2,
		pattern: []Ingredient{
			{alternatives: []Item{ItemStick}},
			{alternatives: []Item{ItemCoal}},
			{alternatives: []Item{ItemIronIngot}},
			{},
		},
	}

	slots := []ItemStack{
		{}, {}, {},
		{}, {Item: ItemCoal, Count: 1}, {Item: ItemStick, Count: 1},
		{}, {}, {Item: ItemIronIngot, Count: 1},
	}

	matched := matchShaped(recipe, 3, 3, slots)
	if !matched {
		t.Fatal("expected offset mirrored shaped recipe to match")
	}

	slots[0] = ItemStack{Item: ItemStick, Count: 1}

	matched = matchShaped(recipe, 3, 3, slots)
	if matched {
		t.Fatal("expected occupied cell outside pattern to reject shaped recipe")
	}
}

func TestMatchShapelessAssignsAlternativesWithoutReusingSlots(t *testing.T) {
	ingredients := []Ingredient{
		{alternatives: []Item{ItemIronIngot, ItemGoldIngot}},
		{alternatives: []Item{ItemIronIngot}},
	}

	slots := []ItemStack{
		{Item: ItemIronIngot, Count: 1},
		{Item: ItemGoldIngot, Count: 1},
	}

	matched := matchShapeless(ingredients, slots)
	if !matched {
		t.Fatal("expected bipartite shapeless assignment to match")
	}

	slots[1] = ItemStack{Item: ItemStick, Count: 1}

	matched = matchShapeless(ingredients, slots)
	if matched {
		t.Fatal("expected unmatched shapeless ingredient to reject recipe")
	}
}

func TestThreeByThreeRecipeRejectsTwoByTwoGrid(t *testing.T) {
	recipe, found := RecipeByName("iron_pickaxe")
	if !found {
		t.Fatal("iron pickaxe recipe not found")
	}

	shaped, found := recipe.Shaped()
	if !found || shaped.Width() != 3 || shaped.Height() != 3 {
		t.Fatalf("iron pickaxe recipe dimensions = %dx%d, want 3x3", shaped.Width(), shaped.Height())
	}

	slots := []ItemStack{
		{Item: ItemIronIngot, Count: 1},
		{Item: ItemIronIngot, Count: 1},
		{Item: ItemIronIngot, Count: 1},
		{Item: ItemStick, Count: 1},
	}

	matched := matchShaped(shaped, 2, 2, slots)
	if matched {
		t.Fatal("expected 3x3 crafting recipe to reject a 2x2 grid")
	}
}

func TestCraftingCatalogueAndCopies(t *testing.T) {
	recipes := CraftingRecipes()
	if len(recipes) != 1010 {
		t.Fatalf("recipe count = %d, want 1010", len(recipes))
	}

	stick, found := RecipeByName("minecraft:stick")
	if !found {
		t.Fatal("stick recipe not found")
	}

	result := stick.Result()
	if result.Item != ItemStick || result.Count != 4 {
		t.Fatalf("stick result = %+v, want four sticks", result)
	}

	shaped, found := stick.Shaped()
	if !found {
		t.Fatal("stick recipe is not shaped")
	}

	pattern := shaped.Pattern()

	alternatives := pattern[0].Alternatives()

	alternatives[0] = ItemAir

	shaped, found = stick.Shaped()
	if !found || shaped.Pattern()[0].Alternatives()[0] == ItemAir {
		t.Fatal("recipe accessors exposed mutable generated ingredients")
	}
}

func TestCraftingRemainders(t *testing.T) {
	testCases := []craftingRemainderTestCase{
		{input: ItemWaterBucket, output: ItemBucket},
		{input: ItemLavaBucket, output: ItemBucket},
		{input: ItemMilkBucket, output: ItemBucket},
		{input: ItemDragonBreath, output: ItemGlassBottle},
		{input: ItemHoneyBottle, output: ItemGlassBottle},
	}

	for _, testCase := range testCases {
		remainder, found := CraftingRemainder(testCase.input)
		if !found || remainder != testCase.output {
			t.Fatalf("remainder for %d = %d, %t; want %d, true", testCase.input, remainder, found, testCase.output)
		}
	}

	_, found := CraftingRemainder(ItemStick)
	if found {
		t.Fatal("stick unexpectedly has a crafting remainder")
	}
}

func TestGeneratedCookingRecipesAndFuelMetadata(t *testing.T) {
	tests := []cookingRecipeTestCase{
		{name: "smelting", recipeType: CookingRecipeSmelting, input: ItemPotato, result: ItemBakedPotato, time: 200},
		{name: "smoking", recipeType: CookingRecipeSmoking, input: ItemPotato, result: ItemBakedPotato, time: 100},
		{name: "blasting", recipeType: CookingRecipeBlasting, input: ItemRawIron, result: ItemIronIngot, time: 100},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recipe, valid := CookingRecipeFor(test.recipeType, ItemStack{Item: test.input, Count: 1})
			if !valid || recipe.Result().Item != test.result || recipe.CookingTime() != test.time {
				t.Fatalf("cooking recipe = %+v, %v", recipe, valid)
			}
		})
	}

	_, valid := CookingRecipeFor(CookingRecipeBlasting, ItemStack{Item: ItemPotato, Count: 1})
	if valid {
		t.Fatal("potato unexpectedly has a blasting recipe")
	}

	if FuelDuration(ItemLavaBucket) != 20000 || FuelDuration(ItemCoal) != 1600 || FuelDuration(ItemDriedKelpBlock) != 4001 {
		t.Fatalf("fuel durations = lava %d, coal %d, kelp %d", FuelDuration(ItemLavaBucket), FuelDuration(ItemCoal), FuelDuration(ItemDriedKelpBlock))
	}

	if IsFuel(ItemCrimsonPlanks) || IsFuel(ItemWarpedStem) {
		t.Fatal("non-flammable wood unexpectedly generated as fuel")
	}

	remainder, valid := CraftingRemainder(ItemLavaBucket)
	if !valid || remainder != ItemBucket {
		t.Fatalf("lava bucket remainder = %v, %v", remainder, valid)
	}
}
