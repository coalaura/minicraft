package babel

import "github.com/coalaura/minicraft/internal/game"

func grandPlazaSurface(relativeX, relativeZ int64) game.Block {
	offsetX := absolute(gridSignedOffset(relativeX, districtScale))
	offsetZ := absolute(gridSignedOffset(relativeZ, districtScale))
	outer := max(offsetX, offsetZ)

	if outer == 12 || outer == 13 {
		return game.QuartzBlock
	}

	if offsetX <= 1 || offsetZ <= 1 {
		return game.PolishedDiorite
	}

	if (offsetX+offsetZ)%7 == 0 {
		return game.GoldBlock
	}

	if ((offsetX>>2)+(offsetZ>>2))&1 == 0 {
		return game.SmoothStone
	}

	return game.StoneBricks
}

func roadSurface(relativeX, relativeZ int64, streets streetState) game.Block {
	if (streets.grandX || streets.boulevardX || streets.streetX) && (streets.grandZ || streets.boulevardZ || streets.streetZ) {
		return game.GrayConcrete
	}

	if streets.grandX {
		offset := absolute(gridSignedOffset(relativeX, districtScale))
		if offset == grandHalfWidth {
			return game.LightGrayConcrete
		}

		if offset == 0 && positiveRemainder(relativeZ, 10) < 5 {
			return game.YellowConcrete
		}

		return game.BlackConcrete
	}

	if streets.grandZ {
		offset := absolute(gridSignedOffset(relativeZ, districtScale))
		if offset == grandHalfWidth {
			return game.LightGrayConcrete
		}

		if offset == 0 && positiveRemainder(relativeX, 10) < 5 {
			return game.YellowConcrete
		}

		return game.BlackConcrete
	}

	if streets.boulevardX {
		offset := absolute(gridSignedOffset(relativeX, boulevardScale))
		if offset == boulevardHalfWidth {
			return game.LightGrayConcrete
		}

		if offset == 0 && positiveRemainder(relativeZ, 12) < 4 {
			return game.WhiteConcrete
		}

		return game.GrayConcrete
	}

	if streets.boulevardZ {
		offset := absolute(gridSignedOffset(relativeZ, boulevardScale))
		if offset == boulevardHalfWidth {
			return game.LightGrayConcrete
		}

		if offset == 0 && positiveRemainder(relativeX, 12) < 4 {
			return game.WhiteConcrete
		}

		return game.GrayConcrete
	}

	if streets.streetX {
		if absolute(gridSignedOffset(relativeX, lotScale)) == streetHalfWidth {
			return game.LightGrayConcrete
		}

		return game.GrayConcrete
	}

	if absolute(gridSignedOffset(relativeZ, lotScale)) == streetHalfWidth {
		return game.LightGrayConcrete
	}

	return game.GrayConcrete
}

func grandIntersectionBlock(seed int64, worldY int32, relativeX, relativeZ int64) game.Block {
	offsetX := absolute(gridSignedOffset(relativeX, districtScale))
	offsetZ := absolute(gridSignedOffset(relativeZ, districtScale))

	if offsetX > grandPlazaRadius || offsetZ > grandPlazaRadius {
		return game.Air
	}

	if worldY >= baseFloorY+1 && worldY <= baseFloorY+12 {
		if offsetX >= 10 && offsetX <= 11 && offsetZ >= 10 && offsetZ <= 11 {
			if worldY == baseFloorY+12 {
				return game.SeaLantern
			}

			return game.QuartzPillar
		}
	}

	if worldY == baseFloorY+12 {
		if (offsetX == 10 && offsetZ <= 10) || (offsetZ == 10 && offsetX <= 10) {
			if (offsetX+offsetZ+int64(uint64(seed)&3))%6 == 0 {
				return game.GoldBlock
			}

			return game.QuartzBlock
		}
	}

	return game.Air
}

func elevatedAvenueBlock(seed int64, worldY int32, relativeX, relativeZ int64, streets streetState) game.Block {
	if !streets.grandX && !streets.grandZ {
		return game.Air
	}

	if inGrandPlaza(relativeX, relativeZ) && worldY < skywayFloorY {
		return game.Air
	}

	if streets.grandX {
		offset := absolute(gridSignedOffset(relativeX, districtScale))

		block := avenueAxisBlock(seed, worldY, offset, relativeZ)
		if block != game.Air {
			return block
		}
	}

	if streets.grandZ {
		offset := absolute(gridSignedOffset(relativeZ, districtScale))

		block := avenueAxisBlock(seed^0x5bd1e995, worldY, offset, relativeX)
		if block != game.Air {
			return block
		}
	}

	return game.Air
}

func avenueAxisBlock(seed int64, worldY int32, crossOffset, alongCoordinate int64) game.Block {
	if crossOffset > walkwayHalfWidth {
		if worldY >= baseFloorY+1 && worldY < skywayFloorY && crossOffset == walkwayHalfWidth+2 && positiveRemainder(alongCoordinate, 24) <= 1 {
			if worldY%6 == 0 {
				return game.SeaLantern
			}

			return game.QuartzPillar
		}

		return game.Air
	}

	if worldY == skywayFloorY {
		if positiveRemainder(alongCoordinate+int64(seed&7), 12) <= 1 {
			return game.SeaLantern
		}

		return game.SmoothQuartz
	}

	if worldY > skywayFloorY && worldY < skywayBeamY && crossOffset == walkwayHalfWidth {
		return game.LightBlueStainedGlass
	}

	if worldY == skywayBeamY && positiveRemainder(alongCoordinate, 8) <= 1 {
		if crossOffset == walkwayHalfWidth {
			return game.QuartzPillar
		}

		return game.QuartzBlock
	}

	return game.Air
}

