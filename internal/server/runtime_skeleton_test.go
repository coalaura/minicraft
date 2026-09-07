package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type skeletonStrafeTestCase struct {
	name     string
	forward  float32
	sideways float32
	wantX    float64
	wantZ    float64
}

type skeletonStaticPathfindTestCase struct {
	name    string
	blocked bool
}

func TestSpawnSkeletonTracksBowWithoutFakeEquipment(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	viewer, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Viewer")

	viewer.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	joinTestSession(t, runtime, viewer)

	connection.reset()

	skeleton := runtime.SpawnSkeleton(game.Position{X: 1.5, Y: 4, Z: 2.5})
	if skeleton == nil {
		t.Fatal("spawn skeleton returned nil")
	}

	if skeleton.Living.Width != 0.6 || skeleton.Living.Height != 1.99 || skeleton.Living.Health != 20 || !skeleton.MainHand.Equal(game.ItemStack{Item: game.ItemBow, Count: 1}) {
		t.Fatalf("skeleton spawn state = %+v", skeleton)
	}

	if skeleton.Head.Empty() == false || skeleton.RuntimeEntityTrackingConfig() != (RuntimeEntityTrackingConfig{ClientRangeChunks: 8, UpdateInterval: 3, TrackDeltas: true}) {
		t.Fatal("skeleton spawn configured fake equipment or wrong tracking")
	}

	equipment := skeleton.EntityEquipment()
	if len(equipment) != 1 || equipment[0].Slot != protocol.EquipmentSlotMainHand || !equipment[0].Item.Equal(skeleton.MainHand) {
		t.Fatalf("skeleton equipment = %+v", equipment)
	}

	assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundAddEntityID, protocol.ClientboundEntityMetadataID, protocol.ClientboundEntityEquipmentID})

	if !viewer.tracksRuntimeEntity(skeleton.State.ID) {
		t.Fatal("loaded viewer did not track skeleton")
	}
}

func TestSkeletonTargetsOnlyNearestValidVisiblePlayerAndRetainsLOS(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.entityRandom = func() float32 {
		return 0
	}

	valid := addRuntimeMobTestPlayer(t, runtime, game.Position{X: 5}, game.GameModeSurvival)

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 1}, game.GameModeCreative)
	addRuntimeMobTestPlayer(t, runtime, game.Position{X: 2}, game.GameModeSpectator)

	skeleton := runtime.SpawnSkeleton(game.Position{})

	skeleton.tickTarget(runtime, true)

	if skeleton.Target.Target.session != valid {
		t.Fatal("skeleton did not select nearest valid player")
	}

	valid.Player.GameMode = game.GameModeCreative

	skeleton.tickTarget(runtime, true)

	if skeleton.Target.Target.present() {
		t.Fatal("skeleton targeted creative or spectator players")
	}

	valid.Player.GameMode = game.GameModeSurvival

	skeleton.tickTarget(runtime, true)

	if skeleton.Target.Target.session != valid {
		t.Fatal("skeleton did not reacquire restored survival target")
	}

	runtime.World.SetBlock(game.BlockPosition{X: 2, Y: 1}, game.Stone)

	for range skeletonNearestUnseenChecks {
		skeleton.tickTarget(runtime, true)

		if skeleton.Target.Target.session != valid {
			t.Fatal("skeleton dropped target before LOS memory expired")
		}
	}

	skeleton.tickTarget(runtime, true)

	if skeleton.Target.Target.present() {
		t.Fatal("skeleton retained target after LOS memory expired")
	}
}

func TestSkeletonRetaliatesAgainstAcceptedLivingDamageCause(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	skeleton := runtime.SpawnSkeleton(game.Position{})

	attacker := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 2}, 20)

	damage := game.Damage{Type: game.DamageMobAttack, Amount: 1, CauseEntityID: attacker.State.ID}

	_, applied := runtime.damageRuntimeLivingEntityLocked(skeleton, damage)
	if !applied {
		t.Fatal("skeleton damage was rejected")
	}

	skeleton.tickTarget(runtime, false)

	if skeleton.Target.Target.entity != attacker || !skeleton.Target.Retaliating {
		t.Fatal("skeleton did not retaliate against accepted runtime living damage cause")
	}

	attacker.State.Removed = true

	skeleton.tickTarget(runtime, false)

	if skeleton.Target.Target.present() || skeleton.Target.Retaliating {
		t.Fatal("skeleton retained invalidated retaliation target")
	}
}

