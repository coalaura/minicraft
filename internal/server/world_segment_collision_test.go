package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type worldSegmentCollisionTest struct {
	name           string
	blocks         map[game.BlockPosition]game.Block
	from           game.Position
	to             game.Position
	wantFound      bool
	wantFraction   float64
	wantPosition   game.Position
	wantBlock      game.BlockPosition
	wantBlockState game.Block
}

func TestSweepWorldBlockSegment(t *testing.T) {
	bottomSlab := bottomSlabForWorldSegmentCollision(t)

	fullBlockPosition := game.BlockPosition{X: 2, Y: 70}
	partialBlockPosition := game.BlockPosition{X: 2, Y: 70}
	nearBlockPosition := game.BlockPosition{X: 2, Y: 70}
	farBlockPosition := game.BlockPosition{X: 4, Y: 70}
	highSpeedBlockPosition := game.BlockPosition{X: 50, Y: 70}

	tests := []worldSegmentCollisionTest{
		{
			name:           "full block",
			blocks:         map[game.BlockPosition]game.Block{fullBlockPosition: game.Stone},
			from:           game.Position{X: 0, Y: 70.5, Z: 0.5},
			to:             game.Position{X: 3, Y: 70.5, Z: 0.5},
			wantFound:      true,
			wantFraction:   2.0 / 3.0,
			wantPosition:   game.Position{X: 2, Y: 70.5, Z: 0.5},
			wantBlock:      fullBlockPosition,
			wantBlockState: game.Stone,
		},
		{
			name:      "partial shape miss",
			blocks:    map[game.BlockPosition]game.Block{partialBlockPosition: bottomSlab},
			from:      game.Position{X: 0, Y: 70.75, Z: 0.5},
			to:        game.Position{X: 3, Y: 70.75, Z: 0.5},
			wantFound: false,
		},
		{
			name:           "partial shape hit",
			blocks:         map[game.BlockPosition]game.Block{partialBlockPosition: bottomSlab},
			from:           game.Position{X: 0, Y: 70.25, Z: 0.5},
			to:             game.Position{X: 3, Y: 70.25, Z: 0.5},
			wantFound:      true,
			wantFraction:   2.0 / 3.0,
			wantPosition:   game.Position{X: 2, Y: 70.25, Z: 0.5},
			wantBlock:      partialBlockPosition,
			wantBlockState: bottomSlab,
		},
		{
			name:           "high speed crossing",
			blocks:         map[game.BlockPosition]game.Block{highSpeedBlockPosition: game.Stone},
			from:           game.Position{X: 0, Y: 70.5, Z: 0.5},
			to:             game.Position{X: 100, Y: 70.5, Z: 0.5},
			wantFound:      true,
			wantFraction:   0.5,
			wantPosition:   game.Position{X: 50, Y: 70.5, Z: 0.5},
			wantBlock:      highSpeedBlockPosition,
			wantBlockState: game.Stone,
		},
		{
			name: "nearest of multiple",
			blocks: map[game.BlockPosition]game.Block{
				nearBlockPosition: game.Dirt,
				farBlockPosition:  game.Stone,
			},
			from:           game.Position{X: 0, Y: 70.5, Z: 0.5},
			to:             game.Position{X: 6, Y: 70.5, Z: 0.5},
			wantFound:      true,
			wantFraction:   1.0 / 3.0,
			wantPosition:   game.Position{X: 2, Y: 70.5, Z: 0.5},
			wantBlock:      nearBlockPosition,
			wantBlockState: game.Dirt,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: blockMutationTestGenerator{block: game.Air}}

			runtime := NewRuntime(world)

			for position, block := range test.blocks {
				world.SetBlock(position, block)
			}

			hit, found := runtime.sweepWorldBlockSegment(test.from, test.to)
			if found != test.wantFound {
				t.Fatalf("found = %t, want %t", found, test.wantFound)
			}

			if !found {
				return
			}

			if hit.Fraction != test.wantFraction || hit.Position != test.wantPosition || hit.BlockPosition != test.wantBlock || hit.Block != test.wantBlockState {
				t.Fatalf("hit = %+v, want fraction %v, position %+v, block position %+v, block %v", hit, test.wantFraction, test.wantPosition, test.wantBlock, test.wantBlockState)
			}
		})
	}
}

func bottomSlabForWorldSegmentCollision(t *testing.T) game.Block {
	t.Helper()

	slab, valid := game.StoneSlab.WithProperties(game.BlockPropertyValue{Name: "type", Value: "bottom"})
	if !valid {
		t.Fatal("create bottom stone slab")
	}

	return slab
}
