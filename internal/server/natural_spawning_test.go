package server

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type spawnTestGenerator struct {
	Floor game.Block
	Roof  bool
	Water bool
}

type spawnTestBiomeGenerator struct {
	spawnTestGenerator
	Biome game.Biome
}

type sourceSpawnEntry struct {
	Type    string `json:"type"`
	Weight  int    `json:"weight"`
	Minimum int    `json:"minCount"`
	Maximum int    `json:"maxCount"`
}

type sourceSpawnBiome struct {
	Spawners map[string][]sourceSpawnEntry `json:"spawners"`
	Costs    map[string]sourceSpawnCost    `json:"spawn_costs"`
}

type sourceSpawnCost struct {
	Charge float64 `json:"charge"`
	Budget float64 `json:"energy_budget"`
}

type structTestSpawnEnvironment struct {
	Name                        string
	Type                        game.EntityType
	Floor                       game.Block
	Roof, Water, Peaceful, Want bool
}

type structTestSpawnDistance struct {
	X        int32
	Category int
	Want     bool
}

func (generator spawnTestGenerator) BlockAt(_ int64, position game.BlockPosition) game.Block {
	if position.Y == -64 || (generator.Roof && position.Y == -60) {
		return generator.Floor
	}

	if generator.Water && position.Y >= -63 && position.Y <= -58 {
		return game.Water
	}

	return game.Air
}

func (generator spawnTestBiomeGenerator) BiomeAt(_ int64, _, _, _ int32) game.Biome {
	return generator.Biome
}

func TestNaturalSpawnTablesMatchPinnedSource(t *testing.T) {
	categoryNames := [4]string{"monster", "creature", "ambient", "water_ambient"}

	supported := map[string]game.EntityType{
		"zombie": game.EntityZombie, "skeleton": game.EntitySkeleton, "creeper": game.EntityCreeper,
		"cow": game.EntityCow, "sheep": game.EntitySheep, "chicken": game.EntityChicken, "bat": game.EntityBat,
		"cod": game.EntityCod, "salmon": game.EntitySalmon, "tropical_fish": game.EntityTropicalFish, "pufferfish": game.EntityPufferfish,
	}

	for biome := range game.BiomeCount {
		name := strings.TrimPrefix(game.BiomeNames[biome], "minecraft:")

		contents, err := os.ReadFile("../../../reference/client_source/data/minecraft/worldgen/biome/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}

		var source sourceSpawnBiome

		err = json.Unmarshal(contents, &source)
		if err != nil {
			t.Fatal(err)
		}

		for category, categoryName := range categoryNames {
			entries := naturalBiomeSpawns[biome][category]
			index := 0

			for _, expected := range source.Spawners[categoryName] {
				entityType, exists := supported[strings.TrimPrefix(expected.Type, "minecraft:")]
				if !exists {
					continue
				}

				if index >= len(entries) {
					t.Fatalf("%s/%s missing source entry", name, categoryName)
				}

				actual := entries[index]
				cost := source.Costs[expected.Type]

				if actual.Type != entityType || actual.Weight != expected.Weight || actual.Minimum != expected.Minimum || actual.Maximum != expected.Maximum || actual.Charge != cost.Charge || actual.Budget != cost.Budget {
					t.Fatalf("%s/%s entry %d differs from source: %+v vs %+v", name, categoryName, index, actual, expected)
				}

				index++
			}

			if index != len(entries) {
				t.Fatalf("%s/%s has extra entries", name, categoryName)
			}
		}
	}
}

func TestNaturalSpawnEnvironment(t *testing.T) {
	position := game.BlockPosition{X: 40, Y: -63}

	tests := []structTestSpawnEnvironment{
		{Name: "dark zombie", Type: game.EntityZombie, Floor: game.Stone, Roof: true, Want: true},
		{Name: "dark skeleton", Type: game.EntitySkeleton, Floor: game.Stone, Roof: true, Want: true},
		{Name: "dark creeper", Type: game.EntityCreeper, Floor: game.Stone, Roof: true, Want: true},
		{Name: "peaceful", Type: game.EntityZombie, Floor: game.Stone, Roof: true, Peaceful: true},
		{Name: "bright hostile", Type: game.EntityZombie, Floor: game.Stone},
		{Name: "cow grass", Type: game.EntityCow, Floor: game.GrassBlock, Want: true},
		{Name: "sheep grass", Type: game.EntitySheep, Floor: game.GrassBlock, Want: true},
		{Name: "chicken grass", Type: game.EntityChicken, Floor: game.GrassBlock, Want: true},
		{Name: "animal stone", Type: game.EntityCow, Floor: game.Stone},
		{Name: "animal dark", Type: game.EntityCow, Floor: game.GrassBlock, Roof: true},
		{Name: "bat cave", Type: game.EntityBat, Floor: game.Stone, Roof: true, Want: true},
		{Name: "bat surface", Type: game.EntityBat, Floor: game.Stone},
		{Name: "bat wrong support", Type: game.EntityBat, Floor: game.GrassBlock, Roof: true},
		{Name: "cod water", Type: game.EntityCod, Floor: game.Water, Water: true, Want: true},
		{Name: "salmon water", Type: game.EntitySalmon, Floor: game.Water, Water: true, Want: true},
		{Name: "tropical water", Type: game.EntityTropicalFish, Floor: game.Water, Water: true, Want: true},
		{Name: "puffer water", Type: game.EntityPufferfish, Floor: game.Water, Water: true, Want: true},
		{Name: "fish dry", Type: game.EntityCod, Floor: game.Stone},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: test.Floor, Roof: test.Roof, Water: test.Water}, Lighting: game.LightingNormal, SeaLevel: -58})

			runtime.entityRandom = func() float32 {
				return 0
			}

			if test.Peaceful {
				runtime.Difficulty = game.DifficultyPeaceful
			}

			actual := runtime.naturalEnvironmentAllows(position, test.Type)
			if actual != test.Want {
				t.Fatalf("allowed=%t want=%t", actual, test.Want)
			}
		})
	}
}

