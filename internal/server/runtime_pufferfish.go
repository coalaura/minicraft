package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	pufferfishSmallState          = int32(0)
	pufferfishMidState            = int32(1)
	pufferfishFullState           = int32(2)
	pufferfishInflateTicks        = int32(40)
	pufferfishMidDeflateTicks     = int32(60)
	pufferfishFullDeflateTicks    = int32(100)
	pufferfishThreatExpansion     = 2
	pufferfishMobContactExpansion = 0.3
	pufferfishPoisonTicksPerState = int32(60)
	pufferfishStingGameEvent      = byte(9)
	pufferfishSmallDimensionScale = 0.5
	pufferfishMidDimensionScale   = 0.7
	pufferfishFullDimensionScale  = float64(1)
	pufferfishBaseEyeHeight       = 0.455
)

type runtimePufferfishEntity struct {
	runtimeAquatic
	PuffState      int32
	InflateCounter int32
	DeflateTimer   int32
}

type pufferfishInflateGoal struct {
	Fish *runtimePufferfishEntity
}

var pufferfishAquaticSpec = runtimeAquaticSpec{
	EntityType: game.EntityPufferfish,
	HurtSound:  game.SoundEntityPufferfishHurt,
	DeathSound: game.SoundEntityPufferfishDeath,
	FlopSound:  game.SoundEntityPufferfishFlop,
	RawDrop:    game.ItemPufferfish,
}

func (entity *runtimePufferfishEntity) Tick(runtime *Runtime, _ *ActiveChunk) {
	runtime.tickAquatic(entity, &entity.runtimeAquatic)
}

func (entity *runtimePufferfishEntity) EntityMetadata() []protocol.EntityMetadataEntry {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	metadata := entity.entityMetadataLocked()

	return append(metadata, protocol.EntityMetadataEntry{Index: protocol.PufferfishStateMetadataIndex, Type: protocol.MetadataTypeInt, Value: protocol.MetadataVarInt(entity.PuffState)})
}

func (entity *runtimePufferfishEntity) RuntimeLivingEyeHeight() float64 {
	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return entity.EyeHeight
}

func (entity *runtimePufferfishEntity) aquaticPreTick(runtime *Runtime) {
	entity.tickPuffState(runtime)
}

func (entity *runtimePufferfishEntity) aquaticPostMovementTick(runtime *Runtime) {
	entity.stingNearbyMobs(runtime)
	entity.stingTouchingPlayers(runtime)
}

func (entity *runtimePufferfishEntity) tickPuffState(runtime *Runtime) {
	entity.State.mu.Lock()

	state := entity.PuffState

	if entity.InflateCounter > 0 {
		if state == pufferfishSmallState {
			entity.setPuffStateLocked(pufferfishMidState)
		} else if entity.InflateCounter > pufferfishInflateTicks && state == pufferfishMidState {
			entity.setPuffStateLocked(pufferfishFullState)
		}

		entity.InflateCounter++
	} else if state != pufferfishSmallState {
		if entity.DeflateTimer > pufferfishMidDeflateTicks && state == pufferfishFullState {
			entity.setPuffStateLocked(pufferfishMidState)
		} else if entity.DeflateTimer > pufferfishFullDeflateTicks && state == pufferfishMidState {
			entity.setPuffStateLocked(pufferfishSmallState)
		}

		entity.DeflateTimer++
	}

	changed := state != entity.PuffState
	newState := entity.PuffState
	entity.State.mu.Unlock()

	if !changed {
		return
	}

	sound := game.SoundEntityPufferfishBlowUp

	if newState < state {
		sound = game.SoundEntityPufferfishBlowOut
	}

	runtime.broadcastRuntimeEntitySound(entity, sound, 1, 1)
}

func (entity *runtimePufferfishEntity) setPuffStateLocked(state int32) {
	entity.PuffState = state
	entity.State.metadataDirty = true

	scale := pufferfishFullDimensionScale

	switch state {
	case pufferfishSmallState:
		scale = pufferfishSmallDimensionScale
	case pufferfishMidState:
		scale = pufferfishMidDimensionScale
	}

	definition, _ := game.EntityPufferfish.Definition()

	entity.Living.Width = definition.Width * scale
	entity.Living.Height = definition.Height * scale
	entity.EyeHeight = pufferfishBaseEyeHeight * scale
}

func (goal *pufferfishInflateGoal) CanUse(runtime *Runtime) bool {
	return goal.Fish.hasNearbyThreat(runtime)
}

func (goal *pufferfishInflateGoal) CanContinue(runtime *Runtime) bool {
	return goal.Fish.hasNearbyThreat(runtime)
}

func (goal *pufferfishInflateGoal) Start(*Runtime) {
	goal.Fish.State.mu.Lock()
	goal.Fish.InflateCounter = 1
	goal.Fish.DeflateTimer = 0
	goal.Fish.State.mu.Unlock()
}

func (goal *pufferfishInflateGoal) Stop(*Runtime) {
	goal.Fish.State.mu.Lock()
	goal.Fish.InflateCounter = 0
	goal.Fish.State.mu.Unlock()
}

func (*pufferfishInflateGoal) Tick(*Runtime) {}

