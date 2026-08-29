package catalog

import (
	"testing"

	"github.com/coalaura/minicraft/internal/generator"
)

func TestCatalogIncludesGenerators(t *testing.T) {
	names := []string{"superflat", "test-world", "wave-terrain"}

	for _, name := range names {
		generated, err := generator.New(name)
		if err != nil {
			t.Fatalf("create %s from catalog: %v", name, err)
		}

		if generated == nil {
			t.Fatalf("catalog returned a nil %s generator", name)
		}
	}
}
