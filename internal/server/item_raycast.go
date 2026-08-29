package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	itemUseRange = 4.5
	worldMinY    = -64
	worldMaxY    = 319
)

type itemRaycastHit struct {
	position game.BlockPosition
	face     int32
}

func (r *Runtime) raycastItemGrid(player game.Player, maximumDistance float64, hit func(game.Block) bool) (itemRaycastHit, bool) {
	origin := player.EyePosition()

	yaw := float64(player.Rotation.Yaw) * math.Pi / 180
	pitch := float64(player.Rotation.Pitch) * math.Pi / 180

	directionX := -math.Sin(yaw) * math.Cos(pitch)
	directionY := -math.Sin(pitch)
	directionZ := math.Cos(yaw) * math.Cos(pitch)

	position := game.BlockPosition{X: int32(math.Floor(origin.X)), Y: int32(math.Floor(origin.Y)), Z: int32(math.Floor(origin.Z))}

	stepX, distanceX, deltaX := rayStep(origin.X, directionX)
	stepY, distanceY, deltaY := rayStep(origin.Y, directionY)
	stepZ, distanceZ, deltaZ := rayStep(origin.Z, directionZ)

	for distance := float64(0); distance <= maximumDistance; {
		if worldPositionValid(position) {
			block := r.World.BlockAt(position)
			if hit(block) {
				boxes := block.CollisionBoxes(position)
				if len(boxes) == 0 && block.FluidState().IsSource() {
					boxes = []game.AABB{fluidRaycastBox(r.World, block, position)}
				}

				for _, box := range boxes {
					hitDistance, face, intersects := raycastAABB(origin, directionX, directionY, directionZ, box)
					if intersects && hitDistance <= maximumDistance {
						return itemRaycastHit{position: position, face: face}, true
					}
				}
			}
		}

		if distanceX < distanceY && distanceX < distanceZ {
			position.X += stepX
			distance = distanceX
			distanceX += deltaX

			continue
		}

		if distanceY < distanceZ {
			position.Y += stepY
			distance = distanceY
			distanceY += deltaY

			continue
		}

		position.Z += stepZ
		distance = distanceZ
		distanceZ += deltaZ
	}

	return itemRaycastHit{}, false
}

func rayStep(origin, direction float64) (int32, float64, float64) {
	if direction > 0 {
		return 1, (math.Floor(origin) + 1 - origin) / direction, 1 / direction
	}

	if direction < 0 {
		return -1, (origin - math.Floor(origin)) / -direction, -1 / direction
	}

	return 0, math.Inf(1), math.Inf(1)
}

func fluidRaycastBox(world *game.World, block game.Block, position game.BlockPosition) game.AABB {
	state := block.FluidState()
	height := state.Height(world, position)

	return game.AABB{
		MinX: float64(position.X),
		MinY: float64(position.Y),
		MinZ: float64(position.Z),
		MaxX: float64(position.X + 1),
		MaxY: float64(position.Y) + height,
		MaxZ: float64(position.Z + 1),
	}
}

func raycastAABB(origin game.Position, directionX, directionY, directionZ float64, box game.AABB) (float64, int32, bool) {
	entryDistance := math.Inf(-1)
	exitDistance := math.Inf(1)
	face := int32(-1)

	entryDistance, exitDistance, face, valid := raycastAABBAxis(origin.X, directionX, box.MinX, box.MaxX, 4, 5, entryDistance, exitDistance, face)
	if !valid {
		return 0, -1, false
	}

	entryDistance, exitDistance, face, valid = raycastAABBAxis(origin.Y, directionY, box.MinY, box.MaxY, 0, 1, entryDistance, exitDistance, face)
	if !valid {
		return 0, -1, false
	}

	entryDistance, exitDistance, face, valid = raycastAABBAxis(origin.Z, directionZ, box.MinZ, box.MaxZ, 2, 3, entryDistance, exitDistance, face)
	if !valid || exitDistance < 0 {
		return 0, -1, false
	}

	if entryDistance < 0 {
		return 0, -1, true
	}

	return entryDistance, face, true
}

func raycastAABBAxis(origin, direction, minimum, maximum float64, negativeFace, positiveFace int32, entryDistance, exitDistance float64, face int32) (float64, float64, int32, bool) {
	if direction == 0 {
		return entryDistance, exitDistance, face, origin >= minimum && origin <= maximum
	}

	minimumDistance := (minimum - origin) / direction
	maximumDistance := (maximum - origin) / direction

	entryFace := negativeFace

	if minimumDistance > maximumDistance {
		distance := minimumDistance
		minimumDistance = maximumDistance
		maximumDistance = distance
		entryFace = positiveFace
	}

	if minimumDistance > entryDistance {
		entryDistance = minimumDistance
		face = entryFace
	}

	if maximumDistance < exitDistance {
		exitDistance = maximumDistance
	}

	return entryDistance, exitDistance, face, entryDistance <= exitDistance
}

func worldPositionValid(position game.BlockPosition) bool {
	return position.Y >= worldMinY && position.Y <= worldMaxY
}
