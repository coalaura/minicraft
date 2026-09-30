package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	naturalMonster = iota
	naturalCreature
	naturalAmbient
	naturalWaterAmbient
	naturalCategoryCount
	naturalSpawnChunkArea           = 289
	naturalLightPreparationsPerTick = 2
)

type naturalSpawnEntry struct {
	Type                     game.EntityType
	Weight, Minimum, Maximum int
	Charge, Budget           float64
}

type naturalSpawnCategory struct {
	Cap      int
	Distance float64
	Interval int64
}

type naturalSpawnPlayer struct {
	Position game.Position
	Chunk    LoadedChunk
	Counts   [naturalCategoryCount]int
}

type naturalSpawnCharge struct {
	Position game.Position
	Charge   float64
}

type naturalSpawnState struct {
	Players           []naturalSpawnPlayer
	Chunks            []LoadedChunk
	Area              map[LoadedChunk]struct{}
	Eligible          map[LoadedChunk]struct{}
	Counts            [naturalCategoryCount]int
	Entities          []RuntimeEntity
	Charges           []naturalSpawnCharge
	Queries           map[LoadedChunk]*naturalSpawnChunk
	InTick            bool
	LightPreparations int
}

var naturalCategories = [naturalCategoryCount]naturalSpawnCategory{
	{Cap: 70, Distance: 128, Interval: 1},
	{Cap: 10, Distance: 128, Interval: 400},
	{Cap: 15, Distance: 128, Interval: 1},
	{Cap: 20, Distance: 64, Interval: 1},
}

var naturalSpawnCostTypes = buildNaturalSpawnCostTypes()

// Called under the ordinary world mutation/lifecycle locks, after despawning.
func (r *Runtime) tickNaturalSpawning(time game.TimeState) {
	r.prepareNaturalSpawnState()

	state := &r.naturalSpawning
	if len(state.Chunks) == 0 {
		return
	}

	var enabled [naturalCategoryCount]bool

	for category, rules := range naturalCategories {
		enabled[category] = time.Age%rules.Interval == 0 && state.Counts[category] < rules.Cap*len(state.Area)/naturalSpawnChunkArea
	}

	enabled[naturalMonster] = enabled[naturalMonster] && r.Difficulty != game.DifficultyPeaceful

	state.InTick = true
	state.LightPreparations = naturalLightPreparationsPerTick

	defer func() {
		state.InTick = false
	}()

	// Vanilla shuffles chunks and filters global caps once before the chunk loop.
	for index := len(state.Chunks) - 1; index > 0; index-- {
		other := runtimeMobRandomInt(r, index+1)

		state.Chunks[index], state.Chunks[other] = state.Chunks[other], state.Chunks[index]
	}

	for _, chunk := range state.Chunks {
		for category := range naturalCategoryCount {
			if enabled[category] && state.localCapAllows(chunk, category) {
				r.spawnNaturalPack(chunk, category)
			}
		}
	}
}

func (r *Runtime) prepareNaturalSpawnState() {
	state := &r.naturalSpawning

	state.Players = state.Players[:0]
	state.Chunks = state.Chunks[:0]
	state.Charges = state.Charges[:0]
	state.Counts = [naturalCategoryCount]int{}

	if state.Area == nil {
		state.Area = make(map[LoadedChunk]struct{}, naturalSpawnChunkArea)
		state.Eligible = make(map[LoadedChunk]struct{}, naturalSpawnChunkArea)
		state.Queries = make(map[LoadedChunk]*naturalSpawnChunk, naturalSpawnChunkArea)
	}

	clear(state.Area)
	clear(state.Eligible)

	for _, session := range r.sessionView() {
		player := session.playerView()
		if player.GameMode == game.GameModeSpectator {
			continue
		}

		chunk := LoadedChunk{X: blockChunkCoordinate(int32(math.Floor(player.Position.X))), Z: blockChunkCoordinate(int32(math.Floor(player.Position.Z)))}

		state.Players = append(state.Players, naturalSpawnPlayer{Position: player.Position, Chunk: chunk})

		for offsetZ := int32(-8); offsetZ <= 8; offsetZ++ {
			for offsetX := int32(-8); offsetX <= 8; offsetX++ {
				state.Area[LoadedChunk{X: chunk.X + offsetX, Z: chunk.Z + offsetZ}] = struct{}{}
			}
		}
	}

	r.activeChunksMu.RLock()

	for _, active := range r.activeChunkList {
		chunk := active.Position
		_, inArea := state.Area[chunk]

		if inArea && state.nearPlayer(chunk) {
			state.Chunks = append(state.Chunks, chunk)
			state.Eligible[chunk] = struct{}{}
		}
	}

	r.activeChunksMu.RUnlock()

	for chunk := range state.Queries {
		if _, eligible := state.Eligible[chunk]; !eligible {
			delete(state.Queries, chunk)
		}
	}

	state.Entities = r.appendRuntimeEntities(state.Entities[:0])

	for _, entity := range state.Entities {
		mob, isMob := entity.(RuntimeMobEntity)
		if !isMob || mob.RuntimeMobRequiresCustomPersistence() {
			continue
		}

		entityState := mob.RuntimeEntityState()

		entityState.mu.RLock()
		entityType := entityState.Type
		position := entityState.Position
		chunk := entityState.Chunk
		excluded := entityState.Removed || mob.RuntimeMob().PersistenceRequired
		entityState.mu.RUnlock()

		category, supported := naturalEntityCategory(entityType)
		if excluded || !supported {
			continue
		}

		// Every ordinary runtime mob counts globally, including mobs outside the
		// spawn circle; local counts associate a mob with every nearby player.
		state.addCount(chunk, category)

		if !naturalSpawnCostTypes[entityType] {
			continue
		}

		biome := r.naturalBiomeAt(game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))})
		if !biome.Valid() {
			continue
		}

		for _, entry := range naturalBiomeSpawns[biome][category] {
			if entry.Type == entityType && entry.Charge != 0 {
				state.Charges = append(state.Charges, naturalSpawnCharge{Position: naturalChargePosition(position), Charge: entry.Charge})

				break
			}
		}
	}

	clear(state.Entities)

	for chunk := range state.Queries {
		if _, eligible := state.Eligible[chunk]; !eligible {
			delete(state.Queries, chunk)
		}
	}
}

