package server

import (
	"math"
	"slices"
	"testing"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type playerFallDamageTestCase struct {
	name     string
	distance float32
	want     float32
}

var playerArmorDamageGameModes = []game.GameMode{game.GameModeCreative, game.GameModeSpectator}

func TestDamagePlayerHitCooldownAndEligibility(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 4})
	if !applied {
		t.Fatal("initial damage was not applied")
	}

	player := session.snapshotPlayer()
	if player.Health != 16 || player.InvulnerableTime != playerHurtCooldownTicks || player.LastHurt != 4 {
		t.Fatalf("initial damage state = %+v", player)
	}

	applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 4})
	if applied {
		t.Fatal("repeated equal damage was applied during cooldown")
	}

	applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 6})
	if !applied {
		t.Fatal("increased damage was not applied during cooldown")
	}

	player = session.snapshotPlayer()
	if player.Health != 14 || player.LastHurt != 6 {
		t.Fatalf("increased cooldown damage state = %+v", player)
	}

	modes := []game.GameMode{game.GameModeCreative, game.GameModeSpectator}

	for _, mode := range modes {
		session.updatePlayerState(func(player *game.Player) bool {
			player.GameMode = mode
			player.InvulnerableTime = 0

			return true
		})

		applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 1})
		if applied {
			t.Fatalf("%v player took normal damage", mode)
		}

		applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageOutOfWorld, Amount: 1})
		if !applied {
			t.Fatalf("%v player did not take void damage", mode)
		}
	}
}

func TestPlayerArmorDamageSemantics(t *testing.T) {
	t.Run("environmental wear mitigation and unbreaking", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		equipPlayerArmor(session.Player, [4]game.Item{game.ItemIronHelmet, game.ItemIronChestplate, game.ItemIronLeggings, game.ItemIronBoots})

		session.Player.Inventory.Armor[3].SetEnchantment(game.EnchantmentUnbreaking, 3)

		randomCalls := 0

		runtime.miningRandom = func(bound int) int {
			if bound != 20 {
				t.Fatalf("unbreaking random bound = %d, want 20", bound)
			}

			randomCalls++

			if randomCalls == 1 {
				return 5
			}

			return 6
		}

		connection.reset()

		applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageInFire, Amount: 11})
		if !applied {
			t.Fatal("armor-applicable environmental damage was not applied")
		}

		player := session.snapshotPlayer()
		wantHealth := float32(game.DefaultPlayerHealth) - 6.82

		if math.Abs(float64(player.Health-wantHealth)) > 1e-5 {
			t.Fatalf("health after iron armor = %v, want %v", player.Health, wantHealth)
		}

		wantDamage := [4]int32{2, 2, 2, 1}

		for index, stack := range player.Inventory.Armor {
			if stack.Damage() != wantDamage[index] {
				t.Fatalf("armor slot %d damage = %d, want %d", index, stack.Damage(), wantDamage[index])
			}
		}

		if randomCalls != 2 {
			t.Fatalf("unbreaking random calls = %d, want 2", randomCalls)
		}

		if countPacketID(connection.packets(t), protocol.ClientboundContainerSetContentID) != 1 {
			t.Fatalf("armor durability inventory packets = %v", connection.packetIDs(t))
		}
	})

	t.Run("armor bypass", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		equipPlayerArmor(session.Player, [4]game.Item{game.ItemDiamondHelmet, game.ItemDiamondChestplate, game.ItemDiamondLeggings, game.ItemDiamondBoots})

		applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageFall, Amount: 11})
		if !applied {
			t.Fatal("armor-bypassing damage was not applied")
		}

		player := session.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth-11 {
			t.Fatalf("armor-bypassing health = %v, want %v", player.Health, game.DefaultPlayerHealth-11)
		}

		for index, stack := range player.Inventory.Armor {
			if stack.Damage() != 0 {
				t.Fatalf("armor-bypassing damage wore slot %d by %d", index, stack.Damage())
			}
		}
	})

	t.Run("creative and spectator", func(t *testing.T) {
		for _, mode := range playerArmorDamageGameModes {
			runtime := NewRuntime(&game.World{})

			session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

			session.Player.GameMode = mode

			equipPlayerArmor(session.Player, [4]game.Item{game.ItemIronHelmet, game.ItemIronChestplate, game.ItemIronLeggings, game.ItemIronBoots})

			applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamagePlayerAttack, Amount: 4})
			if applied {
				t.Fatalf("%v player took ordinary armor-applicable damage", mode)
			}

			for index, stack := range session.snapshotPlayer().Inventory.Armor {
				if stack.Damage() != 0 {
					t.Fatalf("%v armor slot %d damage = %d, want 0", mode, index, stack.Damage())
				}
			}
		}
	})
}

