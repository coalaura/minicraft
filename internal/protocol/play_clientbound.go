package protocol

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

type SetCenterChunk struct {
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

func (p SetCenterChunk) Encode(wr *PacketWriter) {
	wr.VarInt(p.X)
	wr.VarInt(p.Z)
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
