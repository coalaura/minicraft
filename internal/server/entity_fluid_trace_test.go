package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type itemFluidTraceSample struct {
	Tick     int
	Position game.Position
	Velocity game.Velocity
}

type itemFluidTraceCase struct {
	Name     string
	World    *game.World
	Position game.Position
	Velocity game.Velocity
	Item     game.Item
	Expected []itemFluidTraceSample
}

type physicalFluidScaleTestCase struct {
	Name     string
	Fluid    game.Block
	Neighbor game.Block
	FastLava bool
	WantX    float64
}

func TestItemFluidSourceTraces(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")
	shallowWater := mustFluidBlock(t, game.Water, "7")
	fallingWater := mustFluidBlock(t, game.Water, "8")
	flowingLava := mustFluidBlock(t, game.Lava, "1")

	// Values follow Entity.baseTick and ItemEntity.tick from the pinned 1.21.11 source.
	tests := []itemFluidTraceCase{
		{Name: "horizontal", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{}, Replacement: game.Water}, {Position: game.BlockPosition{X: 1}, Replacement: flowingWater}}), Position: game.Position{X: 0.5, Y: 0.5, Z: 0.5}, Item: game.ItemStone, Expected: []itemFluidTraceSample{
			{1, game.Position{X: 0.51386, Y: 0.5005, Z: 0.5}, game.Velocity{X: 0.027582800264358522, Y: 0.00049}},
			{2, game.Position{X: 0.5550269722617149, Y: 0.50149, Z: 0.5}, game.Velocity{X: 0.05434363360167832, Y: 0.0009702}},
			{3, game.Position{X: 0.6226871695273765, Y: 0.5029602, Z: 0.5}, game.Velocity{X: 0.08030699461086416, Y: 0.001440796}},
			{4, game.Position{X: 0.716051094192132, Y: 0.504900996, Z: 0.5}, game.Velocity{X: 0.10549664795223594, Y: 0.00190198008}},
			{5, game.Position{X: 0.8343527756648456, Y: 0.50730297608, Z: 0.5}, game.Velocity{X: 0.12993565009968483, Y: 0.0023539404784}},
			{6, game.Position{X: 0.9768490692635335, Y: 0.5101569165584, Z: 0.5}, game.Velocity{X: 0.15364637044461535, Y: 0.002796861668832}},
		}},
		{Name: "shallow edge", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{}, Replacement: game.Water}, {Position: game.BlockPosition{X: 1}, Replacement: shallowWater}}), Position: game.Position{X: 0.99, Y: 0.02, Z: 0.5}, Item: game.ItemStone, Expected: []itemFluidTraceSample{
			{1, game.Position{X: 1.00386, Y: 0.0205, Z: 0.5}, game.Velocity{X: 0.027582800264358522, Y: 0.00049}},
			{2, game.Position{X: 1.0450269722617149, Y: 0.02149, Z: 0.5}, game.Velocity{X: 0.05434363360167832, Y: 0.0009702}},
			{3, game.Position{X: 1.1126871695273766, Y: 0.0229602, Z: 0.5}, game.Velocity{X: 0.08030699461086416, Y: 0.001440796}},
			{4, game.Position{X: 1.2060510941921321, Y: 0.024900996, Z: 0.5}, game.Velocity{X: 0.10549664795223594, Y: 0.00190198008}},
			{5, game.Position{X: 1.325547742144368, Y: -0.01319702392, Z: 0.5}, game.Velocity{X: 0.1311067172724089, Y: -0.0373360595216}},
			{6, game.Position{X: 1.4692033922440528, Y: -0.0500330834416, Z: 0.5}, game.Velocity{X: 0.15478253983770524, Y: -0.036099338331168}},
		}},
		{Name: "falling cliff", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{Y: 1}, Replacement: fallingWater}, {Position: game.BlockPosition{X: 1, Y: 1}, Replacement: game.Stone}}), Position: game.Position{X: 0.5, Y: 1.5, Z: 0.5}, Item: game.ItemStone, Expected: []itemFluidTraceSample{
			{1, game.Position{X: 0.5, Y: 1.4865, Z: 0.5}, game.Velocity{Y: -0.02723}},
			{2, game.Position{X: 0.5, Y: 1.44577, Z: 0.5}, game.Velocity{Y: -0.0539154}},
			{3, game.Position{X: 0.5, Y: 1.3783546, Z: 0.5}, game.Velocity{Y: -0.080067092}},
			{4, game.Position{X: 0.5, Y: 1.284787508, Z: 0.5}, game.Velocity{Y: -0.10569575016}},
			{5, game.Position{X: 0.5, Y: 1.16559175784, Z: 0.5}, game.Velocity{Y: -0.1308118351568}},
			{6, game.Position{X: 0.5, Y: 1.0212799226832, Z: 0.5}, game.Velocity{Y: -0.155425598453664}},
		}},
		{Name: "entering water", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{}, Replacement: game.Water}, {Position: game.BlockPosition{X: 1}, Replacement: flowingWater}}), Position: game.Position{X: -0.2, Y: 0.5, Z: 0.5}, Velocity: game.Velocity{X: 0.2}, Item: game.ItemStone, Expected: []itemFluidTraceSample{
			{1, game.Position{Y: 0.46, Z: 0.5}, game.Velocity{X: 0.2100000038146973, Y: -0.0392}},
			{2, game.Position{X: 0.22176000377655034, Y: 0.4213, Z: 0.5}, game.Velocity{X: 0.23132480793075574, Y: -0.037926}},
			{3, game.Position{X: 0.4646315636279985, Y: 0.383874, Z: 0.5}, game.Velocity{X: 0.2520141332868266, Y: -0.03667748}},
			{4, game.Position{X: 0.7279855555819568, Y: 0.34769652, Z: 0.5}, game.Velocity{X: 0.2720869171379579, Y: -0.0354539304}},
			{5, game.Position{X: 1.011211603548535, Y: 0.3127425896, Z: 0.5}, game.Velocity{X: 0.29156153240935495, Y: -0.034254851792}},
			{6, game.Position{X: 1.3137175206337965, Y: 0.278987737808, Z: 0.5}, game.Velocity{X: 0.31045580451339866, Y: -0.03307975475616}},
		}},
		{Name: "leaving stream", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{}, Replacement: game.Water}}), Position: game.Position{X: 0.8, Y: 0.5, Z: 0.5}, Velocity: game.Velocity{X: 0.4}, Item: game.ItemStone, Expected: []itemFluidTraceSample{
			{1, game.Position{X: 1.196, Y: 0.5005, Z: 0.5}, game.Velocity{X: 0.38808000755310063, Y: 0.00049}},
			{2, game.Position{X: 1.5840800075531007, Y: 0.46099, Z: 0.5}, game.Velocity{X: 0.38031841480407735, Y: -0.0387198}},
			{3, game.Position{X: 1.9643984223571782, Y: 0.3822702, Z: 0.5}, game.Velocity{X: 0.3727120537619939, Y: -0.077145404}},
			{4, game.Position{X: 2.337110476119172, Y: 0.265124796, Z: 0.5}, game.Velocity{X: 0.3652578197956723, Y: -0.11480249592}},
			{5, game.Position{X: 2.7023682959148445, Y: 0.11032230008, Z: 0.5}, game.Velocity{X: 0.35795267036649886, Y: -0.1517064460016}},
			{6, game.Position{X: 3.0603209662813433, Y: -0.0813841459216, Z: 0.5}, game.Velocity{X: 0.35079362378657425, Y: -0.187872317081568}},
		}},
		{Name: "lava", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{}, Replacement: game.Lava}, {Position: game.BlockPosition{X: 1}, Replacement: flowingLava}}), Position: game.Position{X: 0.5, Y: 0.5, Z: 0.5}, Item: game.ItemNetheriteIngot, Expected: []itemFluidTraceSample{
			{1, game.Position{X: 0.504275, Y: 0.5005, Z: 0.5}, game.Velocity{X: 0.0065228334148724875, Y: 0.00049}},
			{2, game.Position{X: 0.5126883584107955, Y: 0.50149, Z: 0.5}, game.Velocity{X: 0.010578424736385027, Y: 0.0009702}},
			{3, game.Position{X: 0.524954528577028, Y: 0.5029602, Z: 0.5}, game.Velocity{X: 0.014354180330199754, Y: 0.001440796}},
			{4, game.Position{X: 0.5408076665573844, Y: 0.504900996, Z: 0.5}, game.Velocity{X: 0.01786940885645725, Y: 0.00190198008}},
			{5, game.Position{X: 0.5600002716376855, Y: 0.50730297608, Z: 0.5}, game.Velocity{X: 0.021142086678098256, Y: 0.0023539404784}},
			{6, game.Position{X: 0.5823019206485455, Y: 0.5101569165584, Z: 0.5}, game.Velocity{X: 0.024188949789346336, Y: 0.002796861668832}},
		}},
		{Name: "long stream", World: fluidTraceWorld([]game.BlockChange{{Position: game.BlockPosition{}, Replacement: game.Water}, {Position: game.BlockPosition{X: 1}, Replacement: flowingWater}, {Position: game.BlockPosition{X: 2}, Replacement: game.Stone}}), Position: game.Position{X: 0.5, Y: 0.5, Z: 0.5}, Item: game.ItemStone, Expected: []itemFluidTraceSample{
			{1, game.Position{X: 0.51386, Y: 0.5005, Z: 0.5}, game.Velocity{X: 0.027582800264358522, Y: 0.00049}},
			{10, game.Position{X: 1.7746788489994436, Y: 0.5259141884372449, Z: 0.5}, game.Velocity{X: 0.24163090340329355, Y: 0.004481716231255102}},
			{20, game.Position{X: 1.875, Y: 0.5928197653999909, Z: 0.5}, game.Velocity{X: 0.014, Y: 0.008143604692000185}},
			{30, game.Position{X: 1.875, Y: 0.6527182912434857, Z: 0.5}, game.Velocity{X: 0.014, Y: -0.02855436582486971}},
			{40, game.Position{X: 1.875, Y: 0.417463980108252, Z: 0.5}, game.Velocity{X: 0.014, Y: -0.01884927960216503}},
			{50, game.Position{X: 1.875, Y: 0.27097587805470363, Z: 0.5}, game.Velocity{X: 0.014, Y: -0.010919517561094058}},
			{60, game.Position{X: 1.875, Y: 0.1970162316122948, Z: 0.5}, game.Velocity{X: 0.014, Y: -0.004440324632245878}},
			{70, game.Position{X: 1.875, Y: 0.18231761397529855, Z: 0.5}, game.Velocity{X: 0.014, Y: 0.0008536477204940483}},
			{80, game.Position{X: 1.875, Y: 0.21603957148338449, Z: 0.5}, game.Velocity{X: 0.014, Y: 0.005179208570332329}},
		}},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runtime := NewRuntime(test.World)

			item := runtime.SpawnItemEntity(game.ItemStack{Item: test.Item, Count: 1}, test.Position, test.Velocity, 32767)

			expectedIndex := 0
			lastTick := test.Expected[len(test.Expected)-1].Tick

			for tick := 1; tick <= lastTick; tick++ {
				item.Tick(runtime, nil)

				expected := test.Expected[expectedIndex]
				if tick != expected.Tick {
					continue
				}

				assertPositionClose(t, item.State.Position, expected.Position, 1e-14)
				assertVelocityClose(t, item.Velocity, expected.Velocity, 1e-14)

				expectedIndex++
			}
		})
	}
}

