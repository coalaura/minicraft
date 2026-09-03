package server

import (
	"sort"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	defaultRandomTickSpeed = 3
	randomTickPositionAdd  = int32(1013904223)
)

type randomTickChunkSnapshot struct {
	position LoadedChunk
	sections []int32
}

func (r *Runtime) SetRandomTickPositionState(state int32) {
	r.worldMutationMu.Lock()
	defer r.worldMutationMu.Unlock()

	r.randomTickPositionState = state
}

func (r *Runtime) SetRandomTickRandom(random func(int) int) {
	r.worldMutationMu.Lock()
	defer r.worldMutationMu.Unlock()

	r.randomTickRandom = random
}

func (c *ActiveChunk) initializeRandomTickSections(world *game.World) {
	chunk := game.ChunkPosition{X: c.Position.X, Z: c.Position.Z}

	minY := int32(protocol.OverworldMinY)
	maxY := minY + protocol.OverworldSectionCount*game.ChunkWidth - 1

	hasGeneration := world.Generator != nil

	boundedGenerator, bounded := world.Generator.(game.BoundedGenerator)
	if bounded {
		minY, maxY, hasGeneration = boundedGenerator.GenerationBounds(world.Seed, chunk)
	}

	if hasGeneration {
		minY = max(minY, int32(protocol.OverworldMinY))
		maxY = min(maxY, int32(protocol.OverworldMinY+protocol.OverworldSectionCount*game.ChunkWidth-1))

		if minY > maxY {
			hasGeneration = false
		}
	}

	if hasGeneration {
		firstSection := randomTickSectionMinY(minY)
		lastSection := randomTickSectionMinY(maxY)

		hint, hinted := world.Generator.(game.RandomTickSectionGenerator)

		var (
			prepared           preparedChunkGeneration
			preparedGeneration bool
			blocks             [game.SectionVolume]game.Block
		)

		for sectionMinY := firstSection; sectionMinY <= lastSection; sectionMinY += game.ChunkWidth {
			if hinted {
				mayTick, definitive := hint.RandomTickSection(world.Seed, chunk, sectionMinY)
				if definitive {
					if mayTick {
						c.markRandomTickSection(sectionMinY)
					}

					continue
				}
			}

			if !preparedGeneration {
				prepared = prepareChunkGeneration(world, chunk)
				preparedGeneration = true
			}

			uniformBlock, uniform := prepared.GenerateSection(sectionMinY, &blocks)
			if randomTickSectionContents(uniformBlock, uniform, &blocks) {
				c.markRandomTickSection(sectionMinY)
			}
		}
	}

	overrides := world.SnapshotChunkOverrides(chunk)

	for local, block := range overrides {
		if block.RandomlyTicks() {
			c.markRandomTickSection(randomTickSectionMinY(local.Y))
		}
	}
}

func (r *Runtime) tickRandomBlocksLocked() {
	speed := r.RandomTickSpeed
	if r.randomTickRandom == nil || speed == 0 {
		return
	}

	chunks := r.snapshotActiveChunks()

	snapshots := make([]randomTickChunkSnapshot, len(chunks))

	for index, chunk := range chunks {
		snapshots[index] = randomTickChunkSnapshot{
			position: chunk.Position,
			sections: chunk.snapshotRandomTickSections(),
		}
	}

	for _, chunk := range snapshots {
		for _, sectionMinY := range chunk.sections {
			for range speed {
				position := r.nextRandomTickPosition(chunk.position, sectionMinY)

				block := r.World.BlockAt(position)
				if block.RandomlyTicks() {
					r.randomTickBlockLocked(position, block)
				}
			}
		}
	}
}

func (r *Runtime) nextRandomTickPosition(chunk LoadedChunk, sectionMinY int32) game.BlockPosition {
	r.randomTickPositionState = r.randomTickPositionState*3 + randomTickPositionAdd
	value := r.randomTickPositionState >> 2

	return game.BlockPosition{
		X: chunk.X*game.ChunkWidth + (value & 15),
		Y: sectionMinY + (value>>16)&15,
		Z: chunk.Z*game.ChunkWidth + (value>>8)&15,
	}
}

func (r *Runtime) promoteRandomTickSections(changes []game.BlockChange) {
	sections := make(map[*ActiveChunk]map[int32]struct{})

	for _, change := range changes {
		if !change.Replacement.RandomlyTicks() {
			continue
		}

		chunk, active := r.ActiveChunk(blockLoadedChunk(change.Position))
		if !active {
			continue
		}

		if sections[chunk] == nil {
			sections[chunk] = make(map[int32]struct{})
		}

		sections[chunk][randomTickSectionMinY(change.Position.Y)] = struct{}{}
	}

	chunks := make([]*ActiveChunk, 0, len(sections))

	for chunk := range sections {
		chunks = append(chunks, chunk)
	}

	sort.Slice(chunks, func(first, second int) bool {
		if chunks[first].Position.X != chunks[second].Position.X {
			return chunks[first].Position.X < chunks[second].Position.X
		}

		return chunks[first].Position.Z < chunks[second].Position.Z
	})

	for _, chunk := range chunks {
		for sectionMinY := range sections[chunk] {
			chunk.markRandomTickSection(sectionMinY)
		}
	}
}

func (r *Runtime) randomTickBlockLocked(position game.BlockPosition, block game.Block) {
	switch {
	case block.HasTrait(game.BlockTraitLeaves):
		r.randomTickLeafLocked(position, block)
	case sameBlockType(block, game.Farmland):
		r.randomTickFarmlandLocked(position, block)
	case cropMaximumAge(block) != 0:
		r.randomTickCropLocked(position, block)
	}
}

func randomTickSectionMinY(y int32) int32 {
	section := y / game.ChunkWidth

	if y%game.ChunkWidth < 0 {
		section--
	}

	return section * game.ChunkWidth
}

func randomTickSectionContents(uniformBlock game.Block, uniform bool, blocks *[game.SectionVolume]game.Block) bool {
	if uniform {
		return uniformBlock.RandomlyTicks()
	}

	for _, block := range blocks {
		if block.RandomlyTicks() {
			return true
		}
	}

	return false
}
