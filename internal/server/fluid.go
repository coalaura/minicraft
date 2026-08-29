package server

import (
	"strconv"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	waterFluidDelay = int64(5)
	lavaFluidDelay  = int64(30)
	lavaNetherDelay = int64(10)
	waterDropOff    = 1
	lavaDropOff     = 2
	lavaFastDropOff = 1
	waterSlope      = 4
	lavaSlope       = 2
	lavaFastSlope   = 4
)

type FlowingFluid struct {
	block   game.Block
	typeID  game.FluidType
	delay   int64
	dropOff int
	slope   int
}

type FluidRules struct {
	WaterSourceConversion bool
	LavaSourceConversion  bool
}

type FluidEnvironment struct {
	FastLava        bool
	WaterEvaporates bool
}

var (
	waterFluid = FlowingFluid{block: game.Water, typeID: game.FluidTypeWater, delay: waterFluidDelay, dropOff: waterDropOff, slope: waterSlope}
	lavaFluid  = FlowingFluid{block: game.Lava, typeID: game.FluidTypeLava, delay: lavaFluidDelay, dropOff: lavaDropOff, slope: lavaSlope}
	fluidSides = []game.BlockPosition{{X: -1}, {X: 1}, {Z: -1}, {Z: 1}}
)

func fluidForBlock(block game.Block) (FlowingFluid, bool) {
	state := block.FluidState()

	switch state.Type() {
	case game.FluidTypeWater:
		return waterFluid, true
	case game.FluidTypeLava:
		return lavaFluid, true
	default:
		return FlowingFluid{}, false
	}
}

func fluidBlock(fluid FlowingFluid, level int) game.Block {
	block, valid := fluid.block.WithProperties(game.BlockPropertyValue{Name: "level", Value: strconv.Itoa(level)})
	if !valid {
		return fluid.block
	}

	return block
}

func fluidBlockForAmount(fluid FlowingFluid, amount int, falling bool) game.Block {
	if amount < 1 {
		return game.Air
	}

	if falling {
		return fluidBlock(fluid, 8)
	}

	return fluidBlock(fluid, 8-amount)
}

func (r *Runtime) scheduleFluidNeighborsLocked(changes []game.BlockChange) {
	for _, change := range changes {
		positions := [7]game.BlockPosition{
			change.Position,
			{X: change.Position.X - 1, Y: change.Position.Y, Z: change.Position.Z},
			{X: change.Position.X + 1, Y: change.Position.Y, Z: change.Position.Z},
			{X: change.Position.X, Y: change.Position.Y - 1, Z: change.Position.Z},
			{X: change.Position.X, Y: change.Position.Y + 1, Z: change.Position.Z},
			{X: change.Position.X, Y: change.Position.Y, Z: change.Position.Z - 1},
			{X: change.Position.X, Y: change.Position.Y, Z: change.Position.Z + 1},
		}

		for _, position := range positions {
			block := r.World.BlockAt(position)

			fluid, present := fluidForBlock(block)
			if present {
				state := block.FluidState()

				r.scheduleBlockTickLocked(position, state.StateType(), r.fluidDelay(fluid, position, state, state))
			}
		}
	}
}

func (r *Runtime) fluidDelay(fluid FlowingFluid, position game.BlockPosition, oldState, newState game.FluidState) int64 {
	delay := fluid.delay

	if fluid.typeID == game.FluidTypeLava && r.FluidEnvironment.FastLava {
		delay = lavaNetherDelay
	}

	if fluid.typeID != game.FluidTypeLava || oldState.Empty() || newState.Empty() || oldState.IsFalling() || newState.IsFalling() || newState.Height(r.World, position) <= oldState.Height(r.World, position) || r.nextFluidRandom(position, 4) == 0 {
		return delay
	}

	return delay * 4
}

func (r *Runtime) nextFluidRandom(position game.BlockPosition, bound int) int {
	r.fluidRandomMu.Lock()
	defer r.fluidRandomMu.Unlock()

	return r.fluidRandom(position, bound)
}

func (r *Runtime) fluidDropOff(fluid FlowingFluid) int {
	if fluid.typeID == game.FluidTypeLava && r.FluidEnvironment.FastLava {
		return lavaFastDropOff
	}

	return fluid.dropOff
}

func (r *Runtime) fluidSlope(fluid FlowingFluid) int {
	if fluid.typeID == game.FluidTypeLava && r.FluidEnvironment.FastLava {
		return lavaFastSlope
	}

	return fluid.slope
}

