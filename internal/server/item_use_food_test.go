package server

import (
	"math"
	"slices"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestFoodUseCompletesAfter32MainHandTicksAndSynchronizesLivingFlags(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, actorConnection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)
	observer, observerConnection := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)

	position := toBlockPosition(actor.Player.Position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	actor.Player.FoodLevel = 19
	actor.Player.Saturation = 19
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 2}

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 1})
	if err != nil {
		t.Fatalf("start apple use: %v", err)
	}

	player := actor.snapshotPlayer()
	if !player.UsingItem || player.UsingOffhand || player.UseRemainingTicks != 32 {
		t.Fatalf("started food use = %+v", player)
	}

	metadata := foodUseMetadataPacket(t, observerConnection)

	assertLivingFlagsMetadata(t, metadata, actor.Player.EntityID, protocol.LivingFlagUsingItem)

	actorConnection.reset()
	observerConnection.reset()

	runtime.Tick()

	if slices.Contains(observerConnection.packetIDs(t), protocol.ClientboundEntityMetadataID) {
		t.Fatalf("food countdown packets = %v, want no entity metadata", observerConnection.packetIDs(t))
	}

	tickFoodUse(runtime, 30)

	player = actor.snapshotPlayer()
	if !player.UsingItem || player.UseRemainingTicks != 1 || player.Inventory.Hotbar[0].Count != 2 {
		t.Fatalf("food use after 31 ticks = %+v", player)
	}

	observerConnection.reset()

	runtime.Tick()

	player = actor.snapshotPlayer()
	if player.UsingItem || player.Inventory.Hotbar[0].Count != 1 || player.FoodLevel != game.DefaultPlayerFoodLevel || player.Saturation != float32(game.DefaultPlayerFoodLevel) {
		t.Fatalf("completed apple use = %+v", player)
	}

	metadata = foodUseMetadataPacket(t, observerConnection)

	assertLivingFlagsMetadata(t, metadata, actor.Player.EntityID, 0)
	assertFoodCompletionSounds(t, observerConnection, game.SoundEntityGenericEat)
}

func TestFoodUseDriedKelpCompletesAfter16OffhandTicks(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Player.FoodLevel = 10
	actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemDriedKelp, Count: 1}

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 1})
	if err != nil {
		t.Fatalf("start dried kelp use: %v", err)
	}

	tickFoodUse(runtime, 15)

	player := actor.snapshotPlayer()
	if !player.UsingItem || !player.UsingOffhand || player.UseRemainingTicks != 1 || player.Inventory.Offhand.Count != 1 {
		t.Fatalf("dried kelp use after 15 ticks = %+v", player)
	}

	runtime.Tick()

	player = actor.snapshotPlayer()
	if player.UsingItem || !player.Inventory.Offhand.Empty() || player.FoodLevel != 11 {
		t.Fatalf("completed dried kelp use = %+v", player)
	}
}

func TestFoodUseEligibilityAndCreativePreservation(t *testing.T) {
	modes := []game.GameMode{game.GameModeSurvival, game.GameModeCreative, game.GameModeSpectator}
	foods := []game.Item{game.ItemApple, game.ItemHoneyBottle}
	foodLevels := []int32{10, game.DefaultPlayerFoodLevel}

	for _, mode := range modes {
		for _, food := range foods {
			for _, foodLevel := range foodLevels {
				want := mode != game.GameModeSpectator && (mode == game.GameModeCreative || food == game.ItemHoneyBottle || foodLevel < game.DefaultPlayerFoodLevel)

				assertFoodUseEligibility(t, mode, food, foodLevel, want)
			}
		}
	}

	t.Run("creative preserves food", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeCreative)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 1})
		if err != nil {
			t.Fatalf("use apple in creative: %v", err)
		}

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()
		if player.UsingItem || !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemApple, Count: 1}) {
			t.Fatalf("creative food completion = %+v", player)
		}
	})
}

