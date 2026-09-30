package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type boatContractedValidationTestCase struct {
	Name      string
	Start     game.Position
	Target    game.Position
	PreClear  bool
	PostClear bool
	Corrected bool
}

func TestBoatMoveVehicleContractedCollisionBoundaries(t *testing.T) {
	tests := []boatContractedValidationTestCase{
		{Name: "shallow side overlap", Start: game.Position{X: -2}, Target: game.Position{X: -0.65}, PreClear: true, PostClear: true},
		{Name: "contracted side obstructed", Start: game.Position{X: -2}, Target: game.Position{X: -0.60}, PreClear: true, Corrected: true},
		{Name: "preexisting contracted collision", Start: game.Position{X: 0.5}, Target: game.Position{X: -0.60}},
		{Name: "shallow top overlap", Start: game.Position{X: 0.5, Y: -2}, Target: game.Position{X: 0.5, Y: -0.54}, PreClear: true, PostClear: true},
		{Name: "contracted top obstructed", Start: game.Position{X: 0.5, Y: -2}, Target: game.Position{X: 0.5, Y: -0.49}, PreClear: true, Corrected: true},
	}

	entityTypes := []game.EntityType{game.EntityOakBoat, game.EntityBambooRaft}

	for _, entityType := range entityTypes {
		definition, _ := entityType.Definition()

		for _, test := range tests {
			t.Run(definition.Name+"/"+test.Name, func(t *testing.T) {
				world := &game.World{}

				world.SetBlock(game.BlockPosition{}, game.Stone)

				runtime := NewRuntime(world)

				controller, connection := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000096", "controller")

				runtime.AssignEntityID(controller)
				runtime.addSession(controller)

				boat := runtime.SpawnBoat(entityType, test.Start)
				if !runtime.MountPassenger(boat, controller) {
					t.Fatal("mount controller")
				}

				if runtime.boatHasNoCollision(boat.State.ID, test.Start) != test.PreClear || runtime.boatHasNoCollision(boat.State.ID, test.Target) != test.PostClear {
					t.Fatal("fixture does not straddle the source 0.0625 contraction")
				}

				connection.reset()

				controller.handleMoveVehicle(protocol.MoveVehicle{X: test.Target.X, Y: test.Target.Y, Z: test.Target.Z, Yaw: 65, Pitch: 12})

				want := test.Target
				corrections := 0

				if test.Corrected {
					want = test.Start
					corrections = 1
				}

				assertBoatPositionClose(t, boat.State.Position, want)

				if controller.vehicleLastGood != want || boat.Rotation.Yaw != 65 || boat.Rotation.Pitch != 12 || len(packetsByID(t, connection, protocol.ClientboundMoveVehicleID)) != corrections {
					t.Fatalf("validation state: lastGood=%+v rotation=%+v corrections=%d", controller.vehicleLastGood, boat.Rotation, len(packetsByID(t, connection, protocol.ClientboundMoveVehicleID)))
				}
			})
		}
	}
}
