package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testInputPaths() inputPaths {
	root := filepath.Join("..", "..")

	return inputPaths{
		OrderPath:           filepath.Join(root, "data", "enchantment_order.json"),
		EnchantmentsPath:    filepath.Join(root, "data", "enchantments"),
		EnchantmentTagsPath: filepath.Join(root, "data", "enchantment_tags"),
		ItemTagsPath:        filepath.Join(root, "data", "item_tags"),
		ItemsPath:           filepath.Join(root, "data", "items.json"),
	}
}

func TestOrderAndCatalogue(t *testing.T) {
	paths := testInputPaths()

	order, err := readOrder(paths.OrderPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(order) != 43 {
		t.Fatalf("bootstrap order length = %d, want 43", len(order))
	}

	enchantments, err := readEnchantments(paths.EnchantmentsPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(enchantments) != len(order) {
		t.Fatalf("enchantment catalogue length = %d, want %d", len(enchantments), len(order))
	}

	seen := make(map[string]bool, len(order))

	for id, name := range order {
		if seen[name] {
			t.Fatalf("duplicate bootstrap entry %q", name)
		}

		seen[name] = true
		_, exists := enchantments[name]

		if !exists {
			t.Fatalf("bootstrap ID %d references missing enchantment %q", id, name)
		}
	}

	for name := range enchantments {
		if !seen[name] {
			t.Fatalf("catalogue enchantment %q is absent from bootstrap order", name)
		}
	}

	if order[20] != "efficiency" || order[21] != "silk_touch" || order[22] != "unbreaking" || order[23] != "fortune" {
		t.Fatalf("bootstrap IDs 20-23 = %q, %q, %q, %q", order[20], order[21], order[22], order[23])
	}
}

func TestTagExpansionAndTooltipOrder(t *testing.T) {
	paths := testInputPaths()

	items := tagResolver{root: paths.ItemTagsPath}

	mining, err := items.expand("enchantable/mining")
	if err != nil {
		t.Fatal(err)
	}

	if len(mining) == 0 || mining[0] != "diamond_axe" || mining[len(mining)-1] != "shears" {
		t.Fatalf("unexpected recursive mining expansion: %q", mining)
	}

	enchantments := tagResolver{root: paths.EnchantmentTagsPath}

	tooltip, err := enchantments.expand("tooltip_order")
	if err != nil {
		t.Fatal(err)
	}

	if len(tooltip) != 43 {
		t.Fatalf("tooltip_order length = %d, want 43", len(tooltip))
	}

	if tooltip[0] != "binding_curse" || tooltip[1] != "vanishing_curse" || tooltip[len(tooltip)-1] != "mending" {
		t.Fatalf("unexpected tooltip order boundaries: %q", tooltip)
	}

	curse, err := enchantments.expand("curse")
	if err != nil {
		t.Fatal(err)
	}

	wantCurses := []string{"binding_curse", "vanishing_curse"}
	if !reflect.DeepEqual(curse, wantCurses) {
		t.Fatalf("curse tag = %q, want %q", curse, wantCurses)
	}
}

func TestGeneratedParity(t *testing.T) {
	paths := testInputPaths()

	game, protocol, err := generate(paths)
	if err != nil {
		t.Fatal(err)
	}

	root := filepath.Join("..", "..")

	wantGame, err := os.ReadFile(filepath.Join(root, "internal", "game", "enchantments_generated.go"))
	if err != nil {
		t.Fatal(err)
	}

	wantProtocol, err := os.ReadFile(filepath.Join(root, "internal", "protocol", "enchantment_tags_generated.go"))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(game, wantGame) {
		t.Fatal("game generated output is stale; run cmd/generate-enchantments")
	}

	if !bytes.Equal(protocol, wantProtocol) {
		t.Fatal("protocol generated output is stale; run cmd/generate-enchantments")
	}

	if !strings.Contains(string(game), "const MaxEnchantmentID Enchantment = 42") {
		t.Fatal("game output does not declare MaxEnchantmentID")
	}
}
