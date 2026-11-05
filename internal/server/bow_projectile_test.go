package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type bowPickupTest struct {
	name        string
	policy      arrowPickup
	mode        game.GameMode
	full        bool
	wantRemoved bool
	wantItem    bool
}

func TestBowVelocityAndClientMovement(t *testing.T) {
	actor, _ := newBowTestSession(t)
	runtime := actor.Runtime
	player := actor.snapshotPlayer()
	player.Velocity = game.Velocity{X: 10, Y: 10, Z: 10}
	player.Rotation = game.Rotation{Yaw: 90, Pitch: 30}
	movement := game.Velocity{X: 0.1, Y: 0.2, Z: 0.3}
	weapon := game.ItemStack{Item: game.ItemBow, Count: 1}
	pickup := game.ItemStack{Item: game.ItemArrow, Count: 1}

	player.OnGround = true
	arrow := runtime.newPlayerArrow(&player, movement, weapon, pickup, 1, false)
	assertVelocityClose(t, arrow.Velocity, game.Velocity{X: -2.4981, Y: -1.5, Z: 0.3}, 0.001)

	if math.Abs(float64(arrow.Rotation.Yaw+90)) > 0.01 || math.Abs(float64(arrow.Rotation.Pitch+30)) > 0.01 {
		t.Fatalf("rotation before inherited movement = %+v", arrow.Rotation)
	}

	player.OnGround = false
	arrow = runtime.newPlayerArrow(&player, movement, weapon, pickup, 1, false)
	assertVelocityClose(t, arrow.Velocity, game.Velocity{X: -2.4981, Y: -1.3, Z: 0.3}, 0.001)

	startBowTestUse(t, actor, protocol.MainHand)
	tickFoodUse(runtime, 20)
	position := actor.Player.Position

	runtime.updatePlayerMovement(actor, func(player *game.Player) {
		player.Position.X += 0.1
		player.Position.Y += 0.2
		player.OnGround = false
	})

	releaseBowTestUse(t, actor)
	arrow = onlyBowTestArrow(t, runtime)
	assertVelocityClose(t, arrow.Velocity, game.Velocity{X: 0.1, Y: 0.2, Z: 3}, 1e-9)

	if actor.Player.Position.X != position.X+0.1 {
		t.Fatal("movement was not accepted")
	}

	actor.handleClientTickEnd()
	assertVelocityClose(t, actor.knownMovement, game.Velocity{X: 0.1, Y: 0.2}, 1e-9)
	actor.handleClientTickEnd()
	assertVelocityClose(t, actor.knownMovement, game.Velocity{}, 0)
}

func TestBowMountedMovementInheritance(t *testing.T) {
	for seat := range 2 {
		controller, _ := newBowTestSession(t)
		runtime := controller.Runtime
		passenger, _ := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)
		passenger.Player.Inventory = controller.Player.Inventory.Clone()

		boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{Y: 5})
		if !runtime.MountPassenger(boat, controller) || !runtime.MountPassenger(boat, passenger) {
			t.Fatal("failed to mount shooters")
		}

		shooter := controller

		if seat == 1 {
			shooter = passenger
		}

		shooter.Player.OnGround = false
		startBowTestUse(t, shooter, protocol.MainHand)
		tickFoodUse(runtime, 20)
		controller.handleMoveVehicle(protocol.MoveVehicle{X: 0.25, Y: 5.2, Z: 0.5})
		controller.handleMoveVehicle(protocol.MoveVehicle{X: 20, Y: 5.2, Z: 0.5})

		runtime.updatePlayerMovement(controller, func(player *game.Player) {
			player.Rotation.HeadYaw++
		})

		releaseBowTestUse(t, shooter)
		arrow := onlyBowTestArrow(t, runtime)
		assertVelocityClose(t, arrow.Velocity, game.Velocity{X: 0.25, Y: 0.2, Z: 3.5}, 1e-9)
	}
}

