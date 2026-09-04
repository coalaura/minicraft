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

type playerCriticalEligibilityTest struct {
	name           string
	attackTicker   int32
	fallDistance   float32
	onGround       bool
	sprinting      bool
	water          bool
	blindness      bool
	critical       bool
	expectedDamage float32
	expectedSounds []string
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
	runtime, attacker, target, attackerConnection, _ := newPlayerCombatTest(t)

	targetID := target.snapshotPlayer().EntityID

	attack := protocol.Interact{EntityID: targetID, Action: protocol.InteractActionAttack}

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 0

		return true
	})

	attackerConnection.reset()

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

	assertAttackSounds(t, attackerConnection, playerAttackWeakSound)

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

	attackerConnection.reset()

	runtime.handlePlayerInteraction(attacker, attack)

	health = target.snapshotPlayer().Health

	if health != game.DefaultPlayerHealth-7 {
		t.Fatalf("full attack health = %v, want %v", health, game.DefaultPlayerHealth-7)
	}

	assertAttackSounds(t, attackerConnection, playerAttackStrongSound)

	attackerConnection.reset()

	runtime.handlePlayerInteraction(attacker, attack)

	health = target.snapshotPlayer().Health

	if health != game.DefaultPlayerHealth-7 {
		t.Fatalf("repeated immunity health = %v, want %v", health, game.DefaultPlayerHealth-7)
	}

	ticker := attacker.snapshotPlayer().AttackStrengthTicker

	if ticker != 0 {
		t.Fatalf("attacker ticker after attack = %d, want 0", ticker)
	}

	assertAttackSounds(t, attackerConnection, playerAttackNoDamageSound)
}

func TestPlayerMeleeOrdinaryKnockbackAndMotionSynchronization(t *testing.T) {
	runtime, attacker, target, attackerConnection, targetConnection := newPlayerCombatTest(t)

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 20

		return true
	})

	originalVelocity := game.Velocity{X: 0.2, Y: 0.1, Z: 0.4}

	target.updatePlayerState(func(player *game.Player) bool {
		player.Velocity = originalVelocity
		player.OnGround = true
		player.FallDistance = 2

		return true
	})

	attackerConnection.reset()
	targetConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

	targetAfter := target.snapshotPlayer()
	if targetAfter.Velocity != originalVelocity || !targetAfter.OnGround || targetAfter.FallDistance != 2 {
		t.Fatalf("stored target state = velocity %+v onGround %t fall %v", targetAfter.Velocity, targetAfter.OnGround, targetAfter.FallDistance)
	}

	wantMotion := game.Velocity{X: 0.5, Y: 0.4, Z: 0.2}

	assertPlayerMotion(t, targetConnection, targetAfter.EntityID, wantMotion)
	assertPlayerMotion(t, attackerConnection, targetAfter.EntityID, wantMotion)
}

func TestPlayerMeleeDifferentialDamageDoesNotKnockBack(t *testing.T) {
	runtime, attacker, target, attackerConnection, targetConnection := newPlayerCombatTest(t)

	attacker.updatePlayerState(func(player *game.Player) bool {
		player.AttackStrengthTicker = 20

		return true
	})

	target.updatePlayerState(func(player *game.Player) bool {
		player.InvulnerableTime = playerHurtCooldownTicks
		player.LastHurt = 5

		return true
	})

	attackerConnection.reset()
	targetConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

	if target.snapshotPlayer().Health != game.DefaultPlayerHealth-2 {
		t.Fatalf("differential health = %v, want %v", target.snapshotPlayer().Health, game.DefaultPlayerHealth-2)
	}

	if countPacketID(attackerConnection.packets(t), protocol.ClientboundSetEntityMotionID) != 0 || countPacketID(targetConnection.packets(t), protocol.ClientboundSetEntityMotionID) != 0 {
		t.Fatalf("differential motion packets attacker=%v target=%v", attackerConnection.packetIDs(t), targetConnection.packetIDs(t))
	}
}

