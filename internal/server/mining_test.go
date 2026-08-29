package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type miningSpeedTestCase struct {
	item game.Item
	want float64
}

type miningRuleTestCase struct {
	item  game.Item
	block game.Block
	speed float32
}

type baselineBlockDropTestCase struct {
	block game.Block
	item  game.Item
	count int32
}

type doorDropTestCase struct {
	block     game.Block
	wantDrops int
}

type doorMiningTestCase struct {
	target game.BlockPosition
}

func TestBaselineMiningSpeeds(t *testing.T) {
	stone := game.Stone

	hardness := float64(stone.MiningProperties().Hardness)
	tests := map[string]miningSpeedTestCase{
		"hand":      {item: game.ItemAir, want: 1 / hardness / 100},
		"wood":      {item: game.ItemWoodenPickaxe, want: 2 / hardness / 30},
		"copper":    {item: game.ItemCopperPickaxe, want: 5 / hardness / 30},
		"stone":     {item: game.ItemStonePickaxe, want: 4 / hardness / 30},
		"iron":      {item: game.ItemIronPickaxe, want: 6 / hardness / 30},
		"diamond":   {item: game.ItemDiamondPickaxe, want: 8 / hardness / 30},
		"gold":      {item: game.ItemGoldenPickaxe, want: 12 / hardness / 30},
		"netherite": {item: game.ItemNetheritePickaxe, want: 9 / hardness / 30},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			player := game.Player{SelectedHotbarSlot: 0}
			player.Inventory.Hotbar[0] = game.ItemStack{Item: test.item, Count: 1}

			got := destroyProgress(player, stone)
			if math.Abs(got-test.want) > 1e-9 {
				t.Fatalf("destroy progress = %.12f, want %.12f", got, test.want)
			}
		})
	}

	player := game.Player{SelectedHotbarSlot: 0}

	player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemWoodenShovel, Count: 1}

	got := destroyProgress(player, stone)
	want := 1 / hardness / 100

	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("incorrect-tool progress = %.12f, want %.12f", got, want)
	}

	got = destroyProgress(player, game.Bedrock)
	if got != 0 {
		t.Fatalf("bedrock destroy progress = %f, want 0", got)
	}

	if game.ItemAir.IsCorrectToolForDrops(game.Stone) {
		t.Fatal("bare hand is correct for stone drops")
	}

	if !game.ItemWoodenPickaxe.IsCorrectToolForDrops(game.Stone) {
		t.Fatal("wooden pickaxe is not correct for stone drops")
	}

	if game.ItemWoodenPickaxe.IsCorrectToolForDrops(game.Obsidian) {
		t.Fatal("wooden pickaxe is correct for obsidian drops")
	}

	if !game.ItemDiamondPickaxe.IsCorrectToolForDrops(game.Obsidian) {
		t.Fatal("diamond pickaxe is not correct for obsidian drops")
	}
}

func TestSurvivalMiningStopAndDelayedCompletion(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemDiamondPickaxe)

	startMining(t, actor, position, 1)
	stopMining(t, actor, position, 2)

	block := world.BlockAt(position)
	if block != game.Stone {
		t.Fatalf("block after early stop = %d, want stone", block)
	}

	for range 6 {
		runtime.Tick()
	}

	block = world.BlockAt(position)
	if block != game.Air {
		t.Fatalf("block after delayed completion = %d, want air", block)
	}
}

func TestSurvivalMiningAbortAndTargetInvalidation(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemDiamondPickaxe)

	startMining(t, actor, position, 1)

	runtime.abortDestroyingBlock(actor)

	for range 10 {
		runtime.Tick()
	}

	block := world.BlockAt(position)
	if block != game.Stone {
		t.Fatalf("block after abort = %d, want stone", block)
	}

	startMining(t, actor, position, 2)

	world.SetBlock(position, game.Dirt)

	runtime.Tick()

	if actor.mining.active || actor.mining.delayed {
		t.Fatal("mining remained active after target changed")
	}

	world.SetBlock(position, game.Stone)

	startMining(t, actor, position, 3)

	actor.Player.Position.X += 20

	runtime.Tick()

	if actor.mining.active || actor.mining.delayed {
		t.Fatal("mining remained active out of range")
	}
}

