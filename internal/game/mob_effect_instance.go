package game

import "math"

type MobEffectInstance struct {
	Effect       MobEffect
	Duration     int32
	Amplifier    int32
	Ambient      bool
	Visible      bool
	ShowIcon     bool
	HiddenEffect *MobEffectInstance
}

type ActiveMobEffects []MobEffectInstance

func (instance MobEffectInstance) Clone() MobEffectInstance {
	clone := instance

	if instance.HiddenEffect != nil {
		hidden := instance.HiddenEffect.Clone()
		clone.HiddenEffect = &hidden
	}

	return clone
}

func (instance MobEffectInstance) Infinite() bool {
	return instance.Duration == InfiniteMobEffectDuration
}

func (instance MobEffectInstance) HasRemainingDuration() bool {
	return instance.Infinite() || instance.Duration > 0
}

func (instance MobEffectInstance) WithScaledDuration(scale float32) MobEffectInstance {
	clone := instance.Clone()

	if clone.Duration == InfiniteMobEffectDuration || clone.Duration == 0 {
		return clone
	}

	scaled := float64(float32(clone.Duration) * scale)

	switch {
	case math.IsNaN(scaled), scaled < 1:
		clone.Duration = 1
	case scaled >= math.MaxInt32:
		clone.Duration = math.MaxInt32
	default:
		clone.Duration = int32(math.Floor(scaled))
	}

	return clone
}

// Update applies Java's MobEffectInstance takeover and hidden-effect rules.
func (instance *MobEffectInstance) Update(takeOver MobEffectInstance) bool {
	changed := false

	if takeOver.Amplifier > instance.Amplifier {
		if takeOver.shorterThan(*instance) {
			previousHidden := instance.HiddenEffect

			hidden := instance.Clone()

			hidden.HiddenEffect = previousHidden

			instance.HiddenEffect = &hidden
		}

		instance.Amplifier = takeOver.Amplifier
		instance.Duration = takeOver.Duration

		changed = true
	} else if instance.shorterThan(takeOver) {
		if takeOver.Amplifier == instance.Amplifier {
			instance.Duration = takeOver.Duration
			changed = true
		} else if instance.HiddenEffect == nil {
			hidden := takeOver.Clone()
			instance.HiddenEffect = &hidden
		} else {
			instance.HiddenEffect.Update(takeOver)
		}
	}

	if (!takeOver.Ambient && instance.Ambient) || changed {
		instance.Ambient = takeOver.Ambient
		changed = true
	}

	if takeOver.Visible != instance.Visible {
		instance.Visible = takeOver.Visible
		changed = true
	}

	if takeOver.ShowIcon != instance.ShowIcon {
		instance.ShowIcon = takeOver.ShowIcon
		changed = true
	}

	return changed
}

// Tick decrements the instance and its hidden chain, restoring a hidden effect on expiry.
func (instance *MobEffectInstance) Tick() bool {
	if !instance.HasRemainingDuration() {
		return false
	}

	instance.tickDownDuration()
	instance.restoreHiddenEffect()

	return instance.HasRemainingDuration()
}

func (effects ActiveMobEffects) Clone() ActiveMobEffects {
	clone := make(ActiveMobEffects, len(effects))

	for index := range effects {
		clone[index] = effects[index].Clone()
	}

	return clone
}

func (effects ActiveMobEffects) Find(effect MobEffect) (MobEffectInstance, bool) {
	for index := range effects {
		if effects[index].Effect == effect {
			return effects[index].Clone(), true
		}
	}

	return MobEffectInstance{}, false
}

func (effects *ActiveMobEffects) Add(instance MobEffectInstance) bool {
	for index := range *effects {
		if (*effects)[index].Effect == instance.Effect {
			return (*effects)[index].Update(instance)
		}
	}

	*effects = append(*effects, instance.Clone())

	return true
}

func (effects *ActiveMobEffects) Remove(effect MobEffect) bool {
	for index := range *effects {
		if (*effects)[index].Effect != effect {
			continue
		}

		copy((*effects)[index:], (*effects)[index+1:])
		(*effects)[len(*effects)-1] = MobEffectInstance{}
		*effects = (*effects)[:len(*effects)-1]

		return true
	}

	return false
}

func (effects *ActiveMobEffects) Clear() bool {
	if len(*effects) == 0 {
		return false
	}

	clear(*effects)
	*effects = nil

	return true
}

// Tick advances every active effect, removing only effects that have no hidden replacement.
func (effects *ActiveMobEffects) Tick() bool {
	changed := false
	active := (*effects)[:0]

	for index := range *effects {
		instance := &(*effects)[index]
		if instance.Tick() {
			active = append(active, *instance)
		}

		changed = true
	}

	clear((*effects)[len(active):])
	*effects = active

	return changed
}

func (instance MobEffectInstance) shorterThan(other MobEffectInstance) bool {
	return !instance.Infinite() && (instance.Duration < other.Duration || other.Infinite())
}

func (instance *MobEffectInstance) tickDownDuration() {
	if instance.HiddenEffect != nil {
		instance.HiddenEffect.tickDownDuration()
	}

	if !instance.Infinite() && instance.Duration != 0 {
		instance.Duration--
	}
}

func (instance *MobEffectInstance) restoreHiddenEffect() bool {
	if instance.Duration != 0 || instance.HiddenEffect == nil {
		return false
	}

	hidden := instance.HiddenEffect

	instance.Duration = hidden.Duration
	instance.Amplifier = hidden.Amplifier
	instance.Ambient = hidden.Ambient
	instance.Visible = hidden.Visible
	instance.ShowIcon = hidden.ShowIcon
	instance.HiddenEffect = hidden.HiddenEffect

	return true
}

func NewMobEffectInstance(effect MobEffect, duration, amplifier int32, ambient, visible, showIcon bool) MobEffectInstance {
	return MobEffectInstance{
		Effect:    effect,
		Duration:  duration,
		Amplifier: clampMobEffectAmplifier(amplifier),
		Ambient:   ambient,
		Visible:   visible,
		ShowIcon:  showIcon,
	}
}

func clampMobEffectAmplifier(amplifier int32) int32 {
	return min(max(amplifier, MinMobEffectAmplifier), MaxMobEffectAmplifier)
}
