package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type tntTrajectorySnapshot struct {
	position game.Position
	velocity game.Velocity
}

type tntIgnitionTestCase struct {
	name      string
	item      game.Item
	count     int32
	wantCount int32
}

func TestSpawnTntMetadataTrackingAndPhysicsTrace(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	tnt := runtime.SpawnTnt(game.Position{Y: 10, Z: .5}, game.Velocity{X: .3, Y: .2, Z: -.1}, 42)
	if tnt.RuntimeEntityTrackingConfig() != (RuntimeEntityTrackingConfig{ClientRangeChunks: 10, UpdateInterval: 10, TrackDeltas: true}) {
		t.Fatalf("TNT tracking config = %+v", tnt.RuntimeEntityTrackingConfig())
	}

	packet := tnt.AddEntityPacket(runtimeEntitySpawnSnapshot{ID: tnt.State.ID, UUID: tnt.State.UUID, Position: tnt.State.Position, Velocity: tnt.Velocity})
	if packet.Type != int32(game.EntityTnt) || packet.VelocityX != .3 || packet.VelocityY != .2 || packet.VelocityZ != -.1 {
		t.Fatalf("TNT spawn packet = %+v", packet)
	}

	metadata := tnt.EntityMetadata()
	if len(metadata) != 2 || metadata[0] != (protocol.EntityMetadataEntry{Index: protocol.TntFuseMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(tntDefaultFuse)}) || metadata[1] != (protocol.EntityMetadataEntry{Index: protocol.TntBlockStateMetadataIndex, Type: protocol.MetadataTypeBlockState, Value: protocol.MetadataVarInt(game.Tnt)}) {
		t.Fatalf("TNT metadata = %+v", metadata)
	}

	snapshots := []tntTrajectorySnapshot{
		{position: game.Position{X: .3, Y: 10.16, Z: .4}, velocity: game.Velocity{X: .294, Y: .1568, Z: -.098}},
		{position: game.Position{X: .594, Y: 10.2768, Z: .302}, velocity: game.Velocity{X: .28812, Y: .114464, Z: -.09604}},
	}

	for tick, snapshot := range snapshots {
		runtime.Tick()

		assertPositionClose(t, tnt.State.Position, snapshot.position, 1e-12)
		assertVelocityClose(t, tnt.Velocity, snapshot.velocity, 1e-12)

		wantFuse := tntDefaultFuse - int32(tick) - 1
		if tnt.Fuse != wantFuse {
			t.Fatalf("TNT fuse after tick %d = %d, want %d", tick+1, tnt.Fuse, wantFuse)
		}
	}
}

