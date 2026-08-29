package game

import "testing"

type fluidStateTestCase struct {
	name    string
	block   Block
	fluid   FluidType
	amount  uint8
	source  bool
	falling bool
}

func TestFluidStateDerivesWaterAndLavaLevels(t *testing.T) {
	tests := []fluidStateTestCase{
		{name: "water source", block: fluidBlockForTest(t, Water, 0), fluid: FluidTypeWater, amount: 8, source: true},
		{name: "water flowing", block: fluidBlockForTest(t, Water, 3), fluid: FluidTypeWater, amount: 5},
		{name: "water falling", block: fluidBlockForTest(t, Water, 8), fluid: FluidTypeWater, amount: 8, falling: true},
		{name: "lava falling", block: fluidBlockForTest(t, Lava, 15), fluid: FluidTypeLava, amount: 8, falling: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := test.block.FluidState()

			if state.Type() != test.fluid || state.Amount() != test.amount || state.IsSource() != test.source || state.IsFalling() != test.falling {
				t.Fatalf("fluid state = type %d amount %d source %v falling %v", state.Type(), state.Amount(), state.IsSource(), state.IsFalling())
			}

			wantHeight := float64(test.amount) / 9
			if state.OwnHeight() != wantHeight {
				t.Fatalf("own height = %v, want %v", state.OwnHeight(), wantHeight)
			}

			if state.LegacyBlock() != test.block {
				t.Fatalf("legacy block = %d, want %d", state.LegacyBlock(), test.block)
			}
		})
	}
}

func TestFluidStateSupportsWaterloggedBlocks(t *testing.T) {
	stairs, valid := OakStairs.WithProperties(BlockPropertyValue{Name: "waterlogged", Value: "true"})
	if !valid {
		t.Fatal("resolve waterlogged stairs")
	}

	state := FluidStateFromLegacyBlock(stairs)

	if !stairs.Waterloggable() || state.Type() != FluidTypeWater || !state.IsSource() || state.Amount() != 8 || state.IsFalling() {
		t.Fatalf("waterlogged fluid state = type %d amount %d source %v falling %v", state.Type(), state.Amount(), state.IsSource(), state.IsFalling())
	}

	if state.LegacyBlock() != Water {
		t.Fatalf("waterlogged legacy block = %d, want %d", state.LegacyBlock(), Water)
	}

	if Stone.Waterloggable() || !Air.FluidState().Empty() {
		t.Fatal("non-waterloggable or empty fluid metadata is incorrect")
	}
}

func TestWorldFluidAtAndEffectiveHeight(t *testing.T) {
	world := NewOverworld(nil)

	position := BlockPosition{X: 3, Y: 70, Z: -2}

	flowing := fluidBlockForTest(t, Water, 4)

	world.SetBlock(position, flowing)

	state := world.FluidAt(position)
	if state.Height(world, position) != state.OwnHeight() {
		t.Fatalf("height without fluid above = %v, want %v", state.Height(world, position), state.OwnHeight())
	}

	above := position

	above.Y++

	world.SetBlock(above, fluidBlockForTest(t, Water, 7))

	if state.Height(world, position) != 1 {
		t.Fatalf("height with water above = %v, want 1", state.Height(world, position))
	}

	world.SetBlock(above, Lava)

	if state.Height(world, position) != state.OwnHeight() {
		t.Fatalf("height with lava above = %v, want %v", state.Height(world, position), state.OwnHeight())
	}

	if !state.SameFamily(world.FluidAt(position)) || state.SameFamily(world.FluidAt(above)) {
		t.Fatal("fluid family comparison is incorrect")
	}
}

func fluidBlockForTest(t *testing.T, block Block, level uint8) Block {
	t.Helper()

	state, valid := block.WithProperties(BlockPropertyValue{Name: "level", Value: fluidLevelString(level)})
	if !valid {
		t.Fatalf("resolve %d at level %d", block, level)
	}

	return state
}
