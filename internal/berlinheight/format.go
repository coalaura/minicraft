package berlinheight

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	TileSize       = int32(2000)
	TileCellCount  = int(TileSize * TileSize)
	BytesPerCell   = 2
	TileDataSize   = TileCellCount * BytesPerCell
	NoData         = uint16(65535)
	FormatVersion  = uint16(2)
	TileHeaderSize = 32
)

var tileMagic = [8]byte{'M', 'C', 'B', 'H', 'G', 'T', '2', 0}

type TileKey struct {
	Easting  int32
	Northing int32
}

type Header struct {
	Key        TileKey
	MinimumY   uint16
	MaximumY   uint16
	ValidCells uint32
}

func KeyFor(easting, northing int32) TileKey {
	return TileKey{
		Easting:  floorToTile(easting),
		Northing: floorToTile(northing),
	}
}

func Index(key TileKey, easting, northing int32) (int, bool) {
	localEasting := easting - key.Easting
	localNorthing := northing - key.Northing

	if localEasting < 0 || localEasting >= TileSize || localNorthing < 0 || localNorthing >= TileSize {
		return 0, false
	}

	return int(localNorthing*TileSize + localEasting), true
}

func TileFilename(key TileKey) string {
	return fmt.Sprintf("%d_%d.mch", key.Easting, key.Northing)
}

func ParseTileFilename(name string) (TileKey, bool) {
	if filepath.Ext(name) != ".mch" {
		return TileKey{}, false
	}

	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	parts := strings.Split(base, "_")
	if len(parts) != 2 {
		return TileKey{}, false
	}

	easting, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return TileKey{}, false
	}

	northing, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return TileKey{}, false
	}

	key := TileKey{Easting: int32(easting), Northing: int32(northing)}
	if key != KeyFor(key.Easting, key.Northing) {
		return TileKey{}, false
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
		return Header{}, fmt.Errorf("invalid Berlin height tile magic")
	}

	version := binary.LittleEndian.Uint16(buffer[8:10])
	if version != FormatVersion {
		return Header{}, fmt.Errorf("unsupported Berlin height tile version %d", version)
	}

	width := binary.LittleEndian.Uint16(buffer[10:12])
	if width != uint16(TileSize) {
		return Header{}, fmt.Errorf("invalid Berlin height tile width %d", width)
	}

	return Header{
		Key: TileKey{
			Easting:  int32(binary.LittleEndian.Uint32(buffer[12:16])),
			Northing: int32(binary.LittleEndian.Uint32(buffer[16:20])),
		},
		MinimumY:   binary.LittleEndian.Uint16(buffer[20:22]),
		MaximumY:   binary.LittleEndian.Uint16(buffer[22:24]),
		ValidCells: binary.LittleEndian.Uint32(buffer[24:28]),
	}, nil
}

func WriteTile(path string, header Header, heights []uint16) error {
	if len(heights) != TileCellCount {
		return fmt.Errorf("Berlin height tile contains %d cells, want %d", len(heights), TileCellCount)
	}

	if header.Key != KeyFor(header.Key.Easting, header.Key.Northing) {
		return fmt.Errorf("Berlin height tile origin %d,%d is not aligned to %d metres", header.Key.Easting, header.Key.Northing, TileSize)
	}

	headerBuffer := make([]byte, TileHeaderSize)
	copy(headerBuffer[:8], tileMagic[:])
	binary.LittleEndian.PutUint16(headerBuffer[8:10], FormatVersion)
	binary.LittleEndian.PutUint16(headerBuffer[10:12], uint16(TileSize))
	binary.LittleEndian.PutUint32(headerBuffer[12:16], uint32(header.Key.Easting))
	binary.LittleEndian.PutUint32(headerBuffer[16:20], uint32(header.Key.Northing))
	binary.LittleEndian.PutUint16(headerBuffer[20:22], header.MinimumY)
	binary.LittleEndian.PutUint16(headerBuffer[22:24], header.MaximumY)
	binary.LittleEndian.PutUint32(headerBuffer[24:28], header.ValidCells)

	directory := filepath.Dir(path)
	err := os.MkdirAll(directory, 0o755)
	if err != nil {
		return fmt.Errorf("create Berlin height tile directory: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".berlin-height-*")
	if err != nil {
		return fmt.Errorf("create temporary Berlin height tile: %w", err)
	}

	temporaryPath := temporary.Name()
	keepTemporary := false

	defer func() {
		_ = temporary.Close()
		if !keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	_, err = temporary.Write(headerBuffer)
	if err != nil {
		return fmt.Errorf("write Berlin height tile header: %w", err)
	}

	dataBuffer := make([]byte, 64*1024)
	cellsPerBuffer := len(dataBuffer) / BytesPerCell

	for start := 0; start < len(heights); start += cellsPerBuffer {
		end := min(start+cellsPerBuffer, len(heights))
		buffer := dataBuffer[:(end-start)*BytesPerCell]

		for index, height := range heights[start:end] {
			binary.LittleEndian.PutUint16(buffer[index*BytesPerCell:], height)
		}

		_, err = temporary.Write(buffer)
		if err != nil {
			return fmt.Errorf("write Berlin height tile data: %w", err)
		}
	}

	err = temporary.Sync()
	if err != nil {
		return fmt.Errorf("sync Berlin height tile: %w", err)
	}

	err = temporary.Close()
	if err != nil {
		return fmt.Errorf("close Berlin height tile: %w", err)
	}

	err = os.Rename(temporaryPath, path)
	if err != nil {
		removeErr := os.Remove(path)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("replace Berlin height tile: %w", removeErr)
		}

		err = os.Rename(temporaryPath, path)
		if err != nil {
			return fmt.Errorf("install Berlin height tile: %w", err)
		}
	}

	keepTemporary = true
	return nil
}

func floorToTile(value int32) int32 {
	remainder := value % TileSize
	if remainder < 0 {
		remainder += TileSize
	}

	return value - remainder
}
