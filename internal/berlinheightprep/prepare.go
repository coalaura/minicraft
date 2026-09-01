package berlinheightprep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/coalaura/minicraft/internal/berlinheight"
)

type Options struct {
	Source       string
	Output       string
	Dataset      string
	HeightOffset int32
	Origin       Origin
	Logger       *log.Logger
}

type Origin struct {
	Description string `json:"description"`
	Easting     int32  `json:"easting"`
	Northing    int32  `json:"northing"`
}

type Result struct {
	TileCount int
}

type manifest struct {
	FormatVersion uint16 `json:"format_version"`
	Dataset       string `json:"dataset,omitempty"`
	Source        string `json:"source"`
	CRS           string `json:"crs"`
	HeightDatum   string `json:"height_datum"`
	TileSize      int32  `json:"tile_size_metres"`
	TileCount     int    `json:"tile_count"`
	HeightOffset  int32  `json:"minecraft_height_offset_metres"`
	MinecraftMinY int32  `json:"minecraft_min_y"`
	MinecraftMaxY int32  `json:"minecraft_max_y"`
	Origin        Origin `json:"minecraft_origin"`
}

func Run(ctx context.Context, options Options) (Result, error) {
	if options.Source == "" {
		return Result{}, fmt.Errorf("Berlin height source is empty")
	}

	if options.Output == "" {
		return Result{}, fmt.Errorf("Berlin height output directory is empty")
	}

	if options.Origin == (Origin{}) {
		options.Origin = Origin{
			Description: "Minecraft x=0,z=0 at Brandenburg Gate; +x east, +z south",
			Easting:     389918,
			Northing:    5819699,
		}
	}

	if options.HeightOffset == 0 {
		options.HeightOffset = 90
	}

	logger := options.Logger
	if logger == nil {
		logger = log.Default()
	}

	err := os.MkdirAll(options.Output, 0o755)
	if err != nil {
		return Result{}, fmt.Errorf("create output directory: %w", err)
	}

	resources, err := discoverResources(ctx, options.Source)
	if err != nil {
		return Result{}, err
	}

	if len(resources) == 0 {
		return Result{}, fmt.Errorf("no Berlin height XYZ resources found at %q", options.Source)
	}

	logger.Printf("found %d source resource(s)", len(resources))

	for index, resource := range resources {
		complete, err := resourceComplete(options.Output, resource)
		if err != nil {
			return Result{}, err
		}

		if complete {
			logger.Printf("[%d/%d] already prepared: %s", index+1, len(resources), resource.ID)
			continue
		}

		logger.Printf("[%d/%d] preparing: %s", index+1, len(resources), resource.ID)

		tiles, err := prepareResource(ctx, options.Output, resource)
		if err != nil {
			return Result{}, fmt.Errorf("prepare %s: %w", resource.ID, err)
		}

		err = markResourceComplete(options.Output, resource, tiles)
		if err != nil {
			return Result{}, err
		}
	}

	count, err := writeManifest(options)
	if err != nil {
		return Result{}, err
	}

	logger.Printf("Berlin %s ready: %d tile(s) in %s", options.Dataset, count, options.Output)
	return Result{TileCount: count}, nil
}

func writeManifest(options Options) (int, error) {
	entries, err := os.ReadDir(options.Output)
	if err != nil {
		return 0, fmt.Errorf("read prepared Berlin directory: %w", err)
	}

	tiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		_, valid := berlinheight.ParseTileFilename(entry.Name())
		if valid {
			tiles = append(tiles, entry.Name())
		}
	}

	sort.Strings(tiles)

	metadata := manifest{
		FormatVersion: berlinheight.FormatVersion,
		Dataset:       options.Dataset,
		Source:        options.Source,
		CRS:           "EPSG:25833 (ETRS89 / UTM zone 33N)",
		HeightDatum:   "DHHN2016",
		TileSize:      berlinheight.TileSize,
		TileCount:     len(tiles),
		HeightOffset:  options.HeightOffset,
		MinecraftMinY: -64,
		MinecraftMaxY: 319,
		Origin:        options.Origin,
	}

	encoded, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("encode Berlin manifest: %w", err)
	}

	encoded = append(encoded, '\n')
	path := filepath.Join(options.Output, "manifest.json")

	err = os.WriteFile(path, encoded, 0o644)
	if err != nil {
		return 0, fmt.Errorf("write Berlin manifest: %w", err)
	}

	return len(tiles), nil
}
