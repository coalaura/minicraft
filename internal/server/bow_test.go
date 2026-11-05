package server

import (
	"math"
	"reflect"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type bowPowerTest struct {
	ticks uint32
	want  float32
}

type bowAmmoTest struct {
	name      string
	creative  bool
	infinity  bool
	ammo      game.Item
	wantUse   bool
	wantCount int32
}

type bowCancelTest struct {
	name   string
	cancel func(*testing.T, *Session)
}

func TestBowPower(t *testing.T) {
	tests := []bowPowerTest{{ticks: 0, want: 0}, {ticks: 1, want: 0.034166668}, {ticks: 2, want: 0.07}, {ticks: 3, want: 0.1075}, {ticks: 10, want: 0.41666666}, {ticks: 20, want: 1}, {ticks: 100, want: 1}, {ticks: 72000, want: 1}}

	for _, test := range tests {
		actual := bowPower(test.ticks)
		if math.Abs(float64(actual-test.want)) > 1e-7 {
			t.Fatalf("power(%d) = %v, want %v", test.ticks, actual, test.want)
		}
	}
}

func TestBowHandsDrawReleaseAndSpawn(t *testing.T) {
	hands := []int32{protocol.MainHand, protocol.OffHand}

	for _, hand := range hands {
		actor, connection := newBowTestSession(t)

		if hand == protocol.OffHand {
			actor.Player.Inventory.Offhand, actor.Player.Inventory.Hotbar[0] = actor.Player.Inventory.Hotbar[0], actor.Player.Inventory.Offhand
		}

		startBowTestUse(t, actor, hand)
		flags := byte(protocol.LivingFlagUsingItem)

		if hand == protocol.OffHand {
			flags |= protocol.LivingFlagUsingOffhand
		}

		assertLivingFlagsMetadata(t, foodUseMetadataPacket(t, connection), actor.Player.EntityID, flags)

		player := actor.snapshotPlayer()
		if !player.UsingItem || player.UsingOffhand != (hand == protocol.OffHand) || player.UseRemainingTicks != 72000 || player.UseAnimation != game.ItemUseAnimationBow {
			t.Fatalf("draw state hand %d = %+v", hand, player)
		}

		tickFoodUse(actor.Runtime, 20)
		connection.reset()
		releaseBowTestUse(t, actor)

		arrow := onlyBowTestArrow(t, actor.Runtime)

		player = actor.snapshotPlayer()
		bow, _ := heldItemPointer(&player, hand)

		if player.UsingItem || player.UseRemainingTicks != 0 || bow.Damage() != 1 || player.Inventory.Main[0].Count != 1 {
			t.Fatalf("release transaction = bow %+v, ammo %+v, using %t", bow, player.Inventory.Main[0], player.UsingItem)
		}

		assertVelocityClose(t, arrow.Velocity, game.Velocity{Z: 3}, 1e-9)
		position := player.EyePosition()
		position.Y -= float64(float32(0.1))
		assertPositionClose(t, arrow.State.Position, position, 1e-9)

		if arrow.OwnerID != player.EntityID || !arrow.Critical || arrow.Pickup != arrowPickupAllowed || arrow.PickupStack.Count != 1 {
			t.Fatalf("arrow initial state = %+v", arrow)
		}

		packets := connection.packets(t)
		if len(packets) < 2 || packets[0].ID != protocol.ClientboundAddEntityID || packets[1].ID != protocol.ClientboundEntityMetadataID {
			t.Fatalf("release packets = %v", connection.packetIDs(t))
		}

		spawn := arrow.AddEntityPacket(runtimeEntitySpawnSnapshot{ID: arrow.State.ID, UUID: arrow.State.UUID, Position: position, Velocity: arrow.Velocity})
		if spawn.Data != player.EntityID || spawn.Type != int32(game.EntityArrow) || spawn.VelocityZ != 3 {
			t.Fatalf("spawn = %+v", spawn)
		}

		sounds := packetsByID(t, connection, protocol.ClientboundSoundID)
		if len(sounds) != 1 {
			t.Fatalf("shot sounds = %d", len(sounds))
		}

		reader := protocol.NewPacketReader(sounds[0].Data)
		reader.VarInt()
		event := reader.String(32767)
		reader.Bool()
		source := reader.VarInt()
		reader.Int()
		reader.Int()
		reader.Int()

		volume := reader.Float()
		pitch := reader.Float()
		expectedPitch := float32(0.5)
		expectedPitch = 1/(expectedPitch*0.4+1.2) + 0.5

		if reader.Err() != nil || event != "minecraft:entity.arrow.shoot" || source != protocol.SoundSourcePlayer || volume != 1 || pitch != expectedPitch {
			t.Fatalf("shot sound event=%q source=%d volume=%v pitch=%v error=%v", event, source, volume, pitch, reader.Err())
		}

		connection.reset()
		releaseBowTestUse(t, actor)
		assertPacketIDs(t, connection.packetIDs(t), []int32{protocol.ClientboundBlockChangedAckID})

		if onlyBowTestArrow(t, actor.Runtime) != arrow || !reflect.DeepEqual(actor.snapshotPlayer().Inventory, player.Inventory) {
			t.Fatal("duplicate release changed the completed transaction")
		}
	}
}

func TestBowDurationAndRepeatedStart(t *testing.T) {
	actor, _ := newBowTestSession(t)
	startBowTestUse(t, actor, protocol.MainHand)
	tickFoodUse(actor.Runtime, 20)
	startBowTestUse(t, actor, protocol.MainHand)

	if actor.Player.UseRemainingTicks != 71980 {
		t.Fatalf("repeated start reset or overflowed draw: %d", actor.Player.UseRemainingTicks)
	}

	actor.Player.UseRemainingTicks = 1
	before := actor.Player.Inventory.Clone()
	tickFoodUse(actor.Runtime, 1)
	releaseBowTestUse(t, actor)
	assertNoBowTestArrow(t, actor.Runtime)

	if actor.Player.UsingItem || !reflect.DeepEqual(actor.Player.Inventory, before) {
		t.Fatal("expired use fired or changed inventory")
	}
}

func TestBowAmmoRequirements(t *testing.T) {
	tests := []bowAmmoTest{
		{name: "survival empty"},
		{name: "infinity empty", infinity: true},
		{name: "survival arrows", ammo: game.ItemArrow, wantUse: true, wantCount: 1},
		{name: "infinity arrows", infinity: true, ammo: game.ItemArrow, wantUse: true, wantCount: 2},
		{name: "creative empty", creative: true, wantUse: true},
		{name: "creative arrows", creative: true, ammo: game.ItemArrow, wantUse: true, wantCount: 2},
		{name: "spectral unsupported", ammo: game.ItemSpectralArrow, wantCount: 2},
		{name: "tipped unsupported", infinity: true, ammo: game.ItemTippedArrow, wantCount: 2},
		{name: "creative special unsupported", creative: true, ammo: game.ItemSpectralArrow, wantCount: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actor, _ := newBowTestSession(t)
			actor.Player.Inventory.Main[0] = game.ItemStack{}

			if test.ammo != 0 {
				actor.Player.Inventory.Main[0] = game.ItemStack{Item: test.ammo, Count: 2}
			}

			if test.creative {
				actor.Player.GameMode = game.GameModeCreative
			}

			if test.infinity {
				actor.Player.Inventory.Hotbar[0].SetEnchantment(game.EnchantmentInfinity, 1)
			}

			startBowTestUse(t, actor, protocol.MainHand)

			if actor.Player.UsingItem != test.wantUse {
				t.Fatalf("using = %t, want %t", actor.Player.UsingItem, test.wantUse)
			}

			tickFoodUse(actor.Runtime, 20)
			releaseBowTestUse(t, actor)

			player := actor.snapshotPlayer()
			wantDamage := int32(0)

			if test.wantUse && !test.creative {
				wantDamage = 1
			}

			if player.Inventory.Main[0].Count != test.wantCount || player.Inventory.Hotbar[0].Damage() != wantDamage {
				t.Fatalf("inventory = %+v", player.Inventory)
			}

			if test.wantUse {
				arrow := onlyBowTestArrow(t, actor.Runtime)
				if (arrow.Pickup == arrowPickupCreativeOnly) != (test.creative || test.infinity) {
					t.Fatalf("pickup = %v", arrow.Pickup)
				}
			} else {
				assertNoBowTestArrow(t, actor.Runtime)
			}
		})
	}
}