func TestPlayerMeleeSprintKnockbackComposition(t *testing.T) {
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
		player.FallDistance = 2

		return true
	})

	attackerConnection.reset()
	targetConnection.reset()

	runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

	targetAfter := target.snapshotPlayer()
	if targetAfter.Velocity != (game.Velocity{}) || !targetAfter.OnGround || targetAfter.FallDistance != 2 {
		t.Fatalf("stored knockback state = velocity %+v onGround %t fall %v", targetAfter.Velocity, targetAfter.OnGround, targetAfter.FallDistance)
	}

	assertPlayerMotion(t, targetConnection, targetAfter.EntityID, game.Velocity{X: 0.7, Y: 0.4})
	assertPlayerMotion(t, attackerConnection, targetAfter.EntityID, game.Velocity{X: 0.7, Y: 0.4})

	attackerAfter := attacker.snapshotPlayer()
	if attackerAfter.Sprinting || attackerAfter.Velocity.X != 0.6 || attackerAfter.Velocity.Z != 0.6 {
		t.Fatalf("attacker after knockback = sprinting %t velocity %+v", attackerAfter.Sprinting, attackerAfter.Velocity)
	}

	if countPacketID(targetConnection.packets(t), protocol.ClientboundEntityMetadataID) == 0 {
		t.Fatalf("target did not receive attacker sprint metadata: %v", targetConnection.packetIDs(t))
	}

	assertAttackSounds(t, attackerConnection, playerAttackKnockbackSound, playerAttackStrongSound)
}

func TestPlayerMeleeCriticalEligibilityAndFeedback(t *testing.T) {
	tests := []playerCriticalEligibilityTest{
		{name: "falling", attackTicker: 20, fallDistance: 1, critical: true, expectedDamage: 10.5, expectedSounds: []string{playerAttackCriticalSound}},
		{name: "grounded", attackTicker: 20, fallDistance: 1, onGround: true, expectedDamage: 7, expectedSounds: []string{playerAttackStrongSound}},
		{name: "no fall distance", attackTicker: 20, expectedDamage: 7, expectedSounds: []string{playerAttackStrongSound}},
		{name: "insufficient charge", attackTicker: 0, fallDistance: 1, expectedDamage: 7 * (0.2 + 0.8*0.04*0.04), expectedSounds: []string{playerAttackWeakSound}},
		{name: "water", attackTicker: 20, fallDistance: 1, water: true, expectedDamage: 7, expectedSounds: []string{playerAttackStrongSound}},
		{name: "blindness", attackTicker: 20, fallDistance: 1, blindness: true, expectedDamage: 7, expectedSounds: []string{playerAttackStrongSound}},
		{name: "sprinting", attackTicker: 20, fallDistance: 1, sprinting: true, expectedDamage: 7, expectedSounds: []string{playerAttackKnockbackSound, playerAttackStrongSound}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, attacker, target, attackerConnection, targetConnection := newPlayerCombatTest(t)

			attacker.updatePlayerState(func(player *game.Player) bool {
				player.AttackStrengthTicker = test.attackTicker
				player.FallDistance = test.fallDistance
				player.OnGround = test.onGround
				player.Sprinting = test.sprinting

				if test.blindness {
					player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectBlindness, 100, 0, false, true, true))
				}

				return true
			})

			if test.water {
				runtime.World.SetBlock(game.BlockPosition{}, game.Water)
			}

			attackerConnection.reset()
			targetConnection.reset()

			runtime.handlePlayerInteraction(attacker, protocol.Interact{EntityID: target.snapshotPlayer().EntityID, Action: protocol.InteractActionAttack})

			damage := float32(game.DefaultPlayerHealth) - target.snapshotPlayer().Health
			if math.Abs(float64(damage-test.expectedDamage)) > 1e-5 {
				t.Fatalf("damage = %v, want %v", damage, test.expectedDamage)
			}

			assertAttackSounds(t, attackerConnection, test.expectedSounds...)

			animations := packetsByID(t, attackerConnection, protocol.ClientboundEntityAnimationID)

			if test.critical {
				if len(animations) != 1 {
					t.Fatalf("critical animations = %d, want 1", len(animations))
				}

				assertCriticalAnimation(t, animations[0], target.snapshotPlayer().EntityID)
			} else if len(animations) != 0 {
				t.Fatalf("non-critical animations = %d, want 0", len(animations))
			}

			if countPacketID(targetConnection.packets(t), protocol.ClientboundEntityAnimationID) != 0 {
				t.Fatal("critical animation was broadcast to target")
			}
		})
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

	attacker.loadedChunks = map[LoadedChunk]struct{}{{}: {}}
	target.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	return runtime, attacker, target, attackerConnection, targetConnection
}

