package game

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

	SelectedHotbarSlot int
	Hotbar             [HotbarSlotCount]ItemStack
	Offhand            ItemStack
}