func (r *Runtime) tickFluidLocked(position game.BlockPosition) {
	block := r.World.BlockAt(position)

	fluid, present := fluidForBlock(block)
	if !present {
		return
	}

	oldState := block.FluidState()

	replacement, mixes := r.fluidBlockMixingReplacement(position, oldState)

	if mixes {
		r.mutateFluidLocked([]game.BlockChange{{Position: position, Replacement: replacement}}, fluid.typeID)

		return
	}

	newAmount, falling, sourceNeighbors := r.recomputeFluidLevel(position, fluid, oldState)

	newState := fluidBlockForAmount(fluid, newAmount, falling).FluidState()

	if block.Waterloggable() && oldState.Type() == game.FluidTypeWater {
		newState = oldState
		newAmount = int(oldState.Amount())
	}

	changes := make([]game.BlockChange, 0, 6)

	below := game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}

	belowBlock := r.World.BlockAt(below)

	downward := false

	if !newState.Empty() && r.fluidDestinationActive(position, below) && r.canFlowBetween(block, belowBlock, game.BlockFaceDown) {
		replacement, flows, mixes := r.fluidFlowReplacement(position, below, fluid, newAmount, true)
		if mixes || flows {
			changes = append(changes, game.BlockChange{Position: below, Replacement: replacement})
			downward = true
		}
	}

	if !block.Waterloggable() && newState != oldState {
		changes = append(changes, game.BlockChange{Position: position, Replacement: newState.LegacyBlock()})
	}

	spreadSideways := downward && sourceNeighbors >= 3

	if !downward {
		spreadSideways = newState.IsSource() || !r.fluidHole(position, fluid)
	}

	if !newState.Empty() && spreadSideways {
		sideAmount := newAmount - r.fluidDropOff(fluid)

		if oldState.IsFalling() {
			sideAmount = 7
		}

		if sideAmount > 0 {
			r.deferInactiveFluidTargetsLocked(position, fluid, sideAmount)

			for _, direction := range r.fluidFlowDirections(position, fluid, sideAmount) {
				destination := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}
				if !r.fluidDestinationActive(position, destination) {
					continue
				}

				replacement, flows, mixes := r.fluidFlowReplacement(position, destination, fluid, sideAmount, false)
				if flows || mixes {
					changes = append(changes, game.BlockChange{Position: destination, Replacement: replacement})
				}
			}
		}
	}

	if len(changes) != 0 {
		changes = r.withImmediateFluidMixing(changes)
		r.mutateFluidLocked(changes, fluid.typeID)
	}
}

func (r *Runtime) recomputeFluidLevel(position game.BlockPosition, fluid FlowingFluid, oldState game.FluidState) (int, bool, int) {
	currentBlock := r.World.BlockAt(position)

	bestAmount := 0
	sources := 0

	for _, offset := range fluidSides {
		neighbor := game.BlockPosition{X: position.X + offset.X, Y: position.Y, Z: position.Z + offset.Z}

		neighborBlock := r.World.BlockAt(neighbor)

		neighborState := neighborBlock.FluidState()
		canFlow := r.canFlowBetween(neighborBlock, currentBlock, oppositeFluidFace(offset))

		if neighborState.Type() != fluid.typeID || !canFlow {
			continue
		}

		if neighborState.IsSource() {
			sources++
		}

		amount := int(neighborState.Amount())
		if amount > bestAmount {
			bestAmount = amount
		}
	}

	below := game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}

	belowBlock := r.World.BlockAt(below)

	if oldState.IsSource() {
		return 8, false, sources
	}

	belowState := belowBlock.FluidState()

	supported := belowBlock.FullFace(game.BlockFaceUp) || (belowState.Type() == fluid.typeID && belowState.IsSource())

	if r.fluidSourceConversion(fluid.typeID) && sources >= 2 && supported {
		return 8, false, sources
	}

	above := game.BlockPosition{X: position.X, Y: position.Y + 1, Z: position.Z}

	aboveBlock := r.World.BlockAt(above)
	aboveState := aboveBlock.FluidState()

	if aboveState.Type() == fluid.typeID && r.canFlowBetween(aboveBlock, currentBlock, game.BlockFaceDown) {
		return 8, true, sources
	}

	amount := bestAmount - r.fluidDropOff(fluid)

	amount = max(amount, 0)

	return amount, false, sources
}

func (r *Runtime) fluidSourceConversion(fluidType game.FluidType) bool {
	if fluidType == game.FluidTypeWater {
		return r.FluidRules.WaterSourceConversion
	}

	return r.FluidRules.LavaSourceConversion
}

