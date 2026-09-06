package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

type worldSegmentBlockHit struct {
	Fraction      float64
	Position      game.Position
	BlockPosition game.BlockPosition
	Block         game.Block
}

func (r *Runtime) sweepWorldBlockSegment(from, to game.Position) (worldSegmentBlockHit, bool) {
	deltaX := to.X - from.X
	deltaY := to.Y - from.Y
	deltaZ := to.Z - from.Z

	minimumX := int32(math.Floor(min(from.X, to.X)))
	minimumY := int32(math.Floor(min(from.Y, to.Y)))
	minimumZ := int32(math.Floor(min(from.Z, to.Z)))
	maximumX := int32(math.Floor(max(from.X, to.X)))
	maximumY := int32(math.Floor(max(from.Y, to.Y)))
	maximumZ := int32(math.Floor(max(from.Z, to.Z)))

	nearestFraction := math.Inf(1)
	nearest := worldSegmentBlockHit{}

	for y := minimumY; y <= maximumY; y++ {
		for x := minimumX; x <= maximumX; x++ {
			for z := minimumZ; z <= maximumZ; z++ {
				blockPosition := game.BlockPosition{X: x, Y: y, Z: z}

				block := r.World.BlockAt(blockPosition)

				for _, box := range block.CollisionBoxes(blockPosition) {
					fraction, _, intersects := raycastAABB(from, deltaX, deltaY, deltaZ, box)
					if !intersects || fraction < 0 || fraction > 1 || fraction >= nearestFraction {
						continue
					}

					nearestFraction = fraction

					nearest = worldSegmentBlockHit{
						Fraction: fraction,
						Position: game.Position{
							X: from.X + deltaX*fraction,
							Y: from.Y + deltaY*fraction,
							Z: from.Z + deltaZ*fraction,
						},
						BlockPosition: blockPosition,
						Block:         block,
					}
				}
			}
		}
	}

	return nearest, nearestFraction != math.Inf(1)
}
