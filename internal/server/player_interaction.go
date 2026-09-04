package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	playerInteractionVerificationBuffer = 3.0
	playerAttackExhaustion              = 0.1
	playerHurtKnockback                 = 0.4
	playerSprintKnockback               = 0.5
	playerFullAttackStrength            = 0.9
	playerCriticalDamageMultiplier      = 1.5
	playerKnockbackMinimumDirection     = 1.0e-5
	playerKnockbackRandomScale          = 0.01
	playerAttackCriticalSound           = "minecraft:entity.player.attack.crit"
	playerAttackKnockbackSound          = "minecraft:entity.player.attack.knockback"
	playerAttackNoDamageSound           = "minecraft:entity.player.attack.nodamage"
	playerAttackStrongSound             = "minecraft:entity.player.attack.strong"
	playerAttackWeakSound               = "minecraft:entity.player.attack.weak"
)

type playerAttackResult struct {
	targetSession   *Session
	update          playerSurvivalUpdate
	attacker        game.Player
	target          game.Player
	heldItem        game.ItemStack
	preSounds       []protocol.Sound
	postSounds      []protocol.Sound
	damagePerAttack int32
	targetMotion    bool
	sprintCancelled bool
	critical        bool
	attempted       bool
	applied         bool
}

func (r *Runtime) handlePlayerInteraction(attackerSession *Session, interaction protocol.Interact) {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	attackerSession.updatePlayerState(func(player *game.Player) bool {
		player.Sneaking = interaction.SecondaryAction

		return true
	})

	targetSession, target, valid := r.playerInteractionTargetLocked(attackerSession, interaction)
	if !valid || interaction.Action != protocol.InteractActionAttack {
		r.lifecycleMu.Unlock()
		r.worldMutationMu.Unlock()

		return
	}

	result := r.attackPlayerLocked(attackerSession, targetSession, target)

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

	if !result.attempted {
		return
	}

	r.sendPlayerAttackSounds(result.attacker, result.preSounds)

	if !result.applied {
		r.sendPlayerAttackSounds(result.attacker, result.postSounds)

		return
	}

	result.update.attackerName = result.attacker.Name

	r.sendPlayerSurvivalUpdate(result.targetSession, result.update)

	if result.targetMotion {
		r.sendPlayerKnockback(result.target)
	}

	if result.sprintCancelled {
		r.sendPlayerAttackMetadata(attackerSession, result.attacker)
	}

	r.sendPlayerAttackSounds(result.attacker, result.postSounds)

	if result.critical {
		err := attackerSession.sendPlayerAnimation(result.target, protocol.EntityAnimationCriticalHit)
		if err != nil && attackerSession.Log != nil {
			attackerSession.Log.Warnf("[play] failed to synchronize critical attack: %v\n", err)
		}
	}

	if result.damagePerAttack > 0 {
		before, broke := r.damageHeldItem(attackerSession, protocol.MainHand, result.heldItem, result.damagePerAttack)
		if before != nil {
			r.sendPlayerAttackInventoryUpdate(attackerSession, *before, broke)
		}
	}

}

func (r *Runtime) playerInteractionTargetLocked(attackerSession *Session, interaction protocol.Interact) (*Session, game.Player, bool) {
	attacker := attackerSession.snapshotPlayer()
	if attacker.Dead || attacker.GameMode == game.GameModeSpectator || attacker.EntityID == interaction.EntityID {
		return nil, game.Player{}, false
	}

	targetSession := r.playerSessionByEntityIDLocked(interaction.EntityID)
	if targetSession == nil {
		return nil, game.Player{}, false
	}

	target := targetSession.snapshotPlayer()
	if target.Dead {
		return nil, game.Player{}, false
	}

	inclusive := interaction.Action == protocol.InteractActionAttack
	if !attacker.IsWithinEntityInteractionRange(target.CollisionBox(), playerInteractionVerificationBuffer, inclusive) {
		return nil, game.Player{}, false
	}

	return targetSession, target, true
}

