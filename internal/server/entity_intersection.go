package server

import "github.com/coalaura/minicraft/internal/game"

// entityIntersectionObstructed checks the entities that prevent an overlapping block or spawn volume.
func (r *Runtime) entityIntersectionObstructed(box game.AABB) bool {
	for _, session := range r.sessionView() {
		player := session.playerView()
		if player.GameMode != game.GameModeSpectator && box.Intersects(player.collisionBox()) {
			return true
		}
	}

	iterator := r.runtimeEntitiesInBox(box)

	for {
		entity, found := iterator.Next()
		if !found {
			return false
		}

		state := entity.RuntimeEntityState()

		state.mu.RLock()

		if state.Removed {
			state.mu.RUnlock()

			continue
		}

		entityBox, checked := runtimeEntityIntersectionBoxLocked(entity, state.Position)
		intersects := checked && box.Intersects(entityBox)

		state.mu.RUnlock()

		if intersects {
			return true
		}
	}
}

// runtimeEntityIntersectionBoxLocked reads mutable living dimensions under the entity state lock.
func runtimeEntityIntersectionBoxLocked(entity RuntimeEntity, position game.Position) (game.AABB, bool) {
	switch entity := entity.(type) {
	case RuntimeLivingEntity:
		return entity.RuntimeLivingState().CollisionBox(position), true
	case *runtimeBoatEntity, *runtimeTntEntity, *runtimeFallingBlockEntity:
		definition, valid := entity.RuntimeEntityState().Type.Definition()
		if !valid {
			return game.AABB{}, false
		}

		return entityBox(position, definition.Width, definition.Height), true
	default:
		return game.AABB{}, false
	}
}