func (r *Runtime) spawnNaturalPack(origin LoadedChunk, category int) {
	position := game.BlockPosition{X: origin.X*16 + int32(runtimeMobRandomInt(r, 16)), Z: origin.Z*16 + int32(runtimeMobRandomInt(r, 16))}

	position.Y = -64 + int32(runtimeMobRandomInt(r, int(r.naturalSurfaceHeight(position)+1+64)+1))

	if position.Y < -63 || r.World.BlockAt(position).IsRedstoneConductor() {
		return
	}

	total := 0

	for range 3 {
		candidate := position
		attempts := int(math.Ceil(float64(r.nextEntityRandom() * 4)))

		var (
			selected       naturalSpawnEntry
			schoolLeader   schoolingFish
			groupCount     int
			tropicalSchool bool
		)

		for attempt := 0; attempt < attempts; attempt++ {
			positiveX := runtimeMobRandomInt(r, 6)
			negativeX := runtimeMobRandomInt(r, 6)
			positiveZ := runtimeMobRandomInt(r, 6)
			negativeZ := runtimeMobRandomInt(r, 6)

			candidate.X += int32(positiveX - negativeX)
			candidate.Z += int32(positiveZ - negativeZ)

			spawnPosition := game.Position{X: float64(candidate.X) + 0.5, Y: float64(candidate.Y), Z: float64(candidate.Z) + 0.5}
			if !r.naturalDistanceAllows(candidate, spawnPosition, category) {
				continue
			}

			biome := r.naturalBiomeAt(candidate)
			if !biome.Valid() {
				continue
			}

			entries := naturalBiomeSpawns[biome][category]

			if selected.Weight == 0 {
				if category == naturalWaterAmbient && (biome == game.BiomeRiver || biome == game.BiomeFrozenRiver) && r.nextEntityRandom() < 0.98 {
					break
				}

				selected = r.selectNaturalEntry(entries)
				if selected.Weight == 0 {
					break
				}

				attempts = selected.Minimum + runtimeMobRandomInt(r, selected.Maximum-selected.Minimum+1)
			}

			if !naturalContainsEntry(entries, selected) || !r.naturalEnvironmentAllows(candidate, selected.Type) || !r.naturalSpawning.potentialAllows(spawnPosition, selected) {
				continue
			}

			entity, spawned := r.SpawnEntity(selected.Type, spawnPosition)
			if !spawned {
				continue
			}

			if fish, schooling := entity.(schoolingFish); schooling {
				if schoolLeader == nil {
					schoolLeader = fish
				} else {
					connectAquaticFollower(fish, schoolLeader)
				}
			}

			r.naturalSpawning.addCount(LoadedChunk{X: blockChunkCoordinate(candidate.X), Z: blockChunkCoordinate(candidate.Z)}, category)

			if selected.Charge != 0 {
				r.naturalSpawning.Charges = append(r.naturalSpawning.Charges, naturalSpawnCharge{Position: naturalChargePosition(spawnPosition), Charge: selected.Charge})
			}

			total++
			groupCount++
			clusterLimit := 4

			if category == naturalWaterAmbient {
				clusterLimit = 8
			}

			if selected.Type == game.EntitySalmon {
				clusterLimit = 5
			}

			if total >= clusterLimit {
				return
			}

			if selected.Type == game.EntityTropicalFish && !tropicalSchool {
				tropicalSchool = r.nextEntityRandom() < 0.9
				if !tropicalSchool {
					break
				}
			}

			if category != naturalWaterAmbient && groupCount >= 4 {
				break
			}
		}
	}
}

