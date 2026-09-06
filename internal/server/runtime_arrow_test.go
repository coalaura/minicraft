package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type arrowDifficultyDamageTest struct {
	name       string
	difficulty game.Difficulty
	wantHealth float32
}

type arrowTrajectorySnapshot struct {
	position game.Position
	velocity game.Velocity
}

func TestSpawnArrowMetadataAndPhysics(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	arrow := runtime.SpawnArrow(game.Position{X: 1, Y: 2, Z: 3}, game.Velocity{X: 1}, 42)

	packet := arrow.AddEntityPacket(runtimeEntitySpawnSnapshot{ID: arrow.State.ID, UUID: arrow.State.UUID, Position: arrow.State.Position, Velocity: arrow.Velocity})
	if packet.Type != int32(game.EntityArrow) || packet.Data != 42 {
		t.Fatalf("arrow spawn = %+v", packet)
	}

	metadata := arrow.EntityMetadata()
	if len(metadata) != 3 || metadata[0].Index != protocol.ArrowFlagsMetadataIndex || metadata[1].Index != protocol.ArrowPierceLevelMetadataIndex || metadata[2].Index != protocol.ArrowInGroundMetadataIndex || metadata[2].Type != protocol.MetadataTypeBoolean || metadata[2].Value != protocol.MetadataBoolean(false) {
		t.Fatalf("arrow metadata = %+v", metadata)
	}

	arrow.Tick(runtime, nil)

	if arrow.State.Position.X != 2 || math.Abs(arrow.Velocity.X-0.99) > 1e-9 || math.Abs(arrow.Velocity.Y+0.05) > 1e-9 {
		t.Fatalf("arrow trace = position %+v velocity %+v", arrow.State.Position, arrow.Velocity)
	}
}

func TestArrowAirPhysicsTrace(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	arrow := runtime.SpawnArrow(game.Position{Y: 10, Z: .5}, game.Velocity{X: .3, Y: .2, Z: -.1}, 0)

	snapshots := []arrowTrajectorySnapshot{
		{position: game.Position{X: .3, Y: 10.2, Z: .4}, velocity: game.Velocity{X: .297, Y: .148, Z: -.099}},
		{position: game.Position{X: .597, Y: 10.348, Z: .301}, velocity: game.Velocity{X: .29403, Y: .09652, Z: -.09801}},
		{position: game.Position{X: .89103, Y: 10.44452, Z: .20299}, velocity: game.Velocity{X: .2910897, Y: .0455548, Z: -.0970299}},
		{position: game.Position{X: 1.1821197, Y: 10.4900748, Z: .1059601}, velocity: game.Velocity{X: .288178803, Y: -.004900748, Z: -.096059601}},
	}

	for tick, snapshot := range snapshots {
		runtime.Tick()

		assertPositionClose(t, arrow.State.Position, snapshot.position, 1e-12)
		assertVelocityClose(t, arrow.Velocity, snapshot.velocity, 1e-12)

		if arrow.TickCount != int32(tick+1) {
			t.Fatalf("tick count = %d, want %d", arrow.TickCount, tick+1)
		}
	}
}

func TestArrowTicksOnlyInActiveChunks(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	arrow := runtime.SpawnArrow(game.Position{Y: 10}, game.Velocity{X: 1}, 0)

	runtime.Tick()

	if arrow.State.Position != (game.Position{Y: 10}) || arrow.Velocity != (game.Velocity{X: 1}) || arrow.TickCount != 0 {
		t.Fatalf("inactive arrow state = position %+v velocity %+v ticks %d", arrow.State.Position, arrow.Velocity, arrow.TickCount)
	}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.Tick()

	if arrow.State.Position != (game.Position{X: 1, Y: 10}) || arrow.Velocity != (game.Velocity{X: .99, Y: -.05}) || arrow.TickCount != 1 {
		t.Fatalf("active arrow state = position %+v velocity %+v ticks %d", arrow.State.Position, arrow.Velocity, arrow.TickCount)
	}

	runtime.setSessionActiveChunks(viewer, nil)

	runtime.Tick()

	if arrow.State.Position != (game.Position{X: 1, Y: 10}) || arrow.Velocity != (game.Velocity{X: .99, Y: -.05}) || arrow.TickCount != 1 {
		t.Fatalf("paused arrow state = position %+v velocity %+v ticks %d", arrow.State.Position, arrow.Velocity, arrow.TickCount)
	}
}

