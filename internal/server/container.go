package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	containerInteractionRange = 4.5
	containerValidityPadding  = 4.0
)

func containerBlockEntityStillValid(runtime *Runtime, session *Session, expected RuntimeBlockEntity) bool {
	position := expected.BlockPosition()
	block := runtime.World.BlockAt(position)

	entity, present := runtime.authoritativeRuntimeBlockEntityAt(position, block)
	if !present || entity != expected {
		return false
	}

	player := session.snapshotPlayer()
	return containerWithinRange(player, position)
}

func containerWithinRange(player game.Player, position game.BlockPosition) bool {
	eye := player.EyePosition()

	distanceX := eye.X - math.Max(float64(position.X), math.Min(eye.X, float64(position.X+1)))
	distanceY := eye.Y - math.Max(float64(position.Y), math.Min(eye.Y, float64(position.Y+1)))
	distanceZ := eye.Z - math.Max(float64(position.Z), math.Min(eye.Z, float64(position.Z+1)))

	maximumDistance := containerInteractionRange + containerValidityPadding
	return distanceX*distanceX+distanceY*distanceY+distanceZ*distanceZ < maximumDistance*maximumDistance
}
