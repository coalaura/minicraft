package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type playerInteractionRejectionTest struct {
	name        string
	interaction protocol.Interact
	prepare     func()
}

func TestPlayerInteractionTargetResolutionAndRejection(t *testing.T) {
	runtime, attacker, target, _, _ := newPlayerCombatTest(t)

	attackerPlayer := attacker.snapshotPlayer()
	targetPlayer := target.snapshotPlayer()

	tests := []playerInteractionRejectionTest{
		{
			name:        "unknown",
			interaction: protocol.Interact{EntityID: 9999, Action: protocol.InteractActionAttack},
		},
		{
			name:        "self",
			interaction: protocol.Interact{EntityID: attackerPlayer.EntityID, Action: protocol.InteractActionAttack},
		},
		{
			name:        "spectator attacker",
			interaction: protocol.Interact{EntityID: targetPlayer.EntityID, Action: protocol.InteractActionAttack},
			prepare: func() {
				attacker.updatePlayerState(func(player *game.Player) bool {
					player.GameMode = game.GameModeSpectator

					return true
				})
			},
		},
		{
			name:        "dead attacker",
			interaction: protocol.Interact{EntityID: targetPlayer.EntityID, Action: protocol.InteractActionAttack},
			prepare: func() {
				attacker.updatePlayerState(func(player *game.Player) bool {
					player.Dead = true

					return true
				})
			},
		},
		{
			name:        "dead target",
			interaction: protocol.Interact{EntityID: targetPlayer.EntityID, Action: protocol.InteractActionAttack},
			prepare: func() {
				target.updatePlayerState(func(player *game.Player) bool {
					player.Dead = true

					return true
				})
			},
		},
		{
			name:        "outside range",
			interaction: protocol.Interact{EntityID: targetPlayer.EntityID, Action: protocol.InteractActionAttack},
			prepare: func() {
				target.updatePlayerState(func(player *game.Player) bool {
					player.Position.X = 7

					return true
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attacker.updatePlayerState(func(player *game.Player) bool {
				player.GameMode = game.GameModeSurvival
				player.Dead = false
				player.AttackStrengthTicker = 20

				return true
			})

			target.updatePlayerState(func(player *game.Player) bool {
				player.Position = game.Position{X: 1}
				player.Health = game.DefaultPlayerHealth
				player.Dead = false

				return true
			})

			if test.prepare != nil {
				test.prepare()
			}

			runtime.handlePlayerInteraction(attacker, test.interaction)

			health := target.snapshotPlayer().Health

			if health != game.DefaultPlayerHealth {
				t.Fatalf("target health = %v, want %d", health, game.DefaultPlayerHealth)
			}

			ticker := attacker.snapshotPlayer().AttackStrengthTicker

			if ticker != 20 {
				t.Fatalf("attacker ticker = %d, want unchanged 20", ticker)
			}
		})
	}
}

func TestPlayerInteractionsRemainHarmless(t *testing.T) {
	runtime, attacker, target, _, _ := newPlayerCombatTest(t)

	targetID := target.snapshotPlayer().EntityID

	interactions := []protocol.Interact{
		{EntityID: targetID, Action: protocol.InteractActionInteract, Hand: protocol.MainHand},
		{EntityID: targetID, Action: protocol.InteractActionInteractAt, Hand: protocol.OffHand, TargetX: 0.1, TargetY: 1, TargetZ: 0.2},
	}

	for _, interaction := range interactions {
		runtime.handlePlayerInteraction(attacker, interaction)
	}

	health := target.snapshotPlayer().Health

	if health != game.DefaultPlayerHealth {
		t.Fatalf("target health = %v, want %d", health, game.DefaultPlayerHealth)
	}

	ticker := attacker.snapshotPlayer().AttackStrengthTicker

	if ticker != 20 {
		t.Fatalf("attacker ticker = %d, want unchanged 20", ticker)
	}
}

func TestPlayerMeleeDamageCooldownAndExhaustion(t *testing.T) {
	runtime, attacker, target, _, _ := newPlayerCombatTest(t)

	targetID := target.snapshotPlayer().EntityID

	attack := protocol.Interact{EntityID: targetID, Action: protocol.InteractActionAttack}

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 0

		return true
	})

	runtime.handlePlayerInteraction(attacker, attack)

	weakDamage := float32(7) * (0.2 + 0.8*0.04*0.04)
	health := target.snapshotPlayer().Health

	if math.Abs(float64(health-(game.DefaultPlayerHealth-weakDamage))) > 1e-5 {
		t.Fatalf("weak attack health = %v, want %v", health, game.DefaultPlayerHealth-weakDamage)
	}

	exhaustion := attacker.snapshotPlayer().Exhaustion

	if exhaustion != playerAttackExhaustion {
		t.Fatalf("attacker exhaustion = %v, want %v", exhaustion, playerAttackExhaustion)
	}

	target.updatePlayerState(func(player *game.Player) bool {
		player.Health = game.DefaultPlayerHealth
		player.InvulnerableTime = 0
		player.LastHurt = 0

		return true
	})

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 20

		return true
	})

	runtime.handlePlayerInteraction(attacker, attack)

	health = target.snapshotPlayer().Health

	if health != game.DefaultPlayerHealth-7 {
		t.Fatalf("full attack health = %v, want %v", health, game.DefaultPlayerHealth-7)
	}

	runtime.handlePlayerInteraction(attacker, attack)

	health = target.snapshotPlayer().Health

	if health != game.DefaultPlayerHealth-7 {
		t.Fatalf("repeated immunity health = %v, want %v", health, game.DefaultPlayerHealth-7)
	}

	ticker := attacker.snapshotPlayer().AttackStrengthTicker

	if ticker != 0 {
		t.Fatalf("attacker ticker after attack = %d, want 0", ticker)
	}
}

