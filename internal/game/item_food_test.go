package game

import (
	"reflect"
	"testing"
)

type generatedConsumableMetadata struct {
	food       ItemFood
	consumable ItemConsumable
}

func TestGeneratedConsumableMetadata(t *testing.T) {
	cases := map[Item]generatedConsumableMetadata{
		ItemApple: {
			food: ItemFood{Nutrition: 4, Saturation: 2.4},
			consumable: ItemConsumable{
				UseEffects: ItemUseEffects{InteractVibrations: true, SpeedMultiplier: 0.2},
				Particles:  true,
				Sound:      SoundEntityGenericEat,
				Duration:   32,
				Animation:  ItemUseAnimationEat,
			},
		},
		ItemHoneyBottle: {
			food: ItemFood{Nutrition: 6, Saturation: 1.2, AlwaysEdible: true},
			consumable: ItemConsumable{
				UseEffects: ItemUseEffects{InteractVibrations: true, SpeedMultiplier: 0.2},
				Sound:      SoundItemHoneyBottleDrink,
				Duration:   40,
				Animation:  ItemUseAnimationDrink,
				Remainder:  ItemGlassBottle,
				Effects: []ItemConsumeEffect{{
					Type:   ItemConsumeEffectRemoveStatusEffects,
					Remove: []MobEffect{MobEffectPoison},
				}},
			},
		},
		ItemMilkBucket: {
			consumable: ItemConsumable{
				UseEffects: ItemUseEffects{InteractVibrations: true, SpeedMultiplier: 0.2},
				Sound:      SoundEntityGenericDrink,
				Duration:   32,
				Animation:  ItemUseAnimationDrink,
				Remainder:  ItemBucket,
				Effects:    []ItemConsumeEffect{{Type: ItemConsumeEffectClearAllStatusEffects}},
			},
		},
		ItemChorusFruit: {
			food: ItemFood{Nutrition: 4, Saturation: 2.4, AlwaysEdible: true},
			consumable: ItemConsumable{
				UseEffects: ItemUseEffects{InteractVibrations: true, SpeedMultiplier: 0.2},
				Particles:  true,
				Sound:      SoundEntityGenericEat,
				Duration:   32,
				Animation:  ItemUseAnimationEat,
				Effects:    []ItemConsumeEffect{{Type: ItemConsumeEffectTeleportRandomly, Diameter: 16}},
			},
		},
		ItemSuspiciousStew: {
			food: ItemFood{Nutrition: 6, Saturation: 7.2, AlwaysEdible: true},
			consumable: ItemConsumable{
				UseEffects:     ItemUseEffects{InteractVibrations: true, SpeedMultiplier: 0.2},
				Particles:      true,
				Sound:          SoundEntityGenericEat,
				Duration:       32,
				Animation:      ItemUseAnimationEat,
				Remainder:      ItemBowl,
				DynamicEffects: []ItemConsumeEffect{{Type: ItemConsumeEffectSuspiciousStew}},
			},
		},
	}

	for item, expected := range cases {
		definition, valid := item.Definition()
		if !valid {
			t.Fatalf("item %d is invalid", item)
		}

		if definition.Food != expected.food {
			t.Errorf("food metadata for %s = %+v, want %+v", definition.Name, definition.Food, expected.food)
		}

		if !reflect.DeepEqual(definition.Consumable, expected.consumable) {
			t.Errorf("consumable metadata for %s = %+v, want %+v", definition.Name, definition.Consumable, expected.consumable)
		}
	}
}
