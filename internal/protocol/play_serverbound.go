package protocol

type ConfirmTeleport struct {
	TeleportID int32
}

type ChunkBatchReceived struct {
	ChunksPerTick float32
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

func DecodePlayKeepAliveResponse(data []byte) (PlayKeepAliveResponse, error) {
	rd := NewPacketReader(data)

	response := PlayKeepAliveResponse{ID: rd.Long()}

	err := rd.Err()
	if err != nil {
		return PlayKeepAliveResponse{}, err
	}

	return response, nil
}
