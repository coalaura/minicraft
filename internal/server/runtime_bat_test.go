package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type batFlightTrace struct {
	position game.Position
	velocity game.Velocity
}

func TestBatSpawnMetadataDimensionsAndTracking(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	entity, spawned := runtime.SpawnEntity(game.EntityBat, game.Position{X: 0.5, Y: 2, Z: 0.5})
	if !spawned {
		t.Fatal("registered bat did not spawn")
	}

	bat, valid := entity.(*runtimeBatEntity)
	if !valid {
		t.Fatalf("registered bat type = %T", entity)
	}

	metadata := bat.EntityMetadata()

	wantFlags := protocol.EntityMetadataEntry{Index: protocol.BatFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(batRestingFlag)}
	if len(metadata) != 4 || metadata[3] != wantFlags {
		t.Fatalf("bat metadata = %+v, want resting flags %+v", metadata, wantFlags)
	}

	if bat.Living.Health != batMaxHealth || bat.Living.Width != 0.5 || bat.Living.Height != 0.9 || bat.RuntimeLivingEyeHeight() != batEyeHeight {
		t.Fatalf("bat attributes = health %v dimensions %v x %v eye %v", bat.Living.Health, bat.Living.Width, bat.Living.Height, bat.RuntimeLivingEyeHeight())
	}

	tracking := bat.RuntimeEntityTrackingConfig()
	if tracking.ClientRangeChunks != batTrackingRangeChunks || tracking.UpdateInterval != batTrackingInterval || tracking.TrackDeltas {
		t.Fatalf("bat tracking = %+v", tracking)
	}

	hurtSound, hurtVolume, hurtPitch := bat.RuntimeLivingDamageSound(false)
	deathSound, deathVolume, deathPitch := bat.RuntimeLivingDamageSound(true)

	if hurtSound != game.SoundEntityBatHurt || deathSound != game.SoundEntityBatDeath || hurtVolume != 0.1 || deathVolume != 0.1 || hurtPitch != 0.95 || deathPitch != 0.95 {
		t.Fatalf("bat damage sounds = hurt %q/%v/%v death %q/%v/%v", hurtSound, hurtVolume, hurtPitch, deathSound, deathVolume, deathPitch)
	}

	if bat.RuntimeEntitySoundSource() != protocol.SoundSourceNeutral {
		t.Fatalf("bat sound source = %d, want neutral", bat.RuntimeEntitySoundSource())
	}
}

func TestBatHangsUnderValidCeiling(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{Y: 3}, game.Stone)

	runtime, bat := activeBatTestRuntime(world, game.Position{X: 0.5, Y: 2, Z: 0.5})
	runtime.Tick()

	if !bat.Resting || bat.State.Position != (game.Position{X: 0.5, Y: 2.1, Z: 0.5}) || bat.Living.Velocity != (game.Velocity{}) {
		t.Fatalf("hanging bat = resting %t position %+v velocity %+v", bat.Resting, bat.State.Position, bat.Living.Velocity)
	}
}

func TestBatInvalidCeilingWakes(t *testing.T) {
	runtime, bat := activeBatTestRuntime(&game.World{}, game.Position{X: 0.5, Y: 2, Z: 0.5})

	runtime.Tick()

	assertBatAwakeMetadata(t, bat)
}

func TestNearbyPlayerWakesRestingBat(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{Y: 3}, game.Stone)

	runtime := NewRuntime(world)

	player := addRuntimeMobTestPlayer(t, runtime, game.Position{X: 0.5, Y: 2, Z: 0.5}, game.GameModeSurvival)

	runtime.setSessionActiveChunks(player, []LoadedChunk{{}})

	bat := runtime.SpawnBat(game.Position{X: 0.5, Y: 2, Z: 0.5})

	runtime.entityRandom = fixedBatRandom

	runtime.Tick()

	assertBatAwakeMetadata(t, bat)
}

func TestBatDamageAttemptWakesBeforeHurtCooldownRejection(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bat := runtime.SpawnBat(game.Position{})

	_, applied := runtime.damageRuntimeLivingEntityLocked(bat, game.Damage{Type: game.DamageGeneric, Amount: 1})
	if !applied || bat.Resting {
		t.Fatalf("first damage = applied %t resting %t", applied, bat.Resting)
	}

	bat.State.mu.Lock()
	bat.setRestingLocked(true)
	bat.State.mu.Unlock()

	_, applied = runtime.damageRuntimeLivingEntityLocked(bat, game.Damage{Type: game.DamageGeneric, Amount: 1})
	if applied || bat.Resting {
		t.Fatalf("cooldown damage = applied %t resting %t, want rejected and awake", applied, bat.Resting)
	}
}

