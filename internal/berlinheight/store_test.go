package berlinheight

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTripPreservesTallHeights(t *testing.T) {
	directory := t.TempDir()
	key := TileKey{Easting: 388000, Northing: 5818000}
	heights := make([]uint16, TileCellCount)

	for index := range heights {
		heights[index] = NoData
	}

	index, valid := Index(key, 389918, 5819699)
	if !valid {
		t.Fatal("test coordinate lies outside tile")
	}

	heights[index] = 400

	header := Header{
		Key:        key,
		MinimumY:   400,
		MaximumY:   400,
		ValidCells: 1,
	}

	path := filepath.Join(directory, TileFilename(key))
	err := WriteTile(path, header, heights)
	if err != nil {
		t.Fatalf("write tile: %v", err)
	}

	store, err := Open(directory)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	height, valid := store.HeightAt(389918, 5819699)
	if !valid {
		t.Fatal("stored coordinate has no height")
	}

	if height != 400 {
		t.Fatalf("height = %d, want 400", height)
	}
}

func TestStoreRejectsNoDataCells(t *testing.T) {
	directory := t.TempDir()
	key := TileKey{Easting: 388000, Northing: 5818000}
	heights := make([]uint16, TileCellCount)

	for index := range heights {
		heights[index] = NoData
	}

	heights[0] = 35

	header := Header{
		Key:        key,
		MinimumY:   35,
		MaximumY:   35,
		ValidCells: 1,
	}

	path := filepath.Join(directory, TileFilename(key))
	err := WriteTile(path, header, heights)
	if err != nil {
		t.Fatalf("write tile: %v", err)
	}

	store, err := Open(directory)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	_, valid := store.HeightAt(388001, 5818000)
	if valid {
		t.Fatal("no-data cell returned a height")
	}
}
