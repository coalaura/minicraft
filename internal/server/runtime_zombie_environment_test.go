package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type zombieFallDamageTestCase struct {
	name       string
	height     float64
	wantDamage float32
}

type zombieDaylightTestCase struct {
	name        string
	dayTime     int64
	lighting    game.LightingMode
	covered     bool
	random      float32
	wantBurning bool
}

func TestZombieFallDistanceAccumulatesAndInactiveChunkPausesEnvironment(t *testing.T) {
	world := &game.World{}

	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	session, _ := newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 10, Z: 0.5})

	zombie.Living.Velocity.Y = -1
	zombie.Living.RemainingFireTicks = 40

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	runtime.Tick()

	if zombie.Living.FallDistance != 1 || zombie.Living.RemainingFireTicks != 39 {
		t.Fatalf("active environment = fall %v fire %d, want 1 and 39", zombie.Living.FallDistance, zombie.Living.RemainingFireTicks)
	}

	position := zombie.State.Position
	velocity := zombie.Living.Velocity
	fallDistance := zombie.Living.FallDistance
	fireTicks := zombie.Living.RemainingFireTicks

	runtime.releaseSessionActiveChunks(session)

	runtime.Tick()

	if zombie.State.Position != position || zombie.Living.Velocity != velocity || zombie.Living.FallDistance != fallDistance || zombie.Living.RemainingFireTicks != fireTicks {
		t.Fatal("inactive zombie advanced movement or environmental state")
	}
}

func TestZombieFallDamageUsesLivingFormulaAndResetsOnLanding(t *testing.T) {
	tests := []zombieFallDamageTestCase{
		{name: "safe three block fall", height: 3, wantDamage: 0},
		{name: "four block fall", height: 4, wantDamage: 1},
		{name: "six block fall", height: 6, wantDamage: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := zombieGroundWorld(-1, 1, -1, 1)

			world.SetDayTime(18000)

			runtime := NewRuntime(world)

			newZombieTestSession(t, runtime, game.Position{X: 100})

			runtime.entityRandom = func() float32 {
				return 0.9
			}

			zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: test.height, Z: 0.5})

			tickZombieUntilGrounded(t, runtime, zombie)

			if zombie.Living.Health != zombieMaxHealth-test.wantDamage || zombie.Living.FallDistance != 0 {
				t.Fatalf("landed zombie = health %v fall %v, want health %v fall 0", zombie.Living.Health, zombie.Living.FallDistance, zombieMaxHealth-test.wantDamage)
			}
		})
	}
}

func TestZombieLethalFallUsesNormalDeathLifecycle(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	newZombieTestSession(t, runtime, game.Position{X: 100})

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Y: 5, Z: 0.5})

	zombie.Living.Health = 1

	for !zombie.Living.Dead {
		runtime.Tick()
	}

	if !zombie.LootDropped || zombie.Living.DeathTime != 1 {
		t.Fatalf("lethal fall = loot %v death time %d, want true and 1", zombie.LootDropped, zombie.Living.DeathTime)
	}

	for !zombie.State.Removed {
		runtime.Tick()
	}

	if zombie.Living.DeathTime != game.LivingDeathDurationTicks {
		t.Fatalf("removed zombie death time = %d, want %d", zombie.Living.DeathTime, game.LivingDeathDurationTicks)
	}
}

func TestZombieFireContactBurnCadenceAndMetadata(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetBlock(game.BlockPosition{}, game.Fire)
	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	_, connection := newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	connection.reset()

	runtime.Tick()

	if math.Abs(float64(zombie.Living.Health-19.06)) > 1e-4 || zombie.Living.RemainingFireTicks != zombieFireDurationTicks || zombie.Living.EntityFlags() != protocol.EntityFlagOnFire {
		t.Fatalf("fire contact = health %v ticks %d flags %#x", zombie.Living.Health, zombie.Living.RemainingFireTicks, zombie.Living.EntityFlags())
	}

	metadata := packetsByID(t, connection, protocol.ClientboundEntityMetadataID)
	if len(metadata) == 0 || zombieMetadataFlags(t, metadata[len(metadata)-1]) != protocol.EntityFlagOnFire {
		t.Fatal("fire contact did not synchronize on-fire metadata")
	}

	world.SetBlock(game.BlockPosition{}, game.Air)

	zombie.Living.Health = 20
	zombie.Living.InvulnerableTime = 0
	zombie.Living.RemainingFireTicks = 20

	connection.reset()

	runtime.Tick()

	if zombie.Living.Health != 19 || zombie.Living.RemainingFireTicks != 19 {
		t.Fatalf("burn cadence = health %v ticks %d, want 19 and 19", zombie.Living.Health, zombie.Living.RemainingFireTicks)
	}
}

