package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type zombieTargetTestCase struct {
	name       string
	gameMode   game.GameMode
	dead       bool
	wantTarget bool
}

type zombiePlayerAttackDamageTestCase struct {
	name         string
	attackTicker int32
	fallDistance float32
	onGround     bool
	wantHealth   float32
	wantCritical bool
}

func TestSpawnZombieTracksLoadedViewerWithZombieDefinition(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	loaded, loadedConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Loaded")
	unloaded, unloadedConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Unloaded")

	loaded.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, loaded)
	joinTestSession(t, runtime, unloaded)

	loadedConnection.reset()
	unloadedConnection.reset()

	zombie := runtime.SpawnZombie(game.Position{X: 1.5, Y: 4, Z: 2.5})

	if zombie == nil {
		t.Fatal("spawn zombie returned nil")
	}

	if zombie.Living.Width != 0.6 || zombie.Living.Height != 1.95 || zombie.Living.Health != 20 || zombie.Living.Armor != 2 {
		t.Fatalf("zombie definition = width %v height %v health %v armor %d", zombie.Living.Width, zombie.Living.Height, zombie.Living.Health, zombie.Living.Armor)
	}

	assertPacketIDs(t, loadedConnection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID})

	if len(unloadedConnection.packets(t)) != 0 || !loaded.tracksRuntimeEntity(zombie.State.ID) || unloaded.tracksRuntimeEntity(zombie.State.ID) {
		t.Fatal("zombie tracking did not match loaded viewers")
	}

	packet := packetsByID(t, loadedConnection, protocol.ClientboundAddEntityID)[0]

	reader := protocol.NewPacketReader(packet.Data)

	entityID := reader.VarInt()

	reader.UUID()

	entityType := reader.VarInt()

	x := reader.Double()
	y := reader.Double()
	z := reader.Double()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode zombie add entity: %v", err)
	}

	if entityID != zombie.State.ID || entityType != 150 || x != 1.5 || y != 4 || z != 2.5 {
		t.Fatalf("zombie add entity = id %d type %d position (%v, %v, %v)", entityID, entityType, x, y, z)
	}

	metadataPacket := packetsByID(t, loadedConnection, protocol.ClientboundEntityMetadataID)[0]
	metadataReader := protocol.NewPacketReader(metadataPacket.Data)

	if metadataReader.VarInt() != zombie.State.ID || metadataReader.Byte() != protocol.LivingHealthMetadataIndex || metadataReader.VarInt() != protocol.MetadataTypeFloat || metadataReader.Float() != 20 || metadataReader.Byte() != protocol.MobFlagsMetadataIndex || metadataReader.VarInt() != protocol.MetadataTypeByte || metadataReader.Byte() != 0 || metadataReader.Byte() != 0xff {
		t.Fatal("zombie spawn metadata did not contain health and idle mob flags")
	}
}

func TestZombieInactiveChunkPausesMovementAttackAndDeath(t *testing.T) {
	world := zombieGroundWorld(-1, 4, -1, 1)

	runtime := NewRuntime(world)

	session, _ := newZombieTestSession(t, runtime, game.Position{X: 1.1, Y: 0, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 1, Z: 0.5})

	runtime.Tick()

	position := zombie.State.Position
	health := session.snapshotPlayer().Health
	cooldown := zombie.AttackCooldown

	runtime.releaseSessionActiveChunks(session)

	runtime.Tick()

	if zombie.State.Position != position || session.snapshotPlayer().Health != health || zombie.AttackCooldown != cooldown || zombie.TickCount != 1 {
		t.Fatalf("inactive zombie changed: position %+v health %v cooldown %d ticks %d", zombie.State.Position, session.snapshotPlayer().Health, zombie.AttackCooldown, zombie.TickCount)
	}

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	zombie.Living.Dead = true
	zombie.Living.DeathTime = 0

	runtime.Tick()

	if zombie.Living.DeathTime != 1 {
		t.Fatalf("reactivated zombie death time = %d, want 1", zombie.Living.DeathTime)
	}
}

