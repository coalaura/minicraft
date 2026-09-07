package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	animalTemptRangeSquared       = 100
	animalLookRangeSquared        = 36
	animalStrollChance            = 120
	animalLookMinimumTicks        = 40
	animalLookRandomTicks         = 40
	animalRandomLookTicks         = 20
	animalPanicHorizontalRange    = 5
	animalPanicVerticalRange      = 4
	animalPanicPositionAttempts   = 10
	animalPanicWaterVerticalRange = 1
)

type animalPanicGoal struct {
	Entity *runtimeAnimal
	Speed  float64
	Goal   game.Position
}

type animalTemptGoal struct {
	Entity *runtimeAnimal
	Speed  float64
	Target *Session
}

type animalStrollGoal struct {
	Entity *runtimeAnimal
	Speed  float64
	Path   []game.Position
}

type animalLookAtPlayerGoal struct {
	Entity    *runtimeAnimal
	Target    *Session
	LookTicks int32
}

type animalRandomLookGoal struct {
	Entity    *runtimeAnimal
	LookTicks int32
	Direction game.Position
}

func (goal *animalPanicGoal) CanUse(runtime *Runtime) bool {
	goal.Entity.State.mu.Lock()
	damageType, damaged := goal.Entity.Living.LastDamageTypeAt(runtime.World.Time().Age)

	position := goal.Entity.State.Position
	burning := goal.Entity.Living.RemainingFireTicks > 0
	goal.Entity.State.mu.Unlock()

	if !damaged || !damageType.Traits().PanicCauses {
		return false
	}

	if burning {
		water, found := animalPanicWaterPosition(runtime, goal.Entity, position)
		if found {
			goal.Goal = water

			return true
		}
	}

	destination, found := animalPanicRandomPosition(runtime, goal.Entity, position)
	if !found {
		return false
	}

	goal.Goal = destination

	return true
}

func (goal *animalPanicGoal) CanContinue(*Runtime) bool {
	goal.Entity.State.mu.RLock()
	defer goal.Entity.State.mu.RUnlock()

	return !goal.Entity.Navigation.Done()
}

func (goal *animalPanicGoal) Start(runtime *Runtime) {
	goal.Entity.State.mu.Lock()
	position := goal.Entity.State.Position
	goal.Entity.State.mu.Unlock()

	path := runtime.findGroundPath(position, goal.Goal, goal.Entity.Living.Width, goal.Entity.Living.Height, animalFollowRange)

	goal.Entity.State.mu.Lock()
	goal.Entity.Navigation.MoveTo(path, goal.Speed)
	goal.Entity.State.mu.Unlock()
}

func (goal *animalPanicGoal) Stop(*Runtime) {}

func (goal *animalPanicGoal) Tick(*Runtime) {}

func (goal *animalTemptGoal) CanUse(runtime *Runtime) bool {
	goal.Target = nearestTemptingPlayer(runtime, goal.Entity)

	return goal.Target != nil
}

func (goal *animalTemptGoal) CanContinue(runtime *Runtime) bool {
	target := nearestTemptingPlayer(runtime, goal.Entity)
	goal.Target = target

	return target != nil
}

func (goal *animalTemptGoal) Start(*Runtime) {}

func (goal *animalTemptGoal) Stop(*Runtime) {
	goal.Entity.State.mu.Lock()
	goal.Entity.Navigation.Stop()
	goal.Entity.State.mu.Unlock()
	goal.Target = nil
}

func (goal *animalTemptGoal) Tick(runtime *Runtime) {
	if goal.Target == nil {
		return
	}

	player := goal.Target.snapshotPlayer()

	goal.Entity.State.mu.Lock()
	position := goal.Entity.State.Position

	goal.Entity.LookControl.SetWanted(player.EyePosition(), animalMoveMaximumTurn, animalIdleLookMaximumPitch)
	goal.Entity.State.mu.Unlock()

	path := runtime.findGroundPath(position, player.Position, goal.Entity.Living.Width, goal.Entity.Living.Height, animalFollowRange)

	goal.Entity.State.mu.Lock()
	goal.Entity.Navigation.MoveTo(path, goal.Speed)
	goal.Entity.State.mu.Unlock()
}

func (goal *animalStrollGoal) CanUse(runtime *Runtime) bool {
	if runtimeMobRandomInt(runtime, animalStrollChance) != 0 {
		return false
	}

	goal.Entity.State.mu.Lock()
	goal.Path = animalRandomStrollPath(runtime, goal.Entity)
	goal.Entity.State.mu.Unlock()

	return len(goal.Path) > 0
}

func (goal *animalStrollGoal) CanContinue(*Runtime) bool {
	goal.Entity.State.mu.RLock()
	defer goal.Entity.State.mu.RUnlock()

	return !goal.Entity.Navigation.Done()
}

