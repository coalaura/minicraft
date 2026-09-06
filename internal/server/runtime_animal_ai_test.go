package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestRuntimeGoalSelectorKeepsEqualPriorityRegistrationOrder(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	selector := runtimeGoalSelector{}

	first := &runtimeGoalTestGoal{canUse: true, canContinue: true}
	second := &runtimeGoalTestGoal{canUse: true, canContinue: true}

	selector.Add(4, runtimeGoalMove, first)
	selector.Add(4, runtimeGoalMove, second)

	selector.Tick(runtime)

	if first.starts != 1 || second.starts != 0 || !selector.Entries[0].Running || selector.Entries[1].Running {
		t.Fatalf("equal-priority goals = first starts %d second starts %d", first.starts, second.starts)
	}
}

func TestCowPlayerDamageStartsNavigablePanicAtSourceSpeed(t *testing.T) {
	runtime, player, _, _, _ := newPlayerCombatTest(t)

	for x := int32(-8); x <= 8; x++ {
		for z := int32(-8); z <= 8; z++ {
			runtime.World.SetBlock(game.BlockPosition{X: x, Y: -1, Z: z}, game.Stone)
		}
	}

	runtime.setSessionActiveChunks(player, []LoadedChunk{{}})

	randomValues := []float32{0.7, 0.5, 0.5}
	randomIndex := 0

	runtime.entityRandom = func() float32 {
		if randomIndex >= len(randomValues) {
			return 0.9
		}

		value := randomValues[randomIndex]
		randomIndex++

		return value
	}

	cow := runtime.SpawnCow(game.Position{X: 0.5, Z: 0.5})

	start := cow.State.Position

	cow.Navigation.MoveTo([]game.Position{{X: 1.5, Z: 0.5}}, 1)

	stroll := animalStrollGoalEntry(&cow.runtimeAnimal)

	stroll.Running = true

	damage := game.Damage{Type: game.DamagePlayerAttack, Amount: 1, CauseEntityID: player.snapshotPlayer().EntityID}

	update, applied := runtime.damageRuntimeLivingEntityLocked(cow, damage)
	if !applied {
		t.Fatal("cow player damage was rejected")
	}

	if cow.AmbientSoundTime != 0 {
		t.Fatal("damage changed sound state before the sound callback")
	}

	runtime.sendRuntimeLivingDamageUpdate(update)

	if cow.AmbientSoundTime != -animalAmbientSoundInterval {
		t.Fatal("hurt sound callback did not reset ambient sound timing")
	}

	runtime.Tick()

	panicEntry := animalPanicGoalEntry(&cow.runtimeAnimal)
	if !panicEntry.Running || stroll.Running || cow.Navigation.Done() {
		t.Fatalf("panic state = running %t stroll %t navigation done %t", panicEntry.Running, stroll.Running, cow.Navigation.Done())
	}

	if cow.Navigation.SpeedModifier != cow.Spec.PanicSpeed {
		t.Fatalf("panic speed = %v, want %v", cow.Navigation.SpeedModifier, cow.Spec.PanicSpeed)
	}

	for range 8 {
		runtime.Tick()
	}

	if cow.State.Position == start {
		t.Fatal("panicking cow did not change position")
	}
}

func TestAnimalPanicUsesSharedRegistrationsAndStopsWithNavigation(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-2, 2, -2, 2))

	animals := []*runtimeAnimal{
		&runtime.SpawnCow(game.Position{}).runtimeAnimal,
		&runtime.SpawnSheep(game.Position{}).runtimeAnimal,
		&runtime.SpawnChicken(game.Position{}).runtimeAnimal,
	}

	wantSpeeds := []float64{2, 1.25, 1.4}
	wantRandomLookPriorities := []int{7, 8, 7}

	for index, animal := range animals {
		panicEntry := animalPanicGoalEntry(animal)
		panicGoal := panicEntry.Goal.(*animalPanicGoal)

		floatEntry := animalFloatGoalEntry(animal)

		randomLookEntry := animalRandomLookGoalEntry(animal)

		if panicEntry.Priority != 1 || panicEntry.Flags != runtimeGoalMove || panicGoal.Speed != wantSpeeds[index] {
			t.Fatalf("animal %d panic registration = priority %d flags %d speed %v", index, panicEntry.Priority, panicEntry.Flags, panicGoal.Speed)
		}

		if floatEntry.Priority != 0 || floatEntry.Flags != runtimeGoalJump {
			t.Fatalf("animal %d float registration = priority %d flags %d", index, floatEntry.Priority, floatEntry.Flags)
		}

		animal.Navigation.MoveTo([]game.Position{{X: 1.5}}, panicGoal.Speed)
		panicGoal.Stop(runtime)

		if animal.Navigation.Done() {
			t.Fatalf("animal %d panic stop cancelled navigation", index)
		}

		if randomLookEntry.Priority != wantRandomLookPriorities[index] || randomLookEntry.Flags != runtimeGoalMove|runtimeGoalLook {
			t.Fatalf("animal %d random look registration = priority %d flags %d", index, randomLookEntry.Priority, randomLookEntry.Flags)
		}

		panicEntry.Running = true

		animal.Navigation.Stop()

		animal.Goals.Tick(runtime)

		if panicEntry.Running {
			t.Fatalf("animal %d panic continued after navigation completed", index)
		}
	}
}