func TestItemFluidContactsPreserveOverlappingWaterAndLava(t *testing.T) {
	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1}, Replacement: game.Lava},
	})

	runtime := NewRuntime(world)

	item := &runtimeItemEntity{State: RuntimeEntityState{Position: game.Position{X: 1, Y: 0.5, Z: 0.5}}}

	contacts := runtime.fluidContacts(itemEntityBox(item.State.Position), false)
	if contacts.Water.Depth <= 0 || contacts.Lava.Depth <= 0 {
		t.Fatalf("overlapping contacts = %+v", contacts)
	}

	want := item.Velocity

	waterImpulse := fluidCurrentImpulse(want, contacts.Water.Flow, entityWaterPush)

	want.X += waterImpulse.X
	want.Y += waterImpulse.Y
	want.Z += waterImpulse.Z

	lavaImpulse := fluidCurrentImpulse(want, contacts.Lava.Flow, entityLavaPush)

	want.X += lavaImpulse.X
	want.Y += lavaImpulse.Y
	want.Z += lavaImpulse.Z

	applyItemFluidCurrents(runtime, item, contacts)

	assertVelocityClose(t, item.Velocity, want, 0)
}

func TestItemFluidFlowCacheExpiresBetweenActiveChunkTicks(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")

	position := game.BlockPosition{}
	neighbor := game.BlockPosition{X: 1}

	world := fluidTraceWorld([]game.BlockChange{
		{Position: position, Replacement: game.Water},
		{Position: neighbor, Replacement: flowingWater},
	})

	runtime := NewRuntime(world)

	state := world.FluidAt(position)

	runtime.itemFluidFlowCacheActive = true

	before := runtime.itemFluidFlowVector(position, state, true)

	world.SetBlock(neighbor, game.Water)

	cached := runtime.itemFluidFlowVector(position, state, true)

	assertVelocityClose(t, cached, before, 0)

	runtime.itemFluidFlowCacheActive = false

	runtime.tickActiveChunks()

	if len(runtime.itemFluidFlowCache) != 0 {
		t.Fatalf("fluid flow cache retained %d entries", len(runtime.itemFluidFlowCache))
	}

	after := runtime.itemFluidFlowVector(position, state, true)
	if after == before {
		t.Fatalf("fluid flow cache retained stale vector %+v", after)
	}
}

