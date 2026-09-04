//go:generate go run ../../cmd/generate-mob-effects -manifest ../../data/mob_effects.json -output mob_effects_generated.go

package game

const (
	InfiniteMobEffectDuration int32 = -1
	MinMobEffectAmplifier     int32 = 0
	MaxMobEffectAmplifier     int32 = 255
)

type MobEffect int32

type MobEffectDefinition struct {
	ID   MobEffect
	Name string
}

func (effect MobEffect) Valid() bool {
	return effect >= 0 && effect <= MaxMobEffectID
}

func (effect MobEffect) Definition() (MobEffectDefinition, bool) {
	if !effect.Valid() {
		return MobEffectDefinition{}, false
	}

	return generatedMobEffectDefinitions[effect], true
}

func MobEffectByName(name string) (MobEffect, bool) {
	for effect, definition := range generatedMobEffectDefinitions {
		if name == definition.Name || name == "minecraft:"+definition.Name {
			return MobEffect(effect), true
		}
	}

	return 0, false
}
