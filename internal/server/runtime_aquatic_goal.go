package server

import "github.com/coalaura/minicraft/internal/game"

const (
	aquaticAvoidRange             = 8
	aquaticAvoidRangeSquared      = 64
	aquaticAvoidNearSquared       = 49
	aquaticAvoidFarSpeed          = 1.6
	aquaticAvoidNearSpeed         = 1.4
	aquaticRandomSwimChance       = 20
	aquaticRandomHorizontalRange  = 10
	aquaticRandomVerticalRange    = 7
	aquaticRandomPositionAttempts = 10
	aquaticSchoolSearchRange      = 8
	aquaticSchoolContinueSquared  = 121
	aquaticSchoolValidationChance = 200
	aquaticSchoolRepathTicks      = 5
)

type aquaticFish interface {
	RuntimeEntity
	aquaticBase() *runtimeAquatic
}

type aquaticPanicGoal struct {
	Fish aquaticFish
	Path []game.Position
}

type aquaticAvoidPlayerGoal struct {
	Fish   aquaticFish
	Target *Session
	Path   []game.Position
}

type aquaticRandomSwimGoal struct {
	Fish aquaticFish
	Path []game.Position
}

type aquaticFollowSchoolGoal struct {
	Fish          aquaticFish
	NextStartTick int
	RepathTick    int
}

func (entity *runtimeCodEntity) aquaticBase() *runtimeAquatic {
	return &entity.runtimeAquatic
}

func (entity *runtimeSalmonEntity) aquaticBase() *runtimeAquatic {
	return &entity.runtimeAquatic
}

func (entity *runtimeTropicalFishEntity) aquaticBase() *runtimeAquatic {
	return &entity.runtimeAquatic
}

func (goal *aquaticPanicGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.Lock()
	damageType, damaged := entity.Living.LastDamageTypeAt(runtime.World.Time().Age)
	position := entity.State.Position
	entity.State.mu.Unlock()

	if !damaged || !damageType.Traits().PanicCauses {
		return false
	}

	goal.Path = aquaticRandomSwimPath(runtime, entity, goal.Path, position)

	return len(goal.Path) > 0
}

func (goal *aquaticPanicGoal) CanContinue(*Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return !entity.Navigation.Done()
}

func (goal *aquaticPanicGoal) Start(*Runtime) {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.Lock()
	entity.Navigation.SetPath(goal.Path, 1.25, goal.Path[len(goal.Path)-1])
	entity.State.mu.Unlock()
}

func (goal *aquaticPanicGoal) Stop(*Runtime) {
	goal.Fish.aquaticBase().stopNavigation()
}

func (*aquaticPanicGoal) Tick(*Runtime) {}

func (goal *aquaticAvoidPlayerGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Fish.aquaticBase()
	goal.Target = nearestAquaticPlayer(runtime, entity)

	if goal.Target == nil {
		return false
	}

	return goal.selectPath(runtime)
}

func (goal *aquaticAvoidPlayerGoal) CanContinue(*Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return goal.Target != nil && !entity.Navigation.Done()
}

func (goal *aquaticAvoidPlayerGoal) Start(*Runtime) {
	entity := goal.Fish.aquaticBase()
	player := goal.Target.playerView()

	entity.State.mu.Lock()
	distanceSquared := animalDistanceSquared(entity.State.Position, player.Position)

	speed := aquaticAvoidFarSpeed

	if distanceSquared < aquaticAvoidNearSquared {
		speed = aquaticAvoidNearSpeed
	}

	entity.Navigation.SetPath(goal.Path, speed, goal.Path[len(goal.Path)-1])
	entity.State.mu.Unlock()
}

func (goal *aquaticAvoidPlayerGoal) Stop(*Runtime) {
	goal.Fish.aquaticBase().stopNavigation()
	goal.Target = nil
}

func (*aquaticAvoidPlayerGoal) Tick(*Runtime) {}

