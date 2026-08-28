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
	Result      recipeResultSource          `json:"result"`
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

	recipes, err := readRecipes(*recipesPath, &resolver)
	if err != nil {
		fail(err)
	}

	generated, err := generate(recipes)
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

func readRecipes(path string, resolver *tagResolver) ([]generatedRecipe, error) {
	paths, err := filepath.Glob(filepath.Join(path, "*.json"))
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)

	recipes := make([]generatedRecipe, 0, len(paths))

	for _, recipePath := range paths {
		raw, readErr := os.ReadFile(recipePath)
		if readErr != nil {
			return nil, readErr
		}

		var recipeType recipeTypeSource

		parseErr := json.Unmarshal(raw, &recipeType)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", recipePath, parseErr)
		}

		if recipeType.Type != craftingShaped && recipeType.Type != craftingShapeless {
			continue
		}

		var source recipeSource

		parseErr = json.Unmarshal(raw, &source)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", recipePath, parseErr)
		}

		if len(source.Result.Components) != 0 {
			continue
		}

		recipe, convertErr := convertRecipe(strings.TrimSuffix(filepath.Base(recipePath), ".json"), source, resolver)
		if convertErr != nil {
			return nil, fmt.Errorf("convert %s: %w", recipePath, convertErr)
		}

		recipes = append(recipes, recipe)
	}

	if len(recipes) != 1010 {
		return nil, fmt.Errorf("generated %d ordinary recipes, want 1010", len(recipes))
	}

	return recipes, nil
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

func generate(recipes []generatedRecipe) ([]byte, error) {
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

	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("format generated source: %w", err)
	}

	return formatted, nil
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
