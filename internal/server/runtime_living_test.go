package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type testRuntimeLivingEntity struct {
	State     RuntimeEntityState
	Living    RuntimeLivingState
	TickCount int32
}

type runtimeLivingAttackDamageTest struct {
	name         string
	attackTicker int32
	fallDistance float32
	onGround     bool
	wantHealth   float32
	wantCritical bool
}

func (entity *testRuntimeLivingEntity) RuntimeEntityState() *RuntimeEntityState {
	return &entity.State
}

func (entity *testRuntimeLivingEntity) RuntimeLivingState() *RuntimeLivingState {
	return &entity.Living
}

func (entity *testRuntimeLivingEntity) RuntimeEntityTrackingConfig() RuntimeEntityTrackingConfig {
	return RuntimeEntityTrackingConfig{ClientRangeChunks: 6, UpdateInterval: 1, TrackDeltas: true}
}

func (entity *testRuntimeLivingEntity) RuntimeEntityView() runtimeEntityView {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.runtimeEntityViewLocked()
}

func (entity *testRuntimeLivingEntity) runtimeEntityViewLocked() runtimeEntityView {
	return runtimeEntityView{
		ID:       entity.State.ID,
		UUID:     entity.State.UUID,
		Position: entity.State.Position,
		Chunk:    entity.State.Chunk,
		Removed:  entity.State.Removed,
		Velocity: entity.Living.Velocity,
		OnGround: entity.Living.OnGround,
	}
}

func (entity *testRuntimeLivingEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return []protocol.EntityMetadataEntry{{Index: protocol.LivingHealthMetadataIndex, Type: protocol.MetadataTypeFloat, Value: protocol.MetadataFloat(entity.Living.Health)}}
}

func (entity *testRuntimeLivingEntity) EntityVelocity() game.Velocity {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.Living.Velocity
}

func (entity *testRuntimeLivingEntity) AddEntityPacket(snapshot runtimeEntitySpawnSnapshot) protocol.AddEntity {
	return protocol.AddEntity{
		EntityID: snapshot.ID,
		UUID:     snapshot.UUID,
		Type:     protocol.ItemEntityType,
		X:        snapshot.Position.X,
		Y:        snapshot.Position.Y,
		Z:        snapshot.Position.Z,
	}
}

func (entity *testRuntimeLivingEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	entity.State.mu.Lock()

	if !entity.State.Removed {
		entity.TickCount++
	}

	entity.State.mu.Unlock()

	runtime.tickRuntimeLivingEntity(entity)
}

func TestPlayerAttackRuntimeLivingUsesSharedCombat(t *testing.T) {
	runtime, attacker, _, attackerConnection, _ := newPlayerCombatTest(t)

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 20)

	attackerConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

	entity.State.mu.RLock()
	health := entity.Living.Health
	velocity := entity.Living.Velocity
	entity.State.mu.RUnlock()

	if health != 13 {
		t.Fatalf("runtime living health = %v, want 13", health)
	}

	if velocity.X != 0.4 || velocity.Y != 0.4 || velocity.Z != 0 {
		t.Fatalf("runtime living velocity = %+v, want {X:0.4 Y:0.4}", velocity)
	}

	if attacker.snapshotPlayer().Inventory.Hotbar[0].Damage() != 1 {
		t.Fatal("successful runtime living attack did not charge weapon durability")
	}

	packets := attackerConnection.packets(t)
	if countPacketID(packets, protocol.ClientboundDamageEventID) != 1 || countPacketID(packets, protocol.ClientboundSetEntityMotionID) != 1 || countPacketID(packets, protocol.ClientboundEntityMetadataID) != 1 {
		t.Fatalf("runtime hurt packet IDs = %v", attackerConnection.packetIDs(t))
	}

	assertRuntimeLivingDamagePacket(t, attackerConnection, entity.State.ID, attacker.snapshotPlayer().EntityID)
	assertRuntimeLivingHealthMetadata(t, attackerConnection, entity.State.ID, 13)
}

