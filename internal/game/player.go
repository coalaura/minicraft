package game

import "math"

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

const (
	DefaultPlayerHealth     = 20
	DefaultPlayerFoodLevel  = 20
	DefaultPlayerSaturation = 5
	DefaultPlayerAirSupply  = 300
)

type PlayerPose uint8

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

	GameMode            GameMode
	Health              float32
	FoodLevel           int32
	Saturation          float32
	Exhaustion          float32
	FoodTickTimer       int32
	SurvivalTickCount   int64
	AirSupply           int32
	FallDistance        float32
	RemainingFireTicks  int32
	InvulnerableTime    int32
	LastHurt            float32
	Dead                bool
	SurvivalInitialized bool

	OnGround  bool
	Sneaking  bool
	Sprinting bool
	Swimming  bool
	Pose      PlayerPose

	SelectedHotbarSlot int
	Inventory          PlayerInventory

	UsingItem             bool
	UsingOffhand          bool
	UseRemainingTicks     uint16
	UseSelectedHotbarSlot int
	UseAnimation          ItemUseAnimation
	UseStack              ItemStack
}

func (player *Player) ResetSurvivalState() {
	player.Health = DefaultPlayerHealth
	player.FoodLevel = DefaultPlayerFoodLevel
	player.Saturation = DefaultPlayerSaturation
	player.Exhaustion = 0
	player.FoodTickTimer = 0
	player.SurvivalTickCount = 0
	player.AirSupply = DefaultPlayerAirSupply
	player.FallDistance = 0
	player.RemainingFireTicks = 0
	player.InvulnerableTime = 0
	player.LastHurt = 0
	player.Dead = false

	player.StopUsingItem()

	player.SurvivalInitialized = true
}

func (player *Player) AddExhaustion(amount float32) {
	if amount <= 0 || player.GameMode == GameModeCreative || player.GameMode == GameModeSpectator {
		return
	}

	player.Exhaustion = min(player.Exhaustion+amount, 40)
}

func (player *Player) StopUsingItem() bool {
	if !player.UsingItem {
		return false
	}

	player.UsingItem = false
	player.UsingOffhand = false
	player.UseRemainingTicks = 0
	player.UseSelectedHotbarSlot = 0
	player.UseAnimation = ItemUseAnimationNone
	player.UseStack = ItemStack{}

	return true
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
