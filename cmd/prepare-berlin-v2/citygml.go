package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/coalaura/minicraft/internal/berlinvoxel"
)

type surfaceKind uint8

const (
	surfaceNone surfaceKind = iota
	surfaceGround
	surfaceRoof
	surfaceWall
)

type gmlPolygon struct {
	Exterior gmlRingProperty   `xml:"exterior"`
	Interior []gmlRingProperty `xml:"interior"`
}

type gmlRingProperty struct {
	Ring gmlLinearRing `xml:"LinearRing"`
}

type gmlLinearRing struct {
	PosList gmlCoordinates   `xml:"posList"`
	Pos     []gmlCoordinates `xml:"pos"`
}

type gmlCoordinates struct {
	Dimension int    `xml:"srsDimension,attr"`
	Text      string `xml:",chardata"`
}

type polygon3 struct {
	Exterior []berlinvoxel.Point
	Interior [][]berlinvoxel.Point
}

func prepareCityGML(reader io.Reader, tiles *voxelTiles) error {
	decoder := xml.NewDecoder(reader)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return fmt.Errorf("decode CityGML: %w", err)
		}

		start, valid := token.(xml.StartElement)
		if !valid {
			continue
		}

		kind := kindForElement(start.Name.Local)
		if kind == surfaceNone {
			continue
		}

		err = prepareCityGMLSurface(decoder, start, kind, tiles)
		if err != nil {
			return err
		}
	}
}

func prepareCityGMLSurface(decoder *xml.Decoder, surface xml.StartElement, kind surfaceKind, tiles *voxelTiles) error {
	depth := 1

	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode CityGML %s: %w", surface.Name.Local, err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			depth++

			if element.Name.Local != "Polygon" {
				continue
			}

			encoded := gmlPolygon{}
			err = decoder.DecodeElement(&encoded, &element)
			if err != nil {
				return fmt.Errorf("decode CityGML polygon: %w", err)
			}

			depth--

			polygon, valid, err := decodePolygon(encoded)
			if err != nil {
				return err
			}

			if !valid {
				continue
			}

			err = rasterizeCityGMLPolygon(kind, polygon, tiles)
			if err != nil {
				return err
			}
		case xml.EndElement:
			depth--
		}
	}

	return nil
}

func kindForElement(localName string) surfaceKind {
	switch localName {
	case "GroundSurface", "OuterFloorSurface", "FloorSurface":
		return surfaceGround
	case "RoofSurface", "OuterCeilingSurface", "CeilingSurface":
		return surfaceRoof
	case "WallSurface", "ClosureSurface", "InteriorWallSurface":
		return surfaceWall
	default:
		return surfaceNone
	}
}

func decodePolygon(encoded gmlPolygon) (polygon3, bool, error) {
	exterior, err := decodeRing(encoded.Exterior.Ring)
	if err != nil {
		return polygon3{}, false, err
	}

	if len(exterior) < 3 {
		return polygon3{}, false, nil
	}

	polygon := polygon3{Exterior: exterior}

	for _, encodedInterior := range encoded.Interior {
		interior, err := decodeRing(encodedInterior.Ring)
		if err != nil {
			return polygon3{}, false, err
		}

		if len(interior) >= 3 {
			polygon.Interior = append(polygon.Interior, interior)
		}
	}

	return polygon, true, nil
}

func decodeRing(ring gmlLinearRing) ([]berlinvoxel.Point, error) {
	if strings.TrimSpace(ring.PosList.Text) != "" {
		dimension := ring.PosList.Dimension
		if dimension == 0 {
			dimension = 3
		}

		return decodeCoordinateSequence(ring.PosList.Text, dimension)
	}

	points := make([]berlinvoxel.Point, 0, len(ring.Pos))
	for _, encoded := range ring.Pos {
		dimension := encoded.Dimension
		if dimension == 0 {
			dimension = 3
		}

		position, err := decodeCoordinateSequence(encoded.Text, dimension)
		if err != nil {
			return nil, err
		}

		if len(position) != 1 {
			return nil, fmt.Errorf("CityGML gml:pos contains %d positions, want 1", len(position))
		}

		points = append(points, position[0])
	}

	return removeClosingPoint(points), nil
}

