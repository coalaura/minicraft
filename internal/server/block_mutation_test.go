package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type denyBlockMutationPolicy struct{}

func (denyBlockMutationPolicy) AllowBlockMutation(BlockMutation) bool {
	return false
}

type denyPositionBlockMutationPolicy struct {
	position game.BlockPosition
}

func (policy denyPositionBlockMutationPolicy) AllowBlockMutation(mutation BlockMutation) bool {
	return mutation.Position != policy.position
}

type blockMutationTestGenerator struct {
	block game.Block
}

func (g blockMutationTestGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return g.block
}

func TestCreativeBlockBreakingMutatesAndSynchronizesWorld(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)
	observer, observerConnection := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer", game.GameModeCreative)
	unloaded, unloadedConnection := newBlockMutationTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "Unloaded", game.GameModeCreative)

	position := game.BlockPosition{X: -1, Y: 70, Z: -1}

	actor.Player.Position = blockMutationTestPlayerPosition(position)
	observer.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, unloaded)

	actorConnection.reset()
	observerConnection.reset()
	unloadedConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionStartDestroyBlock,
		Position: position,
		Face:     1,
		Sequence: 300,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	actualB := world.BlockAt(position)
	if actualB != game.Air {
		t.Fatalf("block after break = %d, want air", actualB)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundBlockChangedAckID,
	})

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID})
	assertPacketIDs(t, unloadedConnection.packetIDs(t), nil)

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.AirBlockState)
	assertBlockUpdate(t, observerConnection.packets(t)[0], position, protocol.AirBlockState)
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 300)
}

func TestMultiBlockMutationIsAtomicAndSynchronizesEveryChange(t *testing.T) {
	world := &game.World{}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)
	observer, observerConnection := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer", game.GameModeCreative)

	first := game.BlockPosition{Y: 70}
	second := game.BlockPosition{X: 1, Y: 70}

	actor.Player.Position = blockMutationTestPlayerPosition(first)

	markPlacementChunksLoaded(actor, first, second)
	markPlacementChunksLoaded(observer, first, second)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	changes := []game.BlockChange{{Position: first, Replacement: game.Stone}, {Position: second, Replacement: game.Dirt}}

	result, err := runtime.MutateBlocks(actor, BlockMutationPlace, changes)
	if err != nil {
		t.Fatalf("mutate blocks: %v", err)
	}

	if !result.Allowed || !result.Changed || world.BlockAt(first) != game.Stone || world.BlockAt(second) != game.Dirt {
		t.Fatalf("mutation result = %+v, blocks = %d, %d", result, world.BlockAt(first), world.BlockAt(second))
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID})
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID})

	third := game.BlockPosition{X: 2, Y: 70}
	fourth := game.BlockPosition{X: 3, Y: 70}

	runtime.BlockMutationPolicy = denyPositionBlockMutationPolicy{position: fourth}

	actorConnection.reset()
	observerConnection.reset()

	result, err = runtime.MutateBlocks(actor, BlockMutationPlace, []game.BlockChange{{Position: third, Replacement: game.Stone}, {Position: fourth, Replacement: game.Stone}})
	if err != nil {
		t.Fatalf("denied mutate blocks: %v", err)
	}

	if result.Allowed || result.Changed || world.BlockAt(third) != game.Air || world.BlockAt(fourth) != game.Air {
		t.Fatalf("denied mutation was not atomic: result=%+v blocks=%d,%d", result, world.BlockAt(third), world.BlockAt(fourth))
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), nil)
	assertPacketIDs(t, observerConnection.packetIDs(t), nil)
}

func TestDeniedBlockBreakingCorrectsActorWithoutBroadcast(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	runtime.BlockMutationPolicy = denyBlockMutationPolicy{}

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)
	observer, observerConnection := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer", game.GameModeCreative)

	position := game.BlockPosition{X: 3, Y: 70, Z: 4}

	actor.Player.Position = blockMutationTestPlayerPosition(position)
	observer.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	actorConnection.reset()
	observerConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionStartDestroyBlock,
		Position: position,
		Sequence: 42,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	actualB := world.BlockAt(position)
	if actualB != game.Stone {
		t.Fatalf("block after denied break = %d, want stone", actualB)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundBlockChangedAckID,
	})

	assertPacketIDs(t, observerConnection.packetIDs(t), nil)

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 42)
}