func TestBowInaccuracy(t *testing.T) {
	actor, _ := newBowTestSession(t)
	index := 0

	actor.Runtime.entityRandom = func() float32 {
		index++
		if index%2 == 1 {
			return 0.75
		}

		return 0.25
	}

	arrow := actor.Runtime.newPlayerArrow(actor.Player, game.Velocity{}, actor.Player.Inventory.Hotbar[0], actor.Player.Inventory.Main[0], 1, false)
	deviation := 0.0172275 * 0.5 * 3
	assertVelocityClose(t, arrow.Velocity, game.Velocity{X: deviation, Y: deviation, Z: 3 + deviation}, 1e-9)

	if index != 6 {
		t.Fatalf("inaccuracy samples = %d, want 6", index)
	}
}

func TestBowEnchantmentsAndMobDamage(t *testing.T) {
	actor, _ := newBowTestSession(t)
	runtime := actor.Runtime

	runtime.entityRandom = func() float32 {
		return 0
	}

	weapon := &actor.Player.Inventory.Hotbar[0]
	weapon.SetEnchantment(game.EnchantmentPower, 3)
	weapon.SetEnchantment(game.EnchantmentPunch, 2)
	weapon.SetEnchantment(game.EnchantmentFlame, 1)
	startBowTestUse(t, actor, protocol.MainHand)
	tickFoodUse(runtime, 20)
	releaseBowTestUse(t, actor)
	arrow := onlyBowTestArrow(t, runtime)

	if arrow.RemainingFireTicks != 2000 || arrow.Weapon.EnchantmentLevel(game.EnchantmentPower) != 3 {
		t.Fatalf("projectile enchantments = %+v", arrow)
	}

	actor.Player.Inventory.Hotbar[0].SetEnchantment(game.EnchantmentPower, 5)

	if arrow.Weapon.EnchantmentLevel(game.EnchantmentPower) != 3 {
		t.Fatal("projectile weapon aliases inventory components")
	}

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 20, Y: 70}, 40)
	applied := runtime.damageArrowTarget(arrowTarget{entity: target}, arrow.State.ID, actor.Player.EntityID, game.Velocity{Z: 3})

	if !applied || target.Living.Health != 28 || target.Living.LastDamageCauseEntityID != actor.Player.EntityID || target.Living.LastDamageType != game.DamageArrow || target.Living.RemainingFireTicks != 100 {
		t.Fatalf("mob damage state = %+v", target.Living)
	}

	assertVelocityClose(t, target.Living.Velocity, game.Velocity{Y: 0.1, Z: 1.2}, 1e-7)
	target.Living.InvulnerableTime = 0
	runtime.tickRuntimeLivingBaseEnvironment(target)

	if target.Living.Health != 27 || target.Living.RemainingFireTicks != 99 {
		t.Fatal("Flame did not use existing burning damage")
	}

	target.Living.InvulnerableTime = game.LivingHurtCooldownTicks
	target.Living.LastHurt = 100
	target.Living.RemainingFireTicks = 40
	velocity := target.Living.Velocity

	if runtime.damageArrowTarget(arrowTarget{entity: target}, arrow.State.ID, actor.Player.EntityID, game.Velocity{Z: 3}) {
		t.Fatal("invulnerable target accepted arrow damage")
	}

	if target.Living.RemainingFireTicks != 40 || target.Living.Velocity != velocity {
		t.Fatal("rejected damage changed target fire or knockback")
	}
}