func TestPlayerArmorBreakSynchronizesActualSlots(t *testing.T) {
	runtime, observer, playerSession, observerConnection, playerConnection := newPlayerCombatTest(t)

	playerSession.updatePlayerState(func(player *game.Player) bool {
		equipPlayerArmor(player, [4]game.Item{game.ItemIronHelmet, game.ItemIronChestplate, game.ItemIronLeggings, game.ItemIronBoots})

		for index := range player.Inventory.Armor {
			stack := &player.Inventory.Armor[index]

			definition, valid := stack.Item.Definition()
			if !valid {
				t.Fatalf("armor slot %d definition is missing", index)
			}

			stack.SetDamage(definition.MaxDurability - 1)
		}

		return true
	})

	observerConnection.reset()
	playerConnection.reset()

	applied := runtime.DamagePlayer(playerSession, PlayerDamage{Type: PlayerDamagePlayerAttack, Amount: 4, CauseEntityID: observer.snapshotPlayer().EntityID})
	if !applied {
		t.Fatal("armor-breaking damage was not applied")
	}

	player := playerSession.snapshotPlayer()

	for index, stack := range player.Inventory.Armor {
		if !stack.Empty() {
			t.Fatalf("broken armor remains in slot %d: %+v", index, stack)
		}
	}

	if player.Health != game.DefaultPlayerHealth-4 {
		t.Fatalf("health after all armor broke = %v, want %v", player.Health, game.DefaultPlayerHealth-4)
	}

	if countPacketID(playerConnection.packets(t), protocol.ClientboundContainerSetContentID) != 1 {
		t.Fatalf("player armor break inventory packets = %v", playerConnection.packetIDs(t))
	}

	if countPacketID(observerConnection.packets(t), protocol.ClientboundEntityEquipmentID) != 1 {
		t.Fatalf("observer armor break equipment packets = %v", observerConnection.packetIDs(t))
	}

	wantEvents := []byte{playerFeetArmorBreakEvent, playerLegsArmorBreakEvent, playerChestArmorBreakEvent, playerHeadArmorBreakEvent}

	assertEntityEvents(t, playerConnection, player.EntityID, wantEvents)
	assertEntityEvents(t, observerConnection, player.EntityID, wantEvents)
}

func equipPlayerArmor(player *game.Player, items [4]game.Item) {
	for index, item := range items {
		player.Inventory.Armor[index] = game.ItemStack{Item: item, Count: 1}
	}
}

func assertEntityEvents(t *testing.T, connection *recordingConnection, entityID int32, expected []byte) {
	t.Helper()

	packets := packetsByID(t, connection, protocol.ClientboundEntityEventID)
	if len(packets) != len(expected) {
		t.Fatalf("entity event packets = %d, want %d; packet IDs %v", len(packets), len(expected), connection.packetIDs(t))
	}

	for index, packet := range packets {
		reader := protocol.NewPacketReader(packet.Data)

		actualEntityID := reader.Int()
		actualEvent := reader.Byte()

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode entity event: %v", err)
		}

		if actualEntityID != entityID || actualEvent != expected[index] {
			t.Fatalf("entity event %d = entity %d event %d, want entity %d event %d", index, actualEntityID, actualEvent, entityID, expected[index])
		}
	}
}