func (goal *animalStrollGoal) Start(*Runtime) {
	goal.Entity.State.mu.Lock()
	goal.Entity.Navigation.MoveTo(goal.Path, goal.Speed)
	goal.Entity.State.mu.Unlock()
}

func (goal *animalStrollGoal) Stop(*Runtime) {
	goal.Entity.State.mu.Lock()
	goal.Entity.Navigation.Stop()
	goal.Entity.State.mu.Unlock()
}

func (goal *animalStrollGoal) Tick(*Runtime) {}

func (goal *animalLookAtPlayerGoal) CanUse(runtime *Runtime) bool {
	goal.Target = nearestAnimalPlayer(runtime, goal.Entity, animalLookRangeSquared, false)

	return goal.Target != nil
}

func (goal *animalLookAtPlayerGoal) CanContinue(*Runtime) bool {
	if goal.Target == nil || goal.LookTicks <= 0 {
		return false
	}

	player := goal.Target.snapshotPlayer()

	goal.Entity.State.mu.RLock()
	distanceSquared := animalDistanceSquared(goal.Entity.State.Position, player.Position)
	goal.Entity.State.mu.RUnlock()

	return !player.Dead && distanceSquared <= animalLookRangeSquared
}

func (goal *animalLookAtPlayerGoal) Start(runtime *Runtime) {
	goal.LookTicks = animalLookMinimumTicks + int32(runtimeMobRandomInt(runtime, animalLookRandomTicks))
}

func (goal *animalLookAtPlayerGoal) Stop(*Runtime) {
	goal.Target = nil
}

func (goal *animalLookAtPlayerGoal) Tick(*Runtime) {
	if goal.Target == nil {
		return
	}

	player := goal.Target.snapshotPlayer()

	goal.Entity.State.mu.Lock()
	goal.Entity.LookControl.SetWanted(player.EyePosition(), animalMoveMaximumTurn, animalIdleLookMaximumPitch)
	goal.Entity.State.mu.Unlock()

	goal.LookTicks--
}

func (goal *animalRandomLookGoal) CanUse(runtime *Runtime) bool {
	return runtimeMobRandomInt(runtime, 50) == 0
}

func (goal *animalRandomLookGoal) CanContinue(*Runtime) bool {
	return goal.LookTicks > 0
}

func (goal *animalRandomLookGoal) Start(runtime *Runtime) {
	directionX, directionZ := animalLookDirection(runtime)

	goal.Direction = game.Position{X: directionX, Z: directionZ}
	goal.LookTicks = animalRandomLookTicks + int32(runtimeMobRandomInt(runtime, animalRandomLookTicks))
}

func (goal *animalRandomLookGoal) Stop(*Runtime) {}

func (goal *animalRandomLookGoal) Tick(*Runtime) {
	goal.Entity.State.mu.Lock()
	position := animalEyePosition(goal.Entity)

	position.X += goal.Direction.X
	position.Z += goal.Direction.Z

	goal.Entity.LookControl.SetWanted(position, animalMoveMaximumTurn, animalIdleLookMaximumPitch)
	goal.Entity.State.mu.Unlock()

	goal.LookTicks--
}

func (*animalRandomLookGoal) RequiresUpdateEveryTick() bool {
	return true
}

func (runtime *Runtime) configureAnimalGoals(entity *runtimeAnimal) {
	entity.Goals.Add(0, runtimeGoalJump, &groundMobFloatGoal{Entity: entity, EyeHeight: entity.Spec.EyeHeight, MoveControl: &entity.MoveControl})
	entity.Goals.Add(1, runtimeGoalMove, &animalPanicGoal{Entity: entity, Speed: entity.Spec.PanicSpeed})
	entity.Goals.Add(3, runtimeGoalMove|runtimeGoalLook, &animalTemptGoal{Entity: entity, Speed: animalTemptSpeed(entity.Spec.EntityType)})
	entity.Goals.Add(animalStrollPriority(entity.Spec.EntityType), runtimeGoalMove, &animalStrollGoal{Entity: entity, Speed: 1})
	entity.Goals.Add(animalLookPriority(entity.Spec.EntityType), runtimeGoalLook, &animalLookAtPlayerGoal{Entity: entity})
	entity.Goals.Add(animalLookPriority(entity.Spec.EntityType)+1, runtimeGoalMove|runtimeGoalLook, &animalRandomLookGoal{Entity: entity})
}

