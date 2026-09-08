package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	groundNavigationMaxNodes = 2048
	groundNavigationMaxDrop  = 3
	groundSupportProbe       = 0.05
)

var groundNavigationDirections = [...]game.BlockPosition{
	{X: 1},
	{X: -1},
	{Z: 1},
	{Z: -1},
	{X: 1, Z: 1},
	{X: 1, Z: -1},
	{X: -1, Z: 1},
	{X: -1, Z: -1},
}

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
}

type groundPathRecord struct {
	Cost float64
	From game.BlockPosition
	Set  bool
}

type groundPathWorkspace struct {
	records map[game.BlockPosition]groundPathRecord
	queue   []groundPathNode
	nodes   []game.BlockPosition
}

func (r *Runtime) moveGroundEntity(position game.Position, velocity game.Velocity, width, height, stepHeight float64, wasOnGround bool) groundMovement {
	box := entityBox(position, width, height)

	var blockBuffer [64]game.AABB

	blocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, velocity)

	delta := collideAABBWithBlocks(box, blocks, velocity)

	horizontalCollisionX := delta.X != velocity.X
	horizontalCollisionZ := delta.Z != velocity.Z
	verticalCollision := delta.Y != velocity.Y

	if stepHeight > 0 && wasOnGround && (horizontalCollisionX || horizontalCollisionZ) {
		stepVelocity := game.Velocity{X: velocity.X, Y: stepHeight, Z: velocity.Z}

		stepBlocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, stepVelocity)

		stepDelta := collideAABBWithBlocks(box, stepBlocks, stepVelocity)

		steppedBox := box.Translate(stepDelta.X, stepDelta.Y, stepDelta.Z)

		drop := game.Velocity{Y: -stepDelta.Y}

		dropBlocks := r.appendEntityCollisionBoxes(blockBuffer[:0], steppedBox, drop)

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
	return r.findGroundPathInto(nil, start, goal, width, height, maximumRange)
}

func (r *Runtime) findGroundPathInto(destination []game.Position, start, goal game.Position, width, height, maximumRange float64) []game.Position {
	if r.groundPathfindStarted != nil {
		r.groundPathfindStarted()
	}

	startNode, valid := r.closestGroundNode(start, width, height)
	if !valid {
		return nil
	}

	goalNode, valid := r.closestGroundNode(goal, width, height)
	if !valid {
		return nil
	}

	workspace := &r.groundPathWorkspace
	if workspace.records == nil {
		workspace.records = make(map[game.BlockPosition]groundPathRecord, groundNavigationMaxNodes)
	} else {
		clear(workspace.records)
	}

	workspace.queue = workspace.queue[:0]
	workspace.nodes = workspace.nodes[:0]
	workspace.records[startNode] = groundPathRecord{Cost: 0, Set: true}

	workspace.queue = pushGroundPathNode(workspace.queue, groundPathNode{Position: startNode, Estimate: groundNodeDistance(startNode, goalNode)})

	expanded := 0

	for len(workspace.queue) > 0 && expanded < groundNavigationMaxNodes {
		var current groundPathNode

		workspace.queue, current = popGroundPathNode(workspace.queue)

		record := workspace.records[current.Position]

		if current.Cost != record.Cost {
			continue
		}

		expanded++

		if current.Position == goalNode {
			return reconstructGroundPath(destination, workspace, startNode, goalNode)
		}

		for _, direction := range groundNavigationDirections {
			candidate, walkable := r.groundNeighbor(current.Position, direction, width, height)
			if !walkable || groundNodeDistance(startNode, candidate) > maximumRange {
				continue
			}

			cost := record.Cost + groundNodeDistance(current.Position, candidate)
			known := workspace.records[candidate]

			if known.Set && cost >= known.Cost {
				continue
			}

			workspace.records[candidate] = groundPathRecord{Cost: cost, From: current.Position, Set: true}

			estimate := cost + groundNodeDistance(candidate, goalNode)

			workspace.queue = pushGroundPathNode(workspace.queue, groundPathNode{Position: candidate, Cost: cost, Estimate: estimate})
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
	if direction.X != 0 && direction.Z != 0 {
		firstSide, firstWalkable := r.groundNeighbor(current, game.BlockPosition{X: direction.X}, width, height)
		secondSide, secondWalkable := r.groundNeighbor(current, game.BlockPosition{Z: direction.Z}, width, height)

		if !firstWalkable || !secondWalkable || firstSide.Y > current.Y || secondSide.Y > current.Y {
			return game.BlockPosition{}, false
		}
	}

	candidate := game.BlockPosition{X: current.X + direction.X, Y: current.Y, Z: current.Z + direction.Z}

	for offset := int32(1); offset >= -groundNavigationMaxDrop; offset-- {
		candidate.Y = current.Y + offset

		if r.groundNodeWalkable(candidate, width, height) && r.groundTransitionWalkable(current, candidate, width, height) {
			return candidate, true
		}
	}

	return game.BlockPosition{}, false
}

func (r *Runtime) groundTransitionWalkable(current, candidate game.BlockPosition, width, height float64) bool {
	from := game.Position{X: float64(current.X) + 0.5, Y: float64(current.Y), Z: float64(current.Z) + 0.5}
	to := game.Position{X: float64(candidate.X) + 0.5, Y: float64(candidate.Y), Z: float64(candidate.Z) + 0.5}

	if candidate.Y > current.Y {
		from.Y = to.Y
	}

	velocity := game.Velocity{X: to.X - from.X, Z: to.Z - from.Z}

	box := entityBox(from, width, height)

	var blockBuffer [64]game.AABB

	blocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, velocity)

	delta := collideAABBWithBlocks(box, blocks, velocity)

	return math.Abs(delta.X-velocity.X) < 1e-7 && math.Abs(delta.Z-velocity.Z) < 1e-7
}

func (r *Runtime) groundNodeWalkable(node game.BlockPosition, width, height float64) bool {
	position := game.Position{X: float64(node.X) + 0.5, Y: float64(node.Y), Z: float64(node.Z) + 0.5}

	box := entityBox(position, width, height)

	var blockBuffer [64]game.AABB

	blocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, game.Velocity{})

	if slices.ContainsFunc(blocks, box.Intersects) {
		return false
	}

	return r.entityHasSupport(box)
}

