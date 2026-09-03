package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	playerInteractionVerificationBuffer = 3.0
	playerAttackExhaustion              = 0.1
	playerSprintKnockback               = 0.5
	playerFullAttackStrength            = 0.9
)

type playerAttackResult struct {
	targetSession   *Session
	update          playerSurvivalUpdate
	attacker        game.Player
	target          game.Player
	heldItem        game.ItemStack
	damagePerAttack int32
	knockedBack     bool
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

	if !result.applied {
		return
	}

	result.update.attackerName = result.attacker.Name

	r.sendPlayerSurvivalUpdate(result.targetSession, result.update)

	if result.damagePerAttack > 0 {
		before, broke := r.damageHeldItem(attackerSession, protocol.MainHand, result.heldItem, result.damagePerAttack)
		if before != nil {
			r.sendPlayerAttackInventoryUpdate(attackerSession, *before, broke)
		}
	}

	if result.knockedBack {
		r.sendPlayerKnockback(result.target)
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
	result := playerAttackResult{targetSession: targetSession, target: target}

	attacker := attackerSession.snapshotPlayer()

	strength := attacker.AttackStrength()
	damage := attacker.MainHandAttackDamage() * (0.2 + 0.8*strength*strength)

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
		Amount:         damage,
		CauseEntityID:  attacker.EntityID,
		DirectEntityID: attacker.EntityID,
	})

	if !applied {
		return result
	}

	attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
		player.AddExhaustion(playerAttackExhaustion)

		return true
	})

	knockedBack := attacker.Sprinting && strength > playerFullAttackStrength
	if knockedBack {
		target, _ = targetSession.updatePlayerState(func(player *game.Player) bool {
			applyPlayerKnockback(player, attacker)

			return true
		})

		attacker, _ = attackerSession.updatePlayerState(func(player *game.Player) bool {
			player.Velocity.X *= 0.6
			player.Velocity.Z *= 0.6
			player.Sprinting = false

			return true
		})
	}

	result.update = update
	result.attacker = attacker
	result.target = target

	if attacker.GameMode != game.GameModeCreative {
		result.damagePerAttack = attacker.MainHandDamagePerAttack()
	}

	result.knockedBack = knockedBack
	result.applied = true

	return result
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

func applyPlayerKnockback(target *game.Player, attacker game.Player) {
	yaw := float64(attacker.Rotation.Yaw) * math.Pi / 180
	directionX := math.Sin(yaw)
	directionZ := -math.Cos(yaw)

	target.Velocity.X = target.Velocity.X/2 - directionX*playerSprintKnockback

	if target.OnGround {
		target.Velocity.Y = min(0.4, target.Velocity.Y/2+playerSprintKnockback)
	}

	target.Velocity.Z = target.Velocity.Z/2 - directionZ*playerSprintKnockback
	target.OnGround = false
	target.FallDistance = 0
}