func TestMiningCrackProgressAndClearBroadcast(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Stone}}

	runtime := NewRuntime(world)

	actor, actorConnection := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemDiamondPickaxe)
	observer, observerConnection := newMiningTestSession(t, runtime, position, game.GameModeCreative, game.ItemAir)

	actorConnection.reset()
	observerConnection.reset()

	startMining(t, actor, position, 10)

	assertPacketIDs(t, actorConnection.packetIDs(t), []int32{protocol.ClientboundBlockChangedAckID})
	assertMiningCrack(t, observerConnection.packets(t)[0], actor.Player.EntityID, position, 1)

	actor.Runtime.abortDestroyingBlock(actor)

	packets := observerConnection.packets(t)

	assertMiningCrack(t, packets[len(packets)-1], actor.Player.EntityID, position, -1)

	_ = observer
}

func TestSurvivalMiningDropsOrdinaryLoot(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Dirt}}

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemAir)

	startMining(t, actor, position, 1)

	for range 10 {
		runtime.Tick()
	}

	stopMining(t, actor, position, 2)

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 1 {
		t.Fatalf("ordinary drops = %d, want 1", len(entities))
	}

	drop := entities[0].(*runtimeItemEntity)
	if drop.Stack.Item != game.ItemDirt || drop.Stack.Count != 1 || drop.PickupDelay != 10 || drop.Velocity.Y != 0.2 || drop.Velocity.X < -0.1 || drop.Velocity.X >= 0.1 || drop.Velocity.Z < -0.1 || drop.Velocity.Z >= 0.1 {
		t.Fatalf("ordinary drop = %+v", drop)
	}
}

func TestGeneratedBaselineBlockDrops(t *testing.T) {
	tests := map[string]baselineBlockDropTestCase{
		"stone":        {block: game.Stone, item: game.ItemCobblestone, count: 1},
		"deepslate":    {block: game.Deepslate, item: game.ItemCobbledDeepslate, count: 1},
		"stone bricks": {block: game.StoneBricks, item: game.ItemStoneBricks, count: 1},
		"spruce log":   {block: game.SpruceLog, item: game.ItemSpruceLog, count: 1},
		"iron ore":     {block: game.IronOre, item: game.ItemRawIron, count: 1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			record := blockMutationRecord{
				change:      game.BlockChange{Position: game.BlockPosition{Y: 70}, Replacement: game.Air},
				previous:    test.block,
				lootContext: blockLootPlayer,
			}

			runtime.commitOrdinaryBlockDrops([]blockMutationRecord{record})

			entities := runtime.snapshotRuntimeEntities()
			if len(entities) != 1 {
				t.Fatalf("ordinary drops = %d, want 1", len(entities))
			}

			drop := entities[0].(*runtimeItemEntity)
			if drop.Stack.Item != test.item || drop.Stack.Count != test.count {
				t.Fatalf("ordinary drop = %+v, want item %d count %d", drop.Stack, test.item, test.count)
			}
		})
	}
}

