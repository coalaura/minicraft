package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type potionInstantEffectTest struct {
	name       string
	potion     game.Potion
	health     float32
	resistance bool
	absorption float32
	wantHealth float32
	wantAbsorb float32
}

type potionEffectDurationTest struct {
	name    string
	potion  game.Potion
	effects []game.MobEffectInstance
}

var potionVisibilityEffects = []game.MobEffect{game.MobEffectInvisibility, game.MobEffectGlowing}

func TestPotionZeroEffectContentsConsumeWithoutEffects(t *testing.T) {
	potions := []game.Potion{game.PotionWater, game.PotionMundane, game.PotionAwkward}

	for _, potion := range potions {
		t.Run(potionName(t, potion), func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

			actor.Player.Inventory.Hotbar[0] = potionStack(potion)

			startFoodUse(t, actor)

			tickFoodUse(runtime, 32)

			player := actor.snapshotPlayer()
			if len(player.ActiveEffects) != 0 || !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemGlassBottle, Count: 1}) {
				t.Fatalf("zero-effect potion result = %+v", player)
			}
		})
	}
}

func TestPotionInstantEffectsRespectResistanceAndAbsorption(t *testing.T) {
	tests := []potionInstantEffectTest{
		{name: "healing", potion: game.PotionHealing, health: 10, wantHealth: 16},
		{name: "strong healing", potion: game.PotionStrongHealing, health: 10, wantHealth: 20},
		{name: "harming", potion: game.PotionHarming, health: 20, wantHealth: 14},
		{name: "strong harming", potion: game.PotionStrongHarming, health: 20, wantHealth: 8},
		{name: "strong harming resistance absorption", potion: game.PotionStrongHarming, health: 20, resistance: true, absorption: 4, wantHealth: 14.4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

			actor.updatePlayerState(func(player *game.Player) bool {
				player.Health = test.health
				player.Absorption = test.absorption

				if test.resistance {
					player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectResistance, 100, 0, false, true, true))
				}

				if test.absorption > 0 {
					player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectAbsorption, 100, 0, false, true, true))
				}

				return true
			})

			actor.Player.Inventory.Hotbar[0] = potionStack(test.potion)

			startFoodUse(t, actor)

			tickFoodUse(runtime, 32)

			player := actor.snapshotPlayer()
			if player.Health != test.wantHealth || player.Absorption != test.wantAbsorb {
				t.Fatalf("instant potion state = health %v absorption %v, want %v %v", player.Health, player.Absorption, test.wantHealth, test.wantAbsorb)
			}
		})
	}
}

func TestPotionEffectsApplyExpectedDurationsAndTurtleMaster(t *testing.T) {
	tests := []potionEffectDurationTest{
		{name: "regeneration", potion: game.PotionRegeneration, effects: []game.MobEffectInstance{game.NewMobEffectInstance(game.MobEffectRegeneration, 900, 0, false, true, true)}},
		{name: "poison", potion: game.PotionLongPoison, effects: []game.MobEffectInstance{game.NewMobEffectInstance(game.MobEffectPoison, 1800, 0, false, true, true)}},
		{name: "turtle master", potion: game.PotionTurtleMaster, effects: []game.MobEffectInstance{
			game.NewMobEffectInstance(game.MobEffectSlowness, 400, 3, false, true, true),
			game.NewMobEffectInstance(game.MobEffectResistance, 400, 2, false, true, true),
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

			actor.Player.Inventory.Hotbar[0] = potionStack(test.potion)

			startFoodUse(t, actor)

			tickFoodUse(runtime, 32)

			player := actor.snapshotPlayer()

			for _, effect := range test.effects {
				assertPlayerEffect(t, player, effect.Effect, effect.Duration, effect.Amplifier)
			}

		})
	}
}

func TestPotionCompletionDefersPeriodicEffectBehavior(t *testing.T) {
	t.Run("regeneration", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionRegeneration)

		startFoodUse(t, actor)

		tickFoodUse(runtime, 31)

		actor.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10

			return true
		})

		runtime.Tick()

		player := actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectRegeneration, 900, 0)

		if player.Health != 10 {
			t.Fatalf("regeneration ran on completion tick: health %v", player.Health)
		}

		runtime.Tick()

		player = actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectRegeneration, 899, 0)

		if player.Health != 11 {
			t.Fatalf("regeneration did not run on next tick: health %v", player.Health)
		}
	})

	t.Run("poison", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionLongPoison)

		startFoodUse(t, actor)

		tickFoodUse(runtime, 31)

		actor.updatePlayerState(func(player *game.Player) bool {
			player.Health = 10
			player.InvulnerableTime = 0
			player.LastHurt = 0

			return true
		})

		runtime.Tick()

		player := actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectPoison, 1800, 0)

		if player.Health != 10 {
			t.Fatalf("poison ran on completion tick: health %v", player.Health)
		}

		runtime.Tick()

		player = actor.snapshotPlayer()

		assertPlayerEffect(t, player, game.MobEffectPoison, 1799, 0)

		if player.Health != 9 {
			t.Fatalf("poison did not run on next tick: health %v", player.Health)
		}
	})
}