func TestNaturalSpawnObstructionAndHeight(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.GrassBlock}})

	position := game.BlockPosition{X: 40, Y: -63}
	spawn := game.Position{X: 40.5, Y: -63, Z: .5}

	runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, spawn, game.Velocity{}, 0)
	runtime.SpawnArrow(spawn, game.Velocity{}, 0)

	if !runtime.naturalEnvironmentAllows(position, game.EntityCow) {
		t.Fatal("nonblocking entities obstructed spawning")
	}

	runtime.SpawnCow(spawn)

	if runtime.naturalEnvironmentAllows(position, game.EntityCow) {
		t.Fatal("living entity did not obstruct spawning")
	}

	position.X++

	runtime.World.SetBlock(game.BlockPosition{X: position.X, Y: -62}, game.Stone)

	if runtime.naturalEnvironmentAllows(position, game.EntityCow) {
		t.Fatal("full entity clearance ignored head block")
	}

	position.Y = 320

	if runtime.naturalEnvironmentAllows(position, game.EntityChicken) {
		t.Fatal("spawn above build height")
	}
}

func TestNaturalSpawnEligibilityAndCaps(t *testing.T) {
	runtime := NewRuntime(&game.World{Spawn: game.Position{X: 1000}})

	player := addRuntimeMobTestPlayer(t, runtime, game.Position{X: .5, Y: -63, Z: .5}, game.GameModeSurvival)

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 500}, game.GameModeSpectator)

	chunks := []LoadedChunk{{}, {X: 2}, {X: 7}, {X: 8}, {X: 9}, {X: 31}}

	runtime.setSessionActiveChunks(player, chunks)
	runtime.prepareNaturalSpawnState()

	state := &runtime.naturalSpawning
	if len(state.Players) != 1 || len(state.Area) != 289 || len(state.Chunks) != 3 {
		t.Fatalf("players=%d area=%d eligible=%v", len(state.Players), len(state.Area), state.Chunks)
	}

	positions := []structTestSpawnDistance{
		{X: 24}, {X: 32, Want: true}, {X: 80, Want: true}, {X: 128}, {X: 48, Category: naturalWaterAmbient, Want: true}, {X: 80, Category: naturalWaterAmbient},
	}

	for _, test := range positions {
		candidate := game.BlockPosition{X: test.X, Y: -63}

		if test.X < 128 {
			state.Eligible[LoadedChunk{X: test.X >> 4}] = struct{}{}
		}

		actual := runtime.naturalDistanceAllows(candidate, game.Position{X: float64(test.X) + .5, Y: -63, Z: .5}, test.Category)
		if actual != test.Want {
			t.Fatalf("distance %d category %d allowed=%t", test.X, test.Category, actual)
		}
	}

	for range 70 {
		runtime.SpawnZombie(game.Position{X: 40, Y: -63})
	}

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: .5, Y: -63, Z: .5}, game.GameModeSurvival)

	runtime.prepareNaturalSpawnState()

	if len(state.Area) != 289 || state.Counts[naturalMonster] != 70 || state.localCapAllows(LoadedChunk{X: 2}, naturalMonster) {
		t.Fatal("overlapping players multiplied the monster cap")
	}

	for _, playerState := range state.Players {
		if playerState.Counts[naturalMonster] != 70 {
			t.Fatal("mob not charged to every nearby player")
		}
	}
}

