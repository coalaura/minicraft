package server

import (
	"fmt"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	blockInteractionRange       = 6.0
	sectionBlockUpdateThreshold = 8
)

type BlockMutationAction uint8

const (
	BlockMutationBreak BlockMutationAction = iota
	BlockMutationPlace
	BlockMutationInteract
)

type BlockMutation struct {
	Player      game.Player
	Action      BlockMutationAction
	Position    game.BlockPosition
	Current     game.Block
	Replacement game.Block
}

type BlockMutationResult struct {
	Block   game.Block
	Changes []game.BlockChange
	Allowed bool
	Changed bool
}

type blockMutationDelivery struct {
	session            *Session
	changes            []game.BlockChange
	states             []int32
	lightingChanges    []game.BlockChange
	directBreakChanged bool
	breakPosition      game.BlockPosition
	breakState         int32
	recipients         []*Session
	poseChanges        []game.Player
	waitForDelivery    <-chan struct{}
	deliveryComplete   chan struct{}
}

type blockMutationSection struct {
	position protocol.SectionBlocksUpdate
	changes  []game.BlockChange
}

type BlockMutationPolicy interface {
	AllowBlockMutation(BlockMutation) bool
}

type CreativeBlockMutationPolicy struct{}

func (CreativeBlockMutationPolicy) AllowBlockMutation(mutation BlockMutation) bool {
	return mutation.Player.GameMode == game.GameModeCreative
}

func blockWithinInteractionRange(player game.Player, position game.BlockPosition) bool {
	blockX := float64(position.X) + 0.5
	blockY := float64(position.Y) + 0.5
	blockZ := float64(position.Z) + 0.5

	eyePosition := player.EyePosition()

	distanceX := blockX - eyePosition.X
	distanceY := blockY - eyePosition.Y
	distanceZ := blockZ - eyePosition.Z

	distanceSquared := distanceX*distanceX + distanceY*distanceY + distanceZ*distanceZ
	return distanceSquared <= blockInteractionRange*blockInteractionRange
}

func (r *Runtime) MutateBlock(session *Session, action BlockMutationAction, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {
	return r.MutateBlocks(session, action, []game.BlockChange{{Position: position, Replacement: replacement}})
}

// MutateBlocks validates and applies a coordinated group atomically.
func (r *Runtime) MutateBlocks(session *Session, action BlockMutationAction, changes []game.BlockChange) (BlockMutationResult, error) {
	r.worldMutationMu.Lock()

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		if action == BlockMutationPlace {
			for _, change := range changes {
				if r.World.BlockAt(change.Position) != game.Air || change.Replacement == game.Air {
					return BlockMutationResult{Block: r.World.BlockAt(change.Position)}, blockMutationDelivery{}, nil
				}
			}
		}

		if action == BlockMutationBreak && len(changes) == 1 && changes[0].Replacement == game.Air {
			changes = r.breakChanges(changes[0].Position)
		}

		requiredChanges := len(changes)

		changes = r.withStructuralNeighborChanges(changes)

		return r.mutateBlocksLocked(session, action, changes, requiredChanges, true, false, false)
	}()

	return r.completeBlockMutation(result, delivery, err)
}

// MutateWorldBlocks applies authoritative world changes without player interaction restrictions.
func (r *Runtime) MutateWorldBlocks(changes []game.BlockChange) (BlockMutationResult, error) {
	r.worldMutationMu.Lock()

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		changes = r.withAuthoritativeDoorChanges(changes)
		changes = r.withStructuralNeighborChanges(changes)

		return r.mutateBlocksLocked(nil, BlockMutationPlace, changes, len(changes), true, false, true)
	}()

	return r.completeBlockMutation(result, delivery, err)
}

func (r *Runtime) PlaceBlock(session *Session, clicked, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {
	r.worldMutationMu.Lock()

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		current := r.World.BlockAt(position)
		if !session.hasLoadedBlock(clicked) || r.World.BlockAt(clicked) == game.Air || current != game.Air {
			return BlockMutationResult{Block: current}, blockMutationDelivery{}, nil
		}

		changes := r.withStructuralNeighborChanges([]game.BlockChange{{Position: position, Replacement: replacement}})

		return r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true, true, false)
	}()

	return r.completeBlockMutation(result, delivery, err)
}

