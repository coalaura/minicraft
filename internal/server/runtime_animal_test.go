package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type runtimeAnimalSpawnTestCase struct {
	name       string
	entityType game.EntityType
	width      float64
	height     float64
}

type runtimeGoalTestGoal struct {
	canUse      bool
	canContinue bool
	starts      int
	stops       int
	ticks       int
	everyTick   bool
}

func (goal *runtimeGoalTestGoal) CanUse(*Runtime) bool {
	return goal.canUse
}

func (goal *runtimeGoalTestGoal) CanContinue(*Runtime) bool {
	return goal.canContinue
}

func (goal *runtimeGoalTestGoal) Start(*Runtime) {
	goal.starts++
}

func (goal *runtimeGoalTestGoal) Stop(*Runtime) {
	goal.stops++
}

func (goal *runtimeGoalTestGoal) Tick(*Runtime) {
	goal.ticks++
}

func (goal *runtimeGoalTestGoal) RequiresUpdateEveryTick() bool {
	return goal.everyTick
}

func TestRuntimeEntityRegistryContainsImplementedEntities(t *testing.T) {
	wantNames := []string{"minecraft:arrow", "minecraft:chicken", "minecraft:cow", "minecraft:sheep", "minecraft:skeleton", "minecraft:zombie"}
	names := runtimeEntityImplementationNames()

	if len(names) != len(wantNames) {
		t.Fatalf("implemented entity names = %v, want %v", names, wantNames)
	}

	for index := range wantNames {
		if names[index] != wantNames[index] {
			t.Fatalf("implemented entity names = %v, want %v", names, wantNames)
		}
	}

	if runtimeEntityImplemented(game.EntityItem) || runtimeEntityImplemented(game.EntityPig) {
		t.Fatal("unimplemented item or pig entity was registered")
	}
}

func TestSpawnEntityRegistersImplementedAnimals(t *testing.T) {
	tests := []runtimeAnimalSpawnTestCase{
		{name: "cow", entityType: game.EntityCow, width: 0.9, height: 1.4},
		{name: "sheep", entityType: game.EntitySheep, width: 0.9, height: 1.3},
		{name: "chicken", entityType: game.EntityChicken, width: 0.4, height: 0.7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			viewer := &Session{}

			runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

			entity, spawned := runtime.SpawnEntity(test.entityType, game.Position{X: 0.5, Y: 1, Z: 0.5})
			if !spawned || entity == nil {
				t.Fatalf("spawn %s = %v, %t", test.name, entity, spawned)
			}

			living := entity.(RuntimeLivingEntity).RuntimeLivingState()

			state := entity.RuntimeEntityState()

			if living.Width != test.width || living.Height != test.height {
				t.Fatalf("%s dimensions = %v x %v, want %v x %v", test.name, living.Width, living.Height, test.width, test.height)
			}

			if runtime.entities[state.ID] != entity || len(runtime.snapshotEntitiesInChunk(LoadedChunk{})) != 1 {
				t.Fatalf("%s was not authoritatively registered", test.name)
			}

			chunk, active := runtime.ActiveChunk(LoadedChunk{})
			if !active || chunk.EntityCount() != 1 {
				t.Fatalf("%s was not registered in its active chunk", test.name)
			}
		})
	}

	runtime := NewRuntime(&game.World{})

	entity, spawned := runtime.SpawnEntity(game.EntityPig, game.Position{})
	if spawned || entity != nil {
		t.Fatal("unimplemented pig spawned")
	}
}

func TestPassiveAnimalsNeverFarDespawn(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	addRuntimeMobTestPlayer(t, runtime, game.Position{X: runtimeMobDespawnDistance + 1}, game.GameModeSurvival)

	animals := []RuntimeMobEntity{
		runtime.SpawnCow(game.Position{}),
		runtime.SpawnSheep(game.Position{}),
		runtime.SpawnChicken(game.Position{}),
	}

	for _, animal := range animals {
		if runtime.checkRuntimeMobDespawn(animal) || animal.RuntimeEntityState().Removed {
			t.Fatalf("passive animal %T despawned far from a player", animal)
		}
	}
}

func TestRuntimeGoalSelectorExcludesFlagsAndPanicPreemptsStroll(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	selector := runtimeGoalSelector{}

	stroll := &runtimeGoalTestGoal{canUse: true, canContinue: true}
	look := &runtimeGoalTestGoal{canUse: true, canContinue: true}
	panicGoal := &runtimeGoalTestGoal{canContinue: true}

	selector.Add(5, runtimeGoalMove, stroll)
	selector.Add(6, runtimeGoalLook, look)
	selector.Add(1, runtimeGoalMove, panicGoal)

	selector.Tick(runtime)

	if stroll.starts != 1 || look.starts != 1 || panicGoal.starts != 0 {
		t.Fatalf("initial goals = stroll %d look %d panic %d", stroll.starts, look.starts, panicGoal.starts)
	}

	panicGoal.canUse = true

	selector.Tick(runtime)

	if stroll.stops != 1 || panicGoal.starts != 1 || look.stops != 0 {
		t.Fatalf("panic preemption = stroll stops %d panic starts %d look stops %d", stroll.stops, panicGoal.starts, look.stops)
	}

	if selector.Entries[0].Running == false || selector.Entries[1].Running || selector.Entries[2].Running == false {
		t.Fatal("goal flags did not retain panic and look while excluding stroll movement")
	}
}

