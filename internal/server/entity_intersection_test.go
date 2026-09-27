package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type placementEntityIntersectionCase struct {
	name       string
	spawn      func(*Runtime, game.Position)
	obstructed bool
}

type partialCollisionShapeCase struct {
	name       string
	y          float64
	obstructed bool
}

type livingIntersectionDimensionsCase struct {
	name       string
	x          float64
	width      float64
	obstructed bool
}

func TestPlacementChecksRuntimeEntityIntersections(t *testing.T) {
	tests := []placementEntityIntersectionCase{
		{name: "living mob", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnZombie(position)
		}, obstructed: true},
		{name: "boat", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnBoat(game.EntityOakBoat, position)
		}, obstructed: true},
		{name: "raft", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnBoat(game.EntityBambooRaft, position)
		}, obstructed: true},
		{name: "primed tnt", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnTnt(position, game.Velocity{}, 0, false)
		}, obstructed: true},
		{name: "falling block", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnFallingBlock(position, game.Sand, game.BlockPosition{Y: 70})
		}, obstructed: true},
		{name: "dropped item", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, position, game.Velocity{}, 0)
		}},
		{name: "arrow", spawn: func(runtime *Runtime, position game.Position) {
			runtime.SpawnArrow(position, game.Velocity{}, 0)
		}},
	}

	clicked := game.BlockPosition{X: 1, Y: 70}
	target := game.BlockPosition{Y: 70}
	position := game.Position{X: .5, Y: 70, Z: .5}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			markPlacementChunksLoaded(actor, clicked, target)

			joinTestSession(t, runtime, actor)

			test.spawn(runtime, position)

			result, err := runtime.PlaceBlock(actor, clicked, target, game.Stone)
			if err != nil {
				t.Fatalf("place stone: %v", err)
			}

			if result.Changed == test.obstructed {
				t.Fatalf("placement changed = %t, obstructed = %t", result.Changed, test.obstructed)
			}
		})
	}
}

func TestSpectatorDoesNotObstructBlockPlacement(t *testing.T) {
	clicked := game.BlockPosition{X: 1, Y: 70}
	target := game.BlockPosition{Y: 70}

	world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

	runtime := NewRuntime(world)

	actor, _ := newPlacementTestSession(runtime, clicked)
	spectator, _ := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Spectator", game.GameModeSpectator)

	spectator.Player.Position = game.Position{X: .5, Y: 70, Z: .5}

	markPlacementChunksLoaded(actor, clicked, target)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, spectator)

	result, err := runtime.PlaceBlock(actor, clicked, target, game.Stone)
	if err != nil || !result.Changed {
		t.Fatalf("place through spectator = %+v, %v", result, err)
	}
}

func TestPlacementUsesCurrentLivingEntityDimensions(t *testing.T) {
	tests := []livingIntersectionDimensionsCase{
		{name: "narrowed outside", x: 1.15, width: .2},
		{name: "widened inside", x: 1.35, width: .8, obstructed: true},
	}

	clicked := game.BlockPosition{X: 1, Y: 70}
	target := game.BlockPosition{Y: 70}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			markPlacementChunksLoaded(actor, clicked, target)

			joinTestSession(t, runtime, actor)

			zombie := runtime.SpawnZombie(game.Position{X: test.x, Y: 70, Z: .5})

			zombie.State.mu.Lock()
			zombie.RuntimeLivingState().Width = test.width
			zombie.State.mu.Unlock()

			result, err := runtime.PlaceBlock(actor, clicked, target, game.Stone)
			if err != nil {
				t.Fatalf("place stone: %v", err)
			}

			if result.Changed == test.obstructed {
				t.Fatalf("placement changed = %t, obstructed = %t", result.Changed, test.obstructed)
			}
		})
	}
}

func TestPartialCollisionShapeChecksEntityVolume(t *testing.T) {
	tests := []partialCollisionShapeCase{
		{name: "below top trapdoor", y: 70, obstructed: false},
		{name: "inside top trapdoor", y: 70.5, obstructed: true},
		{name: "touching top trapdoor", y: 70.25, obstructed: false},
	}

	clicked := game.BlockPosition{X: 1, Y: 70}
	target := game.BlockPosition{Y: 70}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: placementTestGenerator{clicked: clicked}}

			runtime := NewRuntime(world)

			actor, _ := newPlacementTestSession(runtime, clicked)

			markPlacementChunksLoaded(actor, clicked, target)

			joinTestSession(t, runtime, actor)

			// The boat's top touches the top trapdoor at exactly 70.8125 in the edge case.
			runtime.SpawnBoat(game.EntityOakBoat, game.Position{X: .5, Y: test.y, Z: .5})

			interaction := testUseItemOn(clicked, protocol.BlockFaceWest, protocol.MainHand, 1)
			interaction.CursorY = .9

			result, _, err := runtime.PlaceItem(actor, interaction, game.ItemOakTrapdoor)
			if err != nil {
				t.Fatalf("place top trapdoor: %v", err)
			}

			if result.Changed == test.obstructed {
				t.Fatalf("placement changed = %t, obstructed = %t", result.Changed, test.obstructed)
			}
		})
	}
}
