package game

type Enchantment int32

type EnchantmentDefinition struct {
	ID                Enchantment
	Name              string
	MaximumLevel      int32
	EnchantCategories ItemEnchantCategory
}

const (
	EnchantmentEfficiency Enchantment = 20
	EnchantmentSilkTouch  Enchantment = 21
	EnchantmentUnbreaking Enchantment = 22
	EnchantmentFortune    Enchantment = 23
	maxEnchantmentID      Enchantment = 42
)

var enchantmentDefinitions = map[Enchantment]EnchantmentDefinition{
	EnchantmentEfficiency: {ID: EnchantmentEfficiency, Name: "efficiency", MaximumLevel: 5, EnchantCategories: ItemEnchantCategoryMining},
	EnchantmentSilkTouch:  {ID: EnchantmentSilkTouch, Name: "silk_touch", MaximumLevel: 1, EnchantCategories: ItemEnchantCategoryMiningLoot},
	EnchantmentUnbreaking: {ID: EnchantmentUnbreaking, Name: "unbreaking", MaximumLevel: 3, EnchantCategories: ItemEnchantCategoryDurability},
	EnchantmentFortune:    {ID: EnchantmentFortune, Name: "fortune", MaximumLevel: 3, EnchantCategories: ItemEnchantCategoryMiningLoot},
}

func (enchantment Enchantment) Valid() bool {
	return enchantment >= 0 && enchantment <= maxEnchantmentID
}

func (enchantment Enchantment) Definition() (EnchantmentDefinition, bool) {
	definition, exists := enchantmentDefinitions[enchantment]

	return definition, exists
}

func (enchantment Enchantment) Compatible(other Enchantment) bool {
	return enchantment == other ||
		(enchantment != EnchantmentSilkTouch || other != EnchantmentFortune) &&
			(enchantment != EnchantmentFortune || other != EnchantmentSilkTouch)
}

func (enchantment Enchantment) Supports(item Item) bool {
	definition, exists := enchantment.Definition()
	if !exists {
		return false
	}

	itemDefinition, exists := item.Definition()
	if !exists {
		return false
	}

	return itemDefinition.EnchantCategories&definition.EnchantCategories != 0
}

func EnchantmentByName(name string) (Enchantment, bool) {
	for enchantment, definition := range enchantmentDefinitions {
		if name == definition.Name || name == "minecraft:"+definition.Name {
			return enchantment, true
		}
	}

	return 0, false
}
