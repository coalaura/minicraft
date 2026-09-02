package server

import (
	"bytes"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type playerLandingTestResult struct {
	player     game.Player
	connection *recordingConnection
}

type playerLandingDamageTest struct {
	name         string
	block        game.Block
	distance     float64
	sneaking     bool
	wantHealth   float32
	wantVelocity float64
}

type playerWaterResetTest struct {
	name  string
	water game.Block
}

func TestPlayerLandingDamageDispatch(t *testing.T) {
	tests := []playerLandingDamageTest{
		{name: "ordinary fractional", block: game.Stone, distance: 4.0001, wantHealth: 18, wantVelocity: 0},
		{name: "ordinary nonlethal", block: game.Stone, distance: 10, wantHealth: 13, wantVelocity: 0},
		{name: "hay boundary", block: game.HayBlock, distance: 8, wantHealth: 19, wantVelocity: 0},
		{name: "hay above boundary", block: game.HayBlock, distance: 8.0001, wantHealth: 18, wantVelocity: 0},
		{name: "honey boundary", block: game.HoneyBlock, distance: 8, wantHealth: 19, wantVelocity: 0},
		{name: "honey above boundary", block: game.HoneyBlock, distance: 8.0001, wantHealth: 18, wantVelocity: 0},
		{name: "bed effective distance", block: game.WhiteBed, distance: 8, wantHealth: 19, wantVelocity: 1.32},
		{name: "bed fractional boundary", block: game.RedBed, distance: 8.0002, wantHealth: 18, wantVelocity: 1.32},
		{name: "bed suppress bounce", block: game.BlueBed, distance: 8, sneaking: true, wantHealth: 19, wantVelocity: 0},
		{name: "slime bounce", block: game.SlimeBlock, distance: 10, wantHealth: 20, wantVelocity: 2},
		{name: "slime suppress bounce", block: game.SlimeBlock, distance: 10, sneaking: true, wantHealth: 20, wantVelocity: 0},
		{name: "powder snow", block: game.PowderSnow, distance: 10, wantHealth: 20, wantVelocity: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runPlayerLandingTest(t, test.block, test.distance, test.sneaking)

			if result.player.Health != test.wantHealth || result.player.FallDistance != 0 {
				t.Fatalf("landing state = health %v distance %v, want health %v distance 0", result.player.Health, result.player.FallDistance, test.wantHealth)
			}

			if result.player.Velocity.Y != test.wantVelocity {
				t.Fatalf("landing velocity = %v, want %v", result.player.Velocity.Y, test.wantVelocity)
			}
		})
	}
}

func TestPlayerLandingRequiresAuthoritativeSupport(t *testing.T) {
	runtime := NewRuntime(&game.World{})
	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	session.Player.ResetSurvivalState()

	session.Player.Position = game.Position{X: 0.5, Y: 10, Z: 0.5}

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y = 1
		player.OnGround = true
	})

	player := session.snapshotPlayer()
	if player.Health != game.DefaultPlayerHealth || player.FallDistance != 9 {
		t.Fatalf("unsupported grounded report = health %v distance %v", player.Health, player.FallDistance)
	}

	runtime.World.SetBlock(game.BlockPosition{}, game.Stone)

	runtime.updatePlayerMovement(session, func(player *game.Player) {})

	player = session.snapshotPlayer()
	if player.Health != 14 || player.FallDistance != 0 {
		t.Fatalf("authoritative landing after unsupported report = health %v distance %v", player.Health, player.FallDistance)
	}
}

func TestPlayerOrdinaryWoolAndCarpetLandingParity(t *testing.T) {
	stone := runPlayerLandingTest(t, game.Stone, 10, false).player
	wool := runPlayerLandingTest(t, game.WhiteWool, 10, false).player
	carpet := runPlayerLandingTest(t, game.WhiteCarpet, 10, false).player

	if stone.Health != 13 || wool.Health != stone.Health || carpet.Health != stone.Health {
		t.Fatalf("ordinary landing health = stone %v wool %v carpet %v", stone.Health, wool.Health, carpet.Health)
	}
}

