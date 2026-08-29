package backrooms

import "github.com/coalaura/minicraft/internal/game"

func structureBlock(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks, profile structure) game.Block {
	currentCeilingY := zoneCeilingY(current)

	switch profile {
	case structureWall, structurePillar:
		return wallBlock(seed, worldX, worldY, worldZ, current, blocks)
	case structureDoorway:
		if int32(worldY) == currentCeilingY-1 {
			return wallBlock(seed, worldX, worldY, worldZ, current, blocks)
		}
	case structurePartition:
		if int32(worldY) == floorY+1 {
			return blocks.wall
		}

		if int32(worldY) == floorY+2 {
			return blocks.trim
		}
	case structureBulkhead:
		if int32(worldY) == currentCeilingY-1 {
			return blocks.trim
		}
	}

	return game.Air
}

func wallBlock(seed, worldX, worldY, worldZ int64, current zone, blocks paletteBlocks) game.Block {
	if worldY == int64(floorY+1) {
		return blocks.trim
	}

	patchX := floorDiv(worldX, 3)
	patchZ := floorDiv(worldZ, 3)

	hash := coordinateHash(seed, patchX, patchZ, saltDetail^uint64(worldY))

	if hash%149 == 0 {
		return blocks.accent
	}

	return blocks.wall
}

func structureAt(seed int64, current zone) structure {
	boundary, found := zoneBoundaryStructureAt(seed, current)
	if found {
		return boundary
	}

	if verticalBaseOpenAt(seed, current) {
		return structureOpen
	}

	if zoneSpineOpenAt(seed, current) {
		return structureOpen
	}

	featured, ok := featureStructureAt(seed, current)
	if ok {
		return featured
	}

	oddity, ok := ambientDoorStructureAt(seed, current)
	if ok {
		return oddity
	}

	base := layoutStructureAt(seed, current)
	motif := motifStructureAt(seed, current)

	return mergeStructure(base, motif)
}

func layoutStructureAt(seed int64, current zone) structure {
	switch current.layout {
	case layoutMaze:
		return mazeStructureAt(seed, current)
	case layoutLongHalls:
		return longHallStructureAt(seed, current)
	case layoutCrossroads:
		return crossroadsStructureAt(seed, current)
	case layoutCubicles:
		return cubicleStructureAt(seed, current)
	case layoutPillars:
		return pillarStructureAt(seed, current)
	case layoutSparse:
		return sparseStructureAt(seed, current)
	default:
		return classicStructureAt(seed, current)
	}
}

func zoneBoundaryStructureAt(seed int64, current zone) (structure, bool) {
	xBoundary := current.localX == 0
	zBoundary := current.localZ == 0

	if !xBoundary && !zBoundary {
		return structureOpen, false
	}

	if xBoundary && zBoundary {
		return structureWall, true
	}

	if xBoundary {
		spec := boundaryOpeningSpec(seed, current.z, current.x, true)

		return boundaryPointStructure(current.localZ, spec), true
	}

	spec := boundaryOpeningSpec(seed, current.x, current.z, false)

	return boundaryPointStructure(current.localX, spec), true
}

func boundaryOpening(seed, segment, local, boundary int64, vertical bool) bool {
	spec := boundaryOpeningSpec(seed, segment, boundary, vertical)
	return withinOpening(local, spec.centerA, spec.widthA) || withinOpening(local, spec.centerB, spec.widthB)
}

func boundaryOpeningSpec(seed, segment, boundary int64, vertical bool) openingSpec {
	salt := saltEdge

	if vertical {
		salt ^= 0xe9b91f1b9de263c7
	}

	hash := coordinateHash(seed, boundary, segment, salt)

	return openingSpec{
		hash:    hash,
		centerA: int64(14 + hash%9),
		centerB: int64(41 + (hash>>8)%9),
		widthA:  int64(4 + (hash>>16)%4),
		widthB:  int64(4 + (hash>>24)%4),
	}
}

func boundaryPointStructure(local int64, spec openingSpec) structure {
	if withinOpening(local, spec.centerA, spec.widthA) {
		return openingStructure(spec.widthA, mix64(spec.hash^0x75bf8d2e90c64a13))
	}

	if withinOpening(local, spec.centerB, spec.widthB) {
		return openingStructure(spec.widthB, mix64(spec.hash^0x23617dbe5f09a4c7))
	}

	return structureWall
}

func preferredBoundaryCenter(seed, segment, boundary int64, vertical bool) int64 {
	spec := boundaryOpeningSpec(seed, segment, boundary, vertical)
	if spec.hash&(1<<40) == 0 {
		return spec.centerA
	}

	return spec.centerB
}

func withinOpening(local, center, width int64) bool {
	half := width / 2
	return local >= center-half && local <= center+(width-1)/2
}

