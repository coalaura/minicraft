package main

import (
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/berlinvoxel"
)

func TestPrepareOBJPreservesSurfaceAndOpenSpace(t *testing.T) {
	directory := t.TempDir()
	tiles := newVoxelTiles(directory)

	object := `v 389918 5819699 40
v 389922 5819699 40
v 389922 5819695 40
v 389918 5819695 40
v 389918 5819699 50
v 389918 5819695 50
f 1 2 3 4
f 1 4 6 5
`

	err := prepareOBJ(strings.NewReader(object), tiles)
	if err != nil {
		t.Fatalf("prepare OBJ: %v", err)
	}

	_, err = tiles.Flush()
	if err != nil {
		t.Fatalf("flush OBJ tiles: %v", err)
	}

	store, err := berlinvoxel.Open(directory)
	if err != nil {
		t.Fatalf("open prepared OBJ store: %v", err)
	}

	marked, spans, valid := store.ColumnAt(389920, 5819697)
	if !valid || !marked {
		t.Fatalf("mesh coverage = valid %v marked %v, want true true", valid, marked)
	}

	if !spansContainRaw(spans, 40) {
		t.Fatalf("mesh ground spans = %+v, want height 40", spans)
	}

	if spansContainRaw(spans, 45) {
		t.Fatalf("mesh interior spans = %+v, unexpectedly filled height 45", spans)
	}

	_, wallSpans, valid := store.ColumnAt(389918, 5819697)
	if !valid || !spansContainRaw(wallSpans, 45) {
		t.Fatalf("mesh wall spans = %+v valid=%v, want height 45", wallSpans, valid)
	}
}

func TestDecodeMeshIndexAcceptsUTF8BOM(t *testing.T) {
	contents := []byte{0xef, 0xbb, 0xbf}
	contents = append(contents, []byte(`{"type":"FeatureCollection","features":[{"properties":{"url":"one.zip"},"geometry":{"type":"Polygon","coordinates":[]}}]}`)...)

	index, err := decodeMeshIndex(contents)
	if err != nil {
		t.Fatalf("decode mesh index with UTF-8 BOM: %v", err)
	}

	if len(index.Features) != 1 || index.Features[0].Properties.URL != "one.zip" {
		t.Fatalf("decoded mesh index = %+v, want one.zip", index)
	}
}

func TestSelectMeshFeaturesSupportsExactTiles(t *testing.T) {
	index := meshIndex{
		Type: "FeatureCollection",
		Features: []meshFeature{
			{Properties: meshProperties{URL: "one.zip"}},
			{Properties: meshProperties{URL: "two.zip"}},
		},
	}

	selected, err := selectMeshFeatures(index, meshSelection{
		Mode:       "none",
		ExactTiles: map[string]struct{}{"two.zip": {}},
	})
	if err != nil {
		t.Fatalf("select exact mesh tile: %v", err)
	}

	if len(selected) != 1 || selected[0].Properties.URL != "two.zip" {
		t.Fatalf("selected mesh tiles = %+v, want only two.zip", selected)
	}
}

func TestPrepareOBJIgnoresGeometryEntirelyBelowMinecraftHeight(t *testing.T) {
	directory := t.TempDir()
	tiles := newVoxelTiles(directory)

	object := `v 389918 5819699 -30
v 389922 5819699 -30
v 389922 5819695 -30
f 1 2 3
`

	err := prepareOBJ(strings.NewReader(object), tiles)
	if err != nil {
		t.Fatalf("prepare below-world OBJ: %v", err)
	}

	files, err := tiles.Flush()
	if err != nil {
		t.Fatalf("flush below-world OBJ: %v", err)
	}

	if len(files) != 0 {
		t.Fatalf("below-world OBJ created %d voxel tile(s), want none", len(files))
	}
}