func TestFoodUseContinuesWithAuthoritativeSameItemStack(t *testing.T) {
	t.Run("count increases", func(t *testing.T) {
		assertFoodUseStackContinuation(t, game.ItemStack{Item: game.ItemApple, Count: 2}, game.ItemStack{Item: game.ItemApple, Count: 3}, game.ItemStack{Item: game.ItemApple, Count: 2})
	})

	t.Run("count decreases while nonempty", func(t *testing.T) {
		assertFoodUseStackContinuation(t, game.ItemStack{Item: game.ItemApple, Count: 2}, game.ItemStack{Item: game.ItemApple, Count: 1}, game.ItemStack{})
	})

	t.Run("component patch changes", func(t *testing.T) {
		current := game.ItemStack{Item: game.ItemApple, Count: 2, RemovedComponents: []int32{game.ItemComponentEnchantments}}
		expected := game.ItemStack{Item: game.ItemApple, Count: 1, RemovedComponents: []int32{game.ItemComponentEnchantments}}

		assertFoodUseStackContinuation(t, game.ItemStack{Item: game.ItemApple, Count: 2}, current, expected)
	})

	t.Run("backing stack is replaced", func(t *testing.T) {
		initial := game.ItemStack{Item: game.ItemApple, Count: 2, Components: []game.ItemComponent{{Type: game.ItemComponentEnchantments, Data: []byte{1}}}}
		current := game.ItemStack{Item: game.ItemApple, Count: 2}
		expected := game.ItemStack{Item: game.ItemApple, Count: 1}

		assertFoodUseStackContinuation(t, initial, current, expected)
	})
}

func TestFoodUseSelectedSlotTransitionsMatchActiveHand(t *testing.T) {
	t.Run("main hand cancels even for same item", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}
		actor.Player.Inventory.Hotbar[1] = game.ItemStack{Item: game.ItemApple, Count: 1}

		startFoodUse(t, actor)

		actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})

		if actor.snapshotPlayer().UsingItem {
			t.Fatal("main-hand food use continued across selected-slot change")
		}
	})

	t.Run("main hand cancels for another item", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}
		actor.Player.Inventory.Hotbar[1] = game.ItemStack{Item: game.ItemStone, Count: 1}

		startFoodUse(t, actor)

		actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})

		if actor.snapshotPlayer().UsingItem {
			t.Fatal("main-hand food use continued after selecting another item")
		}
	})

	t.Run("offhand continues", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[1] = game.ItemStack{Item: game.ItemStone, Count: 1}
		actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemApple, Count: 1}

		err := actor.handleUseItem(protocol.UseItem{Hand: protocol.OffHand, Sequence: 1})
		if err != nil {
			t.Fatalf("start offhand food use: %v", err)
		}

		actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()
		if player.UsingItem || !player.Inventory.Offhand.Empty() || player.FoodLevel != 14 {
			t.Fatalf("offhand food use after selected-slot change = %+v", player)
		}
	})
}

func TestFoodUseProgressAndCompletionSoundTicks(t *testing.T) {
	assertFoodUseSoundTicks(t, game.ItemApple, 10, []int{9, 13, 17, 21, 25, 29}, 32)
	assertFoodUseSoundTicks(t, game.ItemDriedKelp, 10, []int{5, 9, 13}, 16)
	assertFoodUseSoundTicks(t, game.ItemHoneyBottle, game.DefaultPlayerFoodLevel, []int{13, 17, 21, 25, 29, 33, 37}, 40)
}

func TestFoodUseCancelsBeforeCompletion(t *testing.T) {
	t.Run("explicit release", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}

		startFoodUse(t, actor)

		err := actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionReleaseUseItem, Sequence: 2})
		if err != nil {
			t.Fatalf("release food use: %v", err)
		}

		tickFoodUse(runtime, 32)
		assertFoodUseCancelled(t, actor, 10, game.ItemApple)
	})

	t.Run("held slot change", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}
		actor.Player.Inventory.Hotbar[1] = game.ItemStack{Item: game.ItemStone, Count: 1}

		startFoodUse(t, actor)

		actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})

		tickFoodUse(runtime, 32)
		assertFoodUseCancelled(t, actor, 10, game.ItemApple)
	})

	t.Run("held stack mutation", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}

		startFoodUse(t, actor)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemDriedKelp, Count: 1}

		runtime.Tick()

		player := actor.snapshotPlayer()
		if player.UsingItem || player.FoodLevel != 10 || player.Inventory.Hotbar[0].Item != game.ItemDriedKelp {
			t.Fatalf("mutated held stack completion = %+v", player)
		}
	})

	t.Run("active hand becomes empty", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}

		startFoodUse(t, actor)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{}

		runtime.Tick()

		player := actor.snapshotPlayer()
		if player.UsingItem || player.FoodLevel != 10 || !player.Inventory.Hotbar[0].Empty() {
			t.Fatalf("empty active hand cancellation = %+v", player)
		}
	})
}

