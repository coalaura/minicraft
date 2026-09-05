package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type runtimeMobSoftDespawnTestCase struct {
	name             string
	noActionTime     int32
	random           float32
	wantRemoved      bool
	wantRandomCalls  int
	wantNoActionTime int32
}

type runtimeMobPlayerModeTestCase struct {
	name        string
	gameMode    game.GameMode
	wantRemoved bool
}

type inactiveRuntimeMobTestCase struct {
	name           string
	playerPosition *game.Position
	persistent     bool
	wantRemoved    bool
}

func TestRuntimeMobSoftDespawnThresholdAndRandomBranches(t *testing.T) {
	tests := []runtimeMobSoftDespawnTestCase{
		{name: "before threshold", noActionTime: 599, random: 0, wantNoActionTime: 599},
		{name: "at threshold", noActionTime: 600, random: 0, wantNoActionTime: 600},
		{name: "after threshold rejected", noActionTime: 601, random: 0.5, wantRandomCalls: 1, wantNoActionTime: 601},
		{name: "after threshold accepted", noActionTime: 601, random: 0, wantRemoved: true, wantRandomCalls: 1, wantNoActionTime: 601},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			addRuntimeMobTestPlayer(t, runtime, game.Position{X: 33}, game.GameModeSurvival)

			zombie := runtime.SpawnZombie(game.Position{})
			zombie.NoActionTime = test.noActionTime

			randomCalls := 0

			runtime.entityRandom = func() float32 {
				randomCalls++

				return test.random
			}

			removed := runtime.checkRuntimeMobDespawn(zombie)
			if removed != test.wantRemoved || zombie.State.Removed != test.wantRemoved {
				t.Fatalf("removed = %t state removed = %t, want %t", removed, zombie.State.Removed, test.wantRemoved)
			}

			if randomCalls != test.wantRandomCalls {
				t.Fatalf("random calls = %d, want %d", randomCalls, test.wantRandomCalls)
			}

			if zombie.NoActionTime != test.wantNoActionTime {
				t.Fatalf("no action time = %d, want %d", zombie.NoActionTime, test.wantNoActionTime)
			}
		})
	}
}

func TestRuntimeMobNoDespawnDistanceBoundariesAndReset(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session := addRuntimeMobTestPlayer(t, runtime, game.Position{X: runtimeMobNoDespawnDistance}, game.GameModeSurvival)

	zombie := runtime.SpawnZombie(game.Position{})

	zombie.NoActionTime = runtimeMobDespawnDelay + 1

	randomCalls := 0

	runtime.entityRandom = func() float32 {
		randomCalls++

		return 0
	}

	if runtime.checkRuntimeMobDespawn(zombie) || zombie.State.Removed {
		t.Fatal("zombie despawned exactly at the no-despawn boundary")
	}

	if zombie.NoActionTime != runtimeMobDespawnDelay+1 || randomCalls != 1 {
		t.Fatalf("boundary state = no action %d random calls %d", zombie.NoActionTime, randomCalls)
	}

	session.Player.Position.X = runtimeMobNoDespawnDistance - 1

	if runtime.checkRuntimeMobDespawn(zombie) || zombie.NoActionTime != 0 {
		t.Fatalf("near zombie state = removed %t no action %d", zombie.State.Removed, zombie.NoActionTime)
	}

	if randomCalls != 2 {
		t.Fatalf("near reset random calls = %d, want 2", randomCalls)
	}
}

func TestRuntimeMobImmediateDespawnUsesStrictThreeDimensionalDistance(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session := addRuntimeMobTestPlayer(t, runtime, game.Position{X: runtimeMobDespawnDistance}, game.GameModeSurvival)

	boundary := runtime.SpawnZombie(game.Position{})
	if runtime.checkRuntimeMobDespawn(boundary) || boundary.State.Removed {
		t.Fatal("zombie despawned exactly at the immediate boundary")
	}

	session.Player.Position = game.Position{X: runtimeMobDespawnDistance + 0.01}

	beyond := runtime.SpawnZombie(game.Position{})
	if !runtime.checkRuntimeMobDespawn(beyond) || !beyond.State.Removed {
		t.Fatal("zombie remained beyond the immediate boundary")
	}

	session.Player.Position = game.Position{Y: runtimeMobDespawnDistance + 1}

	vertical := runtime.SpawnZombie(game.Position{})
	if !runtime.checkRuntimeMobDespawn(vertical) || !vertical.State.Removed {
		t.Fatal("vertical distance did not cause immediate despawn")
	}
}

func TestRuntimeMobNearestNonSpectatorPlayerControlsDespawn(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 200}, game.GameModeSurvival)
	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 20}, game.GameModeCreative)

	zombie := runtime.SpawnZombie(game.Position{})
	zombie.NoActionTime = runtimeMobDespawnDelay + 1

	runtime.entityRandom = func() float32 {
		return 0
	}

	if runtime.checkRuntimeMobDespawn(zombie) || zombie.State.Removed || zombie.NoActionTime != 0 {
		t.Fatalf("nearest player state = removed %t no action %d", zombie.State.Removed, zombie.NoActionTime)
	}
}

