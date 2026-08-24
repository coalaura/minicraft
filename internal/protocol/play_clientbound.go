package protocol

import "github.com/coalaura/minicraft/internal/game"

const (
	PlayerInfoActionAddPlayer      = 1 << 0
	PlayerInfoActionUpdateGameMode = 1 << 2
	PlayerInfoActionUpdateListed   = 1 << 3

	PlayerEntityType = 155

	EntityFlagsMetadataIndex     = 0
	EntityPoseMetadataIndex      = 6
	PlayerSkinPartsMetadataIndex = 16

	MetadataTypeByte = 0
	MetadataTypePose = 20

	MetadataTerminator = 0xFF

	EntityFlagSneaking  = 0x02
	EntityFlagSprinting = 0x08

	EntityPoseStanding  = 0
	EntityPoseCrouching = 5

	EntityAnimationSwingMainHand = 0
	EntityAnimationSwingOffHand  = 3

	EquipmentSlotMainHand byte = 0
	EquipmentSlotOffHand  byte = 1

	LevelEventBlockBreak = 2001
)

type AddEntity struct {
	EntityID int32
	UUID     string
	Type     int32

	X float64
	Y float64
	Z float64

	VelocityX float64
	VelocityY float64
	VelocityZ float64

	Pitch   byte
	Yaw     byte
	HeadYaw byte
	Data    int32
}

type MetadataValue interface {
	EncodeMetadata(*PacketWriter)
}

type MetadataByte byte

type MetadataVarInt int32

type EntityMetadataEntry struct {
	Index byte
	Type  int32
	Value MetadataValue
}

type EntityMetadata struct {
	EntityID int32
	Entries  []EntityMetadataEntry
}

type EntityAnimation struct {
	EntityID  int32
	Animation byte
}

type EquipmentEntry struct {
	Slot byte
	Item game.ItemStack
}

type EntityEquipment struct {
	EntityID  int32
	Equipment []EquipmentEntry
}

type SynchronizeEntityPosition struct {
	EntityID int32

	X float64
	Y float64
	Z float64

	VelocityX float64
	VelocityY float64
	VelocityZ float64

	Yaw   float32
	Pitch float32

	OnGround bool
}

type UpdateEntityPosition struct {
	EntityID int32

	DeltaX int16
	DeltaY int16
	DeltaZ int16

	OnGround bool
}

type UpdateEntityPositionRotation struct {
	EntityID int32

	DeltaX int16
	DeltaY int16
	DeltaZ int16

	Yaw   byte
	Pitch byte

	OnGround bool
}

type UpdateEntityRotation struct {
	EntityID int32

	Yaw   byte
	Pitch byte

	OnGround bool
}

type SetHeadRotation struct {
	EntityID int32
	HeadYaw  byte
}

type RemoveEntities struct {
	EntityIDs []int32
}

type PlayerInfoRemove struct {
	UUIDs []string
}

type PlayLogin struct {
	EntityID           int32
	Hardcore           bool
	Worlds             []string
	MaxPlayers         int32
	ViewDistance       int32
	SimulationDistance int32
	ReducedDebugInfo   bool
	ShowDeathScreen    bool
	LimitedCrafting    bool
	Spawn              SpawnInfo
	EnforcesSecureChat bool
}

type SpawnInfo struct {
	DimensionType    int32
	Dimension        string
	Seed             int64
	GameMode         byte
	PreviousGameMode byte
	Debug            bool
	Flat             bool
	PortalCooldown   int32
	SeaLevel         int32
}

type PlayerPosition struct {
	TeleportID int32

	X float64
	Y float64
	Z float64

	VelocityX float64
	VelocityY float64
	VelocityZ float64

	Yaw   float32
	Pitch float32

	Flags uint32
}

type PlayerInfoUpdate struct {
	Actions byte
	Players []PlayerInfo
}

type PlayerInfo struct {
	UUID       string
	Name       string
	Properties []game.ProfileProperty
	GameMode   int32
	Listed     bool
}

type SetCenterChunk struct {
	X int32
	Z int32
}

type ForgetLevelChunk struct {
	X int32
	Z int32
}

type ChunkBatchEnd struct {
	BatchSize int32
}

type GameEvent struct {
	Event byte
	Value float32
}

type PlayKeepAlive struct {
	ID int64
}

type BlockChangedAck struct {
	Sequence int32
}

type BlockUpdate struct {
	Position game.BlockPosition
	State    int32
}

type LevelEvent struct {
	Event    int32
	Position game.BlockPosition
	Data     int32
	Global   bool
}

type SystemChat struct {
	Content   string
	ActionBar bool
}

func (p AddEntity) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)
	wr.UUID(p.UUID)
	wr.VarInt(p.Type)

	wr.Double(p.X)
	wr.Double(p.Y)
	wr.Double(p.Z)

	wr.LowPrecisionVector(p.VelocityX, p.VelocityY, p.VelocityZ)

	wr.Byte(p.Pitch)
	wr.Byte(p.Yaw)
	wr.Byte(p.HeadYaw)
	wr.VarInt(p.Data)
}

func (p MetadataByte) EncodeMetadata(wr *PacketWriter) {
	wr.Byte(byte(p))
}

func (p MetadataVarInt) EncodeMetadata(wr *PacketWriter) {
	wr.VarInt(int32(p))
}

func (p EntityMetadata) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)

	for _, entry := range p.Entries {
		wr.Byte(entry.Index)
		wr.VarInt(entry.Type)
		entry.Value.EncodeMetadata(wr)
	}

	wr.Byte(MetadataTerminator)
}

func (p EntityAnimation) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)
	wr.Byte(p.Animation)
}

