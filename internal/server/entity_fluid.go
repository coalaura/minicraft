package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const fluidContactDepth = 0.1

type entityFluidContact struct {
	Depth float64
	Flow  game.Velocity
}

type fluidFlowDirection struct {
	X    int32
	Z    int32
	Face game.BlockFace
}

var entityFluidFlowDirections = [...]fluidFlowDirection{
	{X: -1, Face: game.BlockFaceWest},
	{X: 1, Face: game.BlockFaceEast},
	{Z: -1, Face: game.BlockFaceNorth},
	{Z: 1, Face: game.BlockFaceSouth},
}

func (r *Runtime) fluidContact(box game.AABB, fluidType game.FluidType, player bool) entityFluidContact {
	box.MinX += 0.001
	box.MinY += 0.001
	box.MinZ += 0.001
	box.MaxX -= 0.001
	box.MaxY -= 0.001
	box.MaxZ -= 0.001

	minX := int32(math.Floor(box.MinX))
	minY := int32(math.Floor(box.MinY))
	minZ := int32(math.Floor(box.MinZ))
	maxX := int32(math.Ceil(box.MaxX))
	maxY := int32(math.Ceil(box.MaxY))
	maxZ := int32(math.Ceil(box.MaxZ))

	var (
		contact     entityFluidContact
		flowSamples int
	)

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			for z := minZ; z < maxZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}

				state := r.World.FluidAt(position)
				if state.Type() != fluidType {
					continue
				}

				depth := float64(y) + state.Height(r.World, position) - box.MinY
				if depth <= 0 {
					continue
				}

				contact.Depth = max(contact.Depth, depth)

				flow := r.fluidFlowVector(position, state)

				if depth < 0.4 {
					flow.X *= depth
					flow.Y *= depth
					flow.Z *= depth
				}

				contact.Flow.X += flow.X
				contact.Flow.Y += flow.Y
				contact.Flow.Z += flow.Z
				flowSamples++
			}
		}
	}

	if flowSamples == 0 {
		return contact
	}

	contact.Flow.X /= float64(flowSamples)
	contact.Flow.Y /= float64(flowSamples)
	contact.Flow.Z /= float64(flowSamples)

	length := velocityLength(contact.Flow)
	if !player && length != 0 {
		contact.Flow.X /= length
		contact.Flow.Y /= length
		contact.Flow.Z /= length
	}

	return contact
}

func (r *Runtime) fluidFlowVector(position game.BlockPosition, state game.FluidState) game.Velocity {
	fluidHeight := state.OwnHeight()

	var flow game.Velocity

	for _, direction := range entityFluidFlowDirections {
		neighborPosition := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}
		neighborState := r.World.FluidAt(neighborPosition)
		neighborHeight := 0.0

		if state.SameFamily(neighborState) {
			neighborHeight = neighborState.OwnHeight()
		}

		if neighborHeight == 0 {
			neighborBlock := r.World.BlockAt(neighborPosition)
			if len(neighborBlock.CollisionBoxes(neighborPosition)) != 0 {
				continue
			}

			below := neighborPosition
			below.Y--

			belowState := r.World.FluidAt(below)
			if state.SameFamily(belowState) {
				neighborHeight = belowState.OwnHeight() - 0.8888889
			}
		}

		if neighborHeight == 0 {
			continue
		}

		heightDifference := fluidHeight - neighborHeight

		flow.X += float64(direction.X) * heightDifference
		flow.Z += float64(direction.Z) * heightDifference
	}

	if state.Falling() && fallingFluidBesideOccludingFace(r.World, position) {
		length := math.Hypot(flow.X, flow.Z)
		if length != 0 {
			flow.X /= length
			flow.Z /= length
		}

		flow.Y -= 6

		length = velocityLength(flow)
		if length != 0 {
			flow.X /= length
			flow.Y /= length
			flow.Z /= length
		}
	}

	return flow
}

func fallingFluidBesideOccludingFace(world *game.World, position game.BlockPosition) bool {
	block := world.BlockAt(position)

	above := position

	above.Y++

	aboveBlock := world.BlockAt(above)

	for _, direction := range entityFluidFlowDirections {
		neighbor := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}
		if game.CombinedFaceOccludes(block, world.BlockAt(neighbor), direction.Face) {
			return true
		}

		neighbor.Y++

		if game.CombinedFaceOccludes(aboveBlock, world.BlockAt(neighbor), direction.Face) {
			return true
		}
	}

	return false
}

func fluidCurrentImpulse(current, flow game.Velocity, scale float64) game.Velocity {
	impulse := game.Velocity{X: flow.X * scale, Y: flow.Y * scale, Z: flow.Z * scale}
	if math.Abs(current.X) >= 0.003 || math.Abs(current.Z) >= 0.003 || velocityLength(impulse) >= 0.0045 {
		return impulse
	}

	length := velocityLength(impulse)
	if length == 0 {
		return impulse
	}

	impulse.X = impulse.X / length * 0.0045
	impulse.Y = impulse.Y / length * 0.0045
	impulse.Z = impulse.Z / length * 0.0045

	return impulse
}

func velocityLength(velocity game.Velocity) float64 {
	return math.Sqrt(velocity.X*velocity.X + velocity.Y*velocity.Y + velocity.Z*velocity.Z)
}