func (r *Runtime) fluidDestinationActive(from, destination game.BlockPosition) bool {
	if blockLoadedChunk(from) == blockLoadedChunk(destination) {
		return true
	}

	_, active := r.ActiveChunk(blockLoadedChunk(destination))
	return active
}

func (r *Runtime) fluidHole(position game.BlockPosition, fluid FlowingFluid) bool {
	current := r.World.BlockAt(position)

	below := game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}

	belowBlock := r.World.BlockAt(below)

	if !r.canFlowBetween(current, belowBlock, game.BlockFaceDown) {
		return false
	}

	if belowBlock.FluidState().Type() == fluid.typeID {
		return true
	}

	_, flows, mixes := r.fluidFlowReplacement(position, below, fluid, 8, true)

	return flows || mixes
}

func (r *Runtime) deferInactiveFluidTargetsLocked(position game.BlockPosition, fluid FlowingFluid, amount int) {
	from := r.World.BlockAt(position)

	for _, direction := range fluidSides {
		destination := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}
		if r.fluidDestinationActive(position, destination) {
			continue
		}

		target := r.World.BlockAt(destination)
		if !r.canFlowBetween(from, target, fluidFace(direction)) {
			continue
		}

		_, flows, mixes := r.fluidFlowReplacement(position, destination, fluid, amount, false)
		if flows || mixes {
			r.deferFluidSourceLocked(position, destination)
		}
	}
}

func (r *Runtime) deferFluidSourceLocked(source, destination game.BlockPosition) {
	if r.deferredFluidSources == nil {
		r.deferredFluidSources = make(map[LoadedChunk]map[game.BlockPosition]struct{})
	}

	chunk := blockLoadedChunk(destination)

	sources := r.deferredFluidSources[chunk]
	if sources == nil {
		sources = make(map[game.BlockPosition]struct{})
		r.deferredFluidSources[chunk] = sources
	}

	sources[source] = struct{}{}
}

func (r *Runtime) resumeDeferredFluidSourcesLocked(chunks []LoadedChunk) {
	for _, chunk := range chunks {
		sources := r.deferredFluidSources[chunk]

		delete(r.deferredFluidSources, chunk)

		for source := range sources {
			block := r.World.BlockAt(source)

			fluid, present := fluidForBlock(block)
			if !present {
				continue
			}

			state := block.FluidState()

			r.scheduleBlockTickLocked(source, state.StateType(), r.fluidDelay(fluid, source, state, state))
		}
	}
}

func (r *Runtime) canFlowBetween(from, to game.Block, face game.BlockFace) bool {
	return !game.CombinedFaceOccludes(from, to, face)
}

func (r *Runtime) fluidFlowReplacement(from, destination game.BlockPosition, fluid FlowingFluid, amount int, downward bool) (game.Block, bool, bool) {
	target := r.World.BlockAt(destination)

	targetState := target.FluidState()
	if targetState.Type() == fluid.typeID {
		return game.Air, false, false
	}

	if !targetState.Empty() {
		if target.Waterloggable() {
			if fluid.typeID == game.FluidTypeLava && downward && targetState.Type() == game.FluidTypeWater {
				return target, false, true
			}

			return game.Air, false, false
		}

		source := r.World.BlockAt(from).FluidState().IsSource()

		return r.fluidMixingReplacement(fluid, source, targetState, downward), false, true
	}

	if target.Waterloggable() {
		replacement, valid := target.WithContainedFluid(fluid.typeID)
		if valid {
			return replacement, true, false
		}

		return game.Air, false, false
	}

	if !target.HasTrait(game.BlockTraitFluidExcluded) && len(target.CollisionBoxes(game.BlockPosition{})) == 0 {
		replacement := fluidBlockForAmount(fluid, amount, downward)

		if fluid.typeID == game.FluidTypeLava {
			mixed, mixes := r.fluidBlockMixingReplacement(destination, replacement.FluidState())
			if mixes {
				return mixed, false, true
			}
		}

		return replacement, true, false
	}

	return game.Air, false, false
}

func (r *Runtime) fluidMixingReplacement(fluid FlowingFluid, source bool, target game.FluidState, downward bool) game.Block {
	if fluid.typeID == game.FluidTypeLava {
		if downward {
			return game.Stone
		}

		if target.Type() == game.FluidTypeWater && source {
			return game.Obsidian
		}

		return game.Cobblestone
	}

	if target.Type() == game.FluidTypeLava && target.IsSource() {
		return game.Obsidian
	}

	return game.Cobblestone
}

