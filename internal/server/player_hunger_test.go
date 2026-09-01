package server

import (
	"math"
	"slices"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type hungerCheckpoint struct {
	ticks      int
	health     float32
	saturation float32
	exhaustion float32
	food       int32
	timer      int32
}

func TestPlayerHungerExhaustionUsesStrictThresholdAndSaturationFirst(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newHungerTestSession(t, runtime)

	session.updatePlayerState(func(player *game.Player) bool {
		player.Exhaustion = 4
		player.Saturation = 1
		player.FoodLevel = 17

		return true
	})

	runtime.Tick()

	player := session.snapshotPlayer()
	if player.Exhaustion != 4 || player.Saturation != 1 || player.FoodLevel != 17 {
		t.Fatalf("exhaustion at threshold = %+v", player)
	}

	session.updatePlayerState(func(player *game.Player) bool {
		player.Exhaustion = 4.5

		return true
	})

	runtime.Tick()

	player = session.snapshotPlayer()
	if player.Exhaustion != 0.5 || player.Saturation != 0 || player.FoodLevel != 17 {
		t.Fatalf("exhaustion crossing with saturation = %+v", player)
	}

	session.updatePlayerState(func(player *game.Player) bool {
		player.Exhaustion = 4.5

		return true
	})

	runtime.Tick()

	player = session.snapshotPlayer()
	if player.Saturation != 0 || player.FoodLevel != 16 {
		t.Fatalf("exhaustion crossing without saturation = %+v", player)
	}

	session.updatePlayerState(func(player *game.Player) bool {
		player.Exhaustion = 9
		player.Saturation = 2

		return true
	})

	runtime.Tick()

	player = session.snapshotPlayer()
	if player.Exhaustion != 5 || player.Saturation != 1 {
		t.Fatalf("single exhaustion conversion per tick = %+v", player)
	}
}

func TestPlayerHungerRegenerationCadencesAndExhaustion(t *testing.T) {
	t.Run("five saturation transitions from fast to ordinary regeneration", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.FoodLevel = 20
			player.Saturation = 5

			return true
		})

		checkpoints := []hungerCheckpoint{
			{ticks: 9, health: 10, saturation: 5, exhaustion: 0, food: 20, timer: 9},
			{ticks: 10, health: 10 + 5.0/6.0, saturation: 5, exhaustion: 5, food: 20, timer: 0},
			{ticks: 11, health: 10 + 5.0/6.0, saturation: 4, exhaustion: 1, food: 20, timer: 1},
			{ticks: 30, health: 12, saturation: 3, exhaustion: 4, food: 20, timer: 0},
			{ticks: 40, health: 12.5, saturation: 3, exhaustion: 7, food: 20, timer: 0},
			{ticks: 91, health: 13.5, saturation: 0, exhaustion: 1, food: 20, timer: 1},
			{ticks: 169, health: 13.5, saturation: 0, exhaustion: 1, food: 20, timer: 79},
			{ticks: 170, health: 14.5, saturation: 0, exhaustion: 7, food: 20, timer: 0},
			{ticks: 171, health: 14.5, saturation: 0, exhaustion: 3, food: 19, timer: 1},
		}

		assertHungerCheckpoints(t, runtime, session, checkpoints)
	})

	t.Run("strong food supplies a rapidly spent high saturation budget", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.FoodLevel = 14
			player.Saturation = 0
			player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemGoldenCarrot, Count: 1}

			return true
		})

		startFoodUse(t, session)

		tickFoodUse(runtime, 32)

		player := session.snapshotPlayer()
		if player.FoodLevel != 20 || !closeHungerValue(player.Saturation, 14.4) {
			t.Fatalf("golden carrot food state = %+v", player)
		}

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.Exhaustion = 0
			player.FoodTickTimer = 0

			return true
		})

		checkpoints := []hungerCheckpoint{
			{ticks: 10, health: 11, saturation: 14.4, exhaustion: 6, food: 20, timer: 0},
			{ticks: 11, health: 11, saturation: 13.4, exhaustion: 2, food: 20, timer: 1},
			{ticks: 20, health: 12, saturation: 13.4, exhaustion: 8, food: 20, timer: 0},
			{ticks: 21, health: 12, saturation: 12.4, exhaustion: 4, food: 20, timer: 1},
		}

		assertHungerCheckpoints(t, runtime, session, checkpoints)
	})

	t.Run("fractional saturation retains exact fast regeneration accounting", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.FoodLevel = 20
			player.Saturation = 0.5

			return true
		})

		checkpoints := []hungerCheckpoint{
			{ticks: 10, health: 10 + 0.5/6.0, saturation: 0.5, exhaustion: 0.5, food: 20, timer: 0},
			{ticks: 80, health: 10 + 4.0/6.0, saturation: 0.5, exhaustion: 4, food: 20, timer: 0},
			{ticks: 90, health: 10.75, saturation: 0.5, exhaustion: 4.5, food: 20, timer: 0},
			{ticks: 91, health: 10.75, saturation: 0, exhaustion: 0.5, food: 20, timer: 1},
			{ticks: 170, health: 11.75, saturation: 0, exhaustion: 6.5, food: 20, timer: 0},
			{ticks: 171, health: 11.75, saturation: 0, exhaustion: 2.5, food: 19, timer: 1},
		}

		assertHungerCheckpoints(t, runtime, session, checkpoints)
	})

	t.Run("food thresholds and branch timer semantics", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.FoodLevel = 19
			player.Saturation = 5
			player.FoodTickTimer = 9

			return true
		})

		tickFoodUse(runtime, 71)

		player := session.snapshotPlayer()
		if player.Health != 11 || player.Exhaustion != 6 || player.FoodTickTimer != 0 {
			t.Fatalf("food 19 ordinary regeneration = %+v", player)
		}

		session.updatePlayerState(func(player *game.Player) bool {
			player.FoodLevel = 20
			player.Saturation = 1
			player.Exhaustion = 0
			player.FoodTickTimer = 9

			return true
		})

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.FoodTickTimer != 0 || !closeHungerValue(player.Health, 11+1.0/6.0) || player.Exhaustion != 1 {
			t.Fatalf("food 20 fast regeneration threshold = %+v", player)
		}

		session.updatePlayerState(func(player *game.Player) bool {
			player.FoodLevel = 17
			player.FoodTickTimer = 12

			return true
		})

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.FoodTickTimer != 0 {
			t.Fatalf("inactive regeneration timer = %d, want 0", player.FoodTickTimer)
		}
	})
}

