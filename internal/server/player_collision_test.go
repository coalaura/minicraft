package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type partialPlacementCollisionTestCase struct {
	name    string
	cursorY float32
	allowed bool
}

func TestPlayerPlacementObstruction(t *testing.T) {
	t.Run("self", func(t *testing.T) {
		world := &game.World{Generator: placementTestGenerator{clicked: game.BlockPosition{X: 1, Y: 70}}}

		runtime := NewRuntime(world)

		actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

		actor.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.5}

		markPlacementChunksLoaded(actor, game.BlockPosition{X: 1, Y: 70}, game.BlockPosition{Y: 70})

		joinTestSession(t, runtime, actor)

		result, _, err := runtime.PlaceItem(actor, testUseItemOn(game.BlockPosition{X: 1, Y: 70}, protocol.BlockFaceWest, protocol.MainHand, 1), game.ItemStone)
		if err != nil {
			t.Fatalf("place intersecting self: %v", err)
		}

		if result.Allowed || result.Changed || world.BlockAt(game.BlockPosition{Y: 70}) != game.Air {
			t.Fatalf("intersecting placement result = %+v", result)
		}
	})

	t.Run("other player", func(t *testing.T) {
		clicked := game.BlockPosition{X: 1, Y: 70}
		target := game.BlockPosition{Y: 70}

		world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

		runtime := NewRuntime(world)

		actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

		actor.Player.Position = game.Position{X: 2.5, Y: 70, Z: 0.5}

		markPlacementChunksLoaded(actor, clicked, target)

		other, _ := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Other", game.GameModeCreative)

		other.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.5}

		joinTestSession(t, runtime, actor)
		joinTestSession(t, runtime, other)

		result, _, err := runtime.PlaceItem(actor, testUseItemOn(clicked, protocol.BlockFaceWest, protocol.MainHand, 2), game.ItemStone)
		if err != nil {
			t.Fatalf("place intersecting other player: %v", err)
		}

		if result.Allowed || result.Changed || world.BlockAt(target) != game.Air {
			t.Fatalf("other-player obstruction result = %+v", result)
		}
	})

	t.Run("adjacent", func(t *testing.T) {
		clicked := game.BlockPosition{X: 1, Y: 69}
		target := game.BlockPosition{Y: 69}

		world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

		runtime := NewRuntime(world)

		actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

		actor.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.5}

		markPlacementChunksLoaded(actor, clicked, target)

		joinTestSession(t, runtime, actor)

		result, _, err := runtime.PlaceItem(actor, testUseItemOn(clicked, protocol.BlockFaceWest, protocol.MainHand, 3), game.ItemStone)
		if err != nil {
			t.Fatalf("place adjacent block: %v", err)
		}

		if !result.Allowed || !result.Changed || world.BlockAt(target) != game.Stone {
			t.Fatalf("adjacent placement result = %+v", result)
		}
	})
}

func TestPartialPlacementUsesResolvedCollisionShape(t *testing.T) {
	tests := []partialPlacementCollisionTestCase{
		{name: "bottom slab touches feet", cursorY: 0.25, allowed: true},
		{name: "top slab intersects player", cursorY: 0.75, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clicked := game.BlockPosition{X: 1, Y: 70}
			target := game.BlockPosition{Y: 70}

			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

			actor.Player.Position = game.Position{X: 0.5, Y: 70.5, Z: 0.5}

			markPlacementChunksLoaded(actor, clicked, target)

			joinTestSession(t, runtime, actor)

			interaction := testUseItemOn(clicked, protocol.BlockFaceWest, protocol.MainHand, 4)

			interaction.CursorY = test.cursorY

			result, _, err := runtime.PlaceItem(actor, interaction, game.ItemOakSlab)
			if err != nil {
				t.Fatalf("place slab: %v", err)
			}

			if result.Changed != test.allowed {
				t.Fatalf("slab placement result = %+v, want allowed %t", result, test.allowed)
			}
		})
	}
}