func (r *Runtime) mutateBlocksLocked(session *Session, action BlockMutationAction, changes []game.BlockChange, requiredChanges int, allowOccupied, checkPlayerObstruction, authoritative bool) (BlockMutationResult, blockMutationDelivery, error) {
	r.mu.RLock()
	_, active := r.sessions[session]
	r.mu.RUnlock()

	result := BlockMutationResult{}

	if len(changes) != 0 {
		result.Block = r.World.BlockAt(changes[0].Position)
	}

	if (!active && !authoritative) || len(changes) == 0 {
		return result, blockMutationDelivery{}, nil
	}

	if !authoritative && action == BlockMutationBreak && !r.AllowBlockBreaking {
		return result, blockMutationDelivery{}, nil
	}

	if !authoritative && (action == BlockMutationPlace || action == BlockMutationInteract) && !r.AllowBlockPlacing {
		return result, blockMutationDelivery{}, nil
	}

	var player game.Player

	if !authoritative {
		player = session.snapshotPlayer()
	}

	policy := r.BlockMutationPolicy
	if policy == nil {
		policy = CreativeBlockMutationPolicy{}
	}

	seen := make(map[game.BlockPosition]struct{}, len(changes))
	committed := make([]game.BlockChange, 0, len(changes))
	states := make([]int32, 0, len(changes))

	breakState := int32(0)
	directBreakChanged := false

	lightingChanges := make([]game.BlockChange, 0, len(changes))

	if action == BlockMutationBreak && requiredChanges > 0 {
		var err error

		directBlock := r.World.BlockAt(changes[0].Position)
		directBreakChanged = directBlock != changes[0].Replacement

		breakState, err = protocolBlockState(directBlock)
		if err != nil {
			return BlockMutationResult{}, blockMutationDelivery{}, fmt.Errorf("encode broken block: %w", err)
		}
	}

	for index, change := range changes {
		if _, duplicate := seen[change.Position]; duplicate {
			return result, blockMutationDelivery{}, nil
		}

		seen[change.Position] = struct{}{}

		current := r.World.BlockAt(change.Position)

		if !change.Replacement.Valid() {
			return result, blockMutationDelivery{}, nil
		}

		if index < requiredChanges && !authoritative {
			if !session.hasLoadedBlock(change.Position) || !blockWithinInteractionRange(player, change.Position) {
				return result, blockMutationDelivery{}, nil
			}

			if action == BlockMutationPlace && !allowOccupied && current != game.Air {
				return result, blockMutationDelivery{}, nil
			}

			if action == BlockMutationPlace && change.Replacement == game.Air {
				return result, blockMutationDelivery{}, nil
			}

			mutation := BlockMutation{Player: player, Action: action, Position: change.Position, Current: current, Replacement: change.Replacement}
			if !policy.AllowBlockMutation(mutation) {
				return result, blockMutationDelivery{}, nil
			}
		}

		if current == change.Replacement {
			continue
		}

		if !current.SameLightProperties(change.Replacement) {
			lightingChanges = append(lightingChanges, change)
		}

		state, err := protocolBlockState(change.Replacement)
		if err != nil {
			return BlockMutationResult{}, blockMutationDelivery{}, fmt.Errorf("encode replacement block: %w", err)
		}

		committed = append(committed, change)
		states = append(states, state)
	}

	result.Allowed = true
	if len(committed) == 0 {
		return result, blockMutationDelivery{}, nil
	}

	if checkPlayerObstruction && r.placementObstructed(committed) {
		result.Allowed = false

		return result, blockMutationDelivery{}, nil
	}

	r.World.SetBlocks(committed)
	poseChanges := r.recalculateActivePlayerPoses()

	result.Block = committed[0].Replacement
	result.Changes = committed
	result.Changed = true

	deliveryComplete := make(chan struct{})

	delivery := blockMutationDelivery{
		session:            session,
		changes:            committed,
		states:             states,
		lightingChanges:    lightingChanges,
		directBreakChanged: directBreakChanged,
		breakPosition:      changes[0].Position,
		breakState:         breakState,
		recipients:         r.snapshotSessions(),
		poseChanges:        poseChanges,
		waitForDelivery:    r.blockMutationDeliveryTail,
		deliveryComplete:   deliveryComplete,
	}

	r.blockMutationDeliveryTail = deliveryComplete

	return result, delivery, nil
}