func TestPlayerSurvivalAirDrowningAndEnvironmentalDamage(t *testing.T) {
	t.Run("air recovery and game mode eligibility", func(t *testing.T) {
		world := &game.World{}

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		session.Player.AirSupply = game.DefaultPlayerAirSupply - 4

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.AirSupply != game.DefaultPlayerAirSupply {
			t.Fatalf("recovered air supply = %d, want %d", player.AirSupply, game.DefaultPlayerAirSupply)
		}

		world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

		modes := []game.GameMode{game.GameModeSurvival, game.GameModeCreative, game.GameModeSpectator}

		for _, mode := range modes {

			session.updatePlayerState(func(player *game.Player) bool {
				player.GameMode = mode
				player.AirSupply = 10

				return true
			})

			runtime.Tick()

			player := session.snapshotPlayer()

			want := int32(14)

			if mode == game.GameModeSurvival {
				want = 9
			}

			if player.AirSupply != want {
				t.Fatalf("%v air supply = %d, want %d", mode, player.AirSupply, want)
			}
		}
	})

	t.Run("drowning cadence", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		session.Player.AirSupply = playerDrowningThreshold + 1

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.AirSupply != 0 || player.Health != game.DefaultPlayerHealth-playerDrowningDamage {
			t.Fatalf("first drowning tick = air %d health %v", player.AirSupply, player.Health)
		}

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.AirSupply != -1 || player.Health != game.DefaultPlayerHealth-playerDrowningDamage {
			t.Fatalf("drowning reset cadence = air %d health %v", player.AirSupply, player.Health)
		}
	})

	t.Run("fire and lava", func(t *testing.T) {
		world := &game.World{}

		fireDefinition, valid := game.Fire.Definition()
		if !valid {
			t.Fatal("fire definition is invalid")
		}

		statefulFire := game.Fire + 1
		statefulDefinition, valid := statefulFire.Definition()

		if !valid || statefulDefinition.ID != fireDefinition.ID || statefulFire == game.Fire {
			t.Fatalf("stateful fire = %d, definition %+v", statefulFire, statefulDefinition)
		}

		world.SetBlock(game.BlockPosition{}, statefulFire)

		runtime := NewRuntime(world)

		runtime.entityRandom = func() float32 {
			return 0
		}

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth-1 || player.RemainingFireTicks != playerFireDurationTicks {
			t.Fatalf("fire state = health %v fire %d", player.Health, player.RemainingFireTicks)
		}

		world.SetBlock(game.BlockPosition{}, game.Lava)

		session.updatePlayerState(func(player *game.Player) bool {
			player.InvulnerableTime = 0
			player.LastHurt = 0

			return true
		})

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth-5 || player.RemainingFireTicks != playerLavaFireDurationTicks {
			t.Fatalf("lava state = health %v fire %d", player.Health, player.RemainingFireTicks)
		}
	})

	t.Run("creative fire expires after leaving lava", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Lava)

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		joinTestSession(t, runtime, session)

		session.Player.ResetSurvivalState()

		session.Player.GameMode = game.GameModeCreative

		runtime.Tick()

		player := session.snapshotPlayer()
		if player.RemainingFireTicks != 1 || player.Health != game.DefaultPlayerHealth {
			t.Fatalf("creative lava state = health %v fire %d", player.Health, player.RemainingFireTicks)
		}

		world.SetBlock(game.BlockPosition{}, game.Air)

		runtime.Tick()

		player = session.snapshotPlayer()
		if player.RemainingFireTicks != 0 {
			t.Fatalf("creative fire after leaving lava = %d, want 0", player.RemainingFireTicks)
		}
	})
}

