package server

import (
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
)

const blockInteractionRange = 6.0

type BlockMutationAction uint8

const (
	BlockMutationBreak BlockMutationAction = iota
	BlockMutationPlace
)

type BlockMutation struct {
	Player      game.Player
	Action      BlockMutationAction
	Position    game.BlockPosition
	Current     game.Block
	Replacement game.Block
}

type BlockMutationResult struct {
	Block   game.Block
	Allowed bool
	Changed bool
}

type BlockMutationPolicy interface {
	AllowBlockMutation(BlockMutation) bool
}

type CreativeBlockMutationPolicy struct{}

func (CreativeBlockMutationPolicy) AllowBlockMutation(mutation BlockMutation) bool {
	return mutation.Player.GameMode == game.GameModeCreative
}

func blockWithinInteractionRange(player game.Player, position game.BlockPosition) bool {
	blockX := float64(position.X) + 0.5
	blockY := float64(position.Y) + 0.5
	blockZ := float64(position.Z) + 0.5

	eyeY := player.Position.Y + 1.62

	distanceX := blockX - player.Position.X
	distanceY := blockY - eyeY
	distanceZ := blockZ - player.Position.Z

	distanceSquared := distanceX*distanceX + distanceY*distanceY + distanceZ*distanceZ

	return distanceSquared <= blockInteractionRange*blockInteractionRange
}

func (r *Runtime) MutateBlock(session *Session, action BlockMutationAction, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	return r.mutateBlock(session, action, position, replacement)
}

func (r *Runtime) PlaceBlock(session *Session, clicked, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	current := r.World.BlockAt(position)

	if !session.hasLoadedBlock(clicked) || r.World.BlockAt(clicked) == game.Air {
		return BlockMutationResult{Block: current}, nil
	}

	return r.mutateBlock(session, BlockMutationPlace, position, replacement)
}

func (r *Runtime) mutateBlock(session *Session, action BlockMutationAction, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {

	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	current := r.World.BlockAt(position)

	result := BlockMutationResult{Block: current}

	if !active || !session.hasLoadedBlock(position) {
		return result, nil
	}

	mutation := BlockMutation{
		Player:      session.snapshotPlayer(),
		Action:      action,
		Position:    position,
		Current:     current,
		Replacement: replacement,
	}

	if !blockWithinInteractionRange(mutation.Player, position) {
		return result, nil
	}

	if action == BlockMutationBreak && !r.AllowBlockBreaking {
		return result, nil
	}

	if action == BlockMutationPlace && (!r.AllowBlockPlacing || current != game.Air || replacement == game.Air) {
		return result, nil
	}

	policy := r.BlockMutationPolicy
	if policy == nil {
		policy = CreativeBlockMutationPolicy{}
	}

	if !policy.AllowBlockMutation(mutation) {
		return result, nil
	}

	result.Allowed = true

	if current == replacement {
		return result, nil
	}

	state, err := protocolBlockState(replacement)
	if err != nil {
		return BlockMutationResult{}, fmt.Errorf("encode replacement block: %w", err)
	}

	r.World.SetBlock(position, replacement)

	result.Block = replacement
	result.Changed = true

	for _, other := range r.snapshotSessions() {
		err = other.sendBlockUpdateIfLoaded(position, state)
		if err != nil {
			other.Log.Warnf("[play] failed to update block: %v\n", err)
		}
	}

	return result, nil
}
