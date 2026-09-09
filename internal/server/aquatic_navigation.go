package server

import (
	"math"
	"slices"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	swimNavigationMaxNodes      = 2048
	swimNavigationTraceStep     = 0.25
	swimNavigationWaypointScale = 0.5
)

type swimPathNode struct {
	Position game.BlockPosition
	Cost     float64
	Estimate float64
}

type swimPathRecord struct {
	Cost float64
	From game.BlockPosition
	Set  bool
}

type swimPathWorkspace struct {
	records map[game.BlockPosition]swimPathRecord
	queue   []swimPathNode
	nodes   []game.BlockPosition
}

// swimNavigation holds the path state consumed by a three-dimensional MoveControl.
type swimNavigation struct {
	Path      []game.Position
	Index     int
	Speed     float64
	Target    game.Position
	HasTarget bool
}

var swimNavigationCardinalDirections = [...]game.BlockPosition{
	{X: 1},
	{X: -1},
	{Y: 1},
	{Y: -1},
	{Z: 1},
	{Z: -1},
}

var swimNavigationDiagonalDirections = [...]game.BlockPosition{
	{X: 1, Z: 1},
	{X: 1, Z: -1},
	{X: -1, Z: 1},
	{X: -1, Z: -1},
}

func (navigation *swimNavigation) SetPath(path []game.Position, speed float64, target game.Position) {
	navigation.Path = path
	navigation.Index = 0
	navigation.Speed = speed
	navigation.Target = target
	navigation.HasTarget = true
}

func (navigation *swimNavigation) Stop() {
	navigation.Path = navigation.Path[:0]
	navigation.Index = 0
	navigation.Speed = 0
	navigation.HasTarget = false
}

func (navigation *swimNavigation) Done() bool {
	return !navigation.HasTarget || navigation.Index >= len(navigation.Path)
}

// advanceSwimNavigation returns the current waypoint after consuming reached waypoints and safe shortcuts.
func (r *Runtime) advanceSwimNavigation(position game.Position, width, height float64, navigation *swimNavigation) (game.Position, bool) {
	if navigation.Done() {
		return game.Position{}, false
	}

	reachDistance := width * swimNavigationWaypointScale
	reachDistance *= reachDistance

	for navigation.Index < len(navigation.Path) && swimPositionDistanceSquared(position, navigation.Path[navigation.Index]) <= reachDistance {
		navigation.Index++
	}

	if navigation.Index >= len(navigation.Path) {
		return game.Position{}, false
	}

	for index := len(navigation.Path) - 1; index > navigation.Index; index-- {
		if r.swimDirectPathClear(position, navigation.Path[index], width, height) {
			navigation.Index = index

			break
		}
	}

	return navigation.Path[navigation.Index], true
}

func (r *Runtime) findSwimPath(start, goal game.Position, width, height, maximumRange float64) []game.Position {
	return r.findSwimPathInto(nil, start, goal, width, height, maximumRange)
}

// findSwimPathInto appends a path to destination, reusing its backing storage when possible.
func (r *Runtime) findSwimPathInto(destination []game.Position, start, goal game.Position, width, height, maximumRange float64) []game.Position {
	startNode := swimStartNode(start, width, height)
	if !r.swimNodeValid(startNode, width, height) {
		return destination[:0]
	}

	goalNode := swimStartNode(goal, width, height)
	if !r.swimNodeValid(goalNode, width, height) {
		return destination[:0]
	}

	workspace := &r.swimPathWorkspace
	if workspace.records == nil {
		workspace.records = make(map[game.BlockPosition]swimPathRecord, swimNavigationMaxNodes)
	} else {
		clear(workspace.records)
	}

	workspace.queue = workspace.queue[:0]
	workspace.nodes = workspace.nodes[:0]
	workspace.records[startNode] = swimPathRecord{Set: true}

	workspace.queue = pushSwimPathNode(workspace.queue, swimPathNode{Position: startNode, Estimate: swimNodeDistance(startNode, goalNode)})

	expanded := 0

	for len(workspace.queue) > 0 && expanded < swimNavigationMaxNodes {
		var current swimPathNode

		workspace.queue, current = popSwimPathNode(workspace.queue)
		record := workspace.records[current.Position]

		if current.Cost != record.Cost {
			continue
		}

		expanded++

		if current.Position == goalNode {
			return reconstructSwimPath(destination, workspace, startNode, goalNode)
		}

		for _, direction := range swimNavigationCardinalDirections {
			r.addSwimNeighbor(workspace, current.Position, goalNode, startNode, direction, width, height, maximumRange)
		}

		for _, direction := range swimNavigationDiagonalDirections {
			first := game.BlockPosition{X: current.Position.X + direction.X, Y: current.Position.Y, Z: current.Position.Z}
			second := game.BlockPosition{X: current.Position.X, Y: current.Position.Y, Z: current.Position.Z + direction.Z}

			if !r.swimNodeValid(first, width, height) || !r.swimNodeValid(second, width, height) {
				continue
			}

			r.addSwimNeighbor(workspace, current.Position, goalNode, startNode, direction, width, height, maximumRange)
		}
	}

	return destination[:0]
}