func TestPhysicalEntityFluidCurrentScalesMatchSource(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")
	flowingLava := mustFluidBlock(t, game.Lava, "1")

	tests := []physicalFluidScaleTestCase{
		{Name: "water", Fluid: game.Water, Neighbor: flowingWater, WantX: 0.014},
		{Name: "lava minimum", Fluid: game.Lava, Neighbor: flowingLava, WantX: 0.0045},
		{Name: "fast lava", Fluid: game.Lava, Neighbor: flowingLava, FastLava: true, WantX: 0.007},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			world := fluidTraceWorld([]game.BlockChange{
				{Position: game.BlockPosition{}, Replacement: test.Fluid},
				{Position: game.BlockPosition{X: 1}, Replacement: test.Neighbor},
			})

			runtime := NewRuntime(world)

			runtime.FluidEnvironment.FastLava = test.FastLava

			velocity := game.Velocity{}

			runtime.applyPhysicalEntityFluidCurrents(itemEntityBox(game.Position{X: 0.5, Y: 0.5, Z: 0.5}), &velocity)

			assertVelocityClose(t, velocity, game.Velocity{X: test.WantX}, 1e-15)
		})
	}
}

func TestPhysicalEntityWaterCurrentMovementTraces(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1}, Replacement: flowingWater},
	})

	t.Run("ground living", func(t *testing.T) {
		runtime := NewRuntime(world)

		cow := runtime.SpawnCow(game.Position{X: 0.5, Y: 0.5, Z: 0.5})

		configuration := animalGroundControlConfig(cow.Spec)

		runtime.tickGroundMobMovement(cow, &cow.Living, &cow.Rotation, &cow.Navigation, &cow.MoveControl, &cow.LookControl, &cow.BodyControl, configuration, false)

		assertPositionClose(t, cow.State.Position, game.Position{X: 0.514, Y: 0.5, Z: 0.5}, 1e-12)
		assertVelocityClose(t, cow.Living.Velocity, game.Velocity{X: 0.012740000367164613, Y: -0.0784}, 1e-12)

		runtime.tickGroundMobMovement(cow, &cow.Living, &cow.Rotation, &cow.Navigation, &cow.MoveControl, &cow.LookControl, &cow.BodyControl, configuration, false)

		assertPositionClose(t, cow.State.Position, game.Position{X: 0.5407400003671646, Y: 0.4216, Z: 0.5}, 1e-12)
		assertVelocityClose(t, cow.Living.Velocity, game.Velocity{X: 0.024333401035404215, Y: -0.155232}, 1e-12)
	})

	t.Run("primed tnt", func(t *testing.T) {
		runtime := NewRuntime(world)

		tnt := runtime.SpawnTnt(game.Position{X: 0.5, Y: 0.5, Z: 0.5}, game.Velocity{}, 0, false)

		tnt.Tick(runtime, nil)

		assertPositionClose(t, tnt.State.Position, game.Position{X: 0.514, Y: 0.46, Z: 0.5}, 1e-12)
		assertVelocityClose(t, tnt.Velocity, game.Velocity{X: 0.01372, Y: -0.0392}, 1e-12)

		tnt.Tick(runtime, nil)

		assertPositionClose(t, tnt.State.Position, game.Position{X: 0.54172, Y: 0.3808, Z: 0.5}, 1e-12)
		assertVelocityClose(t, tnt.Velocity, game.Velocity{X: 0.0271656, Y: -0.077616}, 1e-12)
	})

	t.Run("flying arrow", func(t *testing.T) {
		runtime := NewRuntime(world)

		arrow := runtime.SpawnArrow(game.Position{X: 0.5, Y: 0.5, Z: 0.5}, game.Velocity{}, 0)

		arrow.Tick(runtime, nil)

		assertPositionClose(t, arrow.State.Position, game.Position{X: 0.514, Y: 0.5, Z: 0.5}, 1e-12)
		assertVelocityClose(t, arrow.Velocity, game.Velocity{X: 0.0084, Y: -0.05}, 1e-12)

		arrow.Tick(runtime, nil)

		assertPositionClose(t, arrow.State.Position, game.Position{X: 0.5364, Y: 0.45, Z: 0.5}, 1e-12)
		assertVelocityClose(t, arrow.Velocity, game.Velocity{X: 0.01344, Y: -0.08}, 1e-12)
	})
}