func TestPlayerSurvivalFallAndVoid(t *testing.T) {
	world := &game.World{}
	world.SetBlock(game.BlockPosition{Y: -1}, game.Stone)

	runtime := NewRuntime(world)

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	session.Player.ResetSurvivalState()

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.FallDistance = 10
		player.OnGround = true
	})

	player := session.snapshotPlayer()
	if player.Health != 13 || player.FallDistance != 0 {
		t.Fatalf("fall landing state = health %v distance %v", player.Health, player.FallDistance)
	}

	session.updatePlayerState(func(player *game.Player) bool {
		player.Position.Y = float64(protocol.OverworldMinY-playerVoidDistanceBelowWorld) - 1
		player.InvulnerableTime = 0

		return true
	})

	runtime.Tick()

	player = session.snapshotPlayer()
	if player.Health != 9 {
		t.Fatalf("void damage health = %v, want 9", player.Health)
	}
}

func TestCalculatePlayerFallDamageBoundaries(t *testing.T) {
	tests := []playerFallDamageTestCase{
		{name: "below safe distance", distance: 2.9999, want: 0},
		{name: "safe distance", distance: 3, want: 0},
		{name: "just above safe distance", distance: 3.0001, want: 1},
		{name: "within first interval", distance: 3.75, want: 1},
		{name: "integer boundary", distance: 4, want: 1},
		{name: "above integer boundary", distance: 4.0001, want: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := calculatePlayerFallDamage(test.distance)
			if got != test.want {
				t.Fatalf("damage at %v = %v, want %v", test.distance, got, test.want)
			}
		})
	}
}

func TestPlayerFallDistanceFluidProgression(t *testing.T) {
	world := &game.World{}

	for y := int32(0); y <= 6; y++ {
		world.SetBlock(game.BlockPosition{Y: y}, game.Lava)
	}

	runtime := NewRuntime(world)

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	session.Player.ResetSurvivalState()

	session.Player.Position = game.Position{X: 0.5, Y: 6, Z: 0.5}

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y = 4
		player.OnGround = false
	})

	runtime.Tick()

	distance := session.snapshotPlayer().FallDistance
	if distance != 1 {
		t.Fatalf("first lava fall distance = %v, want 1", distance)
	}

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y = 2
		player.OnGround = false
	})

	runtime.Tick()

	distance = session.snapshotPlayer().FallDistance
	if distance != 1.5 {
		t.Fatalf("second lava fall distance = %v, want 1.5", distance)
	}

	healthBeforeLanding := session.snapshotPlayer().Health

	world.SetBlock(game.BlockPosition{}, game.Stone)

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y = 1
		player.OnGround = true
	})

	player := session.snapshotPlayer()
	if player.FallDistance != 0 || player.Health != healthBeforeLanding {
		t.Fatalf("lava landing = distance %v health %v, want 0 and %v", player.FallDistance, player.Health, healthBeforeLanding)
	}

	world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

	session.updatePlayerState(func(player *game.Player) bool {
		player.FallDistance = 5

		return true
	})

	runtime.Tick()

	distance = session.snapshotPlayer().FallDistance
	if distance != 0 {
		t.Fatalf("water reset fall distance = %v, want 0", distance)
	}

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y -= 0.5
		player.OnGround = false
	})

	distance = session.snapshotPlayer().FallDistance
	if distance != 0 {
		t.Fatalf("water movement fall distance = %v, want 0", distance)
	}
}