func localSkybridgeBlock(seed int64, worldY int32, relativeX, relativeZ int64, _ streetState) (game.Block, bool) {
	distanceX := absolute(gridSignedOffset(relativeX, lotScale))
	distanceZ := absolute(gridSignedOffset(relativeZ, lotScale))

	if distanceX <= 10 {
		boundaryX := nearestGridIndex(relativeX, lotScale)
		cellZ := floorDiv(relativeZ, lotScale)
		localZ := positiveRemainder(relativeZ, lotScale)

		if boundaryX%2 != 0 && centerDistance(localZ) <= 2 {
			left := describeLot(seed, boundaryX-1, cellZ)
			right := describeLot(seed, boundaryX, cellZ)
			bridgeHash := hashCoordinates(seed, boundaryX, cellZ, 0x6d2b79f5)

			if bridgeHash%4 == 0 {
				block, claimed := skybridgeAxisBlock(worldY, distanceX, centerDistance(localZ), left, right, bridgeHash)
				if claimed {
					return block, true
				}
			}
		}
	}

	if distanceZ <= 10 {
		boundaryZ := nearestGridIndex(relativeZ, lotScale)
		cellX := floorDiv(relativeX, lotScale)
		localX := positiveRemainder(relativeX, lotScale)

		if boundaryZ%2 != 0 && centerDistance(localX) <= 2 {
			near := describeLot(seed, cellX, boundaryZ-1)
			far := describeLot(seed, cellX, boundaryZ)
			bridgeHash := hashCoordinates(seed, cellX, boundaryZ, 0x9e3779b9)

			if bridgeHash%4 == 0 {
				block, claimed := skybridgeAxisBlock(worldY, distanceZ, centerDistance(localX), near, far, bridgeHash)
				if claimed {
					return block, true
				}
			}
		}
	}

	return game.Air, false
}

func skybridgeAxisBlock(worldY int32, crossingOffset, widthOffset int64, first, second lotDescription, bridgeHash uint64) (game.Block, bool) {
	if first.kind == lotPlaza || second.kind == lotPlaza {
		return game.Air, false
	}

	bridgeY := baseFloorY + 28 + int32((bridgeHash>>8)%7)*6
	if first.height < bridgeY-baseFloorY+6 || second.height < bridgeY-baseFloorY+6 {
		return game.Air, false
	}

	bridgeRelativeY := bridgeY - baseFloorY

	span := max(lotOuterInsetAt(first, bridgeRelativeY), lotOuterInsetAt(second, bridgeRelativeY)) + 1
	if crossingOffset > span || widthOffset > 2 || worldY < bridgeY || worldY > bridgeY+4 {
		return game.Air, false
	}

	palette := first.palette

	if worldY == bridgeY {
		if widthOffset == 0 && crossingOffset%5 == 0 {
			return palette.light, true
		}

		return palette.floor, true
	}

	if worldY < bridgeY+4 && widthOffset == 2 {
		return palette.glass, true
	}

	if worldY == bridgeY+4 {
		if crossingOffset%4 == 0 {
			return palette.trim, true
		}

		return palette.wall2, true
	}

	return game.Air, true
}

func classifyStreets(relativeX, relativeZ int64) streetState {
	grandX := absolute(gridSignedOffset(relativeX, districtScale)) <= grandHalfWidth
	grandZ := absolute(gridSignedOffset(relativeZ, districtScale)) <= grandHalfWidth

	boulevardX := !grandX && absolute(gridSignedOffset(relativeX, boulevardScale)) <= boulevardHalfWidth
	boulevardZ := !grandZ && absolute(gridSignedOffset(relativeZ, boulevardScale)) <= boulevardHalfWidth

	streetX := !grandX && !boulevardX && absolute(gridSignedOffset(relativeX, lotScale)) <= streetHalfWidth
	streetZ := !grandZ && !boulevardZ && absolute(gridSignedOffset(relativeZ, lotScale)) <= streetHalfWidth

	return streetState{
		grandX:     grandX,
		grandZ:     grandZ,
		boulevardX: boulevardX,
		boulevardZ: boulevardZ,
		streetX:    streetX,
		streetZ:    streetZ,
	}
}

func nearStreet(relativeX, relativeZ int64) bool {
	if absolute(gridSignedOffset(relativeX, districtScale)) <= grandWalkHalfWidth || absolute(gridSignedOffset(relativeZ, districtScale)) <= grandWalkHalfWidth {
		return true
	}

	if absolute(gridSignedOffset(relativeX, boulevardScale)) <= boulevardWalkHalfWidth || absolute(gridSignedOffset(relativeZ, boulevardScale)) <= boulevardWalkHalfWidth {
		return true
	}

	return absolute(gridSignedOffset(relativeX, lotScale)) <= streetWalkHalfWidth || absolute(gridSignedOffset(relativeZ, lotScale)) <= streetWalkHalfWidth
}

func inGrandPlaza(relativeX, relativeZ int64) bool {
	return absolute(gridSignedOffset(relativeX, districtScale)) <= grandPlazaRadius && absolute(gridSignedOffset(relativeZ, districtScale)) <= grandPlazaRadius
}