func TestAnimalPanicFailureDoesNotDirectSteer(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(0, 0, 0, 0))

	runtime.entityRandom = func() float32 {
		return 0.99
	}

	cow := runtime.SpawnCow(game.Position{X: 0.5, Z: 0.5})

	cow.Living.LastDamageType = game.DamagePlayerAttack
	cow.Living.LastDamageStamp = runtime.World.Time().Age
	cow.Living.HasLastDamage = true

	panicGoal := animalPanicGoalEntry(&cow.runtimeAnimal).Goal.(*animalPanicGoal)
	if panicGoal.CanUse(runtime) || !cow.Navigation.Done() || cow.MoveControl.Moving {
		t.Fatal("failed panic destination started direct movement")
	}
}

func TestBurningAnimalPanicPrefersClosestWater(t *testing.T) {
	world := zombieGroundWorld(-5, 5, -5, 5)

	world.SetBlock(game.BlockPosition{X: 2}, game.Water)
	world.SetBlock(game.BlockPosition{X: -1, Z: 2}, game.Water)

	runtime := NewRuntime(world)

	cow := runtime.SpawnCow(game.Position{X: 0.5, Z: 0.5})

	cow.Living.RemainingFireTicks = 20
	cow.Living.LastDamageType = game.DamageInFire
	cow.Living.LastDamageStamp = runtime.World.Time().Age
	cow.Living.HasLastDamage = true

	panicGoal := animalPanicGoalEntry(&cow.runtimeAnimal).Goal.(*animalPanicGoal)
	if !panicGoal.CanUse(runtime) {
		t.Fatal("burning cow did not find nearby water")
	}

	want := game.Position{X: 2}
	if panicGoal.Goal != want {
		t.Fatalf("panic water destination = %+v, want %+v", panicGoal.Goal, want)
	}

	panicGoal.Start(runtime)

	if cow.Navigation.Done() {
		t.Fatal("reachable panic water did not produce a path")
	}
}

func TestAnimalFloatGoalJumpsInWaterAndPausesInactive(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetBlock(game.BlockPosition{}, game.Water)

	runtime := NewRuntime(world)

	runtime.entityRandom = func() float32 {
		return 0
	}

	cow := runtime.SpawnCow(game.Position{X: 0.5, Z: 0.5})
	start := cow.State.Position

	runtime.Tick()

	if cow.State.Position != start || cow.MoveControl.JumpRequested {
		t.Fatal("inactive float goal advanced animal state")
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.Tick()

	floatEntry := animalFloatGoalEntry(&cow.runtimeAnimal)
	if !floatEntry.Running || !cow.MoveControl.Jump || cow.State.Position.Y <= start.Y {
		t.Fatalf("active float state = running %t jump %t position %+v", floatEntry.Running, cow.MoveControl.Jump, cow.State.Position)
	}
}

func animalPanicGoalEntry(animal *runtimeAnimal) *runtimeGoalEntry {
	for index := range animal.Goals.Entries {
		entry := &animal.Goals.Entries[index]

		_, panicGoal := entry.Goal.(*animalPanicGoal)
		if panicGoal {
			return entry
		}
	}

	return nil
}

func animalFloatGoalEntry(animal *runtimeAnimal) *runtimeGoalEntry {
	for index := range animal.Goals.Entries {
		entry := &animal.Goals.Entries[index]

		_, floatGoal := entry.Goal.(*animalFloatGoal)
		if floatGoal {
			return entry
		}
	}

	return nil
}

func animalStrollGoalEntry(animal *runtimeAnimal) *runtimeGoalEntry {
	for index := range animal.Goals.Entries {
		entry := &animal.Goals.Entries[index]

		_, strollGoal := entry.Goal.(*animalStrollGoal)
		if strollGoal {
			return entry
		}
	}

	return nil
}

func animalRandomLookGoalEntry(animal *runtimeAnimal) *runtimeGoalEntry {
	for index := range animal.Goals.Entries {
		entry := &animal.Goals.Entries[index]

		_, randomLookGoal := entry.Goal.(*animalRandomLookGoal)
		if randomLookGoal {
			return entry
		}
	}

	return nil
}
