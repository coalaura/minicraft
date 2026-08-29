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

func (r *Runtime) fluidContact(box game.AABB, fluidType game.FluidType) entityFluidContact {
	minX := int32(math.Floor(box.MinX))
	minY := int32(math.Floor(box.MinY))
	minZ := int32(math.Floor(box.MinZ))
	maxX := int32(math.Floor(box.MaxX - 1e-7))
	maxY := int32(math.Floor(box.MaxY - 1e-7))
	maxZ := int32(math.Floor(box.MaxZ - 1e-7))

	var (
		contact     entityFluidContact
		flowSamples int
	)

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
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

				contact.Flow.X += flow.X
				contact.Flow.Z += flow.Z

				flowSamples++
			}
		}
	}

	if flowSamples == 0 {
		return contact
	}

	contact.Flow.X /= float64(flowSamples)
	contact.Flow.Z /= float64(flowSamples)

	length := math.Hypot(contact.Flow.X, contact.Flow.Z)
	if length != 0 {
		contact.Flow.X /= length
		contact.Flow.Z /= length
	}

	return contact
}

func (r *Runtime) fluidFlowVector(position game.BlockPosition, state game.FluidState) game.Velocity {
	fluidHeight := state.Height(r.World, position)

	block := r.World.BlockAt(position)

	var flow game.Velocity

	for _, direction := range entityFluidFlowDirections {
		neighborPosition := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}

		neighborState := r.World.FluidAt(neighborPosition)

		if state.SameFamily(neighborState) {
			heightDifference := fluidHeight - neighborState.Height(r.World, neighborPosition)

			flow.X += float64(direction.X) * heightDifference
			flow.Z += float64(direction.Z) * heightDifference

			continue
		}

		if !neighborState.Empty() || game.CombinedFaceOccludes(block, r.World.BlockAt(neighborPosition), direction.Face) {
			continue
		}

		below := neighborPosition

		below.Y--

		if state.SameFamily(r.World.FluidAt(below)) {
			flow.X += float64(direction.X) * fluidHeight
			flow.Z += float64(direction.Z) * fluidHeight
		}
	}

	return flow
}
