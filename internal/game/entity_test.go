package game

import "testing"

type entityRegistryTest struct {
	name       string
	entityType EntityType
	width      float64
	height     float64
}

func TestEntityRegistry(t *testing.T) {
	tests := []entityRegistryTest{
		{name: "item", entityType: EntityItem, width: 0.25, height: 0.25},
		{name: "zombie", entityType: EntityZombie, width: 0.6, height: 1.95},
		{name: "player", entityType: EntityPlayer, width: 0.6, height: 1.8},
	}

	if EntityItem != EntityType(71) || EntityZombie != EntityType(150) || EntityPlayer != EntityType(155) {
		t.Fatalf("selected entity types = %d, %d, %d", EntityItem, EntityZombie, EntityPlayer)
	}

	for _, test := range tests {
		definition, valid := test.entityType.Definition()
		if !valid {
			t.Fatalf("entity %s is invalid", test.name)
		}

		if definition.ID != test.entityType || definition.Name != test.name || definition.Width != test.width || definition.Height != test.height {
			t.Fatalf("entity %s definition = %+v", test.name, definition)
		}
	}

	entityType, found := EntityByName("minecraft:player")
	if !found || entityType != EntityPlayer {
		t.Fatalf("minecraft:player = %d, found %t", entityType, found)
	}
}
