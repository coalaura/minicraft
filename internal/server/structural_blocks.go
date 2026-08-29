package server

import (
	"math"
	"strconv"
	"strings"

	"github.com/coalaura/minicraft/internal/game"
)

type horizontalDirection uint8

const (
	directionNorth horizontalDirection = iota
	directionSouth
	directionWest
	directionEast
)

var horizontalDirections = [...]horizontalDirection{directionNorth, directionSouth, directionWest, directionEast}

func horizontalFacing(yaw float32) horizontalDirection {
	index := int(math.Floor(float64(yaw/90)+0.5)) & 3
	return [...]horizontalDirection{directionSouth, directionWest, directionNorth, directionEast}[index]
}

func (direction horizontalDirection) name() string {
	return [...]string{"north", "south", "west", "east"}[direction]
}

func (direction horizontalDirection) opposite() horizontalDirection {
	return [...]horizontalDirection{directionSouth, directionNorth, directionEast, directionWest}[direction]
}

func (direction horizontalDirection) left() horizontalDirection {
	return [...]horizontalDirection{directionWest, directionEast, directionSouth, directionNorth}[direction]
}

func (direction horizontalDirection) right() horizontalDirection {
	return direction.left().opposite()
}

func (direction horizontalDirection) offset(position game.BlockPosition) (game.BlockPosition, bool) {
	switch direction {
	case directionNorth:
		if position.Z == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		position.Z--
	case directionSouth:
		if position.Z == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		position.Z++
	case directionWest:
		if position.X == math.MinInt32 {
			return game.BlockPosition{}, false
		}

		position.X--
	case directionEast:
		if position.X == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		position.X++
	}

	return position, true
}

func directionFromName(name string) (horizontalDirection, bool) {
	for _, direction := range horizontalDirections {
		if direction.name() == name {
			return direction, true
		}
	}

	return 0, false
}

func sameBlockType(first, second game.Block) bool {
	firstDefinition, firstOK := first.Definition()
	secondDefinition, secondOK := second.Definition()

	return firstOK && secondOK && firstDefinition.ID == secondDefinition.ID
}

func blockProperty(block game.Block, name string) string {
	value, _ := block.Property(name)
	return value
}

func blockPropertyInt(block game.Block, name string) int {
	value, err := strconv.Atoi(blockProperty(block, name))
	if err != nil {
		return 0
	}

	return value
}

func withBlockProperties(block game.Block, values ...game.BlockPropertyValue) game.Block {
	state, ok := block.WithProperties(values...)
	if !ok {
		return block
	}

	return state
}

func waterloggedMutationReplacement(current, replacement game.Block, cause blockMutationCause) game.Block {
	currentFluid := current.FluidState()

	doubleSlab := replacement.Behavior() == game.BlockBehaviorSlab && blockProperty(replacement, "type") == "double"
	if (cause == blockMutationDirectPlace || cause == blockMutationStructural) && currentFluid.Type() == game.FluidTypeWater && replacement.Waterloggable() && !doubleSlab {
		return withBlockProperties(replacement, game.BlockPropertyValue{Name: "waterlogged", Value: "true"})
	}

	waterlogged, valid := current.Property("waterlogged")
	if (cause == blockMutationDirectBreak || cause == blockMutationSupportLoss || cause == blockMutationStructural) && replacement == game.Air && valid && waterlogged == "true" {
		return game.Water
	}

	return replacement
}

func (r *Runtime) breakChanges(position game.BlockPosition) []game.BlockChange {
	block := r.World.BlockAt(position)

	changes := []game.BlockChange{{Position: position, Replacement: game.Air}}

	if !isTwoBlockDoor(block) {
		return changes
	}

	other := position

	if blockProperty(block, "half") == "upper" {
		other.Y--
	} else {
		other.Y++
	}

	otherBlock := r.World.BlockAt(other)

	if isTwoBlockDoor(otherBlock) && sameBlockType(block, otherBlock) && blockProperty(block, "half") != blockProperty(otherBlock, "half") {
		changes = append(changes, game.BlockChange{Position: other, Replacement: game.Air})
	}

	return changes
}