func openingStructure(width int64, hash uint64) structure {
	if width >= 5 && hash%4 == 0 {
		return structureOpen
	}

	return structureDoorway
}

func zoneSpineOpenAt(seed int64, current zone) bool {
	hubX := int64(24 + (current.hash>>32)%17)
	hubZ := int64(24 + (current.hash>>40)%17)

	width := int64(3)

	if current.localX <= hubX+width/2 {
		westZ := preferredBoundaryCenter(seed, current.z, current.x, true)

		if horizontalPathAt(current.localX, current.localZ, 0, hubX, westZ, width) || (withinOpening(current.localX, hubX, width) && between(current.localZ, westZ, hubZ)) {
			return true
		}
	}

	if current.localX >= hubX-width/2 {
		eastZ := preferredBoundaryCenter(seed, current.z, current.x+1, true)

		if horizontalPathAt(current.localX, current.localZ, hubX, zoneSize-1, eastZ, width) || (withinOpening(current.localX, hubX, width) && between(current.localZ, eastZ, hubZ)) {
			return true
		}
	}

	if current.localZ <= hubZ+width/2 {
		northX := preferredBoundaryCenter(seed, current.x, current.z, false)

		if verticalPathAt(current.localX, current.localZ, 0, hubZ, northX, width) || (withinOpening(current.localZ, hubZ, width) && between(current.localX, northX, hubX)) {
			return true
		}
	}

	if current.localZ >= hubZ-width/2 {
		southX := preferredBoundaryCenter(seed, current.x, current.z+1, false)

		if verticalPathAt(current.localX, current.localZ, hubZ, zoneSize-1, southX, width) || (withinOpening(current.localZ, hubZ, width) && between(current.localX, southX, hubX)) {
			return true
		}
	}

	return withinOpening(current.localX, hubX, width) && withinOpening(current.localZ, hubZ, width)
}

func horizontalPathAt(x, z, startX, endX, centerZ, width int64) bool {
	minimum := min(startX, endX)
	maximum := max(startX, endX)

	return x >= minimum && x <= maximum && withinOpening(z, centerZ, width)
}

func verticalPathAt(x, z, startZ, endZ, centerX, width int64) bool {
	minimum := min(startZ, endZ)
	maximum := max(startZ, endZ)

	return z >= minimum && z <= maximum && withinOpening(x, centerX, width)
}

func classicStructureAt(seed int64, current zone) structure {
	base := gridStructureAt(seed, current, 16, 3, 5, 35, 0x39a45bb4829f2e1d, 0xb1af2e88384f7cc5)
	divider := classicDividerStructureAt(seed, current)

	return mergeStructure(base, divider)
}

const dividerCellSize = int64(16)

func classicDividerStructureAt(seed int64, current zone) structure {
	cellX := current.localX / dividerCellSize
	cellZ := current.localZ / dividerCellSize

	withinX := current.localX % dividerCellSize
	withinZ := current.localZ % dividerCellSize

	hash := coordinateHash(seed, current.x*4+cellX, current.z*4+cellZ, saltWall^0x8f0b72d3be9c1465)
	if hash%100 >= 68 {
		return structureOpen
	}

	wallX := int64(5 + (hash>>8)%6)
	wallZ := int64(5 + (hash>>16)%6)

	doorX := int64(4 + (hash>>24)%7)
	doorZ := int64(4 + (hash>>32)%7)

	switch (hash >> 40) % 6 {
	case 0:
		return verticalSegmentStructure(withinX, withinZ, wallX, 2, 13, doorZ, 3, hash)
	case 1:
		return horizontalSegmentStructure(withinX, withinZ, wallZ, 2, 13, doorX, 3, hash)
	case 2:
		vertical := verticalSegmentStructure(withinX, withinZ, wallX, 2, 13, doorZ, 3, hash)
		horizontal := horizontalSegmentStructure(withinX, withinZ, wallZ, wallX, 13, doorX, 3, hash>>1)

		return mergeStructure(vertical, horizontal)
	case 3:
		vertical := verticalSegmentStructure(withinX, withinZ, wallX, wallZ, 13, doorZ, 3, hash)
		horizontal := horizontalSegmentStructure(withinX, withinZ, wallZ, 2, wallX, doorX, 3, hash>>1)

		return mergeStructure(vertical, horizontal)
	case 4:
		if withinZ == wallZ && withinX >= 3 && withinX <= 12 {
			return structureWall
		}
	case 5:
		if (withinX == wallX || withinX == wallX+3) && withinZ >= 4 && withinZ <= 11 {
			if withinOpening(withinZ, doorZ, 3) {
				return structureDoorway
			}

			return structureWall
		}
	}

	return structureOpen
}

