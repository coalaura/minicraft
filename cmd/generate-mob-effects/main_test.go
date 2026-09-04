package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedParity(t *testing.T) {
	root := filepath.Join("..", "..")

	manifestPath := filepath.Join(root, "data", "mob_effects.json")

	generated, err := generate(manifestPath)
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

func TestDefinitionsPreserveRegistrationOrder(t *testing.T) {
	root := filepath.Join("..", "..")

	manifestPath := filepath.Join(root, "data", "mob_effects.json")

	manifest, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(manifest.Effects) != 40 {
		t.Fatalf("registered effects = %d, want 40", len(manifest.Effects))
	}

	if manifest.Effects[0] != "speed" || manifest.Effects[38] != "infested" || manifest.Effects[39] != "breath_of_the_nautilus" {
		t.Fatalf("registration boundaries = %q, %q, %q", manifest.Effects[0], manifest.Effects[38], manifest.Effects[39])
	}
}
