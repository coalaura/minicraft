package server

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	ChunkWidth = 16

	initialChunkBatchSize  = 1
	fallbackChunkBatchSize = 4
	maxChunkBatchSize      = 32
	chunkBatchTargetTicks  = 4
	chunkBatchAckTimeout   = time.Second
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

	packets, err := buildChunkPackets(context.Background(), s.Runtime.World, chunks)
	if err != nil {
		return 0, err
	}

	return s.sendPreparedChunkBatch(packets)
}

func (s *Session) sendPreparedChunkBatch(packets []protocol.Packet) (int, error) {
	if len(packets) == 0 {
		return 0, nil
	}

	var wr protocol.PacketWriter

	batchEnd := protocol.ChunkBatchEnd{BatchSize: int32(len(packets))}

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

	return len(packets), err
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

func (s *Session) sendBlockUpdate(position game.BlockPosition, state int32) error {
	var wr protocol.PacketWriter

	update := protocol.BlockUpdate{Position: position, State: state}

	update.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundBlockUpdateID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendBlockUpdateIfLoaded(position game.BlockPosition, state int32) error {
	s.chunkMx.Lock()
	defer s.chunkMx.Unlock()

	if _, loaded := s.loadedChunks[blockLoadedChunk(position)]; !loaded {
		return nil
	}

	return s.sendBlockUpdate(position, state)
}

func (s *Session) sendLightUpdateIfLoaded(update protocol.UpdateLight) error {
	s.chunkMx.Lock()
	defer s.chunkMx.Unlock()

	chunk := LoadedChunk{X: update.Position.X, Z: update.Position.Z}
	if _, loaded := s.loadedChunks[chunk]; !loaded {
		return nil
	}

	var wr protocol.PacketWriter

	update.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundUpdateLightID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) sendLevelEventIfLoaded(event protocol.LevelEvent) error {
	s.chunkMx.Lock()
	defer s.chunkMx.Unlock()

	if _, loaded := s.loadedChunks[blockLoadedChunk(event.Position)]; !loaded {
		return nil
	}

	var wr protocol.PacketWriter

	event.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundLevelEventID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) hasLoadedBlock(position game.BlockPosition) bool {
	s.chunkMx.Lock()
	defer s.chunkMx.Unlock()

	_, loaded := s.loadedChunks[blockLoadedChunk(position)]

	return loaded
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

	if s.hasChunkCenter && s.centerChunk == center {
		s.chunkMx.Unlock()

		return nil
	}

	visibleChunks := chunksInView(center, s.renderDistance())
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
		delete(s.loadedChunks, chunk)
	}

	queuedChunks := make([]LoadedChunk, 0, len(visibleChunks))

	for _, chunk := range visibleChunks {
		if _, loaded := s.loadedChunks[chunk]; !loaded {
			queuedChunks = append(queuedChunks, chunk)
		}
	}

	if s.loadedChunks == nil {
		s.loadedChunks = make(map[LoadedChunk]struct{}, len(visibleChunks))
	}

	s.centerChunk = center
	s.hasChunkCenter = true
	s.queuedChunks = queuedChunks
	s.chunkRevision++
	s.chunkQueueReady = false

	s.ensureChunkStreamLocked()

	notify := s.chunkStreamNotify
	streamStarted := s.chunkStreamStarted
	s.chunkMx.Unlock()

	err := s.sendCenterChunk(center.X, center.Z)
	if err != nil {
		return err
	}

	for _, chunk := range chunksToUnload {
		err = s.sendForgetChunk(chunk)
		if err != nil {
			return err
		}
	}

	s.chunkMx.Lock()
	s.chunkQueueReady = true
	s.chunkMx.Unlock()

	notifyChunkStream(notify)

	if !streamStarted {
		return s.sendQueuedChunksSynchronously()
	}

	return nil
}

func chunksInView(center LoadedChunk, radius int32) []LoadedChunk {
	chunkCount := int((radius*2 + 1) * (radius*2 + 1))
	chunks := make([]LoadedChunk, 0, chunkCount)

	for chunkZ := center.Z - radius; chunkZ <= center.Z+radius; chunkZ++ {
		for chunkX := center.X - radius; chunkX <= center.X+radius; chunkX++ {
			chunks = append(chunks, LoadedChunk{X: chunkX, Z: chunkZ})
		}
	}

	sort.Slice(chunks, func(first, second int) bool {
		firstX := chunks[first].X - center.X
		firstZ := chunks[first].Z - center.Z

		secondX := chunks[second].X - center.X
		secondZ := chunks[second].Z - center.Z

		firstDistance := firstX*firstX + firstZ*firstZ
		secondDistance := secondX*secondX + secondZ*secondZ

		if firstDistance != secondDistance {
			return firstDistance < secondDistance
		}

		if chunks[first].Z != chunks[second].Z {
			return chunks[first].Z < chunks[second].Z
		}

		return chunks[first].X < chunks[second].X
	})

	return chunks
}

func (s *Session) startChunkStream(ctx context.Context) {
	s.chunkMx.Lock()
	s.ensureChunkStreamLocked()

	if s.chunkStreamStarted {
		s.chunkMx.Unlock()

		return
	}

	s.chunkStreamStarted = true

	notify := s.chunkStreamNotify
	s.chunkMx.Unlock()

	go func() {
		defer func() {
			s.chunkMx.Lock()
			s.chunkStreamStarted = false
			s.chunkBatchAwaiting = false
			s.chunkMx.Unlock()
		}()

		err := s.chunkStreamLoop(ctx)
		if err != nil && ctx.Err() == nil && s.Log != nil {
			s.Log.Warnf("[play] chunk stream failed: %v\n", err)
		}
	}()

	notifyChunkStream(notify)
}

func (s *Session) sendQueuedChunksSynchronously() error {
	s.chunkMx.Lock()
	chunks := append([]LoadedChunk(nil), s.queuedChunks...)

	s.queuedChunks = nil

	revision := s.chunkRevision
	s.chunkMx.Unlock()

	packets, err := buildChunkPackets(context.Background(), s.Runtime.World, chunks)
	if err != nil {
		return err
	}

	s.chunkMx.Lock()
	defer s.chunkMx.Unlock()

	if revision != s.chunkRevision {
		return nil
	}

	sent, err := s.sendPreparedChunkBatch(packets)

	for _, chunk := range chunks[:sent] {
		s.loadedChunks[chunk] = struct{}{}
	}

	return err
}

func (s *Session) chunkStreamLoop(ctx context.Context) error {
	for {
		s.chunkMx.Lock()
		s.ensureChunkStreamLocked()

		notify := s.chunkStreamNotify

		if s.chunkBatchAwaiting {
			wait := time.Until(s.chunkBatchSentAt.Add(chunkBatchAckTimeout))
			s.chunkMx.Unlock()

			if wait > 0 {
				timer := time.NewTimer(wait)

				select {
				case <-ctx.Done():
					stopTimer(timer)

					return nil
				case <-notify:
					stopTimer(timer)
				case <-timer.C:
				}

				continue
			}

			s.chunkMx.Lock()
			if s.chunkBatchAwaiting && time.Since(s.chunkBatchSentAt) >= chunkBatchAckTimeout {
				s.chunkBatchAwaiting = false
				s.chunkFeedbackTimedOut = true
			}
			s.chunkMx.Unlock()

			continue
		}

		if !s.chunkQueueReady || len(s.queuedChunks) == 0 {
			s.chunkMx.Unlock()

			select {
			case <-ctx.Done():
				return nil
			case <-notify:
			}

			continue
		}

		batchSize := min(s.chunkBatchSizeLocked(), len(s.queuedChunks))
		chunks := append([]LoadedChunk(nil), s.queuedChunks[:batchSize]...)
		s.queuedChunks = s.queuedChunks[batchSize:]

		revision := s.chunkRevision
		s.chunkMx.Unlock()

		packets, err := buildChunkPackets(ctx, s.Runtime.World, chunks)
		if err != nil {
			return err
		}

		if err = ctx.Err(); err != nil {
			return nil
		}

		s.chunkMx.Lock()
		if revision != s.chunkRevision || !s.chunkQueueReady {
			s.chunkMx.Unlock()

			continue
		}

		sent, err := s.sendPreparedChunkBatch(packets)
		if err != nil {
			s.chunkMx.Unlock()

			return err
		}

		for _, chunk := range chunks[:sent] {
			s.loadedChunks[chunk] = struct{}{}
		}

		s.chunkBatchAwaiting = true
		s.chunkBatchSentAt = time.Now()
		s.chunkMx.Unlock()
	}
}

func (s *Session) chunkBatchSizeLocked() int {
	if s.chunksPerTick > 0 && !float32Invalid(s.chunksPerTick) {
		batchSize := int(math.Ceil(float64(s.chunksPerTick * chunkBatchTargetTicks)))

		return min(max(batchSize, 1), maxChunkBatchSize)
	}

	if s.chunkFeedbackTimedOut {
		return fallbackChunkBatchSize
	}

	return initialChunkBatchSize
}

func (s *Session) ensureChunkStreamLocked() {
	if s.chunkStreamNotify == nil {
		s.chunkStreamNotify = make(chan struct{}, 1)
	}
}

func buildChunkPackets(ctx context.Context, world *game.World, chunks []LoadedChunk) ([]protocol.Packet, error) {
	packets := make([]protocol.Packet, len(chunks))

	workerCount := min(len(chunks), min(runtime.GOMAXPROCS(0), 8))
	if workerCount == 0 {
		return packets, nil
	}

	buildContext, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		nextIndex atomic.Int64
		workers   sync.WaitGroup
		firstErr  error
		errOnce   sync.Once
	)

	workers.Add(workerCount)

	for range workerCount {
		go func() {
			defer workers.Done()

			for {
				index := int(nextIndex.Add(1) - 1)
				if index >= len(chunks) {
					return
				}

				if buildContext.Err() != nil {
					return
				}

				chunk := chunks[index]

				packet, err := chunkPacket(world, chunk.X, chunk.Z)
				if err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})

					return
				}

				packets[index] = packet
			}
		}()
	}

	workers.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return packets, nil
}