func (p EntityEquipment) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)

	for index, equipment := range p.Equipment {
		slot := equipment.Slot
		if index < len(p.Equipment)-1 {
			slot |= 0x80
		}

		wr.Byte(slot)
		encodeItemStack(wr, equipment.Item)
	}
}

func encodeItemStack(wr *PacketWriter, stack game.ItemStack) {
	if stack.Empty() {
		wr.VarInt(0)

		return
	}

	wr.VarInt(stack.Count)
	wr.VarInt(int32(stack.Item))
	wr.VarInt(0)
	wr.VarInt(0)
}

func (p SynchronizeEntityPosition) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)

	wr.Double(p.X)
	wr.Double(p.Y)
	wr.Double(p.Z)

	wr.Double(p.VelocityX)
	wr.Double(p.VelocityY)
	wr.Double(p.VelocityZ)

	wr.Float(p.Yaw)
	wr.Float(p.Pitch)

	wr.Bool(p.OnGround)
}

func (p UpdateEntityPosition) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)

	wr.Short(p.DeltaX)
	wr.Short(p.DeltaY)
	wr.Short(p.DeltaZ)

	wr.Bool(p.OnGround)
}

func (p UpdateEntityPositionRotation) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)

	wr.Short(p.DeltaX)
	wr.Short(p.DeltaY)
	wr.Short(p.DeltaZ)

	wr.Byte(p.Yaw)
	wr.Byte(p.Pitch)

	wr.Bool(p.OnGround)
}

func (p UpdateEntityRotation) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)
	wr.Byte(p.Yaw)
	wr.Byte(p.Pitch)
	wr.Bool(p.OnGround)
}

func (p SetHeadRotation) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)
	wr.Byte(p.HeadYaw)
}

func (p RemoveEntities) Encode(wr *PacketWriter) {
	wr.VarInt(int32(len(p.EntityIDs)))

	for _, entityID := range p.EntityIDs {
		wr.VarInt(entityID)
	}
}

func (p PlayerInfoRemove) Encode(wr *PacketWriter) {
	wr.VarInt(int32(len(p.UUIDs)))

	for _, uuid := range p.UUIDs {
		wr.UUID(uuid)
	}
}

func (p PlayLogin) Encode(wr *PacketWriter) {
	wr.Int(p.EntityID)
	wr.Bool(p.Hardcore)

	wr.VarInt(int32(len(p.Worlds)))

	for _, world := range p.Worlds {
		wr.String(world)
	}

	wr.VarInt(p.MaxPlayers)
	wr.VarInt(p.ViewDistance)
	wr.VarInt(p.SimulationDistance)

	wr.Bool(p.ReducedDebugInfo)
	wr.Bool(p.ShowDeathScreen)
	wr.Bool(p.LimitedCrafting)

	p.Spawn.Encode(wr)

	wr.Bool(p.EnforcesSecureChat)
}

func (p SpawnInfo) Encode(wr *PacketWriter) {
	wr.VarInt(p.DimensionType)
	wr.String(p.Dimension)
	wr.Long(p.Seed)
	wr.Byte(p.GameMode)
	wr.Byte(p.PreviousGameMode)
	wr.Bool(p.Debug)
	wr.Bool(p.Flat)

	// No death location.
	wr.Bool(false)

	wr.VarInt(p.PortalCooldown)
	wr.VarInt(p.SeaLevel)
}

func (p PlayerPosition) Encode(wr *PacketWriter) {
	wr.VarInt(p.TeleportID)

	wr.Double(p.X)
	wr.Double(p.Y)
	wr.Double(p.Z)

	wr.Double(p.VelocityX)
	wr.Double(p.VelocityY)
	wr.Double(p.VelocityZ)

	wr.Float(p.Yaw)
	wr.Float(p.Pitch)

	wr.Int(int32(p.Flags))
}

func (p PlayerInfoUpdate) Encode(wr *PacketWriter) {
	wr.Byte(p.Actions)
	wr.VarInt(int32(len(p.Players)))

	for _, player := range p.Players {
		wr.UUID(player.UUID)

		if p.Actions&PlayerInfoActionAddPlayer != 0 {
			wr.String(player.Name)
			encodeProfileProperties(wr, player.Properties)
		}

		if p.Actions&PlayerInfoActionUpdateGameMode != 0 {
			wr.VarInt(player.GameMode)
		}

		if p.Actions&PlayerInfoActionUpdateListed != 0 {
			if player.Listed {
				wr.VarInt(1)
			} else {
				wr.VarInt(0)
			}
		}
	}
}

func (p SetCenterChunk) Encode(wr *PacketWriter) {
	wr.VarInt(p.X)
	wr.VarInt(p.Z)
}

func (p ForgetLevelChunk) Encode(wr *PacketWriter) {
	wr.Int(p.Z)
	wr.Int(p.X)
}

func (p ChunkBatchEnd) Encode(wr *PacketWriter) {
	wr.VarInt(p.BatchSize)
}

func (p GameEvent) Encode(wr *PacketWriter) {
	wr.Byte(p.Event)
	wr.Float(p.Value)
}

func (p PlayKeepAlive) Encode(wr *PacketWriter) {
	wr.Long(p.ID)
}

func (p BlockChangedAck) Encode(wr *PacketWriter) {
	wr.VarInt(p.Sequence)
}

func (p BlockUpdate) Encode(wr *PacketWriter) {
	wr.BlockPosition(p.Position)
	wr.VarInt(p.State)
}

func (p LevelEvent) Encode(wr *PacketWriter) {
	wr.Int(p.Event)
	wr.BlockPosition(p.Position)
	wr.Int(p.Data)
	wr.Bool(p.Global)
}

func (p SystemChat) Encode(wr *PacketWriter) {
	wr.AnonymousNBTString(p.Content)
	wr.Bool(p.ActionBar)
}
