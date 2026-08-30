//go:generate go run ../../cmd/generate-enchantments -order ../../data/enchantment_order.json -enchantments ../../data/enchantments -enchantment-tags ../../data/enchantment_tags -item-tags ../../data/item_tags -items ../../data/items.json -game-output enchantments_generated.go -protocol-output ../protocol/enchantment_tags_generated.go

package game

import "slices"

type Enchantment int32

type EnchantmentDefinition struct {
	ID             Enchantment
	Name           string
	MaximumLevel   int32
	SupportedItems []Item
	ExclusiveMask  uint64
	Curse          bool
}

func (enchantment Enchantment) Valid() bool {
	return enchantment >= 0 && enchantment <= MaxEnchantmentID
}

func (enchantment Enchantment) Definition() (EnchantmentDefinition, bool) {
	if !enchantment.Valid() {
		return EnchantmentDefinition{}, false
	}

	return generatedEnchantmentDefinitions[enchantment], true
}

func (enchantment Enchantment) Compatible(other Enchantment) bool {
	if enchantment == other {
		return false
	}

	definition, valid := enchantment.Definition()
	if !valid || !other.Valid() {
		return false
	}

	otherDefinition, _ := other.Definition()
	otherMask := uint64(1) << uint(other)
	enchantmentMask := uint64(1) << uint(enchantment)

	return definition.ExclusiveMask&otherMask == 0 && otherDefinition.ExclusiveMask&enchantmentMask == 0
}

func (enchantment Enchantment) Supports(item Item) bool {
	definition, exists := enchantment.Definition()
	if !exists {
		return false
	}

	return slices.Contains(definition.SupportedItems, item)
}

func (enchantment Enchantment) FullName(level int32) TextComponent {
	definition, valid := enchantment.Definition()
	if !valid {
		return TextComponent{}
	}

	color := TextColorGray

	if definition.Curse {
		color = TextColorRed
	}

	name := TranslatableText("enchantment.minecraft." + definition.Name).WithColor(color)

	if level != 1 || definition.MaximumLevel != 1 {
		name = name.Append(LiteralText(" "), TranslatableText("enchantment.level."+formatInt32(level)))
	}

	return name
}

func EnchantmentByName(name string) (Enchantment, bool) {
	for enchantment, definition := range generatedEnchantmentDefinitions {
		if name == definition.Name || name == "minecraft:"+definition.Name {
			return Enchantment(enchantment), true
		}
	}

	return 0, false
}