func TestRuntimeLivingAttackStrengthAndCriticalDamage(t *testing.T) {
	tests := []runtimeLivingAttackDamageTest{
		{name: "weak", attackTicker: 0, onGround: true, wantHealth: 18.59104},
		{name: "full", attackTicker: 20, onGround: true, wantHealth: 13},
		{name: "critical", attackTicker: 20, fallDistance: 1, onGround: false, wantHealth: 9.5, wantCritical: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, attacker, _, attackerConnection, _ := newPlayerCombatTest(t)

			entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 20)

			attacker.updatePlayerState(func(player *game.Player) bool {
				player.AttackStrengthTicker = test.attackTicker
				player.FallDistance = test.fallDistance
				player.OnGround = test.onGround

				return true
			})

			attackerConnection.reset()

			runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

			assertLivingHealth(t, entity, test.wantHealth)

			criticalPackets := countPacketID(attackerConnection.packets(t), protocol.ClientboundEntityAnimationID)
			if (criticalPackets == 1) != test.wantCritical {
				t.Fatalf("critical animation packets = %d, want critical %t", criticalPackets, test.wantCritical)
			}
		})
	}
}

func TestRuntimeLivingAttackRejection(t *testing.T) {
	runtime, attacker, _, _, _ := newPlayerCombatTest(t)

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 8}, 20)

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

	if entity.Living.Health != 20 || attacker.snapshotPlayer().Inventory.Hotbar[0].Damage() != 0 {
		t.Fatal("out-of-range runtime living attack changed combat state")
	}

	item := runtime.SpawnItemEntity(game.ItemStack{Item: game.ItemStone, Count: 1}, game.Position{X: 1}, game.Velocity{}, 0)

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: item.State.ID, Action: protocol.InteractActionAttack})

	if item.Health != 5 || attacker.snapshotPlayer().Inventory.Hotbar[0].Damage() != 0 {
		t.Fatal("arbitrary runtime entity was attackable")
	}
}

func TestRuntimeLivingKnockbackResistanceAndSprint(t *testing.T) {
	runtime, attacker, _, _, _ := newPlayerCombatTest(t)

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 20)

	entity.Living.KnockbackResistance = 0.5

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.Sprinting = true
		player.Rotation.Yaw = -90

		return true
	})

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

	entity.State.mu.RLock()
	velocity := entity.Living.Velocity
	entity.State.mu.RUnlock()

	if math.Abs(velocity.X-0.35) > 1e-9 || math.Abs(velocity.Y-0.35) > 1e-9 || math.Abs(velocity.Z) > 1e-9 {
		t.Fatalf("resistant sprint velocity = %+v, want {X:0.35 Y:0.35}", velocity)
	}
}

func TestRuntimeLivingHurtPacketsOnlyReachTrackingViewers(t *testing.T) {
	runtime, attacker, target, attackerConnection, targetConnection := newPlayerCombatTest(t)

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 20)

	target.untrackRuntimeEntity(entity.State.ID)

	attackerConnection.reset()
	targetConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

	if countPacketID(attackerConnection.packets(t), protocol.ClientboundDamageEventID) != 1 {
		t.Fatal("tracking attacker did not receive runtime damage event")
	}

	if countPacketID(targetConnection.packets(t), protocol.ClientboundDamageEventID) != 0 || countPacketID(targetConnection.packets(t), protocol.ClientboundEntityMetadataID) != 0 {
		t.Fatalf("untracked viewer received runtime hurt packets: %v", targetConnection.packetIDs(t))
	}
}

func TestRuntimeLivingDeathTimingActivityAndRemoval(t *testing.T) {
	runtime, attacker, _, attackerConnection, _ := newPlayerCombatTest(t)

	runtime.setSessionActiveChunks(attacker, []LoadedChunk{{}})

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 1)

	attackerConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

	if !entity.Living.Dead || entity.Living.DeathTime != 0 {
		t.Fatalf("lethal attack state = dead %t death time %d", entity.Living.Dead, entity.Living.DeathTime)
	}

	durability := attacker.snapshotPlayer().Inventory.Hotbar[0].Damage()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: entity.State.ID, Action: protocol.InteractActionAttack})

	if attacker.snapshotPlayer().Inventory.Hotbar[0].Damage() != durability {
		t.Fatal("dead runtime living entity accepted another attack")
	}

	for range game.LivingDeathDurationTicks - 1 {
		runtime.Tick()
	}

	if entity.Living.DeathTime != game.LivingDeathDurationTicks-1 || entity.State.Removed {
		t.Fatalf("runtime living removed early at death time %d", entity.Living.DeathTime)
	}

	runtime.releaseSessionActiveChunks(attacker)

	runtime.Tick()

	if entity.Living.DeathTime != game.LivingDeathDurationTicks-1 {
		t.Fatal("inactive runtime living death lifecycle advanced")
	}

	runtime.setSessionActiveChunks(attacker, []LoadedChunk{{}})

	runtime.Tick()

	if !entity.State.Removed || len(runtime.snapshotRuntimeEntities()) != 0 || attacker.tracksRuntimeEntity(entity.State.ID) {
		t.Fatal("completed runtime living death remained authoritative or tracked")
	}

	if countPacketID(attackerConnection.packets(t), protocol.ClientboundEntityEventID) != 2 || countPacketID(attackerConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 {
		t.Fatalf("runtime death packet IDs = %v", attackerConnection.packetIDs(t))
	}

	assertEntityEvents(t, attackerConnection, entity.State.ID, []byte{runtimeLivingDeathEvent, runtimeLivingPoofEvent})
}

