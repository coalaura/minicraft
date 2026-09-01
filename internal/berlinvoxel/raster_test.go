package berlinvoxel

import "testing"

func TestRasterizeHorizontalTriangle(t *testing.T) {
	blocks := make(map[[3]int32]struct{})

	RasterizeTriangle(
		Point{X: 0, Y: 0, Z: 42},
		Point{X: 3, Y: 0, Z: 42},
		Point{X: 0, Y: 3, Z: 42},
		func(easting, northing int32, height int) {
			blocks[[3]int32{easting, northing, int32(height)}] = struct{}{}
		},
	)

	for _, coordinate := range [][3]int32{{0, 0, 42}, {1, 1, 42}, {2, 0, 42}} {
		if _, exists := blocks[coordinate]; !exists {
			t.Fatalf("missing rasterized horizontal block %v", coordinate)
		}
	}
}

func TestRasterizeVerticalTriangle(t *testing.T) {
	blocks := make(map[[3]int32]struct{})

	RasterizeTriangle(
		Point{X: 10, Y: 20, Z: 30},
		Point{X: 10, Y: 24, Z: 30},
		Point{X: 10, Y: 20, Z: 35},
		func(easting, northing int32, height int) {
			blocks[[3]int32{easting, northing, int32(height)}] = struct{}{}
		},
	)

	for _, coordinate := range [][3]int32{{10, 20, 30}, {10, 21, 31}, {10, 20, 34}} {
		if _, exists := blocks[coordinate]; !exists {
			t.Fatalf("missing rasterized vertical block %v", coordinate)
		}
	}
}
