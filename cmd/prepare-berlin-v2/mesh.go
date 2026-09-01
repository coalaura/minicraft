package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/coalaura/minicraft/internal/berlinvoxel"
)

const (
	meshPortalURL = "https://www.businesslocationcenter.de/berlin3d-downloadportal/"
	meshIndexURL  = meshPortalURL + "datasource-data/berlin-mesh-2025/mesh-index-2025.json"
	meshBaseURL   = meshPortalURL + "datasource-data/berlin-mesh-2025"
	meshTermsURL  = meshPortalURL + "resources/terms/terms.de.html"

	originLongitude = 13.377704
	originLatitude  = 52.516275
)

type meshIndex struct {
	Type     string        `json:"type"`
	Features []meshFeature `json:"features"`
}

type meshFeature struct {
	Properties meshProperties `json:"properties"`
	Geometry   meshGeometry   `json:"geometry"`
}

type meshProperties struct {
	URL string `json:"url"`
}

type meshGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type meshSelection struct {
	Mode       string
	Radius     float64
	ExactTiles map[string]struct{}
}

func prepareMesh(ctx context.Context, output string, selection meshSelection, logger *log.Logger) (int, int, error) {
	err := os.MkdirAll(output, 0o755)
	if err != nil {
		return 0, 0, fmt.Errorf("create Berlin mesh output directory: %w", err)
	}

	index, err := fetchMeshIndex(ctx)
	if err != nil {
		return 0, 0, err
	}

	features, err := selectMeshFeatures(index, selection)
	if err != nil {
		return 0, 0, err
	}

	logger.Printf("3D mesh: selected %d source tile(s)", len(features))

	for index, feature := range features {
		filename := feature.Properties.URL
		url := meshBaseURL + "/" + filename

		complete, err := preparationComplete(output, url)
		if err != nil {
			return 0, 0, err
		}

		if complete {
			logger.Printf("3D mesh [%d/%d] already prepared: %s", index+1, len(features), filename)
			continue
		}

		logger.Printf("3D mesh [%d/%d] preparing: %s", index+1, len(features), filename)

		path, cleanup, err := downloadTemporary(ctx, output, url, "mesh-*")
		if err != nil {
			return 0, 0, err
		}

		files, prepareErr := prepareMeshZip(path, output)
		cleanup()

		if prepareErr != nil {
			return 0, 0, fmt.Errorf("prepare 3D mesh %s: %w", filename, prepareErr)
		}

		err = markPreparationComplete(output, url, files)
		if err != nil {
			return 0, 0, err
		}
	}

	files, err := voxelTileFiles(output)
	if err != nil {
		return 0, 0, fmt.Errorf("list Berlin mesh voxel tiles: %w", err)
	}

	err = writeMeshManifest(output, selection.Mode, selection.Radius, len(features), len(files))
	if err != nil {
		return 0, 0, fmt.Errorf("write Berlin mesh manifest: %w", err)
	}

	return len(features), len(files), nil
}

func fetchMeshIndex(ctx context.Context) (meshIndex, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, meshIndexURL, nil)
	if err != nil {
		return meshIndex{}, err
	}

	request.Header.Set("User-Agent", "minicraft-berlin-v2/1")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return meshIndex{}, fmt.Errorf("download Berlin 3D mesh index: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return meshIndex{}, fmt.Errorf("download Berlin 3D mesh index: HTTP %s", response.Status)
	}

	contents, err := io.ReadAll(io.LimitReader(response.Body, 128<<20))
	if err != nil {
		return meshIndex{}, fmt.Errorf("read Berlin 3D mesh index: %w", err)
	}

	if len(contents) >= 2 && contents[0] == 0x1f && contents[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(contents))
		if err != nil {
			return meshIndex{}, fmt.Errorf("open compressed Berlin 3D mesh index: %w", err)
		}

		contents, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return meshIndex{}, fmt.Errorf("decompress Berlin 3D mesh index: %w", err)
		}

		if closeErr != nil {
			return meshIndex{}, fmt.Errorf("close Berlin 3D mesh index: %w", closeErr)
		}
	}

	return decodeMeshIndex(contents)
}

