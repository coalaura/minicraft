package game

import "testing"

func TestStateSpecificLightProperties(t *testing.T) {
	levelThree, ok := Light.WithProperties(BlockPropertyValue{Name: "level", Value: "3"})
	if !ok {
		t.Fatal("build level-three light state")
	}

	emission, filter := levelThree.LightProperties()
	if emission != 3 || filter != 0 {
		t.Fatalf("level-three light properties = %d, %d", emission, filter)
	}

	darkAnchor, ok := RespawnAnchor.WithProperties(BlockPropertyValue{Name: "charges", Value: "0"})
	if !ok {
		t.Fatal("build uncharged respawn anchor state")
	}

	emission, _ = darkAnchor.LightProperties()
	if emission != 0 {
		t.Fatalf("uncharged respawn anchor emission = %d", emission)
	}
}

func TestLitStateEmission(t *testing.T) {
	tests := []struct {
		name     string
		block    Block
		expected uint8
	}{
		{name: "furnace", block: Furnace, expected: 13},
		{name: "redstone ore", block: RedstoneOre, expected: 9},
		{name: "redstone lamp", block: RedstoneLamp, expected: 15},
		{name: "candle cake", block: CandleCake, expected: 3},
		{name: "copper bulb", block: CopperBulb, expected: 15},
		{name: "exposed copper bulb", block: ExposedCopperBulb, expected: 12},
		{name: "weathered copper bulb", block: WeatheredCopperBulb, expected: 8},
		{name: "oxidized copper bulb", block: OxidizedCopperBulb, expected: 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lit, ok := test.block.WithProperties(BlockPropertyValue{Name: "lit", Value: "true"})
			if !ok {
				t.Fatal("build lit block state")
			}

			emission, _ := lit.LightProperties()
			if emission != test.expected {
				t.Fatalf("lit emission = %d, want %d", emission, test.expected)
			}

			unlit, ok := test.block.WithProperties(BlockPropertyValue{Name: "lit", Value: "false"})
			if !ok {
				t.Fatal("build unlit block state")
			}

			emission, _ = unlit.LightProperties()
			if emission != 0 {
				t.Fatalf("unlit emission = %d, want 0", emission)
			}
		})
	}

	fourCandles, ok := Candle.WithProperties(
		BlockPropertyValue{Name: "candles", Value: "4"},
		BlockPropertyValue{Name: "lit", Value: "true"},
	)

	if !ok {
		t.Fatal("build four-candle state")
	}

	emission, _ := fourCandles.LightProperties()
	if emission != 12 {
		t.Fatalf("four-candle emission = %d, want 12", emission)
	}
}
