package game

import "slices"

//go:generate go run ../../cmd/generate-recipes -items ../../data/items.json -recipes ../../data/recipes -tags ../../data/item_tags -output recipes_generated.go

type Ingredient struct {
	alternatives []Item
}

type ShapedRecipe struct {
	width   int
	height  int
	pattern []Ingredient
}

type Recipe struct {
	name      string
	result    ItemStack
	shaped    *ShapedRecipe
	shapeless []Ingredient
}

type CookingRecipeType uint8

const (
	CookingRecipeSmelting CookingRecipeType = iota
	CookingRecipeSmoking
	CookingRecipeBlasting
)

type CookingRecipe struct {
	name        string
	result      ItemStack
	experience  float32
	cookingTime int32
}

func (ingredient Ingredient) Alternatives() []Item {
	return append([]Item(nil), ingredient.alternatives...)
}

func (ingredient Ingredient) Matches(stack ItemStack) bool {
	if stack.Empty() {
		return false
	}

	return slices.Contains(ingredient.alternatives, stack.Item)
}

func (recipe ShapedRecipe) Width() int {
	return recipe.width
}

func (recipe ShapedRecipe) Height() int {
	return recipe.height
}

func (recipe ShapedRecipe) Pattern() []Ingredient {
	return cloneIngredients(recipe.pattern)
}

func (recipe Recipe) Name() string {
	return recipe.name
}

func (recipe Recipe) Result() ItemStack {
	return recipe.result.Clone()
}

func (recipe Recipe) Shaped() (ShapedRecipe, bool) {
	if recipe.shaped == nil {
		return ShapedRecipe{}, false
	}

	return cloneShapedRecipe(*recipe.shaped), true
}

func (recipe Recipe) Shapeless() []Ingredient {
	return cloneIngredients(recipe.shapeless)
}

// CraftingRecipes returns copies so callers cannot alter the generated catalogue.
func CraftingRecipes() []Recipe {
	recipes := make([]Recipe, len(generatedCraftingRecipes))

	for index, recipe := range generatedCraftingRecipes {
		recipes[index] = cloneRecipe(recipe)
	}

	return recipes
}

// RecipeByName returns a recipe by its bare or minecraft-namespaced identifier.
func RecipeByName(name string) (Recipe, bool) {
	name, valid := generatedName(name)
	if !valid {
		return Recipe{}, false
	}

	for _, recipe := range generatedCraftingRecipes {
		if recipe.name == name {
			return cloneRecipe(recipe), true
		}
	}

	return Recipe{}, false
}

// MatchCrafting matches stacks in a row-major crafting grid.
func MatchCrafting(width, height int, slots []ItemStack) (Recipe, bool) {
	if width <= 0 || height <= 0 || len(slots) != width*height {
		return Recipe{}, false
	}

	for _, recipe := range generatedCraftingRecipes {
		if recipe.shaped != nil {
			if matchShaped(*recipe.shaped, width, height, slots) {
				return cloneRecipe(recipe), true
			}

			continue
		}

		if matchShapeless(recipe.shapeless, slots) {
			return cloneRecipe(recipe), true
		}
	}

	return Recipe{}, false
}

// CraftingRemainder returns the item left after an ordinary crafting ingredient is consumed.
func CraftingRemainder(item Item) (Item, bool) {
	remainder, valid := generatedCraftingRemainders[item]
	return remainder, valid
}

func CookingRecipeFor(recipeType CookingRecipeType, input ItemStack) (CookingRecipe, bool) {
	if input.Empty() {
		return CookingRecipe{}, false
	}

	recipes, valid := generatedCookingRecipes[recipeType]
	if !valid {
		return CookingRecipe{}, false
	}

	recipe, valid := recipes[input.Item]
	if !valid {
		return CookingRecipe{}, false
	}

	recipe.result = recipe.result.Clone()

	return recipe, true
}

func CookingRecipeInputs(recipeType CookingRecipeType) []Item {
	recipes := generatedCookingRecipes[recipeType]

	inputs := make([]Item, 0, len(recipes))

	for input := range recipes {
		inputs = append(inputs, input)
	}

	slices.Sort(inputs)

	return inputs
}

