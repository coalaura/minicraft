package backrooms

import (
	"github.com/coalaura/minicraft/internal/game"
)

func libraryBlockAt(seed, worldX, worldY, worldZ int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 2); ok {
		return block, true
	}

	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	if worldY == int64(floorY+1) && current.localX == centerX && current.localZ == centerZ {
		return game.Lectern, true
	}

	hash := mix64(current.hash ^ saltFurniture)
	verticalRows := hash&1 == 0

	if verticalRows {
		relativeX := current.localX - room.x0
		if relativeX >= 3 && floorMod(relativeX-3, 5) == 0 && current.localZ >= room.z0+3 && current.localZ <= room.z1-3 {
			return bookshelfBlock(seed, worldX, worldZ), true
		}
	} else {
		relativeZ := current.localZ - room.z0
		if relativeZ >= 3 && floorMod(relativeZ-3, 5) == 0 && current.localX >= room.x0+3 && current.localX <= room.x1-3 {
			return bookshelfBlock(seed, worldX, worldZ), true
		}
	}

	if current.localZ == room.z0+1 && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
		return bookshelfBlock(seed, worldX, worldZ), true
	}

	return game.Air, false
}

func archiveBlockAt(seed, worldX, worldY, worldZ int64, current zone, room featureRoom) (game.Block, bool) {
	door := game.OakDoor
	if mix64(current.hash^saltFurniture)%4 == 0 {
		door = game.IronDoor
	}

	if block, ok := roomDoorBlockAt(door, worldY, current, room, 1); ok {
		return block, true
	}

	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	hash := mix64(current.hash ^ saltFurniture ^ 0x247ebca5d109836f)
	verticalRows := hash&1 == 0

	if verticalRows {
		relativeX := current.localX - room.x0
		if relativeX >= 2 && floorMod(relativeX-2, 4) == 0 && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
			return archiveShelfBlock(seed, worldX, worldZ), true
		}
	} else {
		relativeZ := current.localZ - room.z0
		if relativeZ >= 2 && floorMod(relativeZ-2, 4) == 0 && current.localX >= room.x0+2 && current.localX <= room.x1-2 {
			return archiveShelfBlock(seed, worldX, worldZ), true
		}
	}

	return game.Air, false
}

func receptionBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	counter := false
	lectern := false

	switch room.entranceSide {
	case featureNorth:
		line := room.z0 + 5
		counter = current.localZ == line && abs64(current.localX-centerX) <= 5
		counter = counter || current.localX == centerX+5 && current.localZ >= line && current.localZ <= line+4
		lectern = current.localX == centerX && current.localZ == line
	case featureSouth:
		line := room.z1 - 5
		counter = current.localZ == line && abs64(current.localX-centerX) <= 5
		counter = counter || current.localX == centerX-5 && current.localZ <= line && current.localZ >= line-4
		lectern = current.localX == centerX && current.localZ == line
	case featureEast:
		line := room.x1 - 5
		counter = current.localX == line && abs64(current.localZ-centerZ) <= 5
		counter = counter || current.localZ == centerZ+5 && current.localX <= line && current.localX >= line-4
		lectern = current.localX == line && current.localZ == centerZ
	case featureWest:
		line := room.x0 + 5
		counter = current.localX == line && abs64(current.localZ-centerZ) <= 5
		counter = counter || current.localZ == centerZ-5 && current.localX >= line && current.localX <= line+4
		lectern = current.localX == line && current.localZ == centerZ
	}

	if lectern && worldY == int64(floorY+2) {
		return game.Lectern, true
	}

	if counter && worldY == int64(floorY+1) {
		return game.OakPlanks, true
	}

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if receptionBackdropAt(current, room) {
			return game.Bookshelf, true
		}
	}

	return game.Air, false
}

func receptionBackdropAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1-1 && abs64(current.localX-centerX) <= 4
	case featureSouth:
		return current.localZ == room.z0+1 && abs64(current.localX-centerX) <= 4
	case featureEast:
		return current.localX == room.x0+1 && abs64(current.localZ-centerZ) <= 4
	default:
		return current.localX == room.x1-1 && abs64(current.localZ-centerZ) <= 4
	}
}

func serviceRoomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	door := game.OakDoor
	if mix64(current.hash^saltFurniture)&1 == 0 {
		door = game.IronDoor
	}

	if block, ok := roomDoorBlockAt(door, worldY, current, room, 1); ok {
		return block, true
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	if serviceShelfAt(current, room) {
		return game.OakPlanks, true
	}

	return game.Air, false
}

func serviceShelfAt(current zone, room featureRoom) bool {
	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1-1 && current.localX >= room.x0+2 && current.localX <= room.x1-2
	case featureSouth:
		return current.localZ == room.z0+1 && current.localX >= room.x0+2 && current.localX <= room.x1-2
	case featureEast:
		return current.localX == room.x0+1 && current.localZ >= room.z0+2 && current.localZ <= room.z1-2
	default:
		return current.localX == room.x1-1 && current.localZ >= room.z0+2 && current.localZ <= room.z1-2
	}
}

func conferenceBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 2); ok {
		return block, true
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	horizontal := room.x1-room.x0 >= room.z1-room.z0

	if horizontal {
		tableHalf := min(int64(7), (room.x1-room.x0)/2-4)
		if abs64(current.localX-centerX) <= tableHalf && abs64(current.localZ-centerZ) <= 1 {
			if current.localX == centerX-tableHalf && current.localZ == centerZ {
				return game.Lectern, true
			}

			return game.OakSlab, true
		}

		if abs64(current.localZ-centerZ) == 3 && abs64(current.localX-centerX) <= tableHalf && floorMod(current.localX-centerX, 3) == 0 {
			facing := "south"
			if current.localZ > centerZ {
				facing = "north"
			}

			return stairBlock(game.OakStairs, facing), true
		}
	} else {
		tableHalf := min(int64(7), (room.z1-room.z0)/2-4)
		if abs64(current.localZ-centerZ) <= tableHalf && abs64(current.localX-centerX) <= 1 {
			if current.localZ == centerZ-tableHalf && current.localX == centerX {
				return game.Lectern, true
			}

			return game.OakSlab, true
		}

		if abs64(current.localX-centerX) == 3 && abs64(current.localZ-centerZ) <= tableHalf && floorMod(current.localZ-centerZ, 3) == 0 {
			facing := "east"
			if current.localX > centerX {
				facing = "west"
			}

			return stairBlock(game.OakStairs, facing), true
		}
	}

	return game.Air, false
}

func bathroomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.IronDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	hash := mix64(current.hash ^ saltFurniture ^ 0x41d7c9a2be830f65)

	verticalStalls := hash&1 == 0

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if index, door := bathroomStallDoorIndexAt(current, room, verticalStalls); door {
			if index == 1 && hash&(1<<12) != 0 {
				return game.Air, true
			}

			hinge := "left"
			if index%2 != 0 {
				hinge = "right"
			}

			facing := "north"
			if !verticalStalls {
				facing = "west"
			}

			return actualDoorBlock(game.OakDoor, worldY, facing, hinge)
		}
	}

	if worldY != int64(floorY+1) {
		return game.Air, false
	}

	if verticalStalls {
		if current.localZ == room.z0+2 && current.localX >= room.x0+3 && current.localX <= room.x1-3 && floorMod(current.localX-room.x0, 3) == 0 {
			return game.SmoothQuartz, true
		}
	} else if current.localX == room.x0+2 && current.localZ >= room.z0+3 && current.localZ <= room.z1-3 && floorMod(current.localZ-room.z0, 3) == 0 {
		return game.SmoothQuartz, true
	}

	return game.Air, false
}

func renovationBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0

	hash := mix64(current.hash ^ saltFurniture ^ 0x1c97e406b53af82d)

	post := floorMod(relativeX+int64(hash%5), 8) == 3 && floorMod(relativeZ+int64((hash>>8)%5), 8) == 3
	if post {
		if worldY == int64(floorY+1) && (relativeX+relativeZ)%3 == 0 {
			return game.YellowTerracotta, true
		}

		return game.OakPlanks, true
	}

	if worldY == int64(floorY+1) && floorMod(relativeX+relativeZ+int64(hash%7), 23) == 0 {
		return game.OakPlanks, true
	}

	return game.Air, false
}

func windowRoomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if windowWallAt(current, room) {
			return game.TintedGlass, true
		}
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		if current.localZ == room.z1-3 && abs64(current.localX-centerX) <= 3 {
			return game.OakSlab, true
		}
	case featureSouth:
		if current.localZ == room.z0+3 && abs64(current.localX-centerX) <= 3 {
			return game.OakSlab, true
		}
	case featureEast:
		if current.localX == room.x0+3 && abs64(current.localZ-centerZ) <= 3 {
			return game.OakSlab, true
		}
	case featureWest:
		if current.localX == room.x1-3 && abs64(current.localZ-centerZ) <= 3 {
			return game.OakSlab, true
		}
	}

	return game.Air, false
}

func windowWallAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1 && abs64(current.localX-centerX) <= 5
	case featureSouth:
		return current.localZ == room.z0 && abs64(current.localX-centerX) <= 5
	case featureEast:
		return current.localX == room.x0 && abs64(current.localZ-centerZ) <= 5
	default:
		return current.localX == room.x1 && abs64(current.localZ-centerZ) <= 5
	}
}

func storageBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.IronDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	if current.localX == centerX && current.localZ >= room.z0+2 && current.localZ <= room.z1-2 {
		openingCenter := (room.z0 + room.z1) / 2
		if abs64(current.localZ-openingCenter) > 1 && (worldY == int64(floorY+1) || worldY == int64(floorY+2)) {
			return game.IronBars, true
		}
	}

	if worldY != int64(floorY+1) && worldY != int64(floorY+2) {
		return game.Air, false
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0

	crate := floorMod(relativeX, 6) <= 1 && floorMod(relativeZ, 6) <= 1 && relativeX >= 3 && relativeZ >= 3
	if !crate {
		return game.Air, false
	}

	hash := mix64(current.hash ^ uint64(relativeX*131+relativeZ*719) ^ saltFurniture)
	if worldY == int64(floorY+2) && hash%3 != 0 {
		return game.Air, false
	}

	return game.OakPlanks, true
}

func classroomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.OakDoor, worldY, current, room, 2); ok {
		return block, true
	}

	if worldY == int64(floorY+1) || worldY == int64(floorY+2) {
		if classroomBoardAt(current, room) {
			return game.BlackWool, true
		}
	}

	if worldY != int64(floorY+1) || !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	if classroomTeacherPositionAt(current, room) {
		return game.Lectern, true
	}

	horizontalRows := room.entranceSide == featureNorth || room.entranceSide == featureSouth
	if horizontalRows {
		relativeX := current.localX - room.x0
		relativeZ := current.localZ - room.z0

		if relativeX >= 4 && relativeX <= room.x1-room.x0-4 && relativeZ >= 6 && relativeZ <= room.z1-room.z0-5 {
			if floorMod(relativeX, 4) == 1 && floorMod(relativeZ, 4) == 1 {
				return game.OakSlab, true
			}

			if floorMod(relativeX, 4) == 1 && floorMod(relativeZ, 4) == 2 {
				facing := "north"
				if room.entranceSide == featureNorth {
					facing = "south"
				}

				return stairBlock(game.OakStairs, facing), true
			}
		}
	} else {
		relativeX := current.localX - room.x0
		relativeZ := current.localZ - room.z0

		if relativeZ >= 4 && relativeZ <= room.z1-room.z0-4 && relativeX >= 6 && relativeX <= room.x1-room.x0-5 {
			if floorMod(relativeZ, 4) == 1 && floorMod(relativeX, 4) == 1 {
				return game.OakSlab, true
			}

			if floorMod(relativeZ, 4) == 1 && floorMod(relativeX, 4) == 2 {
				facing := "west"
				if room.entranceSide == featureWest {
					facing = "east"
				}

				return stairBlock(game.OakStairs, facing), true
			}
		}
	}

	_ = centerX
	_ = centerZ

	return game.Air, false
}

func classroomBoardAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localZ == room.z1 && abs64(current.localX-centerX) <= 4
	case featureSouth:
		return current.localZ == room.z0 && abs64(current.localX-centerX) <= 4
	case featureEast:
		return current.localX == room.x0 && abs64(current.localZ-centerZ) <= 4
	default:
		return current.localX == room.x1 && abs64(current.localZ-centerZ) <= 4
	}
}

func classroomTeacherPositionAt(current zone, room featureRoom) bool {
	centerX := (room.x0 + room.x1) / 2
	centerZ := (room.z0 + room.z1) / 2

	switch room.entranceSide {
	case featureNorth:
		return current.localX == centerX && current.localZ == room.z1-3
	case featureSouth:
		return current.localX == centerX && current.localZ == room.z0+3
	case featureEast:
		return current.localX == room.x0+3 && current.localZ == centerZ
	default:
		return current.localX == room.x1-3 && current.localZ == centerZ
	}
}

func machineRoomBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	if block, ok := roomDoorBlockAt(game.IronDoor, worldY, current, room, 1); ok {
		return block, true
	}

	if !roomInteriorContains(room, current.localX, current.localZ) {
		return game.Air, false
	}

	relativeX := current.localX - room.x0
	relativeZ := current.localZ - room.z0

	machine := relativeX >= 3 && relativeZ >= 3 && floorMod(relativeX, 6) <= 1 && floorMod(relativeZ, 6) <= 1

	if machine && (worldY == int64(floorY+1) || worldY == int64(floorY+2)) {
		if worldY == int64(floorY+2) && (relativeX+relativeZ)%3 == 0 {
			return game.IronBars, true
		}

		return game.CopperBlock, true
	}

	if worldY == int64(floorY+2) && floorMod(relativeX+2, 7) == 3 && floorMod(relativeZ+1, 5) == 2 {
		return game.IronBars, true
	}

	return game.Air, false
}

func doorGalleryBlockAt(worldY int64, current zone, room featureRoom) (game.Block, bool) {
	vertical, line, direction := doorGalleryWall(current, room)

	index, ok := doorGalleryDoorIndex(current, room, vertical, line)
	if !ok {
		return game.Air, false
	}

	facing := "south"
	if vertical {
		if direction > 0 {
			facing = "east"
		} else {
			facing = "west"
		}
	} else if direction < 0 {
		facing = "north"
	}

	door := game.OakDoor
	if index == 2 && mix64(current.hash^saltFurniture)&1 == 0 {
		door = game.IronDoor
	}

	hinge := "left"
	if index%2 != 0 {
		hinge = "right"
	}

	return actualDoorBlock(door, worldY, facing, hinge)
}