func TestNaturalSpawnPopulationRepeatableAndReplenishes(t *testing.T) {
	first := newSpawnPopulationTestRuntime(t, 73)
	second := newSpawnPopulationTestRuntime(t, 73)

	for tick := int64(1); tick <= 100; tick++ {
		first.tickNaturalSpawning(game.TimeState{Age: tick})
		second.tickNaturalSpawning(game.TimeState{Age: tick})
	}

	first.prepareNaturalSpawnState()

	count := first.naturalSpawning.Counts[naturalMonster]
	if count < 70 || count > 73 {
		t.Fatalf("100-tick monster population=%d, expected 70..73 (70 cap + cluster overshoot)", count)
	}

	t.Logf("100-tick population: %d monsters", count)

	firstEntities := first.snapshotRuntimeEntities()
	secondEntities := second.snapshotRuntimeEntities()

	if len(firstEntities) != len(secondEntities) {
		t.Fatal("deterministic RNG changed population count")
	}

	firstPositions := spawnPopulationPositions(firstEntities)
	secondPositions := spawnPopulationPositions(secondEntities)

	if !reflect.DeepEqual(firstPositions, secondPositions) {
		t.Fatal("deterministic RNG changed population positions/types")
	}

	for _, entity := range firstEntities {
		mob := entity.(RuntimeMobEntity)
		if mob.RuntimeMob().PersistenceRequired || mob.RuntimeMobRequiresCustomPersistence() {
			t.Fatal("natural mob became persistent")
		}

		if _, active := first.naturalSpawning.Eligible[entity.RuntimeEntityState().Chunk]; !active {
			t.Fatal("natural mob appeared in an inactive chunk")
		}
	}

	for _, session := range first.sessionView() {
		session.Player.Position.X = 1000

		first.setSessionActiveChunks(session, nil)
	}

	first.cleanupInactiveRuntimeMobs()

	if len(first.snapshotRuntimeEntities()) != 0 {
		t.Fatal("ordinary distance despawning did not release population")
	}

	for _, session := range first.sessionView() {
		session.Player.Position.X = 0.5

		first.setSessionActiveChunks(session, []LoadedChunk{{X: 2}, {X: 3}, {X: 4}, {X: 5}})
	}

	for tick := int64(101); tick <= 200; tick++ {
		first.tickNaturalSpawning(game.TimeState{Age: tick})
	}

	first.prepareNaturalSpawnState()

	if first.naturalSpawning.Counts[naturalMonster] < 70 || first.naturalSpawning.Counts[naturalMonster] > 73 {
		t.Fatal("population failed to replenish after despawning")
	}

	t.Logf("100-tick replenished population: %d monsters", first.naturalSpawning.Counts[naturalMonster])
}

