package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type pufferfishDifficultyDamageTest struct {
	Name       string
	Difficulty game.Difficulty
	Health     float32
	Poisoned   bool
}

func TestPufferfishPuffStateExactTiming(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	pufferfish := runtime.SpawnPufferfish(game.Position{})

	pufferfish.InflateCounter = 1

	pufferfish.tickPuffState(runtime)

	assertPufferfishState(t, pufferfish, pufferfishMidState, 2, 0)

	for range 39 {
		pufferfish.tickPuffState(runtime)
	}

	assertPufferfishState(t, pufferfish, pufferfishMidState, 41, 0)

	pufferfish.tickPuffState(runtime)

	assertPufferfishState(t, pufferfish, pufferfishFullState, 42, 0)

	pufferfish.InflateCounter = 0

	for range 61 {
		pufferfish.tickPuffState(runtime)
	}

	assertPufferfishState(t, pufferfish, pufferfishFullState, 0, 61)

	pufferfish.tickPuffState(runtime)

	assertPufferfishState(t, pufferfish, pufferfishMidState, 0, 62)

	for range 39 {
		pufferfish.tickPuffState(runtime)
	}

	assertPufferfishState(t, pufferfish, pufferfishMidState, 0, 101)

	pufferfish.tickPuffState(runtime)

	assertPufferfishState(t, pufferfish, pufferfishSmallState, 0, 102)
}

func TestPufferfishProximityStartsInflationAndLeavingDeflates(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	pufferfish := runtime.SpawnPufferfish(game.Position{})
	cow := runtime.SpawnCow(game.Position{X: 2})

	goal := pufferfishInflateGoal{Fish: pufferfish}

	if !goal.CanUse(runtime) {
		t.Fatal("nearby scary mob did not start inflation")
	}

	goal.Start(runtime)

	if pufferfish.PuffState != pufferfishSmallState {
		t.Fatal("inflate goal changed state before the next entity tick")
	}

	pufferfish.tickPuffState(runtime)

	if pufferfish.PuffState != pufferfishMidState {
		t.Fatalf("state after next tick = %d, want mid", pufferfish.PuffState)
	}

	cow.State.mu.Lock()
	cow.State.Position.X = 10
	cow.State.mu.Unlock()

	if goal.CanContinue(runtime) {
		t.Fatal("distant mob kept inflate goal active")
	}

	goal.Stop(runtime)

	for range 102 {
		pufferfish.tickPuffState(runtime)
	}

	if pufferfish.PuffState != pufferfishSmallState {
		t.Fatalf("state after leaving = %d, want small", pufferfish.PuffState)
	}
}

func TestPufferfishIgnoresFishInNotScaryTag(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	pufferfish := runtime.SpawnPufferfish(game.Position{})

	runtime.SpawnCod(game.Position{X: 1})
	runtime.SpawnSalmon(game.Position{X: 1})
	runtime.SpawnTropicalFish(game.Position{X: 1})
	runtime.SpawnPufferfish(game.Position{X: 1})

	goal := pufferfishInflateGoal{Fish: pufferfish}
	if goal.CanUse(runtime) {
		t.Fatal("fish from not-scary tag triggered inflation")
	}
}

