package server

import (
	"io"
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

type naturalSalmonTestCase struct {
	Name      string
	Choice    int
	Variant   int32
	Width     float64
	Height    float64
	EyeHeight float64
}

type naturalSheepTestCase struct {
	Name   string
	Biome  game.Biome
	Colors [5]byte
}

func TestNaturalSpawnCandidateInclusiveYBoundaries(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}})

	top := runtime.naturalSurfaceHeight(game.BlockPosition{})
	if top != -59 {
		t.Fatalf("first empty surface Y = %d, want -59", top)
	}

	setNaturalRandomSequence(t, runtime, 0, 0, 0)

	minimum := runtime.naturalRandomPosition(LoadedChunk{})

	setNaturalRandomSequence(t, runtime, 0, 0, math.Nextafter32(1, 0))

	maximum := runtime.naturalRandomPosition(LoadedChunk{})

	if minimum.Y != -64 || maximum.Y != top {
		t.Fatalf("candidate boundaries = %d..%d, want -64..%d", minimum.Y, maximum.Y, top)
	}
}

func TestNaturalSalmonSizesAndSchool(t *testing.T) {
	tests := []naturalSalmonTestCase{
		{Name: "small lower", Choice: 0, Variant: 0, Width: 0.35, Height: 0.2, EyeHeight: 0.13},
		{Name: "small upper", Choice: 29, Variant: 0, Width: 0.35, Height: 0.2, EyeHeight: 0.13},
		{Name: "medium lower", Choice: 30, Variant: 1, Width: 0.7, Height: 0.4, EyeHeight: 0.26},
		{Name: "medium upper", Choice: 79, Variant: 1, Width: 0.7, Height: 0.4, EyeHeight: 0.26},
		{Name: "large lower", Choice: 80, Variant: 2, Width: 1.05, Height: 0.6, EyeHeight: 0.39},
		{Name: "large upper", Choice: 94, Variant: 2, Width: 1.05, Height: 0.6, EyeHeight: 0.39},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{})

			setNaturalRandomSequence(t, runtime, 0.5, 0.25, (float32(test.Choice)+0.5)/95, 0.5, 0.5, 0.5)

			group := naturalSpawnGroup{}

			entity := runtime.spawnNaturalEntity(game.EntitySalmon, game.Position{Y: 10}, &group).(*runtimeSalmonEntity)

			if entity.Variant != test.Variant || math.Abs(entity.Living.Width-test.Width) > 1e-12 || math.Abs(entity.Living.Height-test.Height) > 1e-12 || math.Abs(entity.RuntimeLivingEyeHeight()-test.EyeHeight) > 1e-12 {
				t.Fatalf("salmon size = variant %d, %g x %g, eye %g", entity.Variant, entity.Living.Width, entity.Living.Height, entity.RuntimeLivingEyeHeight())
			}

			assertNaturalVariantMetadata(t, entity, test.Variant)

			if group.Leader != entity || entity.School.Leader != nil || entity.School.SchoolSize != 1 || entity.MaximumSchoolSize != 5 {
				t.Fatal("salmon leader/max school state differs from source")
			}

			for index := 1; index < 5; index++ {
				setNaturalRandomSequence(t, runtime, 0.5, 0.25, 0.5, 0.5, 0.5, 0.5)

				follower := runtime.spawnNaturalEntity(game.EntitySalmon, game.Position{X: float64(index), Y: 10}, &group).(*runtimeSalmonEntity)
				if follower.School.Leader != entity {
					t.Fatal("natural salmon did not follow its group leader")
				}
			}

			setNaturalRandomSequence(t, runtime, 0.5)

			if entity.School.SchoolSize != 5 || connectAquaticFollower(runtime.newSalmon(), entity) {
				t.Fatal("salmon school exceeded five")
			}
		})
	}
}