func (r *Runtime) entityHasSupport(box game.AABB) bool {
	probe := game.Velocity{Y: -groundSupportProbe}

	var blockBuffer [64]game.AABB

	blocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, probe)

	delta := collideAABBWithBlocks(box, blocks, probe)

	return delta.Y != probe.Y
}

func reconstructGroundPath(destination []game.Position, workspace *groundPathWorkspace, start, goal game.BlockPosition) []game.Position {
	workspace.nodes = append(workspace.nodes[:0], goal)

	for current := goal; current != start; {
		record := workspace.records[current]
		current = record.From
		workspace.nodes = append(workspace.nodes, current)
	}

	slices.Reverse(workspace.nodes)

	pathLength := len(workspace.nodes) - 1
	if cap(destination) < pathLength {
		destination = make([]game.Position, 0, pathLength)
	} else {
		destination = destination[:0]
	}

	for _, node := range workspace.nodes[1:] {
		destination = append(destination, game.Position{X: float64(node.X) + 0.5, Y: float64(node.Y), Z: float64(node.Z) + 0.5})
	}

	return destination
}

func pushGroundPathNode(queue []groundPathNode, node groundPathNode) []groundPathNode {
	queue = append(queue, node)
	index := len(queue) - 1

	for index > 0 {
		parent := (index - 1) / 2
		if queue[parent].Estimate <= node.Estimate {
			break
		}

		queue[index] = queue[parent]
		index = parent
	}

	queue[index] = node

	return queue
}

func popGroundPathNode(queue []groundPathNode) ([]groundPathNode, groundPathNode) {
	result := queue[0]
	lastIndex := len(queue) - 1
	last := queue[lastIndex]
	queue = queue[:lastIndex]

	if lastIndex == 0 {
		return queue, result
	}

	index := 0

	for {
		left := index*2 + 1
		if left >= len(queue) {
			break
		}

		child := left

		right := left + 1
		if right < len(queue) && queue[right].Estimate < queue[left].Estimate {
			child = right
		}

		if last.Estimate <= queue[child].Estimate {
			break
		}

		queue[index] = queue[child]
		index = child
	}

	queue[index] = last

	return queue, result
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
