package game

const supportEpsilon = 1e-9

type supportRectangle struct {
	minFirst  float64
	minSecond float64
	maxFirst  float64
	maxSecond float64
}

func (block Block) FaceSturdy(face BlockFace) bool {
	return block.FullFace(face)
}

func (block Block) FullFace(face BlockFace) bool {
	return block.coversFace(face, []supportRectangle{{maxFirst: 1, maxSecond: 1}})
}

func (block Block) SupportsCenter(face BlockFace) bool {
	return block.coversFace(face, []supportRectangle{{
		minFirst: 7.0 / 16, minSecond: 7.0 / 16,
		maxFirst: 9.0 / 16, maxSecond: 9.0 / 16,
	}})
}

func (block Block) SupportsRigid(face BlockFace) bool {
	minimum := 2.0 / 16
	maximum := 14.0 / 16

	targets := []supportRectangle{
		{maxFirst: 1, maxSecond: minimum},
		{minSecond: maximum, maxFirst: 1, maxSecond: 1},
		{minSecond: minimum, maxFirst: minimum, maxSecond: maximum},
		{minFirst: maximum, minSecond: minimum, maxFirst: 1, maxSecond: maximum},
	}

	return block.coversFace(face, targets)
}

func (block Block) coversFace(face BlockFace, targets []supportRectangle) bool {
	boxes := block.CollisionBoxes(BlockPosition{})
	rectangles := make([]supportRectangle, 0, len(boxes))

	for _, box := range boxes {
		rectangle, touches := boxFaceRectangle(box, face)
		if touches {
			rectangles = append(rectangles, rectangle)
		}
	}

	for _, target := range targets {
		if !rectangleCovered(target, rectangles) {
			return false
		}
	}

	return len(targets) != 0
}

func boxFaceRectangle(box AABB, face BlockFace) (supportRectangle, bool) {
	switch face {
	case BlockFaceDown:
		return supportRectangle{box.MinX, box.MinZ, box.MaxX, box.MaxZ}, box.MinY <= supportEpsilon
	case BlockFaceUp:
		return supportRectangle{box.MinX, box.MinZ, box.MaxX, box.MaxZ}, box.MaxY >= 1-supportEpsilon
	case BlockFaceNorth:
		return supportRectangle{box.MinX, box.MinY, box.MaxX, box.MaxY}, box.MinZ <= supportEpsilon
	case BlockFaceSouth:
		return supportRectangle{box.MinX, box.MinY, box.MaxX, box.MaxY}, box.MaxZ >= 1-supportEpsilon
	case BlockFaceWest:
		return supportRectangle{box.MinZ, box.MinY, box.MaxZ, box.MaxY}, box.MinX <= supportEpsilon
	case BlockFaceEast:
		return supportRectangle{box.MinZ, box.MinY, box.MaxZ, box.MaxY}, box.MaxX >= 1-supportEpsilon
	default:
		return supportRectangle{}, false
	}
}

func rectangleCovered(target supportRectangle, rectangles []supportRectangle) bool {
	firstCoordinates := []float64{target.minFirst, target.maxFirst}
	secondCoordinates := []float64{target.minSecond, target.maxSecond}

	for _, rectangle := range rectangles {
		if rectangle.maxFirst <= target.minFirst || rectangle.minFirst >= target.maxFirst || rectangle.maxSecond <= target.minSecond || rectangle.minSecond >= target.maxSecond {
			continue
		}

		firstCoordinates = append(firstCoordinates, rectangle.minFirst, rectangle.maxFirst)
		secondCoordinates = append(secondCoordinates, rectangle.minSecond, rectangle.maxSecond)
	}

	for firstIndex := 0; firstIndex < len(firstCoordinates); firstIndex++ {
		for secondIndex := 0; secondIndex < len(secondCoordinates); secondIndex++ {
			first := firstCoordinates[firstIndex]
			second := secondCoordinates[secondIndex]

			if first < target.minFirst || first >= target.maxFirst || second < target.minSecond || second >= target.maxSecond {
				continue
			}

			covered := false

			for _, rectangle := range rectangles {
				if first+supportEpsilon >= rectangle.minFirst && first < rectangle.maxFirst-supportEpsilon && second+supportEpsilon >= rectangle.minSecond && second < rectangle.maxSecond-supportEpsilon {
					covered = true

					break
				}
			}

			if !covered {
				return false
			}
		}
	}

	return true
}