func assertPlayerMotion(t *testing.T, connection *recordingConnection, entityID int32, expected game.Velocity) {
	t.Helper()

	packets := packetsByID(t, connection, protocol.ClientboundSetEntityMotionID)
	if len(packets) != 1 {
		t.Fatalf("motion packets = %d, want 1; packet IDs %v", len(packets), connection.packetIDs(t))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	actualEntityID := reader.VarInt()

	actual := decodeLowPrecisionVector(reader)

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode player motion: %v", err)
	}

	if actualEntityID != entityID || math.Abs(actual.X-expected.X) > 1.0/16000 || math.Abs(actual.Y-expected.Y) > 1.0/16000 || math.Abs(actual.Z-expected.Z) > 1.0/16000 {
		t.Fatalf("motion = entity %d velocity %+v, want entity %d velocity %+v", actualEntityID, actual, entityID, expected)
	}
}

func decodeLowPrecisionVector(reader *protocol.PacketReader) game.Velocity {
	packed := uint64(reader.Byte())
	packed |= uint64(reader.Byte()) << 8
	packed |= uint64(uint32(reader.Int())) << 16

	markers := packed & 7
	scale := markers

	if markers > 3 {
		scale = uint64(uint32(reader.VarInt()))*4 + (markers & 3)
	}

	decode := func(value uint64) float64 {
		return (float64(value)/16383 - 1) * float64(scale)
	}

	return game.Velocity{
		X: decode(packed >> 3 & 0x7fff),
		Y: decode(packed >> 18 & 0x7fff),
		Z: decode(packed >> 33 & 0x7fff),
	}
}

func assertAttackSounds(t *testing.T, connection *recordingConnection, expected ...string) {
	t.Helper()

	packets := packetsByID(t, connection, protocol.ClientboundSoundID)
	actual := make([]string, 0, len(packets))

	for _, packet := range packets {
		reader := protocol.NewPacketReader(packet.Data)

		holder := reader.VarInt()
		if holder != 0 {
			t.Fatalf("attack sound holder = %d, want direct holder", holder)
		}

		name := reader.String(32767)

		hasFixedRange := reader.Bool()
		if hasFixedRange {
			reader.Float()
		}

		source := reader.VarInt()

		reader.Int()
		reader.Int()
		reader.Int()

		volume := reader.Float()
		pitch := reader.Float()

		reader.Long()

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode attack sound: %v", err)
		}

		if source != protocol.SoundSourcePlayer || volume != 1 || pitch != 1 {
			t.Fatalf("attack sound %q = source %d volume %v pitch %v", name, source, volume, pitch)
		}

		actual = append(actual, name)
	}

	if len(actual) != len(expected) {
		t.Fatalf("attack sounds = %v, want %v", actual, expected)
	}

	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("attack sounds = %v, want %v", actual, expected)
		}
	}
}

func assertCriticalAnimation(t *testing.T, packet protocol.Packet, entityID int32) {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	actualEntityID := reader.VarInt()
	animation := reader.Byte()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode critical animation: %v", err)
	}

	if actualEntityID != entityID || animation != protocol.EntityAnimationCriticalHit {
		t.Fatalf("critical animation = entity %d action %d, want entity %d action %d", actualEntityID, animation, entityID, protocol.EntityAnimationCriticalHit)
	}
}
