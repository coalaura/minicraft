package protocol

// gost:preserve-layout
type World struct {
	Name          string
	DimensionType int32

	Spawn Vec3

	ViewDistance       int32
	SimulationDistance int32
	SeaLevel           int32
}

// gost:preserve-layout
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// gost:preserve-layout
type Player struct {
	EntityID int32
	UUID     string
	Name     string

	Position Vec3
	Yaw      float32
	Pitch    float32
	GameMode byte
}
