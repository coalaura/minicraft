package game

import (
	"math"
	"testing"
)

func TestPlayerResetSurvivalState(t *testing.T) {
	player := Player{
		Health:             1,
		FoodLevel:          1,
		Saturation:         1,
		AirSupply:          1,
		FallDistance:       1,
		RemainingFireTicks: 1,
		InvulnerableTime:   1,
		LastHurt:           1,
		ActiveEffects: ActiveMobEffects{
			NewMobEffectInstance(MobEffectSpeed, 20, 0, false, true, true),
		},
		Dead:                true,
		DeathTime:           19,
		DeathEntityRemoved:  true,
		SurvivalInitialized: false,
	}

	player.ResetSurvivalState()

	if player.Health != DefaultPlayerHealth || player.FoodLevel != DefaultPlayerFoodLevel || player.Saturation != DefaultPlayerSaturation || player.AirSupply != DefaultPlayerAirSupply {
		t.Fatalf("reset vital state = health %v food %d saturation %v air %d", player.Health, player.FoodLevel, player.Saturation, player.AirSupply)
	}

	if player.FallDistance != 0 || player.RemainingFireTicks != 0 || player.InvulnerableTime != 0 || player.LastHurt != 0 || len(player.ActiveEffects) != 0 || player.Dead || player.DeathTime != 0 || player.DeathEntityRemoved || !player.SurvivalInitialized {
		t.Fatalf("reset transient state = %+v", player)
	}
}

func TestPlayerCloneCopiesActiveEffects(t *testing.T) {
	hidden := NewMobEffectInstance(MobEffectSpeed, 20, 0, false, true, true)

	player := Player{ActiveEffects: ActiveMobEffects{NewMobEffectInstance(MobEffectSpeed, 10, 1, false, true, true)}}

	player.ActiveEffects[0].HiddenEffect = &hidden

	clone := player.Clone()

	clone.ActiveEffects[0].Duration = 99
	clone.ActiveEffects[0].HiddenEffect.Duration = 88

	if player.ActiveEffects[0].Duration != 10 || player.ActiveEffects[0].HiddenEffect.Duration != 20 {
		t.Fatalf("player active effects changed through clone: %+v", player.ActiveEffects)
	}
}

func TestPlayerMainHandAttackAttributes(t *testing.T) {
	player := Player{SelectedHotbarSlot: 0}

	if player.MainHandAttackDamage() != DefaultPlayerAttackDamage || player.MainHandAttackSpeed() != DefaultPlayerAttackSpeed {
		t.Fatalf("empty main hand attack attributes = damage %v speed %v", player.MainHandAttackDamage(), player.MainHandAttackSpeed())
	}

	player.Inventory.Hotbar[0] = ItemStack{Item: ItemCopperAxe, Count: 1}

	if player.MainHandAttackDamage() != 9 || math.Abs(float64(player.MainHandAttackSpeed()-0.8)) > 1e-6 {
		t.Fatalf("copper axe attack attributes = damage %v speed %v", player.MainHandAttackDamage(), player.MainHandAttackSpeed())
	}

	player.Inventory.Hotbar[0] = ItemStack{Item: ItemStone, Count: 1}

	if player.MainHandAttackDamage() != DefaultPlayerAttackDamage || player.MainHandAttackSpeed() != DefaultPlayerAttackSpeed {
		t.Fatalf("non-tool main hand attack attributes = damage %v speed %v", player.MainHandAttackDamage(), player.MainHandAttackSpeed())
	}
}

func TestPlayerAttackStrengthProgressionAndItemChanges(t *testing.T) {
	player := Player{SelectedHotbarSlot: 0}

	for range 5 {
		player.TickAttackStrength()
	}

	strength := player.AttackStrength()
	if math.Abs(float64(strength-0.9)) > 1e-6 {
		t.Fatalf("empty-hand attack strength = %v, want 0.9", strength)
	}

	player.TickAttackStrength()

	strength = player.AttackStrength()

	if strength != 1 {
		t.Fatalf("fully charged empty-hand strength = %v, want 1", strength)
	}

	player.Inventory.Hotbar[0] = ItemStack{Item: ItemDiamondSword, Count: 1}

	player.TickAttackStrength()

	if player.AttackStrengthTicker != 0 {
		t.Fatalf("item change ticker = %d, want 0", player.AttackStrengthTicker)
	}

	wantInitial := float32(0.5 / 12.5)
	strength = player.AttackStrength()

	if math.Abs(float64(strength-wantInitial)) > 1e-6 {
		t.Fatalf("sword initial attack strength = %v, want %v", strength, wantInitial)
	}

	player.Inventory.Hotbar[0].SetDamage(1)

	player.TickAttackStrength()

	if player.AttackStrengthTicker != 1 {
		t.Fatalf("durability change ticker = %d, want 1", player.AttackStrengthTicker)
	}

	player.Inventory.Hotbar[1] = ItemStack{Item: ItemDiamondSword, Count: 32}
	player.SelectedHotbarSlot = 1

	player.TickAttackStrength()

	if player.AttackStrengthTicker != 2 {
		t.Fatalf("same-item slot change ticker = %d, want 2", player.AttackStrengthTicker)
	}

	player.Inventory.Hotbar[1].Count = 1

	player.TickAttackStrength()

	if player.AttackStrengthTicker != 3 {
		t.Fatalf("same-item count change ticker = %d, want 3", player.AttackStrengthTicker)
	}

	player.ResetAttackStrength()

	if player.AttackStrengthTicker != 0 {
		t.Fatalf("reset ticker = %d, want 0", player.AttackStrengthTicker)
	}
}

func TestPlayerEntityInteractionRangeBoundaries(t *testing.T) {
	player := Player{Position: Position{Y: -standingPlayerEyeHeight}}

	target := AABB{MinX: 6, MinY: 0, MinZ: 0, MaxX: 7, MaxY: 1, MaxZ: 1}

	if player.IsWithinEntityInteractionRange(target, 3, false) {
		t.Fatal("ordinary interaction accepted strict six-block boundary")
	}

	if !player.IsWithinEntityInteractionRange(target, 3, true) {
		t.Fatal("attack rejected inclusive six-block boundary")
	}
}