func TestNaturalSpawnPeacefulPopulation(t *testing.T) {
	runtime := newSpawnPopulationTestRuntime(t, 17)

	runtime.Difficulty = game.DifficultyPeaceful

	for tick := int64(1); tick <= 100; tick++ {
		runtime.tickNaturalSpawning(game.TimeState{Age: tick})
	}

	runtime.prepareNaturalSpawnState()

	if runtime.naturalSpawning.Counts[naturalMonster] != 0 {
		t.Fatal("Peaceful produced natural hostiles")
	}
}

func TestNaturalSpawnBiomeAndPack(t *testing.T) {
	runtime := newSpawnPopulationTestRuntime(t, 37)
	if runtime.naturalBiomeAt(game.BlockPosition{}) != game.BiomePlains {
		t.Fatal("generator without biome provider did not use normal plains default")
	}

	monsterTypes := []game.EntityType{game.EntityZombie, game.EntitySkeleton, game.EntityCreeper}

	for _, entityType := range monsterTypes {
		found := false

		for _, entry := range naturalBiomeSpawns[game.BiomePlains][naturalMonster] {
			found = found || entry.Type == entityType
		}

		if !found {
			t.Fatal("plains missing supported monster")
		}
	}

	if len(naturalBiomeSpawns[game.BiomeDeepDark][naturalMonster]) != 0 || len(naturalBiomeSpawns[game.BiomeDesert][naturalCreature]) != 0 {
		t.Fatal("biome restrictions lost")
	}

	for range 100 {
		before := len(runtime.snapshotRuntimeEntities())

		runtime.prepareNaturalSpawnState()

		runtime.spawnNaturalPack(LoadedChunk{X: 3}, naturalMonster)

		after := len(runtime.snapshotRuntimeEntities())
		if after-before > 4 {
			t.Fatal("monster pack exceeded vanilla cluster limit")
		}
	}
}

func TestNaturalSpawnWaterHeightAndBiomeBoundary(t *testing.T) {
	position := game.BlockPosition{X: 40, Y: -63}

	runtime := NewRuntime(&game.World{Generator: spawnTestBiomeGenerator{spawnTestGenerator: spawnTestGenerator{Floor: game.Water, Water: true}, Biome: game.BiomeLushCaves}, SeaLevel: 63})

	if runtime.naturalEnvironmentAllows(position, game.EntityCod) {
		t.Fatal("cod ignored sea-level minus 13 lower boundary")
	}

	if !runtime.naturalEnvironmentAllows(position, game.EntityTropicalFish) {
		t.Fatal("lush cave tropical fish lost any-height biome exception")
	}

	runtime.World.SetBlock(game.BlockPosition{X: 40, Y: -62}, game.Air)

	if runtime.naturalEnvironmentAllows(position, game.EntityTropicalFish) {
		t.Fatal("fish accepted absent water block above")
	}

	farFish := runtime.SpawnCod(game.Position{X: 80, Y: -63})

	addRuntimeMobTestPlayer(t, runtime, game.Position{Y: -63}, game.GameModeSurvival)

	runtime.cleanupInactiveRuntimeMobs()

	if !farFish.State.Removed {
		t.Fatal("WATER_AMBIENT fish failed to despawn beyond source-derived 64 blocks")
	}
}

func TestNaturalSpawnLightCacheInvalidation(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})

	runtime.entityRandom = func() float32 {
		return 0
	}

	position := game.BlockPosition{X: 40, Y: -63}

	if !runtime.naturalEnvironmentAllows(position, game.EntityZombie) {
		t.Fatal("initial dark cave rejected")
	}

	runtime.World.SetBlock(game.BlockPosition{X: 41, Y: -63}, game.Glowstone)

	if runtime.naturalEnvironmentAllows(position, game.EntityZombie) {
		t.Fatal("cached light did not observe block edit in its halo")
	}

	if runtime.naturalEnvironmentAllows(position, game.EntityBat) {
		t.Fatal("bat ignored cave-light boundary")
	}

	query := runtime.naturalChunkAt(position)

	allocations := testing.AllocsPerRun(100, func() {
		runtime.naturalEnvironmentAllows(position, game.EntityZombie)
	})

	if allocations != 0 || len(query.Sky) == 0 {
		t.Fatalf("warm candidate allocations=%g, expected zero with prepared lighting", allocations)
	}
}