func TestGeneratedStateCountBlockDrops(t *testing.T) {
	tests := map[string]baselineBlockDropTestCase{}

	for count := 1; count <= 4; count++ {
		value := blockPropertyValue(count)

		candle := mustBlockState(t, game.Candle, game.BlockPropertyValue{Name: "candles", Value: value})
		redCandle := mustBlockState(t, game.RedCandle, game.BlockPropertyValue{Name: "candles", Value: value})
		seaPickle := mustBlockState(t, game.SeaPickle, game.BlockPropertyValue{Name: "pickles", Value: value})

		tests["candle "+value] = baselineBlockDropTestCase{block: candle, item: game.ItemCandle, count: int32(count)}
		tests["red candle "+value] = baselineBlockDropTestCase{block: redCandle, item: game.ItemRedCandle, count: int32(count)}
		tests["sea pickle "+value] = baselineBlockDropTestCase{block: seaPickle, item: game.ItemSeaPickle, count: int32(count)}
	}

	for count := 1; count <= 8; count++ {
		value := blockPropertyValue(count)
		snow := mustBlockState(t, game.Snow, game.BlockPropertyValue{Name: "layers", Value: value})

		tests["snow "+value] = baselineBlockDropTestCase{block: snow, item: game.ItemSnowball, count: int32(count)}
	}

	bottomSlab := mustBlockState(t, game.StoneSlab, game.BlockPropertyValue{Name: "type", Value: "bottom"})
	doubleSlab := mustBlockState(t, game.StoneSlab, game.BlockPropertyValue{Name: "type", Value: "double"})

	tests["single slab"] = baselineBlockDropTestCase{block: bottomSlab, item: game.ItemStoneSlab, count: 1}
	tests["double slab"] = baselineBlockDropTestCase{block: doubleSlab, item: game.ItemStoneSlab, count: 2}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})
			record := blockMutationRecord{
				change:      game.BlockChange{Position: game.BlockPosition{Y: 70}, Replacement: game.Air},
				previous:    test.block,
				lootContext: blockLootPlayer,
			}

			runtime.commitOrdinaryBlockDrops([]blockMutationRecord{record})

			entities := runtime.snapshotRuntimeEntities()
			if len(entities) != 1 {
				t.Fatalf("ordinary drops = %d, want 1", len(entities))
			}

			drop := entities[0].(*runtimeItemEntity)
			if drop.Stack.Item != test.item || drop.Stack.Count != test.count {
				t.Fatalf("ordinary drop = %+v, want item %d count %d", drop.Stack, test.item, test.count)
			}
		})
	}
}

func TestSnowDropRequiresPlayerLootContext(t *testing.T) {
	snow := mustBlockState(t, game.Snow, game.BlockPropertyValue{Name: "layers", Value: "4"})

	runtime := NewRuntime(&game.World{})

	record := blockMutationRecord{
		change:      game.BlockChange{Position: game.BlockPosition{Y: 70}, Replacement: game.Air},
		previous:    snow,
		lootContext: blockLootNoBreaker,
	}

	runtime.commitOrdinaryBlockDrops([]blockMutationRecord{record})

	if len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("snow dropped without a player loot entity")
	}
}

func TestGeneratedSwordAndShearsMiningRules(t *testing.T) {
	tests := map[string]miningRuleTestCase{
		"sword cobweb":         {item: game.ItemIronSword, block: game.Cobweb, speed: 15},
		"sword bamboo":         {item: game.ItemWoodenSword, block: game.Bamboo, speed: math.MaxFloat32},
		"sword leaves":         {item: game.ItemDiamondSword, block: game.OakLeaves, speed: 1.5},
		"shears cobweb":        {item: game.ItemShears, block: game.Cobweb, speed: 15},
		"shears leaves":        {item: game.ItemShears, block: game.OakLeaves, speed: 15},
		"shears wool":          {item: game.ItemShears, block: game.WhiteWool, speed: 5},
		"shears vine":          {item: game.ItemShears, block: game.Vine, speed: 2},
		"shears glow lichen":   {item: game.ItemShears, block: game.GlowLichen, speed: 2},
		"shears default speed": {item: game.ItemShears, block: game.Stone, speed: 1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			speed := test.item.BaseDestroySpeed(test.block)
			if speed != test.speed {
				t.Fatalf("destroy speed = %g, want %g", speed, test.speed)
			}
		})
	}

	if !game.ItemIronSword.IsCorrectToolForDrops(game.Cobweb) {
		t.Fatal("sword is not correct for cobweb drops")
	}

	if !game.ItemShears.IsCorrectToolForDrops(game.Cobweb) {
		t.Fatal("shears are not correct for cobweb drops")
	}
}

