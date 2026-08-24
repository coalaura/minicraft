package server

import (
	"fmt"
	"math"
	"sort"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	// Keep one empty chunk around generated chunks so the client can build
	// meshes for blocks at their borders.
	ChunkRadius = 2
	ChunkWidth  = 16
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

	return s.updatePlayerChunks()
}

func (s *Session) sendGameEvent(event byte, value float32) error {
	var wr protocol.PacketWriter

	gameEvent := protocol.GameEvent{Event: event, Value: value}

	gameEvent.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundGameEventID,
		Data: wr.Buffer.Bytes(),
	})
}

func chunkPacket(world *game.World, chunkX, chunkZ int32) (protocol.Packet, error) {
	chunk, err := buildLevelChunk(world, chunkX, chunkZ)
	if err != nil {
		return protocol.Packet{}, err
	}

	var wr protocol.PacketWriter

	chunk.Encode(&wr)

	err = wr.Err()
	if err != nil {
		return protocol.Packet{}, err
	}

	return protocol.Packet{
		ID:   protocol.ClientboundLevelChunkWithLightID,
		Data: wr.Buffer.Bytes(),
	}, nil
}

func (s *Session) sendChunkBatch(chunks []LoadedChunk) (int, error) {
	if len(chunks) == 0 {
		return 0, nil
	}

	packets := make([]protocol.Packet, len(chunks))

	for index, chunk := range chunks {
		packet, err := chunkPacket(s.Runtime.World, chunk.X, chunk.Z)
		if err != nil {
			return 0, err
		}

		packets[index] = packet
	}

	var wr protocol.PacketWriter

	batchEnd := protocol.ChunkBatchEnd{BatchSize: int32(len(chunks))}

	batchEnd.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return 0, err
	}

	s.writeMx.Lock()
	defer s.writeMx.Unlock()

	err = s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundChunkBatchBeginID,
		Data: []byte{},
	})

	if err != nil {
		return 0, err
	}

	for index, packet := range packets {
		err := s.Conn.WritePacket(packet)
		if err != nil {
			return index, err
		}
	}

	err = s.Conn.WritePacket(protocol.Packet{
		ID:   protocol.ClientboundChunkBatchEndID,
		Data: wr.Buffer.Bytes(),
	})

	return len(chunks), err
}

func (s *Session) sendCenterChunk(chunkX, chunkZ int32) error {
	var wr protocol.PacketWriter

	center := protocol.SetCenterChunk{X: chunkX, Z: chunkZ}

	center.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundSetCenterChunkID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendForgetChunk(chunk LoadedChunk) error {
	var wr protocol.PacketWriter

	forget := protocol.ForgetLevelChunk{X: chunk.X, Z: chunk.Z}

	forget.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundForgetLevelChunkID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) updatePlayerChunks() error {
	player := s.snapshotPlayer()
	center := LoadedChunk{
		X: chunkCoordinate(player.Position.X),
		Z: chunkCoordinate(player.Position.Z),
	}

	return s.updateVisibleChunks(center)
}

func (s *Session) updateVisibleChunks(center LoadedChunk) error {
	s.chunkMx.Lock()
	defer s.chunkMx.Unlock()

	if !s.hasChunkCenter || s.centerChunk != center {
		err := s.sendCenterChunk(center.X, center.Z)
		if err != nil {
			return err
		}

		s.centerChunk = center
		s.hasChunkCenter = true
	}

	visibleChunks := chunksInView(center)
	visibleSet := make(map[LoadedChunk]struct{}, len(visibleChunks))

	for _, chunk := range visibleChunks {
		visibleSet[chunk] = struct{}{}
	}

	var chunksToUnload []LoadedChunk

	for chunk := range s.loadedChunks {
		if _, visible := visibleSet[chunk]; !visible {
			chunksToUnload = append(chunksToUnload, chunk)
		}
	}

	sort.Slice(chunksToUnload, func(first, second int) bool {
		if chunksToUnload[first].Z == chunksToUnload[second].Z {
			return chunksToUnload[first].X < chunksToUnload[second].X
		}

		return chunksToUnload[first].Z < chunksToUnload[second].Z
	})

	for _, chunk := range chunksToUnload {
		err := s.sendForgetChunk(chunk)
		if err != nil {
			return err
		}

		delete(s.loadedChunks, chunk)
	}

	chunksToLoad := make([]LoadedChunk, 0, len(visibleChunks))

	for _, chunk := range visibleChunks {
		if _, loaded := s.loadedChunks[chunk]; !loaded {
			chunksToLoad = append(chunksToLoad, chunk)
		}
	}

	sent, err := s.sendChunkBatch(chunksToLoad)

	if s.loadedChunks == nil {
		s.loadedChunks = make(map[LoadedChunk]struct{}, len(visibleChunks))
	}

	for _, chunk := range chunksToLoad[:sent] {
		s.loadedChunks[chunk] = struct{}{}
	}

	return err
}

func chunksInView(center LoadedChunk) []LoadedChunk {
	chunkCount := (ChunkRadius*2 + 1) * (ChunkRadius*2 + 1)
	chunks := make([]LoadedChunk, 0, chunkCount)

	for chunkZ := center.Z - ChunkRadius; chunkZ <= center.Z+ChunkRadius; chunkZ++ {
		for chunkX := center.X - ChunkRadius; chunkX <= center.X+ChunkRadius; chunkX++ {
			chunks = append(chunks, LoadedChunk{X: chunkX, Z: chunkZ})
		}
	}

	return chunks
}

func chunkCoordinate(position float64) int32 {
	return int32(math.Floor(position / ChunkWidth))
}

func buildLevelChunk(world *game.World, chunkX, chunkZ int32) (protocol.LevelChunkWithLight, error) {
	if world == nil {
		return protocol.LevelChunkWithLight{}, fmt.Errorf("world is nil")
	}

	chunk := protocol.NewEmptyOverworldChunk(chunkX, chunkZ, 0)

	chunkMinX := chunkX * ChunkWidth
	chunkMinZ := chunkZ * ChunkWidth

	for sectionIndex := range chunk.Sections {
		sectionMinY := int32(protocol.OverworldMinY + sectionIndex*ChunkWidth)
		var (
			blocks    protocol.SectionBlocks
			hasBlocks bool
		)

		for localY := 0; localY < ChunkWidth; localY++ {
			for localZ := 0; localZ < ChunkWidth; localZ++ {
				for localX := 0; localX < ChunkWidth; localX++ {
					position := game.BlockPosition{
						X: chunkMinX + int32(localX),
						Y: sectionMinY + int32(localY),
						Z: chunkMinZ + int32(localZ),
					}

					state, err := protocolBlockState(world.BlockAt(position))
					if err != nil {
						return protocol.LevelChunkWithLight{}, fmt.Errorf("block at %+v: %w", position, err)
					}

					if state == protocol.AirBlockState {
						continue
					}

					blocks.Set(localX, localY, localZ, state)

					hasBlocks = true
				}
			}
		}

		if hasBlocks {
			chunk.Sections[sectionIndex] = blocks.ToSection(0)
		}
	}

	return chunk, nil
}

func protocolBlockState(block game.Block) (int32, error) {
	switch block {
	case game.Air:
		return protocol.AirBlockState, nil
	case game.Stone:
		return protocol.StoneBlockState, nil
	default:
		return 0, fmt.Errorf("unsupported game block %d", block)
	}
}
