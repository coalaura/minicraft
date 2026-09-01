package berlinvoxel

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coalaura/minicraft/internal/berlinheight"
)

const (
	FormatVersion  = uint16(1)
	TileHeaderSize = 48
	MaskSize       = (berlinheight.TileCellCount + 7) / 8
	ColumnSize     = 8
	SpanSize       = 4
)

var tileMagic = [8]byte{'M', 'C', 'B', 'V', 'O', 'X', '1', 0}

type Header struct {
	Key         berlinheight.TileKey
	MinimumY    uint16
	MaximumY    uint16
	MarkedCells uint32
	ColumnCount uint32
	SpanCount   uint32
}

type Span struct {
	MinimumY uint16
	MaximumY uint16
}

type Column struct {
	Index     uint32
	FirstSpan uint32
}

func TileFilename(key berlinheight.TileKey) string {
	return fmt.Sprintf("%d_%d.mcv", key.Easting, key.Northing)
}

func ParseTileFilename(name string) (berlinheight.TileKey, bool) {
	if filepath.Ext(name) != ".mcv" {
		return berlinheight.TileKey{}, false
	}

	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	parts := strings.Split(base, "_")
	if len(parts) != 2 {
		return berlinheight.TileKey{}, false
	}

	easting, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return berlinheight.TileKey{}, false
	}

	northing, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return berlinheight.TileKey{}, false
	}

	key := berlinheight.TileKey{Easting: int32(easting), Northing: int32(northing)}
	if key != berlinheight.KeyFor(key.Easting, key.Northing) {
		return berlinheight.TileKey{}, false
	}

	return key, true
}

func ReadHeader(reader io.Reader) (Header, error) {
	buffer := make([]byte, TileHeaderSize)
	_, err := io.ReadFull(reader, buffer)
	if err != nil {
		return Header{}, err
	}

	var magic [8]byte
	copy(magic[:], buffer[:8])

	if magic != tileMagic {
		return Header{}, fmt.Errorf("invalid Berlin voxel tile magic")
	}

	version := binary.LittleEndian.Uint16(buffer[8:10])
	if version != FormatVersion {
		return Header{}, fmt.Errorf("unsupported Berlin voxel tile version %d", version)
	}

	tileSize := binary.LittleEndian.Uint16(buffer[10:12])
	if tileSize != uint16(berlinheight.TileSize) {
		return Header{}, fmt.Errorf("invalid Berlin voxel tile width %d", tileSize)
	}

	return Header{
		Key: berlinheight.TileKey{
			Easting:  int32(binary.LittleEndian.Uint32(buffer[12:16])),
			Northing: int32(binary.LittleEndian.Uint32(buffer[16:20])),
		},
		MinimumY:    binary.LittleEndian.Uint16(buffer[20:22]),
		MaximumY:    binary.LittleEndian.Uint16(buffer[22:24]),
		MarkedCells: binary.LittleEndian.Uint32(buffer[24:28]),
		ColumnCount: binary.LittleEndian.Uint32(buffer[28:32]),
		SpanCount:   binary.LittleEndian.Uint32(buffer[32:36]),
	}, nil
}

