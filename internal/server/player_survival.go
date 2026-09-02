package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	playerHurtCooldownTicks      = 20
	playerHurtCooldownThreshold  = 10
	playerDrowningThreshold      = -20
	playerDrowningDamage         = 2
	playerLavaDamage             = 4
	playerBurningDamage          = 1
	playerFireDurationTicks      = 8 * 20
	playerLavaFireDurationTicks  = 15 * 20
	playerVoidDamage             = 4
	playerVoidDistanceBelowWorld = 64
	playerSafeFallDistance       = 3
	fastRegenerationFoodLevel    = 20
	slowRegenerationFoodLevel    = 18
)

type PlayerDamageType uint8

const (
	PlayerDamageFall PlayerDamageType = iota
	PlayerDamageDrown
	PlayerDamageInFire
	PlayerDamageLava
	PlayerDamageOnFire
	PlayerDamageOutOfWorld
	PlayerDamageGenericKill
	PlayerDamageStarve
	PlayerDamageMagic
)

type PlayerDamage struct {
	Type           PlayerDamageType
	Amount         float32
	CauseEntityID  int32
	DirectEntityID int32
	SourcePosition *game.Position
}

type playerSurvivalUpdate struct {
	player          game.Player
	damage          PlayerDamage
	healthChanged   bool
	metadataChanged bool
	fullHurt        bool
	drowned         bool
	died            bool
	inventoryBefore *game.PlayerInventory
	sounds          []protocol.Sound
	effectChanges   []playerMobEffectChange
}

func (s *Session) sendHealth() error {
	player := s.snapshotPlayer()

	return s.writePacket(protocol.ClientboundSetHealthID, protocol.SetHealth{
		Health:     player.Health,
		Food:       player.FoodLevel,
		Saturation: player.Saturation,
	})
}

func (s *Session) playerAlive() bool {
	return !s.snapshotPlayer().Dead
}

func (r *Runtime) DamagePlayer(session *Session, damage PlayerDamage) bool {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	update, applied := r.damagePlayerLocked(session, damage)

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

	if applied {
		r.sendPlayerSurvivalUpdate(session, update)
	}

	return applied
}

func (r *Runtime) RespawnPlayer(session *Session) error {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	player, changed := session.updatePlayerState(func(player *game.Player) bool {
		if !player.Dead {
			return false
		}

		player.ResetSurvivalState()

		player.Position = r.World.Spawn
		player.Rotation = game.Rotation{}
		player.Velocity = game.Velocity{}
		player.OnGround = true
		player.Sneaking = false
		player.Sprinting = false
		player.Swimming = false
		player.Pose = game.PlayerPoseStanding

		return true
	})

	if changed {
		r.cancelMiningLocked(session)
	}

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

	if !changed {
		return nil
	}

	respawn := protocol.Respawn{
		Spawn: protocol.SpawnInfo{
			DimensionType:    0,
			Dimension:        r.World.Name,
			Seed:             r.World.Seed,
			GameMode:         byte(player.GameMode),
			PreviousGameMode: byte(player.GameMode),
			SeaLevel:         r.World.SeaLevel,
		},
	}

	session.resetChunksForRespawn()

	err := session.writePacket(protocol.ClientboundRespawnID, respawn)
	if err != nil {
		return err
	}

	err = session.sendHealth()
	if err != nil {
		return err
	}

	err = session.sendInitialChunks()
	if err != nil {
		return err
	}

	err = session.sendPlayerPosition()
	if err != nil {
		return err
	}

	err = session.sendPlayerInventory()
	if err != nil {
		return err
	}

	err = session.sendPlayerMetadata(player)
	if err != nil {
		return err
	}

	for _, other := range r.snapshotSessions() {
		if other == session || !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
			continue
		}

		err = other.sendPlayerRemoval(player)
		if err == nil {
			err = other.sendPlayerEntity(player)
		}

		if err != nil && other.Log != nil {
			other.Log.Warnf("[play] failed to synchronize player respawn: %v\n", err)
		}
	}

	return nil
}