func isTwoBlockDoor(block game.Block) bool {
	return block.Behavior() == game.BlockBehaviorDoor
}

func (r *Runtime) withAuthoritativeDoorChanges(primary []game.BlockChange) []game.BlockChange {
	changeIndexes := make(map[game.BlockPosition]int, len(primary))

	for index, change := range primary {
		changeIndexes[change.Position] = index
	}

	changes := append([]game.BlockChange(nil), primary...)

	for _, change := range primary {
		current := r.World.BlockAt(change.Position)
		if current == change.Replacement || !isTwoBlockDoor(current) {
			continue
		}

		otherPosition := change.Position

		if blockProperty(current, "half") == "upper" {
			if otherPosition.Y == math.MinInt32 {
				continue
			}

			otherPosition.Y--
		} else {
			if otherPosition.Y == math.MaxInt32 {
				continue
			}

			otherPosition.Y++
		}

		other := r.World.BlockAt(otherPosition)
		if !isTwoBlockDoor(other) || !sameBlockType(current, other) || blockProperty(current, "half") == blockProperty(other, "half") {
			continue
		}

		otherIndex, changed := changeIndexes[otherPosition]
		if changed {
			if changes[otherIndex].Replacement == other {
				changes[otherIndex].Replacement = game.Air
			}

			continue
		}

		changes = append(changes, game.BlockChange{Position: otherPosition, Replacement: game.Air})

		changeIndexes[otherPosition] = len(changes) - 1
	}

	return changes
}

func (r *Runtime) withStructuralNeighborChanges(primary []game.BlockChange) []game.BlockChange {
	overlay := make(map[game.BlockPosition]game.Block, len(primary)+8)
	positions := make([]game.BlockPosition, 0, len(primary)*7)
	seenPositions := make(map[game.BlockPosition]struct{}, len(primary)*7)
	queue := make([]game.BlockPosition, 0, len(primary)*7)
	pending := make(map[game.BlockPosition]struct{}, len(primary)*7)

	enqueue := func(position game.BlockPosition) {
		if _, exists := pending[position]; exists {
			return
		}

		pending[position] = struct{}{}
		queue = append(queue, position)
	}

	for _, change := range primary {
		overlay[change.Position] = change.Replacement
		enqueueStructuralNeighborhood(change.Position, enqueue)
	}

	blockAt := func(position game.BlockPosition) game.Block {
		if block, ok := overlay[position]; ok {
			return block
		}

		return r.World.BlockAt(position)
	}

	for len(queue) != 0 {
		position := queue[0]
		queue = queue[1:]
		delete(pending, position)

		positions = appendStructuralPosition(positions, seenPositions, position)

		block := blockAt(position)

		updated := recalculateStructuralBlock(blockAt, position, block)

		if updated != block {
			overlay[position] = updated

			enqueueStructuralNeighborhood(position, enqueue)
		}
	}

	changes := make([]game.BlockChange, 0, len(overlay))
	added := make(map[game.BlockPosition]struct{}, len(overlay))

	for _, change := range primary {
		change.Replacement = overlay[change.Position]

		changes = append(changes, change)

		added[change.Position] = struct{}{}
	}

	for _, position := range positions {
		block, changed := overlay[position]
		if !changed || block == r.World.BlockAt(position) {
			continue
		}

		if _, exists := added[position]; exists {
			continue
		}

		changes = append(changes, game.BlockChange{Position: position, Replacement: block})
	}

	return changes
}

func enqueueStructuralNeighborhood(position game.BlockPosition, enqueue func(game.BlockPosition)) {
	enqueue(position)

	for _, direction := range horizontalDirections {
		neighbor, ok := direction.offset(position)
		if ok {
			enqueue(neighbor)
		}
	}

	if position.Y < math.MaxInt32 {
		above := position

		above.Y++

		enqueue(above)
	}

	if position.Y > math.MinInt32 {
		below := position

		below.Y--

		enqueue(below)
	}
}