func decodeMeshIndex(contents []byte) (meshIndex, error) {
	contents = bytes.TrimPrefix(contents, []byte{0xef, 0xbb, 0xbf})

	index := meshIndex{}
	err := json.Unmarshal(contents, &index)
	if err != nil {
		return meshIndex{}, fmt.Errorf("decode Berlin 3D mesh index: %w", err)
	}

	if index.Type != "FeatureCollection" || len(index.Features) == 0 {
		return meshIndex{}, fmt.Errorf("Berlin 3D mesh index is not a non-empty FeatureCollection")
	}

	return index, nil
}

func selectMeshFeatures(index meshIndex, selection meshSelection) ([]meshFeature, error) {
	mode := strings.ToLower(selection.Mode)
	if mode == "" {
		mode = "origin"
	}

	if mode != "origin" && mode != "all" && mode != "none" {
		return nil, fmt.Errorf("unknown Berlin mesh mode %q; use none, origin, or all", selection.Mode)
	}

	if mode == "none" && len(selection.ExactTiles) == 0 {
		return nil, nil
	}

	selected := make([]meshFeature, 0)
	foundExact := make(map[string]struct{})

	for _, feature := range index.Features {
		filename := feature.Properties.URL
		if filename == "" {
			continue
		}

		if len(selection.ExactTiles) > 0 {
			if _, wanted := selection.ExactTiles[filename]; !wanted {
				continue
			}

			selected = append(selected, feature)
			foundExact[filename] = struct{}{}
			continue
		}

		if mode == "all" || featureIntersectsOriginRadius(feature, selection.Radius) {
			selected = append(selected, feature)
		}
	}

	if len(selection.ExactTiles) > 0 {
		missing := make([]string, 0)
		for filename := range selection.ExactTiles {
			if _, found := foundExact[filename]; !found {
				missing = append(missing, filename)
			}
		}

		if len(missing) > 0 {
			sort.Strings(missing)
			return nil, fmt.Errorf("Berlin 3D mesh index does not contain requested tile(s): %s", strings.Join(missing, ", "))
		}
	}

	sort.Slice(selected, func(left, right int) bool {
		return selected[left].Properties.URL < selected[right].Properties.URL
	})

	return selected, nil
}

func featureIntersectsOriginRadius(feature meshFeature, radius float64) bool {
	if radius <= 0 {
		radius = 2000
	}

	var coordinates any
	err := json.Unmarshal(feature.Geometry.Coordinates, &coordinates)
	if err != nil {
		return false
	}

	minimumX := math.Inf(1)
	maximumX := math.Inf(-1)
	minimumY := math.Inf(1)
	maximumY := math.Inf(-1)

	walkCoordinates(coordinates, func(longitude, latitude float64) {
		x, y := geographicOffset(longitude, latitude)
		minimumX = min(minimumX, x)
		maximumX = max(maximumX, x)
		minimumY = min(minimumY, y)
		maximumY = max(maximumY, y)
	})

	if math.IsInf(minimumX, 0) {
		return false
	}

	closestX := max(minimumX, min(0.0, maximumX))
	closestY := max(minimumY, min(0.0, maximumY))

	return math.Hypot(closestX, closestY) <= radius
}

func walkCoordinates(value any, visit func(longitude, latitude float64)) {
	values, valid := value.([]any)
	if !valid || len(values) == 0 {
		return
	}

	if len(values) >= 2 {
		longitude, longitudeOK := values[0].(float64)
		latitude, latitudeOK := values[1].(float64)

		if longitudeOK && latitudeOK {
			visit(longitude, latitude)
			return
		}
	}

	for _, child := range values {
		walkCoordinates(child, visit)
	}
}

func geographicOffset(longitude, latitude float64) (float64, float64) {
	metresPerDegree := 111320.0
	x := (longitude - originLongitude) * metresPerDegree * math.Cos(originLatitude*math.Pi/180)
	y := (latitude - originLatitude) * metresPerDegree
	return x, y
}

func prepareMeshZip(path, output string) ([]string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open Berlin 3D mesh ZIP %q: %w", path, err)
	}
	defer archive.Close()

	var objectFile *zip.File
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || strings.ToLower(filepath.Ext(entry.Name)) != ".obj" {
			continue
		}

		if objectFile != nil {
			return nil, fmt.Errorf("Berlin 3D mesh ZIP %q contains multiple OBJ files", path)
		}

		objectFile = entry
	}

	if objectFile == nil {
		return nil, fmt.Errorf("Berlin 3D mesh ZIP %q contains no OBJ file", path)
	}

	reader, err := objectFile.Open()
	if err != nil {
		return nil, fmt.Errorf("open %q in Berlin 3D mesh ZIP: %w", objectFile.Name, err)
	}
	defer reader.Close()

	tiles := newVoxelTiles(output)
	err = prepareOBJ(reader, tiles)
	if err != nil {
		return nil, err
	}

	return tiles.Flush()
}