func (r *Runtime) tickPlayerSurvivalLocked(session *Session) []playerSurvivalUpdate {
	updates := make([]playerSurvivalUpdate, 0, 4)

	session.updatePlayerState(func(player *game.Player) bool {
		if player.SurvivalInitialized {
			return false
		}

		player.ResetSurvivalState()

		return true
	})

	player := session.snapshotPlayer()
	if player.Dead {
		return updates
	}

	useUpdates, useChanged := r.tickUsingItemLocked(session)
	if useChanged {
		updates = append(updates, useUpdates...)
	}

	player = session.snapshotPlayer()

	wasBurning := player.RemainingFireTicks > 0

	lava := r.fluidContact(player.CollisionBox(), game.FluidTypeLava, true).Depth > 0
	water := r.fluidContact(player.CollisionBox(), game.FluidTypeWater, true).Depth > 0

	fireDamage := r.playerFireContactDamage(player)

	burningDamageDue := player.RemainingFireTicks > 0 && player.RemainingFireTicks%20 == 0 && !lava && !water

	player, _ = session.updatePlayerState(func(player *game.Player) bool {
		changed := false

		if player.InvulnerableTime > 0 {
			player.InvulnerableTime--
		}

		if player.RemainingFireTicks > 0 {
			player.RemainingFireTicks--
			changed = true
		}

		if water && player.RemainingFireTicks != 0 {
			player.RemainingFireTicks = 0
			changed = true
		}

		if water && player.FallDistance != 0 {
			player.FallDistance = 0
			changed = true
		}

		if !water && fireDamage > 0 {
			addedTicks := int32(1 + int(r.nextEntityRandom()*2))

			player.RemainingFireTicks += addedTicks

			if player.RemainingFireTicks < playerFireDurationTicks {
				player.RemainingFireTicks = playerFireDurationTicks
			}

			capInvulnerablePlayerFireTicks(player)

			changed = true
		}

		if lava {
			if player.RemainingFireTicks < playerLavaFireDurationTicks {
				player.RemainingFireTicks = playerLavaFireDurationTicks
				changed = true
			}

			capInvulnerablePlayerFireTicks(player)

			player.FallDistance *= 0.5
		}

		return changed
	})

	if wasBurning != (player.RemainingFireTicks > 0) {
		updates = append(updates, playerSurvivalUpdate{player: player, metadataChanged: true})
	}

	if burningDamageDue {
		update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageOnFire, Amount: playerBurningDamage})
		if applied {
			updates = append(updates, update)
		}
	}

	if fireDamage > 0 {
		update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageInFire, Amount: fireDamage})
		if applied {
			updates = append(updates, update)
		}
	}

	if lava {
		update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageLava, Amount: playerLavaDamage})
		if applied {
			updates = append(updates, update)
		}
	}

	player = session.snapshotPlayer()
	if player.Dead {
		return updates
	}

	if player.Position.Y < float64(protocol.OverworldMinY-playerVoidDistanceBelowWorld) {
		update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageOutOfWorld, Amount: playerVoidDamage})
		if applied {
			updates = append(updates, update)
		}
	}

	player = session.snapshotPlayer()
	if player.Dead {
		return updates
	}

	underwater := r.playerEyeSubmerged(player)

	player, airChanged := session.updatePlayerState(func(player *game.Player) bool {
		previous := player.AirSupply

		hasWaterBreathing := playerHasWaterBreathing(*player)
		refillFromEffects := playerEffectsRefillAir(*player)

		if underwater && player.GameMode != game.GameModeCreative && player.GameMode != game.GameModeSpectator && !hasWaterBreathing {
			player.AirSupply--
		} else if player.AirSupply < game.DefaultPlayerAirSupply && (!underwater || refillFromEffects || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator) {
			player.AirSupply = min(player.AirSupply+4, game.DefaultPlayerAirSupply)
		}

		return previous != player.AirSupply
	})

	if player.AirSupply <= playerDrowningThreshold {
		player, _ = session.updatePlayerState(func(player *game.Player) bool {
			player.AirSupply = 0

			return true
		})

		update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageDrown, Amount: playerDrowningDamage})

		update.metadataChanged = true
		update.drowned = true

		if applied {
			updates = append(updates, update)
		} else {
			updates = append(updates, playerSurvivalUpdate{player: player, metadataChanged: true, drowned: true})
		}
	} else if airChanged {
		updates = append(updates, playerSurvivalUpdate{player: player, metadataChanged: true})
	}

	player = session.snapshotPlayer()
	if player.Dead {
		return updates
	}

	updates = append(updates, r.tickPlayerEffectsLocked(session)...)

	player = session.snapshotPlayer()
	if player.Dead {
		return updates
	}

	updates = append(updates, r.tickPlayerFoodLocked(session)...)

	return updates
}