func TestBatDeterministicFreeFlightTrace(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bat := runtime.SpawnBat(game.Position{X: 0.5, Y: 10, Z: 0.5})

	prepareFlyingBat(bat, game.BlockPosition{X: 10, Y: 15, Z: 10})

	runtime.entityRandom = fixedBatRandom

	want := []batFlightTrace{
		{position: game.Position{X: 0.5570710675327987, Y: 10.07, Z: 0.5570710675327987}, velocity: game.Velocity{X: 0.05193467145484675, Y: -0.005880000000000005, Z: 0.05193467145484675}},
		{position: game.Position{X: 0.6608833393749594, Y: 10.134708, Z: 0.6608833393749594}, velocity: game.Velocity{X: 0.09446916737636625, Y: -0.008991696000000007, Z: 0.09446916737636625}},
		{position: game.Position{X: 0.8029766575464876, Y: 10.1966154736, Z: 0.8029766575464876}, velocity: game.Velocity{X: 0.12930491953609072, Y: -0.0106384055232, Z: 0.12930491953609072}},
		{position: game.Position{X: 0.9764221526617679, Y: 10.25704090862912, Z: 0.9764221526617679}, velocity: game.Velocity{X: 0.15783540055490508, Y: -0.011509844202877442, Z: 0.15783540055490508}},
	}

	for tick, expected := range want {
		bat.Tick(runtime, nil)

		assertBatPositionClose(t, tick+1, bat.State.Position, expected.position)
		assertBatVelocityClose(t, tick+1, bat.Living.Velocity, expected.velocity)
	}
}

func TestBatFreeFlightDoesNotClipThroughWall(t *testing.T) {
	world := &game.World{}

	for y := int32(8); y <= 12; y++ {
		world.SetBlock(game.BlockPosition{X: 2, Y: y}, game.Stone)
	}

	runtime := NewRuntime(world)

	bat := runtime.SpawnBat(game.Position{X: 0.5, Y: 10, Z: 0.5})

	prepareFlyingBat(bat, game.BlockPosition{X: 10, Y: 10})

	runtime.entityRandom = fixedBatRandom

	for range 40 {
		bat.Tick(runtime, nil)
	}

	box := bat.Living.CollisionBox(bat.State.Position)
	if box.MaxX > 2 || bat.State.Position.X > 1.75 {
		t.Fatalf("bat clipped through wall: position %+v box %+v", bat.State.Position, box)
	}
}

func TestInactiveChunkPausesBatAI(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bat := runtime.SpawnBat(game.Position{X: 0.5, Y: 10, Z: 0.5})

	prepareFlyingBat(bat, game.BlockPosition{X: 10, Y: 15, Z: 10})

	runtime.entityRandom = fixedBatRandom

	position := bat.State.Position

	runtime.Tick()

	if bat.TickCount != 0 || bat.State.Position != position {
		t.Fatalf("inactive bat = ticks %d position %+v", bat.TickCount, bat.State.Position)
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.Tick()

	if bat.TickCount != 1 || bat.State.Position == position {
		t.Fatalf("active bat = ticks %d position %+v", bat.TickCount, bat.State.Position)
	}
}

func TestWarmedFlyingBatTickAllocations(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bat := runtime.SpawnBat(game.Position{X: 0.5, Y: 100, Z: 0.5})

	prepareFlyingBat(bat, game.BlockPosition{X: 1000, Y: 100, Z: 0})

	runtime.entityRandom = fixedBatRandom

	bat.Tick(runtime, nil)

	allocations := testing.AllocsPerRun(100, func() {
		bat.Tick(runtime, nil)
	})

	if allocations > 0.05 {
		t.Fatalf("warmed flying bat tick allocations = %f, want near zero", allocations)
	}
}

func activeBatTestRuntime(world *game.World, position game.Position) (*Runtime, *runtimeBatEntity) {
	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.entityRandom = fixedBatRandom

	return runtime, runtime.SpawnBat(position)
}

func prepareFlyingBat(bat *runtimeBatEntity, target game.BlockPosition) {
	bat.Resting = false
	bat.FlightTarget = target
	bat.HasFlightTarget = true
}

func fixedBatRandom() float32 {
	return 0.5
}

func assertBatAwakeMetadata(t *testing.T, bat *runtimeBatEntity) {
	t.Helper()

	metadata := bat.EntityMetadata()

	wantFlags := protocol.EntityMetadataEntry{Index: protocol.BatFlagsMetadataIndex, Type: protocol.MetadataTypeByte, Value: protocol.MetadataByte(0)}

	if bat.Resting || len(metadata) != 4 || metadata[3] != wantFlags {
		t.Fatalf("awake bat = resting %t metadata %+v", bat.Resting, metadata)
	}
}

func assertBatPositionClose(t *testing.T, tick int, actual, expected game.Position) {
	t.Helper()

	if math.Abs(actual.X-expected.X) > 1e-12 || math.Abs(actual.Y-expected.Y) > 1e-12 || math.Abs(actual.Z-expected.Z) > 1e-12 {
		t.Fatalf("tick %d position = %+v, want %+v", tick, actual, expected)
	}
}

func assertBatVelocityClose(t *testing.T, tick int, actual, expected game.Velocity) {
	t.Helper()

	if math.Abs(actual.X-expected.X) > 1e-12 || math.Abs(actual.Y-expected.Y) > 1e-12 || math.Abs(actual.Z-expected.Z) > 1e-12 {
		t.Fatalf("tick %d velocity = %+v, want %+v", tick, actual, expected)
	}
}