func TestRuntimeLivingCrossesChunksOncePerTick(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}, {X: 1}})

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 15.9}, 20)

	entity.Living.Velocity.X = 0.2

	runtime.Tick()

	if entity.TickCount != 1 || entity.State.Chunk != (LoadedChunk{X: 1}) {
		t.Fatalf("crossing runtime living tick count %d chunk %+v", entity.TickCount, entity.State.Chunk)
	}
}

func spawnTestRuntimeLivingEntity(runtime *Runtime, position game.Position, maxHealth float32) *testRuntimeLivingEntity {
	entity := &testRuntimeLivingEntity{
		State: RuntimeEntityState{
			ID:       runtime.allocateEntityID(),
			UUID:     randomEntityUUID(),
			Position: position,
			Chunk:    positionLoadedChunk(position),
		},
		Living: RuntimeLivingState{OnGround: true, Width: 0.6, Height: 1.8},
	}

	entity.Living.Reset(maxHealth)

	entity.State.tracker = newRuntimeEntityTracker(entity.runtimeEntityViewLocked())

	runtime.entityMu.Lock()
	runtime.entities[entity.State.ID] = entity

	runtime.addEntityToChunkIndexLocked(entity)
	runtime.entityMu.Unlock()

	chunk, active := runtime.ActiveChunk(entity.State.Chunk)
	if active {
		chunk.SetEntity(entity.State.ID, entity)
	}

	runtime.reconcileRuntimeEntityTracking(entity)

	return entity
}

func assertLivingHealth(t *testing.T, entity *testRuntimeLivingEntity, expected float32) {
	t.Helper()

	entity.State.mu.RLock()
	health := entity.Living.Health
	entity.State.mu.RUnlock()

	if math.Abs(float64(health-expected)) > 1e-4 {
		t.Fatalf("runtime living health = %v, want %v", health, expected)
	}
}

func assertRuntimeLivingDamagePacket(t *testing.T, connection *recordingConnection, entityID, attackerID int32) {
	t.Helper()

	packets := packetsByID(t, connection, protocol.ClientboundDamageEventID)
	if len(packets) != 1 {
		t.Fatalf("damage event packets = %d, want 1", len(packets))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	actualEntityID := reader.VarInt()
	damageType := reader.VarInt()
	causeEntityID := reader.VarInt()
	directEntityID := reader.VarInt()
	hasSourcePosition := reader.Bool()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode runtime living damage event: %v", err)
	}

	protocolAttackerID := protocolEntityID(attackerID)

	if actualEntityID != entityID || damageType != game.DamagePlayerAttack.Traits().RegistryID || causeEntityID != protocolAttackerID || directEntityID != protocolAttackerID || hasSourcePosition {
		t.Fatalf("runtime living damage event = entity %d type %d cause %d direct %d source %t", actualEntityID, damageType, causeEntityID, directEntityID, hasSourcePosition)
	}
}

func assertRuntimeLivingHealthMetadata(t *testing.T, connection *recordingConnection, entityID int32, expected float32) {
	t.Helper()

	packets := packetsByID(t, connection, protocol.ClientboundEntityMetadataID)
	if len(packets) != 1 {
		t.Fatalf("living metadata packets = %d, want 1", len(packets))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	actualEntityID := reader.VarInt()
	index := reader.Byte()
	metadataType := reader.VarInt()
	health := reader.Float()
	terminator := reader.Byte()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode runtime living metadata: %v", err)
	}

	if actualEntityID != entityID || index != protocol.LivingHealthMetadataIndex || metadataType != protocol.MetadataTypeFloat || health != expected || terminator != 0xff {
		t.Fatalf("runtime living metadata = entity %d index %d type %d health %v terminator %#x", actualEntityID, index, metadataType, health, terminator)
	}
}
