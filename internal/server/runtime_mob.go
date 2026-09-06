package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	runtimeMobNoDespawnDistance = 32
	runtimeMobDespawnDistance   = 128
	runtimeMobDespawnDelay      = 600
	runtimeMobDespawnChance     = 800
)

type RuntimeMobState struct {
	NoActionTime        int32
	PersistenceRequired bool
}

type RuntimeMobDespawnConfig struct {
	NoDespawnDistance float64
	DespawnDistance   float64
	AllowedInPeaceful bool
}

type RuntimeMobEntity interface {
	RuntimeLivingEntity
	RuntimeMob() *RuntimeMobState
	RuntimeMobDespawnConfig() RuntimeMobDespawnConfig
	RuntimeMobRemoveWhenFarAway(float64) bool
	RuntimeMobRequiresCustomPersistence() bool
}

func (r *Runtime) checkRuntimeMobDespawn(entity RuntimeMobEntity) bool {
	state := entity.RuntimeEntityState()
	living := entity.RuntimeLivingState()

	state.mu.RLock()
	removed := state.Removed
	dead := living.Dead
	position := state.Position
	state.mu.RUnlock()

	if removed {
		return true
	}

	configuration := entity.RuntimeMobDespawnConfig()
	if r.Difficulty == game.DifficultyPeaceful && !configuration.AllowedInPeaceful {
		r.removeRuntimeEntity(state.ID)

		return true
	}

	if dead {
		return false
	}

	mob := entity.RuntimeMob()
	customPersistence := entity.RuntimeMobRequiresCustomPersistence()

	state.mu.Lock()
	noActionTime := mob.NoActionTime

	persistent := mob.PersistenceRequired
	if persistent || customPersistence {
		mob.NoActionTime = 0
	}

	state.mu.Unlock()

	if persistent || customPersistence {
		return false
	}

	distanceSquared, playerPresent := r.nearestMobPlayerDistanceSquared(position)
	if !playerPresent {
		return false
	}

	despawned := false

	despawnDistanceSquared := configuration.DespawnDistance * configuration.DespawnDistance
	if distanceSquared > despawnDistanceSquared && entity.RuntimeMobRemoveWhenFarAway(distanceSquared) {
		r.removeRuntimeEntity(state.ID)

		return true
	}

	noDespawnDistanceSquared := configuration.NoDespawnDistance * configuration.NoDespawnDistance
	randomDespawn := noActionTime > runtimeMobDespawnDelay && runtimeMobRandomInt(r, runtimeMobDespawnChance) == 0

	if randomDespawn && distanceSquared > noDespawnDistanceSquared && entity.RuntimeMobRemoveWhenFarAway(distanceSquared) {
		r.removeRuntimeEntity(state.ID)

		despawned = true
	} else if distanceSquared < noDespawnDistanceSquared {
		state.mu.Lock()
		mob.NoActionTime = 0
		state.mu.Unlock()
	}

	return despawned
}

func (r *Runtime) cleanupInactiveRuntimeMobs() {
	for _, entity := range r.snapshotRuntimeEntities() {
		mob, isMob := entity.(RuntimeMobEntity)
		if !isMob {
			continue
		}

		state := mob.RuntimeEntityState()
		living := mob.RuntimeLivingState()

		state.mu.RLock()
		removed := state.Removed
		dead := living.Dead
		position := state.Position
		chunkPosition := state.Chunk
		persistent := mob.RuntimeMob().PersistenceRequired
		state.mu.RUnlock()

		if removed || dead || persistent || mob.RuntimeMobRequiresCustomPersistence() {
			continue
		}

		_, active := r.ActiveChunk(chunkPosition)
		if active {
			continue
		}

		distanceSquared, playerPresent := r.nearestMobPlayerDistanceSquared(position)
		if !playerPresent {
			continue
		}

		configuration := mob.RuntimeMobDespawnConfig()

		despawnDistanceSquared := configuration.DespawnDistance * configuration.DespawnDistance
		if distanceSquared > despawnDistanceSquared && mob.RuntimeMobRemoveWhenFarAway(distanceSquared) {
			r.removeRuntimeEntity(state.ID)
		}
	}
}

func (r *Runtime) nearestMobPlayerDistanceSquared(position game.Position) (float64, bool) {
	nearestDistanceSquared := math.MaxFloat64
	playerPresent := false

	for _, session := range r.snapshotSessions() {
		player := session.snapshotPlayer()
		if player.GameMode == game.GameModeSpectator {
			continue
		}

		candidateDistanceSquared := distanceSquared(position, player.Position)
		if candidateDistanceSquared < nearestDistanceSquared {
			nearestDistanceSquared = candidateDistanceSquared
			playerPresent = true
		}
	}

	return nearestDistanceSquared, playerPresent
}

func runtimeMobRandomInt(runtime *Runtime, bound int) int {
	value := int(runtime.nextEntityRandom() * float32(bound))

	return min(value, bound-1)
}
