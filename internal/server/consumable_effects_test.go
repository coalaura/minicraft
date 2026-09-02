package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type chanceFoodEffectTest struct {
	name   string
	item   game.Item
	random float32
	want   bool
}

func TestConsumableCompletionPrecedesFoodTick(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Player.FoodLevel = 16
	actor.Player.Saturation = 0
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}

	startFoodUse(t, actor)

	tickFoodUse(runtime, 31)

	actor.updatePlayerState(func(player *game.Player) bool {
		player.Health = 10
		player.FoodTickTimer = 9

		return true
	})

	runtime.Tick()

	player := actor.snapshotPlayer()
	if player.FoodLevel != 20 || !closeHungerValue(player.Health, 10.4) || player.FoodTickTimer != 0 {
		t.Fatalf("completion-tick food processing = %+v", player)
	}
}

func TestGoldenAppleConsumablesApplyExpectedEffectsAndAbsorption(t *testing.T) {
	t.Run("golden apple", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.Health = game.DefaultPlayerHealth
		actor.Player.FoodLevel = 10
		actor.Player.Saturation = 0
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemGoldenApple, Count: 1}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectRegeneration, 100, 1)
		assertPlayerEffect(t, player, game.MobEffectAbsorption, 2400, 0)

		if player.FoodLevel != 14 || player.Saturation != 9.6 || player.Absorption != 4 {
			t.Fatalf("golden apple state = %+v", player)
		}

		applied := runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageFall, Amount: 4})
		if !applied {
			t.Fatal("absorption damage was not applied")
		}

		player = actor.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth || player.Absorption != 0 {
			t.Fatalf("golden apple absorption damage = %+v", player)
		}
	})

	t.Run("enchanted golden apple", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemEnchantedGoldenApple, Count: 1}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectRegeneration, 400, 1)
		assertPlayerEffect(t, player, game.MobEffectResistance, 6000, 0)
		assertPlayerEffect(t, player, game.MobEffectFireResistance, 6000, 0)
		assertPlayerEffect(t, player, game.MobEffectAbsorption, 2400, 3)

		if player.Absorption != 16 {
			t.Fatalf("enchanted golden apple absorption = %v, want 16", player.Absorption)
		}
	})
}

func TestChanceFoodEffectsUseRuntimeEntityRandom(t *testing.T) {
	cases := []chanceFoodEffectTest{
		{name: "chicken applies hunger", item: game.ItemChicken, random: 0.29, want: true},
		{name: "chicken skips hunger", item: game.ItemChicken, random: 0.3, want: false},
		{name: "rotten flesh applies hunger", item: game.ItemRottenFlesh, random: 0.79, want: true},
		{name: "rotten flesh skips hunger", item: game.ItemRottenFlesh, random: 0.8, want: false},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			runtime.entityRandom = func() float32 {
				return test.random
			}

			actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

			actor.Player.FoodLevel = 10
			actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: 1}

			startFoodUse(t, actor)

			tickFoodUse(runtime, 31)

			actor.updatePlayerState(func(player *game.Player) bool {
				player.Exhaustion = 0

				return true
			})

			runtime.Tick()

			player := actor.snapshotPlayer()

			instance, active := player.ActiveEffects.Find(game.MobEffectHunger)
			if active != test.want {
				t.Fatalf("hunger active = %t, want %t", active, test.want)
			}

			if test.want && (instance.Duration != 600 || player.Exhaustion != 0) {
				t.Fatalf("completion-tick hunger state = effect %+v exhaustion %v", instance, player.Exhaustion)
			}

			if !test.want {
				return
			}

			runtime.Tick()

			player = actor.snapshotPlayer()
			instance, active = player.ActiveEffects.Find(game.MobEffectHunger)

			if !active || instance.Duration != 599 || player.Exhaustion != 0.005 {
				t.Fatalf("next-tick hunger state = effect %+v active %t exhaustion %v", instance, active, player.Exhaustion)
			}
		})
	}
}