func TestBowPlayerDamageAndAttribution(t *testing.T) {
	actor, _ := newBowTestSession(t)
	runtime := actor.Runtime
	runtime.Difficulty = game.DifficultyPeaceful

	runtime.entityRandom = func() float32 {
		return 0
	}

	actor.Player.Inventory.Hotbar[0].SetEnchantment(game.EnchantmentFlame, 1)
	actor.Player.Inventory.Hotbar[0].SetEnchantment(game.EnchantmentPunch, 1)
	startBowTestUse(t, actor, protocol.MainHand)
	tickFoodUse(runtime, 20)
	releaseBowTestUse(t, actor)
	arrow := onlyBowTestArrow(t, runtime)

	target, connection := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)
	applied := runtime.damageArrowTarget(arrowTarget{session: target}, arrow.State.ID, actor.Player.EntityID, game.Velocity{Z: 3})

	if !applied || target.Player.Health != 14 || target.Player.RemainingFireTicks != 100 {
		t.Fatalf("PvP damage = health %v, fire %d", target.Player.Health, target.Player.RemainingFireTicks)
	}

	assertVelocityClose(t, target.Player.Velocity, game.Velocity{Y: 0.1, Z: 0.6}, 1e-7)
	foundDamage := false

	for _, packet := range connection.packets(t) {
		if packet.ID != protocol.ClientboundDamageEventID {
			continue
		}

		reader := protocol.NewPacketReader(packet.Data)
		entityID := reader.VarInt()
		reader.VarInt()

		cause := reader.VarInt()
		direct := reader.VarInt()

		if entityID != target.Player.EntityID || cause != actor.Player.EntityID+1 || direct != arrow.State.ID+1 {
			t.Fatalf("damage packet entity/cause/direct = %d/%d/%d", entityID, cause, direct)
		}

		foundDamage = true
	}

	if !foundDamage {
		t.Fatal("missing PvP damage attribution packet")
	}

	modes := []game.GameMode{game.GameModeCreative, game.GameModeSpectator}

	for _, mode := range modes {
		target.Player.ResetSurvivalState()

		target.Player.GameMode = mode
		if runtime.damageArrowTarget(arrowTarget{session: target}, arrow.State.ID, actor.Player.EntityID, game.Velocity{Z: 3}) || target.Player.RemainingFireTicks != 0 || target.Player.Health != 20 {
			t.Fatalf("protected mode %v received damage/fire", mode)
		}
	}
}

func TestBowCriticalDamage(t *testing.T) {
	actor, _ := newBowTestSession(t)
	startBowTestUse(t, actor, protocol.MainHand)
	tickFoodUse(actor.Runtime, 20)
	releaseBowTestUse(t, actor)
	arrow := onlyBowTestArrow(t, actor.Runtime)

	actor.Runtime.entityRandom = func() float32 {
		return 0.999
	}

	target := spawnTestRuntimeLivingEntity(actor.Runtime, game.Position{}, 20)
	actor.Runtime.damageArrowTarget(arrowTarget{entity: target}, arrow.State.ID, actor.Player.EntityID, game.Velocity{Z: 3})

	if target.Living.Health != 10 {
		t.Fatalf("critical damage = %v, want 10", 20-target.Living.Health)
	}
}

func TestBowArrowPickup(t *testing.T) {
	tests := []bowPickupTest{
		{name: "ordinary", policy: arrowPickupAllowed, mode: game.GameModeSurvival, wantRemoved: true, wantItem: true},
		{name: "full", policy: arrowPickupAllowed, mode: game.GameModeSurvival, full: true},
		{name: "infinite survival", policy: arrowPickupCreativeOnly, mode: game.GameModeSurvival},
		{name: "infinite creative", policy: arrowPickupCreativeOnly, mode: game.GameModeCreative, full: true, wantRemoved: true},
		{name: "disallowed", policy: arrowPickupDisallowed, mode: game.GameModeCreative},
		{name: "spectator", policy: arrowPickupAllowed, mode: game.GameModeSpectator},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actor, _ := newBowTestSession(t)
			actor.Player.GameMode = test.mode
			actor.Player.Inventory = game.PlayerInventory{}

			if test.full {
				for index := range actor.Player.Inventory.Hotbar {
					actor.Player.Inventory.Hotbar[index] = game.ItemStack{Item: game.ItemStone, Count: 64}
				}

				for index := range actor.Player.Inventory.Main {
					actor.Player.Inventory.Main[index] = game.ItemStack{Item: game.ItemStone, Count: 64}
				}
			}

			arrow := actor.Runtime.SpawnArrow(actor.Player.Position, game.Velocity{}, actor.Player.EntityID)
			arrow.InGround = true
			arrow.Pickup = test.policy
			arrow.PickupStack = game.ItemStack{Item: game.ItemArrow, Count: 1}
			arrow.ShakeTime = 1
			actor.Runtime.pickUpArrow(arrow)

			if arrow.State.Removed {
				t.Fatal("arrow picked up before shake ended")
			}

			arrow.ShakeTime = 0
			actor.Runtime.pickUpArrow(arrow)

			if arrow.State.Removed != test.wantRemoved || (actor.Player.Inventory.Hotbar[0].Item == game.ItemArrow) != test.wantItem {
				t.Fatalf("pickup = removed %t, item %+v", arrow.State.Removed, actor.Player.Inventory.Hotbar[0])
			}
		})
	}
}
