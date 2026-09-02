package server

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	blockInteractionRange       = 4.5
	blockInteractionBuffer      = 1.0
	sectionBlockUpdateThreshold = 8
)

const (
	BlockMutationBreak BlockMutationAction = iota
	BlockMutationPlace
	BlockMutationInteract
	blockMutationLiteral
)

const (
	blockMutationStructural blockMutationCause = iota
	blockMutationDirectPlace
	blockMutationDirectBreak
	blockMutationInteract
	blockMutationSupportLoss
)

const (
	blockLootNone blockLootContext = iota
	blockLootPlayer
	blockLootNoBreaker
)

type BlockMutationAction uint8

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

type blockMutationCause uint8

type blockMutationRecord struct {
	change            game.BlockChange
	previous          game.Block
	previousState     int32
	previousEntity    game.BlockEntity
	hadPreviousEntity bool
	cause             blockMutationCause
	lootContext       blockLootContext
	lootTool          game.ItemStack
}

type blockLootContext uint8

type blockMutationDelivery struct {
	session          *Session
	changes          []game.BlockChange
	states           []int32
	records          []blockMutationRecord
	lightingChanges  []game.BlockChange
	recipients       []*Session
	poseChanges      []game.Player
	waitForDelivery  <-chan struct{}
	deliveryComplete chan struct{}
	runtimeSounds    []positionalBlockSound
	runtimeEvents    []protocol.LevelEvent
	miningInventory  *game.PlayerInventory
	miningToolBroke  bool
}

type queuedBlockMutation struct {
	result   BlockMutationResult
	delivery blockMutationDelivery
}

type positionalBlockSound struct {
	position game.BlockPosition
	sound    protocol.Sound
	exclude  *Session
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
	return mutation.Player.GameMode == game.GameModeSurvival || mutation.Player.GameMode == game.GameModeCreative
}

func (r *Runtime) MutateBlock(session *Session, action BlockMutationAction, position game.BlockPosition, replacement game.Block) (BlockMutationResult, error) {
	return r.MutateBlocks(session, action, []game.BlockChange{{Position: position, Replacement: replacement}})
}

func (r *Runtime) mutateMinedBlockLocked(session *Session, position game.BlockPosition, tool miningTool) (BlockMutationResult, blockMutationDelivery, error) {
	changes := r.breakChanges(position)
	requiredChanges := len(changes)
	physicalBreaks := make(map[game.BlockPosition]struct{}, requiredChanges)

	for _, change := range changes {
		physicalBreaks[change.Position] = struct{}{}
	}

	changes = r.withStructuralNeighborChanges(changes)

	result, delivery, err := r.mutateBlocksLocked(session, BlockMutationBreak, changes, requiredChanges, true, false, false, true)
	if err != nil || !result.Changed {
		return result, delivery, err
	}

	for index := range delivery.records {
		record := &delivery.records[index]
		if _, physicalBreak := physicalBreaks[record.change.Position]; !physicalBreak {
			continue
		}

		if tool.stack.Item.IsCorrectToolForDrops(record.previous) {
			record.lootContext = blockLootPlayer
			record.lootTool = tool.stack.Clone()
		}
	}

	minedBlock := game.Air

	for _, record := range delivery.records {
		if record.change.Position == position {
			minedBlock = record.previous

			break
		}
	}

	miningInventory, broke := r.damageMiningTool(session, tool, minedBlock)

	r.addPlayerExhaustion(session, 0.005)

	delivery.miningInventory = miningInventory
	delivery.miningToolBroke = broke

	return result, delivery, nil
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

		return r.mutateBlocksLocked(session, action, changes, requiredChanges, true, false, false, true)
	}()

	return r.completeBlockMutation(result, delivery, err)
}

// MutateWorldBlocks applies authoritative world changes without player interaction restrictions.
func (r *Runtime) MutateWorldBlocks(changes []game.BlockChange) (BlockMutationResult, error) {
	return r.mutateWorldBlocks(changes, false, false)
}

// MutateWorldBlocksStrict applies authoritative changes without updating neighboring shapes.
func (r *Runtime) MutateWorldBlocksStrict(changes []game.BlockChange) (BlockMutationResult, error) {
	return r.mutateWorldBlocks(changes, true, false)
}