func (r *Runtime) tickPlayerFoodLocked(session *Session) []playerSurvivalUpdate {
	updates := make([]playerSurvivalUpdate, 0, 2)

	var (
		healAmount     float32
		starvationDue  bool
		visibleChanged bool
	)

	player, _ := session.updatePlayerState(func(player *game.Player) bool {
		previousFood := player.FoodLevel
		previousSaturation := player.Saturation

		player.SurvivalTickCount++

		if player.Exhaustion > 4 {
			player.Exhaustion -= 4

			if player.Saturation > 0 {
				player.Saturation = max(player.Saturation-1, 0)
			} else if r.Difficulty != game.DifficultyPeaceful {
				player.FoodLevel = max(player.FoodLevel-1, 0)
			}
		}

		if player.Health < game.DefaultPlayerHealth && player.FoodLevel >= fastRegenerationFoodLevel && player.Saturation > 0 {
			player.FoodTickTimer++

			if player.FoodTickTimer >= 10 {
				saturationSpent := min(player.Saturation, 6)

				healAmount += saturationSpent / 6
				player.AddExhaustion(saturationSpent)

				player.FoodTickTimer = 0
			}
		} else if player.Health < game.DefaultPlayerHealth && player.FoodLevel >= slowRegenerationFoodLevel {
			player.FoodTickTimer++

			if player.FoodTickTimer >= 80 {
				healAmount++

				player.AddExhaustion(6)

				player.FoodTickTimer = 0
			}
		} else if player.FoodLevel <= 0 {
			player.FoodTickTimer++

			if player.FoodTickTimer >= 80 {
				starvationDue = player.Health > 10 || r.Difficulty == game.DifficultyHard || (r.Difficulty == game.DifficultyNormal && player.Health > 1)
				player.FoodTickTimer = 0
			}
		} else {
			player.FoodTickTimer = 0
		}

		if r.Difficulty == game.DifficultyPeaceful {
			if player.SurvivalTickCount%20 == 0 {
				if player.Health < game.DefaultPlayerHealth {
					healAmount++
				}

				if player.Saturation < game.MaxPlayerSaturation {
					player.Saturation = min(player.Saturation+1, float32(game.MaxPlayerSaturation))
				}
			}

			if player.SurvivalTickCount%10 == 0 && player.FoodLevel < game.DefaultPlayerFoodLevel {
				player.FoodLevel++
			}
		}

		visibleChanged = previousFood != player.FoodLevel || (previousSaturation == 0) != (player.Saturation == 0)

		return true
	})

	if healAmount > 0 {
		update, healed := r.healPlayerLocked(session, healAmount)
		if healed {
			updates = append(updates, update)
		}
	} else if visibleChanged {
		updates = append(updates, playerSurvivalUpdate{player: player, healthChanged: true})
	}

	if starvationDue {
		update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageStarve, Amount: 1})
		if applied {
			updates = append(updates, update)
		}
	}

	return updates
}

func (r *Runtime) healPlayerLocked(session *Session, amount float32) (playerSurvivalUpdate, bool) {
	player, healed := session.updatePlayerState(func(player *game.Player) bool {
		if player.Dead || amount <= 0 || player.Health >= game.DefaultPlayerHealth {
			return false
		}

		player.Health = min(player.Health+amount, float32(game.DefaultPlayerHealth))

		return true
	})

	if !healed {
		return playerSurvivalUpdate{}, false
	}

	return playerSurvivalUpdate{player: player, healthChanged: true, metadataChanged: true}, true
}

