package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	miningInteractionBuffer = 1.0
	miningStopThreshold     = 0.7
	miningCompletion        = 1.0
	miningCrackRangeSquared = 32.0 * 32.0
)

type miningState struct {
	position game.BlockPosition
	block    game.Block
	units    int32
	stage    int8
	active   bool
	delayed  bool
}

func (r *Runtime) startDestroyingBlock(session *Session, position game.BlockPosition) (BlockMutationResult, error) {
	r.worldMutationMu.Lock()

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		player := session.snapshotPlayer()

		block := r.World.BlockAt(position)

		result := BlockMutationResult{Block: block}

		if !r.validMiningTarget(session, player, position, block) {
			r.cancelMiningLocked(session)

			return result, blockMutationDelivery{}, nil
		}

		if player.GameMode == game.GameModeCreative {
			r.cancelMiningLocked(session)

			changes := r.breakChanges(position)

			requiredChanges := len(changes)

			changes = r.withStructuralNeighborChanges(changes)

			return r.mutateBlocksLocked(session, BlockMutationBreak, changes, requiredChanges, true, false, false, true)
		}

		if player.GameMode != game.GameModeSurvival {
			r.cancelMiningLocked(session)

			return result, blockMutationDelivery{}, nil
		}

		if session.mining.active || session.mining.delayed {
			r.cancelMiningLocked(session)
		}

		session.mining = miningState{position: position, block: block, units: 1, stage: -1, active: true}

		progress := destroyProgress(player, block) * float64(session.mining.units)

		if progress >= miningCompletion {
			tool := selectedMiningTool(player)

			r.cancelMiningLocked(session)

			return r.mutateMinedBlockLocked(session, position, tool)
		}

		r.updateMiningCrackLocked(session, progress)

		result.Allowed = true

		return result, blockMutationDelivery{}, nil
	}()

	return r.completeBlockMutation(result, delivery, err)
}

func (r *Runtime) stopDestroyingBlock(session *Session, position game.BlockPosition) (BlockMutationResult, error) {
	r.worldMutationMu.Lock()

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		state := session.mining

		result := BlockMutationResult{Block: r.World.BlockAt(position)}

		if !state.active || state.position != position {
			return result, blockMutationDelivery{}, nil
		}

		player := session.snapshotPlayer()
		if !r.validMiningState(session, player, state) {
			r.cancelMiningLocked(session)

			return result, blockMutationDelivery{}, nil
		}

		progress := destroyProgress(player, state.block) * float64(state.units)
		if progress >= miningStopThreshold {
			tool := selectedMiningTool(player)
			r.cancelMiningLocked(session)

			return r.mutateMinedBlockLocked(session, position, tool)
		}

		session.mining.active = false
		session.mining.delayed = true
		result.Allowed = true

		return result, blockMutationDelivery{}, nil
	}()

	return r.completeBlockMutation(result, delivery, err)
}

func (r *Runtime) abortDestroyingBlock(session *Session) {
	r.worldMutationMu.Lock()
	defer r.worldMutationMu.Unlock()

	r.cancelMiningLocked(session)
}

func (r *Runtime) tickMiningAttemptsLocked() []queuedBlockMutation {
	mutations := make([]queuedBlockMutation, 0)

	for _, session := range r.snapshotSessions() {
		state := session.mining
		if !state.active && !state.delayed {
			continue
		}

		player := session.snapshotPlayer()
		if !r.validMiningState(session, player, state) {
			r.cancelMiningLocked(session)

			continue
		}

		session.mining.units++

		progress := destroyProgress(player, state.block) * float64(session.mining.units)

		if state.delayed && progress >= miningCompletion {
			tool := selectedMiningTool(player)

			position := state.position

			r.cancelMiningLocked(session)

			result, delivery, err := r.mutateMinedBlockLocked(session, position, tool)
			if err != nil {
				if session.Log != nil {
					session.Log.Warnf("[play] failed to mine block: %v\n", err)
				}

				continue
			}

			if result.Changed {
				mutations = append(mutations, queuedBlockMutation{result: result, delivery: delivery})
			}

			continue
		}

		r.updateMiningCrackLocked(session, progress)
	}

	return mutations
}

