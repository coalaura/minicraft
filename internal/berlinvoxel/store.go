package berlinvoxel

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/coalaura/minicraft/internal/berlinheight"
)

const defaultCacheTiles = 3

type Store struct {
	directory string
	headers   map[berlinheight.TileKey]Header
	cache     map[berlinheight.TileKey]*cachedTile
	failed    map[berlinheight.TileKey]struct{}
	clock     uint64
	cacheSize int
	mx        sync.Mutex
}

type cachedTile struct {
	contents []byte
	marked   []byte
	columns  []byte
	spans    []byte
	lastUsed uint64
	header   Header
}

func Open(directory string) (*Store, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read Berlin voxel data directory %q: %w", directory, err)
	}

	store := &Store{
		directory: directory,
		headers:   make(map[berlinheight.TileKey]Header),
		cache:     make(map[berlinheight.TileKey]*cachedTile),
		failed:    make(map[berlinheight.TileKey]struct{}),
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
		return nil, fmt.Errorf("Berlin voxel data directory %q contains no .mcv tiles", directory)
	}

	return store, nil
}

func (store *Store) ColumnAt(easting, northing int32) (bool, []Span, bool) {
	key := berlinheight.KeyFor(easting, northing)
	if _, exists := store.headers[key]; !exists {
		return false, nil, false
	}

	tile, loaded := store.load(key)
	if !loaded {
		return false, nil, false
	}

	index, valid := berlinheight.Index(key, easting, northing)
	if !valid {
		return false, nil, false
	}

	marked := maskContains(tile.marked, index)
	columnIndex := tile.findColumn(uint32(index))
	if columnIndex < 0 {
		return marked, nil, true
	}

	firstSpan := tile.columnFirstSpan(columnIndex)
	lastSpan := tile.header.SpanCount

	if columnIndex+1 < int(tile.header.ColumnCount) {
		lastSpan = tile.columnFirstSpan(columnIndex + 1)
	}

	count := int(lastSpan - firstSpan)
	spans := make([]Span, count)

	for spanIndex := range count {
		offset := int(firstSpan+uint32(spanIndex)) * SpanSize
		spans[spanIndex] = Span{
			MinimumY: binary.LittleEndian.Uint16(tile.spans[offset : offset+2]),
			MaximumY: binary.LittleEndian.Uint16(tile.spans[offset+2 : offset+4]),
		}
	}

	return marked, spans, true
}

func (store *Store) BlockAt(easting, northing int32, height uint16) (bool, bool) {
	_, spans, valid := store.ColumnAt(easting, northing)
	if !valid {
		return false, false
	}

	for _, span := range spans {
		if height >= span.MinimumY && height <= span.MaximumY {
			return true, true
		}
	}

	return false, true
}

func (store *Store) AreaMaximum(eastingMin, eastingMax, northingMin, northingMax int32) (int32, bool) {
	if eastingMin > eastingMax {
		eastingMin, eastingMax = eastingMax, eastingMin
	}

	if northingMin > northingMax {
		northingMin, northingMax = northingMax, northingMin
	}

	firstEasting := berlinheight.KeyFor(eastingMin, northingMin).Easting
	lastEasting := berlinheight.KeyFor(eastingMax, northingMin).Easting
	firstNorthing := berlinheight.KeyFor(eastingMin, northingMin).Northing
	lastNorthing := berlinheight.KeyFor(eastingMin, northingMax).Northing

	maximum := int32(-1)
	found := false

	for northing := firstNorthing; northing <= lastNorthing; northing += berlinheight.TileSize {
		for easting := firstEasting; easting <= lastEasting; easting += berlinheight.TileSize {
			header, exists := store.headers[berlinheight.TileKey{Easting: easting, Northing: northing}]
			if !exists || header.SpanCount == 0 {
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

func (store *Store) TileKeys() []berlinheight.TileKey {
	keys := make([]berlinheight.TileKey, 0, len(store.headers))
	for key := range store.headers {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(left, right int) bool {
		if keys[left].Northing == keys[right].Northing {
			return keys[left].Easting < keys[right].Easting
		}

		return keys[left].Northing < keys[right].Northing
	})

	return keys
}

func (store *Store) load(key berlinheight.TileKey) (*cachedTile, bool) {
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

	header := store.headers[key]
	columnsStart := TileHeaderSize + MaskSize
	spansStart := columnsStart + int(header.ColumnCount)*ColumnSize
	expectedSize := spansStart + int(header.SpanCount)*SpanSize

	if len(contents) != expectedSize {
		store.failed[key] = struct{}{}
		return nil, false
	}

	tile := &cachedTile{
		contents: contents,
		marked:   contents[TileHeaderSize:columnsStart],
		columns:  contents[columnsStart:spansStart],
		spans:    contents[spansStart:],
		lastUsed: store.clock,
		header:   header,
	}

	if len(store.cache) >= store.cacheSize {
		store.evictOldest()
	}

	store.cache[key] = tile
	return tile, true
}

func (store *Store) evictOldest() {
	var oldestKey berlinheight.TileKey
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

func (tile *cachedTile) findColumn(index uint32) int {
	left := 0
	right := int(tile.header.ColumnCount)

	for left < right {
		middle := left + (right-left)/2
		candidate := tile.columnCellIndex(middle)

		if candidate < index {
			left = middle + 1
			continue
		}

		right = middle
	}

	if left >= int(tile.header.ColumnCount) || tile.columnCellIndex(left) != index {
		return -1
	}

	return left
}

func (tile *cachedTile) columnCellIndex(index int) uint32 {
	offset := index * ColumnSize
	return binary.LittleEndian.Uint32(tile.columns[offset : offset+4])
}

func (tile *cachedTile) columnFirstSpan(index int) uint32 {
	offset := index * ColumnSize
	return binary.LittleEndian.Uint32(tile.columns[offset+4 : offset+8])
}

func maskContains(mask []byte, index int) bool {
	return mask[index>>3]&(1<<byte(index&7)) != 0
}

func inspectTile(path string, expectedKey berlinheight.TileKey) (Header, error) {
	file, err := os.Open(path)
	if err != nil {
		return Header{}, fmt.Errorf("open Berlin voxel tile %q: %w", path, err)
	}
	defer file.Close()

	header, err := ReadHeader(file)
	if err != nil {
		return Header{}, fmt.Errorf("read Berlin voxel tile %q: %w", path, err)
	}

	if header.Key != expectedKey {
		return Header{}, fmt.Errorf("Berlin voxel tile %q header origin %d,%d does not match filename", path, header.Key.Easting, header.Key.Northing)
	}

	stat, err := file.Stat()
	if err != nil {
		return Header{}, fmt.Errorf("stat Berlin voxel tile %q: %w", path, err)
	}

	expectedSize := int64(TileHeaderSize + MaskSize + int(header.ColumnCount)*ColumnSize + int(header.SpanCount)*SpanSize)
	if stat.Size() != expectedSize {
		return Header{}, fmt.Errorf("Berlin voxel tile %q has %d bytes, want %d", path, stat.Size(), expectedSize)
	}

	return header, nil
}