func TestFoodUseBowlRemainderInventoryAndDrop(t *testing.T) {
	t.Run("room in inventory", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10
		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemMushroomStew, Count: 2}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()
		if !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemMushroomStew, Count: 1}) || foodUseInventoryItemCount(player.Inventory, game.ItemBowl) != 1 || len(runtime.snapshotRuntimeEntities()) != 0 {
			t.Fatalf("stew remainder with inventory room = %+v", player.Inventory)
		}
	})

	t.Run("full inventory drops remainder", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

		actor.Player.FoodLevel = 10

		fillFoodUseInventory(actor.Player)

		actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemMushroomStew, Count: 2}

		startFoodUse(t, actor)

		tickFoodUse(runtime, 32)

		player := actor.snapshotPlayer()
		if !player.Inventory.Hotbar[0].Equal(game.ItemStack{Item: game.ItemMushroomStew, Count: 1}) || countDroppedItem(runtime, game.ItemBowl) != 1 {
			t.Fatalf("stew remainder with full inventory = %+v", player.Inventory)
		}
	})
}

func TestFoodUseCancelsOnDeathAndRespawnResetsState(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Log = &chatTestLogger{}

	renderDistance := int32(config.MinRenderDistance)

	actor.Config = &config.Config{Server: config.ServerConfig{RenderDistance: &renderDistance}}

	actor.Player.FoodLevel = 10
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemApple, Count: 1}

	startFoodUse(t, actor)

	applied := runtime.DamagePlayer(actor, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if !applied {
		t.Fatal("lethal damage was not applied")
	}

	player := actor.snapshotPlayer()
	if !player.Dead || player.UsingItem {
		t.Fatalf("food use after death = %+v", player)
	}

	err := runtime.RespawnPlayer(actor)
	if err != nil {
		t.Fatalf("respawn player: %v", err)
	}

	player = actor.snapshotPlayer()
	if player.UsingItem || player.UsingOffhand || player.UseRemainingTicks != 0 || player.UseAnimation != game.ItemUseAnimationNone || !player.UseStack.Empty() {
		t.Fatalf("food use state after respawn = %+v", player)
	}
}

func newFoodUseTestSession(t *testing.T, runtime *Runtime, name string, mode game.GameMode) (*Session, *recordingConnection) {
	t.Helper()

	uuid := "00010203-0405-0607-0809-0a0b0c0d0e0f"

	if name == "Observer" {
		uuid = "10111213-1415-1617-1819-1a1b1c1d1e1f"
	}

	session, connection := newBlockMutationTestSession(runtime, uuid, name, mode)

	session.Player.ResetSurvivalState()

	joinTestSession(t, runtime, session)

	connection.reset()

	return session, connection
}

func startFoodUse(t *testing.T, session *Session) {
	t.Helper()

	err := session.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 1})
	if err != nil {
		t.Fatalf("start food use: %v", err)
	}

	if !session.snapshotPlayer().UsingItem {
		t.Fatal("food use did not start")
	}
}

func tickFoodUse(runtime *Runtime, ticks int) {
	for range ticks {
		runtime.Tick()
	}
}

func assertFoodUseEligibility(t *testing.T, mode game.GameMode, food game.Item, foodLevel int32, want bool) {
	t.Helper()

	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", mode)

	actor.Player.FoodLevel = foodLevel
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: food, Count: 1}

	err := actor.handleUseItem(protocol.UseItem{Hand: protocol.MainHand, Sequence: 1})
	if err != nil {
		t.Fatalf("use food %d in mode %d at food %d: %v", food, mode, foodLevel, err)
	}

	usingItem := actor.snapshotPlayer().UsingItem
	if usingItem != want {
		t.Fatalf("food %d use in mode %d at food %d = %t, want %t", food, mode, foodLevel, usingItem, want)
	}
}

func assertFoodUseStackContinuation(t *testing.T, initial, current, expected game.ItemStack) {
	t.Helper()

	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)

	actor.Player.FoodLevel = 10
	actor.Player.Inventory.Hotbar[0] = initial

	startFoodUse(t, actor)

	tickFoodUse(runtime, 4)

	actor.Player.Inventory.Hotbar[0] = current

	runtime.Tick()

	player := actor.snapshotPlayer()
	if !player.UsingItem || !player.UseStack.Equal(current) {
		t.Fatalf("same-item continuation = %+v, want use stack %+v", player, current)
	}

	tickFoodUse(runtime, int(player.UseRemainingTicks))

	player = actor.snapshotPlayer()
	if player.UsingItem || !player.Inventory.Hotbar[0].Equal(expected) || player.FoodLevel != 14 {
		t.Fatalf("same-item completion = %+v, want held %+v", player, expected)
	}
}

