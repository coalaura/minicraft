package berlin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/coalaura/minicraft/internal/berlinheight"
	"github.com/coalaura/minicraft/internal/berlinvoxel"
	"github.com/coalaura/minicraft/internal/game"
)

func TestLegacyGeneratorMapsBerlinCoordinatesOneToOne(t *testing.T) {
	directory := t.TempDir()
	writeHeightTile(t, directory, 132, nil)

	generated, err := New(directory)
	if err != nil {
		t.Fatalf("create legacy Berlin generator: %v", err)
	}

	position := game.BlockPosition{X: 10, Y: 42, Z: 20}
	if block := generated.BlockAt(0, position); block != game.Stone {
		t.Fatalf("block at legacy surface height = %v, want stone", block)
	}

	position.Y++
	if block := generated.BlockAt(0, position); block != game.Air {
		t.Fatalf("block above legacy surface = %v, want air", block)
	}
}

func TestV2LoD2MakesBuildingInteriorHollow(t *testing.T) {
	generated := testV2Generator(t)

	roof := game.BlockPosition{X: 10, Y: 57, Z: 20}
	if block := generated.BlockAt(0, roof); block != game.Stone {
		t.Fatalf("LoD2 roof block = %v, want stone", block)
	}

	interior := game.BlockPosition{X: 10, Y: 40, Z: 20}
	if block := generated.BlockAt(0, interior); block != game.Air {
		t.Fatalf("LoD2 building interior = %v, want air", block)
	}

	ground := game.BlockPosition{X: 10, Y: -50, Z: 20}
	if block := generated.BlockAt(0, ground); block != game.Stone {
		t.Fatalf("DGM ground block = %v, want stone", block)
	}
}

func TestV2MeshOverridesLoD2AndBDOM(t *testing.T) {
	generated := testV2Generator(t)

	meshTop := game.BlockPosition{X: 20, Y: 310, Z: 20}
	if block := generated.BlockAt(0, meshTop); block != game.Stone {
		t.Fatalf("400 metre mesh surface = %v, want stone", block)
	}

	openSpace := game.BlockPosition{X: 20, Y: 42, Z: 20}
	if block := generated.BlockAt(0, openSpace); block != game.Air {
		t.Fatalf("space below authoritative mesh = %v, want air", block)
	}
}

func TestV2RetainsBDOMFallbackAwayFromDetailedGeometry(t *testing.T) {
	generated := testV2Generator(t)

	position := game.BlockPosition{X: 30, Y: 42, Z: 20}
	if block := generated.BlockAt(0, position); block != game.Stone {
		t.Fatalf("bDOM fallback surface = %v, want stone", block)
	}

	position.Y++
	if block := generated.BlockAt(0, position); block != game.Air {
		t.Fatalf("block above bDOM fallback = %v, want air", block)
	}
}

func TestGeneratorSpawnsAtHighestOriginSurface(t *testing.T) {
	generated := testV2Generator(t)
	spawn := generated.Spawn(0)

	want := game.Position{X: 0.5, Y: 43, Z: 0.5}
	if spawn != want {
		t.Fatalf("spawn = %+v, want %+v", spawn, want)
	}
}

func TestGeneratedChunkUsesUniformTerrainFastPath(t *testing.T) {
	generated := testV2Generator(t)
	chunk := generated.GenerateChunk(0, game.ChunkPosition{}).(*generatedChunk)

	var blocks [game.SectionVolume]game.Block
	block, uniform := chunk.GenerateSection(-64, &blocks)
	if !uniform {
		t.Fatal("section below DGM terrain was not uniform")
	}

	if block != game.Stone {
		t.Fatalf("uniform block = %v, want stone", block)
	}
}

func TestGenerationBoundsRejectsUncoveredChunks(t *testing.T) {
	generated := testV2Generator(t)

	_, _, valid := generated.GenerationBounds(0, game.ChunkPosition{X: 10000, Z: 10000})
	if valid {
		t.Fatal("uncovered chunk reported generation bounds")
	}
}

func testV2Generator(t *testing.T) *Generator {
	t.Helper()

	directory := t.TempDir()
	writeHeightTile(t, directory, 132, nil)

	terrainDirectory := filepath.Join(directory, terrainDirectoryName)
	err := os.MkdirAll(terrainDirectory, 0o755)
	if err != nil {
		t.Fatalf("create terrain directory: %v", err)
	}
	writeHeightTile(t, terrainDirectory, 40, nil)

	buildingsDirectory := filepath.Join(directory, buildingsDirectoryName)
	err = os.MkdirAll(buildingsDirectory, 0o755)
	if err != nil {
		t.Fatalf("create buildings directory: %v", err)
	}

	buildingKey := berlinheight.KeyFor(originEasting+10, originNorthing-20)
	building := berlinvoxel.NewBuilder(buildingKey)
	building.Mark(originEasting+10, originNorthing-20)
	building.AddBlock(originEasting+10, originNorthing-20, 147)
	if _, err := building.Write(buildingsDirectory); err != nil {
		t.Fatalf("write building voxel tile: %v", err)
	}

	meshDirectory := filepath.Join(directory, meshDirectoryName)
	err = os.MkdirAll(meshDirectory, 0o755)
	if err != nil {
		t.Fatalf("create mesh directory: %v", err)
	}

	meshKey := berlinheight.KeyFor(originEasting+20, originNorthing-20)
	mesh := berlinvoxel.NewBuilder(meshKey)
	mesh.Mark(originEasting+20, originNorthing-20)
	mesh.AddBlock(originEasting+20, originNorthing-20, 400)
	if _, err := mesh.Write(meshDirectory); err != nil {
		t.Fatalf("write mesh voxel tile: %v", err)
	}

	generated, err := New(directory)
	if err != nil {
		t.Fatalf("create Berlin v2 generator: %v", err)
	}

	return generated
}

func writeHeightTile(t *testing.T, directory string, defaultHeight uint16, modify func(key berlinheight.TileKey, heights []uint16)) {
	t.Helper()

	key := berlinheight.KeyFor(originEasting, originNorthing)
	heights := make([]uint16, berlinheight.TileCellCount)
	for index := range heights {
		heights[index] = defaultHeight
	}

	if modify != nil {
		modify(key, heights)
	}

	header := berlinheight.Header{
		Key:        key,
		MinimumY:   defaultHeight,
		MaximumY:   defaultHeight,
		ValidCells: uint32(len(heights)),
	}

	path := filepath.Join(directory, berlinheight.TileFilename(key))
	err := berlinheight.WriteTile(path, header, heights)
	if err != nil {
		t.Fatalf("write test height tile: %v", err)
	}
}
