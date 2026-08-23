package protocol

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/coalaura/minicraft/config"
)

const (
	playEntityID        = 1
	playTeleportID      = 1
	playSpawnX          = 0.5
	playSpawnY          = 70.0
	playSpawnZ          = 0.5
	playViewDistance    = 10
	playSeaLevel        = 64
	playGameMode        = 1 // creative
	playKeepAlivePeriod = 10 * time.Second
)

func HandlePlay(ctx context.Context, c *MCConnection, cfg *config.Config, uuid, name string) error {
	c.Print("play", "entering play state")

	playCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go keepAliveLoop(playCtx, c)

	err := sendPlayLogin(c)
	if err != nil {
		return err
	}

	err = sendSpawnChunks(c)
	if err != nil {
		return err
	}

	err = sendPlayPosition(c)
	if err != nil {
		return err
	}

	c.Print("play", "player %s joined the world", name)

	for {
		packet, err := c.ReadPacket()
		if err != nil {
			if err == io.EOF {
				c.Print("play", "client disconnected")

				return nil
			}

			c.Warn("play", "failed to read packet: %v", err)

			return nil
		}

		switch packet.ID {
		case SB_ConfirmTeleport:
			c.Print("play", "confirmed teleport")
		case SB_ChunkBatchReceived:
			c.Print("play", "chunk batch received")
		case SB_ClientInfoPlay:
			c.Print("play", "received client information")
		case SB_KeepAlivePlay:
			// Keep-alive response; nothing to do.
		case SB_MovePlayerPos, SB_MovePlayerPosRot, SB_MovePlayerRot, SB_MoveStatusOnly:
			// Movement; ignore for now.
		case SB_PlayerLoaded:
			c.Print("play", "player loaded")
		default:
			c.Print("play", "unhandled packet id: 0x%02X", packet.ID)
		}
	}
}

func keepAliveLoop(ctx context.Context, c *MCConnection) {
	ticker := time.NewTicker(playKeepAlivePeriod)
	defer ticker.Stop()

	var id int64

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			id++

			var b bytes.Buffer

			err := WriteLong(&b, id)
			if err != nil {
				c.Warn("play", "failed to encode keep alive: %v", err)

				return
			}

			err = c.WritePacket(Packet{ID: CB_PlayKeepAlive, Data: b.Bytes()})
			if err != nil {
				c.Warn("play", "failed to send keep alive: %v", err)

				return
			}
		}
	}
}

func sendPlayLogin(c *MCConnection) error {
	var b bytes.Buffer

	err := WriteInt(&b, playEntityID)
	if err != nil {
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // is hardcore
		return err
	}

	err = WriteVarInt(&b, 1)
	if err != nil { // dimension names count
		return err
	}

	err = WriteString(&b, "minecraft:overworld")
	if err != nil {
		return err
	}

	err = WriteVarInt(&b, 1)
	if err != nil { // max players
		return err
	}

	err = WriteVarInt(&b, playViewDistance)
	if err != nil { // view distance
		return err
	}

	err = WriteVarInt(&b, playViewDistance)
	if err != nil { // simulation distance
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // reduced debug info
		return err
	}

	err = WriteBool(&b, true)
	if err != nil { // enable respawn screen
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // do limited crafting
		return err
	}

	err = WriteVarInt(&b, 0)
	if err != nil { // dimension type index
		return err
	}

	err = WriteString(&b, "minecraft:overworld")
	if err != nil { // dimension name
		return err
	}

	err = WriteLong(&b, 0)
	if err != nil { // hashed seed
		return err
	}

	err = b.WriteByte(playGameMode)
	if err != nil { // game mode
		return err
	}

	err = b.WriteByte(0xFF)
	if err != nil { // previous game mode (-1)
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // is debug
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // is flat
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // has death location
		return err
	}

	err = WriteVarInt(&b, 0)
	if err != nil { // portal cooldown
		return err
	}

	err = WriteVarInt(&b, playSeaLevel)
	if err != nil { // sea level
		return err
	}

	err = WriteBool(&b, false)
	if err != nil { // enforces secure chat
		return err
	}

	err = c.WritePacket(Packet{ID: CB_PlayLogin, Data: b.Bytes()})
	if err != nil {
		return err
	}

	c.Print("play", "sent login (play)")

	return nil
}

func sendPlayPosition(c *MCConnection) error {
	var b bytes.Buffer

	err := WriteVarInt(&b, playTeleportID)
	if err != nil {
		return err
	}

	err = WriteDouble(&b, playSpawnX)
	if err != nil {
		return err
	}

	err = WriteDouble(&b, playSpawnY)
	if err != nil {
		return err
	}

	err = WriteDouble(&b, playSpawnZ)
	if err != nil {
		return err
	}

	err = WriteDouble(&b, 0)
	if err != nil { // velocity x
		return err
	}

	err = WriteDouble(&b, 0)
	if err != nil { // velocity y
		return err
	}

	err = WriteDouble(&b, 0)
	if err != nil { // velocity z
		return err
	}

	err = WriteFloat(&b, 0)
	if err != nil { // yaw
		return err
	}

	err = WriteFloat(&b, 0)
	if err != nil { // pitch
		return err
	}

	// Flags: all values are absolute.
	err = WriteInt(&b, 0)
	if err != nil {
		return err
	}

	err = c.WritePacket(Packet{ID: CB_PlayPosition, Data: b.Bytes()})
	if err != nil {
		return err
	}

	c.Print("play", "sent position and look")

	return nil
}
