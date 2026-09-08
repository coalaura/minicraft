package game

import "strconv"

const (
	playerWidth            = 0.6
	standingPlayerHeight   = 1.8
	crouchingPlayerHeight  = 1.5
	crawlingPlayerHeight   = 0.6
	blockUnit              = 1.0 / 16.0
	maxBlockCollisionBoxes = 7
)

type AABB struct {
	MinX float64
	MinY float64
	MinZ float64
	MaxX float64
	MaxY float64
	MaxZ float64
}

func (box AABB) Intersects(other AABB) bool {
	return box.MaxX > other.MinX && box.MinX < other.MaxX &&
		box.MaxY > other.MinY && box.MinY < other.MaxY &&
		box.MaxZ > other.MinZ && box.MinZ < other.MaxZ
}

func (box AABB) Translate(x, y, z float64) AABB {
	box.MinX += x
	box.MaxX += x
	box.MinY += y
	box.MaxY += y
	box.MinZ += z
	box.MaxZ += z

	return box
}

func (player Player) CollisionBox() AABB {
	height := standingPlayerHeight

	switch player.Pose {
	case PlayerPoseCrouching:
		height = crouchingPlayerHeight
	case PlayerPoseCrawling:
		height = crawlingPlayerHeight
	}

	halfWidth := playerWidth / 2

	return AABB{
		MinX: player.Position.X - halfWidth,
		MinY: player.Position.Y,
		MinZ: player.Position.Z - halfWidth,
		MaxX: player.Position.X + halfWidth,
		MaxY: player.Position.Y + height,
		MaxZ: player.Position.Z + halfWidth,
	}
}

func (block Block) CollisionBoxes(position BlockPosition) []AABB {
	return block.AppendCollisionBoxes(nil, position)
}

func (block Block) AppendCollisionBoxes(boxes []AABB, position BlockPosition) []AABB {
	definition, valid := block.Definition()
	if !valid {
		return boxes
	}

	start := len(boxes)

	switch definition.Collision {
	case BlockCollisionFull:
		boxes = append(boxes, unitBox(0, 0, 0, 16, 16, 16))
	case BlockCollisionSlab:
		boxes = appendSlabCollisionBoxes(boxes, block)
	case BlockCollisionStairs:
		boxes = appendStairCollisionBoxes(boxes, block)
	case BlockCollisionDoor:
		boxes = appendDoorCollisionBoxes(boxes, block)
	case BlockCollisionTrapdoor:
		boxes = appendTrapdoorCollisionBoxes(boxes, block)
	case BlockCollisionFenceGate:
		boxes = appendFenceGateCollisionBoxes(boxes, block)
	case BlockCollisionFence:
		boxes = appendConnectedCollisionBoxes(boxes, block, 6, 10, 24)
	case BlockCollisionPane:
		boxes = appendConnectedCollisionBoxes(boxes, block, 7, 9, 16)
	case BlockCollisionWall:
		boxes = appendWallCollisionBoxes(boxes, block)
	case BlockCollisionCarpet:
		boxes = append(boxes, unitBox(0, 0, 0, 16, 1, 16))
	case BlockCollisionSnow:
		boxes = appendSnowCollisionBoxes(boxes, block)
	case BlockCollisionPointedDripstone:
		boxes = appendPointedDripstoneCollisionBoxes(boxes, block)
	case BlockCollisionChain:
		boxes = appendChainCollisionBoxes(boxes, block)
	case BlockCollisionCake:
		boxes = appendCakeCollisionBoxes(boxes, block)
	case BlockCollisionChest:
		boxes = appendChestCollisionBoxes(boxes, block)
	case BlockCollisionHopper:
		boxes = appendHopperCollisionBoxes(boxes, block)
	case BlockCollisionBed:
		boxes = appendBedCollisionBoxes(boxes, block)
	}

	for index := start; index < len(boxes); index++ {
		boxes[index] = boxes[index].Translate(float64(position.X), float64(position.Y), float64(position.Z))
	}

	return boxes
}