func (r *Runtime) validMiningTarget(session *Session, player game.Player, position game.BlockPosition, block game.Block) bool {
	if !r.AllowBlockBreaking || !session.hasLoadedBlock(position) || block == game.Air {
		return false
	}

	if !player.IsWithinBlockInteractionRange(position, miningInteractionBuffer) {
		return false
	}

	policy := r.BlockMutationPolicy
	if policy == nil {
		policy = CreativeBlockMutationPolicy{}
	}

	mutation := BlockMutation{Player: player, Action: BlockMutationBreak, Position: position, Current: block, Replacement: game.Air}
	return policy.AllowBlockMutation(mutation)
}

func (r *Runtime) validMiningState(session *Session, player game.Player, state miningState) bool {
	return player.GameMode == game.GameModeSurvival &&
		r.World.BlockAt(state.position) == state.block &&
		r.validMiningTarget(session, player, state.position, state.block)
}

func (r *Runtime) cancelMiningLocked(session *Session) {
	state := session.mining
	if (state.active || state.delayed) && state.stage != -1 {
		r.broadcastMiningCrack(session, state.position, -1)
	}

	session.mining = miningState{}
}

func (r *Runtime) updateMiningCrackLocked(session *Session, progress float64) {
	stage := int8(math.Floor(progress * 10))
	if stage == session.mining.stage {
		return
	}

	session.mining.stage = stage

	r.broadcastMiningCrack(session, session.mining.position, stage)
}

func (r *Runtime) broadcastMiningCrack(session *Session, position game.BlockPosition, stage int8) {
	breaker := session.snapshotPlayer()

	packet := protocol.BlockDestruction{EntityID: breaker.EntityID, Position: position, Stage: stage}

	for _, other := range r.snapshotSessions() {
		if other == session || !other.hasLoadedBlock(position) {
			continue
		}

		observer := other.snapshotPlayer()

		distanceX := observer.Position.X - float64(position.X)
		distanceY := observer.Position.Y - float64(position.Y)
		distanceZ := observer.Position.Z - float64(position.Z)

		distanceSquared := distanceX*distanceX + distanceY*distanceY + distanceZ*distanceZ
		if distanceSquared >= miningCrackRangeSquared {
			continue
		}

		err := other.writePacket(protocol.ClientboundBlockDestructionID, packet)
		if err != nil && other.Log != nil {
			other.Log.Warnf("[play] failed to update block destruction: %v\n", err)
		}
	}
}

func destroyProgress(player game.Player, block game.Block) float64 {
	mining := block.MiningProperties()
	if !mining.Destroyable || mining.Hardness < 0 {
		return 0
	}

	if mining.Hardness == 0 {
		return 1
	}

	tool := selectedMiningTool(player)

	divisor := float64(100)

	if tool.Item.IsCorrectToolForDrops(block) {
		divisor = 30
	}

	return float64(tool.Item.BaseDestroySpeed(block)) / float64(mining.Hardness) / divisor
}

func selectedMiningTool(player game.Player) game.ItemStack {
	stack := player.Inventory.Held(player.SelectedHotbarSlot)
	if stack == nil {
		return game.ItemStack{Item: game.ItemAir}
	}

	return stack.Clone()
}

func (r *Runtime) commitOrdinaryBlockDrops(records []blockMutationRecord) {
	for _, record := range records {
		if !record.ordinaryDrop {
			continue
		}

		mining := record.previous.MiningProperties()
		if mining.DropKind != game.BlockDropExact && mining.DropKind != game.BlockDropStateExact {
			continue
		}

		if mining.DropKind == game.BlockDropStateExact {
			value, valid := record.previous.Property(mining.DropProperty)
			if !valid || value != mining.DropValue {
				continue
			}
		}

		if mining.DropItem == game.ItemAir || mining.DropMax <= 0 {
			continue
		}

		count := mining.DropMin
		if mining.DropMax > mining.DropMin {
			count += int32(r.nextEntityRandom() * float32(mining.DropMax-mining.DropMin+1))
		}

		position := game.Position{
			X: float64(record.change.Position.X) + 0.25 + float64(r.nextEntityRandom())*0.5,
			Y: float64(record.change.Position.Y) + 0.25 + float64(r.nextEntityRandom())*0.5 - 0.125,
			Z: float64(record.change.Position.Z) + 0.25 + float64(r.nextEntityRandom())*0.5,
		}

		r.SpawnItemEntity(game.ItemStack{Item: mining.DropItem, Count: count}, position, game.Velocity{}, 10)
	}
}