func (goal *aquaticAvoidPlayerGoal) selectPath(runtime *Runtime) bool {
	entity := goal.Fish.aquaticBase()
	player := goal.Target.playerView()

	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	currentDistance := animalDistanceSquared(position, player.Position)

	for range aquaticRandomPositionAttempts {
		offsetX := runtimeMobRandomInt(runtime, aquaticRandomHorizontalRange*2+1) - aquaticRandomHorizontalRange
		offsetY := runtimeMobRandomInt(runtime, aquaticRandomVerticalRange*2+1) - aquaticRandomVerticalRange
		offsetZ := runtimeMobRandomInt(runtime, aquaticRandomHorizontalRange*2+1) - aquaticRandomHorizontalRange

		candidate := game.Position{X: position.X + float64(offsetX), Y: position.Y + float64(offsetY), Z: position.Z + float64(offsetZ)}

		if animalDistanceSquared(candidate, player.Position) <= currentDistance {
			continue
		}

		goal.Path = runtime.findSwimPathInto(goal.Path, position, candidate, entity.Living.Width, entity.Living.Height, aquaticFollowRange)
		if len(goal.Path) > 0 {
			return true
		}
	}

	return false
}

func (goal *aquaticRandomSwimGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	noActionTime := entity.NoActionTime
	follower := aquaticFollowerLocked(entity)
	position := entity.State.Position
	entity.State.mu.RUnlock()

	if noActionTime >= 100 || follower || runtimeMobRandomInt(runtime, aquaticRandomSwimChance) != 0 {
		return false
	}

	goal.Path = aquaticRandomSwimPath(runtime, entity, goal.Path, position)

	return len(goal.Path) > 0
}

func (goal *aquaticRandomSwimGoal) CanContinue(*Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return !aquaticFollowerLocked(entity) && !entity.Navigation.Done()
}

func (goal *aquaticRandomSwimGoal) Start(*Runtime) {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.Lock()
	entity.Navigation.SetPath(goal.Path, 1, goal.Path[len(goal.Path)-1])
	entity.State.mu.Unlock()
}

func (goal *aquaticRandomSwimGoal) Stop(*Runtime) {
	goal.Fish.aquaticBase().stopNavigation()
}

func (*aquaticRandomSwimGoal) Tick(*Runtime) {}

func (goal *aquaticFollowSchoolGoal) CanUse(runtime *Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	follower := aquaticFollowerLocked(entity)
	hasFollowers := entity.School.SchoolSize > 1
	entity.State.mu.RUnlock()

	if hasFollowers {
		if runtimeMobRandomInt(runtime, aquaticSchoolValidationChance) == 1 {
			runtime.validateAquaticSchool(goal.Fish)
		}

		return false
	}

	if follower {
		return true
	}

	goal.NextStartTick--

	if goal.NextStartTick > 0 {
		return false
	}

	goal.NextStartTick = reducedTickDelay(200 + runtimeMobRandomInt(runtime, 20))

	return runtime.formAquaticSchool(goal.Fish)
}

func (goal *aquaticFollowSchoolGoal) CanContinue(*Runtime) bool {
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	leader := entity.School.Leader
	position := entity.State.Position
	entity.State.mu.RUnlock()

	if leader == nil {
		return false
	}

	leaderState := leader.RuntimeEntityState()
	leaderLiving := leader.(RuntimeLivingEntity).RuntimeLivingState()

	leaderState.mu.RLock()
	alive := !leaderState.Removed && !leaderLiving.Dead
	leaderPosition := leaderState.Position
	leaderState.mu.RUnlock()

	return alive && animalDistanceSquared(position, leaderPosition) <= aquaticSchoolContinueSquared
}

func (goal *aquaticFollowSchoolGoal) Start(*Runtime) {
	goal.RepathTick = 0
}

func (goal *aquaticFollowSchoolGoal) Stop(*Runtime) {
	disconnectAquaticFollower(goal.Fish)
}

func (goal *aquaticFollowSchoolGoal) Tick(runtime *Runtime) {
	goal.RepathTick--

	if goal.RepathTick > 0 {
		return
	}

	goal.RepathTick = aquaticSchoolRepathTicks
	entity := goal.Fish.aquaticBase()

	entity.State.mu.RLock()
	leader := entity.School.Leader
	position := entity.State.Position
	entity.State.mu.RUnlock()

	if leader == nil {
		return
	}

	leaderState := leader.RuntimeEntityState()
	leaderState.mu.RLock()
	goalPosition := leaderState.Position
	leaderState.mu.RUnlock()

	path := runtime.findSwimPathInto(entity.Navigation.Path, position, goalPosition, entity.Living.Width, entity.Living.Height, aquaticFollowRange)

	entity.State.mu.Lock()
	entity.Navigation.SetPath(path, 1, goalPosition)
	entity.State.mu.Unlock()
}

func (entity *runtimeAquatic) stopNavigation() {
	entity.State.mu.Lock()
	entity.Navigation.Stop()
	entity.State.mu.Unlock()
}