func (block Block) OutlineBoxes(position BlockPosition) []AABB {
	boxes := block.AppendCollisionBoxes(nil, position)
	if len(boxes) != 0 {
		return boxes
	}

	if block == Air || (!block.FluidState().Empty() && !block.Waterloggable()) {
		return nil
	}

	return []AABB{{
		MinX: float64(position.X),
		MinY: float64(position.Y),
		MinZ: float64(position.Z),
		MaxX: float64(position.X + 1),
		MaxY: float64(position.Y + 1),
		MaxZ: float64(position.Z + 1),
	}}
}

func appendBedCollisionBoxes(boxes []AABB, block Block) []AABB {
	boxes = append(boxes, unitBox(0, 3, 0, 16, 9, 16))

	switch collisionProperty(block, "facing") {
	case "north":
		boxes = append(boxes, unitBox(0, 0, 0, 3, 3, 3), unitBox(13, 0, 0, 16, 3, 3))
	case "south":
		boxes = append(boxes, unitBox(0, 0, 13, 3, 3, 16), unitBox(13, 0, 13, 16, 3, 16))
	case "west":
		boxes = append(boxes, unitBox(0, 0, 0, 3, 3, 3), unitBox(0, 0, 13, 3, 3, 16))
	case "east":
		boxes = append(boxes, unitBox(13, 0, 0, 16, 3, 3), unitBox(13, 0, 13, 16, 3, 16))
	}

	return boxes
}

func appendHopperCollisionBoxes(boxes []AABB, block Block) []AABB {
	boxes = append(boxes,
		unitBox(0, 10, 0, 16, 11, 16),
		unitBox(0, 11, 0, 2, 16, 16),
		unitBox(14, 11, 0, 16, 16, 16),
		unitBox(2, 11, 0, 14, 16, 2),
		unitBox(2, 11, 14, 14, 16, 16),
		unitBox(4, 4, 4, 12, 10, 12),
	)

	switch collisionProperty(block, "facing") {
	case "down":
		boxes = append(boxes, unitBox(6, 0, 6, 10, 4, 10))
	case "north":
		boxes = append(boxes, unitBox(6, 4, 0, 10, 8, 4))
	case "south":
		boxes = append(boxes, unitBox(6, 4, 12, 10, 8, 16))
	case "west":
		boxes = append(boxes, unitBox(0, 4, 6, 4, 8, 10))
	case "east":
		boxes = append(boxes, unitBox(12, 4, 6, 16, 8, 10))
	}

	return boxes
}

func appendChestCollisionBoxes(boxes []AABB, block Block) []AABB {
	if collisionProperty(block, "type") == "single" {
		return append(boxes, unitBox(1, 0, 1, 15, 14, 15))
	}

	connected := chestCollisionConnectedDirection(block)

	switch connected {
	case "north":
		return append(boxes, unitBox(1, 0, 0, 15, 14, 15))
	case "south":
		return append(boxes, unitBox(1, 0, 1, 15, 14, 16))
	case "west":
		return append(boxes, unitBox(0, 0, 1, 15, 14, 15))
	case "east":
		return append(boxes, unitBox(1, 0, 1, 16, 14, 15))
	default:
		return boxes
	}
}

func chestCollisionConnectedDirection(block Block) string {
	facing := collisionProperty(block, "facing")
	left := collisionProperty(block, "type") == "left"

	if left {
		return map[string]string{"north": "east", "east": "south", "south": "west", "west": "north"}[facing]
	}

	return map[string]string{"north": "west", "west": "south", "south": "east", "east": "north"}[facing]
}

func appendCakeCollisionBoxes(boxes []AABB, block Block) []AABB {
	bites := collisionPropertyInt(block, "bites")
	return append(boxes, unitBox(float64(1+bites*2), 0, 1, 15, 8, 15))
}

func appendChainCollisionBoxes(boxes []AABB, block Block) []AABB {
	switch collisionProperty(block, "axis") {
	case "x":
		return append(boxes, unitBox(0, 6.5, 6.5, 16, 9.5, 9.5))
	case "z":
		return append(boxes, unitBox(6.5, 6.5, 0, 9.5, 9.5, 16))
	default:
		return append(boxes, unitBox(6.5, 0, 6.5, 9.5, 16, 9.5))
	}
}

