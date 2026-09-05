package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStripEnchantCategories(t *testing.T) {
	input := []byte("[\n  {\n    \"id\": 1,\n    \"enchantCategories\": [\n      \"mining\",\n      \"durability\"\n    ],\n    \"maxDurability\": 10\n  },\n  {\n    \"id\": 2,\n    \"enchantCategories\": [\n      \"vanishing\"\n    ]\n  }\n]\n")
	want := "[\n  {\n    \"id\": 1,\n    \"maxDurability\": 10\n  },\n  {\n    \"id\": 2\n  }\n]\n"

	got, count := stripEnchantCategories(input)
	if count != 2 {
		t.Fatalf("removed fields = %d, want 2", count)
	}

	if string(got) != want {
		t.Fatalf("cleaned items:\n%s\nwant:\n%s", got, want)
	}
}

func TestEnchantmentOrder(t *testing.T) {
	input := []byte(`
public static final ResourceKey<Enchantment> FIRST = key("first");
public static final ResourceKey<Enchantment> SECOND = key("second");
public static void bootstrap(final BootstrapContext<Enchantment> context) {
   register(
      context,
      SECOND,
      builder()
   );
   register(context, FIRST, builder());
}
private static void register() {}
`)

	_, err := enchantmentOrder(input)
	if err == nil {
		t.Fatal("short bootstrap order was accepted")
	}

	repeated := make([]byte, 0, len(input)*22)

	repeated = append(repeated, []byte("public static final ResourceKey<Enchantment> ENTRY = key(\"entry\");\npublic static void bootstrap() {\n")...)

	for range 43 {
		repeated = append(repeated, []byte("register(context, ENTRY, builder());\n")...)
	}

	repeated = append(repeated, []byte("}\nprivate static void register() {}")...)

	got, err := enchantmentOrder(repeated)
	if err != nil {
		t.Fatal(err)
	}

	want := make([]string, 43)

	for index := range want {
		want[index] = "entry"
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

func TestEquipmentManifestsMatchPinnedClientSource(t *testing.T) {
	clientPath := filepath.Join("..", "..", "..", "reference", "client_source")

	_, err := os.Stat(clientPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("pinned client source is not available")
		}

		t.Fatalf("inspect pinned client source: %v", err)
	}

	equipment, armor, err := equipmentManifests(clientPath)
	if err != nil {
		t.Fatalf("derive equipment manifests: %v", err)
	}

	assertCurrentManifest(t, filepath.Join("..", "..", "data", "item_equippables.json"), equipment)
	assertCurrentManifest(t, filepath.Join("..", "..", "data", "item_armor_attributes.json"), armor)
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
		t.Fatalf("%s is stale; run go run ./cmd/sync-data -reference ../reference", path)
	}
}
