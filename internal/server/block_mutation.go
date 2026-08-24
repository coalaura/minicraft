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
	BlockMutationInteract
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
	Changes []game.BlockChange
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
	return r.MutateBlocks(session, action, []game.BlockChange{{Position: position, Replacement: replacement}})
}

// MutateBlocks validates and applies a coordinated group atomically.
func (r *Runtime) MutateBlocks(session *Session, action BlockMutationAction, changes []game.BlockChange) (BlockMutationResult, error) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	if action == BlockMutationPlace {
		for _, change := range changes {
			if r.World.BlockAt(change.Position) != game.Air || change.Replacement == game.Air {
				return BlockMutationResult{Block: r.World.BlockAt(change.Position)}, nil
			}
		}
	}

	if action == BlockMutationBreak && len(changes) == 1 && changes[0].Replacement == game.Air {
		changes = r.breakChanges(changes[0].Position)
	}

	requiredChanges := len(changes)

	changes = r.withStructuralNeighborChanges(changes)

	return r.mutateBlocksLocked(session, action, changes, requiredChanges, true)
}

func (r *Runtime) PlaceBlock(session *Session, clicked, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	current := r.World.BlockAt(position)
	if !session.hasLoadedBlock(clicked) || r.World.BlockAt(clicked) == game.Air || current != game.Air {
		return BlockMutationResult{Block: current}, nil
	}

	changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: position, Replacement: replacement}})

	return r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true)
}

func (r *Runtime) mutateBlocksLocked(session *Session, action BlockMutationAction, changes []game.BlockChange, requiredChanges int, allowOccupied bool) (BlockMutationResult, error) {
	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	result := BlockMutationResult{}

	if len(changes) != 0 {
		result.Block = r.World.BlockAt(changes[0].Position)
	}

	if !active || len(changes) == 0 {
		return result, nil
	}

	if action == BlockMutationBreak && !r.AllowBlockBreaking {
		return result, nil
	}

	if (action == BlockMutationPlace || action == BlockMutationInteract) && !r.AllowBlockPlacing {
		return result, nil
	}

	player := session.snapshotPlayer()

	policy := r.BlockMutationPolicy
	if policy == nil {
		policy = CreativeBlockMutationPolicy{}
	}

	seen := make(map[game.BlockPosition]struct{}, len(changes))
	committed := make([]game.BlockChange, 0, len(changes))
	states := make([]int32, 0, len(changes))

	for index, change := range changes {
		if _, duplicate := seen[change.Position]; duplicate {
			return result, nil
		}

		seen[change.Position] = struct{}{}

		current := r.World.BlockAt(change.Position)
		if !change.Replacement.Valid() {
			return result, nil
		}

		if index < requiredChanges {
			if !session.hasLoadedBlock(change.Position) || !blockWithinInteractionRange(player, change.Position) {
				return result, nil
			}

			if action == BlockMutationPlace && !allowOccupied && current != game.Air {
				return result, nil
			}

			if action == BlockMutationPlace && change.Replacement == game.Air {
				return result, nil
			}

			mutation := BlockMutation{Player: player, Action: action, Position: change.Position, Current: current, Replacement: change.Replacement}
			if !policy.AllowBlockMutation(mutation) {
				return result, nil
			}
		}

		if current == change.Replacement {
			continue
		}

		state, err := protocolBlockState(change.Replacement)
		if err != nil {
			return BlockMutationResult{}, fmt.Errorf("encode replacement block: %w", err)
		}

		committed = append(committed, change)
		states = append(states, state)
	}

	result.Allowed = true
	if len(committed) == 0 {
		return result, nil
	}

	r.World.SetBlocks(committed)

	result.Block = committed[0].Replacement
	result.Changes = committed
	result.Changed = true

	for _, other := range r.snapshotSessions() {
		for index, change := range committed {
			err := other.sendBlockUpdateIfLoaded(change.Position, states[index])
			if err != nil {
				other.Log.Warnf("[play] failed to update block: %v\n", err)
			}
		}
	}

	return result, nil
}
