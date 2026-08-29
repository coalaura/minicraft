package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
)

const featureRoomMargin = int64(6)

func featureStructureAt(seed int64, current zone) (structure, bool) {
	if current.feature == featureNone {
		return structureOpen, false
	}

	room := featureRoomForZone(current)

	switch current.feature {
	case featureLibrary:
		return roomFeatureStructureAt(current, room, 2)
	case featureArchive:
		return roomFeatureStructureAt(current, room, 1)
	case featureDarkRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureServiceRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureConference:
		return roomFeatureStructureAt(current, room, 2)
	case featureBathroom:
		return bathroomStructureAt(current, room)
	case featureRenovation:
		return renovationStructureAt(current, room)
	case featureWindowRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureStorage:
		return roomFeatureStructureAt(current, room, 1)
	case featureClassroom:
		return roomFeatureStructureAt(current, room, 2)
	case featureMachineRoom:
		return roomFeatureStructureAt(current, room, 1)
	case featureReception:
		return receptionStructureAt(current, room)
	case featureDoorGallery:
		return doorGalleryStructureAt(current, room)
	default:
		return structureOpen, false
	}
}

func featureRoomForZone(current zone) featureRoom {
	hash := mix64(current.hash ^ saltFeature ^ 0x5e21a93fc487d60b)

	width := int64(18 + (hash>>8)%5)
	depth := int64(15 + (hash>>16)%5)

	switch current.feature {
	case featureLibrary:
		width = int64(20 + (hash>>8)%5)
		depth = int64(17 + (hash>>16)%5)
	case featureArchive:
		width = int64(17 + (hash>>8)%4)
		depth = int64(15 + (hash>>16)%4)
	case featureDarkRoom:
		width = int64(17 + (hash>>8)%6)
		depth = int64(15 + (hash>>16)%6)
	case featureDoorGallery:
		width = int64(20 + (hash>>8)%4)
		depth = int64(18 + (hash>>16)%4)
	case featureServiceRoom:
		width = int64(9 + (hash>>8)%4)
		depth = int64(8 + (hash>>16)%4)
	case featureConference:
		width = int64(24 + (hash>>8)%6)
		depth = int64(18 + (hash>>16)%6)
	case featureBathroom:
		width = int64(18 + (hash>>8)%5)
		depth = int64(16 + (hash>>16)%5)
	case featureRenovation:
		width = int64(23 + (hash>>8)%7)
		depth = int64(19 + (hash>>16)%7)
	case featureWindowRoom:
		width = int64(20 + (hash>>8)%5)
		depth = int64(17 + (hash>>16)%5)
	case featureStorage:
		width = int64(16 + (hash>>8)%5)
		depth = int64(14 + (hash>>16)%5)
	case featureClassroom:
		width = int64(24 + (hash>>8)%5)
		depth = int64(20 + (hash>>16)%5)
	case featureMachineRoom:
		width = int64(18 + (hash>>8)%5)
		depth = int64(16 + (hash>>16)%5)
	}

	quadrant := (hash >> 24) % 4

	x0 := featureRoomMargin
	z0 := featureRoomMargin

	if quadrant == 1 || quadrant == 3 {
		x0 = zoneSize - featureRoomMargin - width
	}

	if quadrant >= 2 {
		z0 = zoneSize - featureRoomMargin - depth
	}

	x1 := x0 + width - 1
	z1 := z0 + depth - 1

	var entranceSide featureSide

	switch quadrant {
	case 0:
		if hash&(1<<40) == 0 {
			entranceSide = featureEast
		} else {
			entranceSide = featureSouth
		}
	case 1:
		if hash&(1<<40) == 0 {
			entranceSide = featureWest
		} else {
			entranceSide = featureSouth
		}
	case 2:
		if hash&(1<<40) == 0 {
			entranceSide = featureEast
		} else {
			entranceSide = featureNorth
		}
	default:
		if hash&(1<<40) == 0 {
			entranceSide = featureWest
		} else {
			entranceSide = featureNorth
		}
	}

	jitter := int64((hash>>48)%5) - 2
	doorCenter := (z0+z1)/2 + jitter

	if entranceSide == featureNorth || entranceSide == featureSouth {
		doorCenter = (x0+x1)/2 + jitter
	}

	if entranceSide == featureNorth || entranceSide == featureSouth {
		doorCenter = clamp(doorCenter, x0+2, x1-2)
	} else {
		doorCenter = clamp(doorCenter, z0+2, z1-2)
	}

	return featureRoom{
		x0:           x0,
		x1:           x1,
		z0:           z0,
		z1:           z1,
		entranceSide: entranceSide,
		doorCenter:   doorCenter,
	}
}

