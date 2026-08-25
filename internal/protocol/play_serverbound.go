package protocol

import (
	"fmt"
	"unicode/utf8"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	PlayerCommandStartSprinting = 1
	PlayerCommandStopSprinting  = 2

	PlayerInputSneak  = 0x20
	PlayerInputSprint = 0x40

	MainHand = 0
	OffHand  = 1

	PlayerActionStartDestroyBlock = 0
	PlayerActionAbortDestroyBlock = 1
	PlayerActionStopDestroyBlock  = 2
	PlayerActionDropAllItems      = 3
	PlayerActionDropItem          = 4
	PlayerActionSwapWithOffhand   = 6

	BlockFaceDown  = 0
	BlockFaceUp    = 1
	BlockFaceNorth = 2
	BlockFaceSouth = 3
	BlockFaceWest  = 4
	BlockFaceEast  = 5

	maxUntrustedSlotComponents = 1024
	maxUntrustedComponentBytes = 1 << 20
	maxContainerChangedSlots   = 128
	maxChatMessageCharacters   = 256
	chatMessageSignatureLength = 256
	chatAcknowledgementLength  = 3
	maxChatSessionBytes        = 4096
)

type MovementFlags byte

const (
	MovementFlagOnGround MovementFlags = 1 << iota
	MovementFlagHorizontalCollision
)

type ConfirmTeleport struct {
	TeleportID int32
}

type ChunkBatchReceived struct {
	ChunksPerTick float32
}

type ChatMessage struct {
	Message      string
	Timestamp    int64
	Salt         int64
	HasSignature bool
	Signature    [chatMessageSignatureLength]byte
	Offset       int32
	Acknowledged [chatAcknowledgementLength]byte
	Checksum     byte
}

type ChatAck struct {
	MessageCount int32
}

type ChatSessionUpdate struct {
	SessionUUID          string
	ExpiresAt            int64
	PublicKey            []byte
	CertificateSignature []byte
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
	X     float64
	Y     float64
	Z     float64
	Flags MovementFlags
}

type MovePlayerPositionRotation struct {
	X     float64
	Y     float64
	Z     float64
	Yaw   float32
	Pitch float32
	Flags MovementFlags
}

type MovePlayerRotation struct {
	Yaw   float32
	Pitch float32
	Flags MovementFlags
}

type MovePlayerStatus struct {
	Flags MovementFlags
}

type PlayerCommand struct {
	EntityID int32
	Action   int32
	Data     int32
}

type PlayerAction struct {
	Status   int32
	Position game.BlockPosition
	Face     int8
	Sequence int32
}

type PlayerInput struct {
	Flags byte
}

type SetHeldItem struct {
	Slot int16
}

type UntrustedSlotComponent struct {
	Type int32
	Data []byte
}

type UntrustedSlot struct {
	ItemCount         int32
	ItemID            int32
	Components        []UntrustedSlotComponent
	RemovedComponents []int32
}

type HashedSlotComponent struct {
	Type int32
	Hash int32
}

type HashedSlot struct {
	Present           bool
	ItemID            int32
	ItemCount         int32
	Components        []HashedSlotComponent
	RemovedComponents []int32
}

type ChangedSlot struct {
	Location int16
	Item     HashedSlot
}

type ContainerClick struct {
	WindowID     int32
	StateID      int32
	Slot         int16
	MouseButton  int8
	Mode         int32
	ChangedSlots []ChangedSlot
	CursorItem   HashedSlot
}

type SetCreativeModeSlot struct {
	Slot int16
	Item UntrustedSlot
}

type SwingArm struct {
	Hand int32
}

type UseItemOn struct {
	Hand           int32
	Position       game.BlockPosition
	Face           int32
	CursorX        float32
	CursorY        float32
	CursorZ        float32
	InsideBlock    bool
	WorldBorderHit bool
	Sequence       int32
}

type PlayKeepAliveResponse struct {
	ID int64
}

func (f MovementFlags) OnGround() bool {
	return f&MovementFlagOnGround != 0
}