func TestArrowStartingInsideBlockEmbeds(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.World.SetBlock(game.BlockPosition{}, game.Stone)

	arrow := runtime.SpawnArrow(game.Position{X: .5, Y: .5, Z: .5}, game.Velocity{}, 0)

	arrow.Tick(runtime, nil)

	if !arrow.InGround || arrow.EmbeddedAt != (game.BlockPosition{}) || arrow.EmbeddedBlock != game.Stone || arrow.State.Position != (game.Position{X: .5, Y: .5, Z: .5}) {
		t.Fatalf("embedded arrow = %+v", arrow)
	}
}

func TestArrowEmbedsAndExpires(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.World.SetBlock(game.BlockPosition{X: 2}, game.Stone)

	arrow := runtime.SpawnArrow(game.Position{X: 0, Y: 0.5, Z: 0.5}, game.Velocity{X: 4}, 0)

	arrow.Tick(runtime, nil)

	if !arrow.InGround || arrow.ShakeTime != arrowShakeTicks || arrow.Life != 0 || arrow.State.Position.X >= 2 {
		t.Fatalf("embedded arrow = %+v", arrow)
	}

	metadata := arrow.EntityMetadata()

	if metadata[2].Value != protocol.MetadataBoolean(true) {
		t.Fatalf("embedded arrow metadata = %+v", metadata)
	}

	arrow.Life = arrowLifetime - 1

	arrow.Tick(runtime, nil)

	if !arrow.State.Removed {
		t.Fatal("embedded arrow was not removed at its lifetime")
	}
}

func TestGroundedArrowExpirationUntracksViewer(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer")

	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	connection.reset()

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.World.SetBlock(game.BlockPosition{X: 2}, game.Stone)

	arrow := runtime.SpawnArrow(game.Position{Y: .5, Z: .5}, game.Velocity{X: 4}, 0)

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})

	connection.reset()

	runtime.Tick()

	if !arrow.InGround {
		t.Fatal("arrow did not embed before expiration")
	}

	connection.reset()

	arrow.Life = arrowLifetime - 1

	runtime.Tick()

	sessionTracks := viewer.tracksRuntimeEntity(arrow.State.ID)

	if !arrow.State.Removed || sessionTracks {
		t.Fatalf("expired arrow removed=%t tracked=%t", arrow.State.Removed, sessionTracks)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})
}

func TestArrowHitsNearestLivingAndExcludesOwner(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	owner := spawnTestRuntimeLivingEntity(runtime, game.Position{}, 20)
	near := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 2}, 20)
	far := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 4}, 20)

	arrow := runtime.SpawnArrow(game.Position{X: 0, Y: 0.5}, game.Velocity{X: 5}, owner.State.ID)

	arrow.Tick(runtime, nil)

	if owner.Living.Health != 20 || near.Living.Health != 10 || far.Living.Health != 20 {
		t.Fatalf("arrow damage owner=%v near=%v far=%v", owner.Living.Health, near.Living.Health, far.Living.Health)
	}

	if near.Living.LastDamageType != game.DamageArrow || near.Living.LastDamageCauseEntityID != owner.State.ID {
		t.Fatalf("arrow damage attribution type=%v cause=%v", near.Living.LastDamageType, near.Living.LastDamageCauseEntityID)
	}

	if !arrow.State.Removed {
		t.Fatal("nonpiercing arrow remained after a successful hit")
	}
}

func TestArrowReleaseWhenEmbeddedShapeChanges(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	randomValues := []float32{.1, .2, .3}
	randomIndex := 0

	runtime.entityRandom = func() float32 {
		value := randomValues[randomIndex]
		randomIndex++

		return value
	}

	runtime.World.SetBlock(game.BlockPosition{X: 1}, game.Stone)

	arrow := runtime.SpawnArrow(game.Position{X: 0, Y: 0.5, Z: 0.5}, game.Velocity{X: 2, Y: .5, Z: .5}, 0)

	arrow.Tick(runtime, nil)

	runtime.World.SetBlock(game.BlockPosition{X: 1}, game.Air)

	arrow.Tick(runtime, nil)

	if arrow.State.Removed || arrow.InGround || arrow.Life != 0 {
		t.Fatalf("released arrow = %+v", arrow)
	}

	if math.Abs(arrow.Velocity.X-.0396) > 1e-7 || math.Abs(arrow.Velocity.Y+.0302) > 1e-7 || math.Abs(arrow.Velocity.Z-.0297) > 1e-7 {
		t.Fatalf("released arrow velocity = %+v", arrow.Velocity)
	}
}