func TestZombieTargetSelectionRejectsInvalidPlayers(t *testing.T) {
	tests := []zombieTargetTestCase{
		{name: "survival", wantTarget: true},
		{name: "adventure", gameMode: game.GameModeAdventure, wantTarget: true},
		{name: "creative", gameMode: game.GameModeCreative},
		{name: "spectator", gameMode: game.GameModeSpectator},
		{name: "dead", dead: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Target")

			session.Player.Position = game.Position{X: 2}
			session.Player.GameMode = test.gameMode
			session.Player.Dead = test.dead

			joinTestSession(t, runtime, session)

			zombie := runtime.SpawnZombie(game.Position{})

			target := runtime.zombieTarget(zombie)
			if (target == session) != test.wantTarget {
				t.Fatalf("target = %p, want session %t", target, test.wantTarget)
			}
		})
	}

	runtime := NewRuntime(&game.World{})

	valid, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Valid")
	creative, _ := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Creative")

	valid.Player.Position = game.Position{X: 5}

	creative.Player.Position = game.Position{X: 1}
	creative.Player.GameMode = game.GameModeCreative

	joinTestSession(t, runtime, valid)
	joinTestSession(t, runtime, creative)

	zombie := runtime.SpawnZombie(game.Position{})

	if runtime.zombieTarget(zombie) != valid {
		t.Fatal("zombie did not select nearest valid target")
	}

	valid.Player.GameMode = game.GameModeSpectator

	if runtime.zombieTarget(zombie) != nil || zombie.Target != nil {
		t.Fatal("zombie retained an invalid target")
	}

	valid.Player.GameMode = game.GameModeSurvival

	if runtime.zombieTarget(zombie) != valid {
		t.Fatal("zombie did not reacquire valid target")
	}

	runtime.LeaveSession(valid)

	if runtime.zombieTarget(zombie) != nil || zombie.Target != nil {
		t.Fatal("zombie retained a disconnected target")
	}
}

func TestGroundNavigationSupportsChaseCollisionRoutingAndTerrain(t *testing.T) {
	world := zombieGroundWorld(-2, 6, -2, 2)

	runtime := NewRuntime(world)

	path := runtime.findGroundPath(game.Position{X: 0.5, Z: 0.5}, game.Position{X: 4.5, Z: 0.5}, 0.6, 1.95, 35)

	if len(path) == 0 || path[len(path)-1] != (game.Position{X: 4.5, Z: 0.5}) {
		t.Fatalf("unobstructed path = %v", path)
	}

	world.SetBlock(game.BlockPosition{X: 2, Z: 0}, game.Stone)
	world.SetBlock(game.BlockPosition{X: 2, Y: 1, Z: 0}, game.Stone)

	path = runtime.findGroundPath(game.Position{X: 0.5, Z: 0.5}, game.Position{X: 4.5, Z: 0.5}, 0.6, 1.95, 35)

	if len(path) == 0 {
		t.Fatal("wall prevented all routing")
	}

	routed := false

	for _, waypoint := range path {
		if waypoint.Z != 0.5 {
			routed = true
		}
	}

	if !routed {
		t.Fatalf("wall path did not route around obstacle: %v", path)
	}

	world.SetBlock(game.BlockPosition{X: 2, Z: 0}, game.Air)
	world.SetBlock(game.BlockPosition{X: 2, Y: 1, Z: 0}, game.Air)
	world.SetBlock(game.BlockPosition{X: 2, Z: 0}, game.Stone)

	path = runtime.findGroundPath(game.Position{X: 0.5, Z: 0.5}, game.Position{X: 4.5, Y: 1, Z: 0.5}, 0.6, 1.95, 35)

	steppedUp := false

	for _, waypoint := range path {
		if waypoint.Y == 1 {
			steppedUp = true
		}
	}

	if len(path) == 0 || !steppedUp {
		t.Fatalf("one-block terrain path = %v", path)
	}

	world.SetBlock(game.BlockPosition{X: 2, Y: 1, Z: 0}, game.Stone)

	movement := runtime.moveGroundEntity(game.Position{X: 0.5, Y: 1, Z: 0.5}, game.Velocity{X: 3, Y: -0.08}, 0.6, 1.95, 0.6, true)

	if movement.Position.X > 1.7 || !movement.HorizontalCollisionX {
		t.Fatalf("wall collision = %+v", movement)
	}
}