func TestNaturalSpawnSeparatedPlayersAndSpectatorDistance(t *testing.T) {
	runtime := NewRuntime(&game.World{Spawn: game.Position{X: 2000}})

	player := addRuntimeMobTestPlayer(t, runtime, game.Position{Y: -63}, game.GameModeSurvival)

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 40.5, Y: -63, Z: .5}, game.GameModeSpectator)

	runtime.setSessionActiveChunks(player, []LoadedChunk{{X: 2}})
	runtime.prepareNaturalSpawnState()

	if !runtime.naturalDistanceAllows(game.BlockPosition{X: 40, Y: -63}, game.Position{X: 40.5, Y: -63, Z: .5}, naturalMonster) {
		t.Fatal("nearby spectator incorrectly excluded a candidate")
	}

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 1000, Y: -63}, game.GameModeSurvival)

	for range 70 {
		runtime.SpawnZombie(game.Position{X: 40, Y: -63})
	}

	runtime.prepareNaturalSpawnState()

	if len(runtime.naturalSpawning.Area) != 578 || !runtime.naturalSpawning.localCapAllows(LoadedChunk{X: 62}, naturalMonster) || runtime.naturalSpawning.localCapAllows(LoadedChunk{X: 2}, naturalMonster) {
		t.Fatal("separated players lost independent local caps or union global area")
	}
}

func BenchmarkNaturalSpawnActiveAttempts(b *testing.B) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone}})

	session, _ := newMovementTestSession(runtime, randomEntityUUID(), "SpawnAttemptsBenchmark")

	session.Player.Position = game.Position{Y: -63}

	err := runtime.JoinSession(session)
	if err != nil {
		b.Fatal(err)
	}

	chunks := make([]LoadedChunk, 0, 289)

	for worldZ := int32(-8); worldZ <= 8; worldZ++ {
		for worldX := int32(-8); worldX <= 8; worldX++ {
			chunks = append(chunks, LoadedChunk{X: worldX, Z: worldZ})
		}
	}

	runtime.setSessionActiveChunks(session, chunks)

	random := rand.New(rand.NewPCG(29, 31))

	runtime.entityRandom = random.Float32

	for _, chunk := range chunks {
		for localZ := range int32(16) {
			for localX := range int32(16) {
				runtime.naturalSurfaceHeight(game.BlockPosition{X: chunk.X*16 + localX, Z: chunk.Z*16 + localZ})
			}
		}
	}

	runtime.tickNaturalSpawning(game.TimeState{Age: 1})

	b.ReportAllocs()

	for b.Loop() {
		runtime.tickNaturalSpawning(game.TimeState{Age: 1})
	}
}

func BenchmarkNaturalSpawnPrepareLight(b *testing.B) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})

	position := game.BlockPosition{X: 40, Y: -63}

	runtime.naturalLightAt(position)

	query := runtime.naturalChunkAt(position)

	b.ReportAllocs()

	for b.Loop() {
		query.Sky = query.Sky[:0]
		query.Block = query.Block[:0]

		runtime.naturalLightAt(position)
	}
}

