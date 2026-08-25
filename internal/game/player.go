package game

type PlayerPose uint8

const (
	PlayerPoseStanding PlayerPose = iota
	PlayerPoseCrouching
	PlayerPoseCrawling
)

type ProfileProperty struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Signature string `json:"signature"`
}

// gost:preserve-layout
type Player struct {
	EntityID int32
	UUID     string
	Name     string

	Properties []ProfileProperty
	SkinParts  byte

	Position Position
	Rotation Rotation
	Velocity Velocity

	GameMode  GameMode
	OnGround  bool
	Sneaking  bool
	Sprinting bool
	Pose      PlayerPose

	SelectedHotbarSlot int
	Inventory          PlayerInventory
}