func (r *Runtime) addPlayerExhaustion(session *Session, amount float32) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	session.updatePlayerState(func(player *game.Player) bool {
		previous := player.Exhaustion

		player.AddExhaustion(amount)

		return previous != player.Exhaustion
	})
}

func (r *Runtime) damagePlayerLocked(session *Session, damage PlayerDamage) (playerSurvivalUpdate, bool) {
	if damage.Amount <= 0 {
		return playerSurvivalUpdate{}, false
	}

	if math.IsNaN(float64(damage.Amount)) || math.IsInf(float64(damage.Amount), 0) {
		damage.Amount = math.MaxFloat32
	}

	var (
		fullHurt      bool
		effectChanges []playerMobEffectChange
	)

	player, applied := session.updatePlayerState(func(player *game.Player) bool {
		if !player.SurvivalInitialized {
			player.ResetSurvivalState()
		}

		if player.Dead || !playerCanTakeDamage(*player, damage.Type) {
			return false
		}

		if playerDamageIsFire(damage.Type) && playerHasMobEffect(*player, game.MobEffectFireResistance) {
			return false
		}

		amount := damage.Amount

		if player.InvulnerableTime > playerHurtCooldownThreshold {
			if amount <= player.LastHurt {
				return false
			}

			amount -= player.LastHurt
			player.LastHurt = damage.Amount
		} else {
			player.LastHurt = damage.Amount
			player.InvulnerableTime = playerHurtCooldownTicks
			fullHurt = true
		}

		if !playerDamageBypassesEffects(damage.Type) {
			resistance, valid := player.ActiveEffects.Find(game.MobEffectResistance)
			if valid && !playerDamageBypassesResistance(damage.Type) {
				multiplier := max(float32(25-5*(resistance.Amplifier+1))/25, 0)
				amount *= multiplier
			}
		}

		absorbed := min(amount, player.Absorption)
		player.Absorption -= absorbed
		amount -= absorbed

		player.Health = max(0, player.Health-amount)
		if player.Health == 0 {
			player.Dead = true

			player.StopUsingItem()

			player.RemainingFireTicks = 0
			player.FallDistance = 0
			effectChanges = clearPlayerMobEffects(player)
		}

		return true
	})

	if !applied {
		return playerSurvivalUpdate{}, false
	}

	update := playerSurvivalUpdate{
		player:          player,
		damage:          damage,
		healthChanged:   true,
		metadataChanged: true,
		fullHurt:        fullHurt,
		died:            player.Dead,
		effectChanges:   effectChanges,
	}

	if !player.Dead {
		return update, true
	}

	r.cancelMiningLocked(session)

	transientDrops := r.discardMenuOnDeathLocked(session)

	before := session.snapshotPlayer().Inventory.Clone()
	drops := make([]game.ItemStack, 0, game.PlayerInventorySlots-1+len(transientDrops))

	drops = append(drops, transientDrops...)

	player, _ = session.updatePlayerState(func(player *game.Player) bool {
		if player.GameMode != game.GameModeSpectator {
			for slot := 1; slot <= 4; slot++ {
				stack := player.Inventory.Slot(slot)
				if stack == nil || stack.Empty() {
					continue
				}

				drops = append(drops, stack.Clone())
			}

			for slot := 9; slot < game.PlayerInventorySlots; slot++ {
				stack := player.Inventory.Slot(slot)
				if stack == nil || stack.Empty() || stack.PreventsEquipmentDrop() {
					continue
				}

				drops = append(drops, stack.Clone())
			}

			for slot := 5; slot <= 8; slot++ {
				stack := player.Inventory.Slot(slot)
				if stack == nil || stack.Empty() || stack.PreventsEquipmentDrop() {
					continue
				}

				drops = append(drops, stack.Clone())
			}

			player.Inventory = game.PlayerInventory{}
		}

		return true
	})

	for _, stack := range drops {
		r.spawnPlayerDroppedItem(player, stack, true, false)
	}

	update.player = player
	update.inventoryBefore = &before

	return update, true
}

