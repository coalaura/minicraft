package game

const (
	playerWidth           = 0.6
	standingPlayerHeight  = 1.8
	crouchingPlayerHeight = 1.5
	crawlingPlayerHeight  = 0.6
	blockUnit             = 1.0 / 16.0
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
	definition, valid := block.Definition()
	if !valid {
		return nil
	}

	var boxes []AABB

	switch definition.Collision {
	case BlockCollisionFull:
		boxes = []AABB{unitBox(0, 0, 0, 16, 16, 16)}
	case BlockCollisionSlab:
		boxes = slabCollisionBoxes(block)
	case BlockCollisionStairs:
		boxes = stairCollisionBoxes(block)
	case BlockCollisionDoor:
		boxes = doorCollisionBoxes(block)
	case BlockCollisionTrapdoor:
		boxes = trapdoorCollisionBoxes(block)
	case BlockCollisionFenceGate:
		boxes = fenceGateCollisionBoxes(block)
	case BlockCollisionFence:
		boxes = connectedCollisionBoxes(block, 6, 10, 24)
	case BlockCollisionPane:
		boxes = connectedCollisionBoxes(block, 7, 9, 16)
	case BlockCollisionWall:
		boxes = wallCollisionBoxes(block)
	}

	for index := range boxes {
		boxes[index] = boxes[index].Translate(float64(position.X), float64(position.Y), float64(position.Z))
	}

	return boxes
}

func slabCollisionBoxes(block Block) []AABB {
	switch collisionProperty(block, "type") {
	case "top":
		return []AABB{unitBox(0, 8, 0, 16, 16, 16)}
	case "double":
		return []AABB{unitBox(0, 0, 0, 16, 16, 16)}
	default:
		return []AABB{unitBox(0, 0, 0, 16, 8, 16)}
	}
}

func stairCollisionBoxes(block Block) []AABB {
	baseMinY, baseMaxY := 0.0, 8.0
	stepMinY, stepMaxY := 8.0, 16.0

	if collisionProperty(block, "half") == "top" {
		baseMinY, baseMaxY = 8, 16
		stepMinY, stepMaxY = 0, 8
	}

	boxes := []AABB{unitBox(0, baseMinY, 0, 16, baseMaxY, 16)}

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

func doorCollisionBoxes(block Block) []AABB {
	facing := collisionProperty(block, "facing")

	if collisionProperty(block, "open") == "true" {
		hingeLeft := collisionProperty(block, "hinge") == "left"

		facing = rotateCollisionFacing(facing, hingeLeft)
	}

	return []AABB{thinVerticalCollisionBox(facing, 3)}
}

func trapdoorCollisionBoxes(block Block) []AABB {
	if collisionProperty(block, "open") == "true" {
		return []AABB{thinVerticalCollisionBox(collisionProperty(block, "facing"), 3)}
	}

	if collisionProperty(block, "half") == "top" {
		return []AABB{unitBox(0, 13, 0, 16, 16, 16)}
	}

	return []AABB{unitBox(0, 0, 0, 16, 3, 16)}
}

func fenceGateCollisionBoxes(block Block) []AABB {
	if collisionProperty(block, "open") == "true" {
		return nil
	}

	if collisionProperty(block, "facing") == "north" || collisionProperty(block, "facing") == "south" {
		return []AABB{unitBox(0, 0, 6, 16, 24, 10)}
	}

	return []AABB{unitBox(6, 0, 0, 10, 24, 16)}
}

func connectedCollisionBoxes(block Block, centerMin, centerMax, height float64) []AABB {
	boxes := []AABB{unitBox(centerMin, 0, centerMin, centerMax, height, centerMax)}

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

func wallCollisionBoxes(block Block) []AABB {
	boxes := make([]AABB, 0, 5)

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