func TestPlayerHungerStarvationDifficultyLimits(t *testing.T) {
	t.Run("easy stops at 10 health", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		runtime.Difficulty = game.DifficultyEasy

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 11
			player.FoodLevel = 0
			player.FoodTickTimer = 79

			return true
		})

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != 10 || player.Dead {
			t.Fatalf("easy starvation damage = %+v", player)
		}

		session.updatePlayerState(func(player *game.Player) bool {
			player.FoodTickTimer = 79

			return true
		})

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.Health != 10 || player.Dead {
			t.Fatalf("easy starvation floor = %+v", player)
		}
	})

	t.Run("normal stops at 1 health", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		runtime.Difficulty = game.DifficultyNormal

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 2
			player.FoodLevel = 0
			player.FoodTickTimer = 79

			return true
		})

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != 1 || player.Dead {
			t.Fatalf("normal starvation floor = %+v", player)
		}
	})

	t.Run("hard starvation kills and identifies starvation damage", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		runtime.Difficulty = game.DifficultyHard

		session, connection := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 1
			player.FoodLevel = 0
			player.FoodTickTimer = 79

			return true
		})

		connection.reset()

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != 0 || !player.Dead {
			t.Fatalf("hard starvation death = %+v", player)
		}

		packets := connection.packets(t)

		var (
			damagePacket      protocol.Packet
			foundDamagePacket bool
		)

		for _, packet := range packets {
			if packet.ID == protocol.ClientboundDamageEventID {
				damagePacket = packet
				foundDamagePacket = true

				break
			}
		}

		if !foundDamagePacket {
			t.Fatalf("starvation packets = %v, want damage event", connection.packetIDs(t))
		}

		reader := protocol.NewPacketReader(damagePacket.Data)

		entityID := reader.VarInt()
		damageType := reader.VarInt()

		if reader.Err() != nil {
			t.Fatalf("read starvation damage event: %v", reader.Err())
		}

		if entityID != player.EntityID || damageType != 40 {
			t.Fatalf("starvation damage event = entity %d type %d", entityID, damageType)
		}

		if !slices.Contains(connection.packetIDs(t), protocol.ClientboundCombatKillID) {
			t.Fatalf("starvation packets = %v, want combat kill", connection.packetIDs(t))
		}
	})
}