func TestDoorDropUsesMinedBlockState(t *testing.T) {
	lower, valid := game.OakDoor.WithProperties(game.BlockPropertyValue{Name: "half", Value: "lower"})
	if !valid {
		t.Fatal("lower door state is invalid")
	}

	upper, valid := game.OakDoor.WithProperties(game.BlockPropertyValue{Name: "half", Value: "upper"})
	if !valid {
		t.Fatal("upper door state is invalid")
	}

	tests := map[string]doorDropTestCase{
		"lower": {block: lower, wantDrops: 1},
		"upper": {block: upper, wantDrops: 0},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			record := blockMutationRecord{
				change:      game.BlockChange{Position: game.BlockPosition{Y: 70}, Replacement: game.Air},
				previous:    test.block,
				lootContext: blockLootPlayer,
			}

			runtime.commitOrdinaryBlockDrops([]blockMutationRecord{record})

			entities := runtime.snapshotRuntimeEntities()
			if len(entities) != test.wantDrops {
				t.Fatalf("ordinary drops = %d, want %d", len(entities), test.wantDrops)
			}

			if test.wantDrops == 1 {
				drop := entities[0].(*runtimeItemEntity)
				if drop.Stack.Item != game.ItemOakDoor || drop.Stack.Count != 1 {
					t.Fatalf("ordinary drop = %+v, want one oak door", drop.Stack)
				}
			}
		})
	}
}

func TestMiningEitherDoorHalfDropsOneDoor(t *testing.T) {
	lowerPosition := game.BlockPosition{Y: 70}
	upperPosition := game.BlockPosition{Y: 71}

	lower, valid := game.OakDoor.WithProperties(game.BlockPropertyValue{Name: "half", Value: "lower"})
	if !valid {
		t.Fatal("lower door state is invalid")
	}

	upper, valid := game.OakDoor.WithProperties(game.BlockPropertyValue{Name: "half", Value: "upper"})
	if !valid {
		t.Fatal("upper door state is invalid")
	}

	tests := map[string]doorMiningTestCase{
		"lower": {target: lowerPosition},
		"upper": {target: upperPosition},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlock(game.BlockPosition{Y: 69}, game.Stone)
			world.SetBlock(lowerPosition, lower)
			world.SetBlock(upperPosition, upper)

			runtime := NewRuntime(world)

			actor, _ := newMiningTestSession(t, runtime, test.target, game.GameModeSurvival, game.ItemDiamondAxe)

			startMining(t, actor, test.target, 1)

			for range 10 {
				runtime.Tick()
			}

			stopMining(t, actor, test.target, 2)

			if world.BlockAt(lowerPosition) != game.Air || world.BlockAt(upperPosition) != game.Air {
				t.Fatal("door remained after mining")
			}

			entities := runtime.snapshotRuntimeEntities()
			if len(entities) != 1 {
				t.Fatalf("ordinary drops = %d, want 1", len(entities))
			}

			drop := entities[0].(*runtimeItemEntity)
			if drop.Stack.Item != game.ItemOakDoor || drop.Stack.Count != 1 {
				t.Fatalf("ordinary drop = %+v, want one oak door", drop.Stack)
			}
		})
	}
}

func TestNetheritePickaxeMinesAndDropsStoneBricks(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	handWorld := &game.World{Generator: blockMutationTestGenerator{block: game.StoneBricks}}

	handRuntime := NewRuntime(handWorld)

	handActor, _ := newMiningTestSession(t, handRuntime, position, game.GameModeSurvival, game.ItemAir)

	startMining(t, handActor, position, 1)

	for range 3 {
		handRuntime.Tick()
	}

	stopMining(t, handActor, position, 2)

	block := handWorld.BlockAt(position)
	if block != game.StoneBricks {
		t.Fatalf("hand mined stone bricks in four mining units")
	}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.StoneBricks}}

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemNetheritePickaxe)

	startMining(t, actor, position, 3)

	for range 3 {
		runtime.Tick()
	}

	stopMining(t, actor, position, 4)

	block = world.BlockAt(position)
	if block != game.Air {
		t.Fatalf("block after four mining units = %d, want air", block)
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 1 {
		t.Fatalf("ordinary drops = %d, want 1", len(entities))
	}

	drop := entities[0].(*runtimeItemEntity)
	if drop.Stack.Item != game.ItemStoneBricks || drop.Stack.Count != 1 {
		t.Fatalf("ordinary drop = %+v, want one stone bricks", drop.Stack)
	}
}

