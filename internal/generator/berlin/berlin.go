package berlin

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/coalaura/minicraft/internal/berlinheight"
	"github.com/coalaura/minicraft/internal/berlinvoxel"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
)

const (
	Name = "berlin"

	DefaultDataDirectory = "data/berlin"
	DataDirectoryEnv     = "MINICRAFT_BERLIN_DATA"

	terrainDirectoryName   = "terrain"
	buildingsDirectoryName = "buildings"
	meshDirectoryName      = "mesh"

	originEasting  = int32(389918)
	originNorthing = int32(5819699)
	minimumY       = int32(-64)
	maximumY       = int32(319)
	heightOffset   = int32(90)
)

type Generator struct {
	surface   *berlinheight.Store
	terrain   *berlinheight.Store
	buildings *berlinvoxel.Store
	mesh      *berlinvoxel.Store
	legacy    bool
}

type generatedChunk struct {
	columns         [game.ChunkWidth * game.ChunkWidth]generatedColumn
	minimumBaseY    int32
	maximumY        int32
	hasData         bool
	allColumnsSolid bool
}

type generatedColumn struct {
	baseY             int32
	fallbackY         int32
	hasBase           bool
	hasFallback       bool
	meshAuthoritative bool
	buildingAuthority bool
	meshSpans         []berlinvoxel.Span
	buildingSpans     []berlinvoxel.Span
}

var (
	_ game.Generator                    = (*Generator)(nil)
	_ game.SectionGenerator             = (*Generator)(nil)
	_ game.ChunkGenerator               = (*Generator)(nil)
	_ game.BoundedGenerator             = (*Generator)(nil)
	_ game.SpawnGenerator               = (*Generator)(nil)
	_ game.BiomeGenerator               = (*Generator)(nil)
	_ game.WorldMetadataGenerator       = (*Generator)(nil)
	_ game.GeneratedChunk               = (*generatedChunk)(nil)
	_ game.GeneratedChunkBiomeGenerator = (*generatedChunk)(nil)
)

func (generated *Generator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position.Y < minimumY || position.Y > maximumY {
		return game.Air
	}

	column := generated.columnAt(position.X, position.Z)
	if column.blockAt(position.Y) {
		return game.Stone
	}

	return game.Air
}

func (generated *Generator) BiomeAt(_ int64, _, _, _ int32) game.Biome {
	return game.BiomePlains
}

func (generated *Generator) GenerateChunk(_ int64, chunkPosition game.ChunkPosition) game.GeneratedChunk {
	chunk := &generatedChunk{
		minimumBaseY:    math.MaxInt32,
		maximumY:        math.MinInt32,
		allColumnsSolid: true,
	}

	chunkMinX := chunkPosition.X * game.ChunkWidth
	chunkMinZ := chunkPosition.Z * game.ChunkWidth

	for localZ := range int32(game.ChunkWidth) {
		for localX := range int32(game.ChunkWidth) {
			column := generated.columnAt(chunkMinX+localX, chunkMinZ+localZ)
			index := localZ*game.ChunkWidth + localX
			chunk.columns[index] = column

			if !column.hasData() {
				chunk.allColumnsSolid = false
				continue
			}

			chunk.hasData = true
			chunk.maximumY = max(chunk.maximumY, column.maximumY())

			if !column.hasBase {
				chunk.allColumnsSolid = false
				continue
			}

			chunk.minimumBaseY = min(chunk.minimumBaseY, column.baseY)
		}
	}

	if !chunk.hasData {
		chunk.minimumBaseY = 0
		chunk.maximumY = 0
	}

	return chunk
}

func (generated *Generator) GenerateSection(seed int64, chunkPosition game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	chunk := generated.GenerateChunk(seed, chunkPosition)
	return chunk.GenerateSection(sectionMinY, blocks)
}