func (r *Runtime) addSwimNeighbor(workspace *swimPathWorkspace, current, goal, start, direction game.BlockPosition, width, height, maximumRange float64) {
	candidate := game.BlockPosition{X: current.X + direction.X, Y: current.Y + direction.Y, Z: current.Z + direction.Z}
	if !r.swimNodeValid(candidate, width, height) || swimNodeDistance(start, candidate) > maximumRange {
		return
	}

	record := workspace.records[current]

	cost := record.Cost + swimNodeDistance(current, candidate)
	known := workspace.records[candidate]

	if known.Set && cost >= known.Cost {
		return
	}

	workspace.records[candidate] = swimPathRecord{Cost: cost, From: current, Set: true}

	estimate := cost + swimNodeDistance(candidate, goal)
	workspace.queue = pushSwimPathNode(workspace.queue, swimPathNode{Position: candidate, Cost: cost, Estimate: estimate})
}

func (r *Runtime) swimNodeValid(node game.BlockPosition, width, height float64) bool {
	position := swimNodePosition(node)

	box := entityBox(position, width, height)

	if !r.swimBoxInWater(box) {
		return false
	}

	var blockBuffer [64]game.AABB

	blocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, game.Velocity{})

	return !slices.ContainsFunc(blocks, box.Intersects)
}

func (r *Runtime) swimDirectPathClear(from, to game.Position, width, height float64) bool {
	delta := game.Velocity{X: to.X - from.X, Y: to.Y - from.Y, Z: to.Z - from.Z}
	box := entityBox(from, width, height)

	var blockBuffer [64]game.AABB

	blocks := r.appendEntityCollisionBoxes(blockBuffer[:0], box, delta)
	actual := collideAABBWithBlocks(box, blocks, delta)

	if actual != delta {
		return false
	}

	distance := math.Sqrt(delta.X*delta.X + delta.Y*delta.Y + delta.Z*delta.Z)
	steps := int(math.Ceil(distance / swimNavigationTraceStep))

	if steps == 0 {
		return r.swimBoxInWater(box)
	}

	for step := 0; step <= steps; step++ {
		fraction := float64(step) / float64(steps)
		probe := box.Translate(delta.X*fraction, delta.Y*fraction, delta.Z*fraction)

		if !r.swimBoxInWater(probe) {
			return false
		}
	}

	return true
}

func (r *Runtime) swimBoxInWater(box game.AABB) bool {
	minimumX := int32(math.Floor(box.MinX))
	minimumY := int32(math.Floor(box.MinY))
	minimumZ := int32(math.Floor(box.MinZ))
	maximumX := int32(math.Ceil(box.MaxX)) - 1
	maximumY := int32(math.Ceil(box.MaxY)) - 1
	maximumZ := int32(math.Ceil(box.MaxZ)) - 1

	for y := minimumY; y <= maximumY; y++ {
		for x := minimumX; x <= maximumX; x++ {
			for z := minimumZ; z <= maximumZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}
				if r.World.FluidAt(position).Type() != game.FluidTypeWater {
					return false
				}
			}
		}
	}

	return true
}

func reconstructSwimPath(destination []game.Position, workspace *swimPathWorkspace, start, goal game.BlockPosition) []game.Position {
	workspace.nodes = append(workspace.nodes[:0], goal)

	for current := goal; current != start; {
		record := workspace.records[current]
		current = record.From
		workspace.nodes = append(workspace.nodes, current)
	}

	slices.Reverse(workspace.nodes)
	destination = destination[:0]

	for _, node := range workspace.nodes[1:] {
		destination = append(destination, swimNodePosition(node))
	}

	return destination
}

func pushSwimPathNode(queue []swimPathNode, node swimPathNode) []swimPathNode {
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

func popSwimPathNode(queue []swimPathNode) ([]swimPathNode, swimPathNode) {
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

func swimStartNode(position game.Position, width, height float64) game.BlockPosition {
	box := entityBox(position, width, height)

	return game.BlockPosition{
		X: int32(math.Floor(box.MinX)),
		Y: int32(math.Floor(box.MinY + swimNavigationWaypointScale)),
		Z: int32(math.Floor(box.MinZ)),
	}
}

func swimNodePosition(node game.BlockPosition) game.Position {
	return game.Position{X: float64(node.X) + swimNavigationWaypointScale, Y: float64(node.Y), Z: float64(node.Z) + swimNavigationWaypointScale}
}

func swimNodeDistance(first, second game.BlockPosition) float64 {
	deltaX := float64(first.X - second.X)
	deltaY := float64(first.Y - second.Y)
	deltaZ := float64(first.Z - second.Z)

	return math.Sqrt(deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ)
}

func swimPositionDistanceSquared(first, second game.Position) float64 {
	deltaX := first.X - second.X
	deltaY := first.Y - second.Y
	deltaZ := first.Z - second.Z

	return deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ
}