func TestPoisonousFoodsApplyExpectedEffects(t *testing.T) {
	t.Run("poisonous potato", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		runtime.entityRandom = func() float32 {
			return 0.59
		}

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemPoisonousPotato, Count: 1}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		assertPlayerEffect(t, actor.snapshotPlayer(), game.MobEffectPoison, 100, 0)
	})

	t.Run("pufferfish", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemPufferfish, Count: 1}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectPoison, 1200, 1)
		assertPlayerEffect(t, player, game.MobEffectHunger, 300, 2)
		assertPlayerEffect(t, player, game.MobEffectNausea, 300, 0)
	})

	t.Run("spider eye", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemSpiderEye, Count: 1}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		assertPlayerEffect(t, actor.snapshotPlayer(), game.MobEffectPoison, 100, 0)
	})
}

func TestHoneyAndMilkConsumablesRemoveEffectsAndPreserveCreativeItems(t *testing.T) {
	t.Run("honey removes poison only", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = game.DefaultPlayerFoodLevel
		actor.Player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectPoison, 100, 0, false, true, true))
		actor.Player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectSpeed, 100, 0, false, true, true))
		actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemHoneyBottle, Count: 1}

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 1})
		if err != nil {
			t.Fatalf("start offhand honey use: %v", err)
		}

		tickFoodUse(runtime, 40)

		player := actor.snapshotPlayer()

		_, poisoned := player.ActiveEffects.Find(game.MobEffectPoison)

		assertPlayerEffect(t, player, game.MobEffectSpeed, 60, 0)

		if poisoned || !player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemGlassBottle, Count: 1}) {
			t.Fatalf("offhand honey result = %+v", player)
		}
	})

	t.Run("milk clears effects and leaves bucket", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, actorConnection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = game.DefaultPlayerFoodLevel
		actor.Player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectPoison, 100, 0, false, true, true))
		actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemMilkBucket, Count: 1}

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 1})
		if err != nil {
			t.Fatalf("start offhand milk use: %v", err)
		}

		actorConnection.reset()

		tickFoodUse(runtime, 31)

		actor.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.InvulnerableTime = 0
			player.LastHurt = 0
			player.ActiveEffects.Clear()
			player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectPoison, 25, 0, false, true, true))

			return true
		})

		runtime.Tick()

		player := actor.snapshotPlayer()
		if player.Health != 9 || len(player.ActiveEffects) != 0 || !player.Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemBucket, Count: 1}) {
			t.Fatalf("offhand milk result = %+v", player)
		}

		assertEffectPacketCount(t, actorConnection, protocol.ClientboundRemoveMobEffectID, 1)
	})

	t.Run("creative preserves milk bucket", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeCreative)

		actor.Player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectPoison, 100, 0, false, true, true))

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemMilkBucket, Count: 1}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()
		if len(player.ActiveEffects) != 0 || !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemMilkBucket, Count: 1}) {
			t.Fatalf("creative milk result = %+v", player)
		}
	})
}

func TestPlayerEffectCadenceAndPoisonFloor(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.updatePlayerState(func(player *game.Player) bool {
		player.Health = 10
		player.FoodLevel = 0
		player.Saturation = 0

		player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectRegeneration, 50, 0, false, true, true))
		player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectHunger, 10, 1, false, true, true))

		return true
	})

	runtime.Tick()

	player := actor.snapshotPlayer()
	if player.Health != 11 || player.Exhaustion != 0.01 {
		t.Fatalf("regeneration and hunger cadence = %+v", player)
	}

	actor.updatePlayerState(func(player *game.Player) bool {
		player.Health = 1

		player.ActiveEffects.Clear()

		player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectPoison, 25, 0, false, true, true))

		return true
	})

	runtime.Tick()

	player = actor.snapshotPlayer()
	if player.Health != 1 {
		t.Fatalf("poison damaged player below its floor: %+v", player)
	}
}