func (r *Runtime) fluidBlockMixingReplacement(position game.BlockPosition, state game.FluidState) (game.Block, bool) {
	return r.fluidBlockMixingReplacementWith(position, state, r.World.BlockAt)
}

func (r *Runtime) fluidBlockMixingReplacementWith(position game.BlockPosition, state game.FluidState, blockAt func(game.BlockPosition) game.Block) (game.Block, bool) {
	if state.Type() != game.FluidTypeLava {
		return game.Air, false
	}

	neighbors := [...]game.BlockPosition{
		{X: position.X, Y: position.Y + 1, Z: position.Z},
		{X: position.X, Y: position.Y, Z: position.Z - 1},
		{X: position.X, Y: position.Y, Z: position.Z + 1},
		{X: position.X - 1, Y: position.Y, Z: position.Z},
		{X: position.X + 1, Y: position.Y, Z: position.Z},
	}
	below := game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}
	canMakeBasalt := blockAt(below) == game.SoulSoil

	for _, neighbor := range neighbors {
		neighborBlock := blockAt(neighbor)
		if neighborBlock.FluidState().Type() == game.FluidTypeWater {
			if state.IsSource() {
				return game.Obsidian, true
			}

			return game.Cobblestone, true
		}

		if canMakeBasalt && neighborBlock == game.BlueIce {
			return game.Basalt, true
		}
	}

	return game.Air, false
}

func (r *Runtime) withImmediateFluidMixing(changes []game.BlockChange) []game.BlockChange {
	indices := make(map[game.BlockPosition]int, len(changes))
	replacements := make(map[game.BlockPosition]game.Block, len(changes))
	candidates := make([]game.BlockPosition, 0, len(changes)*6)
	seenCandidates := make(map[game.BlockPosition]struct{}, len(changes)*6)

	addCandidate := func(position game.BlockPosition) {
		if _, seen := seenCandidates[position]; seen {
			return
		}

		seenCandidates[position] = struct{}{}
		candidates = append(candidates, position)
	}

	for index, change := range changes {
		indices[change.Position] = index

		replacements[change.Position] = change.Replacement

		addCandidate(change.Position)

		neighbors := [...]game.BlockPosition{
			{X: change.Position.X, Y: change.Position.Y - 1, Z: change.Position.Z},
			{X: change.Position.X, Y: change.Position.Y + 1, Z: change.Position.Z},
			{X: change.Position.X - 1, Y: change.Position.Y, Z: change.Position.Z},
			{X: change.Position.X + 1, Y: change.Position.Y, Z: change.Position.Z},
			{X: change.Position.X, Y: change.Position.Y, Z: change.Position.Z - 1},
			{X: change.Position.X, Y: change.Position.Y, Z: change.Position.Z + 1},
		}

		for _, neighbor := range neighbors {
			addCandidate(neighbor)
		}
	}

	blockAt := func(position game.BlockPosition) game.Block {
		if replacement, present := replacements[position]; present {
			return replacement
		}

		return r.World.BlockAt(position)
	}

	for _, position := range candidates {
		state := blockAt(position).FluidState()

		replacement, mixes := r.fluidBlockMixingReplacementWith(position, state, blockAt)
		if !mixes {
			continue
		}

		index, present := indices[position]
		if present {
			changes[index].Replacement = replacement
		} else {
			indices[position] = len(changes)
			changes = append(changes, game.BlockChange{Position: position, Replacement: replacement})
		}

		replacements[position] = replacement
	}

	return changes
}

func (r *Runtime) fluidFlowDirections(position game.BlockPosition, fluid FlowingFluid, amount int) []game.BlockPosition {
	slope := r.fluidSlope(fluid)
	bestDistance := slope + 1

	directions := make([]game.BlockPosition, 0, len(fluidSides))

	from := r.World.BlockAt(position)

	for _, direction := range fluidSides {
		destination := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}
		if !r.fluidDestinationActive(position, destination) {
			continue
		}

		target := r.World.BlockAt(destination)

		if !r.canFlowBetween(from, target, fluidFace(direction)) {
			continue
		}

		_, flows, mixes := r.fluidFlowReplacement(position, destination, fluid, amount, false)
		if !flows && !mixes {
			continue
		}

		distance := r.fluidSlopeDistance(destination, fluid, direction, 0)

		if distance < bestDistance {
			bestDistance = distance
			directions = directions[:0]
		}

		if distance == bestDistance {
			directions = append(directions, direction)
		}
	}

	return directions
}

