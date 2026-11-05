package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func (r *Runtime) setArrowTargetFire(target arrowTarget, ticks int32, extend bool) int32 {
	previous := int32(0)

	if target.session != nil {
		target.session.mutatePlayer(func(player *game.Player) bool {
			previous = player.RemainingFireTicks

			if player.Dead || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator {
				return false
			}

			if extend {
				ticks = max(ticks, previous)
			}

			player.RemainingFireTicks = ticks

			return true
		})

		return previous
	}

	living, valid := target.entity.(RuntimeLivingEntity)
	if !valid {
		return previous
	}

	state := living.RuntimeEntityState()

	state.mu.Lock()
	defer state.mu.Unlock()

	health := living.RuntimeLivingState()
	previous = health.RemainingFireTicks

	if extend {
		ticks = max(ticks, previous)
	}

	health.RemainingFireTicks = ticks
	state.metadataDirty = true

	return previous
}

func (r *Runtime) pushArrowTarget(target arrowTarget, velocity game.Velocity, punch int32) {
	if target.session != nil {
		player, _ := target.session.updatePlayerState(func(player *game.Player) bool {
			arrowPunch(&player.Velocity, velocity, punch, player.ArmorAttributes().KnockbackResistance)

			return true
		})

		r.sendPlayerKnockback(player)

		return
	}

	living, valid := target.entity.(RuntimeLivingEntity)
	if !valid {
		return
	}

	state := living.RuntimeEntityState()

	state.mu.Lock()
	health := living.RuntimeLivingState()
	arrowPunch(&health.Velocity, velocity, punch, health.KnockbackResistance)
	state.movementSyncDirty = true
	state.mu.Unlock()

	r.synchronizeRuntimeEntity(living)
}

func (r *Runtime) pickUpArrow(arrow *runtimeArrowEntity) bool {
	arrow.State.mu.Lock()

	if arrow.State.Removed || !arrow.InGround || arrow.ShakeTime > 0 || arrow.Pickup == arrowPickupDisallowed {
		arrow.State.mu.Unlock()

		return false
	}

	box := arrowBox(arrow.State.Position)
	pickupBox := game.AABB{MinX: box.MinX - 1, MinY: box.MinY - 0.5, MinZ: box.MinZ - 1, MaxX: box.MaxX + 1, MaxY: box.MaxY + 0.5, MaxZ: box.MaxZ + 1}

	for _, session := range r.sessionView() {
		player := session.playerView()
		if player.Dead || player.GameMode == game.GameModeSpectator || !pickupBox.Intersects(player.collisionBox()) {
			continue
		}

		var mutation playerInventoryMutation

		if arrow.Pickup == arrowPickupCreativeOnly {
			if player.GameMode != game.GameModeCreative {
				continue
			}
		} else {
			_, changed := session.insertPickedUpItem(&mutation, arrow.PickupStack)
			if !changed {
				continue
			}
		}

		entityID := arrow.State.ID
		arrow.Pickup = arrowPickupDisallowed
		arrow.State.mu.Unlock()

		for _, viewer := range r.sessionView() {
			if !viewer.tracksRuntimeEntity(entityID) {
				continue
			}

			err := writeSessionPacket(viewer, protocol.ClientboundTakeItemEntityID, protocol.TakeItemEntity{ItemEntityID: entityID, PlayerEntityID: player.EntityID, Amount: 1})
			if err != nil && viewer.Log != nil {
				viewer.Log.Warnf("[play] failed to animate arrow pickup: %v\n", err)
			}
		}

		err := session.synchronizePlayerInventoryJournal(&mutation)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize arrow pickup: %v\n", err)
		}

		r.removeRuntimeEntity(entityID)

		return true
	}

	arrow.State.mu.Unlock()

	return false
}

func arrowPunch(target *game.Velocity, arrow game.Velocity, level int32, resistance float32) {
	length := math.Hypot(arrow.X, arrow.Z)
	strength := float64(level) * 0.6 * math.Max(0, 1-float64(resistance))

	if length < 1e-5 || strength <= 0 {
		return
	}

	target.X += arrow.X / length * strength
	target.Y += 0.1
	target.Z += arrow.Z / length * strength
}
