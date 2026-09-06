package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

type runtimeLivingTarget struct {
	session *Session
	entity  RuntimeLivingEntity
}

type runtimeLivingEyeHeight interface {
	RuntimeLivingEyeHeight() float64
}

func (target runtimeLivingTarget) present() bool {
	return target.session != nil || target.entity != nil
}

func (target runtimeLivingTarget) position() game.Position {
	if target.session != nil {
		return target.session.snapshotPlayer().Position
	}

	if target.entity != nil {
		state := target.entity.RuntimeEntityState()

		state.mu.RLock()
		defer state.mu.RUnlock()

		return state.Position
	}

	return game.Position{}
}

func (target runtimeLivingTarget) height() float64 {
	if target.session != nil {
		return 1.8
	}

	if target.entity != nil {
		return target.entity.RuntimeLivingState().Height
	}

	return 0
}

func (target runtimeLivingTarget) eyePosition() game.Position {
	position := target.position()

	if target.session != nil {
		position.Y += 1.62
	} else if provider, available := target.entity.(runtimeLivingEyeHeight); available {
		position.Y += provider.RuntimeLivingEyeHeight()
	} else {
		position.Y += target.height() * 0.85
	}

	return position
}

func (target runtimeLivingTarget) collisionBox() game.AABB {
	if target.session != nil {
		return target.session.snapshotPlayer().CollisionBox()
	}

	if target.entity != nil {
		position := target.position()

		return target.entity.RuntimeLivingState().CollisionBox(position)
	}

	return game.AABB{}
}

func (target runtimeLivingTarget) valid(runtime *Runtime, position game.Position, maximumDistance float64) bool {
	if target.session != nil {
		return runtime.playerTargetValid(target.session, position, maximumDistance)
	}

	if target.entity == nil {
		return false
	}

	state := target.entity.RuntimeEntityState()
	living := target.entity.RuntimeLivingState()

	state.mu.RLock()
	defer state.mu.RUnlock()

	return !state.Removed && !living.Dead && distanceSquared(position, state.Position) <= maximumDistance*maximumDistance
}

func (target runtimeLivingTarget) hasLineOfSight(runtime *Runtime, position game.Position, eyeHeight float64) bool {
	position.Y += eyeHeight

	return runtime.livingTargetHasLineOfSight(position, target.eyePosition())
}

func (r *Runtime) runtimeLivingTargetByID(id int32) runtimeLivingTarget {
	for _, session := range r.snapshotSessions() {
		if session.snapshotPlayer().EntityID == id {
			return runtimeLivingTarget{session: session}
		}
	}

	r.entityMu.RLock()
	entity := r.entities[id]
	r.entityMu.RUnlock()

	living, isLiving := entity.(RuntimeLivingEntity)
	if !isLiving {
		return runtimeLivingTarget{}
	}

	return runtimeLivingTarget{entity: living}
}

func (r *Runtime) nearestValidPlayer(position game.Position, eyeHeight, maximumDistance float64) *Session {
	var nearest *Session

	nearestDistance := maximumDistance * maximumDistance

	for _, session := range r.snapshotSessions() {
		player := session.snapshotPlayer()
		if !playerTargetValid(player, position, maximumDistance) || !r.playerTargetHasLineOfSight(position, eyeHeight, player.Position) {
			continue
		}

		distance := distanceSquared(position, player.Position)
		if distance < nearestDistance {
			nearest = session
			nearestDistance = distance
		}
	}

	return nearest
}

func (r *Runtime) playerTargetValid(session *Session, position game.Position, maximumDistance float64) bool {
	player := session.snapshotPlayer()

	return slices.Contains(r.snapshotSessions(), session) && playerTargetValid(player, position, maximumDistance)
}

func (r *Runtime) playerTargetHasLineOfSight(from game.Position, eyeHeight float64, to game.Position) bool {
	from.Y += eyeHeight
	to.Y += 1.62

	return r.livingTargetHasLineOfSight(from, to)
}

func (r *Runtime) livingTargetHasLineOfSight(from, to game.Position) bool {
	deltaX := to.X - from.X
	deltaY := to.Y - from.Y
	deltaZ := to.Z - from.Z

	minimumX := int32(math.Floor(min(from.X, to.X)))
	minimumY := int32(math.Floor(min(from.Y, to.Y)))
	minimumZ := int32(math.Floor(min(from.Z, to.Z)))
	maximumX := int32(math.Floor(max(from.X, to.X)))
	maximumY := int32(math.Floor(max(from.Y, to.Y)))
	maximumZ := int32(math.Floor(max(from.Z, to.Z)))

	for x := minimumX; x <= maximumX; x++ {
		for y := minimumY; y <= maximumY; y++ {
			for z := minimumZ; z <= maximumZ; z++ {
				blockPosition := game.BlockPosition{X: x, Y: y, Z: z}
				block := r.World.BlockAt(blockPosition)

				for _, box := range block.CollisionBoxes(blockPosition) {
					distance, _, intersects := raycastAABB(from, deltaX, deltaY, deltaZ, box)
					if intersects && distance >= 0 && distance <= 1 {
						return false
					}
				}
			}
		}
	}

	return true
}

func playerTargetValid(player game.Player, position game.Position, maximumDistance float64) bool {
	if player.Dead || player.GameMode == game.GameModeCreative || player.GameMode == game.GameModeSpectator {
		return false
	}

	return distanceSquared(position, player.Position) <= maximumDistance*maximumDistance
}
