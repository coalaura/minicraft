package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/coalaura/minicraft/internal/berlinheight"
	"github.com/coalaura/minicraft/internal/berlinvoxel"
)

const (
	minimumMinecraftY  = -64
	maximumMinecraftY  = 319
	berlinHeightOffset = 90

	minimumPreparedHeight = minimumMinecraftY + berlinHeightOffset
	maximumPreparedHeight = maximumMinecraftY + berlinHeightOffset
)

type voxelTiles struct {
	directory string
	builders  map[berlinheight.TileKey]*berlinvoxel.Builder
}

func newVoxelTiles(directory string) *voxelTiles {
	return &voxelTiles{
		directory: directory,
		builders:  make(map[berlinheight.TileKey]*berlinvoxel.Builder),
	}
}

func (tiles *voxelTiles) Mark(easting, northing int32) error {
	builder, err := tiles.builder(easting, northing)
	if err != nil {
		return err
	}

	if !builder.Mark(easting, northing) {
		return fmt.Errorf("mark Berlin voxel outside selected tile at %d,%d", easting, northing)
	}

	return nil
}

func (tiles *voxelTiles) AddBlock(easting, northing int32, height int) (bool, error) {
	if height < minimumPreparedHeight || height > maximumPreparedHeight {
		return false, nil
	}

	builder, err := tiles.builder(easting, northing)
	if err != nil {
		return false, err
	}

	if !builder.AddBlock(easting, northing, height) {
		return false, fmt.Errorf("add Berlin voxel outside selected tile at %d,%d,%d", easting, northing, height)
	}

	return true, nil
}

func preparedHeightRangeIntersects(minimum, maximum float64) bool {
	if minimum > maximum {
		minimum, maximum = maximum, minimum
	}

	return maximum >= minimumPreparedHeight && minimum <= maximumPreparedHeight
}

func (tiles *voxelTiles) Flush() ([]string, error) {
	keys := make([]berlinheight.TileKey, 0, len(tiles.builders))
	for key := range tiles.builders {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(left, right int) bool {
		if keys[left].Northing == keys[right].Northing {
			return keys[left].Easting < keys[right].Easting
		}

		return keys[left].Northing < keys[right].Northing
	})

	files := make([]string, 0, len(keys))
	for _, key := range keys {
		builder := tiles.builders[key]
		if builder.Empty() {
			continue
		}

		filename, err := builder.Write(tiles.directory)
		if err != nil {
			return nil, err
		}

		files = append(files, filename)
	}

	return files, nil
}

func (tiles *voxelTiles) builder(easting, northing int32) (*berlinvoxel.Builder, error) {
	key := berlinheight.KeyFor(easting, northing)
	builder, exists := tiles.builders[key]
	if exists {
		return builder, nil
	}

	builder = berlinvoxel.NewBuilder(key)
	path := filepath.Join(tiles.directory, berlinvoxel.TileFilename(key))

	err := builder.MergeFile(path)
	if err != nil {
		return nil, err
	}

	tiles.builders[key] = builder
	return builder, nil
}

func voxelTileFiles(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		_, valid := berlinvoxel.ParseTileFilename(entry.Name())
		if valid {
			files = append(files, entry.Name())
		}
	}

	sort.Strings(files)
	return files, nil
}
