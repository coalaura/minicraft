package server

import (
	"container/heap"
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	groundNavigationMaxNodes = 2048
	groundNavigationMaxDrop  = 3
	groundSupportProbe       = 0.05
)

type groundMovement struct {
	Position             game.Position
	OnGround             bool
	HorizontalCollisionX bool
	HorizontalCollisionZ bool
	VerticalCollision    bool
}

type groundPathNode struct {
	Position game.BlockPosition
	Cost     float64
	Estimate float64
	Index    int
}

type groundPathQueue []*groundPathNode

type groundPathRecord struct {
	Cost float64
	From game.BlockPosition
	Set  bool
}

func (queue groundPathQueue) Len() int {
	return len(queue)
}

func (queue groundPathQueue) Less(first, second int) bool {
	return queue[first].Estimate < queue[second].Estimate
}

func (queue groundPathQueue) Swap(first, second int) {
	queue[first], queue[second] = queue[second], queue[first]
	queue[first].Index = first
	queue[second].Index = second
}

func (queue *groundPathQueue) Push(value any) {
	node := value.(*groundPathNode)
	node.Index = len(*queue)
	*queue = append(*queue, node)
}

func (queue *groundPathQueue) Pop() any {
	old := *queue
	last := len(old) - 1
	node := old[last]
	old[last] = nil
	node.Index = -1
	*queue = old[:last]

	return node
}

func (r *Runtime) moveGroundEntity(position game.Position, velocity game.Velocity, width, height, stepHeight float64, wasOnGround bool) groundMovement {
	box := entityBox(position, width, height)

	blocks := r.entityCollisionBoxes(box, velocity)

	delta := collideAABBWithBlocks(box, blocks, velocity)

	horizontalCollisionX := delta.X != velocity.X
	horizontalCollisionZ := delta.Z != velocity.Z
	verticalCollision := delta.Y != velocity.Y

	if stepHeight > 0 && wasOnGround && (horizontalCollisionX || horizontalCollisionZ) {
		stepVelocity := game.Velocity{X: velocity.X, Y: stepHeight, Z: velocity.Z}

		stepBlocks := r.entityCollisionBoxes(box, stepVelocity)

		stepDelta := collideAABBWithBlocks(box, stepBlocks, stepVelocity)

		steppedBox := box.Translate(stepDelta.X, stepDelta.Y, stepDelta.Z)

		drop := game.Velocity{Y: -stepDelta.Y}

		dropBlocks := r.entityCollisionBoxes(steppedBox, drop)

		dropDelta := collideAABBWithBlocks(steppedBox, dropBlocks, drop)

		stepDelta.Y += dropDelta.Y

		directDistance := delta.X*delta.X + delta.Z*delta.Z
		stepDistance := stepDelta.X*stepDelta.X + stepDelta.Z*stepDelta.Z

		if stepDistance > directDistance {
			delta = stepDelta
			horizontalCollisionX = delta.X != velocity.X
			horizontalCollisionZ = delta.Z != velocity.Z
			verticalCollision = true
		}
	}

	position.X += delta.X
	position.Y += delta.Y
	position.Z += delta.Z

	onGround := velocity.Y < 0 && verticalCollision
	if !onGround && velocity.Y == 0 {
		onGround = r.entityHasSupport(entityBox(position, width, height))
	}

	return groundMovement{
		Position:             position,
		OnGround:             onGround,
		HorizontalCollisionX: horizontalCollisionX,
		HorizontalCollisionZ: horizontalCollisionZ,
		VerticalCollision:    verticalCollision,
	}
}

