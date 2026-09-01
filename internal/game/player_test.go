package game

import "testing"

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
		SurvivalInitialized: false,
	}

	player.ResetSurvivalState()

	if player.Health != DefaultPlayerHealth || player.FoodLevel != DefaultPlayerFoodLevel || player.Saturation != DefaultPlayerSaturation || player.AirSupply != DefaultPlayerAirSupply {
		t.Fatalf("reset vital state = health %v food %d saturation %v air %d", player.Health, player.FoodLevel, player.Saturation, player.AirSupply)
	}

	if player.FallDistance != 0 || player.RemainingFireTicks != 0 || player.InvulnerableTime != 0 || player.LastHurt != 0 || len(player.ActiveEffects) != 0 || player.Dead || !player.SurvivalInitialized {
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