func TestPlayerDeathDropsInventoryOnlyOnce(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	runtime.entityRandom = func() float32 {
		return 0
	}

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.ResetSurvivalState()

	session.Player.Inventory.CraftingResult = game.ItemStack{Item: game.ItemStone, Count: 1}
	session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}

	boundHelmet := game.ItemStack{Item: game.ItemIronHelmet, Count: 1}

	boundHelmet.SetEnchantment(game.EnchantmentBindingCurse, 1)

	session.Player.Inventory.Armor[0] = boundHelmet
	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemOakLog, Count: 3}
	session.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemShield, Count: 1}

	applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if !applied {
		t.Fatal("lethal damage was not applied")
	}

	player := session.snapshotPlayer()
	if !player.Dead || !player.Inventory.CraftingResult.Empty() || !player.Inventory.Crafting[0].Empty() || !player.Inventory.Armor[0].Empty() || !player.Inventory.Main[0].Empty() || !player.Inventory.Offhand.Empty() {
		t.Fatalf("death state = %+v", player)
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 4 {
		t.Fatalf("death drops = %d, want 4", len(entities))
	}

	foundBoundHelmet := false

	for _, entity := range entities {
		item := entity.(*runtimeItemEntity)
		if item.Stack.Equal(boundHelmet) {
			foundBoundHelmet = true
		}
	}

	if !foundBoundHelmet {
		t.Fatal("bound equipped helmet did not drop on death")
	}

	applied = runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if applied {
		t.Fatal("dead player accepted repeated lethal damage")
	}

	runtime.Tick()

	if len(runtime.snapshotRuntimeEntities()) != 4 {
		t.Fatal("dead player survival tick duplicated inventory drops")
	}
}

func TestPlayerDeathPreventsVanishingEquipmentDrops(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	session.Player.ResetSurvivalState()

	cursed := game.ItemStack{Item: game.ItemDiamond, Count: 1}

	cursed.SetEnchantment(game.EnchantmentVanishingCurse, 1)

	session.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemOakLog, Count: 1}
	session.Player.Inventory.Main[1] = cursed.Clone()

	session.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemStone, Count: 1}
	session.Player.Inventory.Hotbar[1] = cursed.Clone()

	session.Player.Inventory.Armor[0] = game.ItemStack{Item: game.ItemIronHelmet, Count: 1}
	session.Player.Inventory.Armor[1] = cursed.Clone()

	session.Player.Inventory.Offhand = cursed.Clone()

	applied := runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if !applied {
		t.Fatal("lethal damage was not applied")
	}

	entities := runtime.snapshotRuntimeEntities()
	if len(entities) != 3 {
		t.Fatalf("death drops = %d, want 3 ordinary stacks", len(entities))
	}

	for _, entity := range entities {
		item := entity.(*runtimeItemEntity)
		if item.Stack.PreventsEquipmentDrop() {
			t.Fatalf("vanishing stack spawned on death: %+v", item.Stack)
		}
	}
}

func TestPlayerDeathDropsTransientMenuItems(t *testing.T) {
	t.Run("full inventory cursor", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		session.Player.ResetSurvivalState()

		for index := range session.Player.Inventory.Main {
			session.Player.Inventory.Main[index] = game.ItemStack{Item: game.ItemStone, Count: 64}
		}

		for index := range session.Player.Inventory.Hotbar {
			session.Player.Inventory.Hotbar[index] = game.ItemStack{Item: game.ItemStone, Count: 64}
		}

		container := make([]game.ItemStack, 9)

		container[0] = game.ItemStack{Item: game.ItemDirt, Count: 2}

		menu := newGenericContainerMenu(1, 1, container, &session.Player.Inventory)

		menu.carried = game.ItemStack{Item: game.ItemDiamond, Count: 1}
		menu.carried.SetEnchantment(game.EnchantmentVanishingCurse, 1)

		session.containerMenu = menu

		runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})

		if session.activeMenu() != session.inventoryMenu || !session.activeMenu().carried.Empty() {
			t.Fatal("death retained the open menu or carried stack")
		}

		if !container[0].Equal(game.ItemStack{Item: game.ItemDirt, Count: 2}) {
			t.Fatalf("container backing changed on death: %+v", container[0])
		}

		var carriedDropped bool

		for _, entity := range runtime.snapshotRuntimeEntities() {
			item := entity.(*runtimeItemEntity)
			if item.Stack.Item == game.ItemDiamond {
				carriedDropped = true
			}
		}

		if !carriedDropped {
			t.Fatal("carried stack did not drop on death")
		}
	})

	t.Run("crafting inputs", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		session.Player.ResetSurvivalState()
		session.Player.Inventory.Crafting[0] = game.ItemStack{Item: game.ItemEmerald, Count: 2}

		table := &craftingTableBacking{}

		table.inputs[0] = game.ItemStack{Item: game.ItemDiamond, Count: 1}

		session.containerMenu = newCraftingTableMenu(1, table, &session.Player.Inventory)

		runtime.DamagePlayer(session, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})

		var (
			diamondDropped bool
			emeraldDropped bool
		)

		for _, entity := range runtime.snapshotRuntimeEntities() {
			item := entity.(*runtimeItemEntity)

			switch item.Stack.Item {
			case game.ItemDiamond:
				diamondDropped = item.Stack.Count == 1
			case game.ItemEmerald:
				emeraldDropped = item.Stack.Count == 2
			}
		}

		if !diamondDropped || !emeraldDropped {
			t.Fatalf("crafting death drops: diamond %t, emerald %t", diamondDropped, emeraldDropped)
		}
	})
}

