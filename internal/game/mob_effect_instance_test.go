package game

import "testing"

func TestMobEffectInstanceUpdateHigherAmplifierPreservesShorterEffect(t *testing.T) {
	instance := NewMobEffectInstance(MobEffectSpeed, 100, 0, true, true, true)
	takeOver := NewMobEffectInstance(MobEffectSpeed, 20, 1, false, false, false)

	if !instance.Update(takeOver) {
		t.Fatal("higher amplifier update did not report change")
	}

	if instance.Duration != 20 || instance.Amplifier != 1 || instance.Ambient || instance.Visible || instance.ShowIcon {
		t.Fatalf("updated instance = %+v", instance)
	}

	if instance.HiddenEffect == nil || instance.HiddenEffect.Duration != 100 || instance.HiddenEffect.Amplifier != 0 {
		t.Fatalf("hidden effect = %+v", instance.HiddenEffect)
	}
}

func TestMobEffectInstanceUpdateLongerWeakerEffectBecomesHidden(t *testing.T) {
	instance := NewMobEffectInstance(MobEffectSpeed, 20, 1, false, true, true)
	takeOver := NewMobEffectInstance(MobEffectSpeed, 100, 0, true, false, false)

	if !instance.Update(takeOver) {
		t.Fatal("display changes did not report change")
	}

	if instance.Duration != 20 || instance.Amplifier != 1 || instance.Ambient || instance.Visible || instance.ShowIcon {
		t.Fatalf("active effect = %+v", instance)
	}

	if instance.HiddenEffect == nil || instance.HiddenEffect.Duration != 100 || instance.HiddenEffect.Amplifier != 0 || !instance.HiddenEffect.Ambient {
		t.Fatalf("hidden effect = %+v", instance.HiddenEffect)
	}
}

func TestMobEffectInstanceUpdateExtendsEqualAmplifier(t *testing.T) {
	instance := NewMobEffectInstance(MobEffectSpeed, 20, 1, true, true, true)
	takeOver := NewMobEffectInstance(MobEffectSpeed, 100, 1, false, false, false)

	if !instance.Update(takeOver) {
		t.Fatal("equal amplifier extension did not report change")
	}

	if instance.Duration != 100 || instance.Amplifier != 1 || instance.Ambient || instance.Visible || instance.ShowIcon || instance.HiddenEffect != nil {
		t.Fatalf("updated instance = %+v", instance)
	}
}

func TestMobEffectInstanceTickRestoresHiddenEffectAndTicksItsDuration(t *testing.T) {
	hidden := NewMobEffectInstance(MobEffectSpeed, 10, 0, true, false, false)
	instance := NewMobEffectInstance(MobEffectSpeed, 1, 1, false, true, true)

	instance.HiddenEffect = &hidden

	if !instance.Tick() {
		t.Fatal("restored hidden effect is not active")
	}

	if instance.Duration != 9 || instance.Amplifier != 0 || !instance.Ambient || instance.Visible || instance.ShowIcon || instance.HiddenEffect != nil {
		t.Fatalf("restored instance = %+v", instance)
	}
}

func TestMobEffectInstanceInfiniteDurationAndAmplifierBounds(t *testing.T) {
	instance := NewMobEffectInstance(MobEffectSpeed, InfiniteMobEffectDuration, 999, false, true, true)

	if instance.Amplifier != MaxMobEffectAmplifier {
		t.Fatalf("amplifier = %d, want %d", instance.Amplifier, MaxMobEffectAmplifier)
	}

	if !instance.Tick() || instance.Duration != InfiniteMobEffectDuration {
		t.Fatalf("infinite instance after tick = %+v", instance)
	}
}

func TestActiveMobEffectsLifecycleAndCloneIndependence(t *testing.T) {
	effects := ActiveMobEffects{}
	if !effects.Add(NewMobEffectInstance(MobEffectSpeed, 1, 0, false, true, true)) {
		t.Fatal("add did not report change")
	}

	if !effects.Add(NewMobEffectInstance(MobEffectPoison, 2, 0, false, true, true)) {
		t.Fatal("second add did not report change")
	}

	clone := effects.Clone()

	clone[0].Duration = 99

	if effects[0].Duration != 1 {
		t.Fatalf("source duration changed through clone: %d", effects[0].Duration)
	}

	if !effects.Tick() || len(effects) != 1 || effects[0].Effect != MobEffectPoison || effects[0].Duration != 1 {
		t.Fatalf("effects after tick = %+v", effects)
	}

	if !effects.Remove(MobEffectPoison) || effects.Remove(MobEffectPoison) || !effects.Add(NewMobEffectInstance(MobEffectSpeed, 3, 0, false, true, true)) || !effects.Clear() || effects.Clear() {
		t.Fatalf("lifecycle operations failed: %+v", effects)
	}
}

func TestMobEffectInstanceCloneDeepCopiesHiddenChain(t *testing.T) {
	hidden := NewMobEffectInstance(MobEffectSpeed, 5, 0, false, true, true)
	instance := NewMobEffectInstance(MobEffectSpeed, 10, 1, false, true, true)

	instance.HiddenEffect = &hidden

	clone := instance.Clone()

	clone.HiddenEffect.Duration = 99

	if instance.HiddenEffect.Duration != 5 {
		t.Fatalf("source hidden duration changed through clone: %d", instance.HiddenEffect.Duration)
	}
}
