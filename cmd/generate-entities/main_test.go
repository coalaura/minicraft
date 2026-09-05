package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedParity(t *testing.T) {
	root := filepath.Join("..", "..")
	inputPath := filepath.Join(root, "data", "entities.json")

	definitions, err := readDefinitions(inputPath)
	if err != nil {
		t.Fatal(err)
	}

	generated, err := generate(definitions)
	if err != nil {
		t.Fatal(err)
	}

	committed, err := os.ReadFile(filepath.Join(root, "internal", "game", "entities_generated.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(generated, committed) {
		t.Fatal("entities_generated.go is stale; run go generate ./internal/game")
	}
}

func TestCurrentCatalogue(t *testing.T) {
	root := filepath.Join("..", "..")
	inputPath := filepath.Join(root, "data", "entities.json")

	definitions, err := readDefinitions(inputPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(definitions) != expectedEntityCount {
		t.Fatalf("registered entities = %d, want %d", len(definitions), expectedEntityCount)
	}

	if definitions[71].Name != "item" || definitions[150].Name != "zombie" || definitions[155].Name != "player" {
		t.Fatalf("selected entity names = %q, %q, %q", definitions[71].Name, definitions[150].Name, definitions[155].Name)
	}
}
