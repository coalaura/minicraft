package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceHashAndGeneratedParity(t *testing.T) {
	root := filepath.Join("..", "..")

	manifestPath := filepath.Join(root, "data", "mob_effects.json")
	sourcePath := filepath.Join(root, "..", "reference", "client_source", "net", "minecraft", "world", "effect", "MobEffects.java")

	generated, err := generate(manifestPath, sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	committed, err := os.ReadFile(filepath.Join(root, "internal", "game", "mob_effects_generated.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(generated, committed) {
		t.Fatal("mob_effects_generated.go is stale; run go generate ./internal/game")
	}
}

func TestDefinitionsPreserveJavaRegistrationOrder(t *testing.T) {
	root := filepath.Join("..", "..")

	sourcePath := filepath.Join(root, "..", "reference", "client_source", "net", "minecraft", "world", "effect", "MobEffects.java")

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}

	definitions := parseDefinitions(source)
	if len(definitions) != 40 {
		t.Fatalf("registered effects = %d, want 40", len(definitions))
	}

	if definitions[0].Name != "speed" || definitions[38].Name != "infested" || definitions[39].Name != "breath_of_the_nautilus" {
		t.Fatalf("registration boundaries = %q, %q, %q", definitions[0].Name, definitions[38].Name, definitions[39].Name)
	}
}