func (chunk *generatedChunk) GenerateSection(sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	if !chunk.hasData {
		return game.Air, true
	}

	sectionMaxY := sectionMinY + game.ChunkWidth - 1
	if sectionMaxY < minimumY || sectionMinY > chunk.maximumY {
		return game.Air, true
	}

	if chunk.allColumnsSolid && sectionMinY >= minimumY && sectionMaxY <= chunk.minimumBaseY {
		return game.Stone, true
	}

	for localY := range int32(game.ChunkWidth) {
		worldY := sectionMinY + localY

		for localZ := range int32(game.ChunkWidth) {
			for localX := range int32(game.ChunkWidth) {
				columnIndex := localZ*game.ChunkWidth + localX
				index := localY*game.ChunkWidth*game.ChunkWidth + localZ*game.ChunkWidth + localX

				if chunk.columns[columnIndex].blockAt(worldY) {
					blocks[index] = game.Stone
					continue
				}

				blocks[index] = game.Air
			}
		}
	}

	first := blocks[0]
	for _, block := range blocks[1:] {
		if block != first {
			return game.Air, false
		}
	}

	return first, true
}

func (*generatedChunk) BiomeAt(_, _, _ int32) game.Biome {
	return game.BiomePlains
}

func (generated *Generator) GenerationBounds(_ int64, chunkPosition game.ChunkPosition) (int32, int32, bool) {
	chunk := generated.GenerateChunk(0, chunkPosition).(*generatedChunk)
	if !chunk.hasData {
		return 0, 0, false
	}

	minimum := minimumY
	if !chunk.allColumnsSolid {
		minimum = max(minimumY, chunk.minimumOccupiedY())
	}

	return minimum, chunk.maximumY, true
}

func (generated *Generator) Spawn(_ int64) game.Position {
	column := generated.columnAt(0, 0)
	if !column.hasData() {
		return game.Position{X: 0.5, Y: 64, Z: 0.5}
	}

	height := column.maximumY()
	return game.Position{X: 0.5, Y: float64(min(height+1, maximumY)), Z: 0.5}
}

func (*Generator) WorldMetadata(_ int64) game.WorldMetadata {
	return game.WorldMetadata{SeaLevel: -heightOffset}
}

func init() {
	generator.MustRegister(Name, newRegistered)
}

func New(dataDirectory string) (*Generator, error) {
	surface, err := openHeightStoreOptional(dataDirectory)
	if err != nil {
		return nil, err
	}

	terrain, err := openHeightStoreOptional(filepath.Join(dataDirectory, terrainDirectoryName))
	if err != nil {
		return nil, err
	}

	buildings, err := openVoxelStoreOptional(filepath.Join(dataDirectory, buildingsDirectoryName))
	if err != nil {
		return nil, err
	}

	mesh, err := openVoxelStoreOptional(filepath.Join(dataDirectory, meshDirectoryName))
	if err != nil {
		return nil, err
	}

	if surface == nil && terrain == nil {
		return nil, fmt.Errorf("Berlin data directory %q contains neither prepared bDOM surface tiles nor v2 DGM terrain tiles", dataDirectory)
	}

	generated := &Generator{
		surface:   surface,
		terrain:   terrain,
		buildings: buildings,
		mesh:      mesh,
		legacy:    terrain == nil && buildings == nil && mesh == nil,
	}

	if !generated.columnAt(0, 0).hasData() {
		return nil, fmt.Errorf("Berlin data does not cover the Brandenburg Gate origin; prepare the complete height dataset in %q", dataDirectory)
	}

	return generated, nil
}

func newRegistered() (game.Generator, error) {
	dataDirectory := os.Getenv(DataDirectoryEnv)
	if dataDirectory == "" {
		dataDirectory = DefaultDataDirectory
	}

	return New(dataDirectory)
}

func (generated *Generator) columnAt(worldX, worldZ int32) generatedColumn {
	easting := originEasting + worldX
	northing := originNorthing - worldZ
	column := generatedColumn{}

	if generated.surface != nil {
		height, valid := generated.surface.HeightAt(easting, northing)
		if valid {
			column.fallbackY = minecraftHeight(height)
			column.hasFallback = true
		}
	}

	if generated.terrain != nil {
		height, valid := generated.terrain.HeightAt(easting, northing)
		if valid {
			column.baseY = minecraftHeight(height)
			column.hasBase = true
		}
	}

	if generated.legacy && !column.hasBase && column.hasFallback {
		column.baseY = column.fallbackY
		column.hasBase = true
	}

	if generated.mesh != nil {
		marked, spans, valid := generated.mesh.ColumnAt(easting, northing)
		if valid {
			column.meshAuthoritative = marked || len(spans) > 0
			column.meshSpans = spans
		}
	}

	if generated.buildings != nil && !column.meshAuthoritative {
		marked, spans, valid := generated.buildings.ColumnAt(easting, northing)
		if valid {
			column.buildingAuthority = marked || len(spans) > 0
			column.buildingSpans = spans
		}
	}

	return column
}

