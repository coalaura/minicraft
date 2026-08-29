package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

const (
	craftingShaped    = "minecraft:crafting_shaped"
	craftingShapeless = "minecraft:crafting_shapeless"
	cookingSmelting   = "minecraft:smelting"
	cookingSmoking    = "minecraft:smoking"
	cookingBlasting   = "minecraft:blasting"
)

type itemDefinition struct {
	ID   uint16 `json:"id"`
	Name string `json:"name"`
}

type ingredientSource []string

type recipeSource struct {
	Type        string                      `json:"type"`
	Key         map[string]ingredientSource `json:"key"`
	Pattern     []string                    `json:"pattern"`
	Ingredients []ingredientSource          `json:"ingredients"`
	Ingredient  ingredientSource            `json:"ingredient"`
	Result      recipeResultSource          `json:"result"`
	Experience  float32                     `json:"experience"`
	CookingTime int32                       `json:"cookingtime"`
}

type recipeTypeSource struct {
	Type string `json:"type"`
}

type recipeResultSource struct {
	Count      int32           `json:"count"`
	ID         string          `json:"id"`
	Components json.RawMessage `json:"components"`
}

type tagSource struct {
	Values []string `json:"values"`
}

type generatedIngredient struct {
	Alternatives []uint16
}

type generatedRecipe struct {
	Name        string
	Result      uint16
	Count       int32
	Width       int
	Height      int
	Pattern     []generatedIngredient
	Ingredients []generatedIngredient
}

type generatedCookingRecipe struct {
	Name        string
	Type        string
	Ingredient  generatedIngredient
	Result      uint16
	Count       int32
	Experience  float32
	CookingTime int32
}

type fuelRule struct {
	name     string
	duration int32
}

type tagResolver struct {
	items     map[string]uint16
	tagsPath  string
	resolved  map[string][]uint16
	resolving map[string]bool
}

func main() {
	itemsPath := flag.String("items", "", "path to items.json")
	recipesPath := flag.String("recipes", "", "path to minecraft recipe directory")
	tagsPath := flag.String("tags", "", "path to minecraft item tag directory")
	outputPath := flag.String("output", "", "generated Go output path")

	flag.Parse()

	if *itemsPath == "" || *recipesPath == "" || *tagsPath == "" || *outputPath == "" {
		fail(fmt.Errorf("items, recipes, tags and output are required"))
	}

	items, err := readItems(*itemsPath)
	if err != nil {
		fail(err)
	}

	resolver := tagResolver{items: items, tagsPath: *tagsPath, resolved: make(map[string][]uint16), resolving: make(map[string]bool)}

	recipes, cookingRecipes, err := readRecipes(*recipesPath, &resolver)
	if err != nil {
		fail(err)
	}

	fuels, err := generateFuels(&resolver)
	if err != nil {
		fail(err)
	}

	generated, err := generate(recipes, cookingRecipes, fuels, items)
	if err != nil {
		fail(err)
	}

	err = os.WriteFile(*outputPath, generated, 0o644)
	if err != nil {
		fail(err)
	}
}

func (source *ingredientSource) UnmarshalJSON(raw []byte) error {
	var single string

	err := json.Unmarshal(raw, &single)
	if err == nil {
		*source = []string{single}

		return nil
	}

	var alternatives []string

	err = json.Unmarshal(raw, &alternatives)
	if err != nil {
		return err
	}

	*source = alternatives

	return nil
}

func readItems(path string) (map[string]uint16, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var definitions []itemDefinition

	err = json.Unmarshal(raw, &definitions)
	if err != nil {
		return nil, err
	}

	items := make(map[string]uint16, len(definitions))

	for _, definition := range definitions {
		items[definition.Name] = definition.ID
	}

	return items, nil
}

