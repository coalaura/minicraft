package server

import (
	"reflect"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestNaturalSpawnLightCacheRetainedSize(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})
	position := game.BlockPosition{X: 40, Y: -63}

	_, _, ready := runtime.naturalLightAt(position)
	if !ready {
		t.Fatal("light preparation failed")
	}

	query := runtime.naturalChunkAt(position)
	value := reflect.ValueOf(query).Elem()
	retained := int(value.Type().Size())

	// Count every retained slice, so adding separate sky/block buffers fails too.
	for _, field := range value.Fields() {
		if field.Kind() == reflect.Slice {
			retained += field.Cap() * int(field.Type().Elem().Size())
		}
	}

	if cap(query.Light) != 98304 || retained > 98304+4096 {
		t.Fatalf("spawn chunk retains %d bytes (%d light bytes), want 96 KiB lighting plus at most 4 KiB metadata", retained, cap(query.Light))
	}

	// Exercise both nibbles together, rather than only darkness or full skylight.
	runtime.World.SetBlock(game.BlockPosition{X: 41, Y: -59}, game.Glowstone)

	sky, block, ready := runtime.naturalLightAt(game.BlockPosition{X: 40, Y: -59})
	if !ready || sky != 15 || block != 14 {
		t.Fatalf("packed sky/block light = %d/%d (ready %t), want 15/14", sky, block, ready)
	}
}

func TestNaturalSpawnDistantEditPreservesCache(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})
	position := game.BlockPosition{X: 40, Y: -63, Z: 8}

	runtime.naturalLightAt(position)
	runtime.naturalSurfaceHeight(position)

	query := runtime.naturalChunkAt(position)
	revisions := query.Revisions
	column := int(position.Z&15)*16 + int(position.X&15)

	runtime.World.SetBlock(game.BlockPosition{X: 1000, Y: 70, Z: 1000}, game.Glowstone)
	runtime.naturalSpawning.InTick = true
	runtime.naturalSpawning.LightPreparations = 0

	_, _, ready := runtime.naturalLightAt(position)
	if !ready || query.Revisions != revisions || !query.HeightKnown[column] {
		t.Fatal("distant edit invalidated cached lighting or height")
	}

	edit := game.BlockPosition{X: 40, Y: -59, Z: 8}
	runtime.World.SetBlock(edit, game.Stone)

	_, _, ready = runtime.naturalLightAt(position)
	if ready || query.HeightKnown[column] {
		t.Fatal("local edit failed to invalidate height/light or bypassed the cold-light budget")
	}

	if runtime.naturalSurfaceHeight(position) != -58 {
		t.Fatal("height cache did not observe the new top block")
	}

	runtime.naturalSpawning.LightPreparations = 1
	runtime.naturalLightAt(position)

	revisions = query.Revisions

	runtime.World.SetBlock(edit, game.Stone)

	_, _, ready = runtime.naturalLightAt(position)
	if !ready || query.Revisions != revisions || !query.HeightKnown[column] {
		t.Fatal("unchanged block invalidated caches")
	}

	if !runtime.World.CompareAndSetBlock(edit, game.Stone, game.Air) {
		t.Fatal("could not restore generated block")
	}

	_, _, ready = runtime.naturalLightAt(position)
	if ready || runtime.naturalSurfaceHeight(position) != -59 {
		t.Fatal("compare-and-set restoring generated terrain did not invalidate caches")
	}
}

func TestNaturalSpawnNeighborLightInvalidation(t *testing.T) {
	positions := []game.BlockPosition{
		{X: 32, Y: -63, Z: 8},
		{X: 0, Y: -63, Z: 8},
		{X: 8, Y: -63, Z: 32},
		{X: 8, Y: -63, Z: 0},
	}

	for _, position := range positions {
		runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})

		runtime.entityRandom = func() float32 {
			return 0
		}

		if !runtime.naturalEnvironmentAllows(position, game.EntityZombie) {
			t.Fatal("initial dark cave rejected")
		}

		runtime.naturalSurfaceHeight(position)

		query := runtime.naturalChunkAt(position)
		column := int(position.Z&15)*16 + int(position.X&15)
		edit := position

		if position.X == 8 {
			edit.Z--
		} else {
			edit.X--
		}

		runtime.World.SetBlocks([]game.BlockChange{{Position: edit, Replacement: game.Glowstone}})
		runtime.naturalChunkAt(position)

		if len(query.Light) != 0 || !query.HeightKnown[column] {
			t.Fatalf("neighbor edit at %+v must invalidate light but preserve height at %+v", edit, position)
		}

		if runtime.naturalEnvironmentAllows(position, game.EntityZombie) {
			t.Fatalf("neighbor glowstone at %+v did not prevent spawning at %+v", edit, position)
		}

		runtime.World.SetBlock(edit, game.Air)

		if !runtime.naturalEnvironmentAllows(position, game.EntityZombie) {
			t.Fatal("removing neighbor light did not restore dark spawn query")
		}
	}
}