func animalPanicRandomPosition(runtime *Runtime, entity *runtimeAnimal, position game.Position) (game.Position, bool) {
	for range animalPanicPositionAttempts {
		offsetX := runtimeMobRandomInt(runtime, animalPanicHorizontalRange*2+1) - animalPanicHorizontalRange
		offsetY := runtimeMobRandomInt(runtime, animalPanicVerticalRange*2+1) - animalPanicVerticalRange
		offsetZ := runtimeMobRandomInt(runtime, animalPanicHorizontalRange*2+1) - animalPanicHorizontalRange

		candidate := game.Position{X: position.X + float64(offsetX), Y: position.Y + float64(offsetY), Z: position.Z + float64(offsetZ)}

		node, valid := runtime.closestGroundNode(candidate, entity.Living.Width, entity.Living.Height)
		if !valid {
			continue
		}

		return game.Position{X: float64(node.X) + 0.5, Y: float64(node.Y), Z: float64(node.Z) + 0.5}, true
	}

	return game.Position{}, false
}

func animalPanicWaterPosition(runtime *Runtime, entity *runtimeAnimal, position game.Position) (game.Position, bool) {
	origin := game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
	block := runtime.World.BlockAt(origin)

	if len(block.CollisionBoxes(origin)) != 0 {
		return game.Position{}, false
	}

	maximumDepth := animalPanicHorizontalRange*2 + animalPanicWaterVerticalRange

	for depth := 0; depth <= maximumDepth; depth++ {
		maximumX := min(animalPanicHorizontalRange, depth)

		for offsetX := -maximumX; offsetX <= maximumX; offsetX++ {
			maximumY := min(animalPanicWaterVerticalRange, depth-animalIntegerAbs(offsetX))

			for offsetY := -maximumY; offsetY <= maximumY; offsetY++ {
				offsetZ := depth - animalIntegerAbs(offsetX) - animalIntegerAbs(offsetY)
				if offsetZ > animalPanicHorizontalRange {
					continue
				}

				candidate := game.BlockPosition{X: origin.X + int32(offsetX), Y: origin.Y + int32(offsetY), Z: origin.Z + int32(offsetZ)}
				if runtime.World.FluidAt(candidate).Type() == game.FluidTypeWater {
					return game.Position{X: float64(candidate.X), Y: float64(candidate.Y), Z: float64(candidate.Z)}, true
				}

				if offsetZ == 0 {
					continue
				}

				candidate.Z = origin.Z - int32(offsetZ)

				if runtime.World.FluidAt(candidate).Type() == game.FluidTypeWater {
					return game.Position{X: float64(candidate.X), Y: float64(candidate.Y), Z: float64(candidate.Z)}, true
				}
			}
		}
	}

	return game.Position{}, false
}

func nearestTemptingPlayer(runtime *Runtime, entity *runtimeAnimal) *Session {
	return nearestAnimalPlayer(runtime, entity, animalTemptRangeSquared, true)
}

func nearestAnimalPlayer(runtime *Runtime, entity *runtimeAnimal, maximumDistanceSquared float64, requireFood bool) *Session {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	var nearest *Session

	nearestDistanceSquared := maximumDistanceSquared

	for _, session := range runtime.snapshotSessions() {
		player := session.snapshotPlayer()
		if player.Dead || player.GameMode == game.GameModeSpectator {
			continue
		}

		if requireFood && !playerTemptsAnimal(player, entity.Spec.TemptItems) {
			continue
		}

		distanceSquared := animalDistanceSquared(position, player.Position)
		if distanceSquared > nearestDistanceSquared {
			continue
		}

		nearest = session
		nearestDistanceSquared = distanceSquared
	}

	return nearest
}

func playerTemptsAnimal(player game.Player, foods map[game.Item]struct{}) bool {
	mainHand := player.Inventory.Held(player.SelectedHotbarSlot)
	if mainHand != nil {
		_, tempting := foods[mainHand.Item]
		if tempting {
			return true
		}
	}

	_, tempting := foods[player.Inventory.Offhand.Item]

	return tempting
}

func animalTemptSpeed(entityType game.EntityType) float64 {
	switch entityType {
	case game.EntityCow:
		return 1.25
	case game.EntitySheep:
		return 1.1
	default:
		return 1
	}
}

func animalStrollPriority(entityType game.EntityType) int {
	if entityType == game.EntitySheep {
		return 6
	}

	return 5
}

func animalLookPriority(entityType game.EntityType) int {
	if entityType == game.EntitySheep {
		return 7
	}

	return 6
}

func animalDistanceSquared(first, second game.Position) float64 {
	deltaX := first.X - second.X
	deltaY := first.Y - second.Y
	deltaZ := first.Z - second.Z

	return math.FMA(deltaX, deltaX, math.FMA(deltaY, deltaY, deltaZ*deltaZ))
}

func animalIntegerAbs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}
