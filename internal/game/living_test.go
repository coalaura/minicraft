package game

import (
	"math"
	"testing"
)

type livingDamageTest struct {
	name             string
	damage           Damage
	armor            int32
	toughness        float32
	resistance       *MobEffectInstance
	absorption       float32
	wantHealth       float32
	wantAbsorption   float32
	wantHealthDamage float32
}

func TestDamageTypeZeroAndMobAttackTraits(t *testing.T) {
	generic := DamageType(0).Traits()
	if DamageGeneric != 0 || generic.RegistryID != 18 || !generic.BypassesArmor || generic.DamagesArmor {
		t.Fatalf("generic damage traits = %+v", generic)
	}

	if DamageFall == 0 {
		t.Fatal("fall damage retained zero value")
	}

	mobAttack := DamageMobAttack.Traits()
	if mobAttack.RegistryID != 28 || mobAttack.BypassesArmor || !mobAttack.DamagesArmor {
		t.Fatalf("mob attack traits = %+v", mobAttack)
	}
}

func TestResolveLivingDamageMitigationOrdering(t *testing.T) {
	resistance := MobEffectInstance{Effect: MobEffectResistance, Amplifier: 0, Duration: 20}

	tests := []livingDamageTest{
		{name: "unmitigated", damage: Damage{Amount: 10}, wantHealth: 10, wantHealthDamage: 10},
		{name: "armor and toughness", damage: Damage{Type: DamagePlayerAttack, Amount: 10}, armor: 20, toughness: 8, wantHealth: 17, wantHealthDamage: 3},
		{name: "resistance", damage: Damage{Amount: 10}, resistance: &resistance, wantHealth: 12, wantHealthDamage: 8},
		{name: "absorption after resistance", damage: Damage{Amount: 10}, resistance: &resistance, absorption: 3, wantHealth: 15, wantAbsorption: 0, wantHealthDamage: 5},
		{name: "effects bypass", damage: Damage{Type: DamageStarve, Amount: 10}, resistance: &resistance, wantHealth: 10, wantHealthDamage: 10},
		{name: "armor and resistance bypass", damage: Damage{Type: DamageOutOfWorld, Amount: 10}, armor: 20, toughness: 8, resistance: &resistance, wantHealth: 10, wantHealthDamage: 10},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := LivingState{}

			state.Reset(20)

			state.Absorption = test.absorption

			if test.resistance != nil {
				state.ActiveEffects.Add(test.resistance.Clone())
			}

			defense := func() LivingDefense {
				return LivingDefense{Armor: test.armor, Toughness: test.toughness}
			}

			result := ResolveLivingDamage(&state, test.damage, defense, nil)
			if !result.Applied || !result.FullHurt {
				t.Fatalf("damage result = %+v, want applied full hurt", result)
			}

			assertLivingFloat(t, "health", state.Health, test.wantHealth)
			assertLivingFloat(t, "absorption", state.Absorption, test.wantAbsorption)
			assertLivingFloat(t, "health damage", result.HealthDamage, test.wantHealthDamage)
		})
	}
}

func TestResolveLivingDamageRepeatedHitAndDeath(t *testing.T) {
	state := LivingState{}

	state.Reset(10)

	first := ResolveLivingDamage(&state, Damage{Amount: 4}, nil, nil)
	if !first.Applied || !first.FullHurt || state.Health != 6 {
		t.Fatalf("first damage = %+v, state %+v", first, state)
	}

	rejected := ResolveLivingDamage(&state, Damage{Amount: 3}, nil, nil)
	if rejected.Applied || state.Health != 6 {
		t.Fatalf("smaller repeated damage = %+v, health %v", rejected, state.Health)
	}

	differential := ResolveLivingDamage(&state, Damage{Amount: 7}, nil, nil)
	if !differential.Applied || differential.FullHurt || state.Health != 3 || state.LastHurt != 7 {
		t.Fatalf("larger repeated damage = %+v, state %+v", differential, state)
	}

	state.InvulnerableTime = LivingHurtCooldownThreshold

	lethal := ResolveLivingDamage(&state, Damage{Amount: 3}, nil, nil)
	if !lethal.Applied || !lethal.FullHurt || !lethal.Died || !state.Dead || state.Health != 0 {
		t.Fatalf("lethal damage = %+v, state %+v", lethal, state)
	}
}