func (f MovementFlags) HasHorizontalCollision() bool {
	return f&MovementFlagHorizontalCollision != 0
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

func DecodeChatMessage(data []byte) (ChatMessage, error) {
	rd := NewPacketReader(data)

	message := ChatMessage{
		Message:   rd.String(maxChatMessageCharacters),
		Timestamp: rd.Long(),
		Salt:      rd.Long(),
	}

	message.HasSignature = rd.Bool()
	if message.HasSignature {
		for index := range message.Signature {
			message.Signature[index] = rd.Byte()
		}
	}

	message.Offset = rd.VarInt()

	for index := range message.Acknowledged {
		message.Acknowledged[index] = rd.Byte()
	}

	message.Checksum = rd.Byte()

	err := rd.Err()
	if err != nil {
		return ChatMessage{}, err
	}

	if rd.Len() != 0 {
		return ChatMessage{}, fmt.Errorf("chat message has %d trailing bytes", rd.Len())
	}

	if !validChatMessage(message.Message) {
		return ChatMessage{}, fmt.Errorf("invalid chat message")
	}

	if message.Offset < 0 {
		return ChatMessage{}, fmt.Errorf("invalid chat acknowledgement offset %d", message.Offset)
	}

	return message, nil
}

func DecodeChatAck(data []byte) (ChatAck, error) {
	rd := NewPacketReader(data)

	acknowledgement := ChatAck{MessageCount: rd.VarInt()}

	err := rd.Err()
	if err != nil {
		return ChatAck{}, err
	}

	if acknowledgement.MessageCount < 0 {
		return ChatAck{}, fmt.Errorf("invalid chat acknowledgement count %d", acknowledgement.MessageCount)
	}

	if rd.Len() != 0 {
		return ChatAck{}, fmt.Errorf("chat acknowledgement has %d trailing bytes", rd.Len())
	}

	return acknowledgement, nil
}

func DecodeChatSessionUpdate(data []byte) (ChatSessionUpdate, error) {
	rd := NewPacketReader(data)

	update := ChatSessionUpdate{
		SessionUUID:          rd.UUID(),
		ExpiresAt:            rd.Long(),
		PublicKey:            rd.BytesMax(maxChatSessionBytes),
		CertificateSignature: rd.BytesMax(maxChatSessionBytes),
	}

	err := rd.Err()
	if err != nil {
		return ChatSessionUpdate{}, err
	}

	if rd.Len() != 0 {
		return ChatSessionUpdate{}, fmt.Errorf("chat session update has %d trailing bytes", rd.Len())
	}

	return update, nil
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
		X:     rd.Double(),
		Y:     rd.Double(),
		Z:     rd.Double(),
		Flags: MovementFlags(rd.Byte()),
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
		X:     rd.Double(),
		Y:     rd.Double(),
		Z:     rd.Double(),
		Yaw:   rd.Float(),
		Pitch: rd.Float(),
		Flags: MovementFlags(rd.Byte()),
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
		Yaw:   rd.Float(),
		Pitch: rd.Float(),
		Flags: MovementFlags(rd.Byte()),
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
		Flags: MovementFlags(rd.Byte()),
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

func DecodePlayerAction(data []byte) (PlayerAction, error) {
	rd := NewPacketReader(data)

	action := PlayerAction{
		Status:   rd.VarInt(),
		Position: rd.BlockPosition(),
		Face:     int8(rd.Byte()),
		Sequence: rd.VarInt(),
	}

	err := rd.Err()
	if err != nil {
		return PlayerAction{}, err
	}

	return action, nil
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

func DecodeSetHeldItem(data []byte) (SetHeldItem, error) {
	rd := NewPacketReader(data)

	selection := SetHeldItem{Slot: rd.Short()}

	err := rd.Err()
	if err != nil {
		return SetHeldItem{}, err
	}

	return selection, nil
}

func DecodeContainerClick(data []byte) (ContainerClick, error) {
	rd := NewPacketReader(data)

	click := ContainerClick{
		WindowID:    rd.VarInt(),
		StateID:     rd.VarInt(),
		Slot:        rd.Short(),
		MouseButton: int8(rd.Byte()),
		Mode:        rd.VarInt(),
	}

	changedCount := rd.VarInt()
	if changedCount < 0 || changedCount > maxContainerChangedSlots {
		return ContainerClick{}, fmt.Errorf("invalid changed slot count %d", changedCount)
	}

	click.ChangedSlots = make([]ChangedSlot, changedCount)

	for index := range click.ChangedSlots {
		click.ChangedSlots[index].Location = rd.Short()

		item, err := decodeOptionalHashedSlot(rd)
		if err != nil {
			return ContainerClick{}, err
		}

		click.ChangedSlots[index].Item = item
	}

	cursor, err := decodeOptionalHashedSlot(rd)
	if err != nil {
		return ContainerClick{}, err
	}

	click.CursorItem = cursor

	err = rd.Err()
	if err != nil {
		return ContainerClick{}, err
	}

	if rd.Len() != 0 {
		return ContainerClick{}, fmt.Errorf("container click has %d trailing bytes", rd.Len())
	}

	return click, nil
}

func DecodeSetCreativeModeSlot(data []byte) (SetCreativeModeSlot, error) {
	rd := NewPacketReader(data)

	creativeSlot := SetCreativeModeSlot{Slot: rd.Short()}

	item, err := decodeUntrustedSlot(rd)
	if err != nil {
		return SetCreativeModeSlot{}, err
	}

	creativeSlot.Item = item

	err = rd.Err()
	if err != nil {
		return SetCreativeModeSlot{}, err
	}

	if rd.Len() != 0 {
		return SetCreativeModeSlot{}, fmt.Errorf("creative mode slot has %d trailing bytes", rd.Len())
	}

	return creativeSlot, nil
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

func DecodeUseItemOn(data []byte) (UseItemOn, error) {
	rd := NewPacketReader(data)

	interaction := UseItemOn{
		Hand:           rd.VarInt(),
		Position:       rd.BlockPosition(),
		Face:           rd.VarInt(),
		CursorX:        rd.Float(),
		CursorY:        rd.Float(),
		CursorZ:        rd.Float(),
		InsideBlock:    rd.Bool(),
		WorldBorderHit: rd.Bool(),
		Sequence:       rd.VarInt(),
	}

	err := rd.Err()
	if err != nil {
		return UseItemOn{}, err
	}

	return interaction, nil
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

func decodeUntrustedSlot(rd *PacketReader) (UntrustedSlot, error) {
	item := UntrustedSlot{ItemCount: rd.VarInt()}

	err := rd.Err()
	if err != nil || item.ItemCount == 0 {
		return item, err
	}

	item.ItemID = rd.VarInt()

	addedComponentCount := rd.VarInt()
	removedComponentCount := rd.VarInt()

	err = rd.Err()
	if err != nil {
		return UntrustedSlot{}, err
	}

	if addedComponentCount < 0 || addedComponentCount > maxUntrustedSlotComponents {
		return UntrustedSlot{}, fmt.Errorf("invalid added slot component count %d", addedComponentCount)
	}

	if removedComponentCount < 0 || removedComponentCount > maxUntrustedSlotComponents {
		return UntrustedSlot{}, fmt.Errorf("invalid removed slot component count %d", removedComponentCount)
	}

	item.Components = make([]UntrustedSlotComponent, addedComponentCount)

	for index := range item.Components {
		item.Components[index] = UntrustedSlotComponent{
			Type: rd.VarInt(),
			Data: rd.BytesMax(maxUntrustedComponentBytes),
		}
	}

	item.RemovedComponents = make([]int32, removedComponentCount)

	for index := range item.RemovedComponents {
		item.RemovedComponents[index] = rd.VarInt()
	}

	err = rd.Err()
	if err != nil {
		return UntrustedSlot{}, err
	}

	return item, nil
}

func decodeOptionalHashedSlot(rd *PacketReader) (HashedSlot, error) {
	if !rd.Bool() {
		return HashedSlot{}, rd.Err()
	}

	item := HashedSlot{
		Present:   true,
		ItemID:    rd.VarInt(),
		ItemCount: rd.VarInt(),
	}

	componentCount := rd.VarInt()
	if componentCount < 0 || componentCount > maxUntrustedSlotComponents {
		return HashedSlot{}, fmt.Errorf("invalid hashed slot component count %d", componentCount)
	}

	item.Components = make([]HashedSlotComponent, componentCount)

	for index := range item.Components {
		item.Components[index] = HashedSlotComponent{Type: rd.VarInt(), Hash: rd.Int()}
	}

	removedCount := rd.VarInt()
	if removedCount < 0 || removedCount > maxUntrustedSlotComponents {
		return HashedSlot{}, fmt.Errorf("invalid hashed slot removed component count %d", removedCount)
	}

	item.RemovedComponents = make([]int32, removedCount)

	for index := range item.RemovedComponents {
		item.RemovedComponents[index] = rd.VarInt()
	}

	err := rd.Err()
	if err != nil {
		return HashedSlot{}, err
	}

	return item, nil
}

func validChatMessage(message string) bool {
	if message == "" || !utf8.ValidString(message) || utf8.RuneCountInString(message) > maxChatMessageCharacters {
		return false
	}

	for _, character := range message {
		if character < ' ' || character == '\u007f' || character == '\u00a7' {
			return false
		}
	}

	return true
}
