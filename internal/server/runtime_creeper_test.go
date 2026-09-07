package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type creeperIgnitionTestCase struct {
	name  string
	item  game.Item
	count int32
}

func TestSpawnCreeperTracksViewerAndPublishesMetadata(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer")

	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	connection.reset()

	creeper := runtime.SpawnCreeper(game.Position{X: 1.5, Y: 4, Z: 2.5})
	if creeper == nil {
		t.Fatal("spawn creeper returned nil")
	}

	if creeper.Living.Width != .6 || creeper.Living.Height != 1.7 || creeper.Living.Health != 20 || creeper.SwellDirection != -1 {
		t.Fatalf("creeper spawn state = %+v", creeper)
	}

	floatEntry := groundMobFloatGoalEntry(creeper.Goals)
	if floatEntry == nil || floatEntry.Priority != 1 || floatEntry.Flags != runtimeGoalJump {
		t.Fatalf("creeper float registration = %+v", floatEntry)
	}

	if creeper.RuntimeEntityTrackingConfig() != (RuntimeEntityTrackingConfig{ClientRangeChunks: 8, UpdateInterval: 3, TrackDeltas: true}) || !viewer.tracksRuntimeEntity(creeper.State.ID) {
		t.Fatal("creeper tracking configuration or viewer tracking is wrong")
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})

	metadata := creeper.EntityMetadata()
	if len(metadata) != 6 || metadata[3] != (protocol.EntityMetadataEntry{Index: protocol.CreeperSwellMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(-1)}) || metadata[4] != (protocol.EntityMetadataEntry{Index: protocol.CreeperPoweredMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(false)}) || metadata[5] != (protocol.EntityMetadataEntry{Index: protocol.CreeperIgnitedMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(false)}) {
		t.Fatalf("creeper metadata = %+v", metadata)
	}
}

func TestCreeperSwellGoalUsesStrictEnterDistanceAndCancelsForLOSOrLeaveDistance(t *testing.T) {
	world := zombieGroundWorld(-1, 10, -1, 1)

	runtime := NewRuntime(world)

	creeper := runtime.SpawnCreeper(game.Position{X: .5, Y: 1, Z: .5})

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 3.5, Y: 1, Z: .5}, 20)

	goal := &creeperSwellGoal{Entity: creeper}

	creeper.GoalTarget = runtimeLivingTarget{entity: target}

	if goal.CanUse(runtime) {
		t.Fatal("creeper began swelling exactly three blocks away")
	}

	target.State.Position.X = 3.49

	if !goal.CanUse(runtime) {
		t.Fatal("creeper did not begin swelling inside three blocks")
	}

	creeper.Navigation.MoveTo([]game.Position{{X: 2.5, Y: 1, Z: .5}}, 1)

	goal.Start(runtime)

	if !creeper.Navigation.Done() || !creeper.Swell.Target.present() {
		t.Fatal("swell start did not stop navigation and retain target")
	}

	target.State.Position.X = 7.5

	goal.Tick(runtime)

	if creeper.SwellDirection != 1 {
		t.Fatal("creeper cancelled swelling exactly seven blocks away")
	}

	target.State.Position.X = 7.51

	goal.Tick(runtime)

	if creeper.SwellDirection != -1 {
		t.Fatal("creeper retained swelling beyond seven blocks")
	}

	target.State.Position.X = 2.5

	world.SetBlock(game.BlockPosition{X: 1, Y: 2, Z: 0}, game.Stone)

	goal.Tick(runtime)

	if creeper.SwellDirection != -1 {
		t.Fatal("creeper retained swelling without line of sight")
	}
}