func TestPufferfishStateUpdatesDimensionsAndMetadata(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	pufferfish := runtime.SpawnPufferfish(game.Position{})

	pufferfish.State.mu.Lock()
	pufferfish.State.metadataDirty = false

	pufferfish.setPuffStateLocked(pufferfishMidState)

	dirty := pufferfish.State.metadataDirty
	pufferfish.State.mu.Unlock()

	midDimension := 0.7 * pufferfishMidDimensionScale
	midEyeHeight := pufferfishBaseEyeHeight * pufferfishMidDimensionScale

	if !dirty || math.Abs(pufferfish.Living.Width-midDimension) > 1e-12 || math.Abs(pufferfish.Living.Height-midDimension) > 1e-12 || math.Abs(pufferfish.RuntimeLivingEyeHeight()-midEyeHeight) > 1e-12 {
		t.Fatalf("mid state = dirty %t dimensions %vx%v eye %v", dirty, pufferfish.Living.Width, pufferfish.Living.Height, pufferfish.RuntimeLivingEyeHeight())
	}

	metadata := pufferfish.EntityMetadata()
	want := protocol.EntityMetadataEntry{Index: protocol.PufferfishStateMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(pufferfishMidState)}

	if metadata[len(metadata)-1] != want {
		t.Fatalf("puff metadata = %+v, want %+v", metadata[len(metadata)-1], want)
	}

	pufferfish.State.mu.Lock()
	pufferfish.setPuffStateLocked(pufferfishFullState)
	pufferfish.State.mu.Unlock()

	if pufferfish.Living.Width != 0.7 || pufferfish.Living.Height != 0.7 || pufferfish.RuntimeLivingEyeHeight() != pufferfishBaseEyeHeight {
		t.Fatalf("full dimensions = %vx%v eye %v", pufferfish.Living.Width, pufferfish.Living.Height, pufferfish.RuntimeLivingEyeHeight())
	}
}

func TestPufferfishContactDamagesAndPoisonsRuntimeMob(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	pufferfish := runtime.SpawnPufferfish(game.Position{})
	cow := runtime.SpawnCow(game.Position{})

	pufferfish.State.mu.Lock()
	pufferfish.setPuffStateLocked(pufferfishFullState)
	pufferfish.State.mu.Unlock()

	pufferfish.stingNearbyMobs(runtime)

	poison, poisoned := cow.Living.ActiveEffects.Find(game.MobEffectPoison)
	if cow.Living.Health != 7 || !poisoned || poison.Duration != 120 || poison.Amplifier != 0 || poison.Ambient || !poison.Visible || !poison.ShowIcon {
		t.Fatalf("stung cow = health %v poison %+v present %t", cow.Living.Health, poison, poisoned)
	}
}

func TestPufferfishContactDamagesAndPoisonsPlayer(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.Difficulty = game.DifficultyNormal

	session := addRuntimeMobTestPlayer(t, runtime, game.Position{}, game.GameModeSurvival)

	pufferfish := runtime.SpawnPufferfish(game.Position{})

	pufferfish.State.mu.Lock()
	pufferfish.setPuffStateLocked(pufferfishMidState)
	pufferfish.State.mu.Unlock()

	pufferfish.stingTouchingPlayers(runtime)

	player := session.snapshotPlayer()

	poison, poisoned := player.ActiveEffects.Find(game.MobEffectPoison)

	if player.Health != 18 || !poisoned || poison.Duration != 60 || poison.Amplifier != 0 {
		t.Fatalf("stung player = health %v poison %+v present %t", player.Health, poison, poisoned)
	}
}

func TestPufferfishPlayerContactScalesWithDifficulty(t *testing.T) {
	tests := []pufferfishDifficultyDamageTest{
		{Name: "peaceful", Difficulty: game.DifficultyPeaceful, Health: 20},
		{Name: "easy", Difficulty: game.DifficultyEasy, Health: 17.5, Poisoned: true},
		{Name: "normal", Difficulty: game.DifficultyNormal, Health: 17, Poisoned: true},
		{Name: "hard", Difficulty: game.DifficultyHard, Health: 15.5, Poisoned: true},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			runtime.Difficulty = test.Difficulty

			session := addRuntimeMobTestPlayer(t, runtime, game.Position{}, game.GameModeSurvival)

			session.Player.ResetSurvivalState()

			pufferfish := runtime.SpawnPufferfish(game.Position{})

			pufferfish.State.mu.Lock()
			pufferfish.setPuffStateLocked(pufferfishFullState)
			pufferfish.State.mu.Unlock()

			pufferfish.stingTouchingPlayers(runtime)

			player := session.snapshotPlayer()
			_, poisoned := player.ActiveEffects.Find(game.MobEffectPoison)

			if player.Health != test.Health || poisoned != test.Poisoned {
				t.Fatalf("player after contact = health %v poisoned %t, want %v/%t", player.Health, poisoned, test.Health, test.Poisoned)
			}
		})
	}
}

