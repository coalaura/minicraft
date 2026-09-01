package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/coalaura/minicraft/internal/berlinheightprep"
)

const (
	defaultOutput        = "data/berlin"
	defaultTerrainSource = "https://gdi.berlin.de/data/dgm1/atom/"
	defaultLoD2Source    = "https://gdi.berlin.de/data/a_lod2/atom/0.atom"
)

type options struct {
	output          string
	terrainSource   string
	lod2Source      string
	meshMode        string
	meshRadius      float64
	meshTiles       stringList
	skipTerrain     bool
	skipLoD2        bool
	acceptMeshTerms bool
}

type stringList []string

type preparationManifest struct {
	Version            int    `json:"version"`
	TerrainDirectory   string `json:"terrain_directory,omitempty"`
	TerrainTiles       int    `json:"terrain_tiles,omitempty"`
	BuildingsDirectory string `json:"buildings_directory,omitempty"`
	BuildingVoxelTiles int    `json:"building_voxel_tiles,omitempty"`
	MeshDirectory      string `json:"mesh_directory,omitempty"`
	MeshMode           string `json:"mesh_mode,omitempty"`
	MeshSourceTiles    int    `json:"mesh_source_tiles,omitempty"`
	MeshVoxelTiles     int    `json:"mesh_voxel_tiles,omitempty"`
	FallbackSurface    string `json:"fallback_surface"`
	CoordinateSystem   string `json:"coordinate_system"`
	HeightMapping      string `json:"height_mapping"`
	MinecraftOrigin    string `json:"minecraft_origin"`
	GenerationPriority string `json:"generation_priority"`
}

func main() {
	configuration := parseOptions()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	logger := log.Default()
	manifest := preparationManifest{
		Version:            2,
		FallbackSurface:    configuration.output,
		CoordinateSystem:   "EPSG:25833 (ETRS89 / UTM zone 33N), DHHN2016 height",
		HeightMapping:      "1 Minecraft block = 1 metre; Minecraft Y = DHHN2016 - 90",
		MinecraftOrigin:    "Brandenburg Gate: E=389918 N=5819699; +X east, +Z south",
		GenerationPriority: "DGM1 ground -> 2025 mesh -> LoD2 buildings -> bDOM fallback",
	}

	if !configuration.skipTerrain {
		terrainDirectory := filepath.Join(configuration.output, "terrain")
		result, err := berlinheightprep.Run(ctx, berlinheightprep.Options{
			Source:  configuration.terrainSource,
			Output:  terrainDirectory,
			Dataset: "DGM1 terrain",
			Logger:  logger,
		})
		if err != nil {
			log.Fatal(err)
		}

		manifest.TerrainDirectory = terrainDirectory
		manifest.TerrainTiles = result.TileCount
	}

	if !configuration.skipLoD2 {
		buildingsDirectory := filepath.Join(configuration.output, "buildings")
		count, err := prepareLoD2(ctx, configuration.lod2Source, buildingsDirectory, logger)
		if err != nil {
			log.Fatal(err)
		}

		manifest.BuildingsDirectory = buildingsDirectory
		manifest.BuildingVoxelTiles = count
	}

	meshRequested := strings.ToLower(configuration.meshMode) != "none" || len(configuration.meshTiles) > 0
	if meshRequested {
		if !meshTermsAccepted(configuration.acceptMeshTerms) {
			log.Fatalf("Berlin 3D Mesh terms must be accepted before downloading mesh data; read %s and rerun with -accept-mesh-terms or BERLIN_3D_MESH_TERMS_ACCEPTED=true", meshTermsURL)
		}

		meshDirectory := filepath.Join(configuration.output, "mesh")
		exactTiles := make(map[string]struct{}, len(configuration.meshTiles))
		for _, tile := range configuration.meshTiles {
			exactTiles[tile] = struct{}{}
		}

		sourceCount, voxelCount, err := prepareMesh(ctx, meshDirectory, meshSelection{
			Mode:       configuration.meshMode,
			Radius:     configuration.meshRadius,
			ExactTiles: exactTiles,
		}, logger)
		if err != nil {
			log.Fatal(err)
		}

		manifest.MeshDirectory = meshDirectory
		manifest.MeshMode = configuration.meshMode
		manifest.MeshSourceTiles = sourceCount
		manifest.MeshVoxelTiles = voxelCount
	}

	err := writePreparationManifest(configuration.output, manifest)
	if err != nil {
		log.Fatal(err)
	}

	logger.Printf("Berlin v2 data ready in %s", configuration.output)
}

func parseOptions() options {
	configuration := options{}

	flag.StringVar(&configuration.output, "output", defaultOutput, "Berlin data directory; existing v1 bDOM .mch tiles are kept as fallback")
	flag.StringVar(&configuration.terrainSource, "terrain-source", defaultTerrainSource, "Berlin DGM1 Atom feed, local XYZ/ZIP file, or local directory")
	flag.StringVar(&configuration.lod2Source, "lod2-source", defaultLoD2Source, "Berlin LoD2 Atom feed, local CityGML/ZIP file, or local directory")
	flag.StringVar(&configuration.meshMode, "mesh", "origin", "2025 photogrammetric mesh coverage: none, origin, or all")
	flag.Float64Var(&configuration.meshRadius, "mesh-radius", 2000, "radius around Brandenburg Gate to prepare when -mesh origin is used")
	flag.Var(&configuration.meshTiles, "mesh-tile", "prepare one exact 2025 mesh ZIP filename from the portal index; repeat for multiple tiles")
	flag.BoolVar(&configuration.skipTerrain, "skip-terrain", false, "keep existing prepared DGM1 terrain without downloading it")
	flag.BoolVar(&configuration.skipLoD2, "skip-lod2", false, "keep existing prepared LoD2 buildings without downloading them")
	flag.BoolVar(&configuration.acceptMeshTerms, "accept-mesh-terms", false, "confirm the Berlin 3D Mesh portal terms have been read and accepted")
	flag.Parse()

	return configuration
}

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("mesh tile name is empty")
	}

	*values = append(*values, value)
	return nil
}

func meshTermsAccepted(flagAccepted bool) bool {
	if flagAccepted {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(os.Getenv("BERLIN_3D_MESH_TERMS_ACCEPTED")), "true")
}

func writePreparationManifest(output string, manifest preparationManifest) error {
	contents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Berlin v2 manifest: %w", err)
	}

	contents = append(contents, '\n')
	path := filepath.Join(output, "v2-manifest.json")

	err = os.WriteFile(path, contents, 0o644)
	if err != nil {
		return fmt.Errorf("write Berlin v2 manifest: %w", err)
	}

	return nil
}