func (r *Runtime) sendPlayerSurvivalUpdate(session *Session, update playerSurvivalUpdate) {
	if update.healthChanged {
		err := session.sendHealth()
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize player health: %v\n", err)
		}
	}

	if update.inventoryBefore != nil {
		err := session.synchronizePlayerInventoryMutation(*update.inventoryBefore)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize death inventory: %v\n", err)
		}
	}

	for _, recipient := range r.snapshotSessions() {
		recipientPlayer := recipient.snapshotPlayer()
		if recipient != session && !playersVisible(recipientPlayer, update.player, recipient.renderDistance()) {
			continue
		}

		for _, sound := range update.sounds {
			position := toBlockPosition(game.Position{X: sound.X, Y: sound.Y, Z: sound.Z})

			err := recipient.sendSoundIfLoaded(sound, position)
			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize consumption sound: %v\n", err)
			}
		}

		if update.fullHurt {
			damage := protocol.DamageEvent{
				EntityID:       update.player.EntityID,
				DamageType:     playerDamageRegistryID(update.damage.Type),
				CauseEntityID:  protocolEntityID(update.damage.CauseEntityID),
				DirectEntityID: protocolEntityID(update.damage.DirectEntityID),
			}

			if update.damage.SourcePosition != nil {
				damage.HasSourcePosition = true
				damage.SourcePositionX = update.damage.SourcePosition.X
				damage.SourcePositionY = update.damage.SourcePosition.Y
				damage.SourcePositionZ = update.damage.SourcePosition.Z
			}

			err := recipient.writePacket(protocol.ClientboundDamageEventID, damage)
			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize player damage: %v\n", err)
			}
		}

		if update.drowned {
			err := recipient.writePacket(protocol.ClientboundEntityEventID, protocol.EntityEvent{EntityID: update.player.EntityID, Event: 67})
			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize drowning event: %v\n", err)
			}
		}

		if update.metadataChanged {
			err := recipient.sendPlayerMetadata(update.player)
			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize survival metadata: %v\n", err)
			}
		}

		for _, change := range update.effectChanges {
			var err error

			if change.removed {
				err = recipient.sendPlayerMobEffectRemoval(update.player.EntityID, change.instance.Effect)
			} else {
				err = recipient.sendPlayerMobEffect(update.player.EntityID, change.instance)
			}

			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize player effect: %v\n", err)
			}
		}

		if update.died {
			err := recipient.writePacket(protocol.ClientboundEntityEventID, protocol.EntityEvent{EntityID: update.player.EntityID, Event: 3})
			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize player death: %v\n", err)
			}
		}
	}

	if update.died {
		message := game.TranslatableText(playerDeathTranslation(update.damage.Type), game.LiteralText(update.player.Name))

		err := session.writePacket(protocol.ClientboundCombatKillID, protocol.CombatKill{PlayerID: update.player.EntityID, Message: message})
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to present player death: %v\n", err)
		}
	}
}

func (r *Runtime) sendPlayerSurvivalUpdates(session *Session, updates []playerSurvivalUpdate) {
	for _, update := range updates {
		r.sendPlayerSurvivalUpdate(session, update)
	}
}

func (r *Runtime) playerFireContactDamage(player game.Player) float32 {
	return r.fireContactDamage(player.CollisionBox())
}

func (r *Runtime) fireContactDamage(box game.AABB) float32 {
	minX := int32(math.Floor(box.MinX))
	minY := int32(math.Floor(box.MinY))
	minZ := int32(math.Floor(box.MinZ))
	maxX := int32(math.Ceil(box.MaxX)) - 1
	maxY := int32(math.Ceil(box.MaxY)) - 1
	maxZ := int32(math.Ceil(box.MaxZ)) - 1

	var damage float32

	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			for z := minZ; z <= maxZ; z++ {
				block := r.World.BlockAt(game.BlockPosition{X: x, Y: y, Z: z})

				definition, valid := block.Definition()
				if !valid {
					continue
				}

				switch definition.ID {
				case game.SoulFireID:
					damage = max(damage, 2)
				case game.FireID:
					damage = max(damage, 1)
				}
			}
		}
	}

	return damage
}