func prepareOBJ(reader io.Reader, tiles *voxelTiles) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 128*1024), 8<<20)

	vertices := make([]berlinvoxel.Point, 0, 256*1024)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "v ") || strings.HasPrefix(line, "v\t") {
			vertex, err := parseOBJVertex(line)
			if err != nil {
				return fmt.Errorf("parse OBJ line %d: %w", lineNumber, err)
			}

			vertices = append(vertices, vertex)
			continue
		}

		if !strings.HasPrefix(line, "f ") && !strings.HasPrefix(line, "f\t") {
			continue
		}

		indices, err := parseOBJFace(line, len(vertices))
		if err != nil {
			return fmt.Errorf("parse OBJ line %d: %w", lineNumber, err)
		}

		for index := 1; index+1 < len(indices); index++ {
			first := vertices[indices[0]]
			second := vertices[indices[index]]
			third := vertices[indices[index+1]]
			var emitErr error

			berlinvoxel.RasterizeTriangle(first, second, third, func(easting, northing int32, height int) {
				if emitErr != nil {
					return
				}

				added, err := tiles.AddBlock(easting, northing, height)
				if err != nil {
					emitErr = err
					return
				}

				if !added {
					return
				}

				emitErr = tiles.Mark(easting, northing)
			})

			if emitErr != nil {
				return emitErr
			}
		}
	}

	err := scanner.Err()
	if err != nil {
		return fmt.Errorf("read Berlin 3D mesh OBJ: %w", err)
	}

	if len(vertices) == 0 {
		return fmt.Errorf("Berlin 3D mesh OBJ contains no vertices")
	}

	return nil
}

func parseOBJVertex(line string) (berlinvoxel.Point, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return berlinvoxel.Point{}, fmt.Errorf("OBJ vertex has %d fields, want at least 4", len(fields))
	}

	x, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return berlinvoxel.Point{}, fmt.Errorf("OBJ vertex X %q: %w", fields[1], err)
	}

	y, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return berlinvoxel.Point{}, fmt.Errorf("OBJ vertex Y %q: %w", fields[2], err)
	}

	z, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return berlinvoxel.Point{}, fmt.Errorf("OBJ vertex Z %q: %w", fields[3], err)
	}

	return berlinvoxel.Point{X: x, Y: y, Z: z}, nil
}

func parseOBJFace(line string, vertexCount int) ([]int, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return nil, fmt.Errorf("OBJ face has %d vertices, want at least 3", len(fields)-1)
	}

	indices := make([]int, 0, len(fields)-1)

	for _, field := range fields[1:] {
		vertexField := field
		separator := strings.IndexByte(vertexField, '/')
		if separator >= 0 {
			vertexField = vertexField[:separator]
		}

		value, err := strconv.Atoi(vertexField)
		if err != nil || value == 0 {
			return nil, fmt.Errorf("invalid OBJ vertex reference %q", field)
		}

		index := value - 1
		if value < 0 {
			index = vertexCount + value
		}

		if index < 0 || index >= vertexCount {
			return nil, fmt.Errorf("OBJ vertex reference %d outside 1..%d", value, vertexCount)
		}

		indices = append(indices, index)
	}

	return indices, nil
}

func writeMeshManifest(output, mode string, radius float64, sourceTiles, voxelTiles int) error {
	manifest := map[string]any{
		"format_version":       berlinvoxel.FormatVersion,
		"source":               "Berlin 3D Mesh Model 2025",
		"portal":               meshPortalURL,
		"index":                meshIndexURL,
		"terms":                meshTermsURL,
		"provider_attribution": "Berlin Partner für Wirtschaft und Technologie GmbH",
		"crs":                  "EPSG:25833 (ETRS89 / UTM zone 33N)",
		"mesh_mode":            mode,
		"mesh_radius_metres":   radius,
		"source_tile_count":    sourceTiles,
		"voxel_tile_count":     voxelTiles,
	}

	contents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	contents = append(contents, '\n')
	return os.WriteFile(filepath.Join(output, "manifest.json"), contents, 0o644)
}