func TestSkeletonDaylightBurningAndSunGoals(t *testing.T) {
	world := zombieGroundWorld(-12, 12, -2, 2)

	world.SetLightingMode(game.LightingNormal)
	world.SetDayTime(6000)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0
	}

	skeleton := runtime.SpawnSkeleton(game.Position{X: 0.5, Z: 0.5})

	skeleton.tickDaylightBurning(runtime)

	if skeleton.Living.RemainingFireTicks != skeletonFireDurationTicks || !skeleton.shouldRestrictSun(runtime) || !skeleton.shouldFleeSun(runtime) {
		t.Fatal("exposed daylight skeleton did not burn, restrict, and flee")
	}

	skeleton.Head = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	if skeleton.shouldRestrictSun(runtime) || skeleton.shouldFleeSun(runtime) {
		t.Fatal("helmeted skeleton restricted or fled sun")
	}

	skeleton.Head = game.ItemStack{}

	skeleton.Navigation.MoveTo([]game.Position{{X: 1.5, Z: .5}}, 1)

	var (
		fleeEntry     *runtimeGoalEntry
		bowEntry      *runtimeGoalEntry
		restrictEntry *runtimeGoalEntry
	)

	for index := range skeleton.Goals.Entries {
		entry := &skeleton.Goals.Entries[index]

		switch entry.Goal.(type) {
		case *skeletonRestrictSunGoal:
			restrictEntry = entry
		case *skeletonFleeSunGoal:
			fleeEntry = entry
		case *skeletonBowGoal:
			bowEntry = entry
		}
	}

	if restrictEntry == nil || fleeEntry == nil || bowEntry == nil || restrictEntry.Priority != 2 || restrictEntry.Flags != 0 || fleeEntry.Priority != 3 || fleeEntry.Flags != runtimeGoalMove || bowEntry.Priority != 4 || bowEntry.Flags != runtimeGoalMove|runtimeGoalLook {
		t.Fatal("skeleton sun and combat priorities are wrong")
	}

	fleeEntry.Running = true

	skeleton.GoalTarget = runtimeLivingTarget{entity: spawnTestRuntimeLivingEntity(runtime, game.Position{X: 2}, 20)}

	skeleton.Goals.Tick(runtime, true)

	if !fleeEntry.Running || bowEntry.Running {
		t.Fatal("target acquisition interrupted an active flee-sun goal")
	}

	skeleton.Navigation.Stop()

	skeleton.Goals.Tick(runtime, true)

	if fleeEntry.Running || !bowEntry.Running {
		t.Fatal("combat did not resume after shelter navigation completed")
	}

	skeleton.GoalTarget = runtimeLivingTarget{}

	world.SetBlock(game.BlockPosition{Y: 2}, game.Stone)

	if !skeleton.shouldRestrictSun(runtime) || skeleton.shouldFleeSun(runtime) {
		t.Fatal("sheltered daytime skeleton did not restrict sun or tried to flee")
	}

	world.SetDayTime(18000)

	if skeleton.shouldRestrictSun(runtime) || skeleton.shouldFleeSun(runtime) {
		t.Fatal("nighttime skeleton restricted or fled sun")
	}
}

func TestSkeletonStrafeUsesVanillaSpeedAndInputScaling(t *testing.T) {
	tests := []skeletonStrafeTestCase{
		{name: "straight", forward: .5, wantZ: .03125},
		{name: "diagonal", forward: .5, sideways: .5, wantX: .03125, wantZ: .03125},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(zombieGroundWorld(-2, 2, -2, 2))

			skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

			skeleton.Living.OnGround = true

			skeleton.MoveControl.Strafe(test.forward, test.sideways)

			skeleton.MoveControl.TickConfigured(skeleton.State.Position, &skeleton.Rotation, skeletonGroundControlConfig(), func(float64, float64) bool {
				return true
			})

			runtime.applyGroundLivingPhysics(&skeleton.State, &skeleton.Living, &skeleton.Rotation, &skeleton.MoveControl, skeletonGroundControlConfig())

			deltaX := skeleton.State.Position.X - .5
			deltaZ := skeleton.State.Position.Z - .5

			if math.Abs(deltaX-test.wantX) > 1e-8 || math.Abs(deltaZ-test.wantZ) > 1e-8 {
				t.Fatalf("strafe displacement = (%0.8f, %0.8f), want (%0.8f, %0.8f)", deltaX, deltaZ, test.wantX, test.wantZ)
			}
		})
	}
}

