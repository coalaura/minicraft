package server

import (
	"slices"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

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
}

func TestPlayerHungerRegenerationCadencesAndExhaustion(t *testing.T) {
	t.Run("saturated regeneration heals after 10 ticks", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.FoodLevel = 20
			player.Saturation = 3
			player.FoodTickTimer = 9

			return true
		})

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != 10.5 || player.Exhaustion != 0.5 || player.FoodTickTimer != 0 {
			t.Fatalf("saturated regeneration = %+v", player)
		}
	})

	t.Run("high food regeneration heals after 80 ticks", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newHungerTestSession(t, runtime)

		session.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.FoodLevel = 18
			player.Saturation = 0
			player.FoodTickTimer = 79

			return true
		})

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != 11 || player.Exhaustion != 6 || player.FoodTickTimer != 0 {
			t.Fatalf("high food regeneration = %+v", player)
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
			player.UseSelectedHotbarSlot = 2
			player.UseAnimation = game.ItemUseAnimationEat
			player.UseStack = game.ItemStack{Item: game.ItemApple, Count: 1}

			return true
		})

		err := runtime.RespawnPlayer(session)
		if err != nil {
			t.Fatalf("respawn player: %v", err)
		}

		player := session.snapshotPlayer()
		if player.Exhaustion != 0 || player.FoodTickTimer != 0 || player.SurvivalTickCount != 0 || player.UsingItem || player.UsingOffhand || player.UseRemainingTicks != 0 || player.UseSelectedHotbarSlot != 0 || player.UseAnimation != game.ItemUseAnimationNone || !player.UseStack.Empty() {
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