func TestTntInactiveChunkPausesAndDefaultFuseExplodesOnce(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	viewer, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer", game.GameModeSpectator)

	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	tnt := runtime.SpawnTnt(game.Position{X: .5, Y: 1, Z: .5}, game.Velocity{}, 0)

	connection.reset()

	runtime.Tick()

	if tnt.Fuse != tntDefaultFuse || tnt.State.Position != (game.Position{X: .5, Y: 1, Z: .5}) {
		t.Fatalf("inactive TNT changed: fuse %d position %+v", tnt.Fuse, tnt.State.Position)
	}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	for range tntDefaultFuse - 1 {
		runtime.Tick()
	}

	if tnt.Fuse != 1 || tnt.State.Removed {
		t.Fatalf("TNT before final fuse tick = fuse %d removed %t", tnt.Fuse, tnt.State.Removed)
	}

	runtime.Tick()

	if tnt.Fuse != 0 || !tnt.State.Removed {
		t.Fatalf("TNT final fuse tick = fuse %d removed %t", tnt.Fuse, tnt.State.Removed)
	}

	for _, entity := range runtime.snapshotRuntimeEntities() {
		if entity.RuntimeEntityState().ID == tnt.State.ID {
			t.Fatal("exploded TNT remains registered")
		}
	}

	if countPacketID(connection.packets(t), protocol.ClientboundExplodeID) != 1 || countPacketID(connection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 {
		t.Fatalf("TNT removal packets = %v", connection.packetIDs(t))
	}

	runtime.Tick()

	if countPacketID(connection.packets(t), protocol.ClientboundExplodeID) != 1 || countPacketID(connection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 {
		t.Fatal("removed TNT exploded or untracked twice")
	}
}

func TestPlayerOwnedTntUsesPlayerExplosionAndDamagesOwner(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	owner, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Owner", game.GameModeSurvival)

	owner.Player.ResetSurvivalState()

	owner.Player.Position = game.Position{X: 1.5, Y: 1, Z: .5}
	owner.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, owner)

	runtime.setSessionActiveChunks(owner, []LoadedChunk{{}})

	tnt := runtime.SpawnTnt(game.Position{X: .5, Y: 1, Z: .5}, game.Velocity{}, owner.Player.EntityID)

	tnt.Fuse = 1

	connection.reset()

	runtime.Tick()

	player := owner.snapshotPlayer()
	if player.Health >= 20 {
		t.Fatalf("TNT owner health = %v, want self-damage", player.Health)
	}

	assertExplosionDamagePacket(t, connection, owner.Player.EntityID, game.DamagePlayerExplosion, owner.Player.EntityID, tnt.State.ID)
}

func TestTntBlockIgnitionPreservesOwnerAndConsumesItem(t *testing.T) {
	tests := []tntIgnitionTestCase{
		{name: "flint and steel", item: game.ItemFlintAndSteel, count: 1, wantCount: 1},
		{name: "fire charge", item: game.ItemFireCharge, count: 2, wantCount: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position := game.BlockPosition{Y: 70}

			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			owner, _ := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Owner", game.GameModeSurvival)

			owner.Player.ResetSurvivalState()

			owner.Player.Position = blockMutationTestPlayerPosition(position)
			owner.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: test.count}

			markChunkLoaded(owner, position)

			joinTestSession(t, runtime, owner)

			world.SetBlock(position, game.Tnt)

			interaction := testUseItemOn(position, protocol.BlockFaceUp, protocol.MainHand, 1)

			handled, result, _, err := runtime.UseHeldItemOnBlock(owner, interaction, test.item)
			if err != nil || !handled || !result.Changed || world.BlockAt(position) != game.Air {
				t.Fatalf("ignite TNT = handled %t result %+v block %d err %v", handled, result, world.BlockAt(position), err)
			}

			entities := runtime.snapshotRuntimeEntities()
			if len(entities) != 1 {
				t.Fatalf("primed entities = %d, want 1", len(entities))
			}

			primed, valid := entities[0].(*runtimeTntEntity)
			if !valid || primed.OwnerID != owner.Player.EntityID || primed.Fuse != tntDefaultFuse {
				t.Fatalf("primed TNT = %#v", entities[0])
			}

			stack := owner.snapshotPlayer().Inventory.Hotbar[0]
			if stack.Count != test.wantCount {
				t.Fatalf("held item count = %d, want %d", stack.Count, test.wantCount)
			}

			if test.item == game.ItemFlintAndSteel && stack.Damage() != 1 {
				t.Fatalf("flint and steel damage = %d, want 1", stack.Damage())
			}
		})
	}
}

func TestExplosionChainPrimesTntWithShortFuseAndCause(t *testing.T) {
	world := &game.World{}
	position := game.BlockPosition{}

	world.SetBlock(position, game.Tnt)

	runtime := NewRuntime(world)

	random := func() float32 {
		return 0
	}

	result := runtime.Explode(RuntimeExplosion{
		Position:         game.Position{X: .5, Y: .5, Z: .5},
		Radius:           2,
		CauseEntityID:    42,
		BlockInteraction: ExplosionDestroyBlocks,
		Random:           random,
	})

	if !containsBlockPosition(result.DestroyedBlocks, position) || world.BlockAt(position) != game.Air {
		t.Fatalf("chain explosion destroyed = %+v block %d", result.DestroyedBlocks, world.BlockAt(position))
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 1 {
		t.Fatalf("chain primed entities = %d, want 1", len(entities))
	}

	primed, valid := entities[0].(*runtimeTntEntity)
	if !valid || primed.OwnerID != 42 || primed.Fuse != 10 {
		t.Fatalf("chain primed TNT = %#v", entities[0])
	}
}

func assertExplosionDamagePacket(t *testing.T, connection *recordingConnection, entityID int32, damageType game.DamageType, causeEntityID, directEntityID int32) {
	t.Helper()

	for _, packet := range packetsByID(t, connection, protocol.ClientboundDamageEventID) {
		reader := protocol.NewPacketReader(packet.Data)

		actualEntityID := reader.VarInt()
		actualDamageType := reader.VarInt()
		actualCauseEntityID := reader.VarInt()
		actualDirectEntityID := reader.VarInt()
		hasSourcePosition := reader.Bool()

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode explosion damage event: %v", err)
		}

		if actualEntityID != entityID {
			continue
		}

		if actualDamageType != damageType.Traits().RegistryID || actualCauseEntityID != protocolEntityID(causeEntityID) || actualDirectEntityID != protocolEntityID(directEntityID) || !hasSourcePosition {
			t.Fatalf("explosion damage event = entity %d type %d cause %d direct %d source %t", actualEntityID, actualDamageType, actualCauseEntityID, actualDirectEntityID, hasSourcePosition)
		}

		return
	}

	t.Fatalf("no explosion damage event for entity %d", entityID)
}
