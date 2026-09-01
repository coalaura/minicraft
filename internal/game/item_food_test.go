package game

import "testing"

func TestGeneratedFoodMetadata(t *testing.T) {
	cases := map[Item]ItemFood{
		ItemApple: {
			Nutrition:    4,
			Saturation:   2.4,
			ConsumeTicks: 32,
			Animation:    ItemUseAnimationEat,
			Sound:        SoundEntityGenericEat,
		},
		ItemDriedKelp: {
			Nutrition:    1,
			Saturation:   0.6,
			ConsumeTicks: 16,
			Animation:    ItemUseAnimationEat,
			Sound:        SoundEntityGenericEat,
		},
		ItemMushroomStew: {
			Nutrition:    6,
			Saturation:   7.2,
			ConsumeTicks: 32,
			Animation:    ItemUseAnimationEat,
			Sound:        SoundEntityGenericEat,
			Remainder:    ItemBowl,
		},
		ItemHoneyBottle: {
			Nutrition:       6,
			Saturation:      1.2,
			ConsumeTicks:    40,
			Animation:       ItemUseAnimationDrink,
			Sound:           SoundItemHoneyBottleDrink,
			Remainder:       ItemGlassBottle,
			AlwaysEdible:    true,
			DeferredEffects: true,
		},
	}

	for item, expected := range cases {
		definition, valid := item.Definition()
		if !valid {
			t.Fatalf("item %d is invalid", item)
		}

		if definition.Food != expected {
			t.Errorf("food metadata for %s = %+v, want %+v", definition.Name, definition.Food, expected)
		}
	}
}
