package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type zombieDifficultyDamageTestCase struct {
	name       string
	difficulty game.Difficulty
	wantDamage float32
}

type zombieRotationTestCase struct {
	name    string
	current float32
	wanted  float32
	maximum float32
	want    float32
}

func TestZombieFlatGroundMovementMatchesVanillaRecurrence(t *testing.T) {
	world := zombieGroundWorld(-2, 2, -2, 20)

	runtime := NewRuntime(world)

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.OnGround = true
	zombie.MoveControl.ForwardInput = zombieMovementSpeed

	wantIncrements := []float64{
		0.0529000019,
		0.0817834063,
		0.0975537470,
		0.1061643539,
		0.1108657459,
		0.1134327062,
		0.1148342667,
		0.1155995188,
	}

	previousZ := zombie.State.Position.Z

	for tick, want := range wantIncrements {
		zombie.applyGroundLivingPhysics(runtime)

		increment := zombie.State.Position.Z - previousZ
		if math.Abs(increment-want) > 2e-7 {
			t.Fatalf("tick %d displacement = %.10f, want %.10f", tick+1, increment, want)
		}

		previousZ = zombie.State.Position.Z
	}

	for range 92 {
		zombie.applyGroundLivingPhysics(runtime)
	}

	previousZ = zombie.State.Position.Z

	zombie.applyGroundLivingPhysics(runtime)

	increment := zombie.State.Position.Z - previousZ
	if math.Abs(increment-0.1165198443) > 2e-7 {
		t.Fatalf("sustained displacement = %.10f, want %.10f", increment, 0.1165198443)
	}

	if math.Abs(zombie.Living.Velocity.Z-0.0636198424) > 2e-7 {
		t.Fatalf("retained velocity = %.10f, want %.10f", zombie.Living.Velocity.Z, 0.0636198424)
	}
}

func TestZombieMovementUsesBlockFrictionAndAirInput(t *testing.T) {
	world := zombieGroundWorld(-1, 2, -1, 1)

	world.SetBlock(game.BlockPosition{X: 0, Y: -1, Z: 0}, game.Ice)

	runtime := NewRuntime(world)

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.OnGround = true
	zombie.MoveControl.ForwardInput = zombieMovementSpeed

	zombie.applyGroundLivingPhysics(runtime)

	friction := float32(0.98)

	wantGround := float64(zombieMovementSpeed * (groundMovementInputScale / (friction * friction * friction)) * zombieMovementSpeed)
	if math.Abs((zombie.State.Position.Z-0.5)-wantGround) > 1e-8 {
		t.Fatalf("ice startup displacement = %.10f, want %.10f", zombie.State.Position.Z-0.5, wantGround)
	}

	airRuntime := NewRuntime(&game.World{})

	airborne := airRuntime.SpawnZombie(game.Position{X: 0.5, Y: 10, Z: 0.5})

	airborne.MoveControl.ForwardInput = zombieMovementSpeed

	airborne.applyGroundLivingPhysics(airRuntime)

	wantAir := float64(zombieFlyingSpeed * zombieMovementSpeed)
	if math.Abs((airborne.State.Position.Z-0.5)-wantAir) > 1e-8 {
		t.Fatalf("air displacement = %.10f, want %.10f", airborne.State.Position.Z-0.5, wantAir)
	}

	if math.Abs(airborne.Living.Velocity.Z-wantAir*float64(zombieAirFriction)) > 1e-8 {
		t.Fatalf("air retained velocity = %.10f", airborne.Living.Velocity.Z)
	}
}

func TestGroundNavigationRecalculationOmitsCurrentCellAndAcceptsOvershoot(t *testing.T) {
	world := zombieGroundWorld(-1, 6, -1, 1)

	runtime := NewRuntime(world)

	position := game.Position{X: 0.8, Z: 0.5}

	path := runtime.findGroundPath(position, game.Position{X: 4.5, Z: 0.5}, 0.6, 1.95, 35)
	if len(path) == 0 || path[0].X <= position.X {
		t.Fatalf("recalculated path reversed from %+v to %v", position, path)
	}

	overshot := []game.Position{{X: 1.5, Z: 0.5}, {X: 2.5, Z: 0.5}}
	if !groundNavigationNodeReached(game.Position{X: 1.8, Z: 0.5}, overshot, 0, 0.6) {
		t.Fatal("overshot waypoint was not advanced")
	}

	if !groundNavigationNodeReached(game.Position{X: 1.1, Z: 0.5}, overshot, 0, 0.6) {
		t.Fatal("zombie-width waypoint acceptance rejected a nearby node")
	}
}