func readRecipes(path string, resolver *tagResolver) ([]generatedRecipe, []generatedCookingRecipe, error) {
	paths, err := filepath.Glob(filepath.Join(path, "*.json"))
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(paths)

	recipes := make([]generatedRecipe, 0, len(paths))
	cookingRecipes := make([]generatedCookingRecipe, 0, 107)

	for _, recipePath := range paths {
		raw, readErr := os.ReadFile(recipePath)
		if readErr != nil {
			return nil, nil, readErr
		}

		var recipeType recipeTypeSource

		parseErr := json.Unmarshal(raw, &recipeType)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", recipePath, parseErr)
		}

		isCrafting := recipeType.Type == craftingShaped || recipeType.Type == craftingShapeless
		isCooking := recipeType.Type == cookingSmelting || recipeType.Type == cookingSmoking || recipeType.Type == cookingBlasting

		if !isCrafting && !isCooking {
			continue
		}

		var source recipeSource

		parseErr = json.Unmarshal(raw, &source)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", recipePath, parseErr)
		}

		if len(source.Result.Components) != 0 {
			continue
		}

		name := strings.TrimSuffix(filepath.Base(recipePath), ".json")

		if isCooking {
			recipe, convertErr := convertCookingRecipe(name, source, resolver)
			if convertErr != nil {
				return nil, nil, fmt.Errorf("convert %s: %w", recipePath, convertErr)
			}

			cookingRecipes = append(cookingRecipes, recipe)

			continue
		}

		recipe, convertErr := convertRecipe(name, source, resolver)
		if convertErr != nil {
			return nil, nil, fmt.Errorf("convert %s: %w", recipePath, convertErr)
		}

		recipes = append(recipes, recipe)
	}

	if len(recipes) != 1010 {
		return nil, nil, fmt.Errorf("generated %d ordinary recipes, want 1010", len(recipes))
	}

	if len(cookingRecipes) != 107 {
		return nil, nil, fmt.Errorf("generated %d cooking recipes, want 107", len(cookingRecipes))
	}

	return recipes, cookingRecipes, nil
}

func convertCookingRecipe(name string, source recipeSource, resolver *tagResolver) (generatedCookingRecipe, error) {
	result, ok := resolver.items[bareName(source.Result.ID)]
	if !ok || source.CookingTime <= 0 {
		return generatedCookingRecipe{}, fmt.Errorf("invalid cooking recipe result %q or time %d", source.Result.ID, source.CookingTime)
	}

	count := source.Result.Count
	if count == 0 {
		count = 1
	}

	ingredient, err := resolver.resolveIngredient(source.Ingredient)
	if err != nil {
		return generatedCookingRecipe{}, err
	}

	return generatedCookingRecipe{
		Name: name, Type: source.Type, Ingredient: ingredient, Result: result, Count: count,
		Experience: source.Experience, CookingTime: source.CookingTime,
	}, nil
}

func convertRecipe(name string, source recipeSource, resolver *tagResolver) (generatedRecipe, error) {
	result, ok := resolver.items[bareName(source.Result.ID)]
	if !ok || source.Result.Count <= 0 {
		return generatedRecipe{}, fmt.Errorf("invalid result %q", source.Result.ID)
	}

	recipe := generatedRecipe{Name: name, Result: result, Count: source.Result.Count}

	if source.Type == craftingShapeless {
		if len(source.Ingredients) == 0 {
			return generatedRecipe{}, fmt.Errorf("empty shapeless recipe")
		}

		recipe.Ingredients = make([]generatedIngredient, len(source.Ingredients))

		for index, sourceIngredient := range source.Ingredients {
			ingredient, err := resolver.resolveIngredient(sourceIngredient)
			if err != nil {
				return generatedRecipe{}, err
			}

			recipe.Ingredients[index] = ingredient
		}

		return recipe, nil
	}

	if len(source.Pattern) == 0 || len(source.Pattern[0]) == 0 {
		return generatedRecipe{}, fmt.Errorf("empty shaped pattern")
	}

	recipe.Width = len(source.Pattern[0])
	recipe.Height = len(source.Pattern)
	recipe.Pattern = make([]generatedIngredient, recipe.Width*recipe.Height)

	for row, patternRow := range source.Pattern {
		if len(patternRow) != recipe.Width {
			return generatedRecipe{}, fmt.Errorf("uneven shaped pattern")
		}

		for column, symbol := range patternRow {
			if symbol == ' ' {
				continue
			}

			sourceIngredient, ok := source.Key[string(symbol)]
			if !ok {
				return generatedRecipe{}, fmt.Errorf("missing key for %q", symbol)
			}

			ingredient, err := resolver.resolveIngredient(sourceIngredient)
			if err != nil {
				return generatedRecipe{}, err
			}

			recipe.Pattern[row*recipe.Width+column] = ingredient
		}
	}

	return recipe, nil
}

