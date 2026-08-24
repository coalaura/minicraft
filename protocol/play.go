package protocol

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/coalaura/minicraft/config"
)

const (
	playKeepAlivePeriod = 10 * time.Second
)

func HandlePlay(ctx context.Context, c *MCConnection, cfg *config.Config, uuid, name string) error {
	c.Print("play", "entering play state")

	playCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go keepAliveLoop(playCtx, c)

	world := World{
		Name:          "minecraft:overworld",
		DimensionType: 0,

		Spawn: Vec3{
			X: 0.5,
			Y: 70,
			Z: 0.5,
		},

		ViewDistance:       10,
		SimulationDistance: 10,
		SeaLevel:           64,
	}

	player := Player{
		EntityID: 1,
		UUID:     uuid,
		Name:     name,

		Position: world.Spawn,
		Yaw:      0,
		Pitch:    0,
		GameMode: 1,
	}

	login := PlayLogin{
		EntityID: player.EntityID,
		Worlds: []string{
			world.Name,
		},
		MaxPlayers:         int32(cfg.MaxPlayers),
		ViewDistance:       world.ViewDistance,
		SimulationDistance: world.SimulationDistance,

		Spawn: SpawnInfo{
			DimensionType:    world.DimensionType,
			Dimension:        world.Name,
			GameMode:         player.GameMode,
			PreviousGameMode: 0xFF,
			SeaLevel:         world.SeaLevel,
		},
	}

	err := sendPlayLogin(c, login)
	if err != nil {
		return err
	}

	err = sendSpawnChunks(c)
	if err != nil {
		return err
	}

	position := PlayerPosition{
		TeleportID: 1,
		Position:   player.Position,
		Yaw:        player.Yaw,
		Pitch:      player.Pitch,
	}

	err = sendPlayerPosition(c, position)
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
		case SB_ClientTickEnd:
			// End of client tick; nothing to do for now.
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

func sendPlayLogin(c *MCConnection, login PlayLogin) error {
	var w PacketWriter

	login.Encode(&w)

	err := w.Err()
	if err != nil {
		return err
	}

	return c.WritePacket(Packet{
		ID:   CB_PlayLogin,
		Data: w.Bytes(),
	})
}

func sendPlayerPosition(c *MCConnection, position PlayerPosition) error {
	var w PacketWriter

	position.Encode(&w)

	err := w.Err()
	if err != nil {
		return err
	}

	err = c.WritePacket(Packet{
		ID:   CB_PlayPosition,
		Data: w.Bytes(),
	})

	if err != nil {
		return err
	}

	c.Print("play", "sent position and look")

	return nil
}