func TestGroundNavigationUsesValidDiagonalsAndRejectsTouchingCorners(t *testing.T) {
	world := zombieGroundWorld(-3, 4, -3, 4)

	runtime := NewRuntime(world)

	start := game.Position{X: 0.5, Z: 0.5}
	goal := game.Position{X: 3.5, Z: 3.5}

	path := runtime.findGroundPath(start, goal, 0.6, 1.95, 35)
	if len(path) == 0 || path[0] != (game.Position{X: 1.5, Z: 1.5}) {
		t.Fatalf("open diagonal path = %v", path)
	}

	world.SetBlock(game.BlockPosition{X: 1, Z: 0}, game.Stone)
	world.SetBlock(game.BlockPosition{X: 0, Z: 1}, game.Stone)

	path = runtime.findGroundPath(start, game.Position{X: 1.5, Z: 1.5}, 0.6, 1.95, 35)
	if len(path) > 0 && path[0] == (game.Position{X: 1.5, Z: 1.5}) {
		t.Fatalf("path clipped through touching corners: %v", path)
	}
}

func TestZombieFailedPathDoesNotDirectSteer(t *testing.T) {
	world := zombieGroundWorld(-4, 4, -4, 4)

	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			if x == 0 && z == 0 {
				continue
			}

			world.SetBlock(game.BlockPosition{X: x, Z: z}, game.Stone)
			world.SetBlock(game.BlockPosition{X: x, Y: 1, Z: z}, game.Stone)
		}
	}

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0
	}

	session, _ := newZombieTestSession(t, runtime, game.Position{X: 3.5, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Target.Session = session

	for range 30 {
		runtime.Tick()
	}

	if zombie.State.Position.X != 0.5 || zombie.State.Position.Z != 0.5 || zombie.MoveControl.Moving {
		t.Fatalf("failed path directly steered zombie: position %+v control %+v", zombie.State.Position, zombie.MoveControl)
	}
}

func TestZombieMoveLookAndBodyRotationAreBoundedAndWrapAware(t *testing.T) {
	tests := []zombieRotationTestCase{
		{name: "left", current: 0, wanted: -45, maximum: 90, want: -45},
		{name: "right", current: 0, wanted: 45, maximum: 90, want: 45},
		{name: "reversal", current: 0, wanted: 180, maximum: 90, want: -90},
		{name: "wrap", current: 179, wanted: -179, maximum: 90, want: 181},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := rotateTowards(test.current, test.wanted, test.maximum)
			if got != test.want {
				t.Fatalf("rotation = %v, want %v", got, test.want)
			}
		})
	}

	control := groundMoveControlState{Wanted: game.Position{X: 0.5, Z: -10}, Moving: true, SpeedModifier: 1}

	rotation := game.Rotation{}

	control.Tick(game.Position{X: 0.5, Z: 0.5}, &rotation)

	if math.Abs(float64(rotation.Yaw)) > float64(zombieMoveMaximumTurn) {
		t.Fatalf("move-control yaw delta = %v", rotation.Yaw)
	}

	look := groundLookControlState{}

	look.SetWanted(game.Position{X: -10, Y: 1.74}, zombieIdleLookMaximumYaw, zombieIdleLookMaximumPitch)

	look.Tick(game.Position{}, &rotation, false)

	if math.Abs(float64(rotation.HeadYaw)) > float64(zombieIdleLookMaximumYaw) || rotation.HeadYaw == rotation.Yaw {
		t.Fatalf("head rotation was unbounded or coupled: body %v head %v", rotation.Yaw, rotation.HeadYaw)
	}

	rotation.Pitch = 40
	look.Cooldown = 0

	look.Tick(game.Position{}, &rotation, false)

	if rotation.Pitch != 0 {
		t.Fatalf("idle look pitch = %v, want 0", rotation.Pitch)
	}
}

