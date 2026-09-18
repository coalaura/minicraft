package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestPassengerMountRejectsCyclesMultipleVehiclesAndOverflow(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	first, _ := newPassengerTestSession(runtime, "First")
	second, _ := newPassengerTestSession(runtime, "Second")
	third, _ := newPassengerTestSession(runtime, "Third")
	fourth, _ := newPassengerTestSession(runtime, "Fourth")

	if !runtime.MountPassenger(first, second) {
		t.Fatal("mount second on first")
	}

	if runtime.MountPassenger(second, first) {
		t.Fatal("accepted passenger cycle")
	}

	if runtime.MountPassenger(third, second) {
		t.Fatal("accepted second vehicle")
	}

	if runtime.MountPassenger(first, third) || runtime.MountPassenger(first, fourth) {
		t.Fatal("accepted a second passenger on a default vehicle")
	}

	if second.VehicleID() != first.passengerID() {
		t.Fatalf("second vehicle = %d, want %d", second.VehicleID(), first.passengerID())
	}
}

func TestPassengerPacketsPreserveOrder(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer, vehicleConnection := newPassengerTestSession(runtime, "Viewer")
	first, _ := newPassengerTestSession(runtime, "First")
	second, _ := newPassengerTestSession(runtime, "Second")

	vehicle := runtime.SpawnBoat(game.EntityOakBoat, game.Position{})

	viewer.trackedEntities = map[int32]struct{}{vehicle.State.ID: {}}

	if !runtime.MountPassenger(vehicle, first) || !runtime.MountPassenger(vehicle, second) {
		t.Fatal("mount passengers")
	}

	packets := packetsByID(t, vehicleConnection, protocol.ClientboundSetPassengersID)
	if len(packets) != 2 {
		t.Fatalf("passenger packets = %d, want 2", len(packets))
	}

	vehicleID, passengers := decodeSetPassengers(t, packets[1])
	if vehicleID != vehicle.State.ID || len(passengers) != 2 || passengers[0] != first.passengerID() || passengers[1] != second.passengerID() {
		t.Fatalf("set passengers = vehicle %d passengers %v", vehicleID, passengers)
	}
}

func TestPassengerRuntimeRemovalAndDisconnectSynchronizeBeforeRemoval(t *testing.T) {
	t.Run("runtime", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		passenger, _ := newPassengerTestSession(runtime, "Passenger")
		viewer, connection := newPassengerTestSession(runtime, "Viewer")

		vehicle := &runtimeItemEntity{}

		runtime.registerRuntimeEntity(vehicle, game.EntityItem, game.Position{})

		viewer.trackedEntities = map[int32]struct{}{vehicle.State.ID: {}}

		if !runtime.MountPassenger(vehicle, passenger) {
			t.Fatal("mount runtime vehicle")
		}

		connection.reset()

		runtime.removeRuntimeEntity(vehicle.State.ID)

		assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})
	})

	t.Run("disconnect", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})
		vehicle, _ := newPassengerTestSession(runtime, "Vehicle")
		passenger, _ := newPassengerTestSession(runtime, "Passenger")
		_, connection := newPassengerTestSession(runtime, "Viewer")

		if !runtime.MountPassenger(vehicle, passenger) {
			t.Fatal("mount player vehicle")
		}

		connection.reset()
		runtime.LeaveSession(vehicle)

		assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundSetPassengersID, protocol.ClientboundRemoveEntitiesID, protocol.ClientboundPlayerInfoRemoveID})
	})
}

func TestPassengerTrackingEnterLeaveAndPositionUpdates(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	vehicle, _ := newPassengerTestSession(runtime, "Vehicle")
	passenger, _ := newPassengerTestSession(runtime, "Passenger")
	viewer, connection := newPassengerTestSession(runtime, "Viewer")

	viewer.Player.Position.X = 256

	viewer.markPlayerHidden(vehicle.passengerID())
	viewer.markPlayerHidden(passenger.passengerID())

	if !runtime.MountPassenger(vehicle, passenger) {
		t.Fatal("mount passengers")
	}

	if len(packetsByID(t, connection, protocol.ClientboundSetPassengersID)) != 0 {
		t.Fatal("sent passenger packet before both players were visible")
	}

	runtime.updatePlayerMovement(viewer, func(player *game.Player) {
		player.Position = game.Position{}
	})

	if len(packetsByID(t, connection, protocol.ClientboundSetPassengersID)) != 1 {
		t.Fatal("did not synchronize passengers when tracking entered")
	}

	connection.reset()

	runtime.updatePlayerMovement(viewer, func(player *game.Player) {
		player.Position.X = 256
	})

	if len(packetsByID(t, connection, protocol.ClientboundSetPassengersID)) != 0 {
		t.Fatal("sent passenger packet when tracking left")
	}

	update := func(view PassengerView) game.Position {
		return game.Position{X: view.Position.X + 1}
	}

	allocations := testing.AllocsPerRun(100, func() {
		runtime.UpdatePassengerPositions(vehicle, update)
	})

	if allocations != 0 {
		t.Fatalf("passenger position update allocations = %f, want 0", allocations)
	}
}

func TestPassengerSneakDismount(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	vehicle, _ := newPassengerTestSession(runtime, "Vehicle")
	passenger, _ := newPassengerTestSession(runtime, "Passenger")

	if !runtime.MountPassenger(vehicle, passenger) {
		t.Fatal("mount passenger")
	}

	passenger.handlePlayerInput(protocol.PlayerInput{Flags: protocol.PlayerInputSneak})

	if passenger.VehicleID() != 0 {
		t.Fatalf("vehicle after sneak = %d, want 0", passenger.VehicleID())
	}
}

func newPassengerTestSession(runtime *Runtime, name string) (*Session, *recordingConnection) {
	session, connection := newMovementTestSession(runtime, randomEntityUUID(), name)

	runtime.AssignEntityID(session)

	for _, other := range runtime.sessionView() {
		other.markPlayerVisible(session.passengerID())
		session.markPlayerVisible(other.passengerID())
	}

	runtime.addSession(session)

	return session, connection
}

func decodeSetPassengers(t *testing.T, packet protocol.Packet) (int32, []int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	vehicleID := reader.VarInt()
	count := reader.VarInt()

	passengers := make([]int32, count)

	for index := range passengers {
		passengers[index] = reader.VarInt()
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode set passengers: %v", err)
	}

	return vehicleID, passengers
}
