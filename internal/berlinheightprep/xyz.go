package berlinheightprep

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coalaura/minicraft/internal/berlinheight"
)

const xyzBufferSize = 1 << 20

func prepareXYZ(reader io.Reader, name, output string) (string, error) {
	buffered := bufio.NewReaderSize(reader, xyzBufferSize)
	heights := make([]uint16, berlinheight.TileCellCount)

	for index := range heights {
		heights[index] = berlinheight.NoData
	}

	key, hasKey := tileKeyFromName(name)
	minimumHeight := math.MaxUint16
	maximumHeight := 0
	validCells := uint32(0)
	lineNumber := 0

	for {
		line, err := buffered.ReadSlice('\n')
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("read %s line %d: %w", name, lineNumber+1, err)
		}

		if len(line) > 0 {
			lineNumber++
			line = bytes.TrimSpace(line)

			if len(line) > 0 {
				easting, northing, height, parseErr := parseXYZLine(line)
				if parseErr != nil {
					return "", fmt.Errorf("parse %s line %d: %w", name, lineNumber, parseErr)
				}

				if height < 0 || height >= int(berlinheight.NoData) {
					return "", fmt.Errorf("%s line %d has rounded height %d outside supported range 0..65534", name, lineNumber, height)
				}

				pointEasting := int32(easting)
				pointNorthing := int32(northing)

				if !hasKey {
					key = berlinheight.KeyFor(pointEasting, pointNorthing)
					hasKey = true
				}

				index, valid := berlinheight.Index(key, pointEasting, pointNorthing)
				if !valid {
					if excludedOuterBoundary(key, pointEasting, pointNorthing) {
						continue
					}

					pointKey := berlinheight.KeyFor(pointEasting, pointNorthing)
					return "", fmt.Errorf("%s contains data outside 2 km tile %d,%d (line %d belongs to %d,%d)", name, key.Easting, key.Northing, lineNumber, pointKey.Easting, pointKey.Northing)
				}

				if heights[index] == berlinheight.NoData {
					validCells++
				}

				heights[index] = uint16(height)
				minimumHeight = min(minimumHeight, height)
				maximumHeight = max(maximumHeight, height)
			}
		}

		if err == io.EOF {
			break
		}
	}

	if !hasKey || validCells == 0 {
		return "", fmt.Errorf("%s contains no height points", name)
	}

	header := berlinheight.Header{
		Key:        key,
		MinimumY:   uint16(minimumHeight),
		MaximumY:   uint16(maximumHeight),
		ValidCells: validCells,
	}

	filename := berlinheight.TileFilename(key)
	path := filepath.Join(output, filename)

	err := berlinheight.WriteTile(path, header, heights)
	if err != nil {
		return "", fmt.Errorf("write prepared tile for %s: %w", name, err)
	}

	return filename, nil
}

func tileKeyFromName(name string) (berlinheight.TileKey, bool) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	parts := strings.Split(base, "_")
	tileKilometres := int64(berlinheight.TileSize / 1000)

	for index := 0; index+2 < len(parts); index++ {
		eastingKilometres, eastingErr := strconv.ParseInt(parts[index], 10, 32)
		northingKilometres, northingErr := strconv.ParseInt(parts[index+1], 10, 32)
		sizeKilometres, sizeErr := strconv.ParseInt(parts[index+2], 10, 32)

		if eastingErr != nil || northingErr != nil || sizeErr != nil || sizeKilometres != tileKilometres {
			continue
		}

		easting := int32(eastingKilometres * 1000)
		northing := int32(northingKilometres * 1000)
		key := berlinheight.TileKey{Easting: easting, Northing: northing}

		if key == berlinheight.KeyFor(easting, northing) {
			return key, true
		}
	}

	return berlinheight.TileKey{}, false
}

func excludedOuterBoundary(key berlinheight.TileKey, easting, northing int32) bool {
	localEasting := easting - key.Easting
	localNorthing := northing - key.Northing

	if localEasting < 0 || localEasting > berlinheight.TileSize || localNorthing < 0 || localNorthing > berlinheight.TileSize {
		return false
	}

	return localEasting == berlinheight.TileSize || localNorthing == berlinheight.TileSize
}

func parseXYZLine(line []byte) (int, int, int, error) {
	first, remainder, valid := nextField(line)
	if !valid {
		return 0, 0, 0, fmt.Errorf("missing easting")
	}

	second, remainder, valid := nextField(remainder)
	if !valid {
		return 0, 0, 0, fmt.Errorf("missing northing")
	}

	third, _, valid := nextField(remainder)
	if !valid {
		return 0, 0, 0, fmt.Errorf("missing height")
	}

	easting, err := parseRoundedDecimal(first)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("easting: %w", err)
	}

	northing, err := parseRoundedDecimal(second)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("northing: %w", err)
	}

	height, err := parseRoundedDecimal(third)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("height: %w", err)
	}

	return easting, northing, height, nil
}

func nextField(input []byte) ([]byte, []byte, bool) {
	start := 0
	for start < len(input) && whitespace(input[start]) {
		start++
	}

	if start == len(input) {
		return nil, nil, false
	}

	end := start
	for end < len(input) && !whitespace(input[end]) {
		end++
	}

	return input[start:end], input[end:], true
}

func parseRoundedDecimal(value []byte) (int, error) {
	if len(value) >= 3 && value[0] == 0xef && value[1] == 0xbb && value[2] == 0xbf {
		value = value[3:]
	}

	if len(value) == 0 {
		return 0, fmt.Errorf("empty number")
	}

	sign := 1
	index := 0

	if value[index] == '-' {
		sign = -1
		index++
	} else if value[index] == '+' {
		index++
	}

	if index == len(value) {
		return 0, fmt.Errorf("invalid number %q", value)
	}

	integer := 0
	digits := 0

	for index < len(value) && value[index] >= '0' && value[index] <= '9' {
		integer = integer*10 + int(value[index]-'0')
		index++
		digits++
	}

	if digits == 0 {
		return 0, fmt.Errorf("invalid number %q", value)
	}

	roundUp := false
	if index < len(value) && value[index] == '.' {
		index++

		if index < len(value) {
			if value[index] < '0' || value[index] > '9' {
				return 0, fmt.Errorf("invalid number %q", value)
			}

			roundUp = value[index] >= '5'

			for index < len(value) && value[index] >= '0' && value[index] <= '9' {
				index++
			}
		}
	}

	if index != len(value) {
		return 0, fmt.Errorf("invalid number %q", value)
	}

	if roundUp {
		integer++
	}

	return sign * integer, nil
}

func whitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}