func mazeStructureAt(seed int64, current zone) structure {
	base := gridStructureAt(seed, current, 8, 2, 3, 6, 0x0c62b5949e3d81f7, 0x6db18fe3a45c27d9)
	if base != structureOpen {
		return base
	}

	cellX := current.localX / 8
	cellZ := current.localZ / 8

	withinX := current.localX % 8
	withinZ := current.localZ % 8

	hash := coordinateHash(seed, current.x*8+cellX, current.z*8+cellZ, saltWall^0x146bed3fa5c89270)

	if hash%100 >= 24 {
		return structureOpen
	}

	if hash&1 == 0 {
		wallX := int64(2 + (hash>>8)%4)
		if withinX == wallX && withinZ >= 1 && withinZ <= 5 {
			return structureWall
		}

		return structureOpen
	}

	wallZ := int64(2 + (hash>>8)%4)
	if withinZ == wallZ && withinX >= 2 && withinX <= 6 {
		return structureWall
	}

	return structureOpen
}

func gridStructureAt(seed int64, current zone, cellSize, minDoorWidth, maxDoorWidth, missingPercent int64, xSalt, zSalt uint64) structure {
	result := structureOpen

	if current.localX%cellSize == 0 {
		segment := current.localZ / cellSize
		wall := current.localX / cellSize

		hash := coordinateHash(seed, current.x*16+wall, current.z*16+segment, saltWall^xSalt)

		if hash%100 >= uint64(missingPercent) {
			width := doorWidthForHash(hash, minDoorWidth, maxDoorWidth)

			if gridDoorOpen(current.localZ, cellSize, width, hash) {
				result = mergeStructure(result, openingStructure(width, hash>>7))
			} else {
				return structureWall
			}
		}
	}

	if current.localZ%cellSize == 0 {
		segment := current.localX / cellSize
		wall := current.localZ / cellSize

		hash := coordinateHash(seed, current.x*16+segment, current.z*16+wall, saltWall^zSalt)

		if hash%100 >= uint64(missingPercent) {
			width := doorWidthForHash(hash, minDoorWidth, maxDoorWidth)

			if gridDoorOpen(current.localX, cellSize, width, hash) {
				result = mergeStructure(result, openingStructure(width, hash>>7))
			} else {
				return structureWall
			}
		}
	}

	return result
}

func doorWidthForHash(hash uint64, minimum, maximum int64) int64 {
	if maximum <= minimum {
		return minimum
	}

	return minimum + int64((hash>>20)%uint64(maximum-minimum+1))
}

func gridDoorOpen(local, cellSize, width int64, hash uint64) bool {
	position := floorMod(local, cellSize)
	margin := int64(1)
	span := max(cellSize-width-2*margin+1, 1)
	start := margin + int64((hash>>12)%uint64(span))

	return position >= start && position < start+width
}

func longHallParameters(current zone) (bool, int64, int64) {
	vertical := current.hash&1 == 0
	center := int64(28 + (current.hash>>8)%9)
	halfWidth := int64(2 + (current.hash>>16)%2)

	return vertical, center, halfWidth
}

func longHallStructureAt(seed int64, current zone) structure {
	vertical, corridorCenter, halfWidth := longHallParameters(current)

	across := current.localX
	along := current.localZ

	if !vertical {
		swap := across
		across = along
		along = swap
	}

	if abs64(across-corridorCenter) <= halfWidth {
		return structureOpen
	}

	leftWall := corridorCenter - halfWidth - 1
	rightWall := corridorCenter + halfWidth + 1

	if across == leftWall || across == rightWall {
		segment := along / 12

		side := int64(0)

		if across == rightWall {
			side = 1
		}

		hash := coordinateHash(seed, current.x*16+segment, current.z*16+side, saltWall^0xc28fd4d4604d39c7)
		width := doorWidthForHash(hash, 3, 5)

		if gridDoorOpen(along, 12, width, hash) {
			return openingStructure(width, hash>>11)
		}

		return structureWall
	}

	phase := int64((current.hash >> 24) % 6)

	if floorMod(along+phase, 12) == 0 {
		hash := coordinateHash(seed, current.x*8+across/8, current.z*8+along/12, saltWall^0x4ef37cd47f93d123)
		if hash%100 < 78 {
			width := doorWidthForHash(hash, 3, 4)

			if gridDoorOpen(across, 16, width, hash) {
				return openingStructure(width, hash>>9)
			}

			return structureWall
		}
	}

	return structureOpen
}

