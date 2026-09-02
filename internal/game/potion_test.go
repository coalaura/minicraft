package game

import "testing"

func TestPotionRegistry(t *testing.T) {
	if len(generatedPotionDefinitions) != 46 {
		t.Fatalf("potion definitions = %d, want 46", len(generatedPotionDefinitions))
	}

	for potion := Potion(0); int(potion) < len(generatedPotionDefinitions); potion++ {
		definition, valid := potion.Definition()
		if !valid || definition.Name == "" {
			t.Fatalf("potion %d definition = %+v, valid %t", potion, definition, valid)
		}

		resolved, found := PotionByName("minecraft:" + definition.Name)
		if !found || resolved != potion {
			t.Fatalf("potion %q resolved to %d, found %t", definition.Name, resolved, found)
		}
	}

	if Potion(-1).Valid() || Potion(len(generatedPotionDefinitions)).Valid() {
		t.Fatal("out-of-range potions are valid")
	}
}

func TestPotionDefinitionEffects(t *testing.T) {
	potion, found := PotionByName("long_turtle_master")
	if !found {
		t.Fatal("long_turtle_master not found")
	}

	definition, valid := potion.Definition()
	if !valid || len(definition.Effects) != 2 {
		t.Fatalf("long_turtle_master definition = %+v, valid %t", definition, valid)
	}

	if definition.Effects[0].Effect != MobEffectSlowness || definition.Effects[0].Duration != 800 || definition.Effects[0].Amplifier != 3 {
		t.Fatalf("long_turtle_master first effect = %+v", definition.Effects[0])
	}

	if definition.Effects[1].Effect != MobEffectResistance || definition.Effects[1].Duration != 800 || definition.Effects[1].Amplifier != 2 {
		t.Fatalf("long_turtle_master second effect = %+v", definition.Effects[1])
	}
}
