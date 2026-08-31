package protocol

import (
	"errors"
	"unicode/utf8"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	PlayerInfoActionAddPlayer      = 1 << 0
	PlayerInfoActionInitializeChat = 1 << 1
	PlayerInfoActionUpdateGameMode = 1 << 2
	PlayerInfoActionUpdateListed   = 1 << 3

	PlayerEntityType = 155
	ItemEntityType   = 71

	EntityFlagsMetadataIndex     = 0
	EntityPoseMetadataIndex      = 6
	ItemEntityItemMetadataIndex  = 8
	PlayerSkinPartsMetadataIndex = 16

	MetadataTypeByte      = 0
	MetadataTypeItemStack = 7
	MetadataTypePose      = 20

	MetadataTerminator = 0xFF

	EntityFlagSneaking  = 0x02
	EntityFlagSprinting = 0x08
	EntityFlagSwimming  = 0x10

	EntityPoseStanding  = 0
	EntityPoseSwimming  = 3
	EntityPoseCrouching = 5

	EntityAnimationSwingMainHand = 0
	EntityAnimationSwingOffHand  = 3

	EquipmentSlotMainHand byte = 0
	EquipmentSlotOffHand  byte = 1
	EquipmentSlotFeet     byte = 2
	EquipmentSlotLegs     byte = 3
	EquipmentSlotChest    byte = 4
	EquipmentSlotHead     byte = 5

	LevelEventLavaFizz   = 1501
	LevelEventBlockBreak = 2001
	SoundSourceBlock     = 4

	maxCommandTreeNodes       = 32767
	maxCommandNodeChildren    = 32767
	maxCommandSuggestionCount = 32767

	CommandNodeRoot     = 0
	CommandNodeLiteral  = 1
	CommandNodeArgument = 2

	CommandParserBool             = 0
	CommandParserFloat            = 1
	CommandParserDouble           = 2
	CommandParserInteger          = 3
	CommandParserLong             = 4
	CommandParserString           = 5
	CommandParserEntity           = 6
	CommandParserBlockPosition    = 8
	CommandParserVec3             = 10
	CommandParserBlockState       = 12
	CommandParserItemStack        = 14
	CommandParserScore            = 31
	CommandParserGameMode         = 42
	CommandParserTime             = 43
	CommandParserResource         = 46
	CommandParserResourceKey      = 47
	CommandParserResourceSelector = 48
	CommandParserResourceOrTag    = 44
	CommandParserResourceOrTagKey = 45
	CommandParserUUID             = 56
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

type MetadataItemStack struct {
	Stack game.ItemStack
}

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

type SetEntityMotion struct {
	EntityID int32

	VelocityX float64
	VelocityY float64
	VelocityZ float64
}

type TakeItemEntity struct {
	ItemEntityID   int32
	PlayerEntityID int32
	Amount         int32
}

type EquipmentEntry struct {
	Slot byte
	Item game.ItemStack
}

type EntityEquipment struct {
	EntityID  int32
	Equipment []EquipmentEntry
}

type ContainerSetContent struct {
	WindowID    int32
	StateID     int32
	Items       []game.ItemStack
	CarriedItem game.ItemStack
}

type ContainerSetSlot struct {
	WindowID int32
	StateID  int32
	Slot     int16
	Item     game.ItemStack
}

type ContainerSetData struct {
	ContainerID int32
	ID          int16
	Value       int16
}

type RecipePropertySet struct {
	Name  string
	Items []game.Item
}

type UpdateRecipes struct {
	PropertySets []RecipePropertySet
}

type CloseContainer struct {
	ContainerID int32
}

type OpenScreen struct {
	ContainerID int32
	MenuType    int32
	Title       game.TextComponent
}

type SetHeldSlot struct {
	Slot int32
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
	UUID        string
	Name        string
	Properties  []game.ProfileProperty
	GameMode    int32
	Listed      bool
	ChatSession *ChatSession
}

type ChatSession struct {
	UUID                 string
	ExpiresAt            int64
	PublicKey            []byte
	CertificateSignature []byte
}

type PreviousChatSignature struct {
	ID        int32
	Signature [chatMessageSignatureLength]byte
}

type ChatType struct {
	TranslationKey string
	Parameters     []int32
	Style          string
}

type ChatTypes struct {
	Chat      ChatType
	Narration ChatType
}

type ChatTypesHolder struct {
	ID   int32
	Data ChatTypes
}

type PlayerChat struct {
	GlobalIndex int32
	SenderUUID  string
	SenderIndex int32

	HasSignature bool
	Signature    [chatMessageSignatureLength]byte
	PlainMessage string
	Timestamp    int64
	Salt         int64

	PreviousSignatures []PreviousChatSignature
	HasUnsignedContent bool
	UnsignedContent    string
	FilterType         int32
	FilterMask         []int64
	Type               ChatTypesHolder
	NetworkName        string
	HasNetworkTarget   bool
	NetworkTarget      string
}

type PlayDisconnect struct {
	Reason string
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

type EntityEvent struct {
	EntityID int32
	Event    byte
}

type PlayKeepAlive struct {
	ID int64
}

type UpdateTime struct {
	Age         int64
	Time        int64
	TickDayTime bool
}

type BlockChangedAck struct {
	Sequence int32
}

type BlockDestruction struct {
	EntityID int32
	Position game.BlockPosition
	Stage    int8
}

type BlockUpdate struct {
	Position game.BlockPosition
	State    int32
}

type BlockEvent struct {
	Position game.BlockPosition
	Event    byte
	Param    byte
	Block    int32
}

type SectionBlockUpdateRecord struct {
	X     byte
	Y     byte
	Z     byte
	State int32
}

type SectionBlocksUpdate struct {
	SectionX int32
	SectionY int32
	SectionZ int32
	Records  []SectionBlockUpdateRecord
}

type LevelEvent struct {
	Event    int32
	Position game.BlockPosition
	Data     int32
	Global   bool
}

type SoundEventHolder struct {
	RegistryID int32
	Name       string
	FixedRange *float32
}

type Sound struct {
	Event  SoundEventHolder
	Source int32
	X      float64
	Y      float64
	Z      float64
	Volume float32
	Pitch  float32
	Seed   int64
}

type SystemChat struct {
	Content   game.TextComponent
	ActionBar bool
}

type CommandNode struct {
	Type           byte
	Executable     bool
	Restricted     bool
	Children       []int32
	Redirect       int32
	HasRedirect    bool
	Name           string
	Parser         int32
	Properties     CommandParserProperties
	SuggestionType string
}

type CommandParserProperties interface {
	encodeCommandParserProperties(*PacketWriter)
}

type CommandFloatProperties struct {
	HasMin bool
	HasMax bool
	Min    float32
	Max    float32
}

type CommandDoubleProperties struct {
	HasMin bool
	HasMax bool
	Min    float64
	Max    float64
}

type CommandIntegerProperties struct {
	HasMin bool
	HasMax bool
	Min    int32
	Max    int32
}

type CommandLongProperties struct {
	HasMin bool
	HasMax bool
	Min    int64
	Max    int64
}

type CommandStringProperties struct {
	Type int32
}

type CommandEntityProperties struct {
	SingleTarget bool
	OnlyPlayers  bool
}

type CommandScoreHolderProperties struct {
	AllowMultiple bool
}

type CommandTimeProperties struct {
	Min int32
}

type CommandResourceProperties struct {
	Registry string
}

type DeclareCommands struct {
	Nodes     []CommandNode
	RootIndex int32
}

type CommandSuggestion struct {
	Text       string
	HasTooltip bool
	Tooltip    string
}

type CommandSuggestions struct {
	TransactionID int32
	Start         int32
	Length        int32
	Matches       []CommandSuggestion
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

func (p MetadataItemStack) EncodeMetadata(wr *PacketWriter) {
	encodeItemStack(wr, p.Stack)
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

func (p SetEntityMotion) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)
	wr.LowPrecisionVector(p.VelocityX, p.VelocityY, p.VelocityZ)
}

func (p TakeItemEntity) Encode(wr *PacketWriter) {
	wr.VarInt(p.ItemEntityID)
	wr.VarInt(p.PlayerEntityID)
	wr.VarInt(p.Amount)
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

func (p ContainerSetContent) Encode(wr *PacketWriter) {
	wr.VarInt(p.WindowID)
	wr.VarInt(p.StateID)
	wr.VarInt(int32(len(p.Items)))

	for _, item := range p.Items {
		encodeItemStack(wr, item)
	}

	encodeItemStack(wr, p.CarriedItem)
}

func (p ContainerSetSlot) Encode(wr *PacketWriter) {
	wr.VarInt(p.WindowID)
	wr.VarInt(p.StateID)
	wr.Short(p.Slot)
	encodeItemStack(wr, p.Item)
}

func (p ContainerSetData) Encode(wr *PacketWriter) {
	wr.Byte(byte(p.ContainerID))
	wr.Short(p.ID)
	wr.Short(p.Value)
}

func (p UpdateRecipes) Encode(wr *PacketWriter) {
	wr.VarInt(int32(len(p.PropertySets)))

	for _, propertySet := range p.PropertySets {
		wr.String(propertySet.Name)
		wr.VarInt(int32(len(propertySet.Items)))

		for _, item := range propertySet.Items {
			wr.VarInt(int32(item))
		}
	}

	// Stonecutter recipes are not implemented yet.
	wr.VarInt(0)
}

func (p CloseContainer) Encode(wr *PacketWriter) {
	wr.VarInt(p.ContainerID)
}

func (p OpenScreen) Encode(wr *PacketWriter) {
	wr.VarInt(p.ContainerID)
	wr.VarInt(p.MenuType)
	wr.AnonymousNBTText(p.Title)
}

func (p SetHeldSlot) Encode(wr *PacketWriter) {
	wr.VarInt(p.Slot)
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

		if p.Actions&PlayerInfoActionInitializeChat != 0 {
			wr.Bool(player.ChatSession != nil)

			if player.ChatSession != nil {
				player.ChatSession.Encode(wr)
			}
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

func (p ChatSession) Encode(wr *PacketWriter) {
	wr.UUID(p.UUID)
	wr.Long(p.ExpiresAt)
	wr.Bytes(p.PublicKey)
	wr.Bytes(p.CertificateSignature)
}

func (p PlayerChat) Encode(wr *PacketWriter) {
	wr.VarInt(p.GlobalIndex)
	wr.UUID(p.SenderUUID)
	wr.VarInt(p.SenderIndex)
	wr.Bool(p.HasSignature)

	if p.HasSignature {
		for _, value := range p.Signature {
			wr.Byte(value)
		}
	}

	wr.String(p.PlainMessage)
	wr.Long(p.Timestamp)
	wr.Long(p.Salt)
	wr.VarInt(int32(len(p.PreviousSignatures)))

	for _, signature := range p.PreviousSignatures {
		wr.VarInt(signature.ID)

		if signature.ID == 0 {
			for _, value := range signature.Signature {
				wr.Byte(value)
			}
		}
	}

	wr.Bool(p.HasUnsignedContent)

	if p.HasUnsignedContent {
		wr.AnonymousNBTString(p.UnsignedContent)
	}

	wr.VarInt(p.FilterType)

	if p.FilterType == 2 {
		wr.VarInt(int32(len(p.FilterMask)))

		for _, mask := range p.FilterMask {
			wr.Long(mask)
		}
	}

	p.Type.Encode(wr)

	wr.AnonymousNBTString(p.NetworkName)
	wr.Bool(p.HasNetworkTarget)

	if p.HasNetworkTarget {
		wr.AnonymousNBTString(p.NetworkTarget)
	}
}

func (p ChatTypesHolder) Encode(wr *PacketWriter) {
	wr.VarInt(p.ID)

	if p.ID != 0 {
		return
	}

	p.Data.Encode(wr)
}

func (p ChatTypes) Encode(wr *PacketWriter) {
	p.Chat.Encode(wr)
	p.Narration.Encode(wr)
}

func (p ChatType) Encode(wr *PacketWriter) {
	wr.String(p.TranslationKey)
	wr.VarInt(int32(len(p.Parameters)))

	for _, parameter := range p.Parameters {
		wr.VarInt(parameter)
	}

	wr.AnonymousNBTString(p.Style)
}

func (p PlayDisconnect) Encode(wr *PacketWriter) {
	wr.AnonymousNBTString(p.Reason)
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

func (p EntityEvent) Encode(wr *PacketWriter) {
	wr.Int(p.EntityID)
	wr.Byte(p.Event)
}

func (p PlayKeepAlive) Encode(wr *PacketWriter) {
	wr.Long(p.ID)
}

func (p UpdateTime) Encode(wr *PacketWriter) {
	wr.Long(p.Age)
	wr.Long(p.Time)
	wr.Bool(p.TickDayTime)
}

func (p BlockChangedAck) Encode(wr *PacketWriter) {
	wr.VarInt(p.Sequence)
}

func (p BlockDestruction) Encode(wr *PacketWriter) {
	wr.VarInt(p.EntityID)
	wr.BlockPosition(p.Position)
	wr.Byte(byte(p.Stage))
}

func (p BlockUpdate) Encode(wr *PacketWriter) {
	wr.BlockPosition(p.Position)
	wr.VarInt(p.State)
}

func (p BlockEvent) Encode(wr *PacketWriter) {
	wr.BlockPosition(p.Position)
	wr.Byte(p.Event)
	wr.Byte(p.Param)
	wr.VarInt(p.Block)
}

func (p SectionBlocksUpdate) Encode(wr *PacketWriter) {
	sectionPosition := (int64(p.SectionX)&0x3FFFFF)<<42 |
		(int64(p.SectionZ)&0x3FFFFF)<<20 |
		int64(p.SectionY)&0xFFFFF

	wr.Long(sectionPosition)
	wr.VarInt(int32(len(p.Records)))

	for _, record := range p.Records {
		if record.X > 15 || record.Y > 15 || record.Z > 15 || record.State < 0 {
			wr.err = errors.New("invalid section block update")

			return
		}

		packed := record.State<<12 | int32(record.X)<<8 | int32(record.Z)<<4 | int32(record.Y)
		wr.VarInt(packed)
	}
}

func (p LevelEvent) Encode(wr *PacketWriter) {
	wr.Int(p.Event)
	wr.BlockPosition(p.Position)
	wr.Int(p.Data)
	wr.Bool(p.Global)
}

func (p SoundEventHolder) Encode(wr *PacketWriter) {
	if p.Name == "" {
		wr.VarInt(p.RegistryID + 1)

		return
	}

	wr.VarInt(0)
	wr.String(p.Name)
	wr.Bool(p.FixedRange != nil)

	if p.FixedRange != nil {
		wr.Float(*p.FixedRange)
	}
}

func (p Sound) Encode(wr *PacketWriter) {
	p.Event.Encode(wr)

	wr.VarInt(p.Source)
	wr.Int(int32(p.X * 8))
	wr.Int(int32(p.Y * 8))
	wr.Int(int32(p.Z * 8))
	wr.Float(p.Volume)
	wr.Float(p.Pitch)
	wr.Long(p.Seed)
}

func (p SystemChat) Encode(wr *PacketWriter) {
	wr.AnonymousNBTText(p.Content)
	wr.Bool(p.ActionBar)
}

func (p CommandFloatProperties) encodeCommandParserProperties(wr *PacketWriter) {
	flags := byte(0)

	if p.HasMin {
		flags |= 1
	}

	if p.HasMax {
		flags |= 2
	}

	wr.Byte(flags)

	if p.HasMin {
		wr.Float(p.Min)
	}

	if p.HasMax {
		wr.Float(p.Max)
	}
}

func (p CommandDoubleProperties) encodeCommandParserProperties(wr *PacketWriter) {
	flags := byte(0)

	if p.HasMin {
		flags |= 1
	}

	if p.HasMax {
		flags |= 2
	}

	wr.Byte(flags)

	if p.HasMin {
		wr.Double(p.Min)
	}

	if p.HasMax {
		wr.Double(p.Max)
	}
}

func (p CommandIntegerProperties) encodeCommandParserProperties(wr *PacketWriter) {
	flags := byte(0)

	if p.HasMin {
		flags |= 1
	}

	if p.HasMax {
		flags |= 2
	}

	wr.Byte(flags)

	if p.HasMin {
		wr.Int(p.Min)
	}

	if p.HasMax {
		wr.Int(p.Max)
	}
}

func (p CommandLongProperties) encodeCommandParserProperties(wr *PacketWriter) {
	flags := byte(0)

	if p.HasMin {
		flags |= 1
	}

	if p.HasMax {
		flags |= 2
	}

	wr.Byte(flags)

	if p.HasMin {
		wr.Long(p.Min)
	}

	if p.HasMax {
		wr.Long(p.Max)
	}
}

func (p CommandStringProperties) encodeCommandParserProperties(wr *PacketWriter) {
	wr.VarInt(p.Type)
}

func (p CommandEntityProperties) encodeCommandParserProperties(wr *PacketWriter) {
	wr.Byte(boolBits(p.SingleTarget, p.OnlyPlayers))
}

func (p CommandScoreHolderProperties) encodeCommandParserProperties(wr *PacketWriter) {
	wr.Bool(p.AllowMultiple)
}

func (p CommandTimeProperties) encodeCommandParserProperties(wr *PacketWriter) {
	wr.Int(p.Min)
}

func (p CommandResourceProperties) encodeCommandParserProperties(wr *PacketWriter) {
	wr.String(p.Registry)
}

func (p DeclareCommands) Encode(wr *PacketWriter) {
	if len(p.Nodes) == 0 || len(p.Nodes) > maxCommandTreeNodes || p.RootIndex < 0 || int(p.RootIndex) >= len(p.Nodes) {
		wr.err = errors.New("invalid command tree")

		return
	}

	for _, node := range p.Nodes {
		for _, child := range node.Children {
			if child < 0 || int(child) >= len(p.Nodes) {
				wr.err = errors.New("invalid command node child")

				return
			}
		}

		if node.HasRedirect && (node.Redirect < 0 || int(node.Redirect) >= len(p.Nodes)) {
			wr.err = errors.New("invalid command node redirect")

			return
		}
	}

	wr.VarInt(int32(len(p.Nodes)))

	for _, node := range p.Nodes {
		node.Encode(wr)
	}

	wr.VarInt(p.RootIndex)
}

func (p CommandNode) Encode(wr *PacketWriter) {
	if p.Type > CommandNodeArgument || len(p.Children) > maxCommandNodeChildren || (p.Type != CommandNodeRoot && !validCommandNodeName(p.Name)) || (p.Type != CommandNodeArgument && p.SuggestionType != "") {
		wr.err = errors.New("invalid command node")

		return
	}

	flags := p.Type

	if p.Executable {
		flags |= 0x04
	}

	if p.HasRedirect {
		flags |= 0x08
	}

	if p.SuggestionType != "" {
		flags |= 0x10
	}

	if p.Restricted {
		flags |= 0x20
	}

	wr.Byte(flags)
	wr.VarInt(int32(len(p.Children)))

	for _, child := range p.Children {
		if child < 0 {
			wr.err = errors.New("invalid command node child")

			return
		}

		wr.VarInt(child)
	}

	if p.HasRedirect {
		if p.Redirect < 0 {
			wr.err = errors.New("invalid command node redirect")

			return
		}

		wr.VarInt(p.Redirect)
	}

	if p.Type == CommandNodeRoot {
		return
	}

	wr.String(p.Name)

	if p.Type == CommandNodeLiteral {
		return
	}

	if !validCommandParser(p.Parser, p.Properties) {
		wr.err = errors.New("invalid command parser properties")

		return
	}

	wr.VarInt(p.Parser)

	if p.Properties != nil {
		p.Properties.encodeCommandParserProperties(wr)
	}

	if p.SuggestionType != "" {
		if !validCommandNodeName(p.SuggestionType) {
			wr.err = errors.New("invalid command suggestion type")

			return
		}

		wr.String(p.SuggestionType)
	}
}

func (p CommandSuggestions) Encode(wr *PacketWriter) {
	if p.Start < 0 || p.Length < 0 || len(p.Matches) > maxCommandSuggestionCount {
		wr.err = errors.New("invalid command suggestions")

		return
	}

	wr.VarInt(p.TransactionID)
	wr.VarInt(p.Start)
	wr.VarInt(p.Length)
	wr.VarInt(int32(len(p.Matches)))

	for _, match := range p.Matches {
		if !utf8.ValidString(match.Text) || utf8.RuneCountInString(match.Text) > maxCommandCharacters {
			wr.err = errors.New("invalid command suggestion")

			return
		}

		wr.String(match.Text)
		wr.Bool(match.HasTooltip)

		if match.HasTooltip {
			wr.AnonymousNBTString(match.Tooltip)
		}
	}
}

func encodeItemStack(wr *PacketWriter, stack game.ItemStack) {
	if stack.Empty() {
		wr.VarInt(0)

		return
	}

	stack.NormalizeComponents()

	wr.VarInt(stack.Count)
	wr.VarInt(int32(stack.Item))
	wr.VarInt(int32(len(stack.Components)))
	wr.VarInt(int32(len(stack.RemovedComponents)))

	for _, component := range stack.Components {
		wr.VarInt(component.Type)
		wr.Raw(component.Data)
	}

	for _, componentType := range stack.RemovedComponents {
		wr.VarInt(componentType)
	}
}

func validCommandNodeName(value string) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxCommandCharacters
}

func validCommandParser(parser int32, properties CommandParserProperties) bool {
	switch parser {
	case CommandParserFloat:
		_, ok := properties.(CommandFloatProperties)
		return ok
	case CommandParserDouble:
		_, ok := properties.(CommandDoubleProperties)
		return ok
	case CommandParserInteger:
		_, ok := properties.(CommandIntegerProperties)
		return ok
	case CommandParserLong:
		_, ok := properties.(CommandLongProperties)
		return ok
	case CommandParserString:
		stringProperties, ok := properties.(CommandStringProperties)
		return ok && stringProperties.Type >= 0 && stringProperties.Type <= 2
	case CommandParserEntity:
		_, ok := properties.(CommandEntityProperties)
		return ok
	case CommandParserScore:
		_, ok := properties.(CommandScoreHolderProperties)
		return ok
	case CommandParserTime:
		_, ok := properties.(CommandTimeProperties)
		return ok
	case CommandParserResource, CommandParserResourceKey, CommandParserResourceOrTag, CommandParserResourceOrTagKey, CommandParserResourceSelector:
		_, ok := properties.(CommandResourceProperties)
		return ok
	default:
		return parser >= 0 && parser <= CommandParserUUID && properties == nil
	}
}

func boolBits(first, second bool) byte {
	var value byte

	if first {
		value |= 1
	}

	if second {
		value |= 2
	}

	return value
}