func TestPlayerLandingResistanceAndLethalFall(t *testing.T) {
	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, game.HayBlock)

	runtime := NewRuntime(world)

	session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	session.Player.ResetSurvivalState()

	session.Player.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectResistance, 100, 0, false, true, true))

	session.Player.Position = game.Position{X: 0.5, Y: 14, Z: 0.5}

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y = 1
		player.OnGround = true
	})

	player := session.snapshotPlayer()
	if player.Health != 18.4 {
		t.Fatalf("resisted hay landing health = %v, want 18.4", player.Health)
	}

	lethal := runPlayerLandingTest(t, game.Stone, 30, false).player
	if !lethal.Dead || lethal.Health != 0 || lethal.FallDistance != 0 {
		t.Fatalf("lethal landing = dead %t health %v distance %v", lethal.Dead, lethal.Health, lethal.FallDistance)
	}
}

func TestPlayerMovementWaterFallResetOrdering(t *testing.T) {
	flowingWater, valid := game.Water.WithProperties(game.BlockPropertyValue{Name: "level", Value: "5"})
	if !valid {
		t.Fatal("resolve flowing water state")
	}

	tests := []playerWaterResetTest{
		{name: "source", water: game.Water},
		{name: "flowing", water: flowingWater},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{}

			world.SetBlock(game.BlockPosition{}, test.water)

			runtime, session, _ := newPlayerFallTestRuntime(t, world)

			session.Player.Position = game.Position{X: 0.5, Y: 2, Z: 0.5}
			session.Player.FallDistance = 7

			runtime.updatePlayerMovement(session, func(player *game.Player) {
				player.Position.Y = 0.1
				player.OnGround = false
			})

			distance := session.snapshotPlayer().FallDistance
			if distance != 0 {
				t.Fatalf("water entry distance = %v, want 0", distance)
			}
		})
	}

	t.Run("crossing with dry endpoint", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 5}, game.Water)

		runtime, session, _ := newPlayerFallTestRuntime(t, world)

		session.Player.Position = game.Position{X: 0.5, Y: 10, Z: 0.5}
		session.Player.FallDistance = 7

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.Y = 3

			player.OnGround = false
		})

		distance := session.snapshotPlayer().FallDistance
		if distance != 7 {
			t.Fatalf("post-water segment distance = %v, want current movement distance 7", distance)
		}
	})

	t.Run("water and grounded before tick", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Stone)
		world.SetBlock(game.BlockPosition{Y: 1}, game.Water)

		runtime, session, _ := newPlayerFallTestRuntime(t, world)

		session.Player.Position = game.Position{X: 0.5, Y: 8, Z: 0.5}
		session.Player.FallDistance = 7

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.Y = 1
			player.OnGround = true
		})

		player := session.snapshotPlayer()
		if player.Health != game.DefaultPlayerHealth || player.FallDistance != 0 {
			t.Fatalf("water landing before tick = health %v distance %v", player.Health, player.FallDistance)
		}
	})

	t.Run("leaving water starts new fall", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 2}, game.Water)

		runtime, session, _ := newPlayerFallTestRuntime(t, world)

		session.Player.Position = game.Position{X: 0.5, Y: 2.1, Z: 0.5}
		session.Player.FallDistance = 7

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.Y = 2
		})

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.Y = -0.5
		})

		distance := session.snapshotPlayer().FallDistance
		if distance != 2.5 {
			t.Fatalf("new fall distance = %v, want 2.5", distance)
		}
	})
}

func TestCanonicalFallDamageResettingBlocks(t *testing.T) {
	blocks := []game.Block{
		game.Cobweb,
		game.Ladder,
		game.Vine,
		game.Scaffolding,
		game.SweetBerryBush,
		game.WeepingVines,
		game.WeepingVinesPlant,
		game.TwistingVines,
		game.TwistingVinesPlant,
		game.CaveVines,
		game.CaveVinesPlant,
	}

	for _, block := range blocks {
		definition, valid := block.Definition()
		if !valid {
			t.Fatalf("invalid canonical reset block %d", block)
		}

		t.Run(definition.Name, func(t *testing.T) {
			if !block.HasTrait(game.BlockTraitFallDamageResetting) {
				t.Fatalf("%s lacks fall reset trait", definition.Name)
			}

			world := &game.World{}

			world.SetBlock(game.BlockPosition{}, block)

			runtime, session, _ := newPlayerFallTestRuntime(t, world)

			session.Player.Position = game.Position{X: 0.5, Y: 2, Z: 0.5}
			session.Player.FallDistance = 7

			runtime.updatePlayerMovement(session, func(player *game.Player) {
				player.Position.Y = 0.5
			})

			distance := session.snapshotPlayer().FallDistance
			if distance != 1.5 {
				t.Fatalf("reset block movement distance = %v, want current movement distance 1.5", distance)
			}
		})
	}

	t.Run("fast crossing", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 5}, game.Cobweb)

		runtime, session, _ := newPlayerFallTestRuntime(t, world)

		session.Player.Position = game.Position{X: 0.5, Y: 10, Z: 0.5}
		session.Player.FallDistance = 7

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.Y = 4
		})

		distance := session.snapshotPlayer().FallDistance
		if distance != 6 {
			t.Fatalf("crossed reset block distance = %v, want current movement distance 6", distance)
		}
	})

	t.Run("adjacent endpoint", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{}, game.Cobweb)

		runtime, session, _ := newPlayerFallTestRuntime(t, world)

		session.Player.Position = game.Position{X: 1.2, Y: 1, Z: 0.5}
		session.Player.FallDistance = 7

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.Y = 0.5
		})

		distance := session.snapshotPlayer().FallDistance
		if distance != 7.5 {
			t.Fatalf("adjacent reset block distance = %v, want 7.5", distance)
		}
	})
}