func TestPhysicalEntityFallingWaterMovementTraces(t *testing.T) {
	fallingWater := mustFluidBlock(t, game.Water, "8")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{Y: 1}, Replacement: fallingWater},
		{Position: game.BlockPosition{X: 1, Y: 1}, Replacement: game.Stone},
	})

	t.Run("ground living", func(t *testing.T) {
		runtime := NewRuntime(world)

		cow := runtime.SpawnCow(game.Position{X: 0.5, Y: 1.5, Z: 0.5})

		configuration := animalGroundControlConfig(cow.Spec)

		runtime.tickGroundMobMovement(cow, &cow.Living, &cow.Rotation, &cow.Navigation, &cow.MoveControl, &cow.LookControl, &cow.BodyControl, configuration, false)

		assertPositionClose(t, cow.State.Position, game.Position{X: 0.5, Y: 1.486, Z: 0.5}, 1e-12)
		assertVelocityClose(t, cow.Living.Velocity, game.Velocity{Y: -0.09212}, 1e-12)

		runtime.tickGroundMobMovement(cow, &cow.Living, &cow.Rotation, &cow.Navigation, &cow.MoveControl, &cow.LookControl, &cow.BodyControl, configuration, false)

		assertPositionClose(t, cow.State.Position, game.Position{X: 0.5, Y: 1.37988, Z: 0.5}, 1e-12)
		assertVelocityClose(t, cow.Living.Velocity, game.Velocity{Y: -0.1823976}, 1e-12)
	})

	t.Run("primed tnt", func(t *testing.T) {
		runtime := NewRuntime(world)

		tnt := runtime.SpawnTnt(game.Position{X: 0.5, Y: 1.5, Z: 0.5}, game.Velocity{}, 0, false)

		tnt.Tick(runtime, nil)

		assertPositionClose(t, tnt.State.Position, game.Position{X: 0.5, Y: 1.446, Z: 0.5}, 1e-12)
		assertVelocityClose(t, tnt.Velocity, game.Velocity{Y: -0.05292}, 1e-12)

		tnt.Tick(runtime, nil)

		assertPositionClose(t, tnt.State.Position, game.Position{X: 0.5, Y: 1.33908, Z: 0.5}, 1e-12)
		assertVelocityClose(t, tnt.Velocity, game.Velocity{Y: -0.1047816}, 1e-12)
	})
}

