package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

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
	skeleton.GoalTarget = runtimeLivingTarget{entity: spawnTestRuntimeLivingEntity(runtime, game.Position{X: 2}, 20)}

	if (&skeletonFleeSunGoal{Entity: skeleton}).CanUse(runtime) {
		t.Fatal("targeted skeleton fled from sun over combat")
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