func TestBedFamilyLandingAndCollision(t *testing.T) {
	for id := game.WhiteBedID; id <= game.BlackBedID; id++ {
		definition, valid := game.BlockByID(id)
		if !valid {
			t.Fatalf("bed block ID %d is invalid", id)
		}

		if !definition.DefaultState.HasTrait(game.BlockTraitBed) || definition.Collision != game.BlockCollisionBed {
			t.Fatalf("bed %s traits/collision = %v/%v", definition.Name, definition.Traits, definition.Collision)
		}
	}

	state, valid := game.WhiteBed.WithProperties(
		game.BlockPropertyValue{Name: "facing", Value: "east"},
		game.BlockPropertyValue{Name: "occupied", Value: "true"},
		game.BlockPropertyValue{Name: "part", Value: "head"},
	)

	if !valid {
		t.Fatal("resolve non-default bed state")
	}

	boxes := state.CollisionBoxes(game.BlockPosition{})
	if len(boxes) != 3 || boxes[0].MaxY != 9.0/16.0 {
		t.Fatalf("bed collision = %+v", boxes)
	}

	result := runPlayerLandingTest(t, state, 8, false)
	if result.player.Health != 19 || result.player.Velocity.Y != 1.32 {
		t.Fatalf("non-default bed landing = health %v velocity %v", result.player.Health, result.player.Velocity.Y)
	}
}

func TestPlayerBounceMotionPacket(t *testing.T) {
	result := runPlayerLandingTest(t, game.SlimeBlock, 10, false)

	packets := result.connection.packets(t)

	var motionPacket protocol.Packet

	found := false

	for _, packet := range packets {
		if packet.ID == protocol.ClientboundSetEntityMotionID {
			motionPacket = packet
			found = true

			break
		}
	}

	if !found {
		t.Fatalf("landing packet IDs = %v, missing motion", result.connection.packetIDs(t))
	}

	var expected protocol.PacketWriter

	protocol.SetEntityMotion{
		EntityID:  result.player.EntityID,
		VelocityX: result.player.Velocity.X,
		VelocityY: result.player.Velocity.Y,
		VelocityZ: result.player.Velocity.Z,
	}.Encode(&expected)

	if !bytes.Equal(motionPacket.Data, expected.Buffer.Bytes()) {
		t.Fatalf("motion payload = %x, want %x", motionPacket.Data, expected.Buffer.Bytes())
	}
}

func runPlayerLandingTest(t *testing.T, block game.Block, distance float64, sneaking bool) playerLandingTestResult {
	t.Helper()

	world := &game.World{}

	world.SetBlock(game.BlockPosition{}, block)

	runtime, session, recording := newPlayerFallTestRuntime(t, world)

	landingY := 0.0

	for _, box := range block.CollisionBoxes(game.BlockPosition{}) {
		landingY = max(landingY, box.MaxY)
	}

	session.Player.Position = game.Position{X: 0.5, Y: landingY + distance, Z: 0.5}
	session.Player.Velocity.Y = -2
	session.Player.Sneaking = sneaking

	recording.reset()

	runtime.updatePlayerMovement(session, func(player *game.Player) {
		player.Position.Y = landingY
		player.OnGround = true
	})

	return playerLandingTestResult{player: session.snapshotPlayer(), connection: recording}
}

func newPlayerFallTestRuntime(t *testing.T, world *game.World) (*Runtime, *Session, *recordingConnection) {
	t.Helper()

	runtime := NewRuntime(world)

	session, connection := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

	joinTestSession(t, runtime, session)

	session.Player.ResetSurvivalState()

	return runtime, session, connection
}
