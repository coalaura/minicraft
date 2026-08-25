package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

func (r *Runtime) calculatedPlayerPose(player game.Player) game.PlayerPose {
	desired := game.PlayerPoseStanding

	if player.Sneaking {
		desired = game.PlayerPoseCrouching
	}

	player.Pose = desired

	if r.playerFits(player) {
		return desired
	}

	if desired == game.PlayerPoseStanding {
		player.Pose = game.PlayerPoseCrouching

		if r.playerFits(player) {
			return game.PlayerPoseCrouching
		}
	}

	player.Pose = game.PlayerPoseCrawling

	if r.playerFits(player) {
		return game.PlayerPoseCrawling
	}

	return game.PlayerPoseCrawling
}

func (r *Runtime) playerFits(player game.Player) bool {
	playerBox := player.CollisionBox()

	minX := int32(math.Floor(playerBox.MinX))
	minY := int32(math.Floor(playerBox.MinY))
	minZ := int32(math.Floor(playerBox.MinZ))

	maxX := int32(math.Ceil(playerBox.MaxX)) - 1
	maxY := int32(math.Ceil(playerBox.MaxY)) - 1
	maxZ := int32(math.Ceil(playerBox.MaxZ)) - 1

	for blockX := minX; blockX <= maxX; blockX++ {
		for blockY := minY; blockY <= maxY; blockY++ {
			for blockZ := minZ; blockZ <= maxZ; blockZ++ {
				position := game.BlockPosition{X: blockX, Y: blockY, Z: blockZ}

				blockBoxes := r.World.BlockAt(position).CollisionBoxes(position)
				if slices.ContainsFunc(blockBoxes, playerBox.Intersects) {
					return false
				}
			}
		}
	}

	return true
}

func (r *Runtime) placementObstructed(changes []game.BlockChange) bool {
	players := r.snapshotSessions()

	for _, session := range players {
		playerBox := session.snapshotPlayer().CollisionBox()

		for _, change := range changes {
			blockBoxes := change.Replacement.CollisionBoxes(change.Position)
			if slices.ContainsFunc(blockBoxes, playerBox.Intersects) {
				return true
			}
		}
	}

	return false
}

func (r *Runtime) recalculateActivePlayerPoses() []game.Player {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	changedPlayers := make([]game.Player, 0)

	for _, session := range r.snapshotSessions() {
		player, changed := session.updatePlayerState(func(player *game.Player) bool {
			pose := r.calculatedPlayerPose(*player)
			if pose == player.Pose {
				return false
			}

			player.Pose = pose

			return true
		})

		if changed {
			changedPlayers = append(changedPlayers, player)
		}
	}

	return changedPlayers
}