func TestPlayerDeathLifecycle(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	victim, victimConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Victim")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")
	distant, distantConnection := newMovementTestSession(runtime, "20212223-2425-2627-2829-2a2b2c2d2e2f", "Distant")

	distant.Player.Position.X = 1024

	joinTestSession(t, runtime, victim)
	joinTestSession(t, runtime, observer)
	joinTestSession(t, runtime, distant)

	victimConnection.reset()
	observerConnection.reset()
	distantConnection.reset()

	applied := runtime.DamagePlayer(victim, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if !applied {
		t.Fatal("lethal damage was not applied")
	}

	victimPlayer := victim.snapshotPlayer()
	if entityEventCount(t, victimConnection, victimPlayer.EntityID, 3) != 1 || entityEventCount(t, observerConnection, victimPlayer.EntityID, 3) != 1 || entityEventCount(t, distantConnection, victimPlayer.EntityID, 3) != 0 {
		t.Fatalf("initial death events victim=%v observer=%v distant=%v", victimConnection.packetIDs(t), observerConnection.packetIDs(t), distantConnection.packetIDs(t))
	}

	if countPacketID(victimConnection.packets(t), protocol.ClientboundCombatKillID) != 1 {
		t.Fatalf("combat kill packets = %v", victimConnection.packetIDs(t))
	}

	message := game.TranslatableText("death.attack.genericKill", game.LiteralText("Victim"))

	assertSystemComponents(t, victimConnection, message)
	assertSystemComponents(t, observerConnection, message)
	assertSystemComponents(t, distantConnection, message)

	for range playerDeathDuration - 1 {
		runtime.Tick()
	}

	victimPlayer = victim.snapshotPlayer()
	if victimPlayer.DeathTime != playerDeathDuration-1 || victimPlayer.DeathEntityRemoved {
		t.Fatalf("death state before final tick = %+v", victimPlayer)
	}

	if entityEventCount(t, victimConnection, victimPlayer.EntityID, 60) != 0 || entityEventCount(t, observerConnection, victimPlayer.EntityID, 60) != 0 || countPacketID(observerConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 0 {
		t.Fatal("death lifecycle completed before tick 20")
	}

	runtime.Tick()

	victimPlayer = victim.snapshotPlayer()
	if victimPlayer.DeathTime != playerDeathDuration || !victimPlayer.DeathEntityRemoved {
		t.Fatalf("death state after final tick = %+v", victimPlayer)
	}

	if entityEventCount(t, victimConnection, victimPlayer.EntityID, 60) != 1 || entityEventCount(t, observerConnection, victimPlayer.EntityID, 60) != 1 || entityEventCount(t, distantConnection, victimPlayer.EntityID, 60) != 0 {
		t.Fatalf("final death events victim=%v observer=%v distant=%v", victimConnection.packetIDs(t), observerConnection.packetIDs(t), distantConnection.packetIDs(t))
	}

	if countPacketID(observerConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 || countPacketID(victimConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 0 || countPacketID(distantConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 0 {
		t.Fatalf("death removals victim=%v observer=%v distant=%v", victimConnection.packetIDs(t), observerConnection.packetIDs(t), distantConnection.packetIDs(t))
	}

	connections := []*recordingConnection{victimConnection, observerConnection, distantConnection}

	for _, connection := range connections {
		if countPacketID(connection.packets(t), protocol.ClientboundPlayerInfoRemoveID) != 0 {
			t.Fatalf("death sent player info removal: %v", connection.packetIDs(t))
		}

		if connection.isClosed() {
			t.Fatal("death closed an active connection")
		}
	}

	if runtime.PlayerCount() != 3 {
		t.Fatalf("player count after death = %d, want 3", runtime.PlayerCount())
	}

	if playersVisible(observer.snapshotPlayer(), victimPlayer, observer.renderDistance()) {
		t.Fatal("removed corpse remains visible")
	}

	runtime.Tick()

	if entityEventCount(t, observerConnection, victimPlayer.EntityID, 60) != 1 || countPacketID(observerConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 1 {
		t.Fatalf("death lifecycle repeated observer=%v", observerConnection.packetIDs(t))
	}
}

func TestRespawnPlayerResetsAndSynchronizesObservers(t *testing.T) {
	world := &game.World{Spawn: game.Position{X: 4, Y: 80, Z: -2}}

	runtime := NewRuntime(world)

	player, playerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	player.Log = &chatTestLogger{}
	observer.Log = &chatTestLogger{}

	renderDistance := int32(config.MinRenderDistance)

	player.Config = &config.Config{Server: config.ServerConfig{RenderDistance: &renderDistance}}

	joinTestSession(t, runtime, player)
	joinTestSession(t, runtime, observer)

	playerConnection.reset()
	observerConnection.reset()

	player.updatePlayerState(func(current *game.Player) bool {
		current.Dead = true
		current.DeathTime = 5
		current.Health = 0
		current.AirSupply = 0
		current.RemainingFireTicks = 10
		current.Position = game.Position{X: 100, Y: 20}
		current.Rotation = game.Rotation{Yaw: 45}
		current.Velocity = game.Velocity{X: 1}
		current.Sneaking = true
		current.Sprinting = true
		current.Swimming = true
		current.Pose = game.PlayerPoseCrawling

		return true
	})

	player.hasChunkCenter = true
	player.centerChunk = LoadedChunk{Z: -1}
	player.loadedChunks = map[LoadedChunk]struct{}{player.centerChunk: {}}

	err := runtime.RespawnPlayer(player)

	if err != nil {
		t.Fatalf("respawn player: %v", err)
	}

	current := player.snapshotPlayer()
	if current.Dead || current.DeathTime != 0 || current.DeathEntityRemoved || current.Health != game.DefaultPlayerHealth || current.AirSupply != game.DefaultPlayerAirSupply || current.Position != world.Spawn || current.Rotation != (game.Rotation{}) || current.Velocity != (game.Velocity{}) || !current.OnGround || current.Sneaking || current.Sprinting || current.Swimming || current.Pose != game.PlayerPoseStanding {
		t.Fatalf("respawned player = %+v", current)
	}

	playerPacketIDs := playerConnection.packetIDs(t)
	if len(playerPacketIDs) < 7 {
		t.Fatalf("respawn packets = %v, want state and reloaded chunks", playerPacketIDs)
	}

	if playerPacketIDs[0] != protocol.ClientboundRespawnID || playerPacketIDs[1] != protocol.ClientboundSetHealthID || playerPacketIDs[2] != protocol.ClientboundGameEventID || playerPacketIDs[3] != protocol.ClientboundSetCenterChunkID {
		t.Fatalf("respawn packet prefix = %v", playerPacketIDs[:4])
	}

	respawnEvent := protocol.NewPacketReader(playerConnection.packets(t)[2].Data)
	if respawnEvent.Byte() != 13 {
		t.Fatal("respawn did not start client terrain loading")
	}

	assertPacketIDs(t, playerPacketIDs[len(playerPacketIDs)-3:], []int32{
		protocol.ClientboundPlayerPositionID,
		protocol.ClientboundContainerSetContentID,
		protocol.ClientboundEntityMetadataID,
	})

	if !slices.Contains(playerPacketIDs, protocol.ClientboundLevelChunkWithLightID) {
		t.Fatalf("respawn packets = %v, want chunk data", playerPacketIDs)
	}

	assertPacketIDs(t, observerConnection.packetIDs(t), []int32{
		protocol.ClientboundRemoveEntitiesID,
		protocol.ClientboundAddEntityID,
		protocol.ClientboundEntityMetadataID,
	})

	playerConnection.reset()

	err = runtime.RespawnPlayer(player)
	if err != nil {
		t.Fatalf("respawn living player: %v", err)
	}

	ids := playerConnection.packetIDs(t)
	if len(ids) != 0 {
		t.Fatalf("living respawn packets = %v, want none", ids)
	}
}

func TestRespawnPlayerAfterDeathEntityRemovalSynchronizesObservers(t *testing.T) {
	world := &game.World{Spawn: game.Position{X: 4, Y: 80, Z: -2}}

	runtime := NewRuntime(world)

	player, playerConnection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")
	observer, observerConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Observer")

	player.Log = &chatTestLogger{}

	renderDistance := int32(config.MinRenderDistance)

	player.Config = &config.Config{Server: config.ServerConfig{RenderDistance: &renderDistance}}

	joinTestSession(t, runtime, player)
	joinTestSession(t, runtime, observer)

	playerConnection.reset()
	observerConnection.reset()

	applied := runtime.DamagePlayer(player, PlayerDamage{Type: PlayerDamageGenericKill, Amount: math.MaxFloat32})
	if !applied {
		t.Fatal("lethal damage was not applied")
	}

	for range playerDeathDuration {
		runtime.Tick()
	}

	playerConnection.reset()
	observerConnection.reset()

	err := runtime.RespawnPlayer(player)
	if err != nil {
		t.Fatalf("respawn player: %v", err)
	}

	if countPacketID(observerConnection.packets(t), protocol.ClientboundRemoveEntitiesID) != 0 || countPacketID(observerConnection.packets(t), protocol.ClientboundAddEntityID) != 1 {
		t.Fatalf("late respawn observer packets = %v", observerConnection.packetIDs(t))
	}

	current := player.snapshotPlayer()
	if current.Dead || current.DeathTime != 0 || current.DeathEntityRemoved {
		t.Fatalf("late respawn state = %+v", current)
	}
}

func entityEventCount(t *testing.T, connection *recordingConnection, entityID int32, event byte) int {
	t.Helper()

	count := 0

	for _, packet := range packetsByID(t, connection, protocol.ClientboundEntityEventID) {
		reader := protocol.NewPacketReader(packet.Data)

		actualEntityID := reader.Int()
		actualEvent := reader.Byte()

		if actualEntityID == entityID && actualEvent == event {
			count++
		}
	}

	return count
}

func TestKillCommandKillsCreativePlayer(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	player, connection := newCommandTestSession(runtime, commandTestBobUUID, "Bob")

	player.Player.GameMode = game.GameModeCreative

	joinTestSession(t, runtime, player)

	connection.reset()

	executeCommand(t, player, "kill")

	if !player.snapshotPlayer().Dead {
		t.Fatal("kill command did not kill creative player")
	}
}
