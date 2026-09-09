package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type aquaticSpawnTestCase struct {
	Name       string
	EntityType game.EntityType
	Width      float64
	Height     float64
	EyeHeight  float64
	Metadata   []protocol.EntityMetadataEntry
}

type aquaticMovementTraceSample struct {
	PositionY float64
	VelocityY float64
}

func TestSpawnEntityRegistersAquaticFamilyWithExactDefaults(t *testing.T) {
	tests := []aquaticSpawnTestCase{
		{Name: "cod", EntityType: game.EntityCod, Width: 0.5, Height: 0.3, EyeHeight: 0.195},
		{Name: "salmon", EntityType: game.EntitySalmon, Width: 0.7, Height: 0.4, EyeHeight: 0.26, Metadata: []protocol.EntityMetadataEntry{{Index: protocol.FishVariantMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(1)}}},
		{Name: "tropical fish", EntityType: game.EntityTropicalFish, Width: 0.5, Height: 0.4, EyeHeight: 0.26, Metadata: []protocol.EntityMetadataEntry{{Index: protocol.FishVariantMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(0)}}},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			entity, spawned := runtime.SpawnEntity(test.EntityType, game.Position{X: 0.5, Y: 10, Z: 0.5})

			if !spawned || entity == nil {
				t.Fatalf("spawn = %T, %t", entity, spawned)
			}

			livingEntity := entity.(RuntimeLivingEntity)
			living := livingEntity.RuntimeLivingState()
			tracking := entity.(RuntimeEntityTracker).RuntimeEntityTrackingConfig()

			if living.Width != test.Width || living.Height != test.Height || living.MaxHealth != 3 || living.Health != 3 {
				t.Fatalf("living defaults = %v x %v health %v/%v", living.Width, living.Height, living.Health, living.MaxHealth)
			}

			eyeHeight := entity.(runtimeLivingEyeHeight).RuntimeLivingEyeHeight()
			if eyeHeight != test.EyeHeight {
				t.Fatalf("eye height = %v, want %v", eyeHeight, test.EyeHeight)
			}

			if tracking.ClientRangeChunks != aquaticTrackingRange || tracking.UpdateInterval != aquaticTrackingInterval || !tracking.TrackDeltas {
				t.Fatalf("tracking = %+v", tracking)
			}

			metadata := entity.(RuntimeEntityMetadata).EntityMetadata()
			if metadata[3] != (protocol.EntityMetadataEntry{Index: protocol.EntityAirMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(aquaticMaximumAirSupply)}) {
				t.Fatalf("air metadata = %+v", metadata)
			}

			if metadata[4] != (protocol.EntityMetadataEntry{Index: protocol.FishFromBucketMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(false)}) {
				t.Fatalf("from-bucket metadata = %+v", metadata)
			}

			if len(metadata) != 5+len(test.Metadata) {
				t.Fatalf("metadata = %+v", metadata)
			}

			for index := range test.Metadata {
				if metadata[index+5] != test.Metadata[index] {
					t.Fatalf("species metadata = %+v, want %+v", metadata[index+5:], test.Metadata)
				}
			}
		})
	}
}

func TestAquaticWaterMovementMatchesFishRecurrenceAndIgnoresCurrent(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1}, Replacement: flowingWater},
	})

	runtime := NewRuntime(world)

	cod := runtime.SpawnCod(game.Position{X: 0.75, Y: 0.1, Z: 0.5})

	contact := runtime.fluidContact(cod.Living.CollisionBox(cod.State.Position), game.FluidTypeWater, false)

	if contact.Flow.X == 0 {
		t.Fatal("fixture has no horizontal current")
	}

	expected := []aquaticMovementTraceSample{
		{0.105, -0.0005},
		{0.1095, -0.00095},
		{0.11355, -0.001355},
	}

	for tick, want := range expected {
		runtime.tickAquaticMovement(cod, &cod.runtimeAquatic)

		if cod.State.Position.X != 0.75 || cod.Living.Velocity.X != 0 {
			t.Fatalf("tick %d current changed x position/velocity to %v/%v", tick+1, cod.State.Position.X, cod.Living.Velocity.X)
		}

		difference := cod.State.Position.Y - want.PositionY

		if difference < -1e-14 || difference > 1e-14 {
			t.Fatalf("tick %d y = %.16f, want %.16f", tick+1, cod.State.Position.Y, want.PositionY)
		}

		difference = cod.Living.Velocity.Y - want.VelocityY

		if difference < -1e-14 || difference > 1e-14 {
			t.Fatalf("tick %d velocity y = %.16f, want %.16f", tick+1, cod.Living.Velocity.Y, want.VelocityY)
		}
	}
}

func TestAquaticMoveControlTurnsYawWithoutInventingPitchSteering(t *testing.T) {
	runtime := newAquaticNavigationRuntime()

	fillAquaticNavigationWater(runtime.World, 0, 4, 9, 12, 0, 0)

	cod := runtime.SpawnCod(game.Position{X: 0.5, Y: 10, Z: 0.5})

	path := []game.Position{{X: 3.5, Y: 11, Z: 0.5}}

	cod.Navigation.SetPath(path, 1, path[0])

	cod.Rotation.Pitch = 27

	runtime.tickAquaticMovement(cod, &cod.runtimeAquatic)

	if cod.Rotation.Yaw != -90 || cod.Rotation.HeadYaw != -90 {
		t.Fatalf("yaw/head yaw = %v/%v, want -90/-90", cod.Rotation.Yaw, cod.Rotation.HeadYaw)
	}

	if cod.Rotation.Pitch != 27 {
		t.Fatalf("pitch = %v, want unchanged 27", cod.Rotation.Pitch)
	}

	if cod.Living.Velocity.Y <= 0 {
		t.Fatalf("vertical target did not steer velocity: %+v", cod.Living.Velocity)
	}
}

