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
	playerAttackCriticalSound           = "minecraft:entity.player.attack.crit"
	playerAttackKnockbackSound          = "minecraft:entity.player.attack.knockback"
	playerAttackNoDamageSound           = "minecraft:entity.player.attack.nodamage"
	playerAttackStrongSound             = "minecraft:entity.player.attack.strong"
	playerAttackWeakSound               = "minecraft:entity.player.attack.weak"
)

type playerAttackResult struct {
	targetSession   *Session
	update          playerSurvivalUpdate
	runtimeUpdate   runtimeLivingDamageUpdate
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
	targetEntityID  int32
}

type playerAttackTarget struct {
	session       *Session
	player        game.Player
	runtimeLiving RuntimeLivingEntity
	entityID      int32
	position      game.Position
}

func (r *Runtime) handlePlayerInteraction(attackerSession *Session, interaction protocol.Interact) {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	attackerSession.updatePlayerState(func(player *game.Player) bool {
		player.Sneaking = interaction.SecondaryAction

		return true
	})

	target, valid := r.playerInteractionTargetLocked(attackerSession, interaction)
	if !valid || interaction.Action != protocol.InteractActionAttack {
		r.lifecycleMu.Unlock()
		r.worldMutationMu.Unlock()

		return
	}

	result := r.attackLivingTargetLocked(attackerSession, target)

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

	if result.targetSession != nil {
		result.update.attackerName = result.attacker.Name

		r.sendPlayerSurvivalUpdate(result.targetSession, result.update)
	} else {
		r.sendRuntimeLivingDamageUpdate(result.runtimeUpdate)
	}

	if result.targetMotion && result.targetSession != nil {
		r.sendPlayerKnockback(result.target)
	}

	if result.sprintCancelled {
		r.sendPlayerAttackMetadata(attackerSession, result.attacker)
	}

	r.sendPlayerAttackSounds(result.attacker, result.postSounds)

	if result.critical {
		err := attackerSession.sendEntityAnimation(result.targetEntityID, protocol.EntityAnimationCriticalHit)
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

func (r *Runtime) playerInteractionTargetLocked(attackerSession *Session, interaction protocol.Interact) (playerAttackTarget, bool) {
	attacker := attackerSession.snapshotPlayer()
	if attacker.Dead || attacker.GameMode == game.GameModeSpectator || attacker.EntityID == interaction.EntityID {
		return playerAttackTarget{}, false
	}

	targetSession := r.playerSessionByEntityIDLocked(interaction.EntityID)
	if targetSession != nil {
		target := targetSession.snapshotPlayer()
		if target.Dead {
			return playerAttackTarget{}, false
		}

		inclusive := interaction.Action == protocol.InteractActionAttack
		if !attacker.IsWithinEntityInteractionRange(target.CollisionBox(), playerInteractionVerificationBuffer, inclusive) {
			return playerAttackTarget{}, false
		}

		return playerAttackTarget{session: targetSession, player: target, entityID: target.EntityID, position: target.Position}, true
	}

	r.entityMu.RLock()
	runtimeEntity := r.entities[interaction.EntityID]
	r.entityMu.RUnlock()

	runtimeLiving, living := runtimeEntity.(RuntimeLivingEntity)
	if !living {
		return playerAttackTarget{}, false
	}

	state := runtimeLiving.RuntimeEntityState()
	livingState := runtimeLiving.RuntimeLivingState()

	state.mu.RLock()
	removed := state.Removed
	dead := livingState.Dead
	position := state.Position

	box := livingState.CollisionBox(position)

	validDimensions := livingState.Width > 0 && livingState.Height > 0
	state.mu.RUnlock()

	if removed || dead || !validDimensions {
		return playerAttackTarget{}, false
	}

	inclusive := interaction.Action == protocol.InteractActionAttack
	if !attacker.IsWithinEntityInteractionRange(box, playerInteractionVerificationBuffer, inclusive) {
		return playerAttackTarget{}, false
	}

	return playerAttackTarget{runtimeLiving: runtimeLiving, entityID: interaction.EntityID, position: position}, true
}

func (r *Runtime) playerSessionByEntityIDLocked(entityID int32) *Session {
	for _, session := range r.snapshotSessions() {
		if session.snapshotPlayer().EntityID == entityID {
			return session
		}
	}

	return nil
}

func (r *Runtime) attackLivingTargetLocked(attackerSession *Session, target playerAttackTarget) playerAttackResult {
	result := playerAttackResult{
		targetSession:  target.session,
		target:         target.player,
		targetEntityID: target.entityID,
		attempted:      true,
	}

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

	damage := game.Damage{
		Type:           PlayerDamagePlayerAttack,
		Amount:         baseDamage,
		CauseEntityID:  attacker.EntityID,
		DirectEntityID: attacker.EntityID,
	}

	var (
		fullHurt bool
		applied  bool
	)

	if target.session != nil {
		result.update, applied = r.damagePlayerLocked(target.session, damage)
		fullHurt = result.update.fullHurt
	} else {
		result.runtimeUpdate, applied = r.damageRuntimeLivingEntityLocked(target.runtimeLiving, damage)
		fullHurt = result.runtimeUpdate.fullHurt
	}

	if !applied {
		result.attacker = attacker
		result.postSounds = append(result.postSounds, playerAttackSound(attacker, playerAttackNoDamageSound))

		return result
	}

	originalVelocity := target.player.Velocity
	targetMotion := false

	if fullHurt {
		if target.session != nil {
			result.target, _ = target.session.updatePlayerState(func(player *game.Player) bool {
				directionX := attacker.Position.X - player.Position.X
				directionZ := attacker.Position.Z - player.Position.Z

				r.applyPlayerKnockback(player, directionX, directionZ, playerHurtKnockback)

				return true
			})
		} else {
			directionX := attacker.Position.X - target.position.X
			directionZ := attacker.Position.Z - target.position.Z

			r.applyRuntimeLivingKnockback(target.runtimeLiving, directionX, directionZ, playerHurtKnockback)
		}

		targetMotion = true
	}

	attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
		player.AddExhaustion(playerAttackExhaustion)

		return true
	})

	if knockbackAttack {
		yaw := float64(attacker.Rotation.Yaw) * math.Pi / 180

		directionX := math.Sin(yaw)
		directionZ := -math.Cos(yaw)

		if target.session != nil {
			result.target, _ = target.session.updatePlayerState(func(player *game.Player) bool {
				r.applyPlayerKnockback(player, directionX, directionZ, playerSprintKnockback)

				return true
			})
		} else {
			r.applyRuntimeLivingKnockback(target.runtimeLiving, directionX, directionZ, playerSprintKnockback)
		}

		targetMotion = true

		attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
			player.Velocity.X *= 0.6
			player.Velocity.Z *= 0.6
			player.Sprinting = false

			return true
		})
	}

	if targetMotion && target.session != nil {
		result.update.player = result.target

		target.session.updatePlayerState(func(player *game.Player) bool {
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

	result.attacker = attacker

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
	resistance := target.ArmorAttributes().KnockbackResistance

	game.ApplyLivingKnockback(&target.Velocity, target.OnGround, resistance, directionX, directionZ, strength, r.nextEntityRandom)
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
