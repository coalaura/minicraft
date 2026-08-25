package game

import "testing"

type biomeCatalogueTestCase struct {
	biome Biome
	id    int
	name  string
}

func TestBiomeCatalogue(t *testing.T) {
	tests := []biomeCatalogueTestCase{
		{BiomeBadlands, 0, "badlands"},
		{BiomePlains, 40, "plains"},
		{BiomeWoodedBadlands, 64, "wooded_badlands"},
	}

	if BiomeCount != 65 {
		t.Fatalf("BiomeCount = %d, want 65", BiomeCount)
	}

	if len(BiomeNames) != int(BiomeCount) {
		t.Fatalf("len(BiomeNames) = %d, want %d", len(BiomeNames), BiomeCount)
	}

	seen := make(map[string]struct{}, len(BiomeNames))

	for id, name := range BiomeNames {
		if !Biome(id).Valid() {
			t.Fatalf("biome %d is not valid", id)
		}

		if name == "" {
			t.Fatalf("biome %d has an empty name", id)
		}

		if _, exists := seen[name]; exists {
			t.Fatalf("biome %d duplicates name %q", id, name)
		}

		seen[name] = struct{}{}
	}

	if BiomeCount.Valid() {
		t.Fatalf("BiomeCount %d is valid", BiomeCount)
	}

	for _, test := range tests {
		if int(test.biome) != test.id {
			t.Errorf("%s ID = %d, want %d", test.name, test.biome, test.id)
		}

		if BiomeNames[test.biome] != test.name {
			t.Errorf("BiomeNames[%d] = %q, want %q", test.biome, BiomeNames[test.biome], test.name)
		}
	}
}