func appendStructuralPosition(positions []game.BlockPosition, seen map[game.BlockPosition]struct{}, position game.BlockPosition) []game.BlockPosition {
	if _, exists := seen[position]; exists {
		return positions
	}

	seen[position] = struct{}{}

	return append(positions, position)
}

func recalculateStructuralBlock(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	_, hasSnowy := block.Property("snowy")

	if hasSnowy {
		snowy := false

		if position.Y < math.MaxInt32 {
			above := position

			above.Y++

			aboveDefinition, valid := blockAt(above).Definition()

			snowy = valid && aboveDefinition.Name == "snow"
		}

		block = withBlockProperties(block, game.BlockPropertyValue{Name: "snowy", Value: boolProperty(snowy)})
	}

	switch block.Behavior() {
	case game.BlockBehaviorStairs:
		return recalculateStairs(blockAt, position, block)
	case game.BlockBehaviorFenceGate:
		return recalculateFenceGate(blockAt, position, block)
	case game.BlockBehaviorFence, game.BlockBehaviorPane:
		return recalculateConnections(blockAt, position, block)
	case game.BlockBehaviorWall:
		return recalculateWall(blockAt, position, block)
	case game.BlockBehaviorDoor:
		return recalculateDoor(blockAt, position, block)
	case game.BlockBehaviorSupported:
		if supportedFromBelow(blockAt, position, block) {
			return block
		}

		return game.Air
	case game.BlockBehaviorButton:
		if buttonSupported(blockAt, position, block) {
			return block
		}

		return game.Air
	case game.BlockBehaviorPlant:
		if plantSupported(blockAt, position, block) {
			return block
		}

		return game.Air
	case game.BlockBehaviorPointedDripstone:
		return recalculatePointedDripstone(blockAt, position, block)
	case game.BlockBehaviorChest:
		return recalculateChest(blockAt, position, block)
	default:
		return block
	}
}

func recalculateChest(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	facing, valid := directionFromName(blockProperty(block, "facing"))
	if !valid {
		return block
	}

	chestType := blockProperty(block, "type")
	if chestType != "single" {
		connected := chestConnectedDirection(facing, chestType)

		neighbor := blockInDirection(blockAt, position, connected)
		if !chestBlocksCanConnect(block, neighbor) {
			return withBlockProperties(block, game.BlockPropertyValue{Name: "type", Value: "single"})
		}

		_, _, copper := copperChestProperties(block)

		if copper {
			normalized, _ := normalizedCopperChestBlock(block, neighbor)

			return copyChestProperties(normalized, block)
		}

		return block
	}

	for _, direction := range horizontalDirections {
		neighbor := blockInDirection(blockAt, position, direction)
		if !chestBlocksCanConnect(block, neighbor) || blockProperty(neighbor, "type") == "single" || blockProperty(neighbor, "facing") != facing.name() {
			continue
		}

		neighborType := blockProperty(neighbor, "type")

		neighborConnected := chestConnectedDirection(facing, neighborType)
		if neighborConnected != direction.opposite() {
			continue
		}

		replacement := withBlockProperties(block, game.BlockPropertyValue{Name: "type", Value: oppositeChestType(neighborType)})

		_, _, copper := copperChestProperties(block)

		if copper {
			normalized, _ := normalizedCopperChestBlock(block, neighbor)

			return copyChestProperties(normalized, replacement)
		}

		return replacement
	}

	return block
}

func chestConnectedDirection(facing horizontalDirection, chestType string) horizontalDirection {
	if chestType == "left" {
		return facing.right()
	}

	return facing.left()
}

func oppositeChestType(chestType string) string {
	if chestType == "left" {
		return "right"
	}

	return "left"
}

