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
