package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const bowUseDuration uint32 = 72000

func (r *Runtime) startUsingBow(session *Session, hand int32, expected game.ItemStack) error {
	r.lifecycleMu.Lock()

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		if player.Dead || player.GameMode == game.GameModeSpectator {
			return false
		}

		held, valid := heldItemPointer(player, hand)
		if !valid || !held.Equal(expected) {
			return false
		}

		if player.UsingItem && player.UsingOffhand == (hand == protocol.OffHand) && player.UseStack.Equal(expected) {
			return false
		}

		cancelled := player.StopUsingItem()

		ammo := bowAmmunition(player)
		if ammo != nil && ammo.Item != game.ItemArrow || ammo == nil && player.GameMode != game.GameModeCreative {
			return cancelled
		}

		player.UsingItem = true
		player.UsingOffhand = hand == protocol.OffHand
		player.UseRemainingTicks = bowUseDuration
		player.UseAnimation = game.ItemUseAnimationBow
		player.UseStack = held.Clone()

		return true
	})

	r.lifecycleMu.Unlock()

	if changed {
		r.sendPlayerMetadataUpdates([]game.Player{player})
	}

	return nil
}

func (r *Runtime) releaseUsingItem(session *Session) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	movement := r.playerProjectileMovement(session)

	var (
		before game.PlayerInventory
		arrow  *runtimeArrowEntity
		power  float32
		broke  bool
		hand   int32
	)

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		if !player.UsingItem {
			return false
		}

		hand = protocol.MainHand

		if player.UsingOffhand {
			hand = protocol.OffHand
		}

		held, valid := heldItemPointer(player, hand)
		validBow := valid && held.Item == game.ItemBow && held.Equal(player.UseStack)
		remaining := player.UseRemainingTicks

		// Clear before committing: repeated releases and cancelled uses cannot shoot.
		player.StopUsingItem()

		if !validBow || player.Dead || player.GameMode == game.GameModeSpectator || remaining > bowUseDuration {
			return true
		}

		power = bowPower(bowUseDuration - remaining)
		if power < 0.1 {
			return true
		}

		ammo := bowAmmunition(player)
		if ammo != nil && ammo.Item != game.ItemArrow || ammo == nil && player.GameMode != game.GameModeCreative {
			return true
		}

		before = player.Inventory.Clone()
		pickup := game.ItemStack{Item: game.ItemArrow, Count: 1}

		if ammo != nil {
			pickup = ammo.Clone()
			pickup.Count = 1
		}

		infinite := player.GameMode == game.GameModeCreative || held.EnchantmentLevel(game.EnchantmentInfinity) > 0
		arrow = r.newPlayerArrow(player, movement, *held, pickup, power, infinite)

		if !infinite {
			ammo.Count--
			if ammo.Empty() {
				*ammo = game.ItemStack{}
			}
		}

		if player.GameMode != game.GameModeCreative {
			definition, _ := held.Item.Definition()
			broke, _ = r.damageItemStack(held, 1, definition.MaxDurability)
		}

		return true
	})

	if !changed {
		return
	}

	update := playerSurvivalUpdate{player: player, metadataChanged: true}

	if arrow != nil {
		position := player.EyePosition()
		position.Y -= float64(float32(0.1))

		r.registerRuntimeEntity(arrow, game.EntityArrow, position)

		pitch := 1/(r.nextEntityRandom()*0.4+1.2) + power*0.5
		update.sounds = []protocol.Sound{playerConsumptionSound(player, "minecraft:entity.arrow.shoot", protocol.SoundSourcePlayer, 1, pitch)}
		update.inventoryBefore = &before
	}

	r.sendPlayerSurvivalUpdate(session, update)

	if broke {
		r.sendPlayerItemBreakEvent(session, hand)
	}
}

func (r *Runtime) playerProjectileMovement(session *Session) game.Velocity {
	session.playerMx.RLock()
	movement := session.knownMovement
	entityID := session.Player.EntityID
	session.playerMx.RUnlock()

	vehicleID := r.passengerVehicleID(entityID)
	if vehicleID == 0 {
		return movement
	}

	controller := r.boatController(vehicleID)
	if controller != nil {
		controller.playerMx.RLock()
		movement = controller.knownMovement
		controller.playerMx.RUnlock()

		return movement
	}

	r.entityMu.RLock()
	vehicle, valid := r.entities[vehicleID].(RuntimeEntityVelocity)
	r.entityMu.RUnlock()

	if valid {
		return vehicle.EntityVelocity()
	}

	return movement
}

