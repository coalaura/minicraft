package game

import "math"

const (
	PlayerPoseStanding PlayerPose = iota
	PlayerPoseCrouching
	PlayerPoseCrawling
)

const (
	standingPlayerEyeHeight       = 1.62
	crouchingPlayerEyeHeight      = 1.27
	crawlingPlayerEyeHeight       = 0.4
	defaultBlockInteractionRange  = 4.5
	defaultEntityInteractionRange = 3.0
)

const (
	DefaultPlayerHealth       = 20
	DefaultPlayerFoodLevel    = 20
	DefaultPlayerSaturation   = 5
	MaxPlayerSaturation       = 20
	DefaultPlayerAirSupply    = 300
	DefaultPlayerAttackDamage = 1
	DefaultPlayerAttackSpeed  = 4
)

type PlayerPose uint8

type PlayerArmorAttributes struct {
	Armor               int32
	Toughness           float32
	KnockbackResistance float32
}

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

	GameMode GameMode
	LivingState
	FoodLevel            int32
	Saturation           float32
	Exhaustion           float32
	FoodTickTimer        int32
	SurvivalTickCount    int64
	AirSupply            int32
	FallDistance         float32
	DeathEntityRemoved   bool
	SurvivalInitialized  bool
	AttackStrengthTicker int32
	LastMainHandItem     Item
	LastMainHandItemSet  bool

	OnGround  bool
	Sneaking  bool
	Sprinting bool
	Swimming  bool
	Pose      PlayerPose

	SelectedHotbarSlot int
	Inventory          PlayerInventory

	UsingItem         bool
	UsingOffhand      bool
	UseRemainingTicks uint16
	UseAnimation      ItemUseAnimation
	UseStack          ItemStack
}

func (player *Player) ResetSurvivalState() {
	player.Reset(DefaultPlayerHealth)
	player.FoodLevel = DefaultPlayerFoodLevel
	player.Saturation = DefaultPlayerSaturation
	player.Exhaustion = 0
	player.FoodTickTimer = 0
	player.SurvivalTickCount = 0
	player.AirSupply = DefaultPlayerAirSupply
	player.FallDistance = 0
	player.DeathEntityRemoved = false

	player.StopUsingItem()

	player.SurvivalInitialized = true
}

func (player Player) Clone() Player {
	clone := player

	clone.Properties = append([]ProfileProperty(nil), player.Properties...)
	clone.Inventory = player.Inventory.Clone()
	clone.ActiveEffects = player.ActiveEffects.Clone()
	clone.UseStack = player.UseStack.Clone()

	return clone
}

func (player *Player) AddExhaustion(amount float32) {
	if amount <= 0 || player.GameMode == GameModeCreative || player.GameMode == GameModeSpectator {
		return
	}

	player.Exhaustion = min(player.Exhaustion+amount, 40)
}

func (player *Player) TickAttackStrength() {
	player.AttackStrengthTicker++

	item := player.MainHandItem()
	if player.LastMainHandItemSet && item == player.LastMainHandItem {
		return
	}

	player.LastMainHandItem = item
	player.LastMainHandItemSet = true
	player.AttackStrengthTicker = 0
}

func (player *Player) ResetAttackStrength() {
	player.AttackStrengthTicker = 0
}

func (player *Player) StopUsingItem() bool {
	if !player.UsingItem {
		return false
	}

	player.UsingItem = false
	player.UsingOffhand = false
	player.UseRemainingTicks = 0
	player.UseAnimation = ItemUseAnimationNone
	player.UseStack = ItemStack{}

	return true
}

func (player Player) MainHandAttackDamage() float32 {
	stack := player.Inventory.Held(player.SelectedHotbarSlot)
	if stack == nil || stack.Empty() {
		return DefaultPlayerAttackDamage
	}

	definition, valid := stack.Item.Definition()
	if !valid {
		return DefaultPlayerAttackDamage
	}

	return DefaultPlayerAttackDamage + definition.AttackDamageModifier
}

func (player Player) MainHandAttackSpeed() float32 {
	stack := player.Inventory.Held(player.SelectedHotbarSlot)
	if stack == nil || stack.Empty() {
		return DefaultPlayerAttackSpeed
	}

	definition, valid := stack.Item.Definition()
	if !valid {
		return DefaultPlayerAttackSpeed
	}

	return DefaultPlayerAttackSpeed + definition.AttackSpeedModifier
}

func (player Player) MainHandItem() Item {
	stack := player.Inventory.Held(player.SelectedHotbarSlot)
	if stack == nil || stack.Empty() {
		return 0
	}

	return stack.Item
}

func (player Player) MainHandDamagePerAttack() int32 {
	stack := player.Inventory.Held(player.SelectedHotbarSlot)
	if stack == nil || stack.Empty() {
		return 0
	}

	definition, valid := stack.Item.Definition()
	if !valid {
		return 0
	}

	return definition.DamagePerAttack
}

func (player Player) ArmorAttributes() PlayerArmorAttributes {
	slots := [...]ItemEquipmentSlot{ItemEquipmentSlotHead, ItemEquipmentSlotChest, ItemEquipmentSlotLegs, ItemEquipmentSlotFeet}
	attributes := PlayerArmorAttributes{}

	for index, slot := range slots {
		armor, valid := player.Inventory.Armor[index].ArmorAttributes(slot)
		if !valid {
			continue
		}

		attributes.Armor += armor.Defense
		attributes.Toughness += armor.Toughness
		attributes.KnockbackResistance += armor.KnockbackResistance
	}

	return attributes
}

func (player Player) AttackStrength() float32 {
	delay := float32(20) / player.MainHandAttackSpeed()
	strength := (float32(player.AttackStrengthTicker) + 0.5) / delay

	return max(0, min(strength, 1))
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

func (player Player) IsWithinEntityInteractionRange(box AABB, buffer float64, inclusive bool) bool {
	eye := player.EyePosition()

	distanceX := eye.X - math.Max(box.MinX, math.Min(eye.X, box.MaxX))
	distanceY := eye.Y - math.Max(box.MinY, math.Min(eye.Y, box.MaxY))
	distanceZ := eye.Z - math.Max(box.MinZ, math.Min(eye.Z, box.MaxZ))

	maximumDistance := defaultEntityInteractionRange + buffer
	distanceSquared := distanceX*distanceX + distanceY*distanceY + distanceZ*distanceZ
	maximumSquared := maximumDistance * maximumDistance

	if inclusive {
		return distanceSquared <= maximumSquared
	}

	return distanceSquared < maximumSquared
}

func DamageAfterArmorAbsorb(damage float32, armor int32, armorToughness float32) float32 {
	toughness := 2 + armorToughness/4
	realArmor := min(max(float32(armor)-damage/toughness, float32(armor)*0.2), 20)
	armorFraction := realArmor / 25

	return damage * (1 - armorFraction)
}
