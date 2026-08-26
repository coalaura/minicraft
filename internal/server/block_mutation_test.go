package server

import (
	"sync"
	"testing"
	"time"

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

type blockingConnection struct {
	*recordingConnection
	writeStarted chan struct{}
	releaseWrite chan struct{}
	writeOnce    sync.Once
}

func (c *blockingConnection) Write(data []byte) (int, error) {
	c.writeOnce.Do(func() {
		close(c.writeStarted)
	})

	<-c.releaseWrite

	return c.recordingConnection.Write(data)
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

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundLevelEventID})
	assertPacketIDs(t, unloadedConnection.packetIDs(t), nil)

	assertBlockUpdate(t, actorConnection.packets(t)[0], position, protocol.AirBlockState)
	assertBlockUpdate(t, observerConnection.packets(t)[0], position, protocol.AirBlockState)
	assertLevelEvent(t, observerConnection.packets(t)[1], protocol.LevelEventBlockBreak, position, protocol.StoneBlockState, false)
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
	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID, protocol.ClientboundSoundID})
	assertSoundEvent(t, observerConnection.packets(t)[2], game.SoundBlockStonePlace)

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

func TestBulkBlockMutationUsesSectionUpdatesForLoadedChunks(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	loaded, loadedConnection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Loaded", game.GameModeCreative)
	partial, partialConnection := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Partial", game.GameModeCreative)
	unloaded, unloadedConnection := newBlockMutationTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "Unloaded", game.GameModeCreative)

	loaded.loadedChunks = map[LoadedChunk]struct{}{{X: 0, Z: 0}: {}, {X: 1, Z: 0}: {}}
	partial.loadedChunks = map[LoadedChunk]struct{}{{X: 1, Z: 0}: {}}

	joinTestSession(t, runtime, loaded)
	joinTestSession(t, runtime, partial)
	joinTestSession(t, runtime, unloaded)

	loadedConnection.reset()
	partialConnection.reset()
	unloadedConnection.reset()

	changes := make([]game.BlockChange, 0, 32)

	for blockX := int32(0); blockX < 32; blockX++ {
		changes = append(changes, game.BlockChange{Position: game.BlockPosition{X: blockX, Y: 70}, Replacement: game.Stone})
	}

	result, err := runtime.MutateWorldBlocks(changes)
	if err != nil {
		t.Fatalf("mutate world blocks: %v", err)
	}

	if !result.Changed || len(result.Changes) != len(changes) {
		t.Fatalf("bulk mutation result = %+v, want %d changes", result, len(changes))
	}

	assertPacketIDs(t, loadedConnection.packetIDs(t), []int32{protocol.ClientboundSectionBlocksUpdateID, protocol.ClientboundSectionBlocksUpdateID})
	assertPacketIDs(t, partialConnection.packetIDs(t), []int32{protocol.ClientboundSectionBlocksUpdateID})
	assertPacketIDs(t, unloadedConnection.packetIDs(t), nil)

	loadedPackets := loadedConnection.packets(t)

	assertSectionBlocksUpdate(t, loadedPackets[0], 0, 4, 0, 16)
	assertSectionBlocksUpdate(t, loadedPackets[1], 1, 4, 0, 16)
	assertSectionBlocksUpdate(t, partialConnection.packets(t)[0], 1, 4, 0, 16)
}