func TestPhysicalEntityLavaCurrentMovementTraces(t *testing.T) {
	flowingLava := mustFluidBlock(t, game.Lava, "1")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Lava},
		{Position: game.BlockPosition{X: 1}, Replacement: flowingLava},
	})

	t.Run("ground living", func(t *testing.T) {
		runtime := NewRuntime(world)

		cow := runtime.SpawnCow(game.Position{X: 0.5, Y: 0.5, Z: 0.5})

		configuration := animalGroundControlConfig(cow.Spec)

		runtime.tickGroundMobMovement(cow, &cow.Living, &cow.Rotation, &cow.Navigation, &cow.MoveControl, &cow.LookControl, &cow.BodyControl, configuration, false)

		assertPositionClose(t, cow.State.Position, game.Position{X: 0.5045, Y: 0.5, Z: 0.5}, 1e-12)
		assertVelocityClose(t, cow.Living.Velocity, game.Velocity{X: 0.004095000118017196, Y: -0.0784}, 1e-12)
	})

	t.Run("primed tnt", func(t *testing.T) {
		runtime := NewRuntime(world)

		tnt := runtime.SpawnTnt(game.Position{X: 0.5, Y: 0.5, Z: 0.5}, game.Velocity{}, 0, false)

		tnt.Tick(runtime, nil)

		assertPositionClose(t, tnt.State.Position, game.Position{X: 0.5045, Y: 0.46, Z: 0.5}, 1e-12)
		assertVelocityClose(t, tnt.Velocity, game.Velocity{X: 0.00441, Y: -0.0392}, 1e-12)
	})
}