func (r *Runtime) newPlayerArrow(player *game.Player, movement game.Velocity, weapon, pickup game.ItemStack, power float32, infinite bool) *runtimeArrowEntity {
	velocity := bowShotVelocity(player.Rotation, power, r.nextEntityRandom)
	rotation := arrowRotation(velocity)
	velocity.X += movement.X
	velocity.Z += movement.Z

	if !player.OnGround {
		velocity.Y += movement.Y
	}

	entity := &runtimeArrowEntity{
		Velocity:    velocity,
		OwnerID:     player.EntityID,
		Rotation:    rotation,
		Critical:    power == 1,
		Weapon:      weapon.Clone(),
		PickupStack: pickup,
		Pickup:      arrowPickupAllowed,
	}

	if infinite {
		entity.Pickup = arrowPickupCreativeOnly
	}

	if weapon.EnchantmentLevel(game.EnchantmentFlame) > 0 {
		entity.RemainingFireTicks = 2000
	}

	return entity
}

func (s *Session) cancelBowUse() {
	changed := s.mutatePlayer(func(player *game.Player) bool {
		if player.UseStack.Item != game.ItemBow {
			return false
		}

		return player.StopUsingItem()
	})

	if changed {
		player := s.snapshotPlayer()
		s.Runtime.sendPlayerMetadataUpdates([]game.Player{player})
	}
}

func (s *Session) cancelInvalidBowUse() {
	changed := s.mutatePlayer(func(player *game.Player) bool {
		if !player.UsingItem || player.UseStack.Item != game.ItemBow {
			return false
		}

		hand := int32(protocol.MainHand)

		if player.UsingOffhand {
			hand = protocol.OffHand
		}

		held, valid := heldItemPointer(player, hand)
		if valid && held.Equal(player.UseStack) {
			return false
		}

		return player.StopUsingItem()
	})

	if changed {
		player := s.snapshotPlayer()
		s.Runtime.sendPlayerMetadataUpdates([]game.Player{player})
	}
}

func bowPower(ticks uint32) float32 {
	fraction := float32(ticks) / 20

	return min(1, (fraction*fraction+2*fraction)/3)
}

func bowAmmunition(player *game.Player) *game.ItemStack {
	if bowProjectile(player.Inventory.Offhand) {
		return &player.Inventory.Offhand
	}

	held := player.Inventory.Held(player.SelectedHotbarSlot)
	if held != nil && bowProjectile(*held) {
		return held
	}

	for index := range player.Inventory.Hotbar {
		stack := &player.Inventory.Hotbar[index]
		if bowProjectile(*stack) {
			return stack
		}
	}

	for index := range player.Inventory.Main {
		stack := &player.Inventory.Main[index]
		if bowProjectile(*stack) {
			return stack
		}
	}

	return nil
}

func bowProjectile(stack game.ItemStack) bool {
	return !stack.Empty() && (stack.Item == game.ItemArrow || stack.Item == game.ItemSpectralArrow || stack.Item == game.ItemTippedArrow)
}

func bowShotVelocity(rotation game.Rotation, power float32, random func() float32) game.Velocity {
	yaw := rotation.Yaw * float32(math.Pi/180)
	pitch := rotation.Pitch * float32(math.Pi/180)
	direction := game.Velocity{
		X: float64(-minecraftSin(float64(yaw)) * minecraftCos(float64(pitch))),
		Y: float64(-minecraftSin(float64(pitch))),
		Z: float64(minecraftCos(float64(yaw)) * minecraftCos(float64(pitch))),
	}
	length := arrowVelocityLength(direction)
	speed := float64(power * 3)

	return game.Velocity{
		X: (direction.X/length + 0.0172275*(float64(random())-float64(random()))) * speed,
		Y: (direction.Y/length + 0.0172275*(float64(random())-float64(random()))) * speed,
		Z: (direction.Z/length + 0.0172275*(float64(random())-float64(random()))) * speed,
	}
}
