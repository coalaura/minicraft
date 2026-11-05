package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEquipmentManifestsMatchPinnedSourceInputs(t *testing.T) {
	dataPath := filepath.Join("..", "..", "data")

	equipment, armor, err := equipmentManifests(dataPath)
	if err != nil {
		t.Fatalf("derive equipment manifests: %v", err)
	}

	assertCurrentManifest(t, filepath.Join("..", "..", "data", "item_equippables.json"), equipment)
	assertCurrentManifest(t, filepath.Join("..", "..", "data", "item_armor_attributes.json"), armor)
}

func TestRegenerateManifestsWithoutReference(t *testing.T) {
	dataPath := filepath.Join("..", "..", "data")

	standalonePath := t.TempDir()

	inputs := []string{"equipment_source.json", "enchantment_order.json"}

	for _, name := range inputs {
		contents, err := os.ReadFile(filepath.Join(dataPath, name))
		if err != nil {
			t.Fatal(err)
		}

		err = os.WriteFile(filepath.Join(standalonePath, name), contents, 0o644)
		if err != nil {
			t.Fatal(err)
		}
	}

	directories := []string{"enchantments", "item_tags"}

	for _, name := range directories {
		err := os.CopyFS(filepath.Join(standalonePath, name), os.DirFS(filepath.Join(dataPath, name)))
		if err != nil {
			t.Fatal(err)
		}
	}

	err := regenerateManifests(standalonePath)
	if err != nil {
		t.Fatalf("regenerate with only committed JSON inputs: %v", err)
	}

	equipment, armor, err := equipmentManifests(dataPath)
	if err != nil {
		t.Fatal(err)
	}

	assertCurrentManifest(t, filepath.Join(standalonePath, "item_equippables.json"), equipment)
	assertCurrentManifest(t, filepath.Join(standalonePath, "item_armor_attributes.json"), armor)
}

func assertCurrentManifest(t *testing.T, path string, value any) {
	t.Helper()

	want, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}

	want = append(want, '\n')

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("%s is stale; run go run ./cmd/sync-data", path)
	}
}
