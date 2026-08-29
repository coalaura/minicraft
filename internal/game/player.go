package game

import "math"

type PlayerPose uint8

const (
	PlayerPoseStanding PlayerPose = iota
	PlayerPoseCrouching
	PlayerPoseCrawling
)

const (
	standingPlayerEyeHeight      = 1.62
	crouchingPlayerEyeHeight     = 1.27
	crawlingPlayerEyeHeight      = 0.4
	defaultBlockInteractionRange = 4.5
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
	Swimming  bool
	Pose      PlayerPose

	SelectedHotbarSlot int
	Inventory          PlayerInventory
}

func (player Player) EyeHeight() float64 {
	switch player.Pose {
	case PlayerPoseCrouching:
		return crouchingPlayerEyeHeight
	case PlayerPoseCrawling:
		return crawlingPlayerEyeHeight
	default:
		return standingPlayerEyeHeight
	}
}

func (player Player) EyePosition() Position {
	position := player.Position

	position.Y += player.EyeHeight()

	return position
}

// IsWithinBlockInteractionRange matches Player's strict eye-to-block-AABB range test.
func (player Player) IsWithinBlockInteractionRange(position BlockPosition, buffer float64) bool {
	eye := player.EyePosition()

	distanceX := eye.X - math.Max(float64(position.X), math.Min(eye.X, float64(position.X+1)))
	distanceY := eye.Y - math.Max(float64(position.Y), math.Min(eye.Y, float64(position.Y+1)))
	distanceZ := eye.Z - math.Max(float64(position.Z), math.Min(eye.Z, float64(position.Z+1)))

	maximumDistance := defaultBlockInteractionRange + buffer
	return distanceX*distanceX+distanceY*distanceY+distanceZ*distanceZ < maximumDistance*maximumDistance
}