func notifyChunkStream(notify chan struct{}) {
	select {
	case notify <- struct{}{}:
	default:
	}
}

func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func float32Invalid(value float32) bool {
	return math.IsNaN(float64(value)) || math.IsInf(float64(value), 0)
}

func chunkCoordinate(position float64) int32 {
	return int32(math.Floor(position / ChunkWidth))
}

func blockChunkCoordinate(position int32) int32 {
	chunk := position / ChunkWidth
	if position%ChunkWidth < 0 {
		chunk--
	}

	return chunk
}

func blockLoadedChunk(position game.BlockPosition) LoadedChunk {
	return LoadedChunk{
		X: blockChunkCoordinate(position.X),
		Z: blockChunkCoordinate(position.Z),
	}
}

func buildLevelChunk(world *game.World, chunkX, chunkZ int32) (protocol.LevelChunkWithLight, error) {
	if world == nil {
		return protocol.LevelChunkWithLight{}, fmt.Errorf("world is nil")
	}

	if world.Lighting == game.LightingNormal {
		return buildNormalLevelChunk(world, chunkX, chunkZ)
	}

	return buildFullbrightLevelChunk(world, chunkX, chunkZ)
}

func buildFullbrightLevelChunk(world *game.World, chunkX, chunkZ int32) (protocol.LevelChunkWithLight, error) {
	chunk := protocol.NewEmptyOverworldChunk(chunkX, chunkZ, defaultBiomeID)

	chunkPosition := game.ChunkPosition{X: chunkX, Z: chunkZ}

	prepared := prepareChunkGeneration(world, chunkPosition)

	overrides := world.SnapshotChunkOverrides(chunkPosition)

	generator := world.Generator

	generationMinY := int32(protocol.OverworldMinY)
	generationMaxY := generationMinY + protocol.OverworldSectionCount*ChunkWidth - 1

	hasGeneration := generator != nil

	if boundedGenerator, bounded := generator.(game.BoundedGenerator); bounded {
		generationMinY, generationMaxY, hasGeneration = boundedGenerator.GenerationBounds(world.Seed, chunkPosition)
	}

	var (
		generatedBlocks [game.SectionVolume]game.Block
		sectionBlocks   protocol.SectionBlocks
	)

	for sectionIndex := range chunk.Sections {
		sectionMinY := int32(protocol.OverworldMinY + sectionIndex*ChunkWidth)
		sectionMaxY := sectionMinY + ChunkWidth - 1

		sectionBiomes, hasSectionBiomes, err := buildSectionBiomes(prepared, sectionMinY)
		if err != nil {
			return protocol.LevelChunkWithLight{}, err
		}

		hasOverrides := sectionHasOverrides(overrides, sectionMinY, sectionMaxY)

		generateSection := hasGeneration && sectionMaxY >= generationMinY && sectionMinY <= generationMaxY

		if !generateSection && !hasOverrides {
			if hasSectionBiomes {
				chunk.Sections[sectionIndex].SetBiomes(&sectionBiomes)
			}

			continue
		}

		uniformBlock := game.Air
		uniform := true

		if generateSection {
			uniformBlock, uniform = prepared.GenerateSection(sectionMinY, &generatedBlocks)
		}

		if uniform && !hasOverrides {
			state, err := protocolBlockState(uniformBlock)
			if err != nil {
				return protocol.LevelChunkWithLight{}, fmt.Errorf("uniform section at y %d: %w", sectionMinY, err)
			}

			section := protocol.UniformChunkSection(state, defaultBiomeID)

			if hasSectionBiomes {
				section.SetBiomes(&sectionBiomes)
			}

			chunk.Sections[sectionIndex] = section

			continue
		}

		if uniform {
			for index := range generatedBlocks {
				generatedBlocks[index] = uniformBlock
			}
		}

		for position, block := range overrides {
			if position.Y < sectionMinY || position.Y > sectionMaxY {
				continue
			}

			localY := position.Y - sectionMinY
			index := localY*256 + position.Z*16 + position.X

			generatedBlocks[index] = block
		}

		for index, block := range generatedBlocks {
			state, err := protocolBlockState(block)
			if err != nil {
				localY := int32(index / 256)
				localZ := int32(index%256) / 16
				localX := int32(index % 16)

				position := game.BlockPosition{
					X: chunkX*ChunkWidth + localX,
					Y: sectionMinY + localY,
					Z: chunkZ*ChunkWidth + localZ,
				}

				return protocol.LevelChunkWithLight{}, fmt.Errorf("block at %+v: %w", position, err)
			}

			sectionBlocks.States[index] = state
		}

		section := sectionBlocks.ToSection(defaultBiomeID)

		if hasSectionBiomes {
			section.SetBiomes(&sectionBiomes)
		}

		chunk.Sections[sectionIndex] = section
	}

	return chunk, nil
}

func protocolBlockState(block game.Block) (int32, error) {
	if !block.Valid() {
		return 0, fmt.Errorf("unsupported game block %d", block)
	}

	return int32(block), nil
}

func generateSectionBlocks(generator game.Generator, seed int64, chunk game.ChunkPosition, sectionMinY int32, blocks *[game.SectionVolume]game.Block) (game.Block, bool) {
	chunkMinX := chunk.X * ChunkWidth
	chunkMinZ := chunk.Z * ChunkWidth

	first := game.Air

	uniform := true

	for localY := range int32(ChunkWidth) {
		for localZ := range int32(ChunkWidth) {
			for localX := range int32(ChunkWidth) {
				index := localY*256 + localZ*16 + localX

				block := generator.BlockAt(seed, game.BlockPosition{
					X: chunkMinX + localX,
					Y: sectionMinY + localY,
					Z: chunkMinZ + localZ,
				})

				blocks[index] = block

				if index == 0 {
					first = block
				} else if block != first {
					uniform = false
				}
			}
		}
	}

	return first, uniform
}

func sectionHasOverrides(overrides game.ChunkOverrides, minY, maxY int32) bool {
	for position := range overrides {
		if position.Y >= minY && position.Y <= maxY {
			return true
		}
	}

	return false
}
