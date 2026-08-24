package game

const (
	ChunkWidth = 16
)

type Block uint16

const (
	Air Block = iota
	Stone
)

type BlockPosition struct {
	X int32
	Y int32
	Z int32
}

type ChunkPosition struct {
	X int32
	Z int32
}

type Generator interface {
	BlockAt(seed int64, position BlockPosition) Block
}

type SpawnPlatformGenerator struct{}

func (SpawnPlatformGenerator) BlockAt(_ int64, position BlockPosition) Block {
	const (
		platformY      = 69
		platformRadius = 4
	)

	if position.Y != platformY {
		return Air
	}

	if position.X < -platformRadius || position.X > platformRadius {
		return Air
	}

	if position.Z < -platformRadius || position.Z > platformRadius {
		return Air
	}

	return Stone
}