func TestNaturalTropicalCommonSchool(t *testing.T) {
	// Source COMMON_VARIANTS packed independently, in registration order.
	expected := [...]int32{0x07010101, 0x07070001, 0x0b070001, 0x07000501, 0x070b0100, 0x00010000, 0x03060500, 0x040a0301, 0x0e000501, 0x04000500, 0x07000201, 0x01000501, 0x06090300, 0x03050400, 0x000e0401, 0x0e070200, 0x000e0301, 0x04000001, 0x000e0000, 0x00070100, 0x04090300, 0x04040001}

	for index, variant := range expected {
		runtime := NewRuntime(&game.World{})

		setNaturalRandomSequence(t, runtime, 0.5, 0.25, 0.5, 0.5, 0.5, math.Nextafter32(0.9, 0), (float32(index)+0.5)/22)

		group := naturalSpawnGroup{}

		leader := runtime.spawnNaturalEntity(game.EntityTropicalFish, game.Position{Y: 10}, &group).(*runtimeTropicalFishEntity)

		// A follower must reuse the variant without drawing another chance or variant.
		setNaturalRandomSequence(t, runtime, 0.5, 0.75, 0.5, 0.5, 0.5)

		follower := runtime.spawnNaturalEntity(game.EntityTropicalFish, game.Position{X: 1, Y: 10}, &group).(*runtimeTropicalFishEntity)

		if !group.CommonTropical || group.TropicalVariant != variant || leader.Variant != variant || follower.Variant != variant || group.Leader != leader || follower.School.Leader != leader || leader.School.SchoolSize != 2 {
			t.Fatalf("common variant %d = %08x/%08x, group %+v", index, leader.Variant, follower.Variant, group)
		}

		assertNaturalVariantMetadata(t, leader, variant)
		assertNaturalVariantMetadata(t, follower, variant)
	}
}

func TestNaturalTropicalRandomIndividual(t *testing.T) {
	for pattern := range 12 {
		runtime := NewRuntime(&game.World{})

		setNaturalRandomSequence(t, runtime, 0.5, 0.25, 0.5, 0.5, 0.5, 0.9, (float32(pattern)+0.5)/12, 9.5/16, 14.5/16)

		group := naturalSpawnGroup{}

		fish := runtime.spawnNaturalEntity(game.EntityTropicalFish, game.Position{Y: 10}, &group).(*runtimeTropicalFishEntity)

		expected := int32(0x0e090000 | pattern/6 | (pattern%6)<<8)

		if group.CommonTropical || fish.Variant != expected || group.Leader != fish || fish.School.Leader != nil || fish.School.SchoolSize != 1 {
			t.Fatalf("random individual pattern %d = %08x, group %+v", pattern, fish.Variant, group)
		}

		assertNaturalVariantMetadata(t, fish, expected)

		setNaturalRandomSequence(t, runtime, 0.5)

		if !connectAquaticFollower(runtime.newTropicalFish(), fish) {
			t.Fatal("random natural variant incorrectly disabled later schooling AI")
		}
	}
}

func TestNaturalTropicalPackFinalization(t *testing.T) {
	paths := []bool{true, false}

	for _, common := range paths {
		runtime := NewRuntime(&game.World{Generator: spawnTestBiomeGenerator{spawnTestGenerator: spawnTestGenerator{Floor: game.Water, Water: true}, Biome: game.BiomeWarmOcean}, SeaLevel: -58, Spawn: game.Position{X: 1000}})

		player := addRuntimeMobTestPlayer(t, runtime, game.Position{Y: -60}, game.GameModeSurvival)

		runtime.setSessionActiveChunks(player, []LoadedChunk{{X: 2}})
		runtime.prepareNaturalSpawnState()

		values := make([]float32, 0, 100)

		values = append(values, 0, 0, 4.5/8)

		if common {
			values = append(values, 0.5, 0.25, 0, 0, 0, 0.5, 0)
			values = append(values, 0.5, 0.25, 0.5, 0.5, 0.5, 0, 3.5/22)

			for range 7 {
				values = append(values, 0.25, 0, 0, 0, 0.5, 0.25, 0.5, 0.5, 0.5)
			}
		} else {
			for index := range 3 {
				values = append(values, 0.5, (float32(index)+1.5)/6, 0, 0, 0, 0.5, 0)
				values = append(values, 0.5, 0.25, 0.5, 0.5, 0.5, 0.9, (float32(index)+0.5)/12, 9.5/16, 14.5/16)
			}
		}

		setNaturalRandomSequence(t, runtime, values...)

		runtime.spawnNaturalPack(LoadedChunk{X: 2}, naturalWaterAmbient)

		entities := runtime.snapshotRuntimeEntities()

		if common {
			if len(entities) != 8 {
				t.Fatalf("common natural pack size = %d, want 8", len(entities))
			}

			leaders := 0

			for _, entity := range entities {
				fish := entity.(*runtimeTropicalFishEntity)
				if fish.Variant != 0x07000501 {
					t.Fatalf("common pack did not share packed variant: %08x", fish.Variant)
				}

				if fish.School.Leader == nil {
					leaders++

					if fish.School.SchoolSize != 8 {
						t.Fatalf("common school size = %d", fish.School.SchoolSize)
					}
				}
			}

			if leaders != 1 {
				t.Fatalf("common school leaders = %d, want 1", leaders)
			}
		} else {
			if len(entities) != 3 {
				t.Fatalf("random path spawned %d fish, want one per outer group", len(entities))
			}

			for _, entity := range entities {
				fish := entity.(*runtimeTropicalFishEntity)
				if fish.School.Leader != nil || fish.School.SchoolSize != 1 || uint32(fish.Variant)&0xffff0000 != 0x0e090000 {
					t.Fatalf("random pack was not independently finalized: %+v, %08x", fish.School, fish.Variant)
				}
			}
		}
	}
}