func TestSkeletonStrafeMatchesVanillaSteadyStateTrace(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-2, 12, -2, 12))

	skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

	skeleton.Living.OnGround = true

	wantSteps := []float64{
		.03125,
		.048312502,
		.0576286291,
		.0627152352,
		.0654925224,
		.0670089214,
		.0678368753,
		.0682889382,
	}

	previousZ := skeleton.State.Position.Z

	for tick, want := range wantSteps {
		skeleton.MoveControl.Strafe(.5, 0)
		skeleton.MoveControl.TickConfigured(skeleton.State.Position, &skeleton.Rotation, skeletonGroundControlConfig(), nil)

		runtime.applyGroundLivingPhysics(&skeleton.State, &skeleton.Living, &skeleton.Rotation, &skeleton.MoveControl, skeletonGroundControlConfig())

		step := skeleton.State.Position.Z - previousZ
		if math.Abs(step-want) > 1e-8 {
			t.Fatalf("strafe tick %d displacement = %.10f, want %.10f", tick+1, step, want)
		}

		previousZ = skeleton.State.Position.Z
	}

	for range 91 {
		skeleton.MoveControl.Strafe(.5, 0)
		skeleton.MoveControl.TickConfigured(skeleton.State.Position, &skeleton.Rotation, skeletonGroundControlConfig(), nil)

		runtime.applyGroundLivingPhysics(&skeleton.State, &skeleton.Living, &skeleton.Rotation, &skeleton.MoveControl, skeletonGroundControlConfig())

		previousZ = skeleton.State.Position.Z
	}

	skeleton.MoveControl.Strafe(.5, 0)
	skeleton.MoveControl.TickConfigured(skeleton.State.Position, &skeleton.Rotation, skeletonGroundControlConfig(), nil)

	runtime.applyGroundLivingPhysics(&skeleton.State, &skeleton.Living, &skeleton.Rotation, &skeleton.MoveControl, skeletonGroundControlConfig())

	step := skeleton.State.Position.Z - previousZ
	if math.Abs(step-.0688326087) > 1e-8 {
		t.Fatalf("steady-state strafe displacement = %.10f, want %.10f", step, .0688326087)
	}
}

func TestSkeletonStrafeWalkabilityFallbackAndOneShotOperation(t *testing.T) {
	skeleton := NewRuntime(&game.World{}).SpawnSkeleton(game.Position{X: .5, Z: .5})

	probeX := 0.0
	probeZ := 0.0

	skeleton.Rotation.Yaw = 90

	skeleton.MoveControl.Strafe(.5, .5)

	skeleton.MoveControl.TickConfigured(skeleton.State.Position, &skeleton.Rotation, skeletonGroundControlConfig(), func(deltaX, deltaZ float64) bool {
		probeX = deltaX
		probeZ = deltaZ

		return false
	})

	if math.Abs(probeX+.03125) > 1e-8 || math.Abs(probeZ-.03125) > 1e-8 {
		t.Fatalf("rotated walkability probe = (%0.8f, %0.8f)", probeX, probeZ)
	}

	if skeleton.MoveControl.ForwardInput != 1 || skeleton.MoveControl.SidewaysInput != 0 || skeleton.MoveControl.MovementSpeed != .25 {
		t.Fatal("non-walkable strafe did not fall back to vanilla forward input")
	}

	skeleton.MoveControl.TickConfigured(skeleton.State.Position, &skeleton.Rotation, skeletonGroundControlConfig(), nil)

	if skeleton.MoveControl.ForwardInput != 0 || skeleton.MoveControl.SidewaysInput != 0 {
		t.Fatal("strafe operation was not consumed after one tick")
	}
}

func TestSkeletonTargetAcquisitionUsesReducedFullPassCadence(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	player := addRuntimeMobTestPlayer(t, runtime, game.Position{X: 5}, game.GameModeSurvival)

	draws := 0

	runtime.entityRandom = func() float32 {
		draws++

		return .1
	}

	skeleton := runtime.SpawnSkeleton(game.Position{})

	skeleton.tickTarget(runtime, false)

	if draws != 0 || skeleton.Target.Target.present() {
		t.Fatal("lightweight goal pass attempted target acquisition")
	}

	skeleton.tickTarget(runtime, true)

	if draws != 1 || skeleton.Target.Target.session != player {
		t.Fatalf("first full-pass target acquisition = draws %d target %+v", draws, skeleton.Target.Target)
	}

	skeleton.Target.Target = runtimeLivingTarget{}
	player.Player.GameMode = game.GameModeCreative
	draws = 0

	for tick := range 10 {
		skeleton.tickTarget(runtime, tick%2 == 0)
	}

	if draws != 5 || skeletonTargetInterval != reducedTickDelay(10) {
		t.Fatalf("ten-tick target cadence = draws %d interval %d", draws, skeletonTargetInterval)
	}
}

