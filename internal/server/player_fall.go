package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	playerLandingOrdinary playerLandingBehavior = iota
	playerLandingHay
	playerLandingHoney
	playerLandingBed
	playerLandingSlime
	playerLandingPowderSnow
)

const (
	fallResetTraceMaximum  = 8.0
	landingPositionEpsilon = 1e-5
)

type playerLandingBehavior uint8

func (r *Runtime) playerMovementResetsFallDistance(previous, current game.Player, inWater bool) bool {
	if inWater || r.playerTouchesFallDamageResettingBlock(current) {
		return true
	}

	deltaX := current.Position.X - previous.Position.X
	deltaY := current.Position.Y - previous.Position.Y
	deltaZ := current.Position.Z - previous.Position.Z

	distanceSquared := deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ
	if previous.FallDistance == 0 || distanceSquared < 1 {
		return false
	}

	distance := math.Sqrt(distanceSquared)
	traceDistance := min(distance, fallResetTraceMaximum)

	scale := traceDistance / distance

	end := game.Position{
		X: previous.Position.X + deltaX*scale,
		Y: previous.Position.Y + deltaY*scale,
		Z: previous.Position.Z + deltaZ*scale,
	}

	return r.fallResetTraceHits(previous.Position, end)
}

func (r *Runtime) playerTouchesFallDamageResettingBlock(player game.Player) bool {
	position := game.BlockPosition{
		X: int32(math.Floor(player.Position.X)),
		Y: int32(math.Floor(player.Position.Y)),
		Z: int32(math.Floor(player.Position.Z)),
	}

	return r.World.BlockAt(position).HasTrait(game.BlockTraitFallDamageResetting)
}

func (r *Runtime) fallResetTraceHits(start, end game.Position) bool {
	minX := int32(math.Floor(min(start.X, end.X)))
	minY := int32(math.Floor(min(start.Y, end.Y)))
	minZ := int32(math.Floor(min(start.Z, end.Z)))
	maxX := int32(math.Floor(max(start.X, end.X)))
	maxY := int32(math.Floor(max(start.Y, end.Y)))
	maxZ := int32(math.Floor(max(start.Z, end.Z)))

	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			for z := minZ; z <= maxZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}
				block := r.World.BlockAt(position)

				if block.HasTrait(game.BlockTraitFallDamageResetting) {
					box := game.AABB{
						MinX: float64(x),
						MinY: float64(y),
						MinZ: float64(z),
						MaxX: float64(x + 1),
						MaxY: float64(y + 1),
						MaxZ: float64(z + 1),
					}

					if segmentIntersectsBox(start, end, box) {
						return true
					}
				}

				fluid := r.World.FluidAt(position)
				if fluid.Type() != game.FluidTypeWater {
					continue
				}

				box := game.AABB{
					MinX: float64(x),
					MinY: float64(y),
					MinZ: float64(z),
					MaxX: float64(x + 1),
					MaxY: float64(y) + fluid.Height(r.World, position),
					MaxZ: float64(z + 1),
				}

				if segmentIntersectsBox(start, end, box) {
					return true
				}
			}
		}
	}

	return false
}

func (r *Runtime) playerLandingBlock(player game.Player) (game.Block, bool) {
	box := player.CollisionBox()

	feetY := player.Position.Y

	minX := int32(math.Floor(box.MinX))
	minZ := int32(math.Floor(box.MinZ))
	maxX := int32(math.Ceil(box.MaxX))
	maxZ := int32(math.Ceil(box.MaxZ))
	minY := int32(math.Floor(feetY)) - 1
	maxY := int32(math.Floor(feetY))

	var (
		landedBlock game.Block
		landedTop   = math.Inf(-1)
		found       bool
	)

	for y := minY; y <= maxY; y++ {
		for x := minX; x < maxX; x++ {
			for z := minZ; z < maxZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}
				block := r.World.BlockAt(position)

				for _, collision := range block.CollisionBoxes(position) {
					if math.Abs(collision.MaxY-feetY) > landingPositionEpsilon {
						continue
					}

					if collision.MaxX <= box.MinX || collision.MinX >= box.MaxX || collision.MaxZ <= box.MinZ || collision.MinZ >= box.MaxZ {
						continue
					}

					if collision.MaxY > landedTop {
						landedBlock = block
						landedTop = collision.MaxY
						found = true
					}
				}
			}
		}
	}

	if found {
		return landedBlock, true
	}

	powderPosition := game.BlockPosition{
		X: int32(math.Floor(player.Position.X)),
		Y: int32(math.Floor(feetY)),
		Z: int32(math.Floor(player.Position.Z)),
	}

	block := r.World.BlockAt(powderPosition)
	if block == game.PowderSnow {
		return block, true
	}

	powderPosition.Y--

	block = r.World.BlockAt(powderPosition)

	return block, block == game.PowderSnow
}

func playerLandingBehaviorForBlock(block game.Block) playerLandingBehavior {
	if block.HasTrait(game.BlockTraitBed) {
		return playerLandingBed
	}

	switch block {
	case game.HayBlock:
		return playerLandingHay
	case game.HoneyBlock:
		return playerLandingHoney
	case game.SlimeBlock:
		return playerLandingSlime
	case game.PowderSnow:
		return playerLandingPowderSnow
	default:
		return playerLandingOrdinary
	}
}

func playerLandingDamage(behavior playerLandingBehavior, fallDistance float32, suppressBounce bool) float32 {
	switch behavior {
	case playerLandingHay, playerLandingHoney:
		return calculatePlayerFallDamageWithMultiplier(fallDistance, 0.2)
	case playerLandingBed:
		return calculatePlayerFallDamage(fallDistance * 0.5)
	case playerLandingSlime:
		return 0
	case playerLandingPowderSnow:
		return 0
	}

	return calculatePlayerFallDamage(fallDistance)
}

func applyPlayerLandingVelocity(player *game.Player, behavior playerLandingBehavior) {
	if player.Velocity.Y >= 0 {
		return
	}

	switch behavior {
	case playerLandingBed:
		if player.Sneaking {
			player.Velocity.Y = 0
		} else {
			player.Velocity.Y *= -0.66
		}
	case playerLandingSlime:
		if player.Sneaking {
			player.Velocity.Y = 0
		} else {
			player.Velocity.Y = -player.Velocity.Y
		}
	default:
		player.Velocity.Y = 0
	}
}

func calculatePlayerFallDamageWithMultiplier(fallDistance, multiplier float32) float32 {
	unsafeDistance := fallDistance - playerSafeFallDistance
	if unsafeDistance <= 0 {
		return 0
	}

	return float32(math.Ceil(float64(unsafeDistance * multiplier)))
}

func segmentIntersectsBox(start, end game.Position, box game.AABB) bool {
	minimum := 0.0
	maximum := 1.0

	if !clipSegmentAxis(start.X, end.X-start.X, box.MinX, box.MaxX, &minimum, &maximum) {
		return false
	}

	if !clipSegmentAxis(start.Y, end.Y-start.Y, box.MinY, box.MaxY, &minimum, &maximum) {
		return false
	}

	return clipSegmentAxis(start.Z, end.Z-start.Z, box.MinZ, box.MaxZ, &minimum, &maximum)
}

func clipSegmentAxis(start, delta, boxMin, boxMax float64, minimum, maximum *float64) bool {
	if delta == 0 {
		return start >= boxMin && start <= boxMax
	}

	first := (boxMin - start) / delta
	second := (boxMax - start) / delta

	if first > second {
		first, second = second, first
	}

	*minimum = max(*minimum, first)
	*maximum = min(*maximum, second)

	return *minimum <= *maximum
}
