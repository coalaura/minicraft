package server

import (
	"math"

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

func withBlockProperties(block game.Block, values ...game.BlockPropertyValue) game.Block {
	state, ok := block.WithProperties(values...)
	if !ok {
		return block
	}

	return state
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

	if otherBlock := r.World.BlockAt(other); isTwoBlockDoor(otherBlock) && sameBlockType(block, otherBlock) && blockProperty(block, "half") != blockProperty(otherBlock, "half") {
		changes = append(changes, game.BlockChange{Position: other, Replacement: game.Air})
	}

	return changes
}

func isTwoBlockDoor(block game.Block) bool {
	return block.Behavior() == game.BlockBehaviorDoor || sameBlockType(block, game.IronDoor)
}

func (r *Runtime) withStructuralNeighborChanges(primary []game.BlockChange) []game.BlockChange {
	overlay := make(map[game.BlockPosition]game.Block, len(primary)+8)
	positions := make([]game.BlockPosition, 0, len(primary)*5)
	seenPositions := make(map[game.BlockPosition]struct{}, len(primary)*5)

	for _, change := range primary {
		overlay[change.Position] = change.Replacement

		positions = appendStructuralPosition(positions, seenPositions, change.Position)

		for _, direction := range horizontalDirections {
			neighbor, ok := direction.offset(change.Position)
			if ok {
				positions = appendStructuralPosition(positions, seenPositions, neighbor)
			}
		}

		if change.Position.Y < math.MaxInt32 {
			above := change.Position

			above.Y++

			positions = appendStructuralPosition(positions, seenPositions, above)
		}

		if change.Position.Y > math.MinInt32 {
			below := change.Position

			below.Y--

			positions = appendStructuralPosition(positions, seenPositions, below)
		}
	}

	blockAt := func(position game.BlockPosition) game.Block {
		if block, ok := overlay[position]; ok {
			return block
		}

		return r.World.BlockAt(position)
	}

	for _, position := range positions {
		block := blockAt(position)

		updated := recalculateStructuralBlock(blockAt, position, block)

		if updated != block {
			overlay[position] = updated
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

func appendStructuralPosition(positions []game.BlockPosition, seen map[game.BlockPosition]struct{}, position game.BlockPosition) []game.BlockPosition {
	if _, exists := seen[position]; exists {
		return positions
	}

	seen[position] = struct{}{}

	return append(positions, position)
}

func recalculateStructuralBlock(blockAt func(game.BlockPosition) game.Block, position game.BlockPosition, block game.Block) game.Block {
	switch block.Behavior() {
	case game.BlockBehaviorStairs:
		return recalculateStairs(blockAt, position, block)
	case game.BlockBehaviorFenceGate:
		return recalculateFenceGate(blockAt, position, block)
	case game.BlockBehaviorFence, game.BlockBehaviorPane:
		return recalculateConnections(blockAt, position, block)
	case game.BlockBehaviorWall:
		return recalculateWall(blockAt, position, block)
	default:
		return block
	}
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
