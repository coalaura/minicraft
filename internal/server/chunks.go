package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	InitialChunkRadius = 0
)

type LoadedChunk struct {
	X int32
	Z int32
}

func (s *Session) sendInitialChunks() error {
	err := s.sendCenterChunk(0, 0)
	if err != nil {
		return err
	}

	chunks := []LoadedChunk{{X: 0, Z: 0}}

	err = s.sendChunkBatch(chunks)
	if err != nil {
		return err
	}

	var wr protocol.PacketWriter

	event := protocol.GameEvent{Event: 13, Value: 0}

	event.Encode(&wr)

	err = wr.Err()
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