func TestBowAmmoSelectionOrder(t *testing.T) {
	player := game.Player{}
	player.SelectedHotbarSlot = 4
	player.Inventory.Offhand = game.ItemStack{Item: game.ItemSpectralArrow, Count: 1}
	player.Inventory.Hotbar[4] = game.ItemStack{Item: game.ItemTippedArrow, Count: 1}
	player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemArrow, Count: 1}
	player.Inventory.Main[0] = game.ItemStack{Item: game.ItemArrow, Count: 1}
	player.Inventory.Main[1] = game.ItemStack{Item: game.ItemArrow, Count: 1}

	expected := []*game.ItemStack{&player.Inventory.Offhand, &player.Inventory.Hotbar[4], &player.Inventory.Hotbar[0], &player.Inventory.Main[0], &player.Inventory.Main[1]}

	for _, stack := range expected {
		if bowAmmunition(&player) != stack {
			t.Fatal("wrong ammunition priority")
		}

		*stack = game.ItemStack{}
	}

	if bowAmmunition(&player) != nil {
		t.Fatal("empty inventory supplied ammo")
	}
}

func TestBowShortReleaseAndAmmoRevalidation(t *testing.T) {
	t.Run("short", func(t *testing.T) {
		actor, _ := newBowTestSession(t)
		before := actor.Player.Inventory.Clone()
		startBowTestUse(t, actor, protocol.MainHand)
		tickFoodUse(actor.Runtime, 2)
		releaseBowTestUse(t, actor)
		assertNoBowTestArrow(t, actor.Runtime)

		if !reflect.DeepEqual(actor.Player.Inventory, before) || actor.Player.UsingItem {
			t.Fatal("short release changed inventory or left use active")
		}
	})

	items := []game.Item{0, game.ItemSpectralArrow, game.ItemArrow}

	for _, item := range items {
		actor, _ := newBowTestSession(t)
		startBowTestUse(t, actor, protocol.MainHand)
		tickFoodUse(actor.Runtime, 20)
		actor.Player.Inventory.Main[0] = game.ItemStack{}
		actor.Player.Inventory.Offhand = game.ItemStack{Item: item, Count: 1}
		releaseBowTestUse(t, actor)

		if item == game.ItemArrow {
			onlyBowTestArrow(t, actor.Runtime)

			if !actor.Player.Inventory.Offhand.Empty() || actor.Player.Inventory.Hotbar[0].Damage() != 1 {
				t.Fatal("release did not consume replacement ammo")
			}
		} else {
			assertNoBowTestArrow(t, actor.Runtime)

			if actor.Player.Inventory.Hotbar[0].Damage() != 0 {
				t.Fatal("failed release damaged bow")
			}
		}
	}
}

