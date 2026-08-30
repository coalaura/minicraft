package game

import "testing"

func TestGeneratedEnchantmentDefinitions(t *testing.T) {
	if len(generatedEnchantmentDefinitions) != int(MaxEnchantmentID)+1 {
		t.Fatalf("enchantment definitions = %d, want %d", len(generatedEnchantmentDefinitions), MaxEnchantmentID+1)
	}

	for identity, definition := range generatedEnchantmentDefinitions {
		enchantment := Enchantment(identity)
		if definition.ID != enchantment || definition.Name == "" || definition.MaximumLevel < 1 || len(definition.SupportedItems) == 0 {
			t.Fatalf("invalid enchantment definition %d: %+v", identity, definition)
		}

		resolved, valid := EnchantmentByName("minecraft:" + definition.Name)
		if !valid || resolved != enchantment {
			t.Fatalf("resolve minecraft:%s = %d, %v; want %d, true", definition.Name, resolved, valid, enchantment)
		}
	}
}

func TestGeneratedEnchantmentApplicabilityAndCompatibility(t *testing.T) {
	if !EnchantmentSharpness.Supports(ItemIronSword) || EnchantmentSharpness.Supports(ItemBow) {
		t.Fatal("sharpness supported-item set does not match canonical tags")
	}

	if !EnchantmentPower.Supports(ItemBow) || EnchantmentPower.Supports(ItemCrossbow) {
		t.Fatal("power supported-item set does not match canonical tags")
	}

	if EnchantmentSilkTouch.Compatible(EnchantmentFortune) || EnchantmentFortune.Compatible(EnchantmentSilkTouch) {
		t.Fatal("silk touch and fortune are compatible")
	}

	if EnchantmentSharpness.Compatible(EnchantmentSmite) || EnchantmentInfinity.Compatible(EnchantmentMending) {
		t.Fatal("canonical exclusive-set enchantments are compatible")
	}

	if EnchantmentEfficiency.Compatible(EnchantmentEfficiency) {
		t.Fatal("an enchantment is compatible with itself")
	}

	if !EnchantmentEfficiency.Compatible(EnchantmentUnbreaking) {
		t.Fatal("ordinary non-exclusive enchantments are incompatible")
	}
}

func TestEnchantmentFullName(t *testing.T) {
	efficiency := EnchantmentEfficiency.FullName(5)
	if efficiency.Translate != "enchantment.minecraft.efficiency" || efficiency.Style.Color != TextColorGray || len(efficiency.Siblings) != 2 || efficiency.Siblings[1].Translate != "enchantment.level.5" {
		t.Fatalf("efficiency full name = %+v", efficiency)
	}

	binding := EnchantmentBindingCurse.FullName(1)
	if binding.Translate != "enchantment.minecraft.binding_curse" || binding.Style.Color != TextColorRed || len(binding.Siblings) != 0 {
		t.Fatalf("binding curse full name = %+v", binding)
	}
}
