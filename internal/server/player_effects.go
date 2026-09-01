package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type playerMobEffectChange struct {
	instance game.MobEffectInstance
	removed  bool
}

func (r *Runtime) tickPlayerEffectsLocked(session *Session) []playerSurvivalUpdate {
	updates := make([]playerSurvivalUpdate, 0, 3)
	player := session.snapshotPlayer()

	for _, instance := range player.ActiveEffects {
		if session.snapshotPlayer().Dead {
			return updates
		}

		if instance.Effect == game.MobEffectAbsorption && player.Absorption <= 0 {
			change, changed := removePlayerMobEffect(session, game.MobEffectAbsorption)
			if changed {
				updates = append(updates, playerSurvivalUpdate{player: session.snapshotPlayer(), metadataChanged: true, effectChanges: []playerMobEffectChange{change}})
			}

			continue
		}

		switch instance.Effect {
		case game.MobEffectRegeneration:
			if mobEffectCadenceDue(instance.Duration, 50, instance.Amplifier) {
				update, healed := r.healPlayerLocked(session, 1)
				if healed {
					updates = append(updates, update)
				}
			}
		case game.MobEffectPoison:
			current := session.snapshotPlayer()
			if current.Health > 1 && mobEffectCadenceDue(instance.Duration, 25, instance.Amplifier) {
				update, applied := r.damagePlayerLocked(session, PlayerDamage{Type: PlayerDamageMagic, Amount: 1})
				if applied {
					updates = append(updates, update)
				}
			}
		case game.MobEffectHunger:
			session.updatePlayerState(func(player *game.Player) bool {
				previous := player.Exhaustion

				player.AddExhaustion(0.005 * float32(instance.Amplifier+1))

				return previous != player.Exhaustion
			})
		}
	}

	if session.snapshotPlayer().Dead {
		return updates
	}

	var changes []playerMobEffectChange
	metadataChanged := false

	player, _ = session.updatePlayerState(func(player *game.Player) bool {
		active := player.ActiveEffects[:0]

		for index := range player.ActiveEffects {
			instance := &player.ActiveEffects[index]
			before := *instance

			if !instance.Tick() {
				changes = append(changes, playerMobEffectChange{instance: before, removed: true})

				continue
			}

			active = append(active, *instance)

			if mobEffectDetailsChanged(before, *instance) || (!instance.Infinite() && instance.Duration%600 == 0) {
				changes = append(changes, playerMobEffectChange{instance: instance.Clone()})
			}
		}

		clear(player.ActiveEffects[len(active):])

		player.ActiveEffects = active

		previousAbsorption := player.Absorption

		clampPlayerAbsorption(player)

		metadataChanged = previousAbsorption != player.Absorption

		return len(changes) != 0 || metadataChanged
	})

	if len(changes) != 0 || metadataChanged {
		updates = append(updates, playerSurvivalUpdate{player: player, metadataChanged: metadataChanged, effectChanges: changes})
	}

	return updates
}

func addPlayerMobEffect(player *game.Player, instance game.MobEffectInstance) (playerMobEffectChange, bool, bool) {
	changed := player.ActiveEffects.Add(instance)
	previousAbsorption := player.Absorption

	if instance.Effect == game.MobEffectAbsorption {
		player.Absorption = max(player.Absorption, 4*float32(instance.Amplifier+1))

		clampPlayerAbsorption(player)
	}

	active, valid := player.ActiveEffects.Find(instance.Effect)
	if !valid {
		return playerMobEffectChange{}, false, previousAbsorption != player.Absorption
	}

	return playerMobEffectChange{instance: active}, changed, previousAbsorption != player.Absorption
}

