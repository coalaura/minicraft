package berlinvoxel

import "math"

type Point struct {
	X float64
	Y float64
	Z float64
}

type point2 struct {
	x float64
	y float64
}

func RasterizeTriangle(first, second, third Point, emit func(easting, northing int32, height int)) int {
	normal := cross(subtract(second, first), subtract(third, first))
	absoluteX := math.Abs(normal.X)
	absoluteY := math.Abs(normal.Y)
	absoluteZ := math.Abs(normal.Z)

	if absoluteX < 1e-9 && absoluteY < 1e-9 && absoluteZ < 1e-9 {
		return 0
	}

	axis := 2
	if absoluteX >= absoluteY && absoluteX >= absoluteZ {
		axis = 0
	} else if absoluteY >= absoluteZ {
		axis = 1
	}

	projected := [3]point2{
		projectPoint(first, axis),
		projectPoint(second, axis),
		projectPoint(third, axis),
	}

	minimumU := min(projected[0].x, projected[1].x, projected[2].x)
	maximumU := max(projected[0].x, projected[1].x, projected[2].x)
	minimumV := min(projected[0].y, projected[1].y, projected[2].y)
	maximumV := max(projected[0].y, projected[1].y, projected[2].y)

	startU := int(math.Ceil(minimumU - 0.5))
	endU := int(math.Floor(maximumU + 0.5))
	startV := int(math.Ceil(minimumV - 0.5))
	endV := int(math.Floor(maximumV + 0.5))

	emitted := 0

	for valueV := startV; valueV <= endV; valueV++ {
		for valueU := startU; valueU <= endU; valueU++ {
			if !triangleSquareOverlap(projected, float64(valueU), float64(valueV)) {
				continue
			}

			missing, valid := solveMissing(first, normal, axis, float64(valueU), float64(valueV))
			if !valid || math.IsNaN(missing) || math.IsInf(missing, 0) {
				continue
			}

			roundedMissing := int(math.Round(missing))

			switch axis {
			case 0:
				emit(int32(roundedMissing), int32(valueU), valueV)
			case 1:
				emit(int32(valueU), int32(roundedMissing), valueV)
			default:
				emit(int32(valueU), int32(valueV), roundedMissing)
			}

			emitted++
		}
	}

	return emitted
}

func subtract(left, right Point) Point {
	return Point{X: left.X - right.X, Y: left.Y - right.Y, Z: left.Z - right.Z}
}

func cross(left, right Point) Point {
	return Point{
		X: left.Y*right.Z - left.Z*right.Y,
		Y: left.Z*right.X - left.X*right.Z,
		Z: left.X*right.Y - left.Y*right.X,
	}
}

func projectPoint(point Point, missingAxis int) point2 {
	switch missingAxis {
	case 0:
		return point2{x: point.Y, y: point.Z}
	case 1:
		return point2{x: point.X, y: point.Z}
	default:
		return point2{x: point.X, y: point.Y}
	}
}

func solveMissing(origin, normal Point, missingAxis int, firstProjected, secondProjected float64) (float64, bool) {
	switch missingAxis {
	case 0:
		if math.Abs(normal.X) < 1e-12 {
			return 0, false
		}

		value := origin.X - (normal.Y*(firstProjected-origin.Y)+normal.Z*(secondProjected-origin.Z))/normal.X
		return value, true
	case 1:
		if math.Abs(normal.Y) < 1e-12 {
			return 0, false
		}

		value := origin.Y - (normal.X*(firstProjected-origin.X)+normal.Z*(secondProjected-origin.Z))/normal.Y
		return value, true
	default:
		if math.Abs(normal.Z) < 1e-12 {
			return 0, false
		}

		value := origin.Z - (normal.X*(firstProjected-origin.X)+normal.Y*(secondProjected-origin.Y))/normal.Z
		return value, true
	}
}

func triangleSquareOverlap(triangle [3]point2, centerX, centerY float64) bool {
	minimumX := centerX - 0.5
	maximumX := centerX + 0.5
	minimumY := centerY - 0.5
	maximumY := centerY + 0.5

	for _, vertex := range triangle {
		if vertex.x >= minimumX && vertex.x <= maximumX && vertex.y >= minimumY && vertex.y <= maximumY {
			return true
		}
	}

	corners := [4]point2{
		{x: minimumX, y: minimumY},
		{x: maximumX, y: minimumY},
		{x: maximumX, y: maximumY},
		{x: minimumX, y: maximumY},
	}

	for _, corner := range corners {
		if pointInTriangle(corner, triangle) {
			return true
		}
	}

	for edgeIndex := range 3 {
		first := triangle[edgeIndex]
		second := triangle[(edgeIndex+1)%3]

		for sideIndex := range 4 {
			firstCorner := corners[sideIndex]
			secondCorner := corners[(sideIndex+1)%4]

			if segmentsIntersect(first, second, firstCorner, secondCorner) {
				return true
			}
		}
	}

	return false
}

func pointInTriangle(point point2, triangle [3]point2) bool {
	first := signedArea(point, triangle[0], triangle[1])
	second := signedArea(point, triangle[1], triangle[2])
	third := signedArea(point, triangle[2], triangle[0])

	hasNegative := first < -1e-9 || second < -1e-9 || third < -1e-9
	hasPositive := first > 1e-9 || second > 1e-9 || third > 1e-9

	return !(hasNegative && hasPositive)
}

func signedArea(first, second, third point2) float64 {
	return (first.x-third.x)*(second.y-third.y) - (second.x-third.x)*(first.y-third.y)
}

func segmentsIntersect(firstStart, firstEnd, secondStart, secondEnd point2) bool {
	firstOrientation := orientation(firstStart, firstEnd, secondStart)
	secondOrientation := orientation(firstStart, firstEnd, secondEnd)
	thirdOrientation := orientation(secondStart, secondEnd, firstStart)
	fourthOrientation := orientation(secondStart, secondEnd, firstEnd)

	if firstOrientation*secondOrientation < 0 && thirdOrientation*fourthOrientation < 0 {
		return true
	}

	if math.Abs(firstOrientation) <= 1e-9 && pointOnSegment(secondStart, firstStart, firstEnd) {
		return true
	}

	if math.Abs(secondOrientation) <= 1e-9 && pointOnSegment(secondEnd, firstStart, firstEnd) {
		return true
	}

	if math.Abs(thirdOrientation) <= 1e-9 && pointOnSegment(firstStart, secondStart, secondEnd) {
		return true
	}

	return math.Abs(fourthOrientation) <= 1e-9 && pointOnSegment(firstEnd, secondStart, secondEnd)
}

func orientation(first, second, third point2) float64 {
	return (second.x-first.x)*(third.y-first.y) - (second.y-first.y)*(third.x-first.x)
}

func pointOnSegment(point, start, end point2) bool {
	return point.x >= min(start.x, end.x)-1e-9 && point.x <= max(start.x, end.x)+1e-9 &&
		point.y >= min(start.y, end.y)-1e-9 && point.y <= max(start.y, end.y)+1e-9
}
