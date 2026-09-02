package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type representativePotion struct {
	name    string
	effects []potionEffectDefinition
}

func TestGeneratedParity(t *testing.T) {
	root := filepath.Join("..", "..")

	manifestPath := filepath.Join(root, "data", "potions.json")
	sourcePath := filepath.Join(root, "..", "reference", "client_source", "net", "minecraft", "world", "item", "alchemy", "Potions.java")

	generated, err := generate(manifestPath, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	committed, err := os.ReadFile(filepath.Join(root, "internal", "game", "potions_generated.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(generated, committed) {
		t.Fatal("potions_generated.go is stale; run go generate ./internal/game")
	}
}

func TestDefinitionsPreserveJavaRegistrationOrder(t *testing.T) {
	source := potionSource(t)

	definitions, err := parseDefinitions(source)
	if err != nil {
		t.Fatal(err)
	}

	if len(definitions) != 46 {
		t.Fatalf("registered potions = %d, want 46", len(definitions))
	}

	if definitions[0].Name != "water" || definitions[23].Name != "long_water_breathing" || definitions[45].Name != "infested" {
		t.Fatalf("registration boundaries = %q, %q, %q", definitions[0].Name, definitions[23].Name, definitions[45].Name)
	}
}

func TestDefinitionsPreserveRepresentativeEffects(t *testing.T) {
	source := potionSource(t)

	definitions, err := parseDefinitions(source)
	if err != nil {
		t.Fatal(err)
	}

	tests := []representativePotion{
		{name: "water", effects: nil},
		{name: "strong_slowness", effects: []potionEffectDefinition{{Name: "slowness", Duration: 400, Amplifier: 3}}},
		{name: "long_turtle_master", effects: []potionEffectDefinition{{Name: "slowness", Duration: 800, Amplifier: 3}, {Name: "resistance", Duration: 800, Amplifier: 2}}},
		{name: "strong_healing", effects: []potionEffectDefinition{{Name: "instant_health", Duration: 1, Amplifier: 1}}},
	}

	for _, test := range tests {
		definition := definitionByName(t, definitions, test.name)

		if len(definition.Effects) != len(test.effects) {
			t.Fatalf("potion %s effects = %+v, want %+v", test.name, definition.Effects, test.effects)
		}

		for index := range test.effects {
			if definition.Effects[index] != test.effects[index] {
				t.Fatalf("potion %s effect %d = %+v, want %+v", test.name, index, definition.Effects[index], test.effects[index])
			}
		}
	}
}

func TestGenerateRejectsStaleSourceHash(t *testing.T) {
	root := filepath.Join("..", "..")

	manifestPath := filepath.Join(root, "data", "potions.json")

	source := potionSource(t)

	source = append(source, '\n')

	sourcePath := filepath.Join(t.TempDir(), "Potions.java")

	err := os.WriteFile(sourcePath, source, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = generate(manifestPath, sourcePath)
	if err == nil || !strings.Contains(err.Error(), "potions.java SHA-256") {
		t.Fatalf("generate stale source error = %v", err)
	}
}

func potionSource(t *testing.T) []byte {
	t.Helper()

	root := filepath.Join("..", "..")
	path := filepath.Join(root, "..", "reference", "client_source", "net", "minecraft", "world", "item", "alchemy", "Potions.java")

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return source
}

func definitionByName(t *testing.T, definitions []potionDefinition, name string) potionDefinition {
	t.Helper()

	for _, definition := range definitions {
		if definition.Name == name {
			return definition
		}
	}

	t.Fatalf("potion %s not found", name)

	return potionDefinition{}
}