func WriteTile(path string, header Header, marked []byte, columns []Column, spans []Span) error {
	if len(marked) != MaskSize {
		return fmt.Errorf("Berlin voxel tile mark mask contains %d bytes, want %d", len(marked), MaskSize)
	}

	if len(columns) != int(header.ColumnCount) {
		return fmt.Errorf("Berlin voxel tile contains %d columns, header says %d", len(columns), header.ColumnCount)
	}

	if len(spans) != int(header.SpanCount) {
		return fmt.Errorf("Berlin voxel tile contains %d spans, header says %d", len(spans), header.SpanCount)
	}

	if header.Key != berlinheight.KeyFor(header.Key.Easting, header.Key.Northing) {
		return fmt.Errorf("Berlin voxel tile origin %d,%d is not aligned to %d metres", header.Key.Easting, header.Key.Northing, berlinheight.TileSize)
	}

	for index, column := range columns {
		if column.Index >= uint32(berlinheight.TileCellCount) {
			return fmt.Errorf("Berlin voxel column %d has invalid cell index %d", index, column.Index)
		}

		if column.FirstSpan > uint32(len(spans)) {
			return fmt.Errorf("Berlin voxel column %d has invalid first span %d", index, column.FirstSpan)
		}

		if index > 0 {
			previous := columns[index-1]
			if column.Index <= previous.Index || column.FirstSpan < previous.FirstSpan {
				return fmt.Errorf("Berlin voxel columns are not strictly sorted")
			}
		}
	}

	if len(columns) > 0 && columns[len(columns)-1].FirstSpan >= uint32(len(spans)) {
		return fmt.Errorf("Berlin voxel final column has no spans")
	}

	headerBuffer := make([]byte, TileHeaderSize)
	copy(headerBuffer[:8], tileMagic[:])
	binary.LittleEndian.PutUint16(headerBuffer[8:10], FormatVersion)
	binary.LittleEndian.PutUint16(headerBuffer[10:12], uint16(berlinheight.TileSize))
	binary.LittleEndian.PutUint32(headerBuffer[12:16], uint32(header.Key.Easting))
	binary.LittleEndian.PutUint32(headerBuffer[16:20], uint32(header.Key.Northing))
	binary.LittleEndian.PutUint16(headerBuffer[20:22], header.MinimumY)
	binary.LittleEndian.PutUint16(headerBuffer[22:24], header.MaximumY)
	binary.LittleEndian.PutUint32(headerBuffer[24:28], header.MarkedCells)
	binary.LittleEndian.PutUint32(headerBuffer[28:32], header.ColumnCount)
	binary.LittleEndian.PutUint32(headerBuffer[32:36], header.SpanCount)

	directory := filepath.Dir(path)
	err := os.MkdirAll(directory, 0o755)
	if err != nil {
		return fmt.Errorf("create Berlin voxel tile directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".berlin-voxel-*")
	if err != nil {
		return fmt.Errorf("create temporary Berlin voxel tile: %w", err)
	}

	temporaryPath := temporary.Name()
	installed := false

	defer func() {
		_ = temporary.Close()
		if !installed {
			_ = os.Remove(temporaryPath)
		}
	}()

	_, err = temporary.Write(headerBuffer)
	if err != nil {
		return fmt.Errorf("write Berlin voxel tile header: %w", err)
	}

	_, err = temporary.Write(marked)
	if err != nil {
		return fmt.Errorf("write Berlin voxel tile mark mask: %w", err)
	}

	columnBuffer := make([]byte, ColumnSize*4096)
	for start := 0; start < len(columns); start += 4096 {
		end := min(start+4096, len(columns))
		buffer := columnBuffer[:(end-start)*ColumnSize]

		for index, column := range columns[start:end] {
			offset := index * ColumnSize
			binary.LittleEndian.PutUint32(buffer[offset:offset+4], column.Index)
			binary.LittleEndian.PutUint32(buffer[offset+4:offset+8], column.FirstSpan)
		}

		_, err = temporary.Write(buffer)
		if err != nil {
			return fmt.Errorf("write Berlin voxel tile columns: %w", err)
		}
	}

	spanBuffer := make([]byte, SpanSize*8192)
	for start := 0; start < len(spans); start += 8192 {
		end := min(start+8192, len(spans))
		buffer := spanBuffer[:(end-start)*SpanSize]

		for index, span := range spans[start:end] {
			offset := index * SpanSize
			binary.LittleEndian.PutUint16(buffer[offset:offset+2], span.MinimumY)
			binary.LittleEndian.PutUint16(buffer[offset+2:offset+4], span.MaximumY)
		}

		_, err = temporary.Write(buffer)
		if err != nil {
			return fmt.Errorf("write Berlin voxel tile spans: %w", err)
		}
	}

	err = temporary.Sync()
	if err != nil {
		return fmt.Errorf("sync Berlin voxel tile: %w", err)
	}

	err = temporary.Close()
	if err != nil {
		return fmt.Errorf("close Berlin voxel tile: %w", err)
	}

	err = os.Rename(temporaryPath, path)
	if err != nil {
		removeErr := os.Remove(path)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("replace Berlin voxel tile: %w", removeErr)
		}

		err = os.Rename(temporaryPath, path)
		if err != nil {
			return fmt.Errorf("install Berlin voxel tile: %w", err)
		}
	}

	installed = true
	return nil
}