func TestZombieFollowsGroundPathAroundWall(t *testing.T) {
	world := zombieGroundWorld(-1, 6, -2, 2)

	world.SetBlock(game.BlockPosition{X: 2, Z: 0}, game.Stone)
	world.SetBlock(game.BlockPosition{X: 2, Y: 1, Z: 0}, game.Stone)

	runtime := NewRuntime(world)

	newZombieTestSession(t, runtime, game.Position{X: 4.5, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	maximumDeviation := 0.0

	for range 100 {
		runtime.Tick()

		maximumDeviation = max(maximumDeviation, math.Abs(zombie.State.Position.Z-0.5))
	}

	if zombie.State.Position.X <= 2.5 || maximumDeviation < 0.4 {
		t.Fatalf("zombie wall route = position %+v maximum z deviation %v", zombie.State.Position, maximumDeviation)
	}
}

func TestZombieTraversesOneBlockStep(t *testing.T) {
	world := zombieGroundWorld(-1, 6, -1, 1)

	for x := int32(2); x <= 5; x++ {
		world.SetBlock(game.BlockPosition{X: x, Z: 0}, game.Stone)
	}

	runtime := NewRuntime(world)

	newZombieTestSession(t, runtime, game.Position{X: 4.5, Y: 1, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	maximumHeight := 0.0

	for range 100 {
		runtime.Tick()
		maximumHeight = max(maximumHeight, zombie.State.Position.Y)
	}

	if zombie.State.Position.X <= 2.5 || maximumHeight < 1 {
		t.Fatalf("zombie step traversal = position %+v maximum height %v", zombie.State.Position, maximumHeight)
	}
}

func TestZombieLineOfSightUsesBlockCollisionShapes(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{X: 1, Y: 1, Z: 0}, game.GlassPane)

	runtime := NewRuntime(world)

	from := game.Position{X: 0.5, Z: 0.5}
	to := game.Position{X: 2.5, Z: 0.5}

	if runtime.zombieHasLineOfSight(from, to) {
		t.Fatal("zombie line of sight passed through glass pane collision")
	}

	world.SetBlock(game.BlockPosition{X: 1, Y: 1, Z: 0}, game.Air)

	if !runtime.zombieHasLineOfSight(from, to) {
		t.Fatal("zombie line of sight remained blocked after removing pane")
	}
}

func TestZombieGravityChaseAndChunkMigration(t *testing.T) {
	world := zombieGroundWorld(-1, 20, -1, 1)

	runtime := NewRuntime(world)

	newZombieTestSession(t, runtime, game.Position{X: 6.5, Y: 0, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 3, Z: 0.5})

	runtime.Tick()
	runtime.Tick()

	if zombie.State.Position.Y >= 3 || zombie.Living.Velocity.Y >= 0 {
		t.Fatalf("zombie gravity = position %+v velocity %+v", zombie.State.Position, zombie.Living.Velocity)
	}

	for range 20 {
		runtime.Tick()
	}

	if zombie.State.Position.X <= 0.5 {
		t.Fatalf("zombie did not chase unobstructed target: position %+v", zombie.State.Position)
	}

	migrationRuntime := NewRuntime(zombieGroundWorld(-1, 20, -1, 1))

	migrationViewer := &Session{}

	migrationRuntime.setSessionActiveChunks(migrationViewer, []LoadedChunk{{}, {X: 1}})

	migrating := migrationRuntime.SpawnZombie(game.Position{X: 15.9, Y: 1, Z: 0.5})

	migrating.Living.Velocity = game.Velocity{X: 0.2}

	migrationRuntime.Tick()

	if migrating.TickCount != 1 || migrating.State.Chunk != (LoadedChunk{X: 1}) {
		t.Fatalf("zombie migration = ticks %d chunk %+v", migrating.TickCount, migrating.State.Chunk)
	}

	first, _ := migrationRuntime.ActiveChunk(LoadedChunk{})
	second, _ := migrationRuntime.ActiveChunk(LoadedChunk{X: 1})

	if first.EntityCount() != 0 || second.EntityCount() != 1 {
		t.Fatalf("migrated active entities = first %d second %d", first.EntityCount(), second.EntityCount())
	}
}

func TestZombieAttackUsesPlayerSurvivalDamageAndCooldown(t *testing.T) {
	world := zombieGroundWorld(-1, 2, -1, 1)

	runtime := NewRuntime(world)

	session, connection := newZombieTestSession(t, runtime, game.Position{X: 1.1, Y: 0, Z: 0.5})

	session.updatePlayerState(func(player *game.Player) bool {
		player.Inventory.Armor[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}
		player.Absorption = 2

		player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectResistance, 100, 0, false, true, true))

		return true
	})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 1, Z: 0.5})

	connection.reset()

	runtime.Tick()

	player := session.snapshotPlayer()

	if player.Health >= 20 || player.Absorption != 0 || zombie.AttackCooldown != 20 {
		t.Fatalf("zombie attack defense = health %v absorption %v cooldown %d", player.Health, player.Absorption, zombie.AttackCooldown)
	}

	if countPacketID(connection.packets(t), protocol.ClientboundEntityAnimationID) != 1 || countPacketID(connection.packets(t), protocol.ClientboundDamageEventID) != 1 || countPacketID(connection.packets(t), protocol.ClientboundSetEntityMotionID) == 0 {
		t.Fatalf("zombie attack packets = %v", connection.packetIDs(t))
	}

	damagePacket := packetsByID(t, connection, protocol.ClientboundDamageEventID)[0]

	reader := protocol.NewPacketReader(damagePacket.Data)

	if reader.VarInt() != player.EntityID || reader.VarInt() != 28 {
		t.Fatal("zombie damage packet did not use mob attack registry 28")
	}

	velocity := player.Velocity

	runtime.Tick()

	if session.snapshotPlayer().Health != player.Health || session.snapshotPlayer().Velocity != velocity || zombie.AttackCooldown != 19 {
		t.Fatal("zombie attack cooldown did not prevent a second hit")
	}
}

