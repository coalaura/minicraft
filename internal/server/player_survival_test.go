package server

import (
	"math"
	"slices"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestDamagePlayerHitCooldownAndEligibility(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 4})
	if !applied {
		t.Fatal("initial damage was not applied")
	}

	player := session.snapshotPlayer()
	if player.Health != 16 || player.InvulnerableTime != playerHurtCooldownTicks || player.LastHurt != 4 {
		t.Fatalf("initial damage state = %+v", player)
	}

	applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 4})
	if applied {
		t.Fatal("repeated equal damage was applied during cooldown")
	}

	applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 6})
	if !applied {
		t.Fatal("increased damage was not applied during cooldown")
	}

	player = session.snapshotPlayer()
	if player.Health != 14 || player.LastHurt != 6 {
		t.Fatalf("increased cooldown damage state = %+v", player)
	}

	modes := []game.GameMode{game.GameModeCreative, game.GameModeSpectator}

	for _, mode := range modes {
		session.updatePlayerState(func(player *game.Player) bool {
			player.GameMode = mode
			player.InvulnerableTime = 0

			return true
		})

		applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 1})
		if applied {
			t.Fatalf("%v player took normal damage", mode)
		}

		applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageOutOfWorld, Amount: 1})
		if !applied {
			t.Fatalf("%v player did not take void damage", mode)
		}
	}
}

func TestPlayerSurvivalAirDrowningAndEnvironmentalDamage(t *testing.T) {
	t.Run("air recovery and game mode eligibility", func(t *testing.T) {
		world := &game.World{}

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		session.Player.AirSupply = game.DefaultPlayerAirSupply - 4

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.AirSupply != game.DefaultPlayerAirSupply {
			t.Fatalf("recovered air supply = %d, want %d", player.AirSupply, game.DefaultPlayerAirSupply)
		}

		world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

		modes := []game.GameMode{game.GameModeSurvival, game.GameModeCreative, game.GameModeSpectator}

		for _, mode := range modes {

			session.updatePlayerState(func(player *game.Player) bool {
				player.GameMode = mode
				player.AirSupply = 10

				return true
			})

			runtime.Tick()

			player := session.snapshotPlayer()

			want := int32(14)

			if mode == game.GameModeSurvival {
				want = 9
			}

			if player.AirSupply != want {
				t.Fatalf("%v air supply = %d, want %d", mode, player.AirSupply, want)
			}
		}
	})

	t.Run("drowning cadence", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		session.Player.AirSupply = playerDrowningThreshold + 1

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.AirSupply != 0 || player.Health != game.DefaultPlayerHealth-playerDrowningDamage {
			t.Fatalf("first drowning tick = air %d health %v", player.AirSupply, player.Health)
		}

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.AirSupply != -1 || player.Health != game.DefaultPlayerHealth-playerDrowningDamage {
			t.Fatalf("drowning reset cadence = air %d health %v", player.AirSupply, player.Health)
		}
	})

	t.Run("fire and lava", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Fire)

		runtime := NewRuntime(world)

		runtime.entityRandom = func() float32 {
			return 0
		}

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth-1 || player.RemainingFireTicks != playerFireDurationTicks {
			t.Fatalf("fire state = health %v fire %d", player.Health, player.RemainingFireTicks)
		}

		world.SetBlock(game.BlockPosition{}, game.Lava)

		session.updatePlayerState(func(player *game.Player) bool {
			player.InvulnerableTime = 0
			player.LastHurt = 0

			return true
		})

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth-5 || player.RemainingFireTicks != playerLavaFireDurationTicks {
			t.Fatalf("lava state = health %v fire %d", player.Health, player.RemainingFireTicks)
		}
	})

	t.Run("creative fire expires after leaving lava", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Lava)

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		session.Player.GameMode = game.GameModeCreative

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.RemainingFireTicks != 1 || player.Health != game.DefaultPlayerHealth {
			t.Fatalf("creative lava state = health %v fire %d", player.Health, player.RemainingFireTicks)
		}

		world.SetBlock(game.BlockPosition{}, game.Air)

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.RemainingFireTicks != 0 {
			t.Fatalf("creative fire after leaving lava = %d, want 0", player.RemainingFireTicks)
		}
	})
}

func TestPlayerSurvivalFallAndVoid(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	session.Player.ResetSurvivalState()

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.FallDistance = 10
		player.OnGround = true
	})

	player := session.snapshotPlayer()
	if player.Health != 13 || player.FallDistance != 0 {
		t.Fatalf("fall landing state = health %v distance %v", player.Health, player.FallDistance)
	}

	session.updatePlayerState(func(player *game.Player) bool {
		player.Position.Y = float64(protocol.OverworldMinY-playerVoidDistanceBelowWorld) - 1
		player.InvulnerableTime = 0

		return true
	})

	runtime.Tick()

	player = session.snapshotPlayer()
	if player.Health != 9 {
		t.Fatalf("void damage health = %v, want 9", player.Health)
	}
}

