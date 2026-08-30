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

type miningTool struct {
	stack game.ItemStack
	slot  int
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

	if tool.stack.Item.IsCorrectToolForDrops(block) {
		divisor = 30
	}

	speed := tool.stack.Item.BaseDestroySpeed(block)
	efficiency := tool.stack.EnchantmentLevel(game.EnchantmentEfficiency)

	if speed > 1 && efficiency > 0 {
		speed += float32(efficiency*efficiency + 1)
	}

	return float64(speed) / float64(mining.Hardness) / divisor
}

func selectedMiningTool(player game.Player) miningTool {
	stack := player.Inventory.Held(player.SelectedHotbarSlot)
	if stack == nil {
		return miningTool{stack: game.ItemStack{Item: game.ItemAir}, slot: player.SelectedHotbarSlot}
	}

	return miningTool{stack: stack.Clone(), slot: player.SelectedHotbarSlot}
}

func (r *Runtime) damageMiningTool(session *Session, tool miningTool, block game.Block) (*game.PlayerInventory, bool) {
	definition, valid := tool.stack.Item.Definition()

	mining := tool.stack.Item.MiningProperties()
	blockMining := block.MiningProperties()

	if !valid || definition.MaxDurability <= 0 || mining.DamagePerBlock <= 0 {
		return nil, false
	}

	if blockMining.Hardness == 0 {
		blockDefinition, blockValid := block.Definition()

		isFire := blockValid && (blockDefinition.ID == game.FireID || blockDefinition.ID == game.SoulFireID)
		if tool.stack.Item != game.ItemShears || isFire {
			return nil, false
		}
	}

	damage := int32(0)
	unbreaking := tool.stack.EnchantmentLevel(game.EnchantmentUnbreaking)

	for range mining.DamagePerBlock {
		if unbreaking > 0 {
			r.miningRandomMu.Lock()
			prevented := r.miningRandom(int(unbreaking)+1) != 0
			r.miningRandomMu.Unlock()

			if prevented {
				continue
			}
		}

		damage++
	}

	if damage == 0 {
		return nil, false
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	before := session.snapshotPlayer().Inventory
	broke := false

	_, changed := session.updatePlayerState(func(player *game.Player) bool {
		stack := player.Inventory.Held(tool.slot)
		if stack == nil || !stack.SameItem(tool.stack) {
			return false
		}

		newDamage := stack.Damage() + damage
		if newDamage < definition.MaxDurability {
			stack.SetDamage(newDamage)

			return true
		}

		stack.Count--

		if stack.Count <= 0 {
			*stack = game.ItemStack{}
		} else {
			stack.SetDamage(0)
		}

		broke = true

		return true
	})

	if !changed {
		return nil, false
	}

	return &before, broke
}

func (r *Runtime) commitOrdinaryBlockDrops(records []blockMutationRecord) {
	for _, record := range records {
		if record.lootContext == blockLootNone {
			continue
		}

		mining := record.previous.MiningProperties()
		if len(mining.DropRules) == 0 {
			continue
		}

		for _, rule := range mining.DropRules {
			if rule.RequiresActor && record.lootContext != blockLootPlayer {
				continue
			}

			if rule.GateProperty != "" {
				value, valid := record.previous.Property(rule.GateProperty)
				if !valid || value != rule.GateValue {
					continue
				}
			}

			count := rule.Count
			if count <= 0 {
				count = 1
			}

			if rule.CountProperty != "" {
				value, valid := record.previous.Property(rule.CountProperty)
				if !valid {
					continue
				}

				for _, candidate := range rule.Counts {
					if candidate.Value == value {
						count = candidate.Count

						break
					}
				}
			}

			if rule.Item != game.ItemAir && count > 0 {
				r.popBlockResource(record.change.Position, game.ItemStack{Item: rule.Item, Count: count})
			}
		}
	}
}

func (r *Runtime) popBlockResource(blockPosition game.BlockPosition, stack game.ItemStack) {
	position := game.Position{
		X: float64(blockPosition.X) + 0.25 + float64(r.nextEntityRandom())*0.5,
		Y: float64(blockPosition.Y) + 0.125 + float64(r.nextEntityRandom())*0.5,
		Z: float64(blockPosition.Z) + 0.25 + float64(r.nextEntityRandom())*0.5,
	}

	velocity := game.Velocity{
		X: float64(r.nextEntityRandom())*0.2 - 0.1,
		Y: 0.2,
		Z: float64(r.nextEntityRandom())*0.2 - 0.1,
	}

	r.SpawnItemEntity(stack, position, velocity, 10)
}
