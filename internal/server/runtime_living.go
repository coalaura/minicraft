package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	runtimeLivingDeathEvent        = 3
	runtimeLivingPoofEvent         = 60
	runtimeLivingSafeFallDistance  = 3
	runtimeLivingDamageMemoryTicks = 40
)

type RuntimeLivingState struct {
	game.LivingState

	Velocity                game.Velocity
	OnGround                bool
	FallDistance            float32
	MoveDistance            float32
	NextStepDistance        int32
	KnockbackResistance     float32
	Width                   float64
	Height                  float64
	Armor                   int32
	ArmorToughness          float32
	LastDamageType          game.DamageType
	LastDamageCauseEntityID int32
	LastDamageStamp         int64
	HasLastDamage           bool
	UsingItem               bool
	UsingOffhand            bool
	ItemUseTicks            int32
}

type RuntimeLivingEntity interface {
	RuntimeEntity
	RuntimeLivingState() *RuntimeLivingState
}

type runtimeLivingDeathHandler interface {
	RuntimeLivingDied(*Runtime)
}

type runtimeLivingSoundProvider interface {
	RuntimeLivingDamageSound(bool) (game.SoundEvent, float32, float32)
}

type runtimeEntitySoundSourceProvider interface {
	RuntimeEntitySoundSource() int32
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

func (state *RuntimeLivingState) EntityFlags() byte {
	if state.RemainingFireTicks > 0 {
		return protocol.EntityFlagOnFire
	}

	return 0
}

func (state *RuntimeLivingState) ItemUseFlags() byte {
	if !state.UsingItem {
		return 0
	}

	flags := byte(protocol.LivingFlagUsingItem)

	if state.UsingOffhand {
		flags |= protocol.LivingFlagUsingOffhand
	}

	return flags
}

func (state *RuntimeLivingState) StartUsingItem(offhand bool) {
	state.UsingItem = true
	state.UsingOffhand = offhand
	state.ItemUseTicks = 0
}

func (state *RuntimeLivingState) TickUsingItem() int32 {
	if state.UsingItem {
		state.ItemUseTicks++
	}

	return state.ItemUseTicks
}

func (state *RuntimeLivingState) StopUsingItem() {
	state.UsingItem = false
	state.UsingOffhand = false
	state.ItemUseTicks = 0
}

func (state *RuntimeLivingState) LastDamageTypeAt(gameTime int64) (game.DamageType, bool) {
	damageType, _, remembered := state.LastDamageAt(gameTime)

	return damageType, remembered
}

func (state *RuntimeLivingState) LastDamageAt(gameTime int64) (game.DamageType, int32, bool) {
	if state.HasLastDamage && gameTime-state.LastDamageStamp > runtimeLivingDamageMemoryTicks {
		state.LastDamageType = game.DamageGeneric
		state.LastDamageCauseEntityID = 0
		state.LastDamageStamp = 0
		state.HasLastDamage = false
	}

	return state.LastDamageType, state.LastDamageCauseEntityID, state.HasLastDamage
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

	living.LastDamageType = damage.Type
	living.LastDamageCauseEntityID = damage.CauseEntityID
	living.LastDamageStamp = r.World.Time().Age
	living.HasLastDamage = true

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

		sounds, hasSounds := update.entity.(runtimeLivingSoundProvider)
		if hasSounds {
			event, volume, pitch := sounds.RuntimeLivingDamageSound(update.died)
			r.broadcastRuntimeEntitySound(update.entity, event, volume, pitch)
		}
	}

	r.synchronizeRuntimeEntity(update.entity)

	if update.died {
		r.broadcastRuntimeLivingEvent(entityID, runtimeLivingDeathEvent)
	}
}

func (r *Runtime) broadcastRuntimeEntitySound(entity RuntimeEntity, event game.SoundEvent, volume, pitch float32) {
	source := int32(protocol.SoundSourceNeutral)

	provider, provided := entity.(runtimeEntitySoundSourceProvider)

	if provided {
		source = provider.RuntimeEntitySoundSource()
	}

	r.broadcastRuntimeEntitySoundFromSource(entity, event, source, volume, pitch)
}

func (r *Runtime) broadcastRuntimeEntitySoundFromSource(entity RuntimeEntity, event game.SoundEvent, source int32, volume, pitch float32) {
	state := entity.RuntimeEntityState()

	state.mu.RLock()
	entityID := state.ID
	position := state.Position
	state.mu.RUnlock()

	sound := protocol.Sound{
		Event:  protocol.SoundEventHolder{Name: string(event)},
		Source: source,
		X:      position.X,
		Y:      position.Y,
		Z:      position.Z,
		Volume: volume,
		Pitch:  pitch,
		Seed:   int64(entityID),
	}

	r.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{ID: protocol.ClientboundSoundID, Encoder: sound})
}

func (r *Runtime) broadcastRuntimeLivingEvent(entityID int32, event byte) {
	packet := protocol.EntityEvent{EntityID: entityID, Event: event}

	r.broadcastRuntimeEntityPacket(entityID, runtimeEntityPacket{ID: protocol.ClientboundEntityEventID, Encoder: packet})
}

func calculateRuntimeLivingFallDamage(fallDistance float32) float32 {
	damage := float32(math.Floor(float64(fallDistance + 1e-6 - runtimeLivingSafeFallDistance)))
	if damage <= 0 {
		return 0
	}

	return damage
}
