package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	runtimeLivingDeathEvent = 3
	runtimeLivingPoofEvent  = 60
)

type RuntimeLivingState struct {
	game.LivingState

	Velocity            game.Velocity
	OnGround            bool
	KnockbackResistance float32
	Width               float64
	Height              float64
	Armor               int32
	ArmorToughness      float32
}

type RuntimeLivingEntity interface {
	RuntimeEntity
	RuntimeLivingState() *RuntimeLivingState
}

type runtimeLivingDeathHandler interface {
	RuntimeLivingDied(*Runtime)
}

type runtimeLivingDamageUpdate struct {
	entity   RuntimeLivingEntity
	damage   game.Damage
	fullHurt bool
	died     bool
}

func (state *RuntimeLivingState) CollisionBox(position game.Position) game.AABB {
	halfWidth := state.Width / 2

	return game.AABB{
		MinX: position.X - halfWidth,
		MinY: position.Y,
		MinZ: position.Z - halfWidth,
		MaxX: position.X + halfWidth,
		MaxY: position.Y + state.Height,
		MaxZ: position.Z + halfWidth,
	}
}

func (r *Runtime) damageRuntimeLivingEntityLocked(entity RuntimeLivingEntity, damage game.Damage) (runtimeLivingDamageUpdate, bool) {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.Lock()

	if state.Removed {
		state.mu.Unlock()

		return runtimeLivingDamageUpdate{}, false
	}

	defense := func() game.LivingDefense {
		return game.LivingDefense{Armor: living.Armor, Toughness: living.ArmorToughness}
	}

	result := game.ResolveLivingDamage(&living.LivingState, damage, defense, nil)
	if !result.Applied {
		state.mu.Unlock()

		return runtimeLivingDamageUpdate{}, false
	}

	state.metadataDirty = true

	update := runtimeLivingDamageUpdate{
		entity:   entity,
		damage:   damage,
		fullHurt: result.FullHurt,
		died:     result.Died,
	}

	state.mu.Unlock()

	deathHandler, handlesDeath := entity.(runtimeLivingDeathHandler)
	if result.Died && handlesDeath {
		deathHandler.RuntimeLivingDied(r)
	}

	return update, true
}

func (r *Runtime) applyRuntimeLivingKnockback(entity RuntimeLivingEntity, directionX, directionZ, strength float64) bool {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Removed {
		return false
	}

	applied := game.ApplyLivingKnockback(&living.Velocity, living.OnGround, living.KnockbackResistance, directionX, directionZ, strength, r.nextEntityRandom)
	if applied {
		state.movementSyncDirty = true
	}

	return applied
}

func (r *Runtime) tickRuntimeLivingEntity(entity RuntimeLivingEntity) {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.Lock()

	if state.Removed {
		state.mu.Unlock()

		return
	}

	living.TickHurtCooldown()

	if living.Dead {
		remove := living.TickDeath()
		entityID := state.ID

		if remove {
			living.ActiveEffects.Clear()
		}

		state.mu.Unlock()

		if remove {
			r.broadcastRuntimeLivingEvent(entityID, runtimeLivingPoofEvent)
			r.removeRuntimeEntity(entityID)
		}

		return
	}

	state.mu.Unlock()
}

func (r *Runtime) sendRuntimeLivingDamageUpdate(update runtimeLivingDamageUpdate) {
	state := update.entity.RuntimeEntityState()

	state.mu.RLock()
	entityID := state.ID
	state.mu.RUnlock()

	if update.fullHurt {
		traits := update.damage.Type.Traits()

		packet := protocol.DamageEvent{
			EntityID:       entityID,
			DamageType:     traits.RegistryID,
			CauseEntityID:  protocolEntityID(update.damage.CauseEntityID),
			DirectEntityID: protocolEntityID(update.damage.DirectEntityID),
		}

		if update.damage.SourcePosition != nil {
			packet.HasSourcePosition = true
			packet.SourcePositionX = update.damage.SourcePosition.X
			packet.SourcePositionY = update.damage.SourcePosition.Y
			packet.SourcePositionZ = update.damage.SourcePosition.Z
		}

		r.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{ID: protocol.ClientboundDamageEventID, Encoder: packet})
	}

	r.synchronizeRuntimeEntity(update.entity)

	if update.died {
		r.broadcastRuntimeLivingEvent(entityID, runtimeLivingDeathEvent)
	}
}

func (r *Runtime) broadcastRuntimeLivingEvent(entityID int32, event byte) {
	packet := protocol.EntityEvent{EntityID: entityID, Event: event}

	r.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{ID: protocol.ClientboundEntityEventID, Encoder: packet})
}
