package game

// gost:preserve-layout
type Player struct {
	EntityID int32
	UUID     string
	Name     string

	Position Position
	Rotation Rotation
	Velocity Velocity

	GameMode GameMode
	OnGround bool
}