func (r *Runtime) naturalDistanceAllows(candidate game.BlockPosition, position game.Position, category int) bool {
	chunk := LoadedChunk{X: blockChunkCoordinate(candidate.X), Z: blockChunkCoordinate(candidate.Z)}
	if _, eligible := r.naturalSpawning.Eligible[chunk]; !eligible {
		return false
	}

	nearest := math.MaxFloat64

	for _, player := range r.naturalSpawning.Players {
		nearest = min(nearest, distanceSquared(position, player.Position))
	}

	if nearest <= 24*24 || (category != naturalCreature && nearest > naturalCategories[category].Distance*naturalCategories[category].Distance) {
		return false
	}

	spawn := r.World.Spawn

	spawn.X = math.Floor(spawn.X) + 0.5
	spawn.Y = math.Floor(spawn.Y) + 0.5
	spawn.Z = math.Floor(spawn.Z) + 0.5

	return distanceSquared(position, spawn) >= 24*24
}

func (r *Runtime) selectNaturalEntry(entries []naturalSpawnEntry) naturalSpawnEntry {
	weight := 0

	for _, entry := range entries {
		weight += entry.Weight
	}

	if weight == 0 {
		return naturalSpawnEntry{}
	}

	choice := runtimeMobRandomInt(r, weight)

	for _, entry := range entries {
		choice -= entry.Weight
		if choice < 0 {
			return entry
		}
	}

	return naturalSpawnEntry{}
}

func (state *naturalSpawnState) nearPlayer(chunk LoadedChunk) bool {
	for _, player := range state.Players {
		if naturalPlayerNearChunk(player.Position, chunk) {
			return true
		}
	}

	return false
}

func (state *naturalSpawnState) localCapAllows(chunk LoadedChunk, category int) bool {
	for _, player := range state.Players {
		if naturalPlayerNearChunk(player.Position, chunk) && player.Counts[category] < naturalCategories[category].Cap {
			return true
		}
	}

	return false
}

func (state *naturalSpawnState) addCount(chunk LoadedChunk, category int) {
	state.Counts[category]++

	for index := range state.Players {
		player := &state.Players[index]
		if naturalPlayerNearChunk(player.Position, chunk) {
			player.Counts[category]++
		}
	}
}

func (state *naturalSpawnState) potentialAllows(position game.Position, entry naturalSpawnEntry) bool {
	if entry.Charge == 0 {
		return true
	}

	potential := float64(0)
	position = naturalChargePosition(position)

	for _, charge := range state.Charges {
		distance := distanceSquared(position, charge.Position)
		if distance == 0 {
			return false
		}

		potential += charge.Charge / math.Sqrt(distance)
	}

	return potential*entry.Charge <= entry.Budget
}

func naturalPlayerNearChunk(position game.Position, chunk LoadedChunk) bool {
	deltaX := position.X - float64(chunk.X*16+8)
	deltaZ := position.Z - float64(chunk.Z*16+8)

	return deltaX*deltaX+deltaZ*deltaZ < 128*128
}

func naturalEntityCategory(entityType game.EntityType) (int, bool) {
	switch entityType {
	case game.EntityZombie, game.EntitySkeleton, game.EntityCreeper:
		return naturalMonster, true
	case game.EntityCow, game.EntitySheep, game.EntityChicken:
		return naturalCreature, true
	case game.EntityBat:
		return naturalAmbient, true
	case game.EntityCod, game.EntitySalmon, game.EntityTropicalFish, game.EntityPufferfish:
		return naturalWaterAmbient, true
	default:
		return 0, false
	}
}

func naturalContainsEntry(entries []naturalSpawnEntry, selected naturalSpawnEntry) bool {
	return slices.Contains(entries, selected)
}

func naturalChargePosition(position game.Position) game.Position {
	return game.Position{X: math.Floor(position.X), Y: math.Floor(position.Y), Z: math.Floor(position.Z)}
}

func buildNaturalSpawnCostTypes() [game.MaxEntityType + 1]bool {
	var result [game.MaxEntityType + 1]bool

	for _, categories := range naturalBiomeSpawns {
		for _, entries := range categories {
			for _, entry := range entries {
				if entry.Charge != 0 {
					result[entry.Type] = true
				}
			}
		}
	}

	return result
}
