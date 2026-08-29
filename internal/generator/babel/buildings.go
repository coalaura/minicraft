package babel

import "github.com/coalaura/minicraft/internal/game"

func towerBlock(worldY int32, localX, localZ int64, lot lotDescription) game.Block {
	relativeY := worldY - baseFloorY
	if relativeY <= 0 {
		return game.Air
	}

	if relativeY > lot.height {
		return towerCrownBlock(relativeY-lot.height, localX, localZ, lot)
	}

	firstSetback, secondSetback := towerSetbackLevels(lot)
	inset := lotOuterInsetAt(lot, relativeY)

	if relativeY == firstSetback || relativeY == secondSetback {
		previousInset := inset

		if relativeY == firstSetback {
			previousInset = lot.baseInset
		} else {
			previousInset = lot.baseInset + 2
		}

		if insideTowerFootprint(localX, localZ, previousInset, lot.hash) {
			if checker(localX, localZ, 3) {
				return lot.palette.floor
			}

			return lot.palette.wall2
		}
	}

	if !insideTowerFootprint(localX, localZ, inset, lot.hash) {
		return game.Air
	}

	if isEntranceOpening(relativeY, localX, localZ, inset, lot.hash) {
		return game.Air
	}

	if isFloor(relativeY, lot.floorHeight) {
		if insideCoreShaft(localX, localZ, lot.hash) {
			return game.Air
		}

		if facadeDistance(localX, localZ, inset) <= 1 && relativeY > 2 {
			return lot.palette.trim
		}

		return lot.palette.floor
	}

	if isTowerShell(localX, localZ, inset, lot.hash) {
		return towerFacadeBlock(relativeY, localX, localZ, inset, lot)
	}

	if isStructuralColumn(localX, localZ, inset) {
		if relativeY%8 == 0 {
			return lot.palette.light
		}

		return lot.palette.trim
	}

	return game.Air
}

func towerCrownBlock(crownY int32, localX, localZ int64, lot lotDescription) game.Block {
	crownHeight := int32(8 + (lot.hash>>40)%9)
	if crownY <= 0 || crownY > crownHeight {
		return game.Air
	}

	halfWidth := max(int64(5-crownY/4), 2)

	if centerDistance(localX) > halfWidth || centerDistance(localZ) > halfWidth {
		if crownY == crownHeight && centerDistance(localX) == 0 && centerDistance(localZ) == 0 {
			return lot.palette.light
		}

		return game.Air
	}

	if crownY == crownHeight || crownY%4 == 0 {
		return lot.palette.accent
	}

	if centerDistance(localX) == halfWidth || centerDistance(localZ) == halfWidth {
		return lot.palette.trim
	}

	return game.Air
}

func towerFacadeBlock(relativeY int32, localX, localZ, inset int64, lot lotDescription) game.Block {
	distance := facadeDistance(localX, localZ, inset)
	if distance == 0 && isCornerBand(localX, localZ, inset) {
		return lot.palette.trim
	}

	floorPhase := (relativeY - 1) % lot.floorHeight
	if floorPhase == 0 || floorPhase == lot.floorHeight-1 {
		return lot.palette.trim
	}

	panelCoordinate := localX

	if localX == inset || localX == lotScale-1-inset {
		panelCoordinate = localZ
	}

	panel := positiveRemainder(panelCoordinate+int64(lot.hash&3), 5)
	if (floorPhase == 2 || floorPhase == 3) && panel >= 1 && panel <= 3 {
		return lot.palette.glass
	}

	if panel == 0 && relativeY%11 == 0 {
		return lot.palette.accent
	}

	if ((relativeY/int32(lot.floorHeight))+int32(panel))&1 == 0 {
		return lot.palette.wall
	}

	return lot.palette.wall2
}

func courtyardBlock(worldY int32, localX, localZ int64, lot lotDescription) game.Block {
	relativeY := worldY - baseFloorY
	if relativeY <= 0 || relativeY > lot.height {
		return game.Air
	}

	outerInset := lot.baseInset
	if !insideRect(localX, localZ, outerInset) {
		return game.Air
	}

	innerInset := courtyardInnerInset(lot)
	insideInner := insideRect(localX, localZ, innerInset)

	if insideInner {
		gardenBlock := courtyardGardenBlock(relativeY, localX, localZ, lot)
		if gardenBlock != game.Air {
			return gardenBlock
		}

		bridgeLevel := int32(lot.floorHeight*4 + 1)
		if relativeY == bridgeLevel && (centerDistance(localX) <= 1 || centerDistance(localZ) <= 1) {
			return lot.palette.floor
		}

		if relativeY > bridgeLevel && relativeY < bridgeLevel+4 {
			if centerDistance(localX) == 1 || centerDistance(localZ) == 1 {
				return lot.palette.glass
			}
		}

		return game.Air
	}

	if isFloor(relativeY, lot.floorHeight) {
		return lot.palette.floor
	}

	outerShell := isOuterShell(localX, localZ, outerInset)
	innerShell := isInnerShell(localX, localZ, innerInset)
	if outerShell || innerShell {
		if relativeY%lot.floorHeight == 0 {
			return lot.palette.trim
		}

		panel := positiveRemainder(localX+localZ+int64(lot.hash&7), 5)
		floorPhase := (relativeY - 1) % lot.floorHeight
		if (floorPhase == 2 || floorPhase == 3) && panel >= 1 && panel <= 3 {
			return lot.palette.glass
		}

		if panel == 0 {
			return lot.palette.wall2
		}

		return lot.palette.wall
	}

	if isStructuralColumn(localX, localZ, outerInset) {
		return lot.palette.trim
	}

	if relativeY == lot.height && checker(localX, localZ, 2) {
		return lot.palette.accent
	}

	return game.Air
}