func calculatePlayerFallDamage(fallDistance float32) float32 {
	unsafeDistance := fallDistance - playerSafeFallDistance
	if unsafeDistance <= 0 {
		return 0
	}

	return float32(math.Ceil(float64(unsafeDistance)))
}

func playerHasWaterBreathing(player game.Player) bool {
	waterBreathingEffects := []game.MobEffect{game.MobEffectWaterBreathing, game.MobEffectConduitPower, game.MobEffectBreathOfTheNautilus}

	for _, effect := range waterBreathingEffects {
		_, active := player.ActiveEffects.Find(effect)
		if active {
			return true
		}
	}

	return false
}

func playerEffectsRefillAir(player game.Player) bool {
	_, nautilus := player.ActiveEffects.Find(game.MobEffectBreathOfTheNautilus)
	_, waterBreathing := player.ActiveEffects.Find(game.MobEffectWaterBreathing)
	_, conduitPower := player.ActiveEffects.Find(game.MobEffectConduitPower)

	return !nautilus || waterBreathing || conduitPower
}

func playerCanTakeDamage(player game.Player, damageType PlayerDamageType) bool {
	if damageType == PlayerDamageOutOfWorld || damageType == PlayerDamageGenericKill {
		return true
	}

	return player.GameMode != game.GameModeCreative && player.GameMode != game.GameModeSpectator
}

func capInvulnerablePlayerFireTicks(player *game.Player) {
	if player.GameMode != game.GameModeCreative && player.GameMode != game.GameModeSpectator {
		return
	}

	player.RemainingFireTicks = min(player.RemainingFireTicks, 1)
}

func playerDamageRegistryID(damageType PlayerDamageType) int32 {
	switch damageType {
	case PlayerDamageDrown:
		return 6
	case PlayerDamageFall:
		return 10
	case PlayerDamageGenericKill:
		return 19
	case PlayerDamageInFire:
		return 21
	case PlayerDamageLava:
		return 24
	case PlayerDamageOnFire:
		return 31
	case PlayerDamageOutOfWorld:
		return 32
	case PlayerDamageStarve:
		return 40
	case PlayerDamageMagic:
		return 27
	default:
		return 18
	}
}

func playerDeathTranslation(damageType PlayerDamageType) string {
	switch damageType {
	case PlayerDamageDrown:
		return "death.attack.drown"
	case PlayerDamageFall:
		return "death.attack.fall"
	case PlayerDamageGenericKill:
		return "death.attack.genericKill"
	case PlayerDamageInFire:
		return "death.attack.inFire"
	case PlayerDamageLava:
		return "death.attack.lava"
	case PlayerDamageOnFire:
		return "death.attack.onFire"
	case PlayerDamageOutOfWorld:
		return "death.attack.outOfWorld"
	case PlayerDamageStarve:
		return "death.attack.starve"
	case PlayerDamageMagic:
		return "death.attack.magic"
	default:
		return "death.attack.generic"
	}
}

func playerHasMobEffect(player game.Player, effect game.MobEffect) bool {
	_, valid := player.ActiveEffects.Find(effect)
	return valid
}

func playerDamageIsFire(damageType PlayerDamageType) bool {
	return damageType == PlayerDamageInFire || damageType == PlayerDamageLava || damageType == PlayerDamageOnFire
}

func playerDamageBypassesEffects(damageType PlayerDamageType) bool {
	return damageType == PlayerDamageStarve
}

func playerDamageBypassesResistance(damageType PlayerDamageType) bool {
	return damageType == PlayerDamageOutOfWorld || damageType == PlayerDamageGenericKill
}

func protocolEntityID(entityID int32) int32 {
	if entityID == 0 {
		return 0
	}

	return entityID + 1
}