func TestPhysicalEntityFluidCurrentsPauseWithInactiveChunk(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1}, Replacement: flowingWater},
	})

	runtime := NewRuntime(world)

	tnt := runtime.SpawnTnt(game.Position{X: 0.5, Y: 0.5, Z: 0.5}, game.Velocity{}, 0, false)

	runtime.Tick()

	if tnt.State.Position != (game.Position{X: 0.5, Y: 0.5, Z: 0.5}) || tnt.Velocity != (game.Velocity{}) {
		t.Fatalf("inactive TNT changed in current: position %+v velocity %+v", tnt.State.Position, tnt.Velocity)
	}

	viewer := &Session{}

	runtime.setSessionActiveChunks(viewer, []LoadedChunk{{}})

	runtime.Tick()

	assertPositionClose(t, tnt.State.Position, game.Position{X: 0.514, Y: 0.46, Z: 0.5}, 1e-12)
	assertVelocityClose(t, tnt.Velocity, game.Velocity{X: 0.01372, Y: -0.0392}, 1e-12)

	runtime.releaseSessionActiveChunks(viewer)

	position := tnt.State.Position
	velocity := tnt.Velocity

	runtime.Tick()

	if tnt.State.Position != position || tnt.Velocity != velocity {
		t.Fatalf("deactivated TNT changed in current: position %+v velocity %+v", tnt.State.Position, tnt.Velocity)
	}
}

func TestPhysicalEntityFluidCurrentDoesNotAllocateAfterWarmup(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1}, Replacement: flowingWater},
	})

	runtime := NewRuntime(world)

	box := itemEntityBox(game.Position{X: 0.5, Y: 0.5, Z: 0.5})

	velocity := game.Velocity{}

	runtime.applyPhysicalEntityFluidCurrents(box, &velocity)

	allocations := testing.AllocsPerRun(100, func() {
		velocity = game.Velocity{}
		runtime.applyPhysicalEntityFluidCurrents(box, &velocity)
	})

	if allocations != 0 {
		t.Fatalf("physical fluid current allocations = %f, want 0", allocations)
	}
}

func TestEmbeddedArrowIsNotPushedByFluid(t *testing.T) {
	flowingWater := mustFluidBlock(t, game.Water, "1")

	world := fluidTraceWorld([]game.BlockChange{
		{Position: game.BlockPosition{}, Replacement: game.Water},
		{Position: game.BlockPosition{X: 1}, Replacement: flowingWater},
	})

	runtime := NewRuntime(world)

	arrow := runtime.SpawnArrow(game.Position{X: 0.5, Y: 0.5, Z: 0.5}, game.Velocity{}, 0)

	arrow.InGround = true
	arrow.EmbeddedAt = game.BlockPosition{}
	arrow.EmbeddedBlock = game.Water

	arrow.Tick(runtime, nil)

	if arrow.State.Position != (game.Position{X: 0.5, Y: 0.5, Z: 0.5}) || arrow.Velocity != (game.Velocity{}) {
		t.Fatalf("embedded arrow moved with current: position %+v velocity %+v", arrow.State.Position, arrow.Velocity)
	}
}

func fluidTraceWorld(changes []game.BlockChange) *game.World {
	world := &game.World{}

	world.SetBlocks(changes)

	return world
}

func mustFluidBlock(t *testing.T, block game.Block, level string) game.Block {
	t.Helper()

	fluid, valid := block.WithProperties(game.BlockPropertyValue{Name: "level", Value: level})
	if !valid {
		t.Fatalf("resolve fluid level %s", level)
	}

	return fluid
}