func assertFoodUseSoundTicks(t *testing.T, item game.Item, foodLevel int32, progressTicks []int, completionTick int) {
	t.Helper()

	runtime := NewRuntime(&game.World{})

	actor, _ := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)
	observer, observerConnection := newFoodUseTestSession(t, runtime, "Observer", game.GameModeSurvival)

	position := toBlockPosition(actor.Player.Position)

	markChunkLoaded(actor, position)
	markChunkLoaded(observer, position)

	actor.Player.FoodLevel = foodLevel
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: item, Count: 1}

	startFoodUse(t, actor)

	observerConnection.reset()

	actualProgressTicks := make([]int, 0, len(progressTicks))

	for tick := 1; tick <= completionTick; tick++ {
		runtime.Tick()

		soundCount := len(packetsByID(t, observerConnection, protocol.ClientboundSoundID))

		observerConnection.reset()

		if tick == completionTick {
			if soundCount != 3 {
				t.Fatalf("item %d completion sounds at tick %d = %d, want 3", item, tick, soundCount)
			}

			continue
		}

		if soundCount > 0 {
			if soundCount != 1 {
				t.Fatalf("item %d progress sounds at tick %d = %d, want 1", item, tick, soundCount)
			}

			actualProgressTicks = append(actualProgressTicks, tick)
		}
	}

	if !slices.Equal(actualProgressTicks, progressTicks) {
		t.Fatalf("item %d progress sound ticks = %v, want %v", item, actualProgressTicks, progressTicks)
	}
}

func assertFoodUseCancelled(t *testing.T, session *Session, foodLevel int32, item game.Item) {
	t.Helper()

	player := session.snapshotPlayer()
	if player.UsingItem || player.FoodLevel != foodLevel || player.Inventory.Hotbar[0].Item != item || player.Inventory.Hotbar[0].Count != 1 {
		t.Fatalf("cancelled food use = %+v", player)
	}
}

func assertLivingFlagsMetadata(t *testing.T, packet protocol.Packet, entityID int32, expected byte) {
	t.Helper()

	if packet.ID != protocol.ClientboundEntityMetadataID {
		t.Fatalf("packet id = %#x, want entity metadata", packet.ID)
	}

	reader := protocol.NewPacketReader(packet.Data)

	actualEntityID := reader.VarInt()
	if actualEntityID != entityID {
		t.Fatalf("metadata entity id = %d, want %d", actualEntityID, entityID)
	}

	reader.Byte()
	reader.VarInt()
	reader.Byte()
	reader.Byte()
	reader.VarInt()
	reader.VarInt()
	reader.Byte()
	reader.VarInt()

	livingFlags := reader.Byte()
	if livingFlags != expected {
		t.Fatalf("living flags = %#x, want %#x", livingFlags, expected)
	}

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode living flags metadata: %v", err)
	}
}

func foodUseMetadataPacket(t *testing.T, connection *recordingConnection) protocol.Packet {
	t.Helper()

	for _, packet := range connection.packets(t) {
		if packet.ID == protocol.ClientboundEntityMetadataID {
			return packet
		}
	}

	t.Fatal("missing entity metadata packet")

	return protocol.Packet{}
}

func assertFoodCompletionSounds(t *testing.T, connection *recordingConnection, consumeEvent game.SoundEvent) {
	t.Helper()

	events := []game.SoundEvent{consumeEvent, consumeEvent, game.SoundEntityPlayerBurp}
	sources := []int32{protocol.SoundSourcePlayer, protocol.SoundSourceNeutral, protocol.SoundSourcePlayer}

	index := 0

	for _, packet := range connection.packets(t) {
		if packet.ID != protocol.ClientboundSoundID {
			continue
		}

		if index >= len(events) {
			t.Fatalf("food completion sent more than %d sounds", len(events))
		}

		reader := protocol.NewPacketReader(packet.Data)

		reader.VarInt()

		event := reader.String(32767)
		hasFixedRange := reader.Bool()

		if hasFixedRange {
			reader.Float()
		}

		source := reader.VarInt()

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode food completion sound: %v", err)
		}

		if event != string(events[index]) || source != sources[index] {
			t.Fatalf("food completion sound %d = %q source %d, want %q source %d", index, event, source, events[index], sources[index])
		}

		index++
	}

	if index != len(events) {
		t.Fatalf("food completion sounds = %d, want %d", index, len(events))
	}
}

func fillFoodUseInventory(player *game.Player) {
	for slot := range player.Inventory.Main {
		player.Inventory.Main[slot] = game.ItemStack{Item: game.ItemStone, Count: 64}
	}

	for slot := range player.Inventory.Hotbar {
		player.Inventory.Hotbar[slot] = game.ItemStack{Item: game.ItemStone, Count: 64}
	}
}

func foodUseInventoryItemCount(inventory game.PlayerInventory, item game.Item) int32 {
	var count int32

	for _, stack := range inventory.Main {
		if stack.Item == item {
			count += stack.Count
		}
	}

	for _, stack := range inventory.Hotbar {
		if stack.Item == item {
			count += stack.Count
		}
	}

	if inventory.Offhand.Item == item {
		count += inventory.Offhand.Count
	}

	return count
}