func TestSkeletonBowNavigationPathfindOperationCounts(t *testing.T) {
	staticCases := []skeletonStaticPathfindTestCase{
		{name: "visible"},
		{name: "blocked LOS", blocked: true},
	}

	for _, test := range staticCases {
		t.Run(test.name, func(t *testing.T) {
			world := zombieGroundWorld(-2, 16, -3, 3)

			if test.blocked {
				world.SetBlock(game.BlockPosition{X: 8, Y: 1}, game.Stone)
			}

			runtime := NewRuntime(world)

			target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 10.5, Z: .5}, 20)

			skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

			skeleton.GoalTarget = runtimeLivingTarget{entity: target}
			operations := 0

			runtime.groundPathfindStarted = func() {
				operations++
			}

			for range 10 {
				skeleton.tickBowGoal(runtime)
			}

			if operations != 1 {
				t.Fatalf("static target pathfind operations = %d, want 1 (path %d index %d target %t)", operations, len(skeleton.Navigation.Path), skeleton.Navigation.Index, skeleton.Navigation.HasTarget)
			}
		})
	}

	t.Run("moving target", func(t *testing.T) {
		runtime := NewRuntime(zombieGroundWorld(-2, 16, -3, 3))

		target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 10.5, Z: .5}, 20)

		skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

		skeleton.GoalTarget = runtimeLivingTarget{entity: target}
		operations := 0

		runtime.groundPathfindStarted = func() {
			operations++
		}

		for step := range 4 {
			target.State.Position.X = 10.5 + float64(step)

			skeleton.tickBowGoal(runtime)
		}

		if operations != 4 {
			t.Fatalf("moving target pathfind operations = %d, want 4", operations)
		}
	})

	t.Run("failed target path retains current path", func(t *testing.T) {
		runtime := NewRuntime(zombieGroundWorld(-2, 12, -3, 3))

		target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 8.5, Z: .5}, 20)

		skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

		skeleton.GoalTarget = runtimeLivingTarget{entity: target}
		operations := 0

		runtime.groundPathfindStarted = func() {
			operations++
		}

		skeleton.tickBowGoal(runtime)

		path := skeleton.Navigation.Path

		if len(path) == 0 {
			t.Fatal("initial target did not produce a path")
		}

		target.State.Position.X = 30.5

		skeleton.tickBowGoal(runtime)

		if operations != 2 || len(skeleton.Navigation.Path) != len(path) || skeleton.Navigation.Path[0] != path[0] {
			t.Fatalf("failed target path replaced current path: operations %d old %d new %d", operations, len(path), len(skeleton.Navigation.Path))
		}
	})

	t.Run("several skeletons", func(t *testing.T) {
		runtime := NewRuntime(zombieGroundWorld(-2, 16, -6, 6))

		target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 10.5, Z: .5}, 20)

		operations := 0

		runtime.groundPathfindStarted = func() {
			operations++
		}

		for index := range 4 {
			skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: float64(index) + .5})

			skeleton.GoalTarget = runtimeLivingTarget{entity: target}

			for range 5 {
				skeleton.tickBowGoal(runtime)
			}
		}

		if operations != 4 {
			t.Fatalf("four-skeleton pathfind operations = %d, want 4", operations)
		}
	})
}

func TestSkeletonFireDamageAndDaylightReignitionOrdering(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetLightingMode(game.LightingNormal)
	world.SetDayTime(6000)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0
	}

	skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

	skeleton.Living.RemainingFireTicks = 20

	runtime.tickRuntimeLivingBaseEnvironment(skeleton)

	if skeleton.Living.Health != 19 || skeleton.Living.RemainingFireTicks != 19 {
		t.Fatalf("fire cadence = health %v ticks %d", skeleton.Living.Health, skeleton.Living.RemainingFireTicks)
	}

	skeleton.Living.RemainingFireTicks = 1

	skeleton.Tick(runtime, nil)

	if skeleton.Living.RemainingFireTicks != skeletonFireDurationTicks {
		t.Fatalf("daylight reignition ticks = %d, want %d", skeleton.Living.RemainingFireTicks, skeletonFireDurationTicks)
	}
}