func TestZombieWaterExtinguishesAndClearsMetadata(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetBlock(game.BlockPosition{}, game.Water)
	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	_, connection := newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.RemainingFireTicks = 40
	zombie.Living.FallDistance = 5

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	connection.reset()

	runtime.Tick()

	if zombie.Living.RemainingFireTicks != 0 || zombie.Living.FallDistance != 0 || zombie.Living.EntityFlags() != 0 {
		t.Fatalf("water state = fire %d fall %v flags %#x", zombie.Living.RemainingFireTicks, zombie.Living.FallDistance, zombie.Living.EntityFlags())
	}

	metadata := packetsByID(t, connection, protocol.ClientboundEntityMetadataID)
	if len(metadata) == 0 || zombieMetadataFlags(t, metadata[len(metadata)-1]) != 0 {
		t.Fatal("water extinguishing did not synchronize cleared fire metadata")
	}
}

func TestZombieLavaIgnitesDamagesAndPlaysBurnSound(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetBlock(game.BlockPosition{}, game.Lava)
	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	_, connection := newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.FallDistance = 8

	runtime.entityRandom = func() float32 {
		return 0.5
	}

	connection.reset()

	runtime.Tick()

	if math.Abs(float64(zombie.Living.Health-16.06)) > 1e-4 || zombie.Living.RemainingFireTicks != zombieLavaFireDurationTicks || zombie.Living.FallDistance != 0 {
		t.Fatalf("lava contact = health %v fire %d fall %v", zombie.Living.Health, zombie.Living.RemainingFireTicks, zombie.Living.FallDistance)
	}

	assertZombieSound(t, packetsByID(t, connection, protocol.ClientboundSoundID), game.SoundEntityGenericBurn, 0.4, 2.2)
}

func TestZombieFireResistanceBlocksEnvironmentalFireDamage(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetBlock(game.BlockPosition{}, game.Fire)
	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.ActiveEffects.Add(game.NewMobEffectInstance(game.MobEffectFireResistance, 100, 0, false, true, true))

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	runtime.Tick()

	if zombie.Living.Health != 20 || zombie.Living.RemainingFireTicks != zombieFireDurationTicks {
		t.Fatalf("fire resistant zombie = health %v fire %d", zombie.Living.Health, zombie.Living.RemainingFireTicks)
	}
}

func TestZombieEnvironmentalDeathDropsLootOnce(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetBlock(game.BlockPosition{}, game.Fire)
	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.Health = 0.5

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	for range 5 {
		runtime.Tick()
	}

	entities := runtime.snapshotRuntimeEntities()
	if !zombie.Living.Dead || !zombie.LootDropped || len(entities) != 2 {
		t.Fatalf("environmental death = dead %v loot %v entities %d", zombie.Living.Dead, zombie.LootDropped, len(entities))
	}
}

func TestZombieDaylightBurningConditions(t *testing.T) {
	tests := []zombieDaylightTestCase{
		{name: "exposed normal daylight", dayTime: 6000, lighting: game.LightingNormal, random: 0, wantBurning: true},
		{name: "covered normal daylight", dayTime: 6000, lighting: game.LightingNormal, covered: true, random: 0, wantBurning: false},
		{name: "night", dayTime: 18000, lighting: game.LightingNormal, random: 0, wantBurning: false},
		{name: "rejected random branch", dayTime: 6000, lighting: game.LightingNormal, random: 0.9, wantBurning: false},
		{name: "exposed fullbright", dayTime: 6000, lighting: game.LightingFullbright, random: 0, wantBurning: true},
		{name: "covered fullbright", dayTime: 6000, lighting: game.LightingFullbright, covered: true, random: 0, wantBurning: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			world := zombieGroundWorld(-1, 1, -1, 1)

			world.SetLightingMode(test.lighting)
			world.SetDayTime(test.dayTime)

			if test.covered {
				world.SetBlock(game.BlockPosition{Y: 2}, game.Stone)
			}

			runtime := NewRuntime(world)

			newZombieTestSession(t, runtime, game.Position{X: 100})

			runtime.entityRandom = func() float32 {
				return test.random
			}

			zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

			runtime.Tick()

			burning := zombie.Living.RemainingFireTicks > 0
			if burning != test.wantBurning {
				t.Fatalf("daylight burning = %v, want %v", burning, test.wantBurning)
			}
		})
	}
}