func (r *Runtime) findGroundPath(start, goal game.Position, width, height, maximumRange float64) []game.Position {
	startNode, valid := r.closestGroundNode(start, width, height)
	if !valid {
		return nil
	}

	goalNode, valid := r.closestGroundNode(goal, width, height)
	if !valid {
		return nil
	}

	records := map[game.BlockPosition]groundPathRecord{startNode: {Cost: 0, Set: true}}

	queue := groundPathQueue{&groundPathNode{Position: startNode, Estimate: groundNodeDistance(startNode, goalNode)}}

	heap.Init(&queue)

	directions := [...]game.BlockPosition{{X: 1}, {X: -1}, {Z: 1}, {Z: -1}}
	expanded := 0

	for queue.Len() > 0 && expanded < groundNavigationMaxNodes {
		current := heap.Pop(&queue).(*groundPathNode)
		record := records[current.Position]

		if current.Cost != record.Cost {
			continue
		}

		expanded++

		if current.Position == goalNode {
			return reconstructGroundPath(records, startNode, goalNode)
		}

		for _, direction := range directions {
			candidate, walkable := r.groundNeighbor(current.Position, direction, width, height)
			if !walkable || groundNodeDistance(startNode, candidate) > maximumRange {
				continue
			}

			verticalCost := math.Abs(float64(candidate.Y-current.Position.Y)) * 0.5

			cost := record.Cost + 1 + verticalCost
			known := records[candidate]

			if known.Set && cost >= known.Cost {
				continue
			}

			records[candidate] = groundPathRecord{Cost: cost, From: current.Position, Set: true}

			estimate := cost + groundNodeDistance(candidate, goalNode)

			heap.Push(&queue, &groundPathNode{Position: candidate, Cost: cost, Estimate: estimate})
		}
	}

	return nil
}

func (r *Runtime) closestGroundNode(position game.Position, width, height float64) (game.BlockPosition, bool) {
	base := game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}

	for offset := int32(1); offset >= -groundNavigationMaxDrop; offset-- {
		candidate := base
		candidate.Y += offset

		if r.groundNodeWalkable(candidate, width, height) {
			return candidate, true
		}
	}

	return game.BlockPosition{}, false
}

func (r *Runtime) groundNeighbor(current, direction game.BlockPosition, width, height float64) (game.BlockPosition, bool) {
	candidate := game.BlockPosition{X: current.X + direction.X, Y: current.Y, Z: current.Z + direction.Z}

	for offset := int32(1); offset >= -groundNavigationMaxDrop; offset-- {
		candidate.Y = current.Y + offset

		if r.groundNodeWalkable(candidate, width, height) {
			return candidate, true
		}
	}

	return game.BlockPosition{}, false
}

func (r *Runtime) groundNodeWalkable(node game.BlockPosition, width, height float64) bool {
	position := game.Position{X: float64(node.X) + 0.5, Y: float64(node.Y), Z: float64(node.Z) + 0.5}

	box := entityBox(position, width, height)

	if slices.ContainsFunc(r.entityCollisionBoxes(box, game.Velocity{}), box.Intersects) {
		return false
	}

	return r.entityHasSupport(box)
}

func (r *Runtime) entityHasSupport(box game.AABB) bool {
	probe := game.Velocity{Y: -groundSupportProbe}

	blocks := r.entityCollisionBoxes(box, probe)

	delta := collideAABBWithBlocks(box, blocks, probe)

	return delta.Y != probe.Y
}

func reconstructGroundPath(records map[game.BlockPosition]groundPathRecord, start, goal game.BlockPosition) []game.Position {
	nodes := []game.BlockPosition{goal}

	for current := goal; current != start; {
		record := records[current]
		current = record.From
		nodes = append(nodes, current)
	}

	slices.Reverse(nodes)

	path := make([]game.Position, 0, len(nodes))

	for _, node := range nodes {
		path = append(path, game.Position{X: float64(node.X) + 0.5, Y: float64(node.Y), Z: float64(node.Z) + 0.5})
	}

	return path
}

func groundNodeDistance(first, second game.BlockPosition) float64 {
	deltaX := float64(first.X - second.X)
	deltaY := float64(first.Y - second.Y)
	deltaZ := float64(first.Z - second.Z)

	return math.Sqrt(deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ)
}

func entityBox(position game.Position, width, height float64) game.AABB {
	halfWidth := width / 2

	return game.AABB{
		MinX: position.X - halfWidth,
		MinY: position.Y,
		MinZ: position.Z - halfWidth,
		MaxX: position.X + halfWidth,
		MaxY: position.Y + height,
		MaxZ: position.Z + halfWidth,
	}
}