func (r *Runtime) applyConsumableMobEffects(player *game.Player, effects []game.ItemConsumeEffect) ([]playerMobEffectChange, bool) {
	var changes []playerMobEffectChange
	absorptionChanged := false

	for _, effect := range effects {
		switch effect.Type {
		case game.ItemConsumeEffectApplyStatusEffects:
			if r.nextEntityRandom() >= effect.Probability {
				continue
			}

			for _, instance := range effect.Effects {
				change, changed, absorptionUpdated := addPlayerMobEffect(player, instance)
				absorptionChanged = absorptionChanged || absorptionUpdated

				if changed {
					changes = append(changes, change)
				}
			}
		case game.ItemConsumeEffectRemoveStatusEffects:
			for _, removed := range effect.Remove {
				instance, valid := player.ActiveEffects.Find(removed)
				if !valid || !player.ActiveEffects.Remove(removed) {
					continue
				}

				changes = append(changes, playerMobEffectChange{instance: instance, removed: true})
			}

			previousAbsorption := player.Absorption

			clampPlayerAbsorption(player)

			absorptionChanged = absorptionChanged || previousAbsorption != player.Absorption
		case game.ItemConsumeEffectClearAllStatusEffects:
			previousAbsorption := player.Absorption

			changes = append(changes, clearPlayerMobEffects(player)...)

			absorptionChanged = absorptionChanged || previousAbsorption != player.Absorption
		case game.ItemConsumeEffectTeleportRandomly, game.ItemConsumeEffectSuspiciousStew:
			// Dynamic stew effects and safe random teleportation require separate stack-component and teleport primitives.
		}
	}

	return changes, absorptionChanged
}

func removePlayerMobEffect(session *Session, effect game.MobEffect) (playerMobEffectChange, bool) {
	var removed game.MobEffectInstance

	_, changed := session.updatePlayerState(func(player *game.Player) bool {
		var valid bool

		removed, valid = player.ActiveEffects.Find(effect)
		if !valid || !player.ActiveEffects.Remove(effect) {
			return false
		}

		clampPlayerAbsorption(player)

		return true
	})

	return playerMobEffectChange{instance: removed, removed: true}, changed
}

func clearPlayerMobEffects(player *game.Player) []playerMobEffectChange {
	changes := make([]playerMobEffectChange, 0, len(player.ActiveEffects))

	for index := range player.ActiveEffects {
		changes = append(changes, playerMobEffectChange{instance: player.ActiveEffects[index].Clone(), removed: true})
	}

	player.ActiveEffects.Clear()

	clampPlayerAbsorption(player)

	return changes
}

func clampPlayerAbsorption(player *game.Player) {
	maximum := float32(0)
	instance, valid := player.ActiveEffects.Find(game.MobEffectAbsorption)

	if valid {
		maximum = 4 * float32(instance.Amplifier+1)
	}

	player.Absorption = min(max(player.Absorption, 0), maximum)
}

func mobEffectCadenceDue(duration, base, amplifier int32) bool {
	interval := base >> amplifier
	return interval == 0 || duration%interval == 0
}

func mobEffectDetailsChanged(previous, current game.MobEffectInstance) bool {
	expectedDuration := previous.Duration
	if !previous.Infinite() && expectedDuration != 0 {
		expectedDuration--
	}

	return expectedDuration != current.Duration || previous.Amplifier != current.Amplifier ||
		previous.Ambient != current.Ambient ||
		previous.Visible != current.Visible ||
		previous.ShowIcon != current.ShowIcon
}

func (s *Session) sendPlayerMobEffects(player game.Player) error {
	for index := range player.ActiveEffects {
		err := s.sendPlayerMobEffect(player.EntityID, player.ActiveEffects[index])
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) sendPlayerMobEffect(entityID int32, instance game.MobEffectInstance) error {
	packet := protocol.UpdateMobEffect{
		EntityID:  entityID,
		EffectID:  int32(instance.Effect),
		Amplifier: instance.Amplifier,
		Duration:  instance.Duration,
		Flags:     playerMobEffectFlags(instance),
	}

	return s.writePacket(protocol.ClientboundUpdateMobEffectID, packet)
}

func (s *Session) sendPlayerMobEffectRemoval(entityID int32, effect game.MobEffect) error {
	packet := protocol.RemoveMobEffect{EntityID: entityID, EffectID: int32(effect)}

	return s.writePacket(protocol.ClientboundRemoveMobEffectID, packet)
}

func playerMobEffectFlags(instance game.MobEffectInstance) byte {
	flags := byte(0)

	if instance.Ambient {
		flags |= protocol.MobEffectFlagAmbient
	}

	if instance.Visible {
		flags |= protocol.MobEffectFlagShowParticles
	}

	if instance.ShowIcon {
		flags |= protocol.MobEffectFlagShowIcon
	}

	if instance.Effect == game.MobEffectNausea || instance.Effect == game.MobEffectDarkness {
		flags |= protocol.MobEffectFlagBlend
	}

	return flags
}