func roomFeatureStructureAt(current zone, room featureRoom, doorWidth int64) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if !rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		return structureOpen, true
	}

	if roomEntranceAt(current, room, doorWidth) {
		return structureDoorway, true
	}

	return structureWall, true
}

func bathroomStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 1) {
			return structureDoorway, true
		}

		return structureWall, true
	}

	hash := mix64(current.hash ^ saltFurniture ^ 0x41d7c9a2be830f65)
	verticalStalls := hash&1 == 0

	if verticalStalls {
		front := room.z1 - 6
		back := room.z1 - 1

		for x := room.x0 + 3; x <= room.x1-3; x += 4 {
			if current.localX == x && current.localZ >= front && current.localZ <= back {
				return structurePartition, true
			}
		}

		if current.localZ == front && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
			_, door := bathroomStallDoorIndexAt(current, room, true)

			if door {
				return structureOpen, true
			}

			return structurePartition, true
		}
	} else {
		front := room.x1 - 6
		back := room.x1 - 1

		for z := room.z0 + 3; z <= room.z1-3; z += 4 {
			if current.localZ == z && current.localX >= front && current.localX <= back {
				return structurePartition, true
			}
		}

		if current.localX == front && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
			_, door := bathroomStallDoorIndexAt(current, room, false)
			if door {
				return structureOpen, true
			}

			return structurePartition, true
		}
	}

	return structureOpen, true
}

func bathroomStallDoorIndexAt(current zone, room featureRoom, vertical bool) (int, bool) {
	if vertical {
		front := room.z1 - 6
		if current.localZ != front {
			return 0, false
		}

		for dividerX := room.x0 + 3; dividerX <= room.x1-3; dividerX += 4 {
			doorX := dividerX + 2
			if doorX >= room.x1-1 {
				continue
			}

			if current.localX == doorX {
				return int((dividerX - (room.x0 + 3)) / 4), true
			}
		}

		return 0, false
	}

	front := room.x1 - 6

	if current.localX != front {
		return 0, false
	}

	for dividerZ := room.z0 + 3; dividerZ <= room.z1-3; dividerZ += 4 {
		doorZ := dividerZ + 2
		if doorZ >= room.z1-1 {
			continue
		}

		if current.localZ == doorZ {
			return int((dividerZ - (room.z0 + 3)) / 4), true
		}
	}

	return 0, false
}

func renovationStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 8) {
			return structureOpen, true
		}

		hash := mix64(current.hash ^ saltFeature ^ 0x72e84b15c09f36da)
		if hash%3 == 0 {
			gapCenter := int64(8 + (hash>>12)%48)

			coordinate := current.localX

			if current.localX == room.x0 || current.localX == room.x1 {
				coordinate = current.localZ
			}

			if abs64(coordinate-gapCenter) <= 2 {
				return structureOpen, true
			}
		}

		return structureWall, true
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0

	hash := mix64(current.hash ^ saltFeature ^ 0xd53a106cf8be2479)

	if floorMod(relativeX+int64(hash%7), 9) == 4 && relativeZ >= 3 && relativeZ <= room.z1-room.z0-3 {
		if floorMod(relativeZ+int64((hash>>8)%11), 13) > 4 {
			return structurePartition, true
		}
	}

	if floorMod(relativeZ+int64((hash>>16)%7), 11) == 5 && relativeX >= 3 && relativeX <= room.x1-room.x0-3 {
		if floorMod(relativeX+int64((hash>>24)%9), 14) > 6 {
			return structureWall, true
		}
	}

	return structureOpen, true
}

func receptionStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 7) {
			return structureOpen, true
		}

		return structureWall, true
	}

	return structureOpen, true
}

func doorGalleryStructureAt(current zone, room featureRoom) (structure, bool) {
	if !roomContains(room, current.localX, current.localZ) {
		return structureOpen, false
	}

	if rectanglePerimeterAt(current.localX, current.localZ, room.x0, room.x1, room.z0, room.z1) {
		if roomEntranceAt(current, room, 5) {
			return structureOpen, true
		}

		return structureWall, true
	}

	vertical, line, direction := doorGalleryWall(current, room)

	index, ok := doorGalleryDoorIndex(current, room, vertical, line)
	if ok {
		_ = index

		return structureDoorway, true
	}

	if vertical {
		if current.localX == line && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
			return structureWall, true
		}
	} else if current.localZ == line && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
		return structureWall, true
	}

	for index := range 3 {
		if !doorGalleryDoorIsFalse(current, index) {
			continue
		}

		coordinate := doorGalleryDoorCoordinate(room, vertical, index)

		if vertical {
			if current.localX == line+direction && current.localZ == coordinate {
				return structureWall, true
			}
		} else if current.localZ == line+direction && current.localX == coordinate {
			return structureWall, true
		}
	}

	return structureOpen, true
}