func TestSkeletonBowGoalDoesNotReplacePathAfterTargetLoss(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-2, 12, -2, 2))

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 8.5, Z: .5}, 20)

	skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

	skeleton.GoalTarget = runtimeLivingTarget{entity: target}

	skeleton.tickBowGoal(runtime)

	path := skeleton.Navigation.Path

	if len(path) == 0 {
		t.Fatal("bow chase did not create an initial path")
	}

	skeleton.GoalTarget = runtimeLivingTarget{}

	if !(&skeletonBowGoal{Entity: skeleton}).CanContinue(runtime) {
		t.Fatal("bow goal did not continue while its navigation remained active")
	}

	skeleton.tickBowGoal(runtime)

	if len(skeleton.Navigation.Path) != len(path) || skeleton.Navigation.Path[0] != path[0] {
		t.Fatal("bow goal replaced its path after target loss")
	}
}

func TestSkeletonFleeSunSelectsReachableShelter(t *testing.T) {
	world := zombieGroundWorld(-2, 8, -2, 2)

	world.SetLightingMode(game.LightingNormal)
	world.SetDayTime(6000)
	world.SetBlock(game.BlockPosition{X: 2, Y: 2}, game.Stone)

	runtime := NewRuntime(world)

	randomValues := []float32{.6, .5, .5}
	randomIndex := 0

	runtime.entityRandom = func() float32 {
		value := randomValues[randomIndex%len(randomValues)]
		randomIndex++

		return value
	}

	skeleton := runtime.SpawnSkeleton(game.Position{X: .5, Z: .5})

	skeleton.Living.RemainingFireTicks = skeletonFireDurationTicks

	goal := &skeletonFleeSunGoal{Entity: skeleton}

	skeleton.GoalTarget = runtimeLivingTarget{entity: spawnTestRuntimeLivingEntity(runtime, game.Position{X: 6.5, Z: .5}, 20)}

	if goal.CanUse(runtime) {
		t.Fatal("burning skeleton with a target started fleeing sun")
	}

	skeleton.GoalTarget = runtimeLivingTarget{}

	if !goal.CanUse(runtime) {
		t.Fatal("exposed burning skeleton did not find reachable shelter")
	}

	goal.Start(runtime)

	if skeleton.Navigation.Done() {
		t.Fatal("flee sun goal did not start shelter navigation")
	}

	end := skeleton.Sun.FleePath[len(skeleton.Sun.FleePath)-1]

	brightness, seesSky := zombieDaylightBrightness(world, skeletonEyeBlockPosition(end), 6000)
	if seesSky || brightness <= .5 {
		t.Fatalf("shelter endpoint brightness=%v seesSky=%t", brightness, seesSky)
	}
}

func TestSkeletonBowDrawCooldownLOSStrafeAndMeleeFallback(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-2, 20, -2, 2))

	runtime.entityRandom = func() float32 {
		return 0
	}

	target := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 10.5, Z: 0.5}, 20)

	skeleton := runtime.SpawnSkeleton(game.Position{X: 0.5, Z: 0.5})

	skeleton.GoalTarget = runtimeLivingTarget{entity: target}

	skeleton.tickBowGoal(runtime)

	if !skeleton.Living.UsingItem || len(runtime.snapshotRuntimeEntities()) != 2 {
		t.Fatal("skeleton bow began with an instant shot or did not start drawing")
	}

	for range skeletonBowUseTicks - 1 {
		skeleton.tickBowGoal(runtime)
	}

	if len(runtime.snapshotRuntimeEntities()) != 2 || !skeleton.Living.UsingItem {
		t.Fatal("skeleton released bow before twenty ticks")
	}

	skeleton.tickBowGoal(runtime)

	if len(runtime.snapshotRuntimeEntities()) != 3 || skeleton.Living.UsingItem || skeleton.Bow.AttackTime != skeletonBowNormalCooldown {
		t.Fatalf("skeleton bow release = entities %d using %t cooldown %d", len(runtime.snapshotRuntimeEntities()), skeleton.Living.UsingItem, skeleton.Bow.AttackTime)
	}

	hardCooldown := skeleton.bowAttackInterval(game.DifficultyHard)
	normalCooldown := skeleton.bowAttackInterval(game.DifficultyNormal)

	if hardCooldown != skeletonBowHardCooldown || normalCooldown != skeletonBowNormalCooldown {
		t.Fatal("skeleton bow difficulty cooldowns are wrong")
	}

	skeleton.startUsingBow()

	world := runtime.World

	world.SetBlock(game.BlockPosition{X: 5, Y: 1}, game.Stone)

	for range 61 {
		skeleton.tickBowGoal(runtime)
	}

	if skeleton.Living.UsingItem {
		t.Fatal("skeleton retained bow draw after prolonged LOS interruption")
	}

	world.SetBlock(game.BlockPosition{X: 5, Y: 1}, game.Air)

	skeleton.Bow.SeeTime = 19
	skeleton.Bow.StrafeTime = 0

	skeleton.tickBowGoal(runtime)

	if !skeleton.MoveControl.Strafing || skeleton.LookControl.Cooldown == 0 {
		t.Fatal("skeleton bow combat did not request strafe movement and target look")
	}

	skeleton.MainHand = game.ItemStack{}

	if !(&skeletonMeleeGoal{Entity: skeleton}).CanUse(runtime) || (&skeletonBowGoal{Entity: skeleton}).CanUse(runtime) {
		t.Fatal("skeleton did not fall back to melee after bow removal")
	}
}