func TestBowCancellation(t *testing.T) {
	tests := []bowCancelTest{
		{
			name: "slot",
			cancel: func(t *testing.T, actor *Session) {
				actor.handleSetHeldItem(protocol.SetHeldItem{Slot: 1})
			},
		},
		{
			name: "swap",
			cancel: func(t *testing.T, actor *Session) {
				actor.handleSwapWithOffhand()
			},
		},
		{
			name: "replace",
			cancel: func(t *testing.T, actor *Session) {
				before := actor.Player.Inventory.Clone()
				actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBow, Count: 1}
				actor.Player.Inventory.Hotbar[0].SetDamage(5)

				err := actor.synchronizePlayerInventoryMutation(before)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "remove",
			cancel: func(t *testing.T, actor *Session) {
				actor.Player.Inventory.Hotbar[0] = game.ItemStack{}
				tickFoodUse(actor.Runtime, 1)
			},
		},
		{
			name: "gamemode",
			cancel: func(t *testing.T, actor *Session) {
				_, err := actor.Runtime.ChangeGameMode(actor, game.GameModeCreative)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "death",
			cancel: func(t *testing.T, actor *Session) {
				actor.Runtime.DamagePlayer(actor, game.Damage{Type: game.DamageGeneric, Amount: 100})
			},
		},
		{
			name: "respawn",
			cancel: func(t *testing.T, actor *Session) {
				actor.Runtime.DamagePlayer(actor, game.Damage{Type: game.DamageGeneric, Amount: 100})

				err := actor.Runtime.RespawnPlayer(actor)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "disconnect",
			cancel: func(t *testing.T, actor *Session) {
				actor.Runtime.LeaveSession(actor)
			},
		},
		{
			name: "other use",
			cancel: func(t *testing.T, actor *Session) {
				actor.Player.FoodLevel = 10
				actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemApple, Count: 1}
				startBowTestUse(t, actor, protocol.OffHand)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actor, _ := newBowTestSession(t)
			startBowTestUse(t, actor, protocol.MainHand)
			tickFoodUse(actor.Runtime, 20)
			test.cancel(t, actor)

			if actor.Player.UseStack.Item == game.ItemBow || actor.Player.UseAnimation == game.ItemUseAnimationBow {
				t.Fatal("cancelled bow remains active")
			}

			before := actor.Player.Inventory.Clone()
			releaseBowTestUse(t, actor)
			releaseBowTestUse(t, actor)
			assertNoBowTestArrow(t, actor.Runtime)

			if !reflect.DeepEqual(actor.Player.Inventory, before) || actor.Player.UsingItem {
				t.Fatal("cancelled release changed inventory or use state")
			}
		})
	}
}

func TestBowUnbreakingAndBreakEvent(t *testing.T) {
	rolls := []int{0, 1}

	for _, roll := range rolls {
		actor, connection := newBowTestSession(t)
		definition, _ := game.ItemBow.Definition()
		actor.Player.Inventory.Offhand = game.ItemStack{Item: game.ItemBow, Count: 1}
		actor.Player.Inventory.Offhand.SetDamage(definition.MaxDurability - 1)
		actor.Player.Inventory.Offhand.SetEnchantment(game.EnchantmentUnbreaking, 3)

		actor.Runtime.miningRandom = func(bound int) int {
			if bound != 4 {
				t.Fatalf("Unbreaking random bound = %d", bound)
			}

			return roll
		}

		startBowTestUse(t, actor, protocol.OffHand)
		tickFoodUse(actor.Runtime, 20)
		connection.reset()
		releaseBowTestUse(t, actor)
		onlyBowTestArrow(t, actor.Runtime)

		broken := actor.Player.Inventory.Offhand.Empty()
		if broken != (roll == 0) {
			t.Fatalf("broken = %t for roll %d", broken, roll)
		}

		foundEvent := false

		for _, packet := range connection.packets(t) {
			if packet.ID == protocol.ClientboundEntityEventID && len(packet.Data) == 5 && packet.Data[4] == 48 {
				foundEvent = true
			}
		}

		if foundEvent != broken {
			t.Fatal("offhand break event does not match durability transaction")
		}
	}
}

func newBowTestSession(t *testing.T) (*Session, *recordingConnection) {
	t.Helper()
	runtime := NewRuntime(&game.World{})

	runtime.entityRandom = func() float32 {
		return 0.5
	}

	actor, connection := newFoodUseTestSession(t, runtime, "Actor", game.GameModeSurvival)
	actor.Player.Inventory.Hotbar[0] = game.ItemStack{Item: game.ItemBow, Count: 1}
	actor.Player.Inventory.Main[0] = game.ItemStack{Item: game.ItemArrow, Count: 2}
	actor.loadedChunks = map[LoadedChunk]struct{}{{}: {}}

	return actor, connection
}

func startBowTestUse(t *testing.T, actor *Session, hand int32) {
	t.Helper()

	err := actor.handleUseItem(protocol.UseItem{Hand: hand})
	if err != nil {
		t.Fatal(err)
	}
}

func releaseBowTestUse(t *testing.T, actor *Session) {
	t.Helper()

	err := actor.handlePlayerAction(protocol.PlayerAction{Status: protocol.PlayerActionReleaseUseItem})
	if err != nil {
		t.Fatal(err)
	}
}

func onlyBowTestArrow(t *testing.T, runtime *Runtime) *runtimeArrowEntity {
	t.Helper()

	runtime.entityMu.RLock()
	defer runtime.entityMu.RUnlock()

	var found *runtimeArrowEntity

	for _, entity := range runtime.entities {
		if arrow, valid := entity.(*runtimeArrowEntity); valid {
			if found != nil {
				t.Fatal("multiple arrows")
			}

			found = arrow
		}
	}

	if found == nil {
		t.Fatal("no arrow")
	}

	return found
}

func assertNoBowTestArrow(t *testing.T, runtime *Runtime) {
	t.Helper()

	runtime.entityMu.RLock()
	defer runtime.entityMu.RUnlock()

	for _, entity := range runtime.entities {
		if _, valid := entity.(*runtimeArrowEntity); valid {
			t.Fatal("unexpected arrow")
		}
	}
}