func (recipe CookingRecipe) Name() string {
	return recipe.name
}

func (recipe CookingRecipe) Result() ItemStack {
	return recipe.result.Clone()
}

func (recipe CookingRecipe) Experience() float32 {
	return recipe.experience
}

func (recipe CookingRecipe) CookingTime() int32 {
	return recipe.cookingTime
}

func FuelDuration(item Item) int32 {
	return generatedFuelDurations[item]
}

func IsFuel(item Item) bool {
	_, valid := generatedFuelDurations[item]
	return valid
}

func matchShaped(recipe ShapedRecipe, gridWidth, gridHeight int, slots []ItemStack) bool {
	if recipe.width > gridWidth || recipe.height > gridHeight {
		return false
	}

	for offsetY := 0; offsetY <= gridHeight-recipe.height; offsetY++ {
		for offsetX := 0; offsetX <= gridWidth-recipe.width; offsetX++ {
			if matchShapedAt(recipe, gridWidth, gridHeight, slots, offsetX, offsetY, false) {
				return true
			}

			if matchShapedAt(recipe, gridWidth, gridHeight, slots, offsetX, offsetY, true) {
				return true
			}
		}
	}

	return false
}

func matchShapedAt(recipe ShapedRecipe, gridWidth, gridHeight int, slots []ItemStack, offsetX, offsetY int, mirrored bool) bool {
	for row := range gridHeight {
		for column := range gridWidth {
			patternRow := row - offsetY
			patternColumn := column - offsetX
			insidePattern := patternRow >= 0 && patternRow < recipe.height && patternColumn >= 0 && patternColumn < recipe.width

			stack := slots[row*gridWidth+column]

			if !insidePattern {
				if !stack.Empty() {
					return false
				}

				continue
			}

			if mirrored {
				patternColumn = recipe.width - 1 - patternColumn
			}

			ingredient := recipe.pattern[patternRow*recipe.width+patternColumn]
			if len(ingredient.alternatives) == 0 {
				if !stack.Empty() {
					return false
				}

				continue
			}

			if !ingredient.Matches(stack) {
				return false
			}
		}
	}

	return true
}

func matchShapeless(ingredients []Ingredient, slots []ItemStack) bool {
	occupied := make([]ItemStack, 0, len(ingredients))

	for _, stack := range slots {
		if !stack.Empty() {
			occupied = append(occupied, stack)
		}
	}

	if len(occupied) != len(ingredients) {
		return false
	}

	used := make([]bool, len(occupied))

	return assignShapelessIngredients(ingredients, occupied, used, 0)
}

func assignShapelessIngredients(ingredients []Ingredient, occupied []ItemStack, used []bool, ingredientIndex int) bool {
	if ingredientIndex == len(ingredients) {
		return true
	}

	ingredient := ingredients[ingredientIndex]

	for slotIndex, stack := range occupied {
		if used[slotIndex] || !ingredient.Matches(stack) {
			continue
		}

		used[slotIndex] = true

		if assignShapelessIngredients(ingredients, occupied, used, ingredientIndex+1) {
			return true
		}

		used[slotIndex] = false
	}

	return false
}

func cloneRecipe(recipe Recipe) Recipe {
	clone := recipe

	clone.result = recipe.result.Clone()
	clone.shapeless = cloneIngredients(recipe.shapeless)

	if recipe.shaped != nil {
		shaped := cloneShapedRecipe(*recipe.shaped)
		clone.shaped = &shaped
	}

	return clone
}

func cloneShapedRecipe(recipe ShapedRecipe) ShapedRecipe {
	clone := recipe

	clone.pattern = cloneIngredients(recipe.pattern)

	return clone
}

func cloneIngredients(ingredients []Ingredient) []Ingredient {
	clone := make([]Ingredient, len(ingredients))

	for index, ingredient := range ingredients {
		clone[index].alternatives = append([]Item(nil), ingredient.alternatives...)
	}

	return clone
}