func TestCoordinatedPlacementChecksEveryCollisionShape(t *testing.T) {
	clicked := game.BlockPosition{Y: 68}
	lower := game.BlockPosition{Y: 69}
	upper := game.BlockPosition{Y: 70}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

	actor.Player.Position = game.Position{X: 2.5, Y: 69, Z: 0.5}

	markPlacementChunksLoaded(actor, clicked, lower, upper)

	other, _ := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Other", game.GameModeCreative)

	other.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.1}

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, other)

	result, _, err := runtime.PlaceItem(actor, testUseItemOn(clicked, protocol.BlockFaceUp, protocol.MainHand, 5), game.ItemOakDoor)
	if err != nil {
		t.Fatalf("place obstructed door: %v", err)
	}

	if result.Allowed || result.Changed || world.BlockAt(lower) != game.Air || world.BlockAt(upper) != game.Air {
		t.Fatalf("coordinated placement result = %+v, blocks = %d/%d", result, world.BlockAt(lower), world.BlockAt(upper))
	}
}

func TestGenericMutationCanPlaceInsidePlayer(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{}

	runtime := NewRuntime(world)

	player, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player", game.GameModeCreative)

	player.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.5}

	markPlacementChunksLoaded(player, position)

	joinTestSession(t, runtime, player)

	result, err := runtime.MutateBlock(player, BlockMutationPlace, position, game.Stone)
	if err != nil {
		t.Fatalf("generic placement mutation: %v", err)
	}

	if !result.Allowed || !result.Changed || world.BlockAt(position) != game.Stone {
		t.Fatalf("generic placement result = %+v", result)
	}
}

func TestPlayerPoseSelectionAndRestoration(t *testing.T) {
	position := game.Position{X: 0.5, Y: 70, Z: 0.5}
	ceiling := game.BlockPosition{Y: 71}

	world := &game.World{}

	runtime := NewRuntime(world)

	player, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player", game.GameModeCreative)

	player.Player.Position = position

	joinTestSession(t, runtime, player)

	if pose := player.snapshotPlayer().Pose; pose != game.PlayerPoseStanding {
		t.Fatalf("initial pose = %d, want standing", pose)
	}

	player.handlePlayerInput(protocol.PlayerInput{Flags: protocol.PlayerInputSneak})

	if pose := player.snapshotPlayer().Pose; pose != game.PlayerPoseCrouching {
		t.Fatalf("sneaking pose = %d, want crouching", pose)
	}

	world.SetBlock(ceiling, game.Stone)

	err := player.handleMovePlayerPosition(protocol.MovePlayerPosition{X: position.X, Y: position.Y, Z: position.Z})
	if err != nil {
		t.Fatalf("recalculate obstructed pose: %v", err)
	}

	if pose := player.snapshotPlayer().Pose; pose != game.PlayerPoseCrawling {
		t.Fatalf("obstructed pose = %d, want crawling", pose)
	}

	world.SetBlock(ceiling, game.Air)

	err = player.handleMovePlayerPosition(protocol.MovePlayerPosition{X: position.X, Y: position.Y, Z: position.Z})
	if err != nil {
		t.Fatalf("recalculate restored crouch: %v", err)
	}

	if pose := player.snapshotPlayer().Pose; pose != game.PlayerPoseCrouching {
		t.Fatalf("restored sneaking pose = %d, want crouching", pose)
	}

	player.handlePlayerInput(protocol.PlayerInput{})

	if pose := player.snapshotPlayer().Pose; pose != game.PlayerPoseStanding {
		t.Fatalf("restored standing pose = %d, want standing", pose)
	}
}

func TestCalculatedPlayerPosePreservesCurrentPoseWhenNoBoxFits(t *testing.T) {
	position := game.Position{X: 0.5, Y: 70, Z: 0.5}

	world := &game.World{}

	world.SetBlock(game.BlockPosition{Y: 70}, game.Stone)

	runtime := NewRuntime(world)

	player := game.Player{Position: position, Pose: game.PlayerPoseCrouching}

	pose := runtime.calculatedPlayerPose(player)
	if pose != game.PlayerPoseCrouching {
		t.Fatalf("pose = %d, want crouching", pose)
	}
}