func appendSnowCollisionBoxes(boxes []AABB, block Block) []AABB {
	layers := collisionPropertyInt(block, "layers")

	height := (layers - 1) * 2
	if height <= 0 {
		return boxes
	}

	return append(boxes, unitBox(0, 0, 0, 16, float64(height), 16))
}

func appendPointedDripstoneCollisionBoxes(boxes []AABB, block Block) []AABB {
	switch collisionProperty(block, "thickness") {
	case "tip_merge":
		return append(boxes, unitBox(5, 0, 5, 11, 16, 11))
	case "tip":
		if collisionProperty(block, "vertical_direction") == "down" {
			return append(boxes, unitBox(5, 5, 5, 11, 16, 11))
		}

		return append(boxes, unitBox(5, 0, 5, 11, 11, 11))
	case "frustum":
		return append(boxes, unitBox(4, 0, 4, 12, 16, 12))
	case "middle":
		return append(boxes, unitBox(3, 0, 3, 13, 16, 13))
	case "base":
		return append(boxes, unitBox(2, 0, 2, 14, 16, 14))
	default:
		return boxes
	}
}

func appendSlabCollisionBoxes(boxes []AABB, block Block) []AABB {
	switch collisionProperty(block, "type") {
	case "top":
		return append(boxes, unitBox(0, 8, 0, 16, 16, 16))
	case "double":
		return append(boxes, unitBox(0, 0, 0, 16, 16, 16))
	default:
		return append(boxes, unitBox(0, 0, 0, 16, 8, 16))
	}
}

func appendStairCollisionBoxes(boxes []AABB, block Block) []AABB {
	baseMinY := 0.0
	baseMaxY := 8.0
	stepMinY := 8.0
	stepMaxY := 16.0

	if collisionProperty(block, "half") == "top" {
		baseMinY = 8
		baseMaxY = 16
		stepMinY = 0
		stepMaxY = 8
	}

	boxes = append(boxes, unitBox(0, baseMinY, 0, 16, baseMaxY, 16))

	facing := collisionProperty(block, "facing")
	shape := collisionProperty(block, "shape")

	if shape == "outer_left" || shape == "outer_right" {
		minX, minZ, maxX, maxZ := stairQuarter(facing, shape == "outer_left")

		return append(boxes, unitBox(minX, stepMinY, minZ, maxX, stepMaxY, maxZ))
	}

	minX, minZ, maxX, maxZ := stairHalf(facing)

	boxes = append(boxes, unitBox(minX, stepMinY, minZ, maxX, stepMaxY, maxZ))

	if shape == "inner_left" || shape == "inner_right" {
		left := shape == "inner_left"
		minX, minZ, maxX, maxZ = stairSideHalf(facing, left)

		boxes = append(boxes, unitBox(minX, stepMinY, minZ, maxX, stepMaxY, maxZ))
	}

	return boxes
}

func appendDoorCollisionBoxes(boxes []AABB, block Block) []AABB {
	facing := collisionProperty(block, "facing")

	if collisionProperty(block, "open") == "true" {
		hingeLeft := collisionProperty(block, "hinge") == "left"

		facing = rotateCollisionFacing(facing, hingeLeft)
	}

	return append(boxes, thinVerticalCollisionBox(facing, 3))
}

func appendTrapdoorCollisionBoxes(boxes []AABB, block Block) []AABB {
	if collisionProperty(block, "open") == "true" {
		return append(boxes, thinVerticalCollisionBox(collisionProperty(block, "facing"), 3))
	}

	if collisionProperty(block, "half") == "top" {
		return append(boxes, unitBox(0, 13, 0, 16, 16, 16))
	}

	return append(boxes, unitBox(0, 0, 0, 16, 3, 16))
}

func appendFenceGateCollisionBoxes(boxes []AABB, block Block) []AABB {
	if collisionProperty(block, "open") == "true" {
		return boxes
	}

	if collisionProperty(block, "facing") == "north" || collisionProperty(block, "facing") == "south" {
		return append(boxes, unitBox(0, 0, 6, 16, 24, 10))
	}

	return append(boxes, unitBox(6, 0, 0, 10, 24, 16))
}

