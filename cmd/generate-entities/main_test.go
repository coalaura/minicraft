package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type invalidClassificationTest struct {
	name     string
	index    int
	mutate   func(*entityDefinition)
	wantText string
}

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

	if definitions[71].Name != "item" || definitions[71].Kind != "other" || definitions[71].Category != "UNKNOWN" || definitions[150].Name != "zombie" || definitions[150].Kind != "hostile" || definitions[150].Category != "Hostile mobs" || definitions[155].Name != "player" || definitions[155].Kind != "player" || definitions[155].Category != "UNKNOWN" {
		t.Fatalf("selected entity classifications = %+v, %+v, %+v", definitions[71], definitions[150], definitions[155])
	}
}

func TestValidateRejectsInvalidClassifications(t *testing.T) {
	root := filepath.Join("..", "..")
	inputPath := filepath.Join(root, "data", "entities.json")

	definitions, err := readDefinitions(inputPath)
	if err != nil {
		t.Fatal(err)
	}

	tests := []invalidClassificationTest{
		{
			name:  "unsupported type",
			index: 0,
			mutate: func(definition *entityDefinition) {
				definition.Kind = "unknown"
			},
			wantText: `unsupported type "unknown"`,
		},
		{
			name:  "unsupported category",
			index: 0,
			mutate: func(definition *entityDefinition) {
				definition.Category = "unknown"
			},
			wantText: `unsupported category "unknown"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalidDefinitions := append([]entityDefinition(nil), definitions...)

			test.mutate(&invalidDefinitions[test.index])

			err := validate(invalidDefinitions)
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("validate error = %v, want containing %q", err, test.wantText)
			}
		})
	}
}
