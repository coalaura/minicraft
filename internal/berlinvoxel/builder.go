package berlinvoxel

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"sort"

	"github.com/coalaura/minicraft/internal/berlinheight"
)

type Builder struct {
	key         berlinheight.TileKey
	marked      []byte
	columns     map[uint32][]Span
	markedCells uint32
}

func NewBuilder(key berlinheight.TileKey) *Builder {
	return &Builder{
		key:     key,
		marked:  make([]byte, MaskSize),
		columns: make(map[uint32][]Span),
	}
}

func (builder *Builder) Key() berlinheight.TileKey {
	return builder.key
}

func (builder *Builder) Mark(easting, northing int32) bool {
	index, valid := berlinheight.Index(builder.key, easting, northing)
	if !valid {
		return false
	}

	byteIndex := index >> 3
	bit := byte(1 << (index & 7))

	if builder.marked[byteIndex]&bit == 0 {
		builder.marked[byteIndex] |= bit
		builder.markedCells++
	}

	return true
}

func (builder *Builder) AddBlock(easting, northing int32, height int) bool {
	if height < 0 || height >= int(berlinheight.NoData) {
		return false
	}

	index, valid := berlinheight.Index(builder.key, easting, northing)
	if !valid {
		return false
	}

	builder.addSpan(uint32(index), Span{MinimumY: uint16(height), MaximumY: uint16(height)})
	return true
}

func (builder *Builder) AddSpan(easting, northing int32, minimumY, maximumY int) bool {
	if minimumY > maximumY {
		minimumY, maximumY = maximumY, minimumY
	}

	if minimumY < 0 || maximumY >= int(berlinheight.NoData) {
		return false
	}

	index, valid := berlinheight.Index(builder.key, easting, northing)
	if !valid {
		return false
	}

	builder.addSpan(uint32(index), Span{MinimumY: uint16(minimumY), MaximumY: uint16(maximumY)})
	return true
}

func (builder *Builder) Empty() bool {
	return builder.markedCells == 0 && len(builder.columns) == 0
}

func (builder *Builder) Write(directory string) (string, error) {
	columnIndices := make([]int, 0, len(builder.columns))
	for index := range builder.columns {
		columnIndices = append(columnIndices, int(index))
	}
	sort.Ints(columnIndices)

	columns := make([]Column, 0, len(columnIndices))
	spans := make([]Span, 0, len(columnIndices))
	minimumY := math.MaxUint16
	maximumY := 0

	for _, rawIndex := range columnIndices {
		index := uint32(rawIndex)
		columnSpans := builder.columns[index]
		columnSpans = mergeSpans(columnSpans)

		columns = append(columns, Column{Index: index, FirstSpan: uint32(len(spans))})
		spans = append(spans, columnSpans...)

		for _, span := range columnSpans {
			minimumY = min(minimumY, int(span.MinimumY))
			maximumY = max(maximumY, int(span.MaximumY))
		}
	}

	if len(spans) == 0 {
		minimumY = 0
		maximumY = 0
	}

	header := Header{
		Key:         builder.key,
		MinimumY:    uint16(minimumY),
		MaximumY:    uint16(maximumY),
		MarkedCells: builder.markedCells,
		ColumnCount: uint32(len(columns)),
		SpanCount:   uint32(len(spans)),
	}

	filename := TileFilename(builder.key)
	path := filepath.Join(directory, filename)

	err := WriteTile(path, header, builder.marked, columns, spans)
	if err != nil {
		return "", fmt.Errorf("write Berlin voxel tile %s: %w", filename, err)
	}

	return filename, nil
}

func (builder *Builder) addSpan(index uint32, addition Span) {
	spans := builder.columns[index]

	for spanIndex := 0; spanIndex < len(spans); spanIndex++ {
		current := spans[spanIndex]

		if int(addition.MaximumY)+1 < int(current.MinimumY) || int(current.MaximumY)+1 < int(addition.MinimumY) {
			continue
		}

		addition.MinimumY = min(addition.MinimumY, current.MinimumY)
		addition.MaximumY = max(addition.MaximumY, current.MaximumY)
		spans = append(spans[:spanIndex], spans[spanIndex+1:]...)
		spanIndex--
	}

	builder.columns[index] = append(spans, addition)
}

func mergeSpans(spans []Span) []Span {
	if len(spans) < 2 {
		return spans
	}

	sort.Slice(spans, func(left, right int) bool {
		if spans[left].MinimumY == spans[right].MinimumY {
			return spans[left].MaximumY < spans[right].MaximumY
		}

		return spans[left].MinimumY < spans[right].MinimumY
	})

	merged := spans[:1]
	for _, candidate := range spans[1:] {
		current := &merged[len(merged)-1]

		if int(candidate.MinimumY) <= int(current.MaximumY)+1 {
			current.MaximumY = max(current.MaximumY, candidate.MaximumY)
			continue
		}

		merged = append(merged, candidate)
	}

	return merged
}

func (builder *Builder) MergeFile(path string) error {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read existing Berlin voxel tile %q: %w", path, err)
	}

	if len(contents) < TileHeaderSize+MaskSize {
		return fmt.Errorf("existing Berlin voxel tile %q is truncated", path)
	}

	header, err := ReadHeader(bytes.NewReader(contents[:TileHeaderSize]))
	if err != nil {
		return fmt.Errorf("read existing Berlin voxel tile %q header: %w", path, err)
	}

	if header.Key != builder.key {
		return fmt.Errorf("existing Berlin voxel tile %q has unexpected origin %d,%d", path, header.Key.Easting, header.Key.Northing)
	}

	columnsStart := TileHeaderSize + MaskSize
	spansStart := columnsStart + int(header.ColumnCount)*ColumnSize
	expectedSize := spansStart + int(header.SpanCount)*SpanSize
	if len(contents) != expectedSize {
		return fmt.Errorf("existing Berlin voxel tile %q has %d bytes, want %d", path, len(contents), expectedSize)
	}

	mask := contents[TileHeaderSize:columnsStart]
	for index, value := range mask {
		newBits := value &^ builder.marked[index]
		builder.markedCells += uint32(bits.OnesCount8(newBits))
		builder.marked[index] |= value
	}

	columnData := contents[columnsStart:spansStart]
	spanData := contents[spansStart:]

	for columnIndex := 0; columnIndex < int(header.ColumnCount); columnIndex++ {
		offset := columnIndex * ColumnSize
		cellIndex := binary.LittleEndian.Uint32(columnData[offset : offset+4])
		firstSpan := binary.LittleEndian.Uint32(columnData[offset+4 : offset+8])
		lastSpan := header.SpanCount

		if columnIndex+1 < int(header.ColumnCount) {
			nextOffset := (columnIndex + 1) * ColumnSize
			lastSpan = binary.LittleEndian.Uint32(columnData[nextOffset+4 : nextOffset+8])
		}

		for spanIndex := firstSpan; spanIndex < lastSpan; spanIndex++ {
			spanOffset := int(spanIndex) * SpanSize
			span := Span{
				MinimumY: binary.LittleEndian.Uint16(spanData[spanOffset : spanOffset+2]),
				MaximumY: binary.LittleEndian.Uint16(spanData[spanOffset+2 : spanOffset+4]),
			}
			builder.addSpan(cellIndex, span)
		}
	}

	return nil
}