func TestZombieDeterministicIdleStrollStopsAndRandomLookDoesNotMove(t *testing.T) {
	world := zombieGroundWorld(-12, 12, -2, 2)

	runtime := NewRuntime(world)

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	values := []float32{0, 1, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 0.75, 0.5, 0.5, 1, 1}
	index := 0

	runtime.entityRandom = func() float32 {
		value := values[min(index, len(values)-1)]
		index++

		return value
	}

	zombie.tickIdleGoals(runtime, true)

	if !zombie.Idle.Strolling || zombie.Navigation.Done() {
		t.Fatalf("deterministic stroll did not start: idle %+v path %v", zombie.Idle, zombie.Navigation.Path)
	}

	zombie.Navigation.Index = len(zombie.Navigation.Path)

	runtime.entityRandom = func() float32 {
		return 1
	}

	zombie.tickIdleGoals(runtime, true)

	if zombie.Idle.Strolling {
		t.Fatal("completed idle stroll did not stop")
	}

	zombie.NoActionTime = 100

	randomValues := []float32{0, 0, 0, 0.5}
	index = 0

	runtime.entityRandom = func() float32 {
		value := randomValues[min(index, len(randomValues)-1)]
		index++

		return value
	}

	zombie.tickIdleGoals(runtime, true)

	if !zombie.Idle.RandomLook {
		t.Fatal("deterministic random look did not start")
	}

	zombie.tickIdleGoals(runtime, false)

	position := zombie.State.Position

	zombie.tickControlsAndMovement(runtime)

	if zombie.State.Position.X != position.X || zombie.State.Position.Z != position.Z || zombie.Rotation.HeadYaw == 0 {
		t.Fatalf("random look changed movement or not orientation: position %+v head %v", zombie.State.Position, zombie.Rotation.HeadYaw)
	}
}

func TestZombieTargetInterruptsIdleAndLineOfSightMemoryExpires(t *testing.T) {
	world := zombieGroundWorld(-2, 6, -2, 2)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 1
	}

	session, _ := newZombieTestSession(t, runtime, game.Position{X: 4.5, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Idle.Strolling = true

	zombie.Navigation.MoveTo([]game.Position{{X: 1.5, Z: 0.5}}, 1)

	zombie.tickGoals(runtime, session, false)

	if !zombie.Idle.Strolling {
		t.Fatal("lightweight goal pass interrupted idle stroll")
	}

	zombie.tickGoals(runtime, session, true)

	if zombie.Idle.Strolling {
		t.Fatal("full goal pass did not interrupt idle stroll")
	}

	zombie.Target.Session = session

	world.SetBlock(game.BlockPosition{X: 2, Z: 0}, game.Stone)
	world.SetBlock(game.BlockPosition{X: 2, Y: 1, Z: 0}, game.Stone)

	for range zombieTargetUnseenTicks {
		entityTarget := zombie.tickTarget(runtime, true)

		if entityTarget != session {
			t.Fatal("target was dropped before LOS memory expired")
		}
	}

	entityTarget := zombie.tickTarget(runtime, true)

	if entityTarget != nil || zombie.Target.Session != nil {
		t.Fatal("target survived beyond LOS memory")
	}
}

func TestZombieAggressiveTimingAndDifficultyDamage(t *testing.T) {
	tests := []zombieDifficultyDamageTestCase{
		{name: "peaceful", difficulty: game.DifficultyPeaceful, wantDamage: 0},
		{name: "easy", difficulty: game.DifficultyEasy, wantDamage: 2.5},
		{name: "normal", difficulty: game.DifficultyNormal, wantDamage: 3},
		{name: "hard", difficulty: game.DifficultyHard, wantDamage: 4.5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			damage := zombieDamageForDifficulty(test.difficulty)

			if damage != test.wantDamage {
				t.Fatalf("damage = %v, want %v", damage, test.wantDamage)
			}

			testRuntime := NewRuntime(zombieGroundWorld(-1, 2, -1, 1))

			testRuntime.Difficulty = test.difficulty

			testRuntime.entityRandom = func() float32 {
				return 0
			}

			target, _ := newZombieTestSession(t, testRuntime, game.Position{X: 1.1, Z: 0.5})

			testRuntime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

			testRuntime.Tick()

			wantHealth := float32(20) - test.wantDamage
			if target.snapshotPlayer().Health != wantHealth {
				t.Fatalf("pipeline health = %v, want %v", target.snapshotPlayer().Health, wantHealth)
			}
		})
	}

	world := zombieGroundWorld(-1, 2, -1, 1)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 1
	}

	session, _ := newZombieTestSession(t, runtime, game.Position{X: 1.1, Z: 0.5})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Melee.Active = true
	zombie.Melee.PathedTarget = session.snapshotPlayer().Position
	zombie.Melee.TicksUntilNextPathRecalculation = 100
	zombie.Melee.TicksUntilNextAttack = 20

	for range 10 {
		zombie.tickMeleeGoal(runtime, session, false)
	}

	if zombie.Aggressive {
		t.Fatal("zombie raised arms before the latter half of attack cooldown")
	}

	zombie.tickMeleeGoal(runtime, session, false)

	if !zombie.Aggressive {
		t.Fatal("zombie did not raise arms at the source-backed attack timing")
	}
}