func TestPlayerMeleeKnockbackAndMotionSynchronization(t *testing.T) {
	runtime, attacker, target, attackerConnection, targetConnection := newPlayerCombatTest(t)

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 20
		player.Sprinting = true
		player.Rotation.Yaw = -90
		player.Velocity = game.Velocity{X: 1, Z: 1}

		return true
	})

	target.updatePlayerState(func(player *game.Player) bool {
		player.OnGround = true

		return true
	})

	attackerConnection.reset()
	targetConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

	knockedBack := target.snapshotPlayer()
	if knockedBack.Velocity.X <= 0 || knockedBack.Velocity.Y != 0.4 || knockedBack.FallDistance != 0 {
		t.Fatalf("knockback state = velocity %+v fall %v", knockedBack.Velocity, knockedBack.FallDistance)
	}

	attackerAfter := attacker.snapshotPlayer()
	if attackerAfter.Sprinting || attackerAfter.Velocity.X != 0.6 || attackerAfter.Velocity.Z != 0.6 {
		t.Fatalf("attacker after knockback = sprinting %t velocity %+v", attackerAfter.Sprinting, attackerAfter.Velocity)
	}

	if countPacketID(targetConnection.packets(t), protocol.ClientboundSetEntityMotionID) != 1 {
		t.Fatalf("target motion packets = %v", targetConnection.packetIDs(t))
	}

	if countPacketID(attackerConnection.packets(t), protocol.ClientboundSetEntityMotionID) != 1 {
		t.Fatalf("observer motion packets = %v", attackerConnection.packetIDs(t))
	}
}