func validPlacementSupport(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block, rule game.ItemPlacementRule) bool {
	switch rule {
	case game.ItemPlacementSupported, game.ItemPlacementPressurePlate, game.ItemPlacementWeightedPressurePlate, game.ItemPlacementSnow, game.ItemPlacementCandle:
		return supportedFromBelow(blockAt, position, block)
	case game.ItemPlacementButton:
		return buttonSupported(blockAt, position, block)
	case game.ItemPlacementPlant:
		return plantSupported(blockAt, position, block)
	case game.ItemPlacementPointedDripstone:
		return pointedDripstoneSupported(blockAt, position, block)
	default:
		return true
	}
}

func supportedFromBelow(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) bool {
	if position.Y == math.MinInt32 {
		return false
	}

	below := position

	below.Y--

	support := blockAt(below)

	definition, _ := block.Definition()
	if isCarpet(definition.Name) {
		return support != game.Air
	}

	if definition.Name == "snow" {
		if support.HasTrait(game.BlockTraitSnowCannotSurviveOn) {
			return false
		}

		if support.HasTrait(game.BlockTraitSnowCanSurviveOn) {
			return true
		}

		if sameBlockType(block, support) {
			return blockPropertyInt(support, "layers") == 8
		}

		return support.FullFace(game.BlockFaceUp)
	}

	if strings.HasSuffix(definition.Name, "_pressure_plate") {
		return support.SupportsRigid(game.BlockFaceUp) || support.SupportsCenter(game.BlockFaceUp)
	}

	return support.SupportsCenter(game.BlockFaceUp)
}

func isCarpet(name string) bool {
	return name == "moss_carpet" || name != "pale_moss_carpet" && strings.HasSuffix(name, "_carpet")
}

func buttonSupported(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) bool {
	support := position
	supportFace := game.BlockFaceUp

	switch blockProperty(block, "face") {
	case "floor":
		if support.Y == math.MinInt32 {
			return false
		}

		support.Y--
	case "ceiling":
		if support.Y == math.MaxInt32 {
			return false
		}

		support.Y++
		supportFace = game.BlockFaceDown
	case "wall":
		facing, valid := directionFromName(blockProperty(block, "facing"))
		if !valid {
			return false
		}

		var offsetValid bool

		support, offsetValid = facing.opposite().offset(support)
		if !offsetValid {
			return false
		}

		supportFace = horizontalGameFace(facing)
	default:
		return false
	}

	return blockAt(support).FaceSturdy(supportFace)
}

func horizontalGameFace(direction horizontalDirection) game.BlockFace {
	switch direction {
	case directionNorth:
		return game.BlockFaceNorth
	case directionSouth:
		return game.BlockFaceSouth
	case directionWest:
		return game.BlockFaceWest
	default:
		return game.BlockFaceEast
	}
}

func plantSupported(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, plant game.Block) bool {
	if position.Y == math.MinInt32 {
		return false
	}

	below := position

	below.Y--

	supportBlock := blockAt(below)

	supportDefinition, valid := supportBlock.Definition()
	if !valid {
		return false
	}

	plantDefinition, _ := plant.Definition()
	if plantDefinition.Name == "dead_bush" || strings.HasSuffix(plantDefinition.Name, "dry_grass") {
		return supportBlock.HasTrait(game.BlockTraitDirt) || supportDefinition.Name == "farmland" || supportDefinition.Name == "sand" || supportDefinition.Name == "red_sand" || strings.HasSuffix(supportDefinition.Name, "terracotta")
	}

	if plantDefinition.Name == "wither_rose" {
		return supportBlock.HasTrait(game.BlockTraitDirt) || supportDefinition.Name == "farmland" || supportDefinition.Name == "netherrack" || supportDefinition.Name == "soul_sand" || supportDefinition.Name == "soul_soil"
	}

	return supportBlock.HasTrait(game.BlockTraitDirt) || supportDefinition.Name == "farmland"
}

