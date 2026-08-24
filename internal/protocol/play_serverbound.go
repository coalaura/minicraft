package protocol

const (
	PlayerCommandStartSprinting = 1
	PlayerCommandStopSprinting  = 2

	PlayerInputSneak  = 0x20
	PlayerInputSprint = 0x40

	MainHand = 0
	OffHand  = 1
)

type ConfirmTeleport struct {
	TeleportID int32
}

type ChunkBatchReceived struct {
	ChunksPerTick float32
}

type ClientInformation struct {
	Locale         string
	ViewDistance   int8
	ChatMode       int32
	ChatColors     bool
	SkinParts      byte
	MainHand       int32
	TextFiltering  bool
	ServerListing  bool
	ParticleStatus int32
}

type MovePlayerPosition struct {
	X        float64
	Y        float64
	Z        float64
	OnGround bool
}

type MovePlayerPositionRotation struct {
	X        float64
	Y        float64
	Z        float64
	Yaw      float32
	Pitch    float32
	OnGround bool
}

type MovePlayerRotation struct {
	Yaw      float32
	Pitch    float32
	OnGround bool
}

type MovePlayerStatus struct {
	OnGround bool
}

type PlayerCommand struct {
	EntityID int32
	Action   int32
	Data     int32
}

type PlayerInput struct {
	Flags byte
}

type SwingArm struct {
	Hand int32
}

type PlayKeepAliveResponse struct {
	ID int64
}

func DecodeConfirmTeleport(data []byte) (ConfirmTeleport, error) {
	rd := NewPacketReader(data)

	teleportID := rd.VarInt()

	err := rd.Err()
	if err != nil {
		return ConfirmTeleport{}, err
	}

	return ConfirmTeleport{TeleportID: teleportID}, nil
}

func DecodeChunkBatchReceived(data []byte) (ChunkBatchReceived, error) {
	rd := NewPacketReader(data)

	chunksPerTick := rd.Float()

	err := rd.Err()
	if err != nil {
		return ChunkBatchReceived{}, err
	}

	return ChunkBatchReceived{ChunksPerTick: chunksPerTick}, nil
}

func DecodeClientInformation(data []byte) (ClientInformation, error) {
	rd := NewPacketReader(data)

	information := ClientInformation{
		Locale:         rd.String(16),
		ViewDistance:   int8(rd.Byte()),
		ChatMode:       rd.VarInt(),
		ChatColors:     rd.Bool(),
		SkinParts:      rd.Byte(),
		MainHand:       rd.VarInt(),
		TextFiltering:  rd.Bool(),
		ServerListing:  rd.Bool(),
		ParticleStatus: rd.VarInt(),
	}

	err := rd.Err()
	if err != nil {
		return ClientInformation{}, err
	}

	return information, nil
}

func DecodeMovePlayerPosition(data []byte) (MovePlayerPosition, error) {
	rd := NewPacketReader(data)

	move := MovePlayerPosition{
		X:        rd.Double(),
		Y:        rd.Double(),
		Z:        rd.Double(),
		OnGround: rd.Bool(),
	}

	err := rd.Err()
	if err != nil {
		return MovePlayerPosition{}, err
	}

	return move, nil
}

func DecodeMovePlayerPositionRotation(data []byte) (MovePlayerPositionRotation, error) {
	rd := NewPacketReader(data)

	move := MovePlayerPositionRotation{
		X:        rd.Double(),
		Y:        rd.Double(),
		Z:        rd.Double(),
		Yaw:      rd.Float(),
		Pitch:    rd.Float(),
		OnGround: rd.Bool(),
	}

	err := rd.Err()
	if err != nil {
		return MovePlayerPositionRotation{}, err
	}

	return move, nil
}

func DecodeMovePlayerRotation(data []byte) (MovePlayerRotation, error) {
	rd := NewPacketReader(data)

	move := MovePlayerRotation{
		Yaw:      rd.Float(),
		Pitch:    rd.Float(),
		OnGround: rd.Bool(),
	}

	err := rd.Err()
	if err != nil {
		return MovePlayerRotation{}, err
	}

	return move, nil
}

func DecodeMovePlayerStatus(data []byte) (MovePlayerStatus, error) {
	rd := NewPacketReader(data)

	move := MovePlayerStatus{
		OnGround: rd.Bool(),
	}

	err := rd.Err()
	if err != nil {
		return MovePlayerStatus{}, err
	}

	return move, nil
}

func DecodePlayerCommand(data []byte) (PlayerCommand, error) {
	rd := NewPacketReader(data)

	command := PlayerCommand{
		EntityID: rd.VarInt(),
		Action:   rd.VarInt(),
		Data:     rd.VarInt(),
	}

	err := rd.Err()
	if err != nil {
		return PlayerCommand{}, err
	}

	return command, nil
}

func DecodePlayerInput(data []byte) (PlayerInput, error) {
	rd := NewPacketReader(data)

	input := PlayerInput{Flags: rd.Byte()}

	err := rd.Err()
	if err != nil {
		return PlayerInput{}, err
	}

	return input, nil
}

func DecodeSwingArm(data []byte) (SwingArm, error) {
	rd := NewPacketReader(data)

	swing := SwingArm{Hand: rd.VarInt()}

	err := rd.Err()
	if err != nil {
		return SwingArm{}, err
	}

	return swing, nil
}

func DecodePlayKeepAliveResponse(data []byte) (PlayKeepAliveResponse, error) {
	rd := NewPacketReader(data)

	response := PlayKeepAliveResponse{ID: rd.Long()}

	err := rd.Err()
	if err != nil {
		return PlayKeepAliveResponse{}, err
	}

	return response, nil
}