func TestAbsorptionDamageAndEffectLifecycle(t *testing.T) {
	t.Run("partial and exceeding damage", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.updatePlayerState(func(player *game.Player) bool {
			player.Absorption = 4

			return true
		})

		applied := runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageFall, Amount: 2})
		if !applied {
			t.Fatal("partial absorption damage was not applied")
		}

		player := actor.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth || player.Absorption != 2 {
			t.Fatalf("partial absorption damage = %+v", player)
		}

		actor.updatePlayerState(func(player *game.Player) bool {
			player.InvulnerableTime = 0
			player.LastHurt = 0

			return true
		})

		applied = runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageFall, Amount: 3})
		if !applied {
			t.Fatal("exceeding absorption damage was not applied")
		}

		player = actor.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth-1 || player.Absorption != 0 {
			t.Fatalf("exceeding absorption damage = %+v", player)
		}
	})

	t.Run("weaker effect restores after stronger expiration", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.updatePlayerState(func(player *game.Player) bool {
			addPlayerMobEffect(player, game.NewMobEffectInstance(game.MobEffectAbsorption, 20, 0, false, true, true))
			addPlayerMobEffect(player, game.NewMobEffectInstance(game.MobEffectAbsorption, 2, 3, false, true, true))
			addPlayerMobEffect(player, game.NewMobEffectInstance(game.MobEffectAbsorption, 20, 0, false, true, true))

			return true
		})

		player := actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectAbsorption, 2, 3)

		if player.Absorption != 16 {
			t.Fatalf("upgraded absorption = %v, want 16", player.Absorption)
		}

		tickFoodUse(runtime, 2)

		player = actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectAbsorption, 18, 0)

		if player.Absorption != 4 {
			t.Fatalf("restored absorption = %v, want 4", player.Absorption)
		}

		tickFoodUse(runtime, 18)

		player = actor.snapshotPlayer()

		_, active := player.ActiveEffects.Find(game.MobEffectAbsorption)

		if active || player.Absorption != 0 {
			t.Fatalf("expired absorption = %+v", player)
		}
	})
}

func TestDamageEffectsAndDeathLifecycle(t *testing.T) {
	t.Run("fire resistance blocks fire damage without clearing fire", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.updatePlayerState(func(player *game.Player) bool {
			player.RemainingFireTicks = 80

			player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectFireResistance, 100, 0, false, true, true))

			return true
		})

		damageTypes := []PlayerDamageType{PlayerDamageInFire, PlayerDamageLava, PlayerDamageOnFire}

		for _, damageType := range damageTypes {
			applied := runtime.DamagePlayer(actor, PlayerDamage{Type: damageType, Amount: 4})
			if applied {
				t.Fatalf("fire damage type %d was applied through fire resistance", damageType)
			}
		}

		player := actor.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth || player.RemainingFireTicks != 80 {
			t.Fatalf("fire resistance state = %+v", player)
		}
	})

	t.Run("resistance reduces ordinary damage but not starvation", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.updatePlayerState(func(player *game.Player) bool {
			player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectResistance, 100, 0, false, true, true))

			return true
		})

		applied := runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageFall, Amount: 10})
		if !applied {
			t.Fatal("resistance damage was not applied")
		}

		player := actor.snapshotPlayer()
		if player.Health != 12 {
			t.Fatalf("resistance damage health = %v, want 12", player.Health)
		}

		actor.updatePlayerState(func(player *game.Player) bool {
			player.InvulnerableTime = 0
			player.LastHurt = 0

			return true
		})

		applied = runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageStarve, Amount: 10})
		if !applied {
			t.Fatal("starvation damage was not applied")
		}

		player = actor.snapshotPlayer()
		if player.Health != 2 {
			t.Fatalf("starvation damage health = %v, want 2", player.Health)
		}
	})

	t.Run("death clears effects and respawn resets survival state", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, actorConnection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Log = &chatTestLogger{}

		actor.updatePlayerState(func(player *game.Player) bool {
			player.Health = 1
			player.Absorption = 0

			player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectSpeed, 100, 0, false, true, true))

			return true
		})

		actorConnection.reset()

		applied := runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageGenericKill, Amount: 1})
		if !applied {
			t.Fatal("lethal damage was not applied")
		}

		player := actor.snapshotPlayer()
		if !player.Dead || len(player.ActiveEffects) != 0 {
			t.Fatalf("death effect state = %+v", player)
		}

		assertEffectPacketCount(t, actorConnection, protocol.ClientboundRemoveMobEffectID, 1)

		err := runtime.RespawnPlayer(actor)
		if err != nil {
			t.Fatalf("respawn player: %v", err)
		}

		player = actor.snapshotPlayer()
		if player.Dead || len(player.ActiveEffects) != 0 || player.Absorption != 0 || player.Health != game.DefaultPlayerHealth {
			t.Fatalf("respawn effect state = %+v", player)
		}
	})
}