func TestPotionWaterBreathingRefillsAirExpiresAndFireResistanceBlocksLava(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

	runtime := NewRuntime(world)

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionWaterBreathing)

	startFoodUse(t, actor)

	tickFoodUse(runtime, 31)

	actor.updatePlayerState(func(player *game.Player) bool {
		player.AirSupply = 10

		return true
	})

	runtime.Tick()

	player := actor.snapshotPlayer()

	assertPlayerEffect(t, player, game.MobEffectWaterBreathing, 3600, 0)

	if player.AirSupply != 9 {
		t.Fatalf("water breathing changed completion-tick air = %d, want 9", player.AirSupply)
	}

	runtime.Tick()

	player = actor.snapshotPlayer()
	if player.AirSupply != 13 {
		t.Fatalf("water breathing next-tick air refill = %d, want 13", player.AirSupply)
	}

	actor.updatePlayerState(func(player *game.Player) bool {
		effect, active := player.ActiveEffects.Find(game.MobEffectWaterBreathing)
		if !active {
			t.Fatal("water breathing missing")
		}

		effect.Duration = 1

		player.ActiveEffects.Remove(game.MobEffectWaterBreathing)
		player.ActiveEffects.Add(effect)

		return true
	})

	runtime.Tick()
	runtime.Tick()

	if actor.snapshotPlayer().AirSupply != 16 {
		t.Fatalf("air after water breathing expiry = %d, want 16", actor.snapshotPlayer().AirSupply)
	}

	actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionFireResistance)

	startFoodUse(t, actor)

	tickFoodUse(runtime, 31)

	world.SetBlock(game.BlockPosition{}, game.Lava)

	actor.updatePlayerState(func(player *game.Player) bool {
		player.Health = game.DefaultPlayerHealth
		player.InvulnerableTime = 0
		player.LastHurt = 0
		player.RemainingFireTicks = 0

		return true
	})

	runtime.Tick()

	player = actor.snapshotPlayer()

	assertPlayerEffect(t, player, game.MobEffectFireResistance, 3600, 0)

	if player.Health != game.DefaultPlayerHealth-playerLavaDamage {
		t.Fatalf("fire resistance retroactively blocked completion-tick lava: health %v", player.Health)
	}

	health := player.Health

	actor.updatePlayerState(func(player *game.Player) bool {
		player.InvulnerableTime = 0
		player.LastHurt = 0

		return true
	})

	runtime.Tick()

	if actor.snapshotPlayer().Health != health {
		t.Fatalf("fire resistance did not block next-tick lava damage: health %v -> %v", health, actor.snapshotPlayer().Health)
	}
}

func TestPotionFallEffectsResetDistanceAndIncreaseSafeDistance(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime := NewRuntime(world)

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Player.Position = game.Position{X: 0.5, Y: 10, Z: 0.5}
	actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionSlowFalling)

	startFoodUse(t, actor)

	tickFoodUse(runtime, 31)

	actor.updatePlayerState(func(player *game.Player) bool {
		player.FallDistance = 7

		return true
	})

	runtime.Tick()

	player := actor.snapshotPlayer()
	if player.FallDistance != 7 {
		t.Fatalf("slow falling reset accumulated distance on completion: %v", player.FallDistance)
	}

	runtime.updatePlayerMovement(actor, func(player *game.Player) {
		player.Position.Y = 1
		player.OnGround = true
	})

	player = actor.snapshotPlayer()
	if player.FallDistance != 0 || player.Health != game.DefaultPlayerHealth {
		t.Fatalf("slow falling landing = %+v", player)
	}

	actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionLeaping)

	startFoodUse(t, actor)

	tickFoodUse(runtime, 32)

	actor.updatePlayerState(func(player *game.Player) bool {
		player.ActiveEffects.Remove(game.MobEffectSlowFalling)
		player.Health = game.DefaultPlayerHealth
		player.Position.Y = 5
		player.FallDistance = 0

		return true
	})

	runtime.updatePlayerMovement(actor, func(player *game.Player) {
		player.Position.Y = 1
		player.OnGround = true
	})

	if actor.snapshotPlayer().Health != game.DefaultPlayerHealth {
		t.Fatalf("jump boost safe fall caused damage: %+v", actor.snapshotPlayer())
	}
}