func doorGalleryWall(current zone, room featureRoom) (bool, int64, int64) {
	hash := mix64(current.hash ^ saltFeature ^ 0xf3210bc4795a6de8)

	vertical := hash&1 == 0

	direction := int64(1)

	if hash&(1<<8) != 0 {
		direction = -1
	}

	if vertical {
		return true, (room.x0 + room.x1) / 2, direction
	}

	return false, (room.z0 + room.z1) / 2, direction
}

func doorGalleryDoorIndex(current zone, room featureRoom, vertical bool, line int64) (int, bool) {
	if vertical {
		if current.localX != line {
			return 0, false
		}
	} else if current.localZ != line {
		return 0, false
	}

	for index := range 3 {
		coordinate := doorGalleryDoorCoordinate(room, vertical, index)
		if vertical && current.localZ == coordinate {
			return index, true
		}

		if !vertical && current.localX == coordinate {
			return index, true
		}
	}

	return 0, false
}

func doorGalleryDoorCoordinate(room featureRoom, vertical bool, index int) int64 {
	var (
		minimum int64
		maximum int64
	)

	if vertical {
		minimum = room.z0 + 3
		maximum = room.z1 - 3
	} else {
		minimum = room.x0 + 3
		maximum = room.x1 - 3
	}

	switch index {
	case 0:
		return minimum
	case 1:
		return (minimum + maximum) / 2
	default:
		return maximum
	}
}

func doorGalleryDoorIsFalse(current zone, index int) bool {
	if index == 0 {
		return true
	}

	if index == 1 {
		return false
	}

	hash := mix64(current.hash ^ saltFeature ^ 0x861d4e39ba7025cf)
	return hash&1 == 0
}

func featureFloorBlock(current zone, blocks paletteBlocks) (game.Block, bool) {
	if current.feature == featureNone {
		return game.Air, false
	}

	room := featureRoomForZone(current)
	if !roomContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	switch current.feature {
	case featureDarkRoom:
		return blocks.wear, true
	case featureArchive:
		if floorMod(current.localX+current.localZ, 5) == 0 {
			return blocks.wear, true
		}
	case featureBathroom:
		return game.SmoothQuartz, true
	case featureRenovation, featureMachineRoom:
		return game.SmoothStone, true
	case featureStorage:
		return game.GrayWool, true
	case featureWindowRoom:
		return game.LightGrayWool, true
	}

	return game.Air, false
}

func darkRoomAt(current zone) bool {
	if current.feature != featureDarkRoom {
		return false
	}

	room := featureRoomForZone(current)
	return roomContains(room, current.localX, current.localZ)
}

func featureBlockAt(seed, worldX, worldY, worldZ int64, current zone) (game.Block, bool) {
	if current.feature == featureNone || zoneSpineOpenAt(seed, current) {
		return game.Air, false
	}

	room := featureRoomForZone(current)
	if !roomContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	switch current.feature {
	case featureLibrary:
		return libraryBlockAt(seed, worldX, worldY, worldZ, current, room)
	case featureArchive:
		return archiveBlockAt(seed, worldX, worldY, worldZ, current, room)
	case featureReception:
		return receptionBlockAt(worldY, current, room)
	case featureDarkRoom:
		return roomDoorBlockAt(game.OakDoor, worldY, current, room, 1)
	case featureDoorGallery:
		return doorGalleryBlockAt(worldY, current, room)
	case featureServiceRoom:
		return serviceRoomBlockAt(worldY, current, room)
	case featureConference:
		return conferenceBlockAt(worldY, current, room)
	case featureBathroom:
		return bathroomBlockAt(worldY, current, room)
	case featureRenovation:
		return renovationBlockAt(worldY, current, room)
	case featureWindowRoom:
		return windowRoomBlockAt(worldY, current, room)
	case featureStorage:
		return storageBlockAt(worldY, current, room)
	case featureClassroom:
		return classroomBlockAt(worldY, current, room)
	case featureMachineRoom:
		return machineRoomBlockAt(worldY, current, room)
	default:
		return game.Air, false
	}
}
