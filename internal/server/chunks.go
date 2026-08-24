package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	InitialChunkRadius = 1
	ChunkWidth         = 16
)

type LoadedChunk struct {
	X int32
	Z int32
}

func (s *Session) sendInitialChunks() error {
	// Event 13 starts the client's initial terrain wait. It exits that
	// state once the chunk containing the player has been loaded.
	err := s.sendGameEvent(13, 0)
	if err != nil {
		return err
	}

	err = s.sendCenterChunk(0, 0)
	if err != nil {
		return err
	}

	chunkCount := (InitialChunkRadius*2 + 1) * (InitialChunkRadius*2 + 1)
	chunks := make([]LoadedChunk, 0, chunkCount)

	for chunkZ := -InitialChunkRadius; chunkZ <= InitialChunkRadius; chunkZ++ {
		for chunkX := -InitialChunkRadius; chunkX <= InitialChunkRadius; chunkX++ {
			chunks = append(chunks, LoadedChunk{X: int32(chunkX), Z: int32(chunkZ)})
		}
	}

	return s.sendChunkBatch(chunks)
}

func (s *Session) sendGameEvent(event byte, value float32) error {
	var wr protocol.PacketWriter

	gameEvent := protocol.GameEvent{Event: event, Value: value}

	gameEvent.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundGameEventID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendChunk(chunkX, chunkZ int32) error {
	chunk := protocol.NewEmptyOverworldChunk(chunkX, chunkZ, 0)

	// TODO: temporary
	addSpawnPlatform(&chunk)

	var wr protocol.PacketWriter

	chunk.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundLevelChunkWithLightID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendChunkBatch(chunks []LoadedChunk) error {
	err := s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundChunkBatchBeginID,
		Data: []byte{},
	})

	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		err := s.sendChunk(chunk.X, chunk.Z)
		if err != nil {
			return err
		}
	}

	var wr protocol.PacketWriter

	batchEnd := protocol.ChunkBatchEnd{BatchSize: int32(len(chunks))}

	batchEnd.Encode(&wr)

	err = wr.Err()
	if err != nil {
		return err
	}

	return s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundChunkBatchEndID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendCenterChunk(chunkX, chunkZ int32) error {
	var wr protocol.PacketWriter

	center := protocol.SetCenterChunk{X: chunkX, Z: chunkZ}

	center.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundSetCenterChunkID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) updatePlayerChunk() error {
	chunkX := int32(math.Floor(s.Player.Position.X / 16))
	chunkZ := int32(math.Floor(s.Player.Position.Z / 16))

	return s.sendCenterChunk(chunkX, chunkZ)
}

// TODO: temporary
// addSpawnPlatform places a 9x9 stone platform centered on the spawn
// point, with its top surface right under the player's feet.
func addSpawnPlatform(chunk *protocol.LevelChunkWithLight) {
	const (
		spawnPlatformY      = 69
		spawnPlatformRadius = 4
	)

	localY := (spawnPlatformY - protocol.OverworldMinY) % ChunkWidth
	sectionIndex := (spawnPlatformY - protocol.OverworldMinY) / ChunkWidth

	chunkMinX := int(chunk.Position.X) * ChunkWidth
	chunkMinZ := int(chunk.Position.Z) * ChunkWidth

	blocks := &protocol.SectionBlocks{}
	hasPlatformBlocks := false

	for worldX := -spawnPlatformRadius; worldX <= spawnPlatformRadius; worldX++ {
		localX := worldX - chunkMinX
		if localX < 0 || localX >= ChunkWidth {
			continue
		}

		for worldZ := -spawnPlatformRadius; worldZ <= spawnPlatformRadius; worldZ++ {
			localZ := worldZ - chunkMinZ
			if localZ < 0 || localZ >= ChunkWidth {
				continue
			}

			blocks.Set(localX, localY, localZ, protocol.StoneBlockState)

			hasPlatformBlocks = true
		}
	}

	if !hasPlatformBlocks {
		return
	}

	chunk.Sections[sectionIndex] = blocks.ToSection(0)
}
