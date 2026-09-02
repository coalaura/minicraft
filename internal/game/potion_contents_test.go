package game

import (
	"reflect"
	"testing"
)

func TestPotionContentsParseRoundTrip(t *testing.T) {
	hiddenTail := NewMobEffectInstance(MobEffectStrength, 13, 1, true, false, true)
	hidden := NewMobEffectInstance(MobEffectStrength, 27, 2, false, true, false)

	hidden.HiddenEffect = &hiddenTail

	first := NewMobEffectInstance(MobEffectStrength, 42, 3, true, false, true)

	first.HiddenEffect = &hidden

	second := NewMobEffectInstance(MobEffectPoison, InfiniteMobEffectDuration, 0, false, true, true)

	tests := []PotionContents{
		{CustomEffects: []MobEffectInstance{}},
		{HasPotion: true, Potion: PotionWater, CustomEffects: []MobEffectInstance{}},
		{HasCustomColor: true, CustomColor: -1234567, CustomEffects: []MobEffectInstance{}},
		{HasCustomName: true, CustomName: "Tonic \u6c34", CustomEffects: []MobEffectInstance{}},
		{CustomEffects: []MobEffectInstance{first}},
		{CustomEffects: []MobEffectInstance{first, second}},
		{
			Potion:         PotionWater,
			HasPotion:      true,
			CustomColor:    0x12345678,
			HasCustomColor: true,
			CustomEffects:  []MobEffectInstance{first, second},
			CustomName:     "Tonic \u6c34",
			HasCustomName:  true,
		},
	}

	for index := range tests {
		encoded := appendPotionContents(nil, tests[index])

		parsed, err := ParsePotionContents(encoded)
		if err != nil {
			t.Fatalf("case %d parse error: %v", index, err)
		}

		if !reflect.DeepEqual(parsed, tests[index]) {
			t.Fatalf("case %d parsed = %#v, want %#v", index, parsed, tests[index])
		}

		roundTrip := appendPotionContents(nil, parsed)
		if !reflect.DeepEqual(roundTrip, encoded) {
			t.Fatalf("case %d round trip = %v, want %v", index, roundTrip, encoded)
		}
	}
}

func TestParsePotionContentsRejectsInvalidData(t *testing.T) {
	invalidPotion := []byte{1}
	invalidPotion = appendComponentVarInt(invalidPotion, int32(len(generatedPotionDefinitions)))

	invalidEffect := []byte{0, 0, 1}
	invalidEffect = appendComponentVarInt(invalidEffect, int32(MaxMobEffectID+1))

	trailing := appendPotionContents(nil, PotionContents{})
	trailing = append(trailing, 0)

	truncatedHidden := []byte{0, 0, 1}
	truncatedHidden = appendComponentVarInt(truncatedHidden, int32(MobEffectSpeed))
	truncatedHidden = appendComponentVarInt(truncatedHidden, 0)
	truncatedHidden = appendComponentVarInt(truncatedHidden, 1)
	truncatedHidden = append(truncatedHidden, 0, 1, 1, 1)

	overlongHidden := []byte{0, 0, 1}
	overlongHidden = appendComponentVarInt(overlongHidden, int32(MobEffectSpeed))

	for index := 0; index <= maxPotionHiddenEffectDepth; index++ {
		overlongHidden = appendComponentVarInt(overlongHidden, 0)
		overlongHidden = appendComponentVarInt(overlongHidden, 1)
		overlongHidden = append(overlongHidden, 0, 1, 1, 1)
	}

	overlongEffects := []byte{0, 0}
	overlongEffects = appendComponentVarInt(overlongEffects, maxPotionCustomEffects+1)

	tests := [][]byte{invalidPotion, invalidEffect, trailing, truncatedHidden, overlongHidden, overlongEffects}

	for index := range tests {
		_, err := ParsePotionContents(tests[index])
		if err == nil {
			t.Fatalf("case %d parsed invalid data", index)
		}
	}
}

func TestPotionDurationScaleAndEffects(t *testing.T) {
	stack := ItemStack{}

	scale := stack.PotionDurationScale()
	if scale != 1 {
		t.Fatalf("default duration scale = %v, want 1", scale)
	}

	stack.Components = []ItemComponent{{Type: ItemComponentPotionDurationScale, Data: []byte{0, 0, 0}}}

	scale = stack.PotionDurationScale()
	if scale != 1 {
		t.Fatalf("malformed duration scale = %v, want 1", scale)
	}

	stack.SetPotionDurationScale(0.5)

	scale = stack.PotionDurationScale()
	if scale != 0.5 {
		t.Fatalf("stored duration scale = %v, want 0.5", scale)
	}

	hidden := NewMobEffectInstance(MobEffectSpeed, 11, 0, false, true, true)

	contents := PotionContents{CustomEffects: []MobEffectInstance{
		NewMobEffectInstance(MobEffectSpeed, InfiniteMobEffectDuration, 0, false, true, true),
		NewMobEffectInstance(MobEffectStrength, 0, 0, false, true, true),
		NewMobEffectInstance(MobEffectRegeneration, 5, 0, false, true, true),
		NewMobEffectInstance(MobEffectPoison, 5, 0, false, true, true),
	}}

	contents.CustomEffects[2].HiddenEffect = &hidden

	effects := contents.Effects(0.5)
	if effects[0].Duration != InfiniteMobEffectDuration || effects[1].Duration != 0 || effects[2].Duration != 2 || effects[3].Duration != 2 {
		t.Fatalf("scaled durations = %+v", effects)
	}

	if effects[2].HiddenEffect == nil || effects[2].HiddenEffect.Duration != 11 {
		t.Fatalf("scaled hidden effect = %+v", effects[2].HiddenEffect)
	}

	effects = contents.Effects(0)
	if effects[3].Duration != 1 {
		t.Fatalf("minimum scaled duration = %d, want 1", effects[3].Duration)
	}
}

func TestPotionDefinitionReturnsIndependentEffects(t *testing.T) {
	potion, found := PotionByName("long_turtle_master")
	if !found {
		t.Fatal("long_turtle_master not found")
	}

	definition, valid := potion.Definition()
	if !valid || len(definition.Effects) == 0 {
		t.Fatalf("definition = %+v, valid %t", definition, valid)
	}

	definition.Effects[0].Duration = 1
	definition.Effects = append(definition.Effects, NewMobEffectInstance(MobEffectSpeed, 1, 0, false, true, true))

	again, valid := potion.Definition()
	if !valid || len(again.Effects) != 2 || again.Effects[0].Duration != 800 {
		t.Fatalf("definition after mutation = %+v, valid %t", again, valid)
	}
}