func courtyardGardenBlock(relativeY int32, localX, localZ int64, lot lotDescription) game.Block {
	centerX := localX - 23
	centerZ := localZ - 24
	radiusSquared := centerX*centerX + centerZ*centerZ

	if relativeY == 1 && radiusSquared <= 25 {
		return game.Water
	}

	if centerDistance(localX) == 0 && centerDistance(localZ) == 0 {
		if relativeY >= 1 && relativeY <= 6 {
			if relativeY == 6 {
				return lot.palette.light
			}

			return lot.palette.trim
		}
	}

	return game.Air
}

func plazaBlock(worldY int32, localX, localZ int64, lot lotDescription) game.Block {
	relativeY := worldY - baseFloorY
	if relativeY <= 0 || !insideRect(localX, localZ, lot.baseInset) {
		return game.Air
	}

	centerX := localX - 23
	centerZ := localZ - 24
	radiusSquared := centerX*centerX + centerZ*centerZ

	if relativeY == 1 && radiusSquared <= 25 {
		return game.Water
	}

	if centerDistance(localX) == 0 && centerDistance(localZ) == 0 && relativeY <= 8 {
		if relativeY == 8 {
			return lot.palette.light
		}

		return lot.palette.trim
	}

	cornerX := absolute(centerX)
	cornerZ := absolute(centerZ)
	if cornerX >= 9 && cornerX <= 10 && cornerZ >= 9 && cornerZ <= 10 && relativeY <= 11 {
		if relativeY == 11 {
			return lot.palette.light
		}

		return lot.palette.trim
	}

	if relativeY == 11 && ((cornerX == 10 && cornerZ <= 10) || (cornerZ == 10 && cornerX <= 10)) {
		return lot.palette.accent
	}

	return game.Air
}

func plazaSurface(localX, localZ int64, lot lotDescription) game.Block {
	centerX := localX - 23
	centerZ := localZ - 24
	radiusSquared := centerX*centerX + centerZ*centerZ

	if radiusSquared <= 36 {
		if radiusSquared >= 25 {
			return lot.palette.trim
		}

		return game.PrismarineBricks
	}

	if centerDistance(localX) <= 2 || centerDistance(localZ) <= 2 {
		return lot.palette.floor
	}

	if ((localX>>2)+(localZ>>2)+int64(lot.hash&1))&1 == 0 {
		return game.GrassBlock
	}

	return lot.palette.wall2
}

func describeLot(seed int64, cellX, cellZ int64) lotDescription {
	lotHash := hashCoordinates(seed, cellX, cellZ, 0xd1b54a32d192ed03)
	districtX := floorDiv(cellX, districtScale/lotScale)
	districtZ := floorDiv(cellZ, districtScale/lotScale)
	districtHash := hashCoordinates(seed, districtX, districtZ, 0x94d049bb133111eb)

	kind := lotTower

	switch lotHash % 11 {
	case 0:
		kind = lotPlaza
	case 1, 2:
		kind = lotCourtyard
	}

	baseInset := int64(7 + (lotHash>>16)%3)
	floorHeight := int32(5 + (lotHash>>20)&1)

	height := int32(72 + (lotHash>>24)%92 + (districtHash>>12)%28)

	if kind == lotCourtyard {
		height = int32(48 + (lotHash>>24)%54 + (districtHash>>12)%18)
	}

	if height > 178 {
		height = 178
	}

	paletteIndex := int((districtHash >> 32) % uint64(len(palettes)))

	return lotDescription{
		kind:        kind,
		palette:     palettes[paletteIndex],
		hash:        lotHash,
		baseInset:   baseInset,
		height:      height,
		floorHeight: floorHeight,
	}
}

func towerSetbackLevels(lot lotDescription) (int32, int32) {
	first := max(int32(18), lot.height*2/5)
	second := max(first+12, lot.height*3/4)

	return first, second
}