func appendConnectedCollisionBoxes(boxes []AABB, block Block, centerMin, centerMax, height float64) []AABB {
	boxes = append(boxes, unitBox(centerMin, 0, centerMin, centerMax, height, centerMax))

	if collisionProperty(block, "north") == "true" {
		boxes = append(boxes, unitBox(centerMin, 0, 0, centerMax, height, centerMin))
	}

	if collisionProperty(block, "south") == "true" {
		boxes = append(boxes, unitBox(centerMin, 0, centerMax, centerMax, height, 16))
	}

	if collisionProperty(block, "west") == "true" {
		boxes = append(boxes, unitBox(0, 0, centerMin, centerMin, height, centerMax))
	}

	if collisionProperty(block, "east") == "true" {
		boxes = append(boxes, unitBox(centerMax, 0, centerMin, 16, height, centerMax))
	}

	return boxes
}

func appendWallCollisionBoxes(boxes []AABB, block Block) []AABB {

	armMin := 8.0
	armMax := 8.0

	if collisionProperty(block, "up") == "true" {
		boxes = append(boxes, unitBox(4, 0, 4, 12, 24, 12))

		armMin = 4
		armMax = 12
	}

	if collisionProperty(block, "north") != "none" {
		boxes = append(boxes, unitBox(5, 0, 0, 11, 24, armMin))
	}

	if collisionProperty(block, "south") != "none" {
		boxes = append(boxes, unitBox(5, 0, armMax, 11, 24, 16))
	}

	if collisionProperty(block, "west") != "none" {
		boxes = append(boxes, unitBox(0, 0, 5, armMin, 24, 11))
	}

	if collisionProperty(block, "east") != "none" {
		boxes = append(boxes, unitBox(armMax, 0, 5, 16, 24, 11))
	}

	return boxes
}

func thinVerticalCollisionBox(facing string, thickness float64) AABB {
	switch facing {
	case "south":
		return unitBox(0, 0, 0, 16, 16, thickness)
	case "west":
		return unitBox(16-thickness, 0, 0, 16, 16, 16)
	case "east":
		return unitBox(0, 0, 0, thickness, 16, 16)
	default:
		return unitBox(0, 0, 16-thickness, 16, 16, 16)
	}
}

func rotateCollisionFacing(facing string, left bool) string {
	if left {
		return map[string]string{"north": "west", "west": "south", "south": "east", "east": "north"}[facing]
	}

	return map[string]string{"north": "east", "east": "south", "south": "west", "west": "north"}[facing]
}

func stairHalf(facing string) (minX, minZ, maxX, maxZ float64) {
	switch facing {
	case "south":
		return 0, 8, 16, 16
	case "west":
		return 0, 0, 8, 16
	case "east":
		return 8, 0, 16, 16
	default:
		return 0, 0, 16, 8
	}
}

func stairSideHalf(facing string, left bool) (minX, minZ, maxX, maxZ float64) {
	if left {
		return stairHalf(rotateCollisionFacing(facing, true))
	}

	return stairHalf(rotateCollisionFacing(facing, false))
}

func stairQuarter(facing string, left bool) (minX, minZ, maxX, maxZ float64) {
	forwardMinX, forwardMinZ, forwardMaxX, forwardMaxZ := stairHalf(facing)
	sideMinX, sideMinZ, sideMaxX, sideMaxZ := stairSideHalf(facing, left)

	return maxFloat(forwardMinX, sideMinX), maxFloat(forwardMinZ, sideMinZ), minFloat(forwardMaxX, sideMaxX), minFloat(forwardMaxZ, sideMaxZ)
}

func collisionProperty(block Block, name string) string {
	value, _ := block.Property(name)
	return value
}

func collisionPropertyInt(block Block, name string) int {
	value, err := strconv.Atoi(collisionProperty(block, name))
	if err != nil {
		return 0
	}

	return value
}

func unitBox(minX, minY, minZ, maxX, maxY, maxZ float64) AABB {
	return AABB{
		MinX: minX * blockUnit,
		MinY: minY * blockUnit,
		MinZ: minZ * blockUnit,
		MaxX: maxX * blockUnit,
		MaxY: maxY * blockUnit,
		MaxZ: maxZ * blockUnit,
	}
}

func minFloat(first, second float64) float64 {
	if first < second {
		return first
	}

	return second
}

func maxFloat(first, second float64) float64 {
	if first > second {
		return first
	}

	return second
}