func TestPotionCustomEffectsScaleChainsMetadataAndInventory(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)
	observer, observerConnection := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)

	position := toBlockPosition(actor.Player.Position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	hidden := game.NewMobEffectInstance(game.MobEffectSpeed, 10, 0, false, true, true)
	custom := game.NewMobEffectInstance(game.MobEffectSpeed, 2, 1, false, false, false)

	custom.HiddenEffect = &hidden

	stack := potionStack(game.PotionNightVision)

	stack.SetPotionDurationScale(0.5)
	stack.SetPotionContents(game.PotionContents{
		Potion:        game.PotionNightVision,
		HasPotion:     true,
		CustomEffects: []game.MobEffectInstance{custom, game.NewMobEffectInstance(game.MobEffectInvisibility, 10, 0, false, true, true), game.NewMobEffectInstance(game.MobEffectGlowing, 10, 0, false, true, true)},
	})

	actor.Player.Inventory.Hotbar[0] = stack

	actorConnection.reset()
	observerConnection.reset()

	startFoodUse(t, actor)

	tickFoodUse(runtime, 32)

	player := actor.snapshotPlayer()

	assertPlayerEffect(t, player, game.MobEffectNightVision, 1800, 0)

	speed, speedActive := player.ActiveEffects.Find(game.MobEffectSpeed)
	if !speedActive || speed.Duration != 1 || speed.Amplifier != 1 || speed.Ambient || speed.Visible || speed.ShowIcon || speed.HiddenEffect != nil {
		t.Fatalf("scaled custom speed = %+v, active %t", speed, speedActive)
	}

	if player.Inventory.Hotbar[0].Item != game.ItemGlassBottle {
		t.Fatalf("survival potion remainder = %+v", player.Inventory.Hotbar[0])
	}

	assertPotionMetadataFlags(t, observerConnection, actor.Player.EntityID, protocol.EntityFlagInvisible|protocol.EntityFlagGlowing)

	newObserver, newObserverConnection := newBlockMutationTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "New observer", game.GameModeSurvival)

	joinTestSession(t, runtime, newObserver)

	assertPotionMetadataFlags(t, newObserverConnection, actor.Player.EntityID, protocol.EntityFlagInvisible|protocol.EntityFlagGlowing)

	tickFoodUse(runtime, 1)

	player = actor.snapshotPlayer()

	_, speedActive = player.ActiveEffects.Find(game.MobEffectSpeed)
	if speedActive {
		t.Fatalf("scaled speed remained active after expiry: %+v", player.ActiveEffects)
	}

	actor.updatePlayerState(func(player *game.Player) bool {
		for _, effectType := range potionVisibilityEffects {
			effect, active := player.ActiveEffects.Find(effectType)
			if !active {
				t.Fatalf("missing effect %d before expiry", effectType)
			}

			effect.Duration = 1
			player.ActiveEffects.Remove(effectType)
			player.ActiveEffects.Add(effect)
		}

		return true
	})

	runtime.Tick()

	assertPotionMetadataFlags(t, observerConnection, actor.Player.EntityID, 0)

	offhand, _ := newFoodUseTestSession(t, runtime, "Offhand", game.GameModeSurvival)

	offhand.Player.Inventory.Offhand = potionStack(game.PotionHealing)

	err := offhand.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 2})
	if err != nil {
		t.Fatalf("start offhand survival potion: %v", err)
	}

	tickFoodUse(runtime, 32)

	if !offhand.snapshotPlayer().Inventory.Offhand.Equal(game.ItemStack{Item: game.ItemGlassBottle, Count: 1}) {
		t.Fatalf("offhand survival potion remainder = %+v", offhand.snapshotPlayer().Inventory.Offhand)
	}

	creative, _ := newFoodUseTestSession(t, runtime, "Creative", game.GameModeCreative)

	creative.Player.Inventory.Offhand = potionStack(game.PotionHealing)

	err = creative.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 3})
	if err != nil {
		t.Fatalf("start offhand creative potion: %v", err)
	}

	tickFoodUse(runtime, 32)

	if !creative.snapshotPlayer().Inventory.Offhand.Equal(potionStack(game.PotionHealing)) {
		t.Fatalf("creative offhand potion was consumed: %+v", creative.snapshotPlayer().Inventory.Offhand)
	}
}

func TestPotionUseRefreshesSameItemComponents(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Player.Health = game.DefaultPlayerHealth
	actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionHealing)

	startFoodUse(t, actor)

	tickFoodUse(runtime, 4)

	actor.Player.Inventory.Hotbar[0] = potionStack(game.PotionHarming)

	tickFoodUse(runtime, 28)

	if actor.snapshotPlayer().Health != 14 {
		t.Fatalf("mutated potion contents health = %v, want 14", actor.snapshotPlayer().Health)
	}
}

func potionStack(potion game.Potion) game.ItemStack {
	stack := game.ItemStack{Item: game.ItemPotion, Count: 1}

	stack.SetPotionContents(game.PotionContents{Potion: potion, HasPotion: true})

	return stack
}

func potionName(t *testing.T, potion game.Potion) string {
	t.Helper()

	definition, valid := potion.Definition()
	if !valid {
		t.Fatalf("invalid potion %d", potion)
	}

	return definition.Name
}

func assertPotionMetadataFlags(t *testing.T, connection *recordingConnection, entityID int32, flags byte) {
	t.Helper()

	for _, packet := range packetsByID(t, connection, protocol.ClientboundEntityMetadataID) {
		reader := protocol.NewPacketReader(packet.Data)

		if reader.VarInt() != entityID {
			continue
		}

		if reader.Byte() != protocol.EntityFlagsMetadataIndex || reader.VarInt() != protocol.MetadataTypeByte {
			t.Fatalf("effect metadata = %v, want flags %#x", packet, flags)
		}

		if reader.Byte() == flags {
			return
		}
	}

	t.Fatalf("missing metadata for entity %d", entityID)
}