func TestPlayerDeathDropsInventoryOnlyOnce(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.entityRandom = func() float32 {
		return 0
	}

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.ResetSurvivalState()

	session.Player.Inventory.CraftingResult = game.ItemStack{Item: game.ItemStone, Count: 1}
	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}
	session.Player.Inventory.Armor[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}
	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemOakLog, Count: 3}
	session.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemShield, Count: 1}

	applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if !applied {
		t.Fatal("lethal damage was not applied")
	}

	player := session.snapshotPlayer()
	if !player.Dead || !player.Inventory.CraftingResult.Empty() || !player.Inventory.Crafting[0].Empty() || !player.Inventory.Armor[0].Empty() || !player.Inventory.Main[0].Empty() || !player.Inventory.Offhand.Empty() {
		t.Fatalf("death state = %+v", player)
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 4 {
		t.Fatalf("death drops = %d, want 4", len(entities))
	}

	applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if applied {
		t.Fatal("dead player accepted repeated lethal damage")
	}

	runtime.Tick()

	if len(runtime.snapshotRuntimeEntities()) != 4 {
		t.Fatal("dead player survival tick duplicated inventory drops")
	}
}

func TestRespawnPlayerResetsAndSynchronizesObservers(t *testing.T) {
	world := &game.World{Spawn: game.Position{X: 4, Y: 80, Z: -2}}

	runtime := NewRuntime(world)

	player, playerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	player.Log = &chatTestLogger{}
	observer.Log = &chatTestLogger{}

	renderDistance := int32(config.MinRenderDistance)

	player.Config = &config.Config{Server: config.ServerConfig{RenderDistance: &renderDistance}}

	joinTestSession(t, runtime, player)
	joinTestSession(t, runtime, observer)

	playerConnection.reset()
	observerConnection.reset()

	player.updatePlayerState(func(current *game.Player) bool {
		current.Dead = true
		current.Health = 0
		current.AirSupply = 0
		current.RemainingFireTicks = 10
		current.Position = game.Position{X: 100, Y: 20}
		current.Rotation = game.Rotation{Yaw: 45}
		current.Velocity = game.Velocity{X: 1}
		current.Sneaking = true
		current.Sprinting = true
		current.Swimming = true
		current.Pose = game.PlayerPoseCrawling

		return true
	})

	player.hasChunkCenter = true
	player.centerChunk = LoadedChunk{Z: -1}
	player.loadedChunks = map[LoadedChunk]struct{}{player.centerChunk: {}}

	err := runtime.RespawnPlayer(player)

	if err != nil {
		t.Fatalf("respawn player: %v", err)
	}

	current := player.snapshotPlayer()
	if current.Dead || current.Health != game.DefaultPlayerHealth || current.AirSupply != game.DefaultPlayerAirSupply || current.Position != world.Spawn || current.Rotation != (game.Rotation{}) || current.Velocity != (game.Velocity{}) || !current.OnGround || current.Sneaking || current.Sprinting || current.Swimming || current.Pose != game.PlayerPoseStanding {
		t.Fatalf("respawned player = %+v", current)
	}

	playerPacketIDs := playerConnection.packetIDs(t)
	if len(playerPacketIDs) < 7 {
		t.Fatalf("respawn packets = %v, want state and reloaded chunks", playerPacketIDs)
	}

	if playerPacketIDs[0] != protocol.ClientboundRespawnID || playerPacketIDs[1] != protocol.ClientboundSetHealthID || playerPacketIDs[2] != protocol.ClientboundGameEventID || playerPacketIDs[3] != protocol.ClientboundSetCenterChunkID {
		t.Fatalf("respawn packet prefix = %v", playerPacketIDs[:4])
	}

	respawnEvent := protocol.NewPacketReader(playerConnection.packets(t)[2].Data)
	if respawnEvent.Byte() != 13 {
		t.Fatal("respawn did not start client terrain loading")
	}

	assertPacketIDs(t, playerPacketIDs[len(playerPacketIDs)-3:], []int32{
		protocol.ClientboundPlayerPositionID,
		protocol.ClientboundContainerSetContentID,
		protocol.ClientboundEntityMetadataID,
	})

	if !slices.Contains(playerPacketIDs, protocol.ClientboundLevelChunkWithLightID) {
		t.Fatalf("respawn packets = %v, want chunk data", playerPacketIDs)
	}

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{
		protocol.ClientboundRemoveEntitiesID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})

	playerConnection.reset()

	err = runtime.RespawnPlayer(player)
	if err != nil {
		t.Fatalf("respawn living player: %v", err)
	}

	ids := playerConnection.packetIDs(t)
	if len(ids) != 0 {
		t.Fatalf("living respawn packets = %v, want none", ids)
	}
}

func TestKillCommandKillsCreativePlayer(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	player, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	player.Player.GameMode = game.GameModeCreative

	joinTestSession(t, runtime, player)

	connection.reset()

	executeCommand(t, player, "kill")

	if !player.snapshotPlayer().Dead {
		t.Fatal("kill command did not kill creative player")
	}
}
