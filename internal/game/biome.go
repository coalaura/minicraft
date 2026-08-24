package game

type Biome uint8

const (
	BiomePlains Biome = iota
	BiomeForest
	BiomeDesert
	BiomeTaiga
	BiomeSnowyPlains
	BiomeSwamp
	BiomeOcean
	BiomeRiver

	BiomeCount
)

type BiomeGenerator interface {
	BiomeAt(seed int64, x, z int32) Biome
}

func (biome Biome) Valid() bool {
	return biome < BiomeCount
}
