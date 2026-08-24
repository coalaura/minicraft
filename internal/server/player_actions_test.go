package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestRuntimeSynchronizesPlayerStateAndAnimations(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, bobConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Bob")
	alice, aliceConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Alice")

	joinTestSession(t, runtime, bob)
	joinTestSession(t, runtime, alice)

	bobConnection.reset()
	aliceConnection.reset()

	bob.handlePlayerInput(protocol.PlayerInput{Flags: protocol.PlayerInputSneak | protocol.PlayerInputSprint})

	assertPacketIDs(t, bobConnection.packetIDs(t), nil)
	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{protocol.ClientboundEntityMetadataID})
	assertPlayerMetadata(t, aliceConnection.packets(t)[0], bob.Player.EntityID, protocol.EntityFlagSneaking, protocol.EntityPoseCrouching)

	player := bob.snapshotPlayer()
	if !player.Sneaking || player.Sprinting {
		t.Fatalf("player state after input = %+v", player)
	}

	aliceConnection.reset()

	bob.handlePlayerCommand(protocol.PlayerCommand{Action: protocol.PlayerCommandStartSprinting})

	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{protocol.ClientboundEntityMetadataID})
	assertPlayerMetadata(t, aliceConnection.packets(t)[0], bob.Player.EntityID, protocol.EntityFlagSneaking|protocol.EntityFlagSprinting, protocol.EntityPoseCrouching)

	bob.handlePlayerCommand(protocol.PlayerCommand{Action: protocol.PlayerCommandStartSprinting})

	if len(aliceConnection.packets(t)) != 1 {
		t.Fatal("unchanged sprinting state was broadcast")
	}

	aliceConnection.reset()

	bob.handleSwingArm(protocol.SwingArm{Hand: protocol.MainHand})
	bob.handleSwingArm(protocol.SwingArm{Hand: protocol.OffHand})

	packets := aliceConnection.packets(t)

	assertPacketIDs(t, aliceConnection.packetIDs(t), []int32{
		protocol.ClientboundEntityAnimationID,
		protocol.ClientboundEntityAnimationID,
	})

	assertPlayerAnimation(t, packets[0], bob.Player.EntityID, protocol.EntityAnimationSwingMainHand)
	assertPlayerAnimation(t, packets[1], bob.Player.EntityID, protocol.EntityAnimationSwingOffHand)

	assertPacketIDs(t, bobConnection.packetIDs(t), nil)
}

func TestJoiningPlayerReceivesCurrentPlayerState(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	bob, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Bob")

	joinTestSession(t, runtime, bob)

	bob.handlePlayerInput(protocol.PlayerInput{Flags: protocol.PlayerInputSneak})
	bob.handlePlayerCommand(protocol.PlayerCommand{Action: protocol.PlayerCommandStartSprinting})

	alice, aliceConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Alice")

	joinTestSession(t, runtime, alice)

	for _, packet := range aliceConnection.packets(t) {
		if packet.ID != protocol.ClientboundEntityMetadataID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)
		if reader.VarInt() != bob.Player.EntityID {
			continue
		}

		assertPlayerMetadata(t, packet, bob.Player.EntityID, protocol.EntityFlagSneaking|protocol.EntityFlagSprinting, protocol.EntityPoseCrouching)

		return
	}

	t.Fatal("joining player did not receive Bob's current metadata")
}

func joinTestSession(t *testing.T, runtime *Runtime, session *Session) {
	t.Helper()

	runtime.AssignEntityID(session)

	err := runtime.JoinSession(session)
	if err != nil {
		t.Fatalf("join %s: %v", session.Player.Name, err)
	}
}

func assertPlayerMetadata(t *testing.T, packet protocol.Packet, entityID int32, flags byte, pose int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	if actual := reader.VarInt(); actual != entityID {
		t.Fatalf("metadata entity id = %d, want %d", actual, entityID)
	}

	if index, metadataType, actual := reader.Byte(), reader.VarInt(), reader.Byte(); index != protocol.EntityFlagsMetadataIndex || metadataType != protocol.MetadataTypeByte || actual != flags {
		t.Fatalf("flags metadata = (index=%d type=%d value=%#x), want value %#x", index, metadataType, actual, flags)
	}

	if index, metadataType, actual := reader.Byte(), reader.VarInt(), reader.VarInt(); index != protocol.EntityPoseMetadataIndex || metadataType != protocol.MetadataTypePose || actual != pose {
		t.Fatalf("pose metadata = (index=%d type=%d value=%d), want value %d", index, metadataType, actual, pose)
	}
}

func assertPlayerAnimation(t *testing.T, packet protocol.Packet, entityID int32, animation byte) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	if actual := reader.VarInt(); actual != entityID {
		t.Fatalf("animation entity id = %d, want %d", actual, entityID)
	}

	if actual := reader.Byte(); actual != animation {
		t.Fatalf("animation = %d, want %d", actual, animation)
	}

	if err := reader.Err(); err != nil {
		t.Fatalf("decode animation: %v", err)
	}
}