func TestPlayerMeleeDurabilityBreakAndLethalDeath(t *testing.T) {
	t.Run("creative durability", func(t *testing.T) {
		runtime, attacker, target, _, _ := newPlayerCombatTest(t)

		attacker.updatePlayerState(func(player *game.Player) bool {
			player.GameMode = game.GameModeCreative
			player.AttackStrengthTicker = 20

			return true
		})

		runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

		held := attacker.snapshotPlayer().Inventory.Hotbar[0]

		if held.Damage() != 0 {
			t.Fatalf("creative weapon damage = %d, want 0", held.Damage())
		}
	})

	t.Run("durability break", func(t *testing.T) {
		runtime, attacker, target, attackerConnection, targetConnection := newPlayerCombatTest(t)

		attacker.updatePlayerState(func(player *game.Player) bool {
			player.Inventory.Hotbar[0].SetDamage(1560)
			player.AttackStrengthTicker = 20

			return true
		})

		attackerConnection.reset()
		targetConnection.reset()

		runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

		held := attacker.snapshotPlayer().Inventory.Hotbar[0]

		if !held.Empty() {
			t.Fatalf("broken weapon remains: %+v", held)
		}

		if countPacketID(attackerConnection.packets(t), protocol.ClientboundEntityEventID) != 1 || countPacketID(targetConnection.packets(t), protocol.ClientboundEntityEventID) != 1 {
			t.Fatalf("break event packets attacker=%v target=%v", attackerConnection.packetIDs(t), targetConnection.packetIDs(t))
		}
	})

	t.Run("lethal death path", func(t *testing.T) {
		runtime, attacker, target, _, targetConnection := newPlayerCombatTest(t)

		target.updatePlayerState(func(player *game.Player) bool {
			player.Health = 7
			player.Inventory.Main[0] = game.ItemStack{Item: game.ItemDiamond, Count: 1}

			return true
		})

		attacker.updatePlayerState(func(player *game.Player) bool {
			player.AttackStrengthTicker = 20

			return true
		})

		targetConnection.reset()
		runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

		if !target.snapshotPlayer().Dead || len(runtime.snapshotRuntimeEntities()) != 1 {
			t.Fatalf("lethal result dead=%t drops=%d", target.snapshotPlayer().Dead, len(runtime.snapshotRuntimeEntities()))
		}

		packets := targetConnection.packets(t)
		if countPacketID(packets, protocol.ClientboundCombatKillID) != 1 || countPacketID(packets, protocol.ClientboundEntityEventID) != 1 {
			t.Fatalf("lethal packets = %v", targetConnection.packetIDs(t))
		}

		runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

		if len(runtime.snapshotRuntimeEntities()) != 1 {
			t.Fatal("dead target duplicated inventory drops")
		}
	})
}

func TestInteractPacketRoutesToPlayerTarget(t *testing.T) {
	_, attacker, target, _, _ := newPlayerCombatTest(t)

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 20

		return true
	})

	var writer protocol.PacketWriter

	writer.VarInt(target.snapshotPlayer().EntityID)
	writer.VarInt(int32(protocol.InteractActionAttack))
	writer.Bool(false)

	err := attacker.handlePlayPacket(&protocol.Packet{ID: protocol.ServerboundInteractID, Data: writer.Buffer.Bytes()})
	if err != nil {
		t.Fatalf("route interact packet: %v", err)
	}

	health := target.snapshotPlayer().Health

	if health != game.DefaultPlayerHealth-7 {
		t.Fatalf("routed attack health = %v, want %v", health, game.DefaultPlayerHealth-7)
	}
}

func newPlayerCombatTest(t *testing.T) (*Runtime, *Session, *Session, *recordingConnection, *recordingConnection) {
	t.Helper()

	runtime := NewRuntime(&game.World{})
	attacker, attackerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Attacker")
	target, targetConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Target")

	attacker.Player.ResetSurvivalState()
	attacker.Player.Position = game.Position{}
	attacker.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDiamondSword, Count: 1}
	attacker.Player.LastMainHandItem = game.ItemDiamondSword
	attacker.Player.LastMainHandItemSet = true
	attacker.Player.AttackStrengthTicker = 20

	target.Player.ResetSurvivalState()
	target.Player.Position = game.Position{X: 1}

	joinTestSession(t, runtime, attacker)
	joinTestSession(t, runtime, target)

	return runtime, attacker, target, attackerConnection, targetConnection
}