func TestPlayerAttackZombieUsesSharedLivingCombat(t *testing.T) {
	runtime, attacker, _, attackerConnection, _ := newPlayerCombatTest(t)

	zombie := runtime.SpawnZombie(game.Position{X: 1})

	attackerConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: zombie.State.ID, Action: protocol.InteractActionAttack})

	if zombie.Living.Health >= 20 || zombie.Living.Velocity.X == 0 || attacker.snapshotPlayer().Inventory.Hotbar[0].Damage() != 1 {
		t.Fatalf("player zombie attack = health %v velocity %+v durability %d", zombie.Living.Health, zombie.Living.Velocity, attacker.snapshotPlayer().Inventory.Hotbar[0].Damage())
	}

	zombie.Living.Health = 1
	zombie.Living.InvulnerableTime = 0

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 20

		return true
	})

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: zombie.State.ID, Action: protocol.InteractActionAttack})

	durability := attacker.snapshotPlayer().Inventory.Hotbar[0].Damage()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: zombie.State.ID, Action: protocol.InteractActionAttack})

	if !zombie.Living.Dead || attacker.snapshotPlayer().Inventory.Hotbar[0].Damage() != durability {
		t.Fatal("dead zombie accepted another player attack")
	}
}

func TestPlayerAttackZombieStrengthAndCriticalDamage(t *testing.T) {
	tests := []zombiePlayerAttackDamageTestCase{
		{name: "weak", attackTicker: 0, onGround: true, wantHealth: 18.664053},
		{name: "full", attackTicker: 20, onGround: true, wantHealth: 13.112},
		{name: "critical", attackTicker: 20, fallDistance: 1, wantHealth: 9.668, wantCritical: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, attacker, _, connection, _ := newPlayerCombatTest(t)

			zombie := runtime.SpawnZombie(game.Position{X: 1})

			attacker.updatePlayerState(func(player *game.Player) bool {
				player.AttackStrengthTicker = test.attackTicker
				player.FallDistance = test.fallDistance
				player.OnGround = test.onGround

				return true
			})

			connection.reset()

			runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: zombie.State.ID, Action: protocol.InteractActionAttack})

			if math.Abs(float64(zombie.Living.Health-test.wantHealth)) > 1e-4 {
				t.Fatalf("zombie health = %v, want %v", zombie.Living.Health, test.wantHealth)
			}

			criticalPackets := countPacketID(connection.packets(t), protocol.ClientboundEntityAnimationID)
			if (criticalPackets == 1) != test.wantCritical {
				t.Fatalf("critical animation packets = %d, want critical %t", criticalPackets, test.wantCritical)
			}
		})
	}
}

