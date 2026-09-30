package server

import "github.com/coalaura/minicraft/internal/game"

type naturalSpawnGroup struct {
	Leader          schoolingFish
	CommonTropical  bool
	TropicalVariant int32
}

// TropicalFish.COMMON_VARIANTS, in source order; pattern IDs use base | index << 8.
var naturalTropicalCommonVariants = [...]int32{
	packTropicalVariant(0x101, 1, 7),
	packTropicalVariant(0x001, 7, 7),
	packTropicalVariant(0x001, 7, 11),
	packTropicalVariant(0x501, 0, 7),
	packTropicalVariant(0x100, 11, 7),
	packTropicalVariant(0x000, 1, 0),
	packTropicalVariant(0x500, 6, 3),
	packTropicalVariant(0x301, 10, 4),
	packTropicalVariant(0x501, 0, 14),
	packTropicalVariant(0x500, 0, 4),
	packTropicalVariant(0x201, 0, 7),
	packTropicalVariant(0x501, 0, 1),
	packTropicalVariant(0x300, 9, 6),
	packTropicalVariant(0x400, 5, 3),
	packTropicalVariant(0x401, 14, 0),
	packTropicalVariant(0x200, 7, 14),
	packTropicalVariant(0x301, 14, 0),
	packTropicalVariant(0x001, 0, 4),
	packTropicalVariant(0x000, 14, 0),
	packTropicalVariant(0x100, 7, 0),
	packTropicalVariant(0x300, 9, 4),
	packTropicalVariant(0x001, 4, 4),
}

func (r *Runtime) spawnNaturalEntity(entityType game.EntityType, position game.Position, group *naturalSpawnGroup) RuntimeMobEntity {
	var entity RuntimeMobEntity

	switch entityType {
	case game.EntityCow:
		entity = r.newCow()
	case game.EntitySheep:
		entity = r.newSheep()
	case game.EntityChicken:
		entity = r.newChicken()
	case game.EntityZombie:
		entity = r.newZombie()
	case game.EntitySkeleton:
		entity = r.newSkeleton()
	case game.EntityCreeper:
		entity = r.newCreeper()
	case game.EntityBat:
		entity = r.newBat()
	case game.EntityCod:
		entity = r.newCod()
	case game.EntitySalmon:
		entity = r.newSalmon()
	case game.EntityTropicalFish:
		entity = r.newTropicalFish()
	case game.EntityPufferfish:
		entity = r.newPufferfish()
	default:
		return nil
	}

	yaw := r.nextEntityRandom() * 360
	rotation := runtimeLivingRotation(entity)
	*rotation = game.Rotation{Yaw: yaw, HeadYaw: yaw, BodyYaw: yaw}

	// Sheep and salmon select their variants before delegating to Mob.finalizeSpawn.
	switch mob := entity.(type) {
	case *runtimeSheepEntity:
		mob.WoolColor = r.naturalSheepColor(r.naturalBiomeAt(toBlockPosition(position)))
	case *runtimeSalmonEntity:
		mob.setVariant(r.naturalSalmonVariant())
	}

	mobState := entity.RuntimeMob()

	positiveBonus := float64(r.nextEntityRandom())
	negativeBonus := float64(r.nextEntityRandom())

	mobState.FollowRangeBonus = (positiveBonus - negativeBonus) * 0.11485000000000001
	mobState.LeftHanded = r.nextEntityRandom() < 0.05

	if fish, schooling := entity.(schoolingFish); schooling {
		if group.Leader == nil {
			group.Leader = fish
		} else {
			connectAquaticFollower(fish, group.Leader)
		}
	}

	switch mob := entity.(type) {
	case *runtimeTropicalFishEntity:
		r.finalizeNaturalTropicalFish(mob, group)
	case *runtimeZombieEntity:
		mob.Living.KnockbackResistance += r.nextEntityRandom() * 0.05
	}

	// Register only after initialization: registration builds the tracker and sends spawn packets.
	r.registerRuntimeEntity(entity, entityType, position)

	return entity
}

func (r *Runtime) naturalSalmonVariant() int32 {
	choice := runtimeMobRandomInt(r, 95)
	if choice < 30 {
		return 0
	}

	if choice < 80 {
		return 1
	}

	return 2
}

func (entity *runtimeSalmonEntity) setVariant(variant int32) {
	scale := 1.0

	switch variant {
	case 0:
		scale = 0.5
	case 2:
		scale = 1.5
	default:
		variant = 1
	}

	definition, _ := game.EntitySalmon.Definition()

	entity.Variant = variant
	entity.Living.Width = definition.Width * scale
	entity.Living.Height = definition.Height * scale
	entity.EyeHeight = salmonAquaticSpec.EyeHeight * scale
}

func (r *Runtime) finalizeNaturalTropicalFish(entity *runtimeTropicalFishEntity, group *naturalSpawnGroup) {
	if group.CommonTropical {
		entity.Variant = group.TropicalVariant

		return
	}

	if r.nextEntityRandom() < 0.9 {
		group.CommonTropical = true
		group.TropicalVariant = naturalTropicalCommonVariants[runtimeMobRandomInt(r, len(naturalTropicalCommonVariants))]

		entity.Variant = group.TropicalVariant

		return
	}

	// Random variants stop this spawn group; their later schooling AI is unchanged.
	pattern := runtimeMobRandomInt(r, 12)
	baseColor := runtimeMobRandomInt(r, 16)
	patternColor := runtimeMobRandomInt(r, 16)

	entity.Variant = packTropicalVariant(int32(pattern/6)|int32(pattern%6)<<8, int32(baseColor), int32(patternColor))
}

func (r *Runtime) naturalSheepColor(biome game.Biome) byte {
	colors := [5]byte{15, 7, 8, 12, 0}

	if naturalWarmFarmBiomes[biome] {
		colors = [5]byte{7, 8, 0, 15, 12}
	} else if naturalColdFarmBiomes[biome] {
		colors = [5]byte{8, 7, 0, 12, 15}
	}

	choice := runtimeMobRandomInt(r, 100)
	if choice < 15 {
		return colors[choice/5]
	}

	if choice < 18 {
		return colors[3]
	}

	if runtimeMobRandomInt(r, 500) == 499 {
		return 6
	}

	return colors[4]
}

func packTropicalVariant(pattern, baseColor, patternColor int32) int32 {
	return pattern&0xffff | (baseColor&0xff)<<16 | (patternColor&0xff)<<24
}
