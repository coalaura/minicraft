package server

import "github.com/coalaura/minicraft/internal/game"

type freeFlightMovement struct {
	Position             game.Position
	OnGround             bool
	HorizontalCollisionX bool
	HorizontalCollisionZ bool
	VerticalCollision    bool
}

func (runtime *Runtime) moveFreeFlightEntity(position game.Position, velocity game.Velocity, width, height float64) freeFlightMovement {
	box := entityBox(position, width, height)

	var blockBuffer [64]game.AABB

	blocks := runtime.appendEntityCollisionBoxes(blockBuffer[:0], box, velocity)

	delta := collideAABBWithBlocks(box, blocks, velocity)

	horizontalCollisionX := delta.X != velocity.X
	horizontalCollisionZ := delta.Z != velocity.Z
	verticalCollision := delta.Y != velocity.Y

	position.X += delta.X
	position.Y += delta.Y
	position.Z += delta.Z

	return freeFlightMovement{
		Position:             position,
		OnGround:             velocity.Y < 0 && verticalCollision,
		HorizontalCollisionX: horizontalCollisionX,
		HorizontalCollisionZ: horizontalCollisionZ,
		VerticalCollision:    verticalCollision,
	}
}