func decodeCoordinateSequence(text string, dimension int) ([]berlinvoxel.Point, error) {
	if dimension != 3 {
		return nil, fmt.Errorf("CityGML coordinate dimension %d is unsupported; expected 3", dimension)
	}

	fields := strings.Fields(text)
	if len(fields)%dimension != 0 {
		return nil, fmt.Errorf("CityGML coordinate list contains %d values, not divisible by %d", len(fields), dimension)
	}

	points := make([]berlinvoxel.Point, 0, len(fields)/dimension)

	for offset := 0; offset < len(fields); offset += dimension {
		x, err := strconv.ParseFloat(fields[offset], 64)
		if err != nil {
			return nil, fmt.Errorf("parse CityGML easting %q: %w", fields[offset], err)
		}

		y, err := strconv.ParseFloat(fields[offset+1], 64)
		if err != nil {
			return nil, fmt.Errorf("parse CityGML northing %q: %w", fields[offset+1], err)
		}

		z, err := strconv.ParseFloat(fields[offset+2], 64)
		if err != nil {
			return nil, fmt.Errorf("parse CityGML height %q: %w", fields[offset+2], err)
		}

		points = append(points, berlinvoxel.Point{X: x, Y: y, Z: z})
	}

	return removeClosingPoint(points), nil
}

func removeClosingPoint(points []berlinvoxel.Point) []berlinvoxel.Point {
	if len(points) < 2 {
		return points
	}

	first := points[0]
	last := points[len(points)-1]

	if math.Abs(first.X-last.X) <= 1e-7 && math.Abs(first.Y-last.Y) <= 1e-7 && math.Abs(first.Z-last.Z) <= 1e-7 {
		return points[:len(points)-1]
	}

	return points
}

func rasterizeCityGMLPolygon(kind surfaceKind, polygon polygon3, tiles *voxelTiles) error {
	if kind == surfaceGround {
		return rasterizeFootprint(polygon, tiles)
	}

	triangles := triangulatePolygon(polygon.Exterior)

	for _, triangle := range triangles {
		var emitErr error

		berlinvoxel.RasterizeTriangle(triangle[0], triangle[1], triangle[2], func(easting, northing int32, height int) {
			if emitErr != nil {
				return
			}

			if kind == surfaceRoof && !pointInPolygon(float64(easting), float64(northing), polygon) {
				return
			}

			_, emitErr = tiles.AddBlock(easting, northing, height)
		})

		if emitErr != nil {
			return emitErr
		}
	}

	return nil
}

func rasterizeFootprint(polygon polygon3, tiles *voxelTiles) error {
	minimumHeight, maximumHeight := polygonHeightRange(polygon)
	if !preparedHeightRangeIntersects(minimumHeight, maximumHeight) {
		return nil
	}

	minimumX := polygon.Exterior[0].X
	maximumX := minimumX
	minimumY := polygon.Exterior[0].Y
	maximumY := minimumY

	for _, point := range polygon.Exterior[1:] {
		minimumX = min(minimumX, point.X)
		maximumX = max(maximumX, point.X)
		minimumY = min(minimumY, point.Y)
		maximumY = max(maximumY, point.Y)
	}

	for northing := int(math.Floor(minimumY)); northing <= int(math.Ceil(maximumY)); northing++ {
		for easting := int(math.Floor(minimumX)); easting <= int(math.Ceil(maximumX)); easting++ {
			if !pointInPolygon(float64(easting), float64(northing), polygon) {
				continue
			}

			err := tiles.Mark(int32(easting), int32(northing))
			if err != nil {
				return err
			}
		}
	}

	err := markRingBoundary(polygon.Exterior, tiles)
	if err != nil {
		return err
	}

	for _, interior := range polygon.Interior {
		err = markRingBoundary(interior, tiles)
		if err != nil {
			return err
		}
	}

	return nil
}

func polygonHeightRange(polygon polygon3) (float64, float64) {
	minimum := polygon.Exterior[0].Z
	maximum := minimum

	for _, point := range polygon.Exterior[1:] {
		minimum = min(minimum, point.Z)
		maximum = max(maximum, point.Z)
	}

	for _, ring := range polygon.Interior {
		for _, point := range ring {
			minimum = min(minimum, point.Z)
			maximum = max(maximum, point.Z)
		}
	}

	return minimum, maximum
}

