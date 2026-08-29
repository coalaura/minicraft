package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
)

func ambientDoorSpecForZone(seed int64, current zone) ambientDoorSpec {
	if current.feature != featureNone || current.vertical != verticalNone {
		return ambientDoorSpec{}
	}

	hash := mix64(current.hash ^ saltOddity)
	if hash%100 >= 4 {
		return ambientDoorSpec{}
	}

	for attempt := range uint64(6) {
		candidate := mix64(hash ^ (attempt+1)*0x9e3779b97f4a7c15)
		vertical := candidate&1 == 0

		line := int64(11 + (candidate>>8)%42)
		center := int64(11 + (candidate>>16)%42)

		direction := int64(1)

		if candidate&(1<<24) != 0 {
			direction = -1
		}

		falseDoor := (candidate>>32)%100 < 62
		iron := (candidate>>40)%100 < 18

		doorProbe := current
		backProbe := current

		if vertical {
			doorProbe.localX = line
			doorProbe.localZ = center

			backProbe.localX = line + direction
			backProbe.localZ = center
		} else {
			doorProbe.localX = center
			doorProbe.localZ = line

			backProbe.localX = center
			backProbe.localZ = line + direction
		}

		if zoneSpineOpenAt(seed, doorProbe) || zoneSpineOpenAt(seed, backProbe) {
			continue
		}

		if !falseDoor {
			clear := true

			for distance := int64(1); distance <= 2; distance++ {
				probe := current

				if vertical {
					probe.localX = line + direction*distance
					probe.localZ = center
				} else {
					probe.localX = center
					probe.localZ = line + direction*distance
				}

				if zoneSpineOpenAt(seed, probe) {
					continue
				}

				profile := mergeStructure(layoutStructureAt(seed, probe), motifStructureAt(seed, probe))
				if profile == structureWall || profile == structurePillar || profile == structurePartition {
					clear = false

					break
				}
			}

			if !clear {
				continue
			}
		}

		return ambientDoorSpec{
			enabled:   true,
			vertical:  vertical,
			line:      line,
			center:    center,
			direction: direction,
			falseDoor: falseDoor,
			iron:      iron,
		}
	}

	return ambientDoorSpec{}
}

func ambientDoorStructureAt(seed int64, current zone) (structure, bool) {
	spec := ambientDoorSpecForZone(seed, current)
	if !spec.enabled {
		return structureOpen, false
	}

	if spec.vertical {
		if current.localX == spec.line && abs64(current.localZ-spec.center) <= 5 {
			if current.localZ == spec.center {
				return structureDoorway, true
			}

			return structureWall, true
		}

		if spec.falseDoor && current.localX == spec.line+spec.direction && current.localZ == spec.center {
			return structureWall, true
		}
	} else {
		if current.localZ == spec.line && abs64(current.localX-spec.center) <= 5 {
			if current.localX == spec.center {
				return structureDoorway, true
			}

			return structureWall, true
		}

		if spec.falseDoor && current.localZ == spec.line+spec.direction && current.localX == spec.center {
			return structureWall, true
		}
	}

	return structureOpen, false
}

func ambientDoorBlockAt(seed, worldY int64, current zone) (game.Block, bool) {
	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	spec := ambientDoorSpecForZone(seed, current)
	if !spec.enabled {
		return game.Air, false
	}

	onDoor := false
	facing := "south"

	if spec.vertical {
		onDoor = current.localX == spec.line && current.localZ == spec.center

		if spec.direction > 0 {
			facing = "east"
		} else {
			facing = "west"
		}
	} else {
		onDoor = current.localZ == spec.line && current.localX == spec.center

		if spec.direction < 0 {
			facing = "north"
		}
	}

	if !onDoor {
		return game.Air, false
	}

	door := game.OakDoor

	if spec.iron {
		door = game.IronDoor
	}

	hinge := "left"

	if mix64(current.hash^saltOddity^0x75be29c104f8da63)&1 != 0 {
		hinge = "right"
	}

	return actualDoorBlock(door, worldY, facing, hinge)
}

func roomDoorBlockAt(base game.Block, worldY int64, current zone, room featureRoom, width int64) (game.Block, bool) {
	index, ok := roomEntranceIndex(current, room, width)
	if !ok {
		return game.Air, false
	}

	hinge := "left"

	if width > 1 && index%2 != 0 {
		hinge = "right"
	} else if width == 1 && mix64(current.hash^saltFurniture)&1 != 0 {
		hinge = "right"
	}

	return actualDoorBlock(base, worldY, doorFacingForSide(room.entranceSide), hinge)
}

func actualDoorBlock(base game.Block, worldY int64, facing, hinge string) (game.Block, bool) {
	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	half := "lower"

	if worldY == int64(floorY+2) {
		half = "upper"
	}

	block, ok := base.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: facing},
		game.BlockPropertyValue{Name: "half", Value: half},
		game.BlockPropertyValue{Name: "hinge", Value: hinge},
		game.BlockPropertyValue{Name: "open", Value: "false"},
	)

	if !ok {
		return base, true
	}

	return block, true
}

func roomEntranceIndex(current zone, room featureRoom, width int64) (int64, bool) {
	if !roomEntranceAt(current, room, width) {
		return 0, false
	}

	coordinate := current.localZ

	if room.entranceSide == featureNorth || room.entranceSide == featureSouth {
		coordinate = current.localX
	}

	start := room.doorCenter - width/2
	return coordinate - start, true
}

func roomEntranceAt(current zone, room featureRoom, width int64) bool {
	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z0 && withinOpening(current.localX, room.doorCenter, width)
	case featureEast:
		return current.localX == room.x1 && withinOpening(current.localZ, room.doorCenter, width)
	case featureSouth:
		return current.localZ == room.z1 && withinOpening(current.localX, room.doorCenter, width)
	default:
		return current.localX == room.x0 && withinOpening(current.localZ, room.doorCenter, width)
	}
}

func doorFacingForSide(side featureSide) string {
	switch side {
	case featureNorth:
		return "south"
	case featureEast:
		return "west"
	case featureSouth:
		return "north"
	default:
		return "east"
	}
}

func bookshelfBlock(seed, worldX, worldZ int64) game.Block {
	hash := coordinateHash(seed, worldX, worldZ, saltFurniture)
	if hash%5 == 0 {
		return game.ChiseledBookshelf
	}

	return game.Bookshelf
}

func archiveShelfBlock(seed, worldX, worldZ int64) game.Block {
	hash := coordinateHash(seed, worldX, worldZ, saltFurniture^0x5f208ca4d16e739b)
	if hash%3 != 0 {
		return game.ChiseledBookshelf
	}

	return game.Bookshelf
}

func roomContains(room featureRoom, x, z int64) bool {
	return x >= room.x0 && x <= room.x1 && z >= room.z0 && z <= room.z1
}

func roomInteriorContains(room featureRoom, x, z int64) bool {
	return x > room.x0 && x < room.x1 && z > room.z0 && z < room.z1
}
