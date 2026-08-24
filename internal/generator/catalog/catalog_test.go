package catalog

import (
	"testing"

	"github.com/coalaura/minicraft/internal/generator"
)

func TestCatalogIncludesWaveTerrain(t *testing.T) {
	generated, err := generator.New("wave-terrain")
	if err != nil {
		t.Fatalf("create wave terrain from catalog: %v", err)
	}

	if generated == nil {
		t.Fatal("catalog returned a nil wave terrain generator")
	}
}