func (runtime *Runtime) configureAquaticGoals(concrete aquaticFish, entity *runtimeAquatic) {
	followDelay := reducedTickDelay(200 + runtimeMobRandomInt(runtime, 20))

	entity.Goals.Add(0, runtimeGoalMove, &aquaticPanicGoal{Fish: concrete})
	entity.Goals.Add(2, runtimeGoalMove, &aquaticAvoidPlayerGoal{Fish: concrete})
	entity.Goals.Add(4, runtimeGoalMove, &aquaticRandomSwimGoal{Fish: concrete})
	entity.Goals.Add(5, runtimeGoalMove, &aquaticFollowSchoolGoal{Fish: concrete, NextStartTick: followDelay})
}

func (runtime *Runtime) formAquaticSchool(fish aquaticFish) bool {
	entity := fish.aquaticBase()

	entity.State.mu.RLock()
	position := entity.State.Position
	entityType := entity.Spec.EntityType
	entity.State.mu.RUnlock()

	minimumChunk := positionLoadedChunk(game.Position{X: position.X - aquaticSchoolSearchRange, Z: position.Z - aquaticSchoolSearchRange})
	maximumChunk := positionLoadedChunk(game.Position{X: position.X + aquaticSchoolSearchRange, Z: position.Z + aquaticSchoolSearchRange})
	leader := fish

	runtime.entityMu.RLock()

	for chunkX := minimumChunk.X; chunkX <= maximumChunk.X; chunkX++ {
		for chunkZ := minimumChunk.Z; chunkZ <= maximumChunk.Z; chunkZ++ {
			for _, candidateEntity := range runtime.entitiesByChunk[LoadedChunk{X: chunkX, Z: chunkZ}] {
				candidate, valid := candidateEntity.(aquaticFish)
				if !valid || candidate == fish {
					continue
				}

				candidateBase := candidate.aquaticBase()

				candidateBase.State.mu.RLock()
				sameType := candidateBase.Spec.EntityType == entityType
				nearby := animalDistanceSquared(position, candidateBase.State.Position) <= aquaticAvoidRangeSquared
				canLead := candidateBase.School.Leader == nil && candidateBase.School.SchoolSize > 1 && candidateBase.School.SchoolSize < candidateBase.Spec.SchoolSize
				candidateBase.State.mu.RUnlock()

				if sameType && nearby && canLead {
					leader = candidate

					break
				}
			}
		}
	}

	joined := false

	for chunkX := minimumChunk.X; chunkX <= maximumChunk.X; chunkX++ {
		for chunkZ := minimumChunk.Z; chunkZ <= maximumChunk.Z; chunkZ++ {
			for _, candidateEntity := range runtime.entitiesByChunk[LoadedChunk{X: chunkX, Z: chunkZ}] {
				candidate, valid := candidateEntity.(aquaticFish)
				if !valid || candidate == leader {
					continue
				}

				candidateBase := candidate.aquaticBase()

				candidateBase.State.mu.RLock()
				sameType := candidateBase.Spec.EntityType == entityType
				nearby := animalDistanceSquared(position, candidateBase.State.Position) <= aquaticAvoidRangeSquared
				available := candidateBase.School.Leader == nil && candidateBase.School.SchoolSize == 1
				candidateBase.State.mu.RUnlock()

				if !sameType || !nearby || !available || !connectAquaticFollower(candidate, leader) {
					continue
				}

				if candidate == fish {
					joined = true
				}
			}
		}
	}

	runtime.entityMu.RUnlock()

	return joined
}

