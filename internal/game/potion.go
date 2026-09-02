//go:generate go run ../../cmd/generate-potions -manifest ../../data/potions.json -source ../../../reference/client_source/net/minecraft/world/item/alchemy/Potions.java -output potions_generated.go

package game

type Potion int32

type PotionDefinition struct {
	Name    string
	Effects []MobEffectInstance
}

func (potion Potion) Valid() bool {
	return potion >= 0 && int64(potion) < int64(len(generatedPotionDefinitions))
}

func (potion Potion) Definition() (PotionDefinition, bool) {
	if !potion.Valid() {
		return PotionDefinition{}, false
	}

	definition := generatedPotionDefinitions[potion]
	definition.Effects = cloneMobEffectInstances(definition.Effects)

	return definition, true
}

func cloneMobEffectInstances(instances []MobEffectInstance) []MobEffectInstance {
	clones := make([]MobEffectInstance, len(instances))

	for index := range instances {
		clones[index] = instances[index].Clone()
	}

	return clones
}

func PotionByName(name string) (Potion, bool) {
	for potion, definition := range generatedPotionDefinitions {
		if name == definition.Name || name == "minecraft:"+definition.Name {
			return Potion(potion), true
		}
	}

	return 0, false
}
