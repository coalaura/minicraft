package game

import "strconv"

type FluidType uint8

const (
	FluidTypeEmpty FluidType = iota
	FluidTypeWater
	FluidTypeLava
)

type fluidStateData struct {
	fluidType FluidType
	level     uint8
}

// FluidState is the fluid derived from an authoritative block state.
type FluidState struct {
	fluidType FluidType
	level     uint8
}

func (block Block) FluidState() FluidState {
	if !block.Valid() {
		return FluidState{}
	}

	data := stateFluidStates[block]
	return FluidState(data)
}

func FluidStateForBlock(block Block) FluidState {
	return block.FluidState()
}

// FluidStateFromLegacyBlock converts a water, lava, or waterlogged block state
// to its derived fluid state.
func FluidStateFromLegacyBlock(block Block) FluidState {
	return block.FluidState()
}

func (state FluidState) Type() FluidType {
	return state.fluidType
}

func (state FluidState) Empty() bool {
	return state.fluidType == FluidTypeEmpty
}

func (state FluidState) IsSource() bool {
	return !state.Empty() && state.level == 0
}

func (state FluidState) Source() bool {
	return state.IsSource()
}

func (state FluidState) Amount() uint8 {
	if state.Empty() {
		return 0
	}

	if state.level >= 8 {
		return 8
	}

	return 8 - state.level
}

func (state FluidState) IsFalling() bool {
	return !state.Empty() && state.level >= 8
}

func (state FluidState) Falling() bool {
	return state.IsFalling()
}

func (state FluidState) OwnHeight() float64 {
	return float64(state.Amount()) / 9
}

// Height returns the fluid height at position. A same-family fluid directly
// above fills the current block regardless of this state's own level.
func (state FluidState) Height(world *World, position BlockPosition) float64 {
	if state.Empty() {
		return 0
	}

	above := position

	above.Y++

	if state.SameFamily(world.FluidAt(above)) {
		return 1
	}

	return state.OwnHeight()
}

func (state FluidState) EffectiveHeight(world *World, position BlockPosition) float64 {
	return state.Height(world, position)
}

func (state FluidState) SameFamily(other FluidState) bool {
	return !state.Empty() && state.fluidType == other.fluidType
}

func (state FluidState) LegacyBlock() Block {
	if state.Empty() {
		return Air
	}

	block := Water

	if state.fluidType == FluidTypeLava {
		block = Lava
	}

	level := state.level

	level = min(level, 15)

	legacy, valid := block.WithProperties(BlockPropertyValue{Name: "level", Value: fluidLevelString(level)})
	if !valid {
		return Air
	}

	return legacy
}

func (state FluidState) Block() Block {
	return state.LegacyBlock()
}

func fluidLevelString(level uint8) string {
	return strconv.Itoa(int(level))
}