func (entity *runtimePufferfishEntity) hasNearbyThreat(runtime *Runtime) bool {
	entity.State.mu.RLock()
	box := expandAABB(entity.Living.CollisionBox(entity.State.Position), pufferfishThreatExpansion)
	entity.State.mu.RUnlock()

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if player.Dead || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator {
			continue
		}

		if box.Intersects(player.collisionBox()) {
			return true
		}
	}

	iterator := runtime.runtimeLivingEntitiesInBox(box)

	for {
		candidate, found := iterator.Next()
		if !found {
			break
		}

		if candidate == entity || !pufferfishScaryRuntimeEntity(candidate) {
			continue
		}

		return true
	}

	return false
}

func (entity *runtimePufferfishEntity) stingNearbyMobs(runtime *Runtime) {
	entity.State.mu.RLock()
	state := entity.PuffState

	box := expandAABB(entity.Living.CollisionBox(entity.State.Position), pufferfishMobContactExpansion)

	entityID := entity.State.ID
	entity.State.mu.RUnlock()

	if state == pufferfishSmallState {
		return
	}

	iterator := runtime.runtimeLivingEntitiesInBox(box)

	for {
		living, found := iterator.Next()
		if !found {
			break
		}

		candidate := RuntimeEntity(living)
		if candidate == entity || !pufferfishScaryRuntimeEntity(candidate) {
			continue
		}

		_, mobEntity := candidate.(RuntimeMobEntity)
		if !mobEntity {
			continue
		}

		damage := game.Damage{Type: game.DamageMobAttack, Amount: float32(1 + state), CauseEntityID: entityID, DirectEntityID: entityID}

		update, applied := runtime.damageRuntimeLivingEntityLocked(living, damage)
		if !applied {
			continue
		}

		poison := game.NewMobEffectInstance(game.MobEffectPoison, pufferfishPoisonTicksPerState*state, 0, false, true, true)

		runtime.addRuntimeLivingMobEffect(living, poison)
		runtime.sendRuntimeLivingDamageUpdate(update)
		runtime.broadcastRuntimeEntitySound(entity, game.SoundEntityPufferfishSting, 1, 1)
	}
}

func (entity *runtimePufferfishEntity) stingTouchingPlayers(runtime *Runtime) {
	entity.State.mu.RLock()
	state := entity.PuffState

	box := entity.Living.CollisionBox(entity.State.Position)

	entityID := entity.State.ID
	entity.State.mu.RUnlock()

	if state == pufferfishSmallState {
		return
	}

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if !box.Intersects(player.collisionBox()) {
			continue
		}

		damageAmount := pufferfishPlayerDamageForDifficulty(runtime.Difficulty, float32(1+state))
		damage := game.Damage{Type: game.DamageMobAttack, Amount: damageAmount, CauseEntityID: entityID, DirectEntityID: entityID}

		update, applied := runtime.damagePlayerLocked(session, damage)
		if !applied {
			continue
		}

		poison := game.NewMobEffectInstance(game.MobEffectPoison, pufferfishPoisonTicksPerState*state, 0, false, true, true)

		updatedPlayer, changed := session.updatePlayerState(func(player *game.Player) bool {
			change, added, absorptionChanged := addPlayerMobEffect(player, poison)
			if added {
				update.effectChanges = append(update.effectChanges, change)
			}

			update.metadataChanged = update.metadataChanged || absorptionChanged

			return added || absorptionChanged
		})

		if changed {
			update.player = updatedPlayer
		}

		runtime.sendPlayerSurvivalUpdate(session, update)
		_ = session.sendGameEvent(pufferfishStingGameEvent, 0)
	}
}

func (runtime *Runtime) SpawnPufferfish(position game.Position) *runtimePufferfishEntity {
	entity := &runtimePufferfishEntity{}
	if !runtime.initializeAquatic(&entity.runtimeAquatic, &pufferfishAquaticSpec) {
		return nil
	}

	entity.State.mu.Lock()
	entity.setPuffStateLocked(pufferfishSmallState)

	entity.State.metadataDirty = false
	entity.State.mu.Unlock()

	runtime.configureAquaticGoals(entity, &entity.runtimeAquatic)

	entity.Goals.Add(1, 0, &pufferfishInflateGoal{Fish: entity})

	runtime.registerRuntimeEntity(entity, game.EntityPufferfish, position)

	return entity
}

func pufferfishScaryRuntimeEntity(entity RuntimeEntity) bool {
	state := entity.RuntimeEntityState()

	state.mu.RLock()
	entityType := state.Type
	state.mu.RUnlock()

	return !entityType.NotScaryForPufferfish()
}

func pufferfishPlayerDamageForDifficulty(difficulty game.Difficulty, damage float32) float32 {
	switch difficulty {
	case game.DifficultyPeaceful:
		return 0
	case game.DifficultyEasy:
		return min(damage/2+1, damage)
	case game.DifficultyHard:
		return damage * 1.5
	default:
		return damage
	}
}

func expandAABB(box game.AABB, amount float64) game.AABB {
	box.MinX -= amount
	box.MinY -= amount
	box.MinZ -= amount
	box.MaxX += amount
	box.MaxY += amount
	box.MaxZ += amount

	return box
}