func TestZombieAmbientHurtAndDeathSounds(t *testing.T) {
	world := zombieGroundWorld(-1, 1, -1, 1)

	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	_, connection := newZombieTestSession(t, runtime, game.Position{X: 100})

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	runtime.entityRandom = func() float32 {
		return 0
	}

	zombie.AmbientSoundTime = 0

	connection.reset()

	zombie.tickAmbientSound(runtime)

	if sessionHasPacket(connection.packets(t), protocol.ClientboundSoundID) {
		t.Fatal("rejected ambient random tick emitted a sound")
	}

	zombie.AmbientSoundTime = 1

	zombie.tickAmbientSound(runtime)

	assertZombieSound(t, packetsByID(t, connection, protocol.ClientboundSoundID), game.SoundEntityZombieAmbient, 1, 1)

	connection.reset()

	update, applied := runtime.damageRuntimeLivingEntityLocked(zombie, game.Damage{Type: game.DamageInFire, Amount: 2})
	if !applied {
		t.Fatal("initial hurt was rejected")
	}

	runtime.sendRuntimeLivingDamageUpdate(update)

	assertZombieSound(t, packetsByID(t, connection, protocol.ClientboundSoundID), game.SoundEntityZombieHurt, 1, 1)

	connection.reset()

	_, applied = runtime.damageRuntimeLivingEntityLocked(zombie, game.Damage{Type: game.DamageInFire, Amount: 1})
	if applied || sessionHasPacket(connection.packets(t), protocol.ClientboundSoundID) {
		t.Fatal("rejected i-frame hit emitted a hurt sound")
	}

	zombie.Living.InvulnerableTime = 0
	zombie.Living.Health = 1

	update, applied = runtime.damageRuntimeLivingEntityLocked(zombie, game.Damage{Type: game.DamageGenericKill, Amount: 1})
	if !applied || !update.died {
		t.Fatal("lethal damage was rejected")
	}

	runtime.sendRuntimeLivingDamageUpdate(update)

	assertZombieSound(t, packetsByID(t, connection, protocol.ClientboundSoundID), game.SoundEntityZombieDeath, 1, 1)

	if zombie.Living.DeathTime != 0 {
		t.Fatal("death sound waited for death animation progression")
	}
}

func TestZombieStepSoundsUseActualGroundMovementAndTracking(t *testing.T) {
	world := zombieGroundWorld(-1, 2, -1, 1)

	world.SetDayTime(18000)

	runtime := NewRuntime(world)

	_, trackedConnection := newZombieTestSession(t, runtime, game.Position{X: 100})

	untracked, untrackedConnection := newMovementTestSession(runtime, "10111213-1415-1617-1819-1a1b1c1d1e1f", "Untracked")

	untracked.loadedChunks = map[LoadedChunk]struct{}{}

	joinTestSession(t, runtime, untracked)

	runtime.entityRandom = func() float32 {
		return 0.9
	}

	zombie := runtime.SpawnZombie(game.Position{X: 0.5, Z: 0.5})

	zombie.Living.OnGround = true
	zombie.Living.MoveDistance = 1
	zombie.Living.Velocity.X = 0.2

	trackedConnection.reset()
	untrackedConnection.reset()

	runtime.Tick()

	assertZombieSound(t, packetsByID(t, trackedConnection, protocol.ClientboundSoundID), game.SoundEntityZombieStep, 0.15, 1)

	if sessionHasPacket(untrackedConnection.packets(t), protocol.ClientboundSoundID) {
		t.Fatal("untracked viewer received zombie step sound")
	}

	zombie.Living.MoveDistance = 0
	zombie.Living.NextStepDistance = 1
	zombie.Living.Velocity = game.Velocity{}

	trackedConnection.reset()

	runtime.Tick()

	if sessionHasPacket(trackedConnection.packets(t), protocol.ClientboundSoundID) {
		t.Fatal("stationary zombie emitted a step sound")
	}
}

func tickZombieUntilGrounded(t *testing.T, runtime *Runtime, zombie *runtimeZombieEntity) {
	t.Helper()

	for range 200 {
		runtime.Tick()

		if zombie.Living.OnGround {
			return
		}
	}

	t.Fatal("zombie did not land")
}

func zombieMetadataFlags(t *testing.T, packet protocol.Packet) byte {
	t.Helper()

	reader := protocol.NewPacketReader(packet.Data)

	reader.VarInt()

	if reader.Byte() != protocol.EntityFlagsMetadataIndex || reader.VarInt() != protocol.MetadataTypeByte {
		t.Fatal("zombie metadata did not start with entity flags")
	}

	flags := reader.Byte()

	err := reader.Err()
	if err != nil {
		t.Fatalf("decode zombie metadata: %v", err)
	}

	return flags
}

func assertZombieSound(t *testing.T, packets []protocol.Packet, event game.SoundEvent, volume, pitch float32) {
	t.Helper()

	for _, packet := range packets {
		reader := protocol.NewPacketReader(packet.Data)

		reader.VarInt()

		actualEvent := reader.String(32767)

		if reader.Bool() {
			reader.Float()
		}

		source := reader.VarInt()

		reader.Int()
		reader.Int()
		reader.Int()

		actualVolume := reader.Float()
		actualPitch := reader.Float()

		reader.Long()

		err := reader.Err()
		if err != nil {
			t.Fatalf("decode zombie sound: %v", err)
		}

		if actualEvent == string(event) && source == protocol.SoundSourceHostile && actualVolume == volume && actualPitch == pitch {
			return
		}
	}

	t.Fatalf("zombie sound %q volume %v pitch %v not found", event, volume, pitch)
}