func TestAquaticAirDepletesDamagesAndResetsInWater(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: blockMutationTestGenerator{block: game.Air}})

	cod := runtime.SpawnCod(game.Position{X: 0.5, Y: 10, Z: 0.5})

	for range aquaticMaximumAirSupply - aquaticDrowningThreshold {
		runtime.tickAquaticAir(cod, &cod.runtimeAquatic)
	}

	if cod.AirSupply != 0 || cod.Living.Health != 1 {
		t.Fatalf("dehydrated fish = air %d health %v, want 0 and 1", cod.AirSupply, cod.Living.Health)
	}

	fillAquaticNavigationWater(runtime.World, 0, 0, 10, 10, 0, 0)

	runtime.tickAquaticAir(cod, &cod.runtimeAquatic)

	if cod.AirSupply != aquaticMaximumAirSupply {
		t.Fatalf("water air = %d, want %d", cod.AirSupply, aquaticMaximumAirSupply)
	}
}

func TestAquaticFishFlopsFromGroundedVerticalCollision(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0.5
	}

	cod := runtime.SpawnCod(game.Position{X: 0.5, Y: 1, Z: 0.5})

	cod.Living.OnGround = true
	cod.VerticalCollision = true

	runtime.tickAquaticMovement(cod, &cod.runtimeAquatic)

	if cod.State.Position.Y != 1.4 || cod.Living.Velocity.Y != 0.3136 || cod.Living.OnGround {
		t.Fatalf("flop = position %+v velocity %+v on ground %t", cod.State.Position, cod.Living.Velocity, cod.Living.OnGround)
	}
}

func TestAquaticSchoolMembershipTracksLeaderLimitsAndDisconnect(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	leader := runtime.SpawnCod(game.Position{})

	followers := make([]*runtimeCodEntity, 0, aquaticDefaultSchoolSize)

	for index := 1; index < aquaticDefaultSchoolSize; index++ {
		follower := runtime.SpawnCod(game.Position{X: float64(index)})
		followers = append(followers, follower)

		if !connectAquaticFollower(follower, leader) {
			t.Fatalf("follower %d did not join", index)
		}
	}

	overflow := runtime.SpawnCod(game.Position{X: 8})
	if connectAquaticFollower(overflow, leader) {
		t.Fatal("school exceeded maximum size")
	}

	if leader.School.SchoolSize != aquaticDefaultSchoolSize {
		t.Fatalf("leader school size = %d", leader.School.SchoolSize)
	}

	disconnectAquaticFollower(followers[0])

	if leader.School.SchoolSize != aquaticDefaultSchoolSize-1 || followers[0].School.Leader != nil {
		t.Fatalf("disconnect = leader size %d follower leader %T", leader.School.SchoolSize, followers[0].School.Leader)
	}

	salmonLeader := runtime.SpawnSalmon(game.Position{Z: 2})

	for index := 1; index < salmonMaximumSchoolSize; index++ {
		follower := runtime.SpawnSalmon(game.Position{X: float64(index), Z: 2})
		if !connectAquaticFollower(follower, salmonLeader) {
			t.Fatalf("salmon follower %d did not join", index)
		}
	}

	if connectAquaticFollower(runtime.SpawnSalmon(game.Position{X: 5, Z: 2}), salmonLeader) {
		t.Fatal("salmon school exceeded size five")
	}
}

func TestAquaticSchoolFormationUsesNearbySameSpecies(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	leader := runtime.SpawnCod(game.Position{})
	firstFollower := runtime.SpawnCod(game.Position{X: 1})
	secondFollower := runtime.SpawnCod(game.Position{X: 2})

	runtime.SpawnSalmon(game.Position{X: 3})

	if runtime.formAquaticSchool(leader) {
		t.Fatal("new leader reported itself as a follower")
	}

	if leader.School.SchoolSize != 3 || firstFollower.School.Leader != leader || secondFollower.School.Leader != leader {
		t.Fatalf("formed school = size %d leaders %T/%T", leader.School.SchoolSize, firstFollower.School.Leader, secondFollower.School.Leader)
	}
}

func TestAquaticFollowerStopsFollowingDeadLeader(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	leader := runtime.SpawnCod(game.Position{})
	follower := runtime.SpawnCod(game.Position{X: 1})

	if !connectAquaticFollower(follower, leader) {
		t.Fatal("follower did not join leader")
	}

	goal := aquaticFollowSchoolGoal{Fish: follower}
	if !goal.CanContinue(runtime) {
		t.Fatal("live nearby leader did not keep follow goal active")
	}

	leader.Living.Dead = true

	if goal.CanContinue(runtime) {
		t.Fatal("dead leader kept follow goal active")
	}
}

func TestAquaticInactiveChunkPausesMovementAndAir(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: blockMutationTestGenerator{block: game.Air}})

	session := addRuntimeMobTestPlayer(t, runtime, game.Position{X: 100}, game.GameModeSurvival)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	cod := runtime.SpawnCod(game.Position{X: 0.5, Y: 10, Z: 0.5})

	runtime.Tick()

	if cod.TickCount != 1 || cod.AirSupply != aquaticMaximumAirSupply-1 {
		t.Fatalf("active fish = ticks %d air %d", cod.TickCount, cod.AirSupply)
	}

	position := cod.State.Position
	velocity := cod.Living.Velocity
	airSupply := cod.AirSupply

	runtime.releaseSessionActiveChunks(session)

	runtime.Tick()

	if cod.TickCount != 1 || cod.State.Position != position || cod.Living.Velocity != velocity || cod.AirSupply != airSupply {
		t.Fatal("inactive fish advanced movement, AI, or air state")
	}
}
