package game

//go:generate go run ../../cmd/generate-entities -input ../../data/entities.json -output entities_generated.go

const (
	EntityKindAmbient    = "ambient"
	EntityKindAnimal     = "animal"
	EntityKindHostile    = "hostile"
	EntityKindLiving     = "living"
	EntityKindMob        = "mob"
	EntityKindOther      = "other"
	EntityKindPassive    = "passive"
	EntityKindPlayer     = "player"
	EntityKindProjectile = "projectile"

	EntityCategoryHostileMobs = "Hostile mobs"
	EntityCategoryImmobile    = "Immobile"
	EntityCategoryPassiveMobs = "Passive mobs"
	EntityCategoryProjectiles = "Projectiles"
	EntityCategoryUnknown     = "UNKNOWN"
	EntityCategoryVehicles    = "Vehicles"
)

type EntityType int32

type EntityKind string

type EntityCategory string

type EntityDefinition struct {
	ID       EntityType
	Name     string
	Kind     EntityKind
	Category EntityCategory
	Width    float64
	Height   float64
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

func (entityType EntityType) NotScaryForPufferfish() bool {
	switch entityType {
	case EntityTurtle,
		EntityGuardian,
		EntityElderGuardian,
		EntityCod,
		EntityPufferfish,
		EntitySalmon,
		EntityTropicalFish,
		EntityDolphin,
		EntitySquid,
		EntityGlowSquid,
		EntityTadpole,
		EntityNautilus,
		EntityZombieNautilus:
		return true
	default:
		return false
	}
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