func (r *Runtime) playerSessionByEntityIDLocked(entityID int32) *Session {
	for _, session := range r.snapshotSessions() {
		if session.snapshotPlayer().EntityID == entityID {
			return session
		}
	}

	return nil
}

func (r *Runtime) attackPlayerLocked(attackerSession, targetSession *Session, target game.Player) playerAttackResult {
	result := playerAttackResult{targetSession: targetSession, target: target, attempted: true}

	attacker := attackerSession.snapshotPlayer()

	strength := attacker.AttackStrength()
	fullStrength := strength > playerFullAttackStrength
	critical := fullStrength && r.playerCanCriticalAttack(attacker)
	knockbackAttack := fullStrength && attacker.Sprinting

	baseDamage := attacker.MainHandAttackDamage()
	baseDamage *= 0.2 + 0.8*strength*strength

	if critical {
		baseDamage *= playerCriticalDamageMultiplier
	}

	if knockbackAttack {
		result.preSounds = append(result.preSounds, playerAttackSound(attacker, playerAttackKnockbackSound))
	}

	attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
		player.ResetAttackStrength()

		return true
	})

	held := attacker.Inventory.Held(attacker.SelectedHotbarSlot)
	if held != nil {
		result.heldItem = held.Clone()
	}

	update, applied := r.damagePlayerLocked(targetSession, PlayerDamage{
		Type:           PlayerDamagePlayerAttack,
		Amount:         baseDamage,
		CauseEntityID:  attacker.EntityID,
		DirectEntityID: attacker.EntityID,
	})

	if !applied {
		result.attacker = attacker
		result.postSounds = append(result.postSounds, playerAttackSound(attacker, playerAttackNoDamageSound))

		return result
	}

	originalVelocity := target.Velocity
	targetMotion := false

	if update.fullHurt {
		target, _ = targetSession.updatePlayerState(func(player *game.Player) bool {
			directionX := attacker.Position.X - player.Position.X
			directionZ := attacker.Position.Z - player.Position.Z

			r.applyPlayerKnockback(player, directionX, directionZ, playerHurtKnockback)

			return true
		})

		targetMotion = true
	}

	attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
		player.AddExhaustion(playerAttackExhaustion)

		return true
	})

	if knockbackAttack {
		target, _ = targetSession.updatePlayerState(func(player *game.Player) bool {
			yaw := float64(attacker.Rotation.Yaw) * math.Pi / 180
			directionX := math.Sin(yaw)
			directionZ := -math.Cos(yaw)

			r.applyPlayerKnockback(player, directionX, directionZ, playerSprintKnockback)

			return true
		})

		targetMotion = true

		attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
			player.Velocity.X *= 0.6
			player.Velocity.Z *= 0.6
			player.Sprinting = false

			return true
		})
	}

	if targetMotion {
		update.player = target

		targetSession.updatePlayerState(func(player *game.Player) bool {
			player.Velocity = originalVelocity

			return true
		})
	}

	if critical {
		result.postSounds = append(result.postSounds, playerAttackSound(attacker, playerAttackCriticalSound))
	} else if fullStrength {
		result.postSounds = append(result.postSounds, playerAttackSound(attacker, playerAttackStrongSound))
	} else {
		result.postSounds = append(result.postSounds, playerAttackSound(attacker, playerAttackWeakSound))
	}

	result.update = update
	result.attacker = attacker
	result.target = target

	if attacker.GameMode != game.GameModeCreative {
		result.damagePerAttack = attacker.MainHandDamagePerAttack()
	}

	result.targetMotion = targetMotion
	result.sprintCancelled = knockbackAttack
	result.critical = critical
	result.applied = true

	return result
}

func (r *Runtime) sendPlayerAttackSounds(attacker game.Player, sounds []protocol.Sound) {
	position := toBlockPosition(attacker.Position)

	for _, sound := range sounds {
		for _, recipient := range r.snapshotSessions() {
			err := recipient.sendSoundIfLoaded(sound, position)
			if err != nil && recipient.Log != nil {
				recipient.Log.Warnf("[play] failed to synchronize player attack sound: %v\n", err)
			}
		}
	}
}