func markRingBoundary(ring []berlinvoxel.Point, tiles *voxelTiles) error {
	for index, start := range ring {
		end := ring[(index+1)%len(ring)]
		distance := math.Hypot(end.X-start.X, end.Y-start.Y)
		steps := max(1, int(math.Ceil(distance*2)))

		for step := 0; step <= steps; step++ {
			ratio := float64(step) / float64(steps)
			easting := int32(math.Round(start.X + (end.X-start.X)*ratio))
			northing := int32(math.Round(start.Y + (end.Y-start.Y)*ratio))

			err := tiles.Mark(easting, northing)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func pointInPolygon(x, y float64, polygon polygon3) bool {
	if !pointInRing(x, y, polygon.Exterior) {
		return false
	}

	for _, interior := range polygon.Interior {
		if pointInRing(x, y, interior) {
			return false
		}
	}

	return true
}

func pointInRing(x, y float64, ring []berlinvoxel.Point) bool {
	inside := false
	previous := ring[len(ring)-1]

	for _, current := range ring {
		intersects := (current.Y > y) != (previous.Y > y)
		if intersects {
			intersectionX := (previous.X-current.X)*(y-current.Y)/(previous.Y-current.Y) + current.X
			if x < intersectionX {
				inside = !inside
			}
		}

		previous = current
	}

	return inside
}

func triangulatePolygon(points []berlinvoxel.Point) [][3]berlinvoxel.Point {
	if len(points) < 3 {
		return nil
	}

	if len(points) == 3 {
		return [][3]berlinvoxel.Point{{points[0], points[1], points[2]}}
	}

	projectionAxis := dominantProjectionAxis(points)
	projected := make([]point2D, len(points))
	for index, point := range points {
		projected[index] = projectPolygonPoint(point, projectionAxis)
	}

	orientation := polygonArea(projected)
	indices := make([]int, len(points))
	for index := range indices {
		indices[index] = index
	}

	triangles := make([][3]berlinvoxel.Point, 0, len(points)-2)
	guard := len(points) * len(points)

	for len(indices) > 3 && guard > 0 {
		guard--
		foundEar := false

		for index := range indices {
			previousIndex := indices[(index+len(indices)-1)%len(indices)]
			currentIndex := indices[index]
			nextIndex := indices[(index+1)%len(indices)]

			if !convexCorner(projected[previousIndex], projected[currentIndex], projected[nextIndex], orientation) {
				continue
			}

			triangle := [3]point2D{projected[previousIndex], projected[currentIndex], projected[nextIndex]}
			containsPoint := false

			for _, candidateIndex := range indices {
				if candidateIndex == previousIndex || candidateIndex == currentIndex || candidateIndex == nextIndex {
					continue
				}

				if pointInTriangle2D(projected[candidateIndex], triangle) {
					containsPoint = true
					break
				}
			}

			if containsPoint {
				continue
			}

			triangles = append(triangles, [3]berlinvoxel.Point{points[previousIndex], points[currentIndex], points[nextIndex]})
			indices = append(indices[:index], indices[index+1:]...)
			foundEar = true
			break
		}

		if !foundEar {
			break
		}
	}

	if len(indices) == 3 {
		triangles = append(triangles, [3]berlinvoxel.Point{points[indices[0]], points[indices[1]], points[indices[2]]})
	}

	if len(triangles) == len(points)-2 {
		return triangles
	}

	triangles = triangles[:0]
	for index := 1; index+1 < len(points); index++ {
		triangles = append(triangles, [3]berlinvoxel.Point{points[0], points[index], points[index+1]})
	}

	return triangles
}

type point2D struct {
	x float64
	y float64
}

func dominantProjectionAxis(points []berlinvoxel.Point) int {
	normal := berlinvoxel.Point{}

	for index, current := range points {
		next := points[(index+1)%len(points)]
		normal.X += (current.Y - next.Y) * (current.Z + next.Z)
		normal.Y += (current.Z - next.Z) * (current.X + next.X)
		normal.Z += (current.X - next.X) * (current.Y + next.Y)
	}

	absoluteX := math.Abs(normal.X)
	absoluteY := math.Abs(normal.Y)
	absoluteZ := math.Abs(normal.Z)

	if absoluteX >= absoluteY && absoluteX >= absoluteZ {
		return 0
	}

	if absoluteY >= absoluteZ {
		return 1
	}

	return 2
}

func projectPolygonPoint(point berlinvoxel.Point, axis int) point2D {
	switch axis {
	case 0:
		return point2D{x: point.Y, y: point.Z}
	case 1:
		return point2D{x: point.X, y: point.Z}
	default:
		return point2D{x: point.X, y: point.Y}
	}
}

func polygonArea(points []point2D) float64 {
	area := 0.0
	for index, point := range points {
		next := points[(index+1)%len(points)]
		area += point.x*next.y - next.x*point.y
	}

	return area / 2
}

func convexCorner(previous, current, next point2D, orientation float64) bool {
	cross := (current.x-previous.x)*(next.y-current.y) - (current.y-previous.y)*(next.x-current.x)

	if orientation >= 0 {
		return cross > 1e-10
	}

	return cross < -1e-10
}

func pointInTriangle2D(point point2D, triangle [3]point2D) bool {
	first := triangleSign(point, triangle[0], triangle[1])
	second := triangleSign(point, triangle[1], triangle[2])
	third := triangleSign(point, triangle[2], triangle[0])

	hasNegative := first < -1e-10 || second < -1e-10 || third < -1e-10
	hasPositive := first > 1e-10 || second > 1e-10 || third > 1e-10

	return !(hasNegative && hasPositive)
}

func triangleSign(point, first, second point2D) float64 {
	return (point.x-second.x)*(first.y-second.y) - (first.x-second.x)*(point.y-second.y)
}
