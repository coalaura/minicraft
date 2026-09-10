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
	aquaticDefaultSchoolSize      = 8
	salmonMaximumSchoolSize       = 5
)

type aquaticFish interface {
	RuntimeEntity
	aquaticBase() *runtimeAquatic
}

type aquaticTickHooks interface {
	aquaticPreTick(*Runtime)
	aquaticPostMovementTick(*Runtime)
}

type aquaticSchoolState struct {
	Leader     RuntimeEntity
	SchoolSize int
}

type runtimeSchoolingFish struct {
	School            aquaticSchoolState
	MaximumSchoolSize int
}

type schoolingFish interface {
	aquaticFish
	schoolingBase() *runtimeSchoolingFish
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
	Fish          schoolingFish
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

func (entity *runtimePufferfishEntity) aquaticBase() *runtimeAquatic {
	return &entity.runtimeAquatic
}

func (entity *runtimeCodEntity) schoolingBase() *runtimeSchoolingFish {
	return &entity.runtimeSchoolingFish
}

func (entity *runtimeSalmonEntity) schoolingBase() *runtimeSchoolingFish {
	return &entity.runtimeSchoolingFish
}

func (entity *runtimeTropicalFishEntity) schoolingBase() *runtimeSchoolingFish {
	return &entity.runtimeSchoolingFish
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
	schooling, schools := goal.Fish.(schoolingFish)

	entity.State.mu.RLock()
	noActionTime := entity.NoActionTime
	follower := schools && aquaticFollowerLocked(schooling)
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
	schooling, schools := goal.Fish.(schoolingFish)

	entity.State.mu.RLock()
	defer entity.State.mu.RUnlock()

	return (!schools || !aquaticFollowerLocked(schooling)) && !entity.Navigation.Done()
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
	schooling := goal.Fish.schoolingBase()

	entity.State.mu.RLock()
	follower := aquaticFollowerLocked(goal.Fish)
	hasFollowers := schooling.School.SchoolSize > 1
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
	schooling := goal.Fish.schoolingBase()

	entity.State.mu.RLock()
	leader := schooling.School.Leader
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
	schooling := goal.Fish.schoolingBase()

	entity.State.mu.RLock()
	leader := schooling.School.Leader
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
	entity.Goals.Add(0, runtimeGoalMove, &aquaticPanicGoal{Fish: concrete})
	entity.Goals.Add(2, runtimeGoalMove, &aquaticAvoidPlayerGoal{Fish: concrete})
	entity.Goals.Add(4, runtimeGoalMove, &aquaticRandomSwimGoal{Fish: concrete})
}

func (runtime *Runtime) configureSchoolingAquaticGoals(concrete schoolingFish, entity *runtimeAquatic) {
	runtime.configureAquaticGoals(concrete, entity)

	followDelay := reducedTickDelay(200 + runtimeMobRandomInt(runtime, 20))

	entity.Goals.Add(5, runtimeGoalMove, &aquaticFollowSchoolGoal{Fish: concrete, NextStartTick: followDelay})
}

func (runtime *Runtime) formAquaticSchool(fish schoolingFish) bool {
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
				candidate, valid := candidateEntity.(schoolingFish)
				if !valid || candidate == fish {
					continue
				}

				candidateBase := candidate.aquaticBase()
				candidateSchooling := candidate.schoolingBase()

				candidateBase.State.mu.RLock()
				sameType := candidateBase.Spec.EntityType == entityType
				nearby := animalDistanceSquared(position, candidateBase.State.Position) <= aquaticAvoidRangeSquared
				canLead := candidateSchooling.School.Leader == nil && candidateSchooling.School.SchoolSize > 1 && candidateSchooling.School.SchoolSize < candidateSchooling.MaximumSchoolSize
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
				candidate, valid := candidateEntity.(schoolingFish)
				if !valid || candidate == leader {
					continue
				}

				candidateBase := candidate.aquaticBase()
				candidateSchooling := candidate.schoolingBase()

				candidateBase.State.mu.RLock()
				sameType := candidateBase.Spec.EntityType == entityType
				nearby := animalDistanceSquared(position, candidateBase.State.Position) <= aquaticAvoidRangeSquared
				available := candidateSchooling.School.Leader == nil && candidateSchooling.School.SchoolSize == 1
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

func (runtime *Runtime) validateAquaticSchool(fish schoolingFish) {
	entity := fish.aquaticBase()
	schooling := fish.schoolingBase()

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
				candidate, valid := candidateEntity.(schoolingFish)
				if !valid || candidate == fish {
					continue
				}

				candidateBase := candidate.aquaticBase()
				candidateSchooling := candidate.schoolingBase()

				candidateBase.State.mu.RLock()
				sameType := candidateBase.Spec.EntityType == entityType
				nearby := animalDistanceSquared(position, candidateBase.State.Position) <= aquaticAvoidRangeSquared
				follows := candidateSchooling.School.Leader == fish
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
		schooling.School.SchoolSize = 1
		entity.State.mu.Unlock()
	}
}

func connectAquaticFollower(follower, leader schoolingFish) bool {
	followerBase := follower.aquaticBase()
	leaderBase := leader.aquaticBase()
	followerSchooling := follower.schoolingBase()
	leaderSchooling := leader.schoolingBase()

	if followerBase == leaderBase || followerBase.Spec.EntityType != leaderBase.Spec.EntityType {
		return false
	}

	leaderBase.State.mu.Lock()

	if leaderBase.State.Removed || leaderBase.Living.Dead || leaderSchooling.School.SchoolSize >= leaderSchooling.MaximumSchoolSize {
		leaderBase.State.mu.Unlock()

		return false
	}

	leaderSchooling.School.SchoolSize++
	leaderBase.State.mu.Unlock()

	followerBase.State.mu.Lock()

	if followerBase.State.Removed || followerBase.Living.Dead || followerSchooling.School.Leader != nil {
		followerBase.State.mu.Unlock()

		leaderBase.State.mu.Lock()
		leaderSchooling.School.SchoolSize--
		leaderBase.State.mu.Unlock()

		return false
	}

	followerSchooling.School.Leader = leader
	followerBase.State.mu.Unlock()

	return true
}

func disconnectAquaticFollower(follower schoolingFish) {
	followerBase := follower.aquaticBase()
	followerSchooling := follower.schoolingBase()

	followerBase.State.mu.Lock()
	leader := followerSchooling.School.Leader
	followerSchooling.School.Leader = nil
	followerBase.State.mu.Unlock()

	if leader == nil {
		return
	}

	leaderFish, valid := leader.(schoolingFish)
	if !valid {
		return
	}

	leaderBase := leaderFish.aquaticBase()
	leaderSchooling := leaderFish.schoolingBase()

	leaderBase.State.mu.Lock()
	leaderSchooling.School.SchoolSize = max(1, leaderSchooling.School.SchoolSize-1)
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

func initializeSchoolingFish(schooling *runtimeSchoolingFish, maximumSchoolSize int) {
	schooling.School.SchoolSize = 1
	schooling.MaximumSchoolSize = maximumSchoolSize
}

func aquaticFollowerLocked(fish schoolingFish) bool {
	schooling := fish.schoolingBase()

	if schooling.School.Leader == nil {
		return false
	}

	leader := schooling.School.Leader.(schoolingFish).aquaticBase()

	leader.State.mu.RLock()
	alive := !leader.State.Removed && !leader.Living.Dead
	leader.State.mu.RUnlock()

	return alive
}
