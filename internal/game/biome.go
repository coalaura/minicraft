package game

type Biome uint8

//go:generate go run ../../cmd/generate-biomes -input ../../ref/1.21.11/biomes.json -game-output biomes_generated.go -protocol-output ../protocol/biome_registry_generated.go

type BiomeGenerator interface {
	BiomeAt(seed int64, x, y, z int32) Biome
}

func (biome Biome) Valid() bool {
	return biome < BiomeCount
}
