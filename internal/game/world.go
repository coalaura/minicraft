package game

type World struct {
	Name          string
	DimensionType string
	Seed          int64

	Spawn    Position
	SeaLevel int32
}

func NewOverworld() *World {
	return &World{
		Name:          "minecraft:overworld",
		DimensionType: "minecraft:overworld",

		Spawn: Position{
			X: 0.5,
			Y: 70,
			Z: 0.5,
		},

		SeaLevel: 64,
	}
}