// MutateEmptyWorldBlocks applies authoritative changes only where the current block is air.
func (r *Runtime) MutateEmptyWorldBlocks(changes []game.BlockChange) (BlockMutationResult, error) {
	return r.mutateWorldBlocks(changes, false, true)
}

func (r *Runtime) mutateWorldBlocks(changes []game.BlockChange, strict, emptyOnly bool) (BlockMutationResult, error) {
	r.worldMutationMu.Lock()

	result, delivery, err := func() (BlockMutationResult, blockMutationDelivery, error) {
		defer r.worldMutationMu.Unlock()

		if emptyOnly {
			applicable := changes[:0]

			for _, change := range changes {
				if r.World.BlockAt(change.Position) == game.Air {
					applicable = append(applicable, change)
				}
			}

			changes = applicable
		}

		if !strict {
			changes = r.withAuthoritativeDoorChanges(changes)
			requiredChanges := len(changes)
			changes = r.withStructuralNeighborChanges(changes)

			return r.mutateBlocksLocked(nil, BlockMutationPlace, changes, requiredChanges, true, false, true, true)
		}

		return r.mutateBlocksLocked(nil, blockMutationLiteral, changes, len(changes), true, false, true, true)
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

		return r.mutateBlocksLocked(session, BlockMutationPlace, changes, 1, true, true, false, true)
	}()

	return r.completeBlockMutation(result, delivery, err)
}

