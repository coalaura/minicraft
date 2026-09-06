package server

import "github.com/coalaura/minicraft/internal/game"

const (
	runtimeLivingFireDurationTicks     = 8 * 20
	runtimeLivingLavaFireDurationTicks = 15 * 20
	runtimeLivingBurningDamage         = 1
	runtimeLivingLavaDamage            = 4
)

func (runtime *Runtime) tickRuntimeLivingBaseEnvironment(entity RuntimeLivingEntity) {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.Lock()

	box := living.CollisionBox(state.Position)
	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0
	inLava := runtime.fluidContact(box, game.FluidTypeLava, false).Depth > 0
	wasBurning := living.RemainingFireTicks > 0
	burningDamageDue := wasBurning && living.RemainingFireTicks%20 == 0 && !inLava

	if living.RemainingFireTicks > 0 {
		living.RemainingFireTicks--
	}

	if inWater {
		living.RemainingFireTicks = 0
		living.FallDistance = 0
	}

	if inLava {
		living.FallDistance *= 0.5
	}

	if wasBurning != (living.RemainingFireTicks > 0) {
		state.metadataDirty = true
	}

	state.mu.Unlock()

	if burningDamageDue && !inWater {
		runtime.applyRuntimeLivingEnvironmentDamage(entity, game.Damage{Type: game.DamageOnFire, Amount: runtimeLivingBurningDamage})
	}
}

func (runtime *Runtime) tickRuntimeLivingBlockEnvironment(entity RuntimeLivingEntity) {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.Lock()

	box := living.CollisionBox(state.Position)
	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0
	inLava := runtime.fluidContact(box, game.FluidTypeLava, false).Depth > 0
	fireDamage := runtime.fireContactDamage(box)
	wasBurning := living.RemainingFireTicks > 0

	if !inWater && fireDamage > 0 {
		living.RemainingFireTicks = max(living.RemainingFireTicks, runtimeLivingFireDurationTicks)
	}

	if inLava {
		living.RemainingFireTicks = max(living.RemainingFireTicks, runtimeLivingLavaFireDurationTicks)
	}

	if wasBurning != (living.RemainingFireTicks > 0) {
		state.metadataDirty = true
	}

	state.mu.Unlock()

	if !inWater && fireDamage > 0 {
		runtime.applyRuntimeLivingEnvironmentDamage(entity, game.Damage{Type: game.DamageInFire, Amount: fireDamage})
	}

	if inLava && !runtimeLivingDead(entity) {
		applied := runtime.applyRuntimeLivingEnvironmentDamage(entity, game.Damage{Type: game.DamageLava, Amount: runtimeLivingLavaDamage})
		if applied {
			pitch := 2 + runtime.nextEntityRandom()*0.4
			runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityGenericBurn, 0.4, pitch)
		}
	}
}

func (runtime *Runtime) applyRuntimeLivingEnvironmentDamage(entity RuntimeLivingEntity, damage game.Damage) bool {
	update, applied := runtime.damageRuntimeLivingEntityLocked(entity, damage)
	if applied {
		runtime.sendRuntimeLivingDamageUpdate(update)
	}

	return applied
}

func runtimeLivingDead(entity RuntimeLivingEntity) bool {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.RLock()
	defer state.mu.RUnlock()

	return living.Dead
}
