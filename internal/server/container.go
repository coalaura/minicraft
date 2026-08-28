package server

import "github.com/coalaura/minicraft/internal/game"

const containerValidityPadding = 4.0

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
	return player.IsWithinBlockInteractionRange(position, containerValidityPadding)
}