func TestSkeletonPeacefulRemovalDropsOnceAndArrowDamagesPlayerAndAnimal(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-2, 8, -2, 2))

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	player, _ := newZombieTestSession(t, runtime, game.Position{X: 3.5, Z: 0.5})

	skeleton := runtime.SpawnSkeleton(game.Position{X: 0.5, Z: 0.5})

	skeleton.shoot(runtime, runtimeLivingTarget{session: player})

	arrow := runtime.snapshotRuntimeEntities()[1].(*runtimeArrowEntity)

	for range 8 {
		arrow.Tick(runtime, nil)

		if arrow.State.Removed {
			break
		}
	}

	if player.snapshotPlayer().Health >= 20 {
		t.Fatal("skeleton arrow did not damage player")
	}

	cow := runtime.SpawnCow(game.Position{X: 3.5, Z: 0.5})

	skeleton.shoot(runtime, runtimeLivingTarget{entity: cow})

	for _, entity := range runtime.snapshotRuntimeEntities() {
		arrow, isArrow := entity.(*runtimeArrowEntity)

		if isArrow && !arrow.State.Removed {
			for range 8 {
				arrow.Tick(runtime, nil)

				if arrow.State.Removed {
					break
				}
			}
		}
	}

	if cow.Living.Health >= cow.Spec.MaxHealth || !cow.Living.HasLastDamage {
		t.Fatal("skeleton arrow did not damage animal")
	}

	panicGoal := animalPanicGoalEntry(&cow.runtimeAnimal).Goal.(*animalPanicGoal)

	panicValues := []float32{0.7, 0.5, 0.5}
	panicIndex := 0

	runtime.entityRandom = func() float32 {
		value := panicValues[min(panicIndex, len(panicValues)-1)]
		panicIndex++

		return value
	}

	if !panicGoal.CanUse(runtime) {
		t.Fatal("skeleton arrow damage did not enable animal panic")
	}

	panicGoal.Start(runtime)

	if cow.Navigation.Done() {
		t.Fatal("skeleton arrow damage did not start animal panic navigation")
	}

	cow.Living.InvulnerableTime = 0

	update, applied := runtime.damageRuntimeLivingEntityLocked(cow, game.Damage{Type: game.DamageGenericKill, Amount: 100})

	if !applied || !update.died || !cow.LootDropped {
		t.Fatal("damaged animal did not die and drop loot")
	}

	update, applied = runtime.damageRuntimeLivingEntityLocked(skeleton, game.Damage{Type: game.DamageGenericKill, Amount: 20})

	if !applied || !update.died || !skeleton.LootDropped {
		t.Fatal("skeleton lethal damage did not drop loot")
	}

	count := len(runtime.snapshotRuntimeEntities())

	skeleton.RuntimeLivingDied(runtime)

	if len(runtime.snapshotRuntimeEntities()) != count {
		t.Fatal("skeleton dropped loot more than once")
	}

	peaceful := runtime.SpawnSkeleton(game.Position{X: 0.5, Z: 0.5})

	runtime.Difficulty = game.DifficultyPeaceful

	runtime.Tick()

	if !peaceful.State.Removed {
		t.Fatal("peaceful skeleton was not removed immediately")
	}
}