func TestLivingStateLifecycle(t *testing.T) {
	state := LivingState{
		Health:             1,
		MaxHealth:          1,
		Absorption:         2,
		InvulnerableTime:   2,
		LastHurt:           3,
		RemainingFireTicks: 4,
		Dead:               true,
		DeathTime:          5,
	}

	state.ActiveEffects.Add(MobEffectInstance{Effect: MobEffectResistance, Duration: 20})

	state.Reset(30)

	if state.Health != 30 || state.MaxHealth != 30 || state.Absorption != 0 || state.InvulnerableTime != 0 || state.LastHurt != 0 || state.RemainingFireTicks != 0 || state.Dead || state.DeathTime != 0 || len(state.ActiveEffects) != 0 {
		t.Fatalf("reset living state = %+v", state)
	}

	state.InvulnerableTime = 2

	state.TickHurtCooldown()

	if state.InvulnerableTime != 1 {
		t.Fatalf("hurt cooldown = %d, want 1", state.InvulnerableTime)
	}

	if state.TickDeath() || state.DeathTime != 0 {
		t.Fatal("living state ticked death while alive")
	}

	state.Dead = true

	for tick := int32(1); tick < LivingDeathDurationTicks; tick++ {
		if state.TickDeath() {
			t.Fatalf("death completed early at tick %d", tick)
		}
	}

	if !state.TickDeath() || state.DeathTime != LivingDeathDurationTicks {
		t.Fatalf("death completion time = %d, want %d", state.DeathTime, LivingDeathDurationTicks)
	}
}

func TestResolveLivingDamageRejectionsAndNormalization(t *testing.T) {
	state := LivingState{}

	state.Reset(20)

	state.ActiveEffects.Add(MobEffectInstance{Effect: MobEffectFireResistance, Duration: 20})

	if ResolveLivingDamage(&state, Damage{Type: DamageInFire, Amount: 2}, nil, nil).Applied {
		t.Fatal("fire damage bypassed Fire Resistance")
	}

	if ResolveLivingDamage(&state, Damage{Amount: 0}, nil, nil).Applied {
		t.Fatal("zero damage was applied")
	}

	result := ResolveLivingDamage(&state, Damage{Amount: float32(math.NaN())}, nil, nil)
	if !result.Applied || !result.Died || !state.Dead || state.LastHurt != math.MaxFloat32 {
		t.Fatalf("NaN damage result = %+v, state %+v", result, state)
	}
}

func TestApplyLivingKnockback(t *testing.T) {
	velocity := Velocity{X: 0.2, Y: 0.1, Z: -0.2}

	applied := ApplyLivingKnockback(&velocity, true, 0.5, -1, 0, 0.4, func() float32 {
		return 0.5
	})

	if !applied {
		t.Fatal("knockback was not applied")
	}

	assertLivingFloat64(t, "velocity x", velocity.X, 0.3)
	assertLivingFloat64(t, "velocity y", velocity.Y, 0.25)
	assertLivingFloat64(t, "velocity z", velocity.Z, -0.1)

	unchanged := velocity

	applied = ApplyLivingKnockback(&velocity, true, 1, -1, 0, 0.4, func() float32 {
		return 0.5
	})

	if applied || velocity != unchanged {
		t.Fatalf("fully resisted knockback = applied %t velocity %+v", applied, velocity)
	}
}

func TestApplyLivingKnockbackRandomizesZeroDirection(t *testing.T) {
	values := []float32{1, 0, 0.5, 0.5}
	index := 0

	random := func() float32 {
		value := values[index]
		index++

		return value
	}

	velocity := Velocity{}

	applied := ApplyLivingKnockback(&velocity, false, 0, 0, 0, 0.4, random)
	if !applied || index != 4 {
		t.Fatalf("zero-direction knockback = applied %t random calls %d", applied, index)
	}

	assertLivingFloat64(t, "velocity x", velocity.X, -0.4)
	assertLivingFloat64(t, "velocity y", velocity.Y, 0)
	assertLivingFloat64(t, "velocity z", velocity.Z, 0)
}

func assertLivingFloat(t *testing.T, name string, actual, expected float32) {
	t.Helper()

	if math.Abs(float64(actual-expected)) > 1e-5 {
		t.Fatalf("%s = %v, want %v", name, actual, expected)
	}
}

func assertLivingFloat64(t *testing.T, name string, actual, expected float64) {
	t.Helper()

	if math.Abs(actual-expected) > 1e-9 {
		t.Fatalf("%s = %v, want %v", name, actual, expected)
	}
}