func TestAnimalPanicInterruptsStrollNavigation(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-2, 2, -2, 2))

	runtime.entityRandom = func() float32 {
		return 0.7
	}

	cow := runtime.SpawnCow(game.Position{X: 0.5, Z: 0.5})

	cow.Navigation.MoveTo([]game.Position{{X: 1.5, Z: 0.5}}, 1)

	for index := range cow.Goals.Entries {
		entry := &cow.Goals.Entries[index]

		if _, strolling := entry.Goal.(*animalStrollGoal); strolling {
			entry.Running = true
		}
	}

	cow.Living.LastDamageType = game.DamagePlayerAttack
	cow.Living.HasLastDamage = true

	cow.Goals.Tick(runtime)

	if cow.Navigation.Done() || cow.Navigation.SpeedModifier != cow.Spec.PanicSpeed {
		t.Fatal("panic goal did not replace stroll with a panic path")
	}
}

func TestCowMilkAndSheepShearingMutateInteractionState(t *testing.T) {
	runtime, player, _, _, _ := newPlayerCombatTest(t)

	cow := runtime.SpawnCow(game.Position{X: 1})
	sheep := runtime.SpawnSheep(game.Position{X: 1})

	player.updatePlayerState(func(state *game.Player) bool {
		state.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBucket, Count: 1}

		return true
	})

	if !runtime.milkCow(player, cow, protocol.MainHand) {
		t.Fatal("cow rejected an empty bucket")
	}

	if player.snapshotPlayer().Inventory.Hotbar[0].Item != game.ItemMilkBucket {
		t.Fatal("cow interaction did not replace bucket with milk bucket")
	}

	player.updatePlayerState(func(state *game.Player) bool {
		state.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemShears, Count: 1}

		return true
	})

	if !runtime.shearSheep(player, sheep, protocol.MainHand) || !sheep.Sheared {
		t.Fatal("sheep shearing was rejected")
	}

	metadata := sheep.EntityMetadata()
	wool := metadata[len(metadata)-1]

	if wool.Index != protocol.SheepWoolMetadataIndex || wool.Value != protocol.MetadataByte(sheepShearedMask) {
		t.Fatalf("sheared wool metadata = %+v", wool)
	}

	if player.snapshotPlayer().Inventory.Hotbar[0].Damage() != 1 {
		t.Fatal("shearing did not damage the shears")
	}

	if len(runtime.snapshotRuntimeEntities()) != 3 {
		t.Fatal("shearing did not spawn wool item entity")
	}
}

func TestChickenEggTimerAndInactiveChunksPauseAnimalState(t *testing.T) {
	runtime := NewRuntime(zombieGroundWorld(-1, 1, -1, 1))

	chicken := runtime.SpawnChicken(game.Position{X: 0.5, Y: 1, Z: 0.5})

	chicken.EggTime = 1
	chicken.Living.RemainingFireTicks = 20

	runtime.Tick()

	if chicken.EggTime != 1 || chicken.Living.RemainingFireTicks != 20 || chicken.TickCount != 0 {
		t.Fatalf("inactive chicken state = egg %d fire %d ticks %d", chicken.EggTime, chicken.Living.RemainingFireTicks, chicken.TickCount)
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.entityRandom = func() float32 {
		return 0
	}

	runtime.Tick()

	if chicken.EggTime != chickenEggMinimumTicks || chicken.TickCount != 1 || chicken.Living.RemainingFireTicks != 19 {
		t.Fatalf("active chicken state = egg %d fire %d ticks %d", chicken.EggTime, chicken.Living.RemainingFireTicks, chicken.TickCount)
	}

	if len(runtime.snapshotRuntimeEntities()) != 2 {
		t.Fatal("expired chicken egg timer did not spawn an egg item")
	}
}

func TestAnimalEnvironmentSoundSourceFallingAndDeathDrops(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	runtime := NewRuntime(world)

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	cow := runtime.SpawnCow(game.Position{X: 0.5, Z: 0.5})
	chicken := runtime.SpawnChicken(game.Position{X: 0.5, Y: 6, Z: 0.5})
	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	if cow.RuntimeEntitySoundSource() != protocol.SoundSourceNeutral || zombie.RuntimeEntitySoundSource() != protocol.SoundSourceHostile {
		t.Fatal("animal and zombie sound sources do not distinguish neutral and hostile mobs")
	}

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	for range 100 {
		runtime.Tick()

		if chicken.Living.OnGround {
			break
		}
	}

	if !chicken.Living.OnGround || chicken.Living.Health != chicken.Spec.MaxHealth {
		t.Fatalf("chicken falling = grounded %t health %v", chicken.Living.OnGround, chicken.Living.Health)
	}

	world.SetBlock(game.BlockPosition{}, game.Fire)

	cow.Living.Health = 0.5
	cow.Living.InvulnerableTime = 0

	runtime.Tick()
	runtime.Tick()

	if !cow.Living.Dead || !cow.LootDropped {
		t.Fatal("animal shared fire environment did not kill and drop loot")
	}

	entityCount := len(runtime.snapshotRuntimeEntities())

	cow.RuntimeLivingDied(runtime)

	if len(runtime.snapshotRuntimeEntities()) != entityCount {
		t.Fatal("animal death dropped loot more than once")
	}
}