func TestNaturalSpawnPassiveIntervalAndCategoryCaps(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.GrassBlock}, Spawn: game.Position{X: 1000}})

	player := addRuntimeMobTestPlayer(t, runtime, game.Position{X: .5, Y: -63, Z: .5}, game.GameModeSurvival)

	runtime.setSessionActiveChunks(player, []LoadedChunk{{X: 2}, {X: 3}, {X: 4}, {X: 5}})

	random := rand.New(rand.NewPCG(47, 53))

	runtime.entityRandom = random.Float32

	for tick := int64(1); tick < 400; tick++ {
		runtime.tickNaturalSpawning(game.TimeState{Age: tick})
	}

	if len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("CREATURE spawned before the 400-tick persistent-category interval")
	}

	for tick := int64(400); tick <= 4000; tick += 400 {
		runtime.tickNaturalSpawning(game.TimeState{Age: tick})
	}

	runtime.prepareNaturalSpawnState()

	count := runtime.naturalSpawning.Counts[naturalCreature]
	if count < 10 || count > 13 {
		t.Fatalf("creature count=%d, expected cap 10 plus at most 3 cluster overshoot", count)
	}

	expectedCaps := [4]int{70, 10, 15, 20}

	for category, expectedCap := range expectedCaps {
		if naturalCategories[category].Cap != expectedCap {
			t.Fatalf("category %d cap differs from pinned MobCategory", category)
		}

		runtime.naturalSpawning.Players[0].Counts[category] = expectedCap

		if runtime.naturalSpawning.localCapAllows(LoadedChunk{X: 3}, category) {
			t.Fatalf("category %d ignored source cap %d", category, expectedCap)
		}
	}
}

func TestNaturalSpawnWorldSpawnAndPersistentExclusion(t *testing.T) {
	runtime := newSpawnPopulationTestRuntime(t, 71)

	runtime.prepareNaturalSpawnState()

	candidate := game.BlockPosition{X: 40, Y: -63}
	position := game.Position{X: 40.5, Y: -63, Z: .5}

	runtime.World.Spawn = position

	if runtime.naturalDistanceAllows(candidate, position, naturalMonster) {
		t.Fatal("candidate inside world-respawn 24-block exclusion was accepted")
	}

	runtime.World.Spawn.X = 1000

	if !runtime.naturalDistanceAllows(candidate, position, naturalMonster) {
		t.Fatal("valid candidate outside world-respawn exclusion was rejected")
	}

	mob := runtime.SpawnZombie(position)

	mob.RuntimeMob().PersistenceRequired = true

	runtime.prepareNaturalSpawnState()

	if runtime.naturalSpawning.Counts[naturalMonster] != 0 {
		t.Fatal("persistent mob consumed natural category cap")
	}

	mob.RuntimeMob().PersistenceRequired = false

	runtime.prepareNaturalSpawnState()

	if runtime.naturalSpawning.Counts[naturalMonster] != 1 {
		t.Fatal("ordinary mob did not consume natural category cap")
	}
}

func TestNaturalSpawnSourcePotentialCost(t *testing.T) {
	entry := naturalSpawnEntry{Type: game.EntitySkeleton, Charge: .7, Budget: .15}
	state := naturalSpawnState{Charges: []naturalSpawnCharge{{Position: game.Position{}, Charge: .7}}}

	if state.potentialAllows(game.Position{X: .9}, entry) || state.potentialAllows(game.Position{X: 3.9}, entry) {
		t.Fatal("coincident/three-block skeleton potential exceeded source budget but was accepted")
	}

	if !state.potentialAllows(game.Position{X: 4.1}, entry) {
		t.Fatal("four-block skeleton potential below source budget was rejected")
	}
}

func TestNaturalSpawnColdLightingBudget(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})

	state := &runtime.naturalSpawning

	state.InTick = true
	state.LightPreparations = 1

	_, _, first := runtime.naturalLightAt(game.BlockPosition{X: 40, Y: -63})
	_, _, deferred := runtime.naturalLightAt(game.BlockPosition{X: 80, Y: -63})
	_, _, cached := runtime.naturalLightAt(game.BlockPosition{X: 41, Y: -63})

	if !first || deferred || !cached || state.LightPreparations != 0 {
		t.Fatal("cold lighting budget guessed/degraded light or prevented a warm cache query")
	}

	state.LightPreparations = 1

	_, _, resumed := runtime.naturalLightAt(game.BlockPosition{X: 80, Y: -63})
	if !resumed {
		t.Fatal("deferred light query did not prepare on a later tick budget")
	}
}