func (r *Runtime) completeBlockMutation(result BlockMutationResult, delivery blockMutationDelivery, err error) (BlockMutationResult, error) {
	if err != nil || !result.Changed {
		return result, err
	}

	var (
		lightUpdates []protocol.UpdateLight
		lightingErr  error
	)

	if len(delivery.lightingChanges) != 0 && r.World.Lighting == game.LightingNormal {
		lightUpdates, lightingErr = buildChangedLightUpdates(r.World, delivery.lightingChanges)
	}

	// Lighting may run concurrently, but clients must observe committed mutations
	// in the same order as the authoritative world.
	<-delivery.waitForDelivery
	defer close(delivery.deliveryComplete)

	if lightingErr != nil {
		return result, fmt.Errorf("recalculate lighting: %w", lightingErr)
	}

	sections := blockMutationSections(delivery.changes, delivery.states)

	for _, other := range delivery.recipients {
		for _, section := range sections {
			if len(section.position.Records) >= sectionBlockUpdateThreshold {
				err := other.sendSectionBlocksUpdateIfLoaded(section.position)
				if err != nil {
					other.Log.Warnf("[play] failed to update block section: %v\n", err)
				}

				continue
			}

			for index, change := range section.changes {
				err := other.sendBlockUpdateIfLoaded(change.Position, section.position.Records[index].State)
				if err != nil {
					other.Log.Warnf("[play] failed to update block: %v\n", err)
				}
			}
		}

		for _, update := range lightUpdates {
			err := other.sendLightUpdateIfLoaded(update)
			if err != nil {
				other.Log.Warnf("[play] failed to update light: %v\n", err)
			}
		}

		if delivery.directBreakChanged && other != delivery.session {
			event := protocol.LevelEvent{
				Event:    protocol.LevelEventBlockBreak,
				Position: delivery.breakPosition,
				Data:     delivery.breakState,
			}

			err := other.sendLevelEventIfLoaded(event)
			if err != nil {
				other.Log.Warnf("[play] failed to send block break effect: %v\n", err)
			}
		}
	}

	for _, player := range delivery.poseChanges {
		for _, other := range delivery.recipients {
			if other.snapshotPlayer().EntityID == player.EntityID || !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
				continue
			}

			err := other.sendPlayerMetadata(player)
			if err != nil {
				other.Log.Warnf("[play] failed to update player pose: %v\n", err)
			}
		}
	}

	return result, nil
}

func blockMutationSections(changes []game.BlockChange, states []int32) []blockMutationSection {
	sections := make([]blockMutationSection, 0)
	sectionIndexes := make(map[[3]int32]int)

	for index, change := range changes {
		sectionX := change.Position.X >> 4
		sectionY := change.Position.Y >> 4
		sectionZ := change.Position.Z >> 4

		key := [3]int32{sectionX, sectionY, sectionZ}

		sectionIndex, exists := sectionIndexes[key]
		if !exists {
			sectionIndex = len(sections)
			sectionIndexes[key] = sectionIndex

			sections = append(sections, blockMutationSection{position: protocol.SectionBlocksUpdate{
				SectionX: sectionX,
				SectionY: sectionY,
				SectionZ: sectionZ,
			}})
		}

		section := &sections[sectionIndex]

		section.position.Records = append(section.position.Records, protocol.SectionBlockUpdateRecord{
			X:     byte(change.Position.X & 15),
			Y:     byte(change.Position.Y & 15),
			Z:     byte(change.Position.Z & 15),
			State: states[index],
		})

		section.changes = append(section.changes, change)
	}

	return sections
}
