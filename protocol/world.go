package protocol

import "bytes"

const (
	worldSections = 24 // overworld: min y -64, height 384
	biomePlainsID = 0  // our temporary biome registry contains only minecraft:plains

	gameEventStartLoadingChunks = 13

	chunkBatchSize    = 1
	lightSectionCount = worldSections + 2
	fullLightMask     = (int64(1) << lightSectionCount) - 1
	skyLightArrayLen  = 2048
)

func sendSpawnChunks(c *MCConnection) error {
	err := sendCenterChunk(c, 0, 0)
	if err != nil {
		return err
	}

	err = c.WritePacket(Packet{ID: CB_ChunkBatchBeg})
	if err != nil {
		return err
	}

	data, err := emptyChunkData(0, 0)
	if err != nil {
		return err
	}

	err = c.WritePacket(Packet{ID: CB_ChunkData, Data: data})
	if err != nil {
		return err
	}

	var batch bytes.Buffer

	err = WriteVarInt(&batch, chunkBatchSize)
	if err != nil {
		return err
	}

	err = c.WritePacket(Packet{ID: CB_ChunkBatchEnd, Data: batch.Bytes()})
	if err != nil {
		return err
	}

	var event bytes.Buffer

	err = event.WriteByte(gameEventStartLoadingChunks)
	if err != nil {
		return err
	}

	err = WriteFloat(&event, 0)
	if err != nil {
		return err
	}

	return c.WritePacket(Packet{ID: CB_GameEvent, Data: event.Bytes()})
}

func sendCenterChunk(c *MCConnection, chunkX, chunkZ int32) error {
	var b bytes.Buffer

	err := WriteVarInt(&b, chunkX)
	if err != nil {
		return err
	}

	err = WriteVarInt(&b, chunkZ)
	if err != nil {
		return err
	}

	return c.WritePacket(Packet{ID: CB_SetCenterChunk, Data: b.Bytes()})
}

// emptyChunkData builds a Level Chunk with Light packet for an all-air chunk.
func emptyChunkData(chunkX, chunkZ int32) ([]byte, error) {
	var b bytes.Buffer

	err := WriteInt(&b, chunkX)
	if err != nil {
		return nil, err
	}

	err = WriteInt(&b, chunkZ)
	if err != nil {
		return nil, err
	}

	err = WriteVarInt(&b, 0)
	if err != nil { // heightmaps: none, client falls back to minimum heights
		return nil, err
	}

	sections := make([]byte, 0, worldSections*6)
	for range worldSections {
		// Block count (all air), block states palette (air), biomes palette (plains).
		sections = append(sections,
			0x00, 0x00, // block count short: 0
			0x00, // block states: bits per entry 0
			0x00, // palette: minecraft:air
			0x00, // biomes: bits per entry 0
			biomePlainsID,
		)
	}

	err = WriteBytes(&b, sections)
	if err != nil { // chunk data
		return nil, err
	}

	err = WriteVarInt(&b, 0)
	if err != nil { // block entities: none
		return nil, err
	}

	// Sky light mask: all sections present.
	err = WriteVarInt(&b, 1)
	if err != nil {
		return nil, err
	}

	err = WriteLong(&b, fullLightMask)
	if err != nil {
		return nil, err
	}

	// Block light mask: none.
	err = WriteVarInt(&b, 0)
	if err != nil {
		return nil, err
	}

	// Empty sky light mask: none.
	err = WriteVarInt(&b, 0)
	if err != nil {
		return nil, err
	}

	// Empty block light mask: none.
	err = WriteVarInt(&b, 0)
	if err != nil {
		return nil, err
	}

	// Sky light arrays: full brightness everywhere.
	err = WriteVarInt(&b, int32(lightSectionCount))
	if err != nil {
		return nil, err
	}

	fullBright := bytes.Repeat([]byte{0xFF}, skyLightArrayLen)
	for range lightSectionCount {
		err = WriteBytes(&b, fullBright)
		if err != nil {
			return nil, err
		}
	}

	// Block light arrays: none.
	err = WriteVarInt(&b, 0)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