func (column generatedColumn) blockAt(worldY int32) bool {
	if worldY < minimumY || worldY > maximumY {
		return false
	}

	if column.hasBase && worldY <= column.baseY {
		return true
	}

	rawY := worldY + heightOffset

	if column.meshAuthoritative {
		return spansContain(column.meshSpans, rawY)
	}

	if column.buildingAuthority {
		return spansContain(column.buildingSpans, rawY)
	}

	if column.hasFallback && worldY <= column.fallbackY {
		return true
	}

	return false
}

func (column generatedColumn) hasData() bool {
	return column.hasBase || column.hasFallback || column.meshAuthoritative || column.buildingAuthority
}

func (column generatedColumn) maximumY() int32 {
	maximum := int32(math.MinInt32)

	if column.hasBase {
		maximum = max(maximum, column.baseY)
	}

	if column.meshAuthoritative {
		maximum = max(maximum, spansMaximumY(column.meshSpans))
		return maximum
	}

	if column.buildingAuthority {
		maximum = max(maximum, spansMaximumY(column.buildingSpans))
		return maximum
	}

	if column.hasFallback {
		maximum = max(maximum, column.fallbackY)
	}

	return maximum
}

func (chunk *generatedChunk) minimumOccupiedY() int32 {
	minimum := int32(math.MaxInt32)

	for _, column := range chunk.columns {
		if !column.hasData() {
			continue
		}

		if column.hasBase {
			minimum = min(minimum, minimumY)
			continue
		}

		minimum = min(minimum, column.minimumY())
	}

	if minimum == math.MaxInt32 {
		return minimumY
	}

	return minimum
}

func (column generatedColumn) minimumY() int32 {
	if column.hasBase || column.hasFallback {
		return minimumY
	}

	if column.meshAuthoritative {
		return spansMinimumY(column.meshSpans)
	}

	return spansMinimumY(column.buildingSpans)
}

func spansContain(spans []berlinvoxel.Span, rawY int32) bool {
	if rawY < 0 || rawY >= int32(berlinheight.NoData) {
		return false
	}

	for _, span := range spans {
		if rawY >= int32(span.MinimumY) && rawY <= int32(span.MaximumY) {
			return true
		}
	}

	return false
}

func spansMaximumY(spans []berlinvoxel.Span) int32 {
	if len(spans) == 0 {
		return math.MinInt32
	}

	maximum := int32(spans[0].MaximumY)
	for _, span := range spans[1:] {
		maximum = max(maximum, int32(span.MaximumY))
	}

	return minecraftHeight(maximum)
}

func spansMinimumY(spans []berlinvoxel.Span) int32 {
	if len(spans) == 0 {
		return maximumY
	}

	minimum := int32(spans[0].MinimumY)
	for _, span := range spans[1:] {
		minimum = min(minimum, int32(span.MinimumY))
	}

	return minecraftHeight(minimum)
}

func openHeightStoreOptional(directory string) (*berlinheight.Store, error) {
	info, err := os.Stat(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("inspect Berlin height directory %q: %w", directory, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("Berlin height path %q is not a directory", directory)
	}

	containsTiles, err := directoryContainsExtension(directory, ".mch")
	if err != nil {
		return nil, err
	}

	if !containsTiles {
		return nil, nil
	}

	return berlinheight.Open(directory)
}

func openVoxelStoreOptional(directory string) (*berlinvoxel.Store, error) {
	info, err := os.Stat(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("inspect Berlin voxel directory %q: %w", directory, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("Berlin voxel path %q is not a directory", directory)
	}

	containsTiles, err := directoryContainsExtension(directory, ".mcv")
	if err != nil {
		return nil, err
	}

	if !containsTiles {
		return nil, nil
	}

	return berlinvoxel.Open(directory)
}

func directoryContainsExtension(directory, extension string) (bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, fmt.Errorf("read Berlin data directory %q: %w", directory, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) == extension {
			return true, nil
		}
	}

	return false, nil
}

func minecraftHeight(rawHeight int32) int32 {
	return max(min(rawHeight-heightOffset, maximumY), minimumY)
}