func TestPufferfishDoesNotImplementSchoolingCapability(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	pufferfish := runtime.SpawnPufferfish(game.Position{})

	_, schools := any(pufferfish).(schoolingFish)
	if schools {
		t.Fatal("pufferfish implements schooling capability")
	}

	for _, entry := range pufferfish.Goals.Entries {
		_, followsSchool := entry.Goal.(*aquaticFollowSchoolGoal)
		if followsSchool {
			t.Fatal("pufferfish has a follow-school goal")
		}
	}
}

func TestPufferfishDryOutFlopAndInactiveChunkPause(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0.5
	}

	session := addRuntimeMobTestPlayer(t, runtime, game.Position{X: 100}, game.GameModeSurvival)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	pufferfish := runtime.SpawnPufferfish(game.Position{X: 0.5, Y: 1, Z: 0.5})

	pufferfish.Living.OnGround = true
	pufferfish.VerticalCollision = true
	pufferfish.InflateCounter = 1

	runtime.Tick()

	if pufferfish.TickCount != 1 || pufferfish.AirSupply != aquaticMaximumAirSupply-1 || pufferfish.PuffState != pufferfishMidState || pufferfish.State.Position.Y != 1.4 {
		t.Fatalf("active dry pufferfish = ticks %d air %d state %d position %+v", pufferfish.TickCount, pufferfish.AirSupply, pufferfish.PuffState, pufferfish.State.Position)
	}

	position := pufferfish.State.Position
	velocity := pufferfish.Living.Velocity
	airSupply := pufferfish.AirSupply
	inflateCounter := pufferfish.InflateCounter

	runtime.releaseSessionActiveChunks(session)

	runtime.Tick()

	if pufferfish.TickCount != 1 || pufferfish.State.Position != position || pufferfish.Living.Velocity != velocity || pufferfish.AirSupply != airSupply || pufferfish.InflateCounter != inflateCounter {
		t.Fatal("inactive pufferfish advanced movement, air, or puff state")
	}
}

func TestWarmedAquaticSwimmingTicksHaveNearZeroAllocations(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Water}}

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	cod := runtime.SpawnCod(game.Position{X: 0.5, Y: 10, Z: 0.5})
	pufferfish := runtime.SpawnPufferfish(game.Position{X: 20.5, Y: 10, Z: 0.5})

	cod.FromBucket = true
	pufferfish.FromBucket = true

	for range 40 {
		cod.Tick(runtime, nil)

		pufferfish.Tick(runtime, nil)
	}

	codAllocations := testing.AllocsPerRun(100, func() {
		cod.Tick(runtime, nil)
	})

	pufferfishAllocations := testing.AllocsPerRun(100, func() {
		pufferfish.Tick(runtime, nil)
	})

	if codAllocations > 0.1 || pufferfishAllocations > 0.1 {
		t.Fatalf("warmed allocations = cod %.2f pufferfish %.2f", codAllocations, pufferfishAllocations)
	}
}

func assertPufferfishState(t *testing.T, pufferfish *runtimePufferfishEntity, state, inflateCounter, deflateTimer int32) {
	t.Helper()

	if pufferfish.PuffState != state || pufferfish.InflateCounter != inflateCounter || pufferfish.DeflateTimer != deflateTimer {
		t.Fatalf("pufferfish state = %d inflate %d deflate %d, want %d/%d/%d", pufferfish.PuffState, pufferfish.InflateCounter, pufferfish.DeflateTimer, state, inflateCounter, deflateTimer)
	}
}