func TestArrowInGroundMetadataDirtyTransitions(t *testing.T) {
	arrow := &runtimeArrowEntity{}

	arrowSetInGround(arrow, true)

	if !arrow.InGround || !arrow.State.metadataDirty {
		t.Fatalf("embedded state inGround=%t metadataDirty=%t", arrow.InGround, arrow.State.metadataDirty)
	}

	arrow.State.metadataDirty = false

	arrowSetInGround(arrow, false)

	if arrow.InGround || !arrow.State.metadataDirty {
		t.Fatalf("released state inGround=%t metadataDirty=%t", arrow.InGround, arrow.State.metadataDirty)
	}
}

func TestArrowOwnerlessDamageUsesArrowAsCause(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{}, 20)

	arrow := runtime.SpawnArrow(game.Position{}, game.Velocity{X: 1}, 0)

	arrow.SetBaseDamage(4)

	if !runtime.damageArrowTarget(arrowTarget{entity: target}, arrow.State.ID, 0, game.Velocity{X: 1}) {
		t.Fatal("ownerless arrow damage was not applied")
	}

	if target.Living.LastDamageCauseEntityID != arrow.State.ID {
		t.Fatalf("damage cause = %d, want arrow ID %d", target.Living.LastDamageCauseEntityID, arrow.State.ID)
	}

	if target.Living.Health != 16 {
		t.Fatalf("target health = %v, want 16", target.Living.Health)
	}
}

func TestArrowCollisionMarginRamp(t *testing.T) {
	margin := arrowCollisionMarginForTick(2)

	if margin != 0 {
		t.Fatalf("collision margin at tick 2 = %v, want 0", margin)
	}

	margin = arrowCollisionMarginForTick(22)

	if margin != .3 {
		t.Fatalf("collision margin at tick 22 = %v, want .3", margin)
	}
}

func TestArrowRotationSmoothing(t *testing.T) {
	rotation := arrowLerpRotation(0, 90)

	if rotation != 18 {
		t.Fatalf("rotation smoothing = %v, want 18", rotation)
	}

	rotation = arrowLerpRotation(170, -170)

	if rotation != 174 {
		t.Fatalf("wrapped rotation smoothing = %v, want 174", rotation)
	}
}

func TestArrowFailedHitReversesAndDiscardsAtLowSpeed(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: .4}, 20)

	target.Living.InvulnerableTime = game.LivingHurtCooldownTicks
	target.Living.LastHurt = 10

	arrow := runtime.SpawnArrow(game.Position{}, game.Velocity{X: 5}, 0)

	arrow.Tick(runtime, nil)

	if arrow.State.Removed || arrow.Velocity.X >= 0 {
		t.Fatalf("failed hit arrow = %+v", arrow)
	}

	arrow = runtime.SpawnArrow(game.Position{X: .2}, game.Velocity{X: .004}, 0)

	arrow.Tick(runtime, nil)

	if !arrow.State.Removed {
		t.Fatal("low-speed failed hit arrow was not discarded")
	}
}

func TestArrowLivingOwnerPlayerDifficultyDamage(t *testing.T) {
	tests := []arrowDifficultyDamageTest{
		{name: "peaceful", difficulty: game.DifficultyPeaceful, wantHealth: 20},
		{name: "easy", difficulty: game.DifficultyEasy, wantHealth: 14},
		{name: "normal", difficulty: game.DifficultyNormal, wantHealth: 10},
		{name: "hard", difficulty: game.DifficultyHard, wantHealth: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			runtime.Difficulty = test.difficulty

			owner := spawnTestRuntimeLivingEntity(runtime, game.Position{}, 20)

			arrow := runtime.SpawnArrow(game.Position{}, game.Velocity{X: 5}, owner.State.ID)

			connection := &recordingConnection{}

			player := &game.Player{EntityID: 100}

			player.ResetSurvivalState()

			session := &Session{Conn: protocol.NewConnection(connection, nil), Runtime: runtime, Player: player}

			runtime.damageArrowTarget(arrowTarget{session: session}, arrow.State.ID, owner.State.ID, game.Velocity{X: 5})

			if player.Health != test.wantHealth {
				t.Fatalf("player health = %v, want %v", player.Health, test.wantHealth)
			}
		})
	}
}

func TestArrowLivingOwnerRuntimeTargetDamageIsUnscaled(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.Difficulty = game.DifficultyPeaceful

	owner := spawnTestRuntimeLivingEntity(runtime, game.Position{}, 20)

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{}, 20)

	arrow := runtime.SpawnArrow(game.Position{}, game.Velocity{X: 5}, owner.State.ID)

	if !runtime.damageArrowTarget(arrowTarget{entity: target}, arrow.State.ID, owner.State.ID, game.Velocity{X: 5}) {
		t.Fatal("runtime target damage was not applied")
	}

	if target.Living.Health != 10 {
		t.Fatalf("runtime target health = %v, want 10", target.Living.Health)
	}
}