func TestCreeperFusePrimesOnceExplodesOnThirtiethTickAndUsesPoweredRadius(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer")

	viewer.Player.Position = game.Position{}
	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	creeper := runtime.SpawnCreeper(game.Position{})

	victim := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 20)

	creeper.Powered = true
	creeper.SwellDirection = 1

	connection.reset()

	for range creeperFuseTime - 1 {
		creeper.tickFuse(runtime)
	}

	if creeper.SwellCurrent != creeperFuseTime-1 || creeper.Exploded || creeper.State.Removed || countPacketID(connection.packets(t), protocol.ClientboundSoundID) != 1 {
		t.Fatalf("fuse before explosion = current %d exploded %t removed %t packets %v", creeper.SwellCurrent, creeper.Exploded, creeper.State.Removed, connection.packetIDs(t))
	}

	creeper.tickFuse(runtime)

	if creeper.SwellCurrent != creeperFuseTime || !creeper.Exploded || !creeper.State.Removed {
		t.Fatalf("thirtieth fuse tick = current %d exploded %t removed %t", creeper.SwellCurrent, creeper.Exploded, creeper.State.Removed)
	}

	explosions := packetsByID(t, connection, protocol.ClientboundExplodeID)
	if len(explosions) != 1 || countPacketID(connection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 || countPacketID(connection.packets(t), protocol.ClientboundSoundID) != 1 {
		t.Fatalf("creeper fuse packets = %v", connection.packetIDs(t))
	}

	if victim.Living.LastDamageType != game.DamageExplosion || victim.Living.LastDamageCauseEntityID != creeper.State.ID {
		t.Fatalf("creeper damage attribution = type %v cause %d", victim.Living.LastDamageType, victim.Living.LastDamageCauseEntityID)
	}

	assertExplosionDamagePacket(t, connection, victim.State.ID, game.DamageExplosion, creeper.State.ID, creeper.State.ID)

	reader := protocol.NewPacketReader(explosions[0].Data)

	reader.Double()
	reader.Double()
	reader.Double()

	radius := reader.Float()

	err := reader.Err()
	if err != nil || radius != creeperPoweredRadius {
		t.Fatalf("powered explosion radius = %v, decode error = %v", radius, err)
	}

	creeper.tickFuse(runtime)

	if countPacketID(connection.packets(t), protocol.ClientboundExplodeID) != 1 || countPacketID(connection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 {
		t.Fatal("exploded creeper fired a second explosion or removal")
	}
}

func TestCreeperManualIgnitionUpdatesInventoryAndMetadata(t *testing.T) {
	tests := []creeperIgnitionTestCase{
		{name: "flint and steel", item: game.ItemFlintAndSteel, count: 1},
		{name: "fire charge", item: game.ItemFireCharge, count: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Igniter")

			session.Player.ResetSurvivalState()

			session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: test.count}

			joinTestSession(t, runtime, session)

			creeper := runtime.SpawnCreeper(game.Position{})
			if !creeper.RuntimeEntityInteract(runtime, session, RuntimeEntityInteraction{Hand: protocol.MainHand}) || !creeper.Ignited {
				t.Fatal("manual creeper ignition was rejected")
			}

			stack := session.snapshotPlayer().Inventory.Hotbar[0]
			if test.item == game.ItemFireCharge && (stack.Item != game.ItemFireCharge || stack.Count != 1) {
				t.Fatalf("fire charge after ignition = %+v", stack)
			}

			if test.item == game.ItemFlintAndSteel && stack.Damage() != 1 {
				t.Fatalf("flint and steel damage = %d, want 1", stack.Damage())
			}

			metadata := creeper.EntityMetadata()
			if metadata[5] != (protocol.EntityMetadataEntry{Index: protocol.CreeperIgnitedMetadataIndex, Type: protocol.MetadataTypeBoolean, Value: protocol.MetadataBoolean(true)}) {
				t.Fatalf("ignited creeper metadata = %+v", metadata)
			}
		})
	}
}

func TestCreeperInactiveChunkPausesFuse(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-1, 1, -1, 1))

	session, _ := newZombieTestSession(t, runtime, game.Position{X: 4, Z: .5})

	creeper := runtime.SpawnCreeper(game.Position{X: .5, Y: 1, Z: .5})

	creeper.SwellDirection = 1

	runtime.releaseSessionActiveChunks(session)

	runtime.Tick()

	if creeper.TickCount != 0 || creeper.SwellCurrent != 0 || creeper.Exploded {
		t.Fatalf("inactive creeper changed: ticks %d swell %d exploded %t", creeper.TickCount, creeper.SwellCurrent, creeper.Exploded)
	}
}

func TestCreeperLethalDamageUsesNormalLootAndDeathLifecycle(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-1, 2, -1, 1))

	_, connection := newZombieTestSession(t, runtime, game.Position{X: 4, Z: .5})

	runtime.entityRandom = func() float32 {
		return .9
	}

	creeper := runtime.SpawnCreeper(game.Position{X: .5, Y: 1, Z: .5})

	connection.reset()

	update, applied := runtime.damageRuntimeLivingEntityLocked(creeper, game.Damage{Type: game.DamageGenericKill, Amount: 20})
	if !applied || !update.died || !creeper.Living.Dead || creeper.Exploded || !creeper.LootDropped || creeper.State.Removed {
		t.Fatalf("lethal creeper state = applied %t update %+v dead %t exploded %t loot %t removed %t", applied, update, creeper.Living.Dead, creeper.Exploded, creeper.LootDropped, creeper.State.Removed)
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 2 {
		t.Fatalf("entities after lethal damage = %d, want creeper and loot", len(entities))
	}

	loot, ok := entities[1].(*runtimeItemEntity)
	if !ok || !loot.Stack.Equal(game.ItemStack{Item: game.ItemGunpowder, Count: 2}) {
		t.Fatalf("creeper loot = %+v", loot)
	}

	runtime.sendRuntimeLivingDamageUpdate(update)

	if countPacketID(connection.packets(t), protocol.ClientboundExplodeID) != 0 {
		t.Fatal("ordinary lethal damage exploded creeper")
	}

	for range game.LivingDeathDurationTicks {
		runtime.Tick()
	}

	if !creeper.State.Removed || len(runtime.snapshotRuntimeEntities()) != 1 {
		t.Fatal("creeper did not complete normal death removal")
	}
}