func (resolver *tagResolver) resolveIngredient(source ingredientSource) (generatedIngredient, error) {
	if len(source) == 0 {
		return generatedIngredient{}, fmt.Errorf("empty ingredient")
	}

	ingredient := generatedIngredient{}

	for _, value := range source {
		alternatives, err := resolver.resolveValue(value)
		if err != nil {
			return generatedIngredient{}, err
		}

		ingredient.Alternatives = append(ingredient.Alternatives, alternatives...)
	}

	slices.Sort(ingredient.Alternatives)
	ingredient.Alternatives = slices.Compact(ingredient.Alternatives)

	return ingredient, nil
}

func (resolver *tagResolver) resolveValue(value string) ([]uint16, error) {
	if strings.HasPrefix(value, "#") {
		return resolver.resolveTag(bareName(value[1:]))
	}

	item, ok := resolver.items[bareName(value)]
	if !ok {
		return nil, fmt.Errorf("unknown item %q", value)
	}

	return []uint16{item}, nil
}

func (resolver *tagResolver) resolveTag(name string) ([]uint16, error) {
	cached, ok := resolver.resolved[name]
	if ok {
		return cached, nil
	}

	if resolver.resolving[name] {
		return nil, fmt.Errorf("cyclic tag %q", name)
	}

	resolver.resolving[name] = true
	defer delete(resolver.resolving, name)

	raw, err := os.ReadFile(filepath.Join(resolver.tagsPath, name+".json"))
	if err != nil {
		return nil, fmt.Errorf("read tag %q: %w", name, err)
	}

	var source tagSource

	err = json.Unmarshal(raw, &source)
	if err != nil {
		return nil, fmt.Errorf("parse tag %q: %w", name, err)
	}

	var alternatives []uint16

	for _, value := range source.Values {
		resolved, resolveErr := resolver.resolveValue(value)
		if resolveErr != nil {
			return nil, resolveErr
		}

		alternatives = append(alternatives, resolved...)
	}

	slices.Sort(alternatives)
	alternatives = slices.Compact(alternatives)

	resolver.resolved[name] = alternatives

	return alternatives, nil
}

func generateFuels(resolver *tagResolver) (map[uint16]int32, error) {
	fuels := make(map[uint16]int32)

	addItem := func(name string, duration int32) error {
		item, valid := resolver.items[name]
		if !valid {
			return fmt.Errorf("unknown fuel item %q", name)
		}

		fuels[item] = duration

		return nil
	}

	addTag := func(name string, duration int32) error {
		items, err := resolver.resolveTag(name)
		if err != nil {
			return err
		}

		for _, item := range items {
			fuels[item] = duration
		}

		return nil
	}

	itemRules := []fuelRule{
		{"lava_bucket", 20000}, {"coal_block", 16000}, {"blaze_rod", 2400}, {"coal", 1600}, {"charcoal", 1600},
		{"bamboo_mosaic", 300}, {"bamboo_mosaic_stairs", 300}, {"bamboo_mosaic_slab", 150}, {"note_block", 300},
		{"bookshelf", 300}, {"chiseled_bookshelf", 300}, {"lectern", 300}, {"jukebox", 300}, {"chest", 300},
		{"trapped_chest", 300}, {"crafting_table", 300}, {"daylight_detector", 300}, {"bow", 300},
		{"fishing_rod", 300}, {"ladder", 300}, {"wooden_shovel", 200}, {"wooden_sword", 200}, {"wooden_spear", 200},
		{"wooden_hoe", 200}, {"wooden_axe", 200}, {"wooden_pickaxe", 200}, {"stick", 100}, {"bowl", 100},
		{"dried_kelp_block", 4001}, {"crossbow", 300}, {"bamboo", 50}, {"dead_bush", 100}, {"short_dry_grass", 100},
		{"tall_dry_grass", 100}, {"scaffolding", 50}, {"loom", 300}, {"barrel", 300}, {"cartography_table", 300},
		{"fletching_table", 300}, {"smithing_table", 300}, {"composter", 300}, {"azalea", 100},
		{"flowering_azalea", 100}, {"mangrove_roots", 300}, {"leaf_litter", 100},
	}

	tagRules := []fuelRule{
		{"logs", 300}, {"bamboo_blocks", 300}, {"planks", 300}, {"wooden_stairs", 300}, {"wooden_slabs", 150},
		{"wooden_trapdoors", 300}, {"wooden_pressure_plates", 300}, {"wooden_shelves", 300}, {"wooden_fences", 300},
		{"fence_gates", 300}, {"banners", 300}, {"signs", 200}, {"hanging_signs", 800}, {"wooden_doors", 200},
		{"boats", 1200}, {"wool", 100}, {"wooden_buttons", 100}, {"saplings", 100}, {"wool_carpets", 67},
	}

	for _, rule := range itemRules {
		err := addItem(rule.name, rule.duration)
		if err != nil {
			return nil, err
		}
	}

	for _, rule := range tagRules {
		err := addTag(rule.name, rule.duration)
		if err != nil {
			return nil, err
		}
	}

	nonFlammable, err := resolver.resolveTag("non_flammable_wood")
	if err != nil {
		return nil, err
	}

	for _, item := range nonFlammable {
		delete(fuels, item)
	}

	return fuels, nil
}