func TestConfiguredBlockBreakingDenialCorrectsActor(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	runtime.AllowBlockBreaking = false

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

	position := game.BlockPosition{X: 3, Y: 70, Z: 4}

	actor.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	actorConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionStartDestroyBlock,
		Position: position,
		Sequence: 43,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	actualB := world.BlockAt(position)
	if actualB != game.Stone {
		t.Fatalf("block after configured denial = %d, want stone", actualB)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundBlockChangedAckID,
	})

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 43)
}

func TestNonCreativePlayerCannotBreakBlocks(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeSurvival)

	position := game.BlockPosition{X: 3, Y: 70, Z: 4}

	actor.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	actorConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionStartDestroyBlock,
		Position: position,
		Sequence: 9,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	actualB := world.BlockAt(position)
	if actualB != game.Stone {
		t.Fatalf("block after survival break = %d, want stone", actualB)
	}

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 9)
}

func TestAbortBlockBreakingOnlyAcknowledgesSequence(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

	joinTestSession(t, runtime, actor)

	actorConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionAbortDestroyBlock,
		Position: game.BlockPosition{X: 3, Y: 70, Z: 4},
		Sequence: 11,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockChangedAckID})
	assertBlockChangedAck(t, actorConnection.packets(t)[0], 11)
}

func TestUnloadedBlockBreakingIsDenied(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

	position := game.BlockPosition{X: 3, Y: 70, Z: 4}

	actor.Player.Position = blockMutationTestPlayerPosition(position)

	joinTestSession(t, runtime, actor)

	actorConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionStartDestroyBlock,
		Position: position,
		Sequence: 12,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	actualB := world.BlockAt(position)
	if actualB != game.Stone {
		t.Fatalf("unloaded block after break = %d, want stone", actualB)
	}

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 12)
}

func TestDistantBlockBreakingIsDenied(t *testing.T) {
	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

	position := game.BlockPosition{X: 7, Y: 0, Z: 0}

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	actorConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{
		Status:   protocol.PlayerActionStartDestroyBlock,
		Position: position,
		Sequence: 14,
	})

	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	actualB := world.BlockAt(position)
	if actualB != game.Stone {
		t.Fatalf("distant block after break = %d, want stone", actualB)
	}

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.StoneBlockState)
	assertBlockChangedAck(t, actorConnection.packets(t)[1], 14)
}

func TestIgnoredPlayerActionAcknowledgesSequence(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)

	joinTestSession(t, runtime, actor)

	actorConnection.reset()

	err := actor.handlePlayerAction(protocol.PlayerAction{Status: 99, Sequence: 13})
	if err != nil {
		t.Fatalf("handle player action: %v", err)
	}

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockChangedAckID})
	assertBlockChangedAck(t, actorConnection.packets(t)[0], 13)
}

func newBlockMutationTestSession(runtime *Runtime, uuid, name string, mode game.GameMode) (*Session, *recordingConnection) {
	session, connection := newMovementTestSession(runtime, uuid, name)

	session.Player.GameMode = mode

	return session, connection
}

func markChunkLoaded(session *Session, position game.BlockPosition) {
	session.loadedChunks = map[LoadedChunk]struct{}{blockLoadedChunk(position): {}}
}

func blockMutationTestPlayerPosition(position game.BlockPosition) game.Position {
	return game.Position{
		X: float64(position.X) + 0.5,
		Y: float64(position.Y) + 1,
		Z: float64(position.Z) + 0.5,
	}
}

func assertBlockUpdate(t *testing.T, packet protocol.Packet, position game.BlockPosition, state int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actualB := reader.BlockPosition()
	if actualB != position {
		t.Fatalf("block update position = %+v, want %+v", actualB, position)
	}

	actual := reader.VarInt()
	if actual != state {
		t.Fatalf("block update state = %d, want %d", actual, state)
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode block update: %v", err)
	}
}

func assertBlockChangedAck(t *testing.T, packet protocol.Packet, sequence int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actual := reader.VarInt()
	if actual != sequence {
		t.Fatalf("block change sequence = %d, want %d", actual, sequence)
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode block change acknowledgement: %v", err)
	}
}