func TestNaturalSheepBiomeColorWeights(t *testing.T) {
	tests := []naturalSheepTestCase{
		{Name: "temperate", Biome: game.BiomePlains, Colors: [5]byte{15, 7, 8, 12, 0}},
		{Name: "warm", Biome: game.BiomeDesert, Colors: [5]byte{7, 8, 0, 15, 12}},
		{Name: "cold", Biome: game.BiomeSnowyPlains, Colors: [5]byte{8, 7, 0, 12, 15}},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			runtime := NewRuntime(&game.World{Generator: spawnTestBiomeGenerator{Biome: test.Biome}})

			counts := [16]int{}

			for choice := range 100 {
				if choice < 18 {
					setNaturalRandomSequence(t, runtime, (float32(choice)+0.5)/100)
				} else {
					setNaturalRandomSequence(t, runtime, (float32(choice)+0.5)/100, 0)
				}

				counts[runtime.naturalSheepColor(test.Biome)]++
			}

			weights := [5]int{5, 5, 5, 3, 82}

			for index, color := range test.Colors {
				if counts[color] != weights[index] {
					t.Fatalf("color %d weight = %d, want %d", color, counts[color], weights[index])
				}
			}

			pink := 0

			for choice := range 500 {
				setNaturalRandomSequence(t, runtime, 0.5, (float32(choice)+0.5)/500)

				color := runtime.naturalSheepColor(test.Biome)
				if color == 6 {
					pink++
				} else if color != test.Colors[4] {
					t.Fatalf("usual-color branch produced %d", color)
				}
			}

			if pink != 1 {
				t.Fatalf("pink weight = %d/500", pink)
			}

			setNaturalRandomSequence(t, runtime, 0.25, 0.5, 0, 0.5, 0.5, 0.5)

			group := naturalSpawnGroup{}

			sheep := runtime.spawnNaturalEntity(game.EntitySheep, game.Position{}, &group).(*runtimeSheepEntity)
			if sheep.WoolColor != test.Colors[4] {
				t.Fatalf("finalized wool = %d, want %d", sheep.WoolColor, test.Colors[4])
			}

			if runtime.SpawnSheep(game.Position{}).WoolColor != 0 {
				t.Fatal("summon sheep unexpectedly randomized its color")
			}
		})
	}
}