func TestBlockMutationReleasesLocksBeforeBroadcast(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)
	observer, _ := newBlockMutationTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer", game.GameModeCreative)

	first := game.BlockPosition{Y: 70}
	second := game.BlockPosition{X: 1, Y: 70}

	actor.Player.Position = blockMutationTestPlayerPosition(first)

	markPlacementChunksLoaded(actor, first, second)
	markPlacementChunksLoaded(observer, first, second)

	joinTestSession(t, runtime, actor)
	joinTestSession(t, runtime, observer)

	connection := &blockingConnection{
		recordingConnection: &recordingConnection{},
		writeStarted:        make(chan struct{}),
		releaseWrite:        make(chan struct{}),
	}

	observer.Conn = protocol.NewConnection(connection, nil)

	var releaseOnce sync.Once

	releaseWrite := func() {
		releaseOnce.Do(func() {
			close(connection.releaseWrite)
		})
	}

	defer releaseWrite()

	firstResult := make(chan error, 1)

	go func() {
		_, err := runtime.MutateBlock(actor, BlockMutationPlace, first, game.Stone)
		firstResult <- err
	}()

	select {
	case <-connection.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("mutation broadcast did not reach the blocked connection")
	}

	lifecycleComplete := make(chan struct{})

	go func() {
		actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 0})

		close(lifecycleComplete)
	}()

	select {
	case <-lifecycleComplete:
	case <-time.After(time.Second):
		t.Fatal("lifecycle operation blocked by mutation broadcast")
	}

	secondResult := make(chan error, 1)

	go func() {
		_, err := runtime.MutateBlock(actor, BlockMutationPlace, second, game.Dirt)
		secondResult <- err
	}()

	deadline := time.After(time.Second)

	for runtime.World.BlockAt(second) != game.Dirt {
		select {
		case <-deadline:
			t.Fatal("second mutation did not commit during blocked broadcast")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	releaseWrite()

	for _, result := range []chan error{firstResult, secondResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("mutate block: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("mutation did not finish after broadcast was released")
		}
	}
}

func TestBlockMutationBroadcastsFollowCommitOrder(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Actor", game.GameModeCreative)
	position := game.BlockPosition{Y: 70}

	actor.Player.Position = blockMutationTestPlayerPosition(position)

	markChunkLoaded(actor, position)

	joinTestSession(t, runtime, actor)

	connection.reset()

	commit := func(action BlockMutationAction, replacement game.Block) (BlockMutationResult, blockMutationDelivery, error) {
		runtime.worldMutationMu.Lock()
		defer runtime.worldMutationMu.Unlock()

		changes := []game.BlockChange{{Position: position, Replacement: replacement}}

		return runtime.mutateBlocksLocked(actor, action, changes, 1, true, false, false)
	}

	firstResult, firstDelivery, err := commit(BlockMutationPlace, game.Stone)
	if err != nil {
		t.Fatalf("commit first mutation: %v", err)
	}

	secondResult, secondDelivery, err := commit(BlockMutationInteract, game.Dirt)
	if err != nil {
		t.Fatalf("commit second mutation: %v", err)
	}

	secondComplete := make(chan error, 1)

	go func() {
		_, err := runtime.completeBlockMutation(secondResult, secondDelivery, nil)
		secondComplete <- err
	}()

	select {
	case err := <-secondComplete:
		t.Fatalf("second delivery completed before first: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	_, err = runtime.completeBlockMutation(firstResult, firstDelivery, nil)
	if err != nil {
		t.Fatalf("complete first mutation: %v", err)
	}

	select {
	case err := <-secondComplete:
		if err != nil {
			t.Fatalf("complete second mutation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second delivery did not follow first")
	}

	packets := connection.packets(t)

	dirtState, err := protocolBlockState(game.Dirt)
	if err != nil {
		t.Fatalf("encode dirt state: %v", err)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockUpdateID, protocol.ClientboundBlockUpdateID})
	assertBlockUpdate(t, packets[0], position, protocol.StoneBlockState)
	assertBlockUpdate(t, packets[1], position, dirtState)
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

func TestBlockInteractionRangeUsesPlayerEyePosition(t *testing.T) {
	position := game.BlockPosition{Y: -6}

	player := game.Player{}

	player.Pose = game.PlayerPoseStanding

	if blockWithinInteractionRange(player, position) {
		t.Fatal("standing player can reach block below range")
	}

	player.Pose = game.PlayerPoseCrouching

	if blockWithinInteractionRange(player, position) {
		t.Fatal("crouching player can reach block below range")
	}

	player.Pose = game.PlayerPoseCrawling

	if !blockWithinInteractionRange(player, position) {
		t.Fatal("crawling player cannot reach block within range")
	}
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

func assertSectionBlocksUpdate(t *testing.T, packet protocol.Packet, sectionX, sectionY, sectionZ int32, recordCount int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	packedPosition := reader.Long()
	expectedPosition := (int64(sectionX)&0x3FFFFF)<<42 | (int64(sectionZ)&0x3FFFFF)<<20 | int64(sectionY)&0xFFFFF

	if packedPosition != expectedPosition {
		t.Fatalf("section position = %#x, want %#x", packedPosition, expectedPosition)
	}

	actualCount := reader.VarInt()
	if actualCount != recordCount {
		t.Fatalf("section record count = %d, want %d", actualCount, recordCount)
	}

	for range actualCount {
		reader.VarInt()
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode section blocks update: %v", err)
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

func assertLevelEvent(t *testing.T, packet protocol.Packet, event int32, position game.BlockPosition, data int32, global bool) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actualEvent := reader.Int()
	actualPosition := reader.BlockPosition()
	actualData := reader.Int()
	actualGlobal := reader.Bool()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode level event: %v", err)
	}

	if actualEvent != event || actualPosition != position || actualData != data || actualGlobal != global {
		t.Fatalf("level event = event %d position %+v data %d global %v", actualEvent, actualPosition, actualData, actualGlobal)
	}
}

func assertSoundEvent(t *testing.T, packet protocol.Packet, event game.SoundEvent) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actualEvent := reader.VarInt() - 1
	actualSource := reader.VarInt()

	reader.Int()
	reader.Int()
	reader.Int()
	reader.Float()
	reader.Float()
	reader.Long()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode sound: %v", err)
	}

	if actualEvent != int32(event) || actualSource != protocol.SoundSourceBlock {
		t.Fatalf("sound = event %d source %d, want event %d source %d", actualEvent, actualSource, event, protocol.SoundSourceBlock)
	}
}
