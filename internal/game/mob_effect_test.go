package game

import "testing"

func TestMobEffectRegistry(t *testing.T) {
	if MaxMobEffectID != 39 {
		t.Fatalf("maximum effect ID = %d, want 39", MaxMobEffectID)
	}

	for effect := MobEffect(0); effect <= MaxMobEffectID; effect++ {
		definition, valid := effect.Definition()
		if !valid || definition.ID != effect || definition.Name == "" {
			t.Fatalf("effect %d definition = %+v, valid %t", effect, definition, valid)
		}
	}

	names := []string{"speed", "minecraft:breath_of_the_nautilus"}

	for _, name := range names {
		effect, found := MobEffectByName(name)
		if !found || !effect.Valid() {
			t.Fatalf("effect %q = %d, found %t", name, effect, found)
		}
	}

	if MobEffect(-1).Valid() || MobEffect(MaxMobEffectID+1).Valid() {
		t.Fatal("out-of-range effects are valid")
	}
}