func generate(recipes []generatedRecipe, cookingRecipes []generatedCookingRecipe, fuels map[uint16]int32, items map[string]uint16) ([]byte, error) {
	var output bytes.Buffer

	fmt.Fprintln(&output, "// Code generated by cmd/generate-recipes; DO NOT EDIT.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "package game")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedCraftingRecipes = []Recipe{")

	for _, recipe := range recipes {
		fmt.Fprintf(&output, "\t{name: %q, result: ItemStack{Item: Item(%d), Count: %d}", recipe.Name, recipe.Result, recipe.Count)

		if len(recipe.Pattern) != 0 {
			fmt.Fprintf(&output, ", shaped: &ShapedRecipe{width: %d, height: %d, pattern: ", recipe.Width, recipe.Height)

			writeIngredients(&output, recipe.Pattern)

			fmt.Fprint(&output, "}")
		} else {
			fmt.Fprint(&output, ", shapeless: ")

			writeIngredients(&output, recipe.Ingredients)
		}

		fmt.Fprintln(&output, "},")
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedCookingRecipes = map[CookingRecipeType]map[Item]CookingRecipe{")

	cookingTypes := []string{cookingSmelting, cookingSmoking, cookingBlasting}

	for _, recipeType := range cookingTypes {
		fmt.Fprintf(&output, "\t%s: {\n", cookingTypeIdentifier(recipeType))

		for _, recipe := range cookingRecipes {
			if recipe.Type != recipeType {
				continue
			}

			for _, input := range recipe.Ingredient.Alternatives {
				fmt.Fprintf(&output, "\t\tItem(%d): {name: %q, result: ItemStack{Item: Item(%d), Count: %d}, experience: %g, cookingTime: %d},\n", input, recipe.Name, recipe.Result, recipe.Count, recipe.Experience, recipe.CookingTime)
			}
		}

		fmt.Fprintln(&output, "\t},")
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedFuelDurations = map[Item]int32{")

	fuelItems := make([]int, 0, len(fuels))

	for item := range fuels {
		fuelItems = append(fuelItems, int(item))
	}

	sort.Ints(fuelItems)

	for _, item := range fuelItems {
		fmt.Fprintf(&output, "\tItem(%d): %d,\n", item, fuels[uint16(item)])
	}

	fmt.Fprintln(&output, "}")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "var generatedCraftingRemainders = map[Item]Item{")

	craftingRemainders := [][2]string{{"water_bucket", "bucket"}, {"lava_bucket", "bucket"}, {"milk_bucket", "bucket"}, {"dragon_breath", "glass_bottle"}, {"honey_bottle", "glass_bottle"}}

	for _, pair := range craftingRemainders {
		fmt.Fprintf(&output, "\tItem(%d): Item(%d),\n", items[pair[0]], items[pair[1]])
	}

	fmt.Fprintln(&output, "}")

	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}

	return formatted, nil
}

func cookingTypeIdentifier(recipeType string) string {
	switch recipeType {
	case cookingSmelting:
		return "CookingRecipeSmelting"
	case cookingSmoking:
		return "CookingRecipeSmoking"
	case cookingBlasting:
		return "CookingRecipeBlasting"
	default:
		panic("unsupported cooking recipe type")
	}
}

func writeIngredients(output *bytes.Buffer, ingredients []generatedIngredient) {
	fmt.Fprint(output, "[]Ingredient{")

	for _, ingredient := range ingredients {
		fmt.Fprint(output, "{alternatives: []Item{")

		for _, alternative := range ingredient.Alternatives {
			fmt.Fprintf(output, "Item(%d),", alternative)
		}

		fmt.Fprint(output, "}},")
	}

	fmt.Fprint(output, "}")
}

func bareName(name string) string {
	return strings.TrimPrefix(name, "minecraft:")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)

	os.Exit(1)
}