func TestCreativeAndCommandBreakingDoNotDropOrdinaryLoot(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Dirt}}

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeCreative, game.ItemAir)

	startMining(t, actor, position, 1)

	if len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("creative break produced ordinary loot")
	}

	world.SetBlock(position, game.Dirt)

	result, err := runtime.MutateWorldBlocks([]game.BlockChange{{Position: position, Replacement: game.Air}})
	if err != nil || !result.Changed {
		t.Fatalf("command-style replacement = %+v, %v", result, err)
	}

	if len(runtime.snapshotRuntimeEntities()) != 0 {
		t.Fatal("command-style replacement produced ordinary loot")
	}
}

func TestSurvivalChestDropsBlockAndContents(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: generatedRemovalChest{position: position, count: 5}}

	runtime := NewRuntime(world)

	actor, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemDiamondAxe)

	startMining(t, actor, position, 1)

	for range 6 {
		runtime.Tick()
	}

	stopMining(t, actor, position, 2)

	counts := map[game.Item]int32{}
	delays := map[game.Item]int32{}

	for _, entity := range runtime.snapshotRuntimeEntities() {
		drop := entity.(*runtimeItemEntity)
		counts[drop.Stack.Item] += drop.Stack.Count
		delays[drop.Stack.Item] = drop.PickupDelay
	}

	if counts[game.ItemChest] != 1 || delays[game.ItemChest] != 10 {
		t.Fatalf("chest block drop = count %d, delay %d", counts[game.ItemChest], delays[game.ItemChest])
	}

	if counts[game.ItemDiamond] != 5 || delays[game.ItemDiamond] != 0 {
		t.Fatalf("chest contents drop = count %d, delay %d", counts[game.ItemDiamond], delays[game.ItemDiamond])
	}
}

func newMiningTestSession(t *testing.T, runtime *Runtime, position game.BlockPosition, mode game.GameMode, item game.Item) (*Session, *recordingConnection) {
	t.Helper()

	session, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Miner", mode)

	session.Player.Position = blockMutationTestPlayerPosition(position)
	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: item, Count: 1}

	markChunkLoaded(session, position)

	joinTestSession(t, runtime, session)

	return session, connection
}

func startMining(t *testing.T, session *Session, position game.BlockPosition, sequence int32) {
	t.Helper()

	err := session.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionStartDestroyBlock, Position: position, Sequence: sequence})
	if err != nil {
		t.Fatalf("start mining: %v", err)
	}
}

func stopMining(t *testing.T, session *Session, position game.BlockPosition, sequence int32) {
	t.Helper()

	err := session.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionStopDestroyBlock, Position: position, Sequence: sequence})
	if err != nil {
		t.Fatalf("stop mining: %v", err)
	}
}

func assertMiningCrack(t *testing.T, packet protocol.Packet, entityID int32, position game.BlockPosition, stage int8) {
	t.Helper()

	if packet.ID != protocol.ClientboundBlockDestructionID {
		t.Fatalf("packet id = %#x, want block destruction", packet.ID)
	}

	reader := protocol.NewPacketReader(packet.Data)

	got := reader.VarInt()
	if got != entityID {
		t.Fatalf("breaker entity id = %d, want %d", got, entityID)
	}

	actualPosition := reader.BlockPosition()
	if actualPosition != position {
		t.Fatalf("crack position = %+v, want %+v", actualPosition, position)
	}

	actualStage := int8(reader.Byte())
	if actualStage != stage {
		t.Fatalf("crack stage = %d, want %d", actualStage, stage)
	}
}