func recalculateDoor(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	otherPosition := position

	if blockProperty(block, "half") == "upper" {
		if otherPosition.Y == math.MinInt32 {
			return game.Air
		}

		otherPosition.Y--

		other := blockAt(otherPosition)
		if other.Behavior() == game.BlockBehaviorDoor && sameBlockType(block, other) && blockProperty(other, "half") == "lower" {
			return block
		}

		return game.Air
	}

	if otherPosition.Y == math.MinInt32 || otherPosition.Y == math.MaxInt32 {
		return game.Air
	}

	below := otherPosition

	below.Y--

	otherPosition.Y++

	other := blockAt(otherPosition)

	if blockAt(below).FaceSturdy(game.BlockFaceUp) && other.Behavior() == game.BlockBehaviorDoor && sameBlockType(block, other) && blockProperty(other, "half") == "upper" {
		return block
	}

	return game.Air
}

func pointedDripstoneSupported(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) bool {
	direction := blockProperty(block, "vertical_direction")

	support, valid := verticalOffset(position, direction == "down")
	if !valid {
		return false
	}

	neighbor := blockAt(support)
	supportFace := game.BlockFaceUp

	if direction == "down" {
		supportFace = game.BlockFaceDown
	}

	if neighbor.FaceSturdy(supportFace) {
		return true
	}

	return neighbor.Behavior() == game.BlockBehaviorPointedDripstone && blockProperty(neighbor, "vertical_direction") == direction
}

func recalculatePointedDripstone(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	if !pointedDripstoneSupported(blockAt, position, block) {
		return game.Air
	}

	direction := blockProperty(block, "vertical_direction")

	tipPosition, valid := verticalOffset(position, direction == "up")
	if !valid {
		return block
	}

	tipNeighbor := blockAt(tipPosition)
	thickness := "tip"

	if tipNeighbor.Behavior() == game.BlockBehaviorPointedDripstone {
		neighborDirection := blockProperty(tipNeighbor, "vertical_direction")
		neighborThickness := blockProperty(tipNeighbor, "thickness")

		if neighborDirection != direction {
			if neighborThickness == "tip" || neighborThickness == "tip_merge" {
				thickness = "tip_merge"
			}
		} else {
			switch neighborThickness {
			case "tip", "tip_merge":
				thickness = "frustum"
			default:
				supportPosition, supportValid := verticalOffset(position, direction == "down")
				if supportValid {
					supportNeighbor := blockAt(supportPosition)
					if supportNeighbor.Behavior() == game.BlockBehaviorPointedDripstone && blockProperty(supportNeighbor, "vertical_direction") == direction {
						thickness = "middle"
					} else {
						thickness = "base"
					}
				}
			}
		}
	}

	return withBlockProperties(block, game.BlockPropertyValue{Name: "thickness", Value: thickness})
}

func verticalOffset(position game.BlockPosition, up bool) (game.BlockPosition, bool) {
	if up {
		if position.Y == math.MaxInt32 {
			return game.BlockPosition{}, false
		}

		position.Y++

		return position, true
	}

	if position.Y == math.MinInt32 {
		return game.BlockPosition{}, false
	}

	position.Y--

	return position, true
}

func recalculateStairs(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	facing, ok := directionFromName(blockProperty(block, "facing"))
	if !ok {
		return block
	}

	half := blockProperty(block, "half")

	shape := "straight"

	front := blockInDirection(blockAt, position, facing)

	if stairCornerCompatible(front, half, facing) {
		neighborFacing, _ := directionFromName(blockProperty(front, "facing"))

		if differentStair(blockAt, position, neighborFacing.opposite(), block, half) {
			if neighborFacing == facing.left() {
				shape = "outer_left"
			} else {
				shape = "outer_right"
			}
		}
	}

	if shape == "straight" {
		back := blockInDirection(blockAt, position, facing.opposite())

		if stairCornerCompatible(back, half, facing) {
			neighborFacing, _ := directionFromName(blockProperty(back, "facing"))

			if differentStair(blockAt, position, neighborFacing, block, half) {
				if neighborFacing == facing.left() {
					shape = "inner_left"
				} else {
					shape = "inner_right"
				}
			}
		}
	}

	return withBlockProperties(block, game.BlockPropertyValue{Name: "shape", Value: shape})
}