func TestZombieDeathDropsLootAndRemovesAfterTwentyTicks(t *testing.T) {
	world := zombieGroundWorld(-1, 2, -1, 1)

	runtime := NewRuntime(world)

	session, connection := newZombieTestSession(t, runtime, game.Position{X: 4, Y: 0, Z: 0.5})

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 1, Z: 0.5})

	connection.reset()

	update, applied := runtime.damageRuntimeLivingEntityLocked(zombie, game.Damage{Type: game.DamageGenericKill, Amount: 20})

	if !applied || !update.died || !zombie.Living.Dead {
		t.Fatal("lethal zombie damage was not applied")
	}

	items := runtime.snapshotRuntimeEntities()

	if len(items) != 2 {
		t.Fatalf("entities after zombie death = %d, want zombie and loot", len(items))
	}

	var loot *runtimeItemEntity

	for _, entity := range items {
		item, itemEntity := entity.(*runtimeItemEntity)
		if itemEntity {
			loot = item
		}
	}

	if loot == nil || !loot.Stack.Equal(game.ItemStack{Item: game.ItemRottenFlesh, Count: 2}) || loot.PickupDelay != 10 {
		t.Fatalf("zombie loot = %+v", loot)
	}

	runtime.sendRuntimeLivingDamageUpdate(update)

	assertPacketIDs(t, connection.packetIDs(t), []int32{
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
		protocol.ClientboundDamageEventID,
		protocol.ClientboundUpdateEntityPositionID,
		protocol.ClientboundEntityMetadataID,
		protocol.ClientboundEntityEventID,
	})

	connection.reset()

	for range game.LivingDeathDurationTicks {
		runtime.Tick()
	}

	if !zombie.State.Removed || session.tracksRuntimeEntity(zombie.State.ID) || len(runtime.snapshotRuntimeEntities()) != 1 {
		t.Fatal("zombie was not removed and untracked after twenty death ticks")
	}

	if countPacketID(connection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 {
		t.Fatalf("zombie removal packets = %v", connection.packetIDs(t))
	}
}

func TestZombiePeacefulRemovalIsImmediate(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newZombieTestSession(t, runtime, game.Position{})

	zombie := runtime.SpawnZombie(game.Position{})

	connection.reset()

	runtime.Difficulty = game.DifficultyPeaceful

	runtime.Tick()

	if !zombie.State.Removed || len(runtime.snapshotRuntimeEntities()) != 0 || session.tracksRuntimeEntity(zombie.State.ID) {
		t.Fatal("peaceful zombie remained authoritative or tracked")
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundRemoveEntitiesID})
}

func newZombieTestSession(t *testing.T, runtime *Runtime, position game.Position) (*Session, *recordingConnection) {
	t.Helper()

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Target")

	session.Player.ResetSurvivalState()

	session.Player.Position = position
	session.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, session)

	runtime.setSessionActiveChunks(session, []LoadedChunk{{}})

	return session, connection
}

func zombieGroundWorld(minimumX, maximumX, minimumZ, maximumZ int32) *game.World {
	world := &game.World{}

	for x := minimumX; x <= maximumX; x++ {
		for z := minimumZ; z <= maximumZ; z++ {
			world.SetBlock(game.BlockPosition{X: x, Y: -1, Z: z}, game.Stone)
		}
	}

	return world
}