func lotOuterInsetAt(lot lotDescription, relativeY int32) int64 {
	if lot.kind != lotTower {
		return lot.baseInset
	}

	firstSetback, secondSetback := towerSetbackLevels(lot)
	inset := lot.baseInset

	if relativeY > firstSetback {
		inset += 2
	}

	if relativeY > secondSetback {
		inset += 3
	}

	return inset
}

func insideCourtyard(localX, localZ int64, lot lotDescription) bool {
	return insideRect(localX, localZ, courtyardInnerInset(lot))
}

func courtyardInnerInset(lot lotDescription) int64 {
	return int64(17 + (lot.hash>>36)%3)
}

func insideRect(localX, localZ, inset int64) bool {
	return localX >= inset && localX <= lotScale-1-inset && localZ >= inset && localZ <= lotScale-1-inset
}

func insideTowerFootprint(localX, localZ, inset int64, hash uint64) bool {
	if !insideRect(localX, localZ, inset) {
		return false
	}

	maxCoordinate := lotScale - 1 - inset
	edgeX := min(localX-inset, maxCoordinate-localX)
	edgeZ := min(localZ-inset, maxCoordinate-localZ)

	switch (hash >> 52) & 3 {
	case 1:
		return edgeX+edgeZ >= 4
	case 2:
		halfSpan := centerDistance(inset)
		return centerDistance(localX) <= halfSpan-5 || centerDistance(localZ) <= halfSpan-5
	case 3:
		cut := int64(5 + (hash>>54)&3)
		corner := (hash >> 58) & 3

		switch corner {
		case 0:
			return localX > inset+cut || localZ > inset+cut
		case 1:
			return localX < maxCoordinate-cut || localZ > inset+cut
		case 2:
			return localX > inset+cut || localZ < maxCoordinate-cut
		default:
			return localX < maxCoordinate-cut || localZ < maxCoordinate-cut
		}
	default:
		return true
	}
}

func isTowerShell(localX, localZ, inset int64, hash uint64) bool {
	if !insideTowerFootprint(localX, localZ, inset, hash) {
		return false
	}

	return !insideTowerFootprint(localX-1, localZ, inset, hash) ||
		!insideTowerFootprint(localX+1, localZ, inset, hash) ||
		!insideTowerFootprint(localX, localZ-1, inset, hash) ||
		!insideTowerFootprint(localX, localZ+1, inset, hash)
}

func isOuterShell(localX, localZ, inset int64) bool {
	maxCoordinate := lotScale - 1 - inset

	return localX == inset || localX == maxCoordinate || localZ == inset || localZ == maxCoordinate
}

func isInnerShell(localX, localZ, inset int64) bool {
	minCoordinate := inset - 1
	maxCoordinate := lotScale - inset

	return localX == minCoordinate || localX == maxCoordinate || localZ == minCoordinate || localZ == maxCoordinate
}

func isStructuralColumn(localX, localZ, inset int64) bool {
	maxCoordinate := lotScale - 1 - inset
	nearMinX := localX >= inset+2 && localX <= inset+3
	nearMaxX := localX <= maxCoordinate-2 && localX >= maxCoordinate-3
	nearMinZ := localZ >= inset+2 && localZ <= inset+3
	nearMaxZ := localZ <= maxCoordinate-2 && localZ >= maxCoordinate-3

	return (nearMinX || nearMaxX) && (nearMinZ || nearMaxZ)
}

func isCornerBand(localX, localZ, inset int64) bool {
	maxCoordinate := lotScale - 1 - inset

	return localX <= inset+1 || localX >= maxCoordinate-1 || localZ <= inset+1 || localZ >= maxCoordinate-1
}

func facadeDistance(localX, localZ, inset int64) int64 {
	maxCoordinate := lotScale - 1 - inset
	distance := min(localX-inset, maxCoordinate-localX)
	distance = min(distance, localZ-inset)
	distance = min(distance, maxCoordinate-localZ)

	return distance
}

func isFloor(relativeY, floorHeight int32) bool {
	return relativeY == 1 || (relativeY-1)%floorHeight == 0
}

func insideCoreShaft(localX, localZ int64, hash uint64) bool {
	halfWidth := int64(2 + (hash>>44)&1)

	return centerDistance(localX) <= halfWidth && centerDistance(localZ) <= halfWidth
}

func isEntranceOpening(relativeY int32, localX, localZ, inset int64, hash uint64) bool {
	if relativeY < 2 || relativeY > 5 {
		return false
	}

	centeredX := centerDistance(localX) <= 2
	centeredZ := centerDistance(localZ) <= 2
	maxCoordinate := lotScale - 1 - inset

	switch (hash >> 48) & 3 {
	case 0:
		return localZ == inset && centeredX
	case 1:
		return localX == maxCoordinate && centeredZ
	case 2:
		return localZ == maxCoordinate && centeredX
	default:
		return localX == inset && centeredZ
	}
}