func stairCornerCompatible(block game.Block, half string, facing horizontalDirection) bool {
	if block.Behavior() != game.BlockBehaviorStairs || blockProperty(block, "half") != half {
		return false
	}

	neighborFacing, ok := directionFromName(blockProperty(block, "facing"))
	return ok && neighborFacing != facing && neighborFacing != facing.opposite()
}

func differentStair(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, direction horizontalDirection, block game.Block, half string) bool {
	other := blockInDirection(blockAt, position, direction)
	return other.Behavior() != game.BlockBehaviorStairs || blockProperty(other, "facing") != blockProperty(block, "facing") || blockProperty(other, "half") != half
}

func recalculateFenceGate(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	facing, ok := directionFromName(blockProperty(block, "facing"))
	if !ok {
		return block
	}

	inWall := blockInDirection(blockAt, position, facing.left()).Behavior() == game.BlockBehaviorWall || blockInDirection(blockAt, position, facing.right()).Behavior() == game.BlockBehaviorWall

	return withBlockProperties(block, game.BlockPropertyValue{Name: "in_wall", Value: boolProperty(inWall)})
}

func recalculateConnections(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	values := make([]game.BlockPropertyValue, 0, 4)

	for _, direction := range horizontalDirections {
		neighbor := blockInDirection(blockAt, position, direction)

		connected := connectsTo(block.Behavior(), direction, neighbor)

		values = append(values, game.BlockPropertyValue{Name: direction.name(), Value: boolProperty(connected)})
	}

	return withBlockProperties(block, values...)
}

func connectsTo(family game.BlockBehavior, direction horizontalDirection, neighbor game.Block) bool {
	neighborFamily := neighbor.Behavior()
	if neighborFamily == game.BlockBehaviorSolid {
		return true
	}

	switch family {
	case game.BlockBehaviorFence:
		if neighborFamily == game.BlockBehaviorFence {
			return true
		}

		if neighborFamily == game.BlockBehaviorFenceGate {
			facing, ok := directionFromName(blockProperty(neighbor, "facing"))
			return ok && facing != direction && facing != direction.opposite()
		}
	case game.BlockBehaviorPane:
		return neighborFamily == game.BlockBehaviorPane
	case game.BlockBehaviorWall:
		return neighborFamily == game.BlockBehaviorWall || neighborFamily == game.BlockBehaviorFenceGate
	}

	return false
}

func recalculateWall(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	values := make([]game.BlockPropertyValue, 0, 5)
	connected := make(map[horizontalDirection]bool, 4)

	for _, direction := range horizontalDirections {
		neighbor := blockInDirection(blockAt, position, direction)

		connected[direction] = connectsTo(game.BlockBehaviorWall, direction, neighbor)

		arm := "none"

		if connected[direction] {
			arm = "low"
		}

		if neighbor.Behavior() == game.BlockBehaviorSolid {
			arm = "tall"
		}

		values = append(values, game.BlockPropertyValue{Name: direction.name(), Value: arm})
	}

	straight := connected[directionNorth] && connected[directionSouth] && !connected[directionWest] && !connected[directionEast]
	straight = straight || connected[directionWest] && connected[directionEast] && !connected[directionNorth] && !connected[directionSouth]

	tall := false

	for _, value := range values {
		tall = tall || value.Value == "tall"
	}

	above := position

	if above.Y < math.MaxInt32 {
		above.Y++
	}

	aboveSolid := above != position && blockAt(above).Behavior() == game.BlockBehaviorSolid

	values = append(values, game.BlockPropertyValue{Name: "up", Value: boolProperty(!straight || tall || aboveSolid)})

	return withBlockProperties(block, values...)
}

func boolProperty(value bool) string {
	if value {
		return "true"
	}

	return "false"
}

func blockInDirection(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, direction horizontalDirection) game.Block {
	neighbor, ok := direction.offset(position)
	if !ok {
		return game.Air
	}

	return blockAt(neighbor)
}
