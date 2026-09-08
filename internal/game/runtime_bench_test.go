package game

import (
	"testing"
	"unsafe"
)

type benchmarkProceduralGenerator struct{}

var (
	benchmarkBlockSink       Block
	benchmarkBoxesSink       []AABB
	benchmarkStringSink      string
	benchmarkEnchantmentSink int32
)

func (benchmarkProceduralGenerator) BlockAt(seed int64, position BlockPosition) Block {
	value := int64(position.X)*31 + int64(position.Y)*17 + int64(position.Z)*13 + seed
	if value&7 == 0 {
		return Dirt
	}

	return Stone
}

func BenchmarkBlockProperty(b *testing.B) {
	state, valid := ChiseledBookshelf.WithProperties(BlockPropertyValue{Name: "slot_5_occupied", Value: "true"})
	if !valid {
		b.Fatal("resolve chiseled bookshelf state")
	}

	b.ReportAllocs()

	for b.Loop() {
		benchmarkStringSink, _ = state.Property("slot_5_occupied")
	}
}

func BenchmarkItemStackEnchantmentLevel(b *testing.B) {
	stack := NewItemStack(ItemDiamondPickaxe, 1, []ItemComponent{
		{Type: ItemComponentEnchantments, Data: []byte{0x02, 0x17, 0x03, 0x14, 0x05}},
	}, nil)

	b.ReportAllocs()

	for b.Loop() {
		benchmarkEnchantmentSink = stack.EnchantmentLevel(EnchantmentEfficiency)
	}
}

func BenchmarkMixedCollisionShapes(b *testing.B) {
	stairs, valid := OakStairs.WithProperties(
		BlockPropertyValue{Name: "facing", Value: "east"},
		BlockPropertyValue{Name: "shape", Value: "inner_left"},
	)

	if !valid {
		b.Fatal("resolve stairs")
	}

	fence, valid := OakFence.WithProperties(
		BlockPropertyValue{Name: "east", Value: "true"},
		BlockPropertyValue{Name: "north", Value: "true"},
	)

	if !valid {
		b.Fatal("resolve fence")
	}

	wall, valid := CobblestoneWall.WithProperties(
		BlockPropertyValue{Name: "east", Value: "low"},
		BlockPropertyValue{Name: "south", Value: "tall"},
	)

	if !valid {
		b.Fatal("resolve wall")
	}

	blocks := [...]Block{Stone, OakSlab, stairs, OakDoor, fence, wall, Hopper, Chest}

	var storage [64]AABB

	cacheBytes := len(blockCollisionShapes)*int(unsafe.Sizeof(blockCollisionShape{})) + len(blockCollisionShapeIndices)*int(unsafe.Sizeof(blockCollisionShapeIndices[0]))

	b.Run("cached", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			boxes := storage[:0]

			for index, block := range blocks {
				boxes = block.AppendCollisionBoxes(boxes, BlockPosition{X: int32(index)})
			}

			benchmarkBoxesSink = boxes
		}

		b.ReportMetric(float64(cacheBytes), "cache-bytes")
		b.ReportMetric(float64(len(blockCollisionShapes)), "unique-shapes")
	})

	b.Run("uncached", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			boxes := storage[:0]

			for index, block := range blocks {
				start := len(boxes)
				boxes = block.appendCollisionBoxesUncached(boxes)

				for boxIndex := start; boxIndex < len(boxes); boxIndex++ {
					boxes[boxIndex] = boxes[boxIndex].Translate(float64(index), 0, 0)
				}
			}

			benchmarkBoxesSink = boxes
		}
	})
}

func BenchmarkWorldBlockAt(b *testing.B) {
	positions := make([]BlockPosition, 1024)

	for index := range positions {
		positions[index] = BlockPosition{X: int32(index&31) - 16, Y: int32(index%8) - 4, Z: int32(index>>5) - 16}
	}

	b.Run("procedural", func(b *testing.B) {
		world := &World{Seed: 42, Generator: benchmarkProceduralGenerator{}}
		index := 0

		b.ReportAllocs()

		for b.Loop() {
			benchmarkBlockSink = world.BlockAt(positions[index&1023])
			index++
		}
	})

	b.Run("sparse_overrides", func(b *testing.B) {
		world := &World{Seed: 42, Generator: benchmarkProceduralGenerator{}}

		changes := make([]BlockChange, 0, 64)

		for index := 0; index < len(positions); index += 16 {
			changes = append(changes, BlockChange{Position: positions[index], Replacement: GoldBlock})
		}

		world.SetBlocks(changes)

		index := 0

		b.ReportAllocs()

		for b.Loop() {
			benchmarkBlockSink = world.BlockAt(positions[index&1023])
			index++
		}
	})
}