func (r *Runtime) sendPlayerAttackMetadata(attackerSession *Session, attacker game.Player) {
	for _, recipient := range r.snapshotSessions() {
		if recipient == attackerSession || !playersVisible(recipient.snapshotPlayer(), attacker, recipient.renderDistance()) {
			continue
		}

		err := recipient.sendPlayerMetadata(attacker)
		if err != nil && recipient.Log != nil {
			recipient.Log.Warnf("[play] failed to synchronize sprint attack state: %v\n", err)
		}
	}
}

func (r *Runtime) sendPlayerAttackInventoryUpdate(session *Session, before game.PlayerInventory, broke bool) {
	err := session.synchronizePlayerInventoryMutation(before)
	if err != nil && session.Log != nil {
		session.Log.Warnf("[play] failed to synchronize attack durability: %v\n", err)
	}

	if !broke {
		return
	}

	player := session.snapshotPlayer()

	event := protocol.EntityEvent{EntityID: player.EntityID, Event: 47}

	for _, recipient := range r.snapshotSessions() {
		if recipient != session && !playersVisible(recipient.snapshotPlayer(), player, recipient.renderDistance()) {
			continue
		}

		err = recipient.writePacket(protocol.ClientboundEntityEventID, event)
		if err != nil && recipient.Log != nil {
			recipient.Log.Warnf("[play] failed to send weapon break event: %v\n", err)
		}
	}
}

func (r *Runtime) sendPlayerKnockback(target game.Player) {
	motion := protocol.SetEntityMotion{
		EntityID:  target.EntityID,
		VelocityX: target.Velocity.X,
		VelocityY: target.Velocity.Y,
		VelocityZ: target.Velocity.Z,
	}

	for _, recipient := range r.snapshotSessions() {
		if recipient.snapshotPlayer().EntityID != target.EntityID && !playersVisible(recipient.snapshotPlayer(), target, recipient.renderDistance()) {
			continue
		}

		err := recipient.writePacket(protocol.ClientboundSetEntityMotionID, motion)
		if err != nil && recipient.Log != nil {
			recipient.Log.Warnf("[play] failed to synchronize player knockback: %v\n", err)
		}
	}
}

func (r *Runtime) playerCanCriticalAttack(attacker game.Player) bool {
	if attacker.FallDistance <= 0 || attacker.OnGround || attacker.Sprinting {
		return false
	}

	inWater := r.fluidContact(attacker.CollisionBox(), game.FluidTypeWater, true).Depth > 0
	if inWater {
		return false
	}

	return !playerHasMobEffect(attacker, game.MobEffectBlindness)
}

func (r *Runtime) applyPlayerKnockback(target *game.Player, directionX, directionZ, strength float64) {
	resistance := min(max(float64(target.ArmorAttributes().KnockbackResistance), 0), 1)
	strength *= 1 - resistance

	if strength <= 0 {
		return
	}

	for directionX*directionX+directionZ*directionZ < playerKnockbackMinimumDirection {
		directionX = float64(r.nextEntityRandom()-r.nextEntityRandom()) * playerKnockbackRandomScale
		directionZ = float64(r.nextEntityRandom()-r.nextEntityRandom()) * playerKnockbackRandomScale
	}

	length := math.Hypot(directionX, directionZ)
	directionX /= length
	directionZ /= length

	target.Velocity.X = target.Velocity.X/2 - directionX*strength

	if target.OnGround {
		target.Velocity.Y = min(0.4, target.Velocity.Y/2+strength)
	}

	target.Velocity.Z = target.Velocity.Z/2 - directionZ*strength
}

func playerAttackSound(attacker game.Player, name string) protocol.Sound {
	return protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: name},
		Source: protocol.SoundSourcePlayer,
		X:      attacker.Position.X,
		Y:      attacker.Position.Y,
		Z:      attacker.Position.Z,
		Volume: 1,
		Pitch:  1,
	}
}