func TestPlayerHungerPeacefulRecovery(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.Difficulty = game.DifficultyPeaceful

	session, _ := newHungerTestSession(t, runtime)

	session.updatePlayerState(func(player *game.Player) bool {
		player.Health = 10
		player.FoodLevel = 18
		player.Saturation = 3
		player.SurvivalTickCount = 19

		return true
	})

	runtime.Tick()

	player := session.snapshotPlayer()
	if player.Health != 11 || player.FoodLevel != 19 || player.Saturation != 4 {
		t.Fatalf("peaceful 20-tick recovery = %+v", player)
	}

	for range 10 {
		runtime.Tick()
	}

	player = session.snapshotPlayer()
	if player.FoodLevel != 20 {
		t.Fatalf("peaceful 10-tick food recovery = %+v", player)
	}
}

func TestPlayerHungerHealthSynchronizationMatchesVisibleState(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, connection := newHungerTestSession(t, runtime)

	session.updatePlayerState(func(player *game.Player) bool {
		player.Exhaustion = 4.5
		player.Saturation = 2

		return true
	})

	runtime.Tick()

	if slices.Contains(connection.packetIDs(t), protocol.ClientboundSetHealthID) {
		t.Fatalf("nonzero saturation change packets = %v, want no set health", connection.packetIDs(t))
	}

	connection.reset()

	session.updatePlayerState(func(player *game.Player) bool {
		player.Exhaustion = 4.5
		player.Saturation = 1

		return true
	})

	runtime.Tick()

	if !slices.Contains(connection.packetIDs(t), protocol.ClientboundSetHealthID) {
		t.Fatalf("saturation zero crossing packets = %v, want set health", connection.packetIDs(t))
	}
}

func TestPlayerHungerRespawnAndExhaustionImmunity(t *testing.T) {
	t.Run("respawn resets hunger and item use state", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Dead = true
			player.Health = 0
			player.Exhaustion = 3
			player.FoodTickTimer = 79
			player.SurvivalTickCount = 99
			player.UsingItem = true
			player.UsingOffhand = true
			player.UseRemainingTicks = 12
			player.UseAnimation = game.ItemUseAnimationEat
			player.UseStack = game.ItemStack{Item: game.ItemApple, Count: 1}

			return true
		})

		err := runtime.RespawnPlayer(session)
		if err != nil {
			t.Fatalf("respawn player: %v", err)
		}

		player := session.snapshotPlayer()
		if player.Exhaustion != 0 || player.FoodTickTimer != 0 || player.SurvivalTickCount != 0 || player.UsingItem || player.UsingOffhand || player.UseRemainingTicks != 0 || player.UseAnimation != game.ItemUseAnimationNone || !player.UseStack.Empty() {
			t.Fatalf("respawned hunger and item use state = %+v", player)
		}
	})

	t.Run("creative and spectator ignore exhaustion", func(t *testing.T) {
		modes := []game.GameMode{game.GameModeCreative, game.GameModeSpectator}

		for _, mode := range modes {
			player := game.Player{GameMode: mode}

			player.AddExhaustion(5)

			if player.Exhaustion != 0 {
				t.Fatalf("%v player exhaustion = %v", mode, player.Exhaustion)
			}
		}
	})
}

func assertHungerCheckpoints(t *testing.T, runtime *Runtime, session *Session, checkpoints []hungerCheckpoint) {
	t.Helper()

	elapsed := 0

	for _, checkpoint := range checkpoints {
		tickFoodUse(runtime, checkpoint.ticks-elapsed)

		player := session.snapshotPlayer()
		if !closeHungerValue(player.Health, checkpoint.health) || !closeHungerValue(player.Saturation, checkpoint.saturation) || !closeHungerValue(player.Exhaustion, checkpoint.exhaustion) || player.FoodLevel != checkpoint.food || player.FoodTickTimer != checkpoint.timer {
			t.Fatalf("hunger after %d ticks = health %v saturation %v exhaustion %v food %d timer %d, want health %v saturation %v exhaustion %v food %d timer %d", checkpoint.ticks, player.Health, player.Saturation, player.Exhaustion, player.FoodLevel, player.FoodTickTimer, checkpoint.health, checkpoint.saturation, checkpoint.exhaustion, checkpoint.food, checkpoint.timer)
		}

		elapsed = checkpoint.ticks
	}
}

func closeHungerValue(actual, expected float32) bool {
	return math.Abs(float64(actual-expected)) < 0.0001
}

func newHungerTestSession(t *testing.T, runtime *Runtime) (*Session, *recordingConnection) {
	t.Helper()

	session, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player", game.GameModeSurvival)

	session.Player.ResetSurvivalState()

	session.Log = &chatTestLogger{}

	renderDistance := int32(config.MinRenderDistance)

	session.Config = &config.Config{Server: config.ServerConfig{RenderDistance: &renderDistance}}

	joinTestSession(t, runtime, session)

	connection.reset()

	return session, connection
}