func (r *Runtime) fluidSlopeDistance(position game.BlockPosition, fluid FlowingFluid, previous game.BlockPosition, distance int) int {
	slope := r.fluidSlope(fluid)

	below := game.BlockPosition{X: position.X, Y: position.Y - 1, Z: position.Z}

	belowBlock := r.World.BlockAt(below)

	if r.canFlowBetween(r.World.BlockAt(position), belowBlock, game.BlockFaceDown) {
		_, flows, mixes := r.fluidFlowReplacement(position, below, fluid, 8, true)
		if flows || mixes {
			return distance
		}
	}

	if distance >= slope {
		return slope + 1
	}

	bestDistance := slope + 1

	from := r.World.BlockAt(position)

	for _, direction := range fluidSides {
		if direction.X == -previous.X && direction.Z == -previous.Z {
			continue
		}

		next := game.BlockPosition{X: position.X + direction.X, Y: position.Y, Z: position.Z + direction.Z}

		target := r.World.BlockAt(next)
		if !r.canFlowBetween(from, target, fluidFace(direction)) {
			continue
		}

		_, flows, mixes := r.fluidFlowReplacement(position, next, fluid, 1, false)
		if !flows && !mixes {
			continue
		}

		nextDistance := r.fluidSlopeDistance(next, fluid, direction, distance+1)
		if nextDistance < bestDistance {
			bestDistance = nextDistance
		}
	}

	return bestDistance
}

func fluidFace(offset game.BlockPosition) game.BlockFace {
	if offset.X < 0 {
		return game.BlockFaceWest
	}

	if offset.X > 0 {
		return game.BlockFaceEast
	}

	if offset.Z < 0 {
		return game.BlockFaceNorth
	}

	return game.BlockFaceSouth
}

func oppositeFluidFace(offset game.BlockPosition) game.BlockFace {
	if offset.X < 0 {
		return game.BlockFaceEast
	}

	if offset.X > 0 {
		return game.BlockFaceWest
	}

	if offset.Z < 0 {
		return game.BlockFaceSouth
	}

	return game.BlockFaceNorth
}

func (r *Runtime) mutateFluidLocked(changes []game.BlockChange, fluidType game.FluidType) {
	fizzPositions := make([]game.BlockPosition, 0)

	if fluidType == game.FluidTypeLava {
		for _, change := range changes {
			current := r.World.BlockAt(change.Position)
			if current == change.Replacement && current.FluidState().Type() == game.FluidTypeWater {
				fizzPositions = append(fizzPositions, change.Position)
			}
		}
	}

	result, delivery, err := r.mutateBlocksLocked(nil, BlockMutationPlace, changes, len(changes), true, false, true, false)
	if err != nil {
		return
	}

	if !result.Changed {
		for _, position := range fizzPositions {
			r.queueFluidFizzLocked(position)
		}

		return
	}

	for index := range delivery.records {
		record := &delivery.records[index]
		if fluidType == game.FluidTypeWater && record.previous.FluidState().Empty() && record.previous != game.Air && record.previous != record.change.Replacement && !record.previous.Waterloggable() {
			record.lootContext = blockLootNoBreaker
		}
	}

	for _, record := range delivery.records {
		fizz := record.change.Replacement == game.Obsidian || record.change.Replacement == game.Cobblestone || record.change.Replacement == game.Stone || record.change.Replacement == game.Basalt

		if fluidType == game.FluidTypeLava && record.previous.FluidState().Empty() && record.previous != game.Air && !record.previous.Waterloggable() && record.change.Replacement.FluidState().Type() == game.FluidTypeLava {
			fizz = true
		}

		if fizz {
			delivery.runtimeEvents = append(delivery.runtimeEvents, protocol.LevelEvent{Event: protocol.LevelEventLavaFizz, Position: record.change.Position})
		}
	}

	for _, position := range fizzPositions {
		delivery.runtimeEvents = append(delivery.runtimeEvents, protocol.LevelEvent{Event: protocol.LevelEventLavaFizz, Position: position})
	}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})
}

func (r *Runtime) queueFluidFizzLocked(position game.BlockPosition) {
	deliveryComplete := make(chan struct{})

	delivery := blockMutationDelivery{
		recipients:       r.snapshotSessions(),
		waitForDelivery:  r.blockMutationDeliveryTail,
		deliveryComplete: deliveryComplete,
		runtimeEvents:    []protocol.LevelEvent{{Event: protocol.LevelEventLavaFizz, Position: position}},
	}

	r.blockMutationDeliveryTail = deliveryComplete

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: BlockMutationResult{Changed: true}, delivery: delivery})
}