func TestPlayerEffectsSynchronizeToSelfObserverAndNewlyVisibleSessions(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)
	observer, observerConnection := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)

	position := toBlockPosition(actor.Player.Position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	actor.Player.FoodLevel = 10
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemGoldenApple, Count: 1}

	actorConnection.reset()
	observerConnection.reset()

	startFoodUse(t, actor)

	tickFoodUse(runtime, 32)

	assertEffectPacketCount(t, actorConnection, protocol.ClientboundUpdateMobEffectID, 2)
	assertEffectPacketCount(t, observerConnection, protocol.ClientboundUpdateMobEffectID, 2)

	newlyVisible, newlyVisibleConnection := newBlockMutationTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "NewlyVisible", game.GameModeSurvival)

	newlyVisible.Player.ResetSurvivalState()

	joinTestSession(t, runtime, newlyVisible)

	assertEffectPacketCount(t, newlyVisibleConnection, protocol.ClientboundUpdateMobEffectID, 2)

	actorConnection.reset()
	observerConnection.reset()

	actor.updatePlayerState(func(player *game.Player) bool {
		player.ActiveEffects.Clear()
		player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectSpeed, 1, 0, false, true, true))

		return true
	})

	runtime.Tick()

	assertEffectPacketCount(t, actorConnection, protocol.ClientboundRemoveMobEffectID, 1)
	assertEffectPacketCount(t, observerConnection, protocol.ClientboundRemoveMobEffectID, 1)
}

func TestPlayerEffectsDoNotSynchronizeOutsideViewerRange(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)
	observer, observerConnection := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)

	observer.updatePlayerState(func(player *game.Player) bool {
		player.Position.X = 16 * 100

		return true
	})

	actor.Player.FoodLevel = 10
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemGoldenApple, Count: 1}

	actorConnection.reset()
	observerConnection.reset()

	startFoodUse(t, actor)

	tickFoodUse(runtime, 32)

	assertEffectPacketCount(t, actorConnection, protocol.ClientboundUpdateMobEffectID, 2)
	assertEffectPacketCount(t, observerConnection, protocol.ClientboundUpdateMobEffectID, 0)

	actorConnection.reset()
	observerConnection.reset()

	actor.updatePlayerState(func(player *game.Player) bool {
		player.ActiveEffects.Clear()

		player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectSpeed, 1, 0, false, true, true))

		return true
	})

	runtime.Tick()

	assertEffectPacketCount(t, actorConnection, protocol.ClientboundRemoveMobEffectID, 1)
	assertEffectPacketCount(t, observerConnection, protocol.ClientboundRemoveMobEffectID, 0)
}

func assertPlayerEffect(t *testing.T, player game.Player, effect game.MobEffect, duration, amplifier int32) {
	t.Helper()

	instance, active := player.ActiveEffects.Find(effect)
	if !active || instance.Duration != duration || instance.Amplifier != amplifier || !instance.Visible || !instance.ShowIcon {
		t.Fatalf("effect %d = %+v, active %t, want duration %d amplifier %d visible icon", effect, instance, active, duration, amplifier)
	}
}

func assertEffectPacketCount(t *testing.T, connection *recordingConnection, packetID int32, expected int) {
	t.Helper()

	packets := packetsByID(t, connection, packetID)
	if len(packets) != expected {
		t.Fatalf("effect packets %#x = %d, want %d", packetID, len(packets), expected)
	}
}