func TestNaturalSpawnAquaticPackLimits(t *testing.T) {
	biomes := []game.Biome{game.BiomeWarmOcean, game.BiomeFrozenOcean, game.BiomeOcean}

	for _, biome := range biomes {
		runtime := NewRuntime(&game.World{Generator: spawnTestBiomeGenerator{spawnTestGenerator: spawnTestGenerator{Floor: game.Water, Water: true}, Biome: biome}, SeaLevel: -58, Spawn: game.Position{X: 1000}})

		player := addRuntimeMobTestPlayer(t, runtime, game.Position{Y: -63}, game.GameModeSurvival)

		runtime.setSessionActiveChunks(player, []LoadedChunk{{X: 2}, {X: 3}})

		random := rand.New(rand.NewPCG(uint64(biome)+71, 79))

		runtime.entityRandom = random.Float32

		spawned := 0

		for range 100 {
			runtime.prepareNaturalSpawnState()

			before := len(runtime.snapshotRuntimeEntities())

			runtime.spawnNaturalPack(LoadedChunk{X: 2}, naturalWaterAmbient)

			growth := len(runtime.snapshotRuntimeEntities()) - before
			limit := 8

			if biome == game.BiomeFrozenOcean {
				limit = 5
			}

			if growth > limit {
				t.Fatalf("biome %d fish cluster=%d exceeds %d", biome, growth, limit)
			}

			spawned += growth
		}

		if spawned == 0 {
			t.Fatalf("biome %d produced no supported fish", biome)
		}
	}
}

func BenchmarkNaturalSpawnActiveRegion(b *testing.B) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})

	session, _ := newMovementTestSession(runtime, randomEntityUUID(), "SpawnBenchmark")

	session.Player.Position = game.Position{X: .5, Y: -63, Z: .5}

	err := runtime.JoinSession(session)
	if err != nil {
		b.Fatal(err)
	}

	chunks := make([]LoadedChunk, 0, 289)

	for worldZ := int32(-8); worldZ <= 8; worldZ++ {
		for worldX := int32(-8); worldX <= 8; worldX++ {
			chunks = append(chunks, LoadedChunk{X: worldX, Z: worldZ})
		}
	}

	runtime.setSessionActiveChunks(session, chunks)

	random := rand.New(rand.NewPCG(19, 23))

	runtime.entityRandom = random.Float32

	for range 70 {
		runtime.SpawnZombie(game.Position{Y: -63})
	}

	for range 15 {
		runtime.SpawnBat(game.Position{Y: -63})
	}

	runtime.tickNaturalSpawning(game.TimeState{Age: 1})

	b.ReportAllocs()

	for b.Loop() {
		runtime.tickNaturalSpawning(game.TimeState{Age: 1})
	}
}

func BenchmarkNaturalSpawnCandidate(b *testing.B) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal})

	runtime.entityRandom = func() float32 {
		return 0
	}

	position := game.BlockPosition{X: 40, Y: -63}

	runtime.naturalEnvironmentAllows(position, game.EntityZombie)

	b.ReportAllocs()

	for b.Loop() {
		runtime.naturalEnvironmentAllows(position, game.EntityZombie)
	}
}

func newSpawnPopulationTestRuntime(t *testing.T, seed uint64) *Runtime {
	t.Helper()

	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}, Lighting: game.LightingNormal, Spawn: game.Position{X: 1000}})

	player := addRuntimeMobTestPlayer(t, runtime, game.Position{X: .5, Y: -63, Z: .5}, game.GameModeSurvival)

	chunks := []LoadedChunk{{X: 2}, {X: 3}, {X: 4}, {X: 5}}

	runtime.setSessionActiveChunks(player, chunks)

	random := rand.New(rand.NewPCG(seed, 91))

	runtime.entityRandom = random.Float32

	return runtime
}

func spawnPopulationPositions(entities []RuntimeEntity) map[game.Position]game.EntityType {
	positions := make(map[game.Position]game.EntityType, len(entities))

	for _, entity := range entities {
		state := entity.RuntimeEntityState()

		positions[state.Position] = state.Type
	}

	return positions
}
