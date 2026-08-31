package game

import "testing"

func TestPlayerResetSurvivalState(t *testing.T) {
	player := Player{
		Health:              1,
		FoodLevel:           1,
		Saturation:          1,
		AirSupply:           1,
		FallDistance:        1,
		RemainingFireTicks:  1,
		InvulnerableTime:    1,
		LastHurt:            1,
		Dead:                true,
		SurvivalInitialized: false,
	}

	player.ResetSurvivalState()

	if player.Health != DefaultPlayerHealth || player.FoodLevel != DefaultPlayerFoodLevel || player.Saturation != DefaultPlayerSaturation || player.AirSupply != DefaultPlayerAirSupply {
		t.Fatalf("reset vital state = health %v food %d saturation %v air %d", player.Health, player.FoodLevel, player.Saturation, player.AirSupply)
	}

	if player.FallDistance != 0 || player.RemainingFireTicks != 0 || player.InvulnerableTime != 0 || player.LastHurt != 0 || player.Dead || !player.SurvivalInitialized {
		t.Fatalf("reset transient state = %+v", player)
	}
}