func TestNaturalSpawnRotationAttributesAndPackets(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	setNaturalRandomSequence(t, runtime, 0.25, 0.75, 0.25, 0.049, 0.6)

	group := naturalSpawnGroup{}

	zombie := runtime.spawnNaturalEntity(game.EntityZombie, game.Position{Y: 10}, &group).(*runtimeZombieEntity)

	rotation := zombie.Rotation

	if rotation.Yaw != 90 || rotation.HeadYaw != 90 || rotation.BodyYaw != 90 || rotation.Pitch != 0 || zombie.State.tracker.LastYaw != 64 || zombie.State.tracker.LastHeadYaw != 64 {
		t.Fatalf("natural rotation/tracker = %+v/%+v", rotation, zombie.State.tracker)
	}

	if !zombie.LeftHanded || zombie.MetadataFlags() != 2 || math.Abs(zombie.FollowRange(16)-16*(1+0.5*0.11485000000000001)) > 1e-12 || math.Abs(float64(zombie.Living.KnockbackResistance-zombieKnockbackResistance)-0.03) > 1e-7 {
		t.Fatalf("natural attributes = %+v, knockback %g", zombie.RuntimeMobState, zombie.Living.KnockbackResistance)
	}

	view := zombie.runtimeEntityViewLocked()

	snapshot := runtimeEntitySpawnSnapshotLocked(view, zombie.State.tracker)

	packet := zombie.AddEntityPacket(snapshot)
	if packet.Yaw != 64 || packet.HeadYaw != 64 || packet.Pitch != 0 {
		t.Fatalf("spawn packet rotation = %+v", packet)
	}

	viewer, connection := newMovementTestSession(runtime, "00000000-0000-0000-0000-000000000097", "viewer")

	viewer.trackRuntimeEntity(zombie)

	packets := packetsByID(t, connection, protocol.ClientboundAddEntityID)
	if len(packets) != 1 {
		t.Fatalf("spawn packets = %d, want 1", len(packets))
	}

	reader := protocol.NewPacketReader(packets[0].Data)

	reader.VarInt()

	_, err := reader.Seek(16, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}

	reader.VarInt()
	reader.Double()
	reader.Double()
	reader.Double()

	velocity := reader.Byte()
	pitch := reader.Byte()
	yaw := reader.Byte()
	headYaw := reader.Byte()

	if reader.Err() != nil || velocity != 0 || pitch != 0 || yaw != 64 || headYaw != 64 {
		t.Fatalf("transmitted natural rotation = %d/%d/%d, err=%v", pitch, yaw, headYaw, reader.Err())
	}

	summoned := runtime.SpawnZombie(game.Position{})
	if summoned.Rotation != (game.Rotation{}) || summoned.LeftHanded || summoned.FollowRangeBonus != 0 || summoned.Living.KnockbackResistance != zombieKnockbackResistance {
		t.Fatal("generic constructor unexpectedly applied natural finalization")
	}
}

func TestNaturalPopulationInitializationQuantitative(t *testing.T) {
	runtime := newSpawnPopulationTestRuntime(t, 73)

	for tick := int64(1); tick <= 100; tick++ {
		runtime.tickNaturalSpawning(game.TimeState{Age: tick})
	}

	entities := runtime.snapshotRuntimeEntities()

	var (
		nonzeroYaw    int
		leftHanded    int
		positiveRange int
		negativeRange int
	)

	for _, entity := range entities {
		mob := entity.(RuntimeMobEntity)

		rotation := runtimeLivingRotation(mob)
		if rotation.Yaw < 0 || rotation.Yaw >= 360 || rotation.HeadYaw != rotation.Yaw || rotation.BodyYaw != rotation.Yaw {
			t.Fatalf("invalid initial rotation = %+v", rotation)
		}

		if rotation.Yaw != 0 {
			nonzeroYaw++
		}

		if mob.RuntimeMob().LeftHanded {
			leftHanded++
		}

		bonus := mob.RuntimeMob().FollowRangeBonus
		if math.Abs(bonus) > 0.11485000000000001 {
			t.Fatalf("follow range multiplier out of bounds = %g", bonus)
		}

		if bonus > 0 {
			positiveRange++
		} else if bonus < 0 {
			negativeRange++
		}
	}

	if len(entities) < 70 || nonzeroYaw != len(entities) || leftHanded == 0 || leftHanded > 15 || positiveRange < 20 || negativeRange < 20 {
		t.Fatalf("initialization: population=%d, nonzero yaw=%d, left handed=%d, positive/negative range=%d/%d", len(entities), nonzeroYaw, leftHanded, positiveRange, negativeRange)
	}

	t.Logf("natural initialization: %d mobs, %d randomized yaws, %d left-handed, %d/%d positive/negative follow-range modifiers", len(entities), nonzeroYaw, leftHanded, positiveRange, negativeRange)
}

func setNaturalRandomSequence(t *testing.T, runtime *Runtime, values ...float32) {
	t.Helper()

	index := 0

	t.Cleanup(func() {
		if index != len(values) {
			t.Errorf("natural random sequence consumed %d of %d draws", index, len(values))
		}
	})

	runtime.entityRandom = func() float32 {
		if index >= len(values) {
			t.Fatal("unexpected natural-finalization random draw")
		}

		value := values[index]
		index++

		return value
	}
}

func assertNaturalVariantMetadata(t *testing.T, entity RuntimeEntityMetadata, variant int32) {
	t.Helper()

	for _, entry := range entity.EntityMetadata() {
		if entry.Index == protocol.FishVariantMetadataIndex {
			if entry.Type != protocol.MetadataTypeInt || entry.Value != protocol.MetadataVarInt(variant) {
				t.Fatalf("variant metadata = %+v, want %d", entry, variant)
			}

			return
		}
	}

	t.Fatal("missing variant metadata")
}