func crossroadsStructureAt(seed int64, current zone) structure {
	centerX := int64(29 + (current.hash>>8)%7)
	centerZ := int64(29 + (current.hash>>16)%7)

	halfX := int64(6 + (current.hash>>24)%3)
	halfZ := int64(6 + (current.hash>>32)%3)

	deltaX := abs64(current.localX - centerX)
	deltaZ := abs64(current.localZ - centerZ)

	if deltaX <= halfX && deltaZ <= halfZ {
		return structureOpen
	}

	if deltaX == halfX+1 && deltaZ <= halfZ+1 {
		if withinOpening(current.localZ, centerZ, 5) {
			return structureDoorway
		}

		return structureWall
	}

	if deltaZ == halfZ+1 && deltaX <= halfX+1 {
		if withinOpening(current.localX, centerX, 5) {
			return structureDoorway
		}

		return structureWall
	}

	return gridStructureAt(seed, current, 12, 3, 5, 24, 0x947e2a5db6813fc0, 0x2a1dc4b9538e76f1)
}

func cubicleStructureAt(seed int64, current zone) structure {
	cellX := current.localX / 8
	cellZ := current.localZ / 8

	withinX := current.localX % 8
	withinZ := current.localZ % 8

	hash := coordinateHash(seed, current.x*8+cellX, current.z*8+cellZ, saltWall^0x71873ff6f885b9d1)

	if hash%100 < 18 {
		return structureOpen
	}

	if hash%19 == 0 {
		pillarX := int64(3 + (hash>>8)%2)
		pillarZ := int64(3 + (hash>>12)%2)

		if withinX == pillarX && withinZ == pillarZ {
			return structurePillar
		}
	}

	arm := int64(3 + (hash>>16)%3)
	orientation := (hash >> 24) % 4

	switch orientation {
	case 0:
		if (withinX == 1 && withinZ <= arm) || (withinZ == 1 && withinX <= arm) {
			return structurePartition
		}
	case 1:
		if (withinX == 6 && withinZ <= arm) || (withinZ == 1 && withinX >= 7-arm) {
			return structurePartition
		}
	case 2:
		if (withinX == 6 && withinZ >= 7-arm) || (withinZ == 6 && withinX >= 7-arm) {
			return structurePartition
		}
	default:
		if (withinX == 1 && withinZ >= 7-arm) || (withinZ == 6 && withinX <= arm) {
			return structurePartition
		}
	}

	return structureOpen
}

const pillarCellSize = int64(8)

func pillarStructureAt(seed int64, current zone) structure {
	cellX := current.localX / pillarCellSize
	cellZ := current.localZ / pillarCellSize

	withinX := current.localX % pillarCellSize
	withinZ := current.localZ % pillarCellSize

	hash := coordinateHash(seed, current.x*8+cellX, current.z*8+cellZ, saltWall^0x52f01d1fa98ddfed)

	if hash%100 < 24 {
		return structureOpen
	}

	size := int64(1)

	if (hash>>8)%100 < 34 {
		size = 2
	}

	if (hash>>16)%100 < 5 {
		size = 3
	}

	span := max(pillarCellSize-size-2, 1)

	startX := int64(1 + (hash>>24)%uint64(span))
	startZ := int64(1 + (hash>>32)%uint64(span))

	if withinX >= startX && withinX < startX+size && withinZ >= startZ && withinZ < startZ+size {
		return structurePillar
	}

	if hash%13 == 0 {
		if withinZ == startZ && withinX >= startX && withinX <= min(startX+5, pillarCellSize-2) {
			return structureWall
		}
	}

	return structureOpen
}

const sparseCellSize = int64(16)

func sparseStructureAt(seed int64, current zone) structure {
	cellX := current.localX / sparseCellSize
	cellZ := current.localZ / sparseCellSize

	withinX := current.localX % sparseCellSize
	withinZ := current.localZ % sparseCellSize

	hash := coordinateHash(seed, current.x*4+cellX, current.z*4+cellZ, saltWall^0xd21fb6417307e88f)

	switch hash % 8 {
	case 0, 1:
		return structureOpen
	case 2:
		if withinZ == 6 && withinX >= 2 && withinX <= 13 {
			if withinOpening(withinX, 9, 4) {
				return structureDoorway
			}

			return structureWall
		}
	case 3:
		if withinX == 8 && withinZ >= 2 && withinZ <= 13 {
			if withinOpening(withinZ, 6, 3) {
				return structureDoorway
			}

			return structureWall
		}
	case 4:
		if (withinX == 4 && withinZ >= 4 && withinZ <= 12) ||
			(withinZ == 4 && withinX >= 4 && withinX <= 11) {
			return structureWall
		}
	case 5:
		if rectanglePerimeterAt(withinX, withinZ, 3, 12, 4, 11) {
			if withinZ == 11 && withinOpening(withinX, 8, 4) {
				return structureDoorway
			}

			return structureWall
		}
	case 6:
		if withinZ == 5 && withinX >= 3 && withinX <= 12 {
			return structurePartition
		}
	case 7:
		if withinX >= 6 && withinX <= 8 && withinZ >= 5 && withinZ <= 10 {
			return structurePillar
		}
	}

	return structureOpen
}