func TestRuntimeMobCreativeAndSpectatorDespawnSelection(t *testing.T) {
	tests := []runtimeMobPlayerModeTestCase{
		{name: "survival", gameMode: game.GameModeSurvival},
		{name: "creative", gameMode: game.GameModeCreative},
		{name: "spectator", gameMode: game.GameModeSpectator, wantRemoved: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			addRuntimeMobTestPlayer(t, runtime, game.Position{X: 10}, test.gameMode)
			addRuntimeMobTestPlayer(t, runtime, game.Position{X: 200}, game.GameModeSurvival)

			zombie := runtime.SpawnZombie(game.Position{})
			runtime.checkRuntimeMobDespawn(zombie)

			if zombie.State.Removed != test.wantRemoved {
				t.Fatalf("removed = %t, want %t", zombie.State.Removed, test.wantRemoved)
			}
		})
	}

	runtime := NewRuntime(&game.World{})

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 200}, game.GameModeSpectator)

	zombie := runtime.SpawnZombie(game.Position{})
	if runtime.checkRuntimeMobDespawn(zombie) || zombie.State.Removed {
		t.Fatal("spectator-only runtime behaved as though a despawn player was present")
	}
}

func TestRuntimeMobActiveTickAccumulatesAndResetsNoActionTime(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session := addRuntimeMobTestPlayer(t, runtime, game.Position{X: 40}, game.GameModeSurvival)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	runtime.entityRandom = func() float32 {
		return 1
	}

	zombie := runtime.SpawnZombie(game.Position{})

	runtime.Tick()

	if zombie.NoActionTime != 3 {
		t.Fatalf("bright active no action time = %d, want 3", zombie.NoActionTime)
	}

	zombie.NoActionTime = 100
	session.Player.Position = game.Position{X: 10}

	runtime.Tick()

	if zombie.NoActionTime != 3 {
		t.Fatalf("near active no action time = %d, want reset then bright increment to 3", zombie.NoActionTime)
	}
}

func TestRuntimeMobPersistenceAndPeacefulOrdering(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 200}, game.GameModeSurvival)

	zombie := runtime.SpawnZombie(game.Position{})
	if zombie.PersistenceRequired {
		t.Fatal("ordinary summoned zombie was persistence-required")
	}

	zombie.PersistenceRequired = true
	zombie.NoActionTime = 100

	if runtime.checkRuntimeMobDespawn(zombie) || zombie.State.Removed || zombie.NoActionTime != 0 {
		t.Fatalf("persistent zombie state = removed %t no action %d", zombie.State.Removed, zombie.NoActionTime)
	}

	runtime.Difficulty = game.DifficultyPeaceful

	if !runtime.checkRuntimeMobDespawn(zombie) || !zombie.State.Removed {
		t.Fatal("persistence prevented Peaceful removal")
	}
}

func TestInactiveRuntimeMobCleanupOnlyRemovesFarDespawnableMobs(t *testing.T) {
	near := game.Position{X: runtimeMobDespawnDistance}
	far := game.Position{X: runtimeMobDespawnDistance + 1}

	tests := []inactiveRuntimeMobTestCase{
		{name: "no players"},
		{name: "inside retention boundary", playerPosition: &near},
		{name: "far persistent", playerPosition: &far, persistent: true},
		{name: "far despawnable", playerPosition: &far, wantRemoved: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			runtime.entityRandom = func() float32 {
				t.Fatal("inactive mob cleanup consumed entity randomness")

				return 0
			}

			if test.playerPosition != nil {
				addRuntimeMobTestPlayer(t, runtime, *test.playerPosition, game.GameModeSurvival)
			}

			zombie := runtime.SpawnZombie(game.Position{})

			zombie.NoActionTime = runtimeMobDespawnDelay + 1
			zombie.PersistenceRequired = test.persistent

			runtime.Tick()

			if zombie.State.Removed != test.wantRemoved {
				t.Fatalf("removed = %t, want %t", zombie.State.Removed, test.wantRemoved)
			}

			if zombie.NoActionTime != runtimeMobDespawnDelay+1 {
				t.Fatalf("inactive no action time = %d, want %d", zombie.NoActionTime, runtimeMobDespawnDelay+1)
			}
		})
	}
}

func TestRuntimeMobDespawnUsesOrdinaryRemovalWithoutDeathEffects(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer")

	session.Player.Position = game.Position{X: 40}
	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	zombie := runtime.SpawnZombie(game.Position{})

	zombie.NoActionTime = runtimeMobDespawnDelay + 1

	runtime.entityRandom = func() float32 {
		return 0
	}

	connection.reset()
	runtime.Tick()

	if !zombie.State.Removed || zombie.Living.Dead || zombie.Living.DeathTime != 0 || zombie.LootDropped {
		t.Fatalf("despawn lifecycle = removed %t dead %t death time %d loot %t", zombie.State.Removed, zombie.Living.Dead, zombie.Living.DeathTime, zombie.LootDropped)
	}

	if len(runtime.snapshotRuntimeEntities()) != 0 || len(runtime.snapshotEntitiesInChunk(LoadedChunk{})) != 0 || session.tracksRuntimeEntity(zombie.State.ID) {
		t.Fatal("despawn left authoritative, chunk-indexed, or viewer-tracked state")
	}

	chunk, active := runtime.ActiveChunk(LoadedChunk{})
	if !active || chunk.EntityCount() != 0 {
		t.Fatal("despawn left active-chunk membership")
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})
}

func addRuntimeMobTestPlayer(t *testing.T, runtime *Runtime, position game.Position, mode game.GameMode) *Session {
	t.Helper()

	session, _ := newMovementTestSession(runtime, randomEntityUUID(), "DespawnPlayer")

	session.Player.Position = position
	session.Player.GameMode = mode

	joinTestSession(t, runtime, session)

	return session
}