func (runtime *Runtime) validateAquaticSchool(fish aquaticFish) {
	entity := fish.aquaticBase()

	entity.State.mu.RLock()
	position := entity.State.Position
	entityType := entity.Spec.EntityType
	entity.State.mu.RUnlock()

	minimumChunk := positionLoadedChunk(game.Position{X: position.X - aquaticSchoolSearchRange, Z: position.Z - aquaticSchoolSearchRange})
	maximumChunk := positionLoadedChunk(game.Position{X: position.X + aquaticSchoolSearchRange, Z: position.Z + aquaticSchoolSearchRange})
	found := false

	runtime.entityMu.RLock()

	for chunkX := minimumChunk.X; chunkX <= maximumChunk.X && !found; chunkX++ {
		for chunkZ := minimumChunk.Z; chunkZ <= maximumChunk.Z && !found; chunkZ++ {
			for _, candidateEntity := range runtime.entitiesByChunk[LoadedChunk{X: chunkX, Z: chunkZ}] {
				candidate, valid := candidateEntity.(aquaticFish)
				if !valid || candidate == fish {
					continue
				}

				candidateBase := candidate.aquaticBase()

				candidateBase.State.mu.RLock()
				sameType := candidateBase.Spec.EntityType == entityType
				nearby := animalDistanceSquared(position, candidateBase.State.Position) <= aquaticAvoidRangeSquared
				follows := candidateBase.School.Leader == fish
				candidateBase.State.mu.RUnlock()

				if sameType && nearby && follows {
					found = true

					break
				}
			}
		}
	}

	runtime.entityMu.RUnlock()

	if !found {
		entity.State.mu.Lock()
		entity.School.SchoolSize = 1
		entity.State.mu.Unlock()
	}
}

func connectAquaticFollower(follower, leader aquaticFish) bool {
	followerBase := follower.aquaticBase()
	leaderBase := leader.aquaticBase()

	if followerBase == leaderBase || followerBase.Spec.EntityType != leaderBase.Spec.EntityType {
		return false
	}

	leaderBase.State.mu.Lock()

	if leaderBase.State.Removed || leaderBase.Living.Dead || leaderBase.School.SchoolSize >= leaderBase.Spec.SchoolSize {
		leaderBase.State.mu.Unlock()

		return false
	}

	leaderBase.School.SchoolSize++
	leaderBase.State.mu.Unlock()

	followerBase.State.mu.Lock()

	if followerBase.State.Removed || followerBase.Living.Dead || followerBase.School.Leader != nil {
		followerBase.State.mu.Unlock()

		leaderBase.State.mu.Lock()
		leaderBase.School.SchoolSize--
		leaderBase.State.mu.Unlock()

		return false
	}

	followerBase.School.Leader = leader
	followerBase.State.mu.Unlock()

	return true
}

func disconnectAquaticFollower(follower aquaticFish) {
	followerBase := follower.aquaticBase()

	followerBase.State.mu.Lock()
	leader := followerBase.School.Leader
	followerBase.School.Leader = nil
	followerBase.State.mu.Unlock()

	if leader == nil {
		return
	}

	leaderFish, valid := leader.(aquaticFish)
	if !valid {
		return
	}

	leaderBase := leaderFish.aquaticBase()

	leaderBase.State.mu.Lock()
	leaderBase.School.SchoolSize = max(1, leaderBase.School.SchoolSize-1)
	leaderBase.State.mu.Unlock()
}

func aquaticRandomSwimPath(runtime *Runtime, entity *runtimeAquatic, destination []game.Position, position game.Position) []game.Position {
	for range aquaticRandomPositionAttempts {
		offsetX := runtimeMobRandomInt(runtime, aquaticRandomHorizontalRange*2+1) - aquaticRandomHorizontalRange
		offsetY := runtimeMobRandomInt(runtime, aquaticRandomVerticalRange*2+1) - aquaticRandomVerticalRange
		offsetZ := runtimeMobRandomInt(runtime, aquaticRandomHorizontalRange*2+1) - aquaticRandomHorizontalRange

		goal := game.Position{X: position.X + float64(offsetX), Y: position.Y + float64(offsetY), Z: position.Z + float64(offsetZ)}

		destination = runtime.findSwimPathInto(destination, position, goal, entity.Living.Width, entity.Living.Height, aquaticFollowRange)
		if len(destination) > 0 {
			return destination
		}
	}

	return destination[:0]
}

func nearestAquaticPlayer(runtime *Runtime, entity *runtimeAquatic) *Session {
	entity.State.mu.RLock()
	position := entity.State.Position
	entity.State.mu.RUnlock()

	var nearest *Session

	nearestDistanceSquared := float64(aquaticAvoidRangeSquared)

	for _, session := range runtime.sessionView() {
		player := session.playerView()
		if player.Dead || player.GameMode == game.GameModeSpectator {
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

func aquaticFollowerLocked(entity *runtimeAquatic) bool {
	if entity.School.Leader == nil {
		return false
	}

	leader := entity.School.Leader.(aquaticFish).aquaticBase()

	leader.State.mu.RLock()
	alive := !leader.State.Removed && !leader.Living.Dead
	leader.State.mu.RUnlock()

	return alive
}