func TestTrapdoorInteractionForcesCrawlingAndBroadcastsSwimmingPose(t *testing.T) {
	position := game.BlockPosition{Y: 71}

	trapdoor, valid := game.OakTrapdoor.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: "north"},
		game.BlockPropertyValue{Name: "half", Value: "bottom"},
		game.BlockPropertyValue{Name: "open", Value: "true"},
		game.BlockPropertyValue{Name: "powered", Value: "false"},
		game.BlockPropertyValue{Name: "waterlogged", Value: "false"},
	)

	if !valid {
		t.Fatal("resolve open trapdoor")
	}

	world := &game.World{}

	world.SetBlock(position, trapdoor)

	runtime := NewRuntime(world)

	player, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player", game.GameModeCreative)

	player.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.5}
	player.Player.Sneaking = true

	markPlacementChunksLoaded(player, position)

	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	observer.Player.Position = game.Position{X: 2.5, Y: 70, Z: 0.5}

	joinTestSession(t, runtime, player)
	joinTestSession(t, runtime, observer)

	observerConnection.reset()

	handled, result, _, err := runtime.InteractBlock(player, position)
	if err != nil {
		t.Fatalf("close trapdoor: %v", err)
	}

	if !handled || !result.Changed || player.snapshotPlayer().Pose != game.PlayerPoseCrawling {
		t.Fatalf("closed trapdoor result = handled %t, result %+v, player %+v", handled, result, player.snapshotPlayer())
	}

	packets := observerConnection.packets(t)

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundEntityMetadataID})
	assertPlayerMetadata(t, packets[0], player.Player.EntityID, protocol.EntityFlagSneaking, protocol.EntityPoseSwimming)

	observerConnection.reset()

	_, result, _, err = runtime.InteractBlock(player, position)
	if err != nil {
		t.Fatalf("reopen trapdoor: %v", err)
	}

	if !result.Changed || player.snapshotPlayer().Pose != game.PlayerPoseCrouching {
		t.Fatalf("reopened trapdoor result = %+v, pose = %d", result, player.snapshotPlayer().Pose)
	}

	assertPlayerMetadata(t, observerConnection.packets(t)[0], player.Player.EntityID, protocol.EntityFlagSneaking, protocol.EntityPoseCrouching)
}

func TestCrawlingCollisionBoxAllowsPlacementAbovePlayer(t *testing.T) {
	clicked := game.BlockPosition{X: 1, Y: 70}
	target := game.BlockPosition{Y: 70}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	world.SetBlock(game.BlockPosition{Y: 71}, game.Stone)

	runtime := NewRuntime(world)

	player, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player", game.GameModeCreative)

	player.Player.Position = game.Position{X: 0.5, Y: 70, Z: 0.5}

	markPlacementChunksLoaded(player, clicked, target)

	joinTestSession(t, runtime, player)

	if pose := player.snapshotPlayer().Pose; pose != game.PlayerPoseCrawling {
		t.Fatalf("initial pose = %d, want crawling", pose)
	}

	interaction := testUseItemOn(clicked, protocol.BlockFaceWest, protocol.MainHand, 6)

	interaction.CursorY = 0.9

	result, _, err := runtime.PlaceItem(player, interaction, game.ItemOakTrapdoor)
	if err != nil {
		t.Fatalf("place trapdoor above crawling box: %v", err)
	}

	if !result.Allowed || !result.Changed || world.BlockAt(target) == game.Air {
		t.Fatalf("crawling placement result = %+v", result)
	}
}

func TestInvalidMovementInputIsRejectedWithoutMutation(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	player, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	player.Player.Position = game.Position{X: 1, Y: 70, Z: 2}
	player.Player.Rotation = game.Rotation{Yaw: 10, Pitch: 20}

	joinTestSession(t, runtime, player)

	connection.reset()

	initial := player.snapshotPlayer()

	invalidMoves := []protocol.MovePlayerPosition{
		{X: math.NaN(), Y: 70, Z: 2},
		{X: math.Inf(1), Y: 70, Z: 2},
		{X: 30_000_001, Y: 70, Z: 2},
	}

	for _, move := range invalidMoves {
		if err := player.handleMovePlayerPosition(move); err == nil {
			t.Fatalf("accepted invalid position %+v", move)
		}
	}

	err := player.handleMovePlayerRotation(protocol.MovePlayerRotation{Yaw: float32(math.Inf(1)), Pitch: 0})
	if err == nil {
		t.Fatal("accepted infinite rotation")
	}

	if current := player.snapshotPlayer(); current.Position != initial.Position || current.Rotation != initial.Rotation {
		t.Fatalf("invalid movement changed state from %+v to %+v", initial, current)
	}

	assertPacketIDs(t, connection.packetIDs(t), nil)
}
