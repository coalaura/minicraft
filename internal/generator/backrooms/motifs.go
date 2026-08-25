package backrooms

func motifStructureAt(seed int64, current zone) structure {
	firstHash := mix64(current.hash ^ saltMotif)
	first := singleMotifStructureAt(seed, current, firstHash, 52)

	secondHash := mix64(firstHash ^ 0x7d816a3cf205e94b)
	second := singleMotifStructureAt(seed, current, secondHash, 24)

	return mergeStructure(first, second)
}

func singleMotifStructureAt(seed int64, current zone, hash uint64, chance uint64) structure {
	if hash%100 >= chance {
		return structureOpen
	}

	switch (hash >> 8) % 5 {
	case 0:
		return freestandingWallMotif(current, hash)
	case 1:
		return islandRoomMotif(current, hash)
	case 2:
		return cornerMotif(current, hash)
	case 3:
		return bulkheadMotif(current, hash)
	default:
		return partitionMotif(current, hash)
	}
}

func freestandingWallMotif(current zone, hash uint64) structure {
	vertical := hash&(1<<16) == 0
	line := int64(12 + (hash>>20)%40)
	start := int64(8 + (hash>>28)%20)
	length := int64(14 + (hash>>36)%15)
	end := min(start+length, zoneSize-8)
	door := start + length/2

	if vertical {
		return verticalSegmentStructure(current.localX, current.localZ, line, start, end, door, 3, hash)
	}

	return horizontalSegmentStructure(current.localX, current.localZ, line, start, end, door, 3, hash)
}

func islandRoomMotif(current zone, hash uint64) structure {
	x0 := int64(8 + (hash>>16)%29)
	z0 := int64(8 + (hash>>24)%29)

	width := int64(9 + (hash>>32)%8)
	depth := int64(9 + (hash>>40)%8)

	x1 := min(x0+width, zoneSize-7)
	z1 := min(z0+depth, zoneSize-7)

	if !rectanglePerimeterAt(current.localX, current.localZ, x0, x1, z0, z1) {
		return structureOpen
	}

	side := (hash >> 48) % 4

	switch side {
	case 0:
		if current.localZ == z0 && withinOpening(current.localX, (x0+x1)/2, 3) {
			return structureDoorway
		}
	case 1:
		if current.localX == x1 && withinOpening(current.localZ, (z0+z1)/2, 3) {
			return structureDoorway
		}
	case 2:
		if current.localZ == z1 && withinOpening(current.localX, (x0+x1)/2, 3) {
			return structureDoorway
		}
	default:
		if current.localX == x0 && withinOpening(current.localZ, (z0+z1)/2, 3) {
			return structureDoorway
		}
	}

	return structureWall
}

func cornerMotif(current zone, hash uint64) structure {
	cornerX := int64(12 + (hash>>16)%40)
	cornerZ := int64(12 + (hash>>24)%40)

	lengthX := int64(7 + (hash>>32)%10)
	lengthZ := int64(7 + (hash>>40)%10)

	directionX := int64(1)
	directionZ := int64(1)

	if hash&(1<<50) != 0 {
		directionX = -1
	}
	if hash&(1<<51) != 0 {
		directionZ = -1
	}

	xEnd := clamp(cornerX+directionX*lengthX, 5, zoneSize-6)
	zEnd := clamp(cornerZ+directionZ*lengthZ, 5, zoneSize-6)

	horizontal := current.localZ == cornerZ && between(current.localX, cornerX, xEnd)
	vertical := current.localX == cornerX && between(current.localZ, cornerZ, zEnd)

	if horizontal || vertical {
		return structureWall
	}

	return structureOpen
}

func bulkheadMotif(current zone, hash uint64) structure {
	vertical := hash&(1<<16) == 0
	line := int64(10 + (hash>>20)%44)
	start := int64(8 + (hash>>28)%18)
	end := min(start+int64(18+(hash>>36)%20), zoneSize-7)

	if vertical {
		if current.localX == line && current.localZ >= start && current.localZ <= end {
			return structureBulkhead
		}

		return structureOpen
	}

	if current.localZ == line && current.localX >= start && current.localX <= end {
		return structureBulkhead
	}

	return structureOpen
}

func partitionMotif(current zone, hash uint64) structure {
	vertical := hash&(1<<16) == 0
	line := int64(10 + (hash>>20)%44)
	start := int64(8 + (hash>>28)%22)
	end := min(start+int64(10+(hash>>36)%14), zoneSize-7)

	if vertical {
		if current.localX == line && current.localZ >= start && current.localZ <= end {
			return structurePartition
		}

		return structureOpen
	}

	if current.localZ == line && current.localX >= start && current.localX <= end {
		return structurePartition
	}

	return structureOpen
}

func verticalSegmentStructure(x, z, wallX, startZ, endZ, doorCenter, doorWidth int64, hash uint64) structure {
	if x != wallX || !between(z, startZ, endZ) {
		return structureOpen
	}

	if withinOpening(z, doorCenter, doorWidth) {
		return openingStructure(doorWidth, hash)
	}

	return structureWall
}

func horizontalSegmentStructure(x, z, wallZ, startX, endX, doorCenter, doorWidth int64, hash uint64) structure {
	if z != wallZ || !between(x, startX, endX) {
		return structureOpen
	}

	if withinOpening(x, doorCenter, doorWidth) {
		return openingStructure(doorWidth, hash)
	}

	return structureWall
}
