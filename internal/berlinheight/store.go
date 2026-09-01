package berlinheight

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const defaultCacheTiles = 4

type Store struct {
	directory string
	headers   map[TileKey]Header
	cache     map[TileKey]*cachedTile
	failed    map[TileKey]struct{}
	clock     uint64
	cacheSize int
	mx        sync.Mutex
}

type cachedTile struct {
	data     []byte
	lastUsed uint64
}

func Open(directory string) (*Store, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read Berlin height data directory %q: %w", directory, err)
	}

	store := &Store{
		directory: directory,
		headers:   make(map[TileKey]Header),
		cache:     make(map[TileKey]*cachedTile),
		failed:    make(map[TileKey]struct{}),
		cacheSize: defaultCacheTiles,
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		key, valid := ParseTileFilename(entry.Name())
		if !valid {
			continue
		}

		path := filepath.Join(directory, entry.Name())
		header, err := inspectTile(path, key)
		if err != nil {
			return nil, err
		}

		store.headers[key] = header
	}

	if len(store.headers) == 0 {
		return nil, fmt.Errorf("Berlin height data directory %q contains no .mch tiles", directory)
	}

	return store, nil
}

func (store *Store) HeightAt(easting, northing int32) (int32, bool) {
	key := KeyFor(easting, northing)

	if _, exists := store.headers[key]; !exists {
		return 0, false
	}

	tile, loaded := store.load(key)
	if !loaded {
		return 0, false
	}

	index, valid := Index(key, easting, northing)
	if !valid {
		return 0, false
	}

	offset := index * BytesPerCell
	height := binary.LittleEndian.Uint16(tile.data[offset : offset+BytesPerCell])
	if height == NoData {
		return 0, false
	}

	return int32(height), true
}

func (store *Store) TileBounds(key TileKey) (Header, bool) {
	header, exists := store.headers[key]
	return header, exists
}

func (store *Store) AreaMaximum(eastingMin, eastingMax, northingMin, northingMax int32) (int32, bool) {
	if eastingMin > eastingMax {
		eastingMin, eastingMax = eastingMax, eastingMin
	}

	if northingMin > northingMax {
		northingMin, northingMax = northingMax, northingMin
	}

	firstEasting := KeyFor(eastingMin, northingMin).Easting
	lastEasting := KeyFor(eastingMax, northingMin).Easting
	firstNorthing := KeyFor(eastingMin, northingMin).Northing
	lastNorthing := KeyFor(eastingMin, northingMax).Northing

	maximum := int32(-1)
	found := false

	for northing := firstNorthing; northing <= lastNorthing; northing += TileSize {
		for easting := firstEasting; easting <= lastEasting; easting += TileSize {
			header, exists := store.headers[TileKey{Easting: easting, Northing: northing}]
			if !exists || header.ValidCells == 0 {
				continue
			}

			maximum = max(maximum, int32(header.MaximumY))
			found = true
		}
	}

	return maximum, found
}

func (store *Store) TileCount() int {
	return len(store.headers)
}

func (store *Store) load(key TileKey) (*cachedTile, bool) {
	store.mx.Lock()
	defer store.mx.Unlock()

	store.clock++

	if tile, exists := store.cache[key]; exists {
		tile.lastUsed = store.clock
		return tile, true
	}

	if _, failed := store.failed[key]; failed {
		return nil, false
	}

	path := filepath.Join(store.directory, TileFilename(key))
	contents, err := os.ReadFile(path)
	if err != nil {
		store.failed[key] = struct{}{}
		return nil, false
	}

	if len(contents) != TileHeaderSize+TileDataSize {
		store.failed[key] = struct{}{}
		return nil, false
	}

	tile := &cachedTile{
		data:     contents[TileHeaderSize:],
		lastUsed: store.clock,
	}

	if len(store.cache) >= store.cacheSize {
		store.evictOldest()
	}

	store.cache[key] = tile
	return tile, true
}

func (store *Store) evictOldest() {
	var oldestKey TileKey
	var oldestUse uint64
	found := false

	for key, tile := range store.cache {
		if !found || tile.lastUsed < oldestUse {
			oldestKey = key
			oldestUse = tile.lastUsed
			found = true
		}
	}

	if found {
		delete(store.cache, oldestKey)
	}
}

func inspectTile(path string, expectedKey TileKey) (Header, error) {
	file, err := os.Open(path)
	if err != nil {
		return Header{}, fmt.Errorf("open Berlin height tile %q: %w", path, err)
	}
	defer file.Close()

	header, err := ReadHeader(file)
	if err != nil {
		return Header{}, fmt.Errorf("read Berlin height tile %q: %w", path, err)
	}

	if header.Key != expectedKey {
		return Header{}, fmt.Errorf("Berlin height tile %q header origin is %d,%d, filename says %d,%d", path, header.Key.Easting, header.Key.Northing, expectedKey.Easting, expectedKey.Northing)
	}

	info, err := file.Stat()
	if err != nil {
		return Header{}, fmt.Errorf("stat Berlin height tile %q: %w", path, err)
	}

	expectedSize := int64(TileHeaderSize + TileDataSize)
	if info.Size() != expectedSize {
		return Header{}, fmt.Errorf("Berlin height tile %q is %d bytes, want %d", path, info.Size(), expectedSize)
	}

	return header, nil
}
