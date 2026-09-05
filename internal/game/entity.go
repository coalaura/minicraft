package game

//go:generate go run ../../cmd/generate-entities -input ../../data/entities.json -output entities_generated.go

type EntityType int32

type EntityDefinition struct {
	ID     EntityType
	Name   string
	Width  float64
	Height float64
}

func (entityType EntityType) Valid() bool {
	return entityType >= 0 && entityType <= MaxEntityType
}

func (entityType EntityType) Definition() (EntityDefinition, bool) {
	if !entityType.Valid() {
		return EntityDefinition{}, false
	}

	return entityDefinitions[entityType], true
}

func EntityByName(name string) (EntityType, bool) {
	name, ok := generatedName(name)
	if !ok {
		return 0, false
	}

	for entityType, definition := range entityDefinitions {
		if definition.Name == name {
			return EntityType(entityType), true
		}
	}

	return 0, false
}
