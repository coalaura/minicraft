package protocol

// gost:preserve-layout
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

func (p PlayLogin) Encode(w *PacketWriter) {
	w.Int(p.EntityID)
	w.Bool(p.Hardcore)

	w.VarInt(int32(len(p.Worlds)))

	for _, world := range p.Worlds {
		w.String(world)
	}

	w.VarInt(p.MaxPlayers)
	w.VarInt(p.ViewDistance)
	w.VarInt(p.SimulationDistance)

	w.Bool(p.ReducedDebugInfo)
	w.Bool(p.ShowDeathScreen)
	w.Bool(p.LimitedCrafting)

	p.Spawn.Encode(w)

	w.Bool(p.EnforcesSecureChat)
}

// gost:preserve-layout
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

func (p SpawnInfo) Encode(w *PacketWriter) {
	w.VarInt(p.DimensionType)
	w.String(p.Dimension)
	w.Long(p.Seed)
	w.Byte(p.GameMode)
	w.Byte(p.PreviousGameMode)
	w.Bool(p.Debug)
	w.Bool(p.Flat)

	// No death location.
	w.Bool(false)

	w.VarInt(p.PortalCooldown)
	w.VarInt(p.SeaLevel)
}

// gost:preserve-layout
type PlayerPosition struct {
	TeleportID int32

	Position Vec3
	Velocity Vec3

	Yaw   float32
	Pitch float32

	Flags uint32
}

func (p PlayerPosition) Encode(w *PacketWriter) {
	w.VarInt(p.TeleportID)

	w.Double(p.Position.X)
	w.Double(p.Position.Y)
	w.Double(p.Position.Z)

	w.Double(p.Velocity.X)
	w.Double(p.Velocity.Y)
	w.Double(p.Velocity.Z)

	w.Float(p.Yaw)
	w.Float(p.Pitch)

	w.Int(int32(p.Flags))
}
