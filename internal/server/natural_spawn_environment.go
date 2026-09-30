package server

import (
	"math"
	"slices"
	"strings"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type naturalSpawnChunk struct {
	Prepared    preparedChunkGeneration
	Revision    uint64
	Heights     [256]int32
	HeightKnown [256]bool
	Sky, Block  []byte
}

type naturalSpawnBlock struct {
	Support, Full, Empty, Animals, Bats bool
}

var naturalSpawnBlocks = buildNaturalSpawnBlocks()

func (r *Runtime) naturalChunkAt(position game.BlockPosition) *naturalSpawnChunk {
	state := &r.naturalSpawning
	if state.Queries == nil {
		state.Queries = make(map[LoadedChunk]*naturalSpawnChunk, naturalSpawnChunkArea)
	}

	chunk := LoadedChunk{X: blockChunkCoordinate(position.X), Z: blockChunkCoordinate(position.Z)}

	query := state.Queries[chunk]
	if query == nil {
		query = &naturalSpawnChunk{Prepared: prepareChunkGeneration(r.World, game.ChunkPosition{X: chunk.X, Z: chunk.Z}), Revision: r.World.BlockRevision()}

		state.Queries[chunk] = query
	}

	if query.Revision != r.World.BlockRevision() {
		query.Revision = r.World.BlockRevision()

		clear(query.HeightKnown[:])

		query.Sky = query.Sky[:0]
		query.Block = query.Block[:0]
	}

	return query
}

func (r *Runtime) naturalBiomeAt(position game.BlockPosition) game.Biome {
	query := r.naturalChunkAt(position)

	biome, supplied := query.Prepared.BiomeAt((position.X>>2)*4+2, (position.Y>>2)*4+2, (position.Z>>2)*4+2)
	if !supplied {
		return game.BiomePlains
	}

	return biome
}

func (r *Runtime) naturalSurfaceHeight(position game.BlockPosition) int32 {
	query := r.naturalChunkAt(position)
	column := int(position.Z&15)*16 + int(position.X&15)

	if query.HeightKnown[column] {
		return query.Heights[column]
	}

	height := int32(-64)

	for worldY := int32(319); worldY >= -64; worldY-- {
		position.Y = worldY

		block := r.World.BlockAt(position)
		if block != game.Air && block != game.CaveAir && block != game.VoidAir {
			height = worldY + 1

			break
		}
	}

	query.Heights[column] = height
	query.HeightKnown[column] = true

	return height
}

func (r *Runtime) naturalLightAt(position game.BlockPosition) (byte, byte, bool) {
	if r.World.Lighting == game.LightingFullbright {
		return 15, 15, true
	}

	query := r.naturalChunkAt(position)
	if len(query.Sky) == 0 {
		state := &r.naturalSpawning
		if state.InTick {
			if state.LightPreparations == 0 {
				return 0, 0, false
			}

			state.LightPreparations--
		}

		buffer := acquireLightingBuffer()

		characteristics, err := generateLightingBlocks(r.World, query.Prepared.position.X, query.Prepared.position.Z, buffer, nil)
		if err != nil {
			releaseLightingBuffer(buffer)

			return 0, 0, false
		}

		calculateLight(buffer, characteristics)

		volume := 16 * 16 * lightingHeight
		if cap(query.Sky) < volume {
			query.Sky = make([]byte, volume)
			query.Block = make([]byte, volume)
		} else {
			query.Sky = query.Sky[:volume]
			query.Block = query.Block[:volume]
		}

		for worldY := range lightingHeight {
			for localZ := range 16 {
				for localX := range 16 {
					destination := chunkBlockIndex(localX, worldY, localZ)

					source := lightingIndex(localX+lightingHalo, worldY, localZ+lightingHalo)

					query.Sky[destination] = buffer.sky[source]
					query.Block[destination] = buffer.block[source]
				}
			}
		}

		releaseLightingBuffer(buffer)
	}

	index := chunkBlockIndex(int(position.X&15), int(position.Y+64), int(position.Z&15))
	return query.Sky[index], query.Block[index], true
}

func (r *Runtime) naturalEnvironmentAllows(position game.BlockPosition, entityType game.EntityType) bool {
	category, supported := naturalEntityCategory(entityType)
	if !supported || position.Y < -64 || position.Y >= 320 {
		return false
	}

	if category == naturalMonster && r.Difficulty == game.DifficultyPeaceful {
		return false
	}

	below := position
	below.Y--

	above := position
	above.Y++

	feet := r.World.BlockAt(position)
	support := r.World.BlockAt(below)
	head := r.World.BlockAt(above)

	if category == naturalWaterAmbient {
		if feet.FluidState().Type() != game.FluidTypeWater || support.FluidState().Type() != game.FluidTypeWater || head.IsRedstoneConductor() {
			return false
		}

		headDefinition, valid := head.Definition()
		if !valid || headDefinition.Name != "water" {
			return false
		}

		anyHeight := entityType == game.EntityTropicalFish && naturalTropicalAnyHeightBiomes[r.naturalBiomeAt(position)]
		if !anyHeight && (position.Y < r.World.SeaLevel-13 || position.Y > r.World.SeaLevel) {
			return false
		}
	} else {
		if !naturalSpawnBlocks[support].Support || !naturalSpawnBlocks[feet].Empty || !naturalSpawnBlocks[head].Empty {
			return false
		}

		if category == naturalCreature && !naturalSpawnBlocks[support].Animals {
			return false
		}

		if category == naturalAmbient && (position.Y >= r.naturalSurfaceHeight(position) || !naturalSpawnBlocks[support].Bats || runtimeMobRandomInt(r, 2) != 0) {
			return false
		}
	}

	definition, valid := entityType.Definition()
	if !valid {
		return false
	}

	spawnPosition := game.Position{X: float64(position.X) + 0.5, Y: float64(position.Y), Z: float64(position.Z) + 0.5}

	box := entityBox(spawnPosition, definition.Width, definition.Height)
	if box.MaxY > 320 || !r.naturalBoxClear(box, category == naturalWaterAmbient) || r.entityIntersectionObstructed(box) {
		return false
	}

	if category == naturalWaterAmbient {
		return true
	}

	sky, block, prepared := r.naturalLightAt(position)
	if !prepared {
		return false
	}

	switch category {
	case naturalMonster:
		if int(sky) > runtimeMobRandomInt(r, 32) || block > 0 {
			return false
		}

		return r.naturalRawBrightness(sky, block) <= runtimeMobRandomInt(r, 8)
	case naturalCreature:
		return max(sky, block) > 8
	case naturalAmbient:
		return r.naturalRawBrightness(sky, block) <= runtimeMobRandomInt(r, 4)
	default:
		return true
	}
}

func (r *Runtime) naturalRawBrightness(sky, block byte) int {
	dayTime := r.World.Time().DayTime
	darkening := int(15 - zombieSkyLightLevel(dayTime))

	return max(int(block), int(sky)-darkening)
}

func (r *Runtime) naturalBoxClear(box game.AABB, aquatic bool) bool {
	var storage [32]game.AABB

	for worldY := int32(math.Floor(box.MinY)); float64(worldY) < box.MaxY; worldY++ {
		for worldZ := int32(math.Floor(box.MinZ)); float64(worldZ) < box.MaxZ; worldZ++ {
			for worldX := int32(math.Floor(box.MinX)); float64(worldX) < box.MaxX; worldX++ {
				position := game.BlockPosition{X: worldX, Y: worldY, Z: worldZ}

				block := r.World.BlockAt(position)
				if !aquatic && !block.FluidState().Empty() {
					return false
				}

				boxes := block.AppendCollisionBoxes(storage[:0], position)
				if slices.ContainsFunc(boxes, box.Intersects) {
					return false
				}
			}
		}
	}

	return true
}

func buildNaturalSpawnBlocks() [game.MaxBlockState + 1]naturalSpawnBlock {
	var (
		result    [game.MaxBlockState + 1]naturalSpawnBlock
		animals   [game.MaxBlockID + 1]bool
		bats      [game.MaxBlockID + 1]bool
		prevented [game.MaxBlockID + 1]bool
		noSupport [game.MaxBlockID + 1]bool
		signals   [game.MaxBlockID + 1]bool
		fire      [game.MaxBlockID + 1]bool
		campfires [game.MaxBlockID + 1]bool
	)

	for _, registry := range protocol.ConfigurationTags {
		if registry.RegistryID != "minecraft:block" {
			continue
		}

		for _, tag := range registry.Tags {
			for _, identifier := range tag.Entries {
				switch tag.ID {
				case "minecraft:animals_spawnable_on":
					animals[identifier] = true
				case "minecraft:bats_spawnable_on":
					bats[identifier] = true
				case "minecraft:prevent_mob_spawning_inside":
					prevented[identifier] = true
				case "minecraft:leaves", "minecraft:trapdoors":
					noSupport[identifier] = true
				case "minecraft:buttons", "minecraft:pressure_plates", "minecraft:lightning_rods":
					signals[identifier] = true
				case "minecraft:fire":
					fire[identifier] = true
				case "minecraft:campfires":
					campfires[identifier] = true
				}
			}
		}
	}

	var storage [32]game.AABB

	for block := game.Block(0); block <= game.MaxBlockState; block++ {
		definition, _ := block.Definition()
		name := definition.Name

		boxes := block.AppendCollisionBoxes(storage[:0], game.BlockPosition{})
		full := len(boxes) == 1 && boxes[0] == (game.AABB{MaxX: 1, MaxY: 1, MaxZ: 1})

		emission, _ := block.LightProperties()
		support := block.FaceSturdy(game.BlockFaceUp) && emission < 14

		switch name {
		case "soul_sand", "mud", "carved_pumpkin", "jack_o_lantern", "redstone_lamp":
			support = true
		case "bedrock", "glass", "tinted_glass", "ice", "frosted_ice", "magma_block", "barrier", "scaffolding":
			support = false
		}

		if noSupport[definition.ID] || strings.HasSuffix(name, "_stained_glass") || strings.HasSuffix(name, "copper_grate") {
			support = false
		}

		dangerous := fire[definition.ID] || name == "lava" || name == "lava_cauldron" || name == "magma_block" || name == "wither_rose" || name == "sweet_berry_bush" || name == "cactus" || name == "powder_snow"

		if campfires[definition.ID] {
			lit, _ := block.Property("lit")
			dangerous = lit == "true"
		}

		empty := !full && !signals[definition.ID] && !naturalSignalSource(name) && block.FluidState().Empty() && !prevented[definition.ID] && !dangerous

		result[block] = naturalSpawnBlock{Support: support, Full: full, Empty: empty, Animals: animals[definition.ID], Bats: bats[definition.ID]}
	}

	return result
}

func naturalSignalSource(name string) bool {
	switch name {
	case "daylight_detector", "detector_rail", "repeater", "comparator", "jukebox", "lectern", "lever", "observer", "redstone_block", "redstone_torch", "redstone_wall_torch", "redstone_wire", "sculk_sensor", "calibrated_sculk_sensor", "target", "trapped_chest", "tripwire_hook":
		return true
	default:
		return false
	}
}