func (r *Runtime) mutateBlocksLocked(session *Session, action BlockMutationAction, changes []game.BlockChange, requiredChanges int, allowOccupied, checkPlayerObstruction, authoritative, recalculatePlayerPoses bool, lifecycleLocked ...bool) (BlockMutationResult, blockMutationDelivery, error) {
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

	changes = r.withImmediateFluidMixing(changes)

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
	records := make([]blockMutationRecord, 0, len(changes))

	lightingChanges := make([]game.BlockChange, 0, len(changes))

	for index, change := range changes {
		if _, duplicate := seen[change.Position]; duplicate {
			return result, blockMutationDelivery{}, nil
		}

		seen[change.Position] = struct{}{}

		current := r.World.BlockAt(change.Position)

		cause := blockMutationStructural

		if !authoritative && index < requiredChanges {
			switch action {
			case BlockMutationPlace:
				cause = blockMutationDirectPlace
			case BlockMutationBreak:
				if index == 0 {
					cause = blockMutationDirectBreak
				}
			case BlockMutationInteract:
				cause = blockMutationInteract
			}
		} else if index >= requiredChanges && current != game.Air && change.Replacement == game.Air {
			cause = blockMutationSupportLoss
		}

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

		if action != blockMutationLiteral {
			change.Replacement = waterloggedMutationReplacement(current, change.Replacement, cause)
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

		previousState, err := protocolBlockState(current)
		if err != nil {
			return BlockMutationResult{}, blockMutationDelivery{}, fmt.Errorf("encode previous block: %w", err)
		}

		committed = append(committed, change)
		states = append(states, state)

		var previousEntity game.BlockEntity

		hadPreviousEntity := false

		if game.BlockEntityTypeForBlock(current) != game.BlockEntityTypeForBlock(change.Replacement) {
			previousEntity, hadPreviousEntity = r.World.BlockEntityAt(change.Position)
		}

		record := blockMutationRecord{
			change: change, previous: current, previousState: previousState,
			previousEntity: previousEntity, hadPreviousEntity: hadPreviousEntity, cause: cause,
		}

		if cause == blockMutationSupportLoss {
			record.lootContext = blockLootNoBreaker
		}

		records = append(records, record)
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

	r.promoteRandomTickSections(committed)

	r.scheduleFarmlandSurvivalChecksLocked(committed)

	r.scheduleFluidNeighborsLocked(committed)

	r.reconcileRuntimeBlockEntities(records)

	if len(lifecycleLocked) != 0 && lifecycleLocked[0] {
		r.closeRemovedBlockEntityMenusLocked(records)
	} else {
		r.closeRemovedBlockEntityMenus(records)
	}

	var poseChanges []game.Player

	if recalculatePlayerPoses {
		poseChanges = r.recalculateActivePlayerPoses()
	}

	result.Block = committed[0].Replacement
	result.Changes = committed
	result.Changed = true

	deliveryComplete := make(chan struct{})

	delivery := blockMutationDelivery{
		session:          session,
		changes:          committed,
		states:           states,
		records:          records,
		lightingChanges:  lightingChanges,
		recipients:       r.snapshotSessions(),
		poseChanges:      poseChanges,
		waitForDelivery:  r.blockMutationDeliveryTail,
		deliveryComplete: deliveryComplete,
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
		lightUpdates, lightingErr = buildChangedLightUpdatesWith(r.World, delivery.lightingChanges, r.chunkLightBuilder)
	}

	// Lighting may run concurrently, but clients must observe committed mutations
	// in the same order as the authoritative world.
	<-delivery.waitForDelivery
	defer close(delivery.deliveryComplete)

	deliveryErr := r.deliverBlockMutation(delivery, lightUpdates)
	if deliveryErr != nil {
		return result, deliveryErr
	}

	if lightingErr != nil {
		return result, fmt.Errorf("recalculate lighting: %w", lightingErr)
	}

	return result, nil
}

func (r *Runtime) deliverBlockMutation(delivery blockMutationDelivery, lightUpdates []protocol.UpdateLight) error {

	sections := blockMutationSections(delivery.changes, delivery.states)
	placementSound, hasPlacementSound := blockPlacementSound(delivery.records)
	interactionSound, hasInteractionSound := blockInteractionSound(delivery.records)

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

		for _, record := range delivery.records {
			if record.cause != blockMutationSupportLoss && (record.cause != blockMutationDirectBreak || other == delivery.session) {
				continue
			}

			event := protocol.LevelEvent{Event: protocol.LevelEventBlockBreak, Position: record.change.Position, Data: record.previousState}

			err := other.sendLevelEventIfLoaded(event)
			if err != nil {
				other.Log.Warnf("[play] failed to send block break effect: %v\n", err)
			}
		}

		if hasPlacementSound && other != delivery.session {
			err := other.sendSoundIfLoaded(placementSound.sound, placementSound.position)
			if err != nil {
				other.Log.Warnf("[play] failed to send block placement sound: %v\n", err)
			}
		}

		if hasInteractionSound && other != delivery.session {
			err := other.sendSoundIfLoaded(interactionSound.sound, interactionSound.position)
			if err != nil {
				other.Log.Warnf("[play] failed to send block interaction sound: %v\n", err)
			}
		}

		for _, runtimeSound := range delivery.runtimeSounds {
			if other == runtimeSound.exclude {
				continue
			}

			err := other.sendSoundIfLoaded(runtimeSound.sound, runtimeSound.position)
			if err != nil {
				other.Log.Warnf("[play] failed to send runtime block sound: %v\n", err)
			}
		}

		for _, event := range delivery.runtimeEvents {
			err := other.sendLevelEventIfLoaded(event)
			if err != nil {
				other.Log.Warnf("[play] failed to send runtime level event: %v\n", err)
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

	r.commitBlockEntityRemovalEffects(delivery.records)
	r.commitOrdinaryBlockDrops(delivery.records)

	if delivery.miningInventory != nil {
		err := delivery.session.synchronizePlayerInventoryMutation(*delivery.miningInventory)
		if err != nil {
			return fmt.Errorf("synchronize mining tool: %w", err)
		}
	}

	if delivery.miningToolBroke {
		player := delivery.session.snapshotPlayer()

		event := protocol.EntityEvent{EntityID: player.EntityID, Event: 47}

		for _, other := range delivery.recipients {
			if other != delivery.session && !playersVisible(other.snapshotPlayer(), player, other.renderDistance()) {
				continue
			}

			err := other.writePacket(protocol.ClientboundEntityEventID, event)
			if err != nil && other.Log != nil {
				other.Log.Warnf("[play] failed to send tool break event: %v\n", err)
			}
		}
	}

	return nil
}

func (r *Runtime) takeRuntimeBlockMutationsLocked() []queuedBlockMutation {
	deliveries := r.runtimeBlockMutations
	r.runtimeBlockMutations = nil

	return deliveries
}

func (r *Runtime) completeRuntimeBlockMutations(mutations []queuedBlockMutation) {
	if len(mutations) == 0 {
		return
	}

	lightingChanges := make([]game.BlockChange, 0)
	recipientSet := make(map[*Session]struct{})
	recipients := make([]*Session, 0)

	for _, mutation := range mutations {
		lightingChanges = append(lightingChanges, mutation.delivery.lightingChanges...)

		for _, session := range mutation.delivery.recipients {
			_, present := recipientSet[session]
			if present {
				continue
			}

			recipientSet[session] = struct{}{}
			recipients = append(recipients, session)
		}
	}

	var (
		lightUpdates []protocol.UpdateLight
		lightingErr  error
	)

	if len(lightingChanges) != 0 && r.World.Lighting == game.LightingNormal {
		lightUpdates, lightingErr = buildChangedLightUpdatesWith(r.World, lightingChanges, r.chunkLightBuilder)
	}

	// The captured queue is one contiguous delivery-chain segment. Waiting on
	// its external predecessor avoids waiting on completion nodes owned here.
	<-mutations[0].delivery.waitForDelivery

	for _, mutation := range mutations {
		err := r.deliverBlockMutation(mutation.delivery, nil)
		if err != nil {
			for _, session := range mutation.delivery.recipients {
				if session.Log != nil {
					session.Log.Warnf("[play] failed to complete runtime block mutation: %v\n", err)
				}
			}
		}
	}

	for _, session := range recipients {
		for _, update := range lightUpdates {
			err := session.sendLightUpdateIfLoaded(update)
			if err != nil && session.Log != nil {
				session.Log.Warnf("[play] failed to update light: %v\n", err)
			}
		}
	}

	if lightingErr != nil {
		for _, session := range recipients {
			if session.Log != nil {
				session.Log.Warnf("[play] failed to complete runtime block mutation: recalculate lighting: %v\n", lightingErr)
			}
		}
	}

	for _, mutation := range mutations {
		close(mutation.delivery.deliveryComplete)
	}
}

func blockWithinInteractionRange(player game.Player, position game.BlockPosition) bool {
	eyePosition := player.EyePosition()

	distanceX := distanceToInterval(eyePosition.X, float64(position.X), float64(position.X)+1)
	distanceY := distanceToInterval(eyePosition.Y, float64(position.Y), float64(position.Y)+1)
	distanceZ := distanceToInterval(eyePosition.Z, float64(position.Z), float64(position.Z)+1)

	distanceSquared := distanceX*distanceX + distanceY*distanceY + distanceZ*distanceZ
	maximum := blockInteractionRange + blockInteractionBuffer

	return distanceSquared < maximum*maximum
}

func distanceToInterval(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum - value
	}

	if value > maximum {
		return value - maximum
	}

	return 0
}

func blockPlacementSound(records []blockMutationRecord) (positionalBlockSound, bool) {
	for _, record := range records {
		if record.cause != blockMutationDirectPlace {
			continue
		}

		soundType := record.change.Replacement.SoundType()
		if soundType.Place == "" {
			return positionalBlockSound{}, false
		}

		return positionalSound(record.change.Position, soundType.Place, (soundType.Volume+1)/2, soundType.Pitch*0.8), true
	}

	return positionalBlockSound{}, false
}

func blockInteractionSound(records []blockMutationRecord) (positionalBlockSound, bool) {
	for _, record := range records {
		if record.cause != blockMutationInteract {
			continue
		}

		if blockProperty(record.previous, "powered") != blockProperty(record.change.Replacement, "powered") && record.change.Replacement.Behavior() == game.BlockBehaviorButton {
			event, valid := blockButtonSound(record.change.Replacement, blockProperty(record.change.Replacement, "powered") == "true")
			if !valid {
				return positionalBlockSound{}, false
			}

			return positionalSound(record.change.Position, event, 1, 1), true
		}

		if blockProperty(record.previous, "open") == blockProperty(record.change.Replacement, "open") {
			continue
		}

		event, valid := blockOpenCloseSound(record.change.Replacement, blockProperty(record.change.Replacement, "open") == "true")
		if !valid {
			return positionalBlockSound{}, false
		}

		return positionalSound(record.change.Position, event, 1, 0.9+rand.Float32()*0.1), true
	}

	return positionalBlockSound{}, false
}

func blockButtonSound(block game.Block, powered bool) (game.SoundEvent, bool) {
	definition, valid := block.Definition()
	if !valid || block.Behavior() != game.BlockBehaviorButton {
		return "", false
	}

	switch {
	case strings.HasPrefix(definition.Name, "bamboo_"):
		return chooseSound(powered, game.SoundBlockBambooWoodButtonClickOn, game.SoundBlockBambooWoodButtonClickOff), true
	case strings.HasPrefix(definition.Name, "cherry_"):
		return chooseSound(powered, game.SoundBlockCherryWoodButtonClickOn, game.SoundBlockCherryWoodButtonClickOff), true
	case strings.HasPrefix(definition.Name, "crimson_") || strings.HasPrefix(definition.Name, "warped_"):
		return chooseSound(powered, game.SoundBlockNetherWoodButtonClickOn, game.SoundBlockNetherWoodButtonClickOff), true
	case definition.Name == "stone_button" || definition.Name == "polished_blackstone_button":
		return chooseSound(powered, game.SoundBlockStoneButtonClickOn, game.SoundBlockStoneButtonClickOff), true
	default:
		return chooseSound(powered, game.SoundBlockWoodenButtonClickOn, game.SoundBlockWoodenButtonClickOff), true
	}
}

func positionalSound(position game.BlockPosition, event game.SoundEvent, volume, pitch float32) positionalBlockSound {
	return positionalBlockSound{
		position: position,
		sound: protocol.Sound{
			Event:  protocol.SoundEventHolder{Name: string(event)},
			Source: protocol.SoundSourceBlock,
			X:      float64(position.X) + 0.5,
			Y:      float64(position.Y) + 0.5,
			Z:      float64(position.Z) + 0.5,
			Volume: volume,
			Pitch:  pitch,
			Seed:   rand.Int64(),
		},
	}
}

func blockOpenCloseSound(block game.Block, open bool) (game.SoundEvent, bool) {
	definition, valid := block.Definition()
	if !valid {
		return "", false
	}

	variant := ""

	switch {
	case strings.HasPrefix(definition.Name, "bamboo_"):
		variant = "bamboo"
	case strings.HasPrefix(definition.Name, "cherry_"):
		variant = "cherry"
	case strings.HasPrefix(definition.Name, "crimson_") || strings.HasPrefix(definition.Name, "warped_"):
		variant = "nether"
	}

	switch block.Behavior() {
	case game.BlockBehaviorDoor:
		switch variant {
		case "bamboo":
			return chooseSound(open, game.SoundBlockBambooWoodDoorOpen, game.SoundBlockBambooWoodDoorClose), true
		case "cherry":
			return chooseSound(open, game.SoundBlockCherryWoodDoorOpen, game.SoundBlockCherryWoodDoorClose), true
		case "nether":
			return chooseSound(open, game.SoundBlockNetherWoodDoorOpen, game.SoundBlockNetherWoodDoorClose), true
		default:
			return chooseSound(open, game.SoundBlockWoodenDoorOpen, game.SoundBlockWoodenDoorClose), true
		}
	case game.BlockBehaviorTrapdoor:
		switch variant {
		case "bamboo":
			return chooseSound(open, game.SoundBlockBambooWoodTrapdoorOpen, game.SoundBlockBambooWoodTrapdoorClose), true
		case "cherry":
			return chooseSound(open, game.SoundBlockCherryWoodTrapdoorOpen, game.SoundBlockCherryWoodTrapdoorClose), true
		case "nether":
			return chooseSound(open, game.SoundBlockNetherWoodTrapdoorOpen, game.SoundBlockNetherWoodTrapdoorClose), true
		default:
			return chooseSound(open, game.SoundBlockWoodenTrapdoorOpen, game.SoundBlockWoodenTrapdoorClose), true
		}
	case game.BlockBehaviorFenceGate:
		switch variant {
		case "bamboo":
			return chooseSound(open, game.SoundBlockBambooWoodFenceGateOpen, game.SoundBlockBambooWoodFenceGateClose), true
		case "cherry":
			return chooseSound(open, game.SoundBlockCherryWoodFenceGateOpen, game.SoundBlockCherryWoodFenceGateClose), true
		case "nether":
			return chooseSound(open, game.SoundBlockNetherWoodFenceGateOpen, game.SoundBlockNetherWoodFenceGateClose), true
		default:
			return chooseSound(open, game.SoundBlockFenceGateOpen, game.SoundBlockFenceGateClose), true
		}
	default:
		return "", false
	}
}

func chooseSound(condition bool, whenTrue, whenFalse game.SoundEvent) game.SoundEvent {
	if condition {
		return whenTrue
	}

	return whenFalse
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
