package game

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	maxPotionCustomEffects     = 256
	maxPotionHiddenEffectDepth = 32
	maxPotionCustomNameRunes   = 32767
	maxPotionCustomNameBytes   = maxPotionCustomNameRunes * utf8.UTFMax
)

type PotionContents struct {
	Potion         Potion
	CustomColor    int32
	CustomEffects  []MobEffectInstance
	CustomName     string
	HasPotion      bool
	HasCustomColor bool
	HasCustomName  bool
}

type potionComponentDecoder struct {
	data   []byte
	offset int
}

func (contents PotionContents) Clone() PotionContents {
	clone := contents
	clone.CustomEffects = cloneMobEffectInstances(contents.CustomEffects)

	return clone
}

func (contents PotionContents) Effects(durationScale float32) []MobEffectInstance {
	var effects []MobEffectInstance

	if contents.HasPotion {
		definition, valid := contents.Potion.Definition()
		if valid {
			effects = append(effects, definition.Effects...)
		}
	}

	effects = append(effects, cloneMobEffectInstances(contents.CustomEffects)...)

	for index := range effects {
		effects[index] = effects[index].WithScaledDuration(durationScale)
	}

	return effects
}

func (stack ItemStack) PotionContents() (PotionContents, bool) {
	data, exists := stack.component(ItemComponentPotionContents)
	if !exists {
		return PotionContents{}, false
	}

	contents, err := ParsePotionContents(data)
	return contents, err == nil
}

func (stack ItemStack) PotionDurationScale() float32 {
	data, exists := stack.component(ItemComponentPotionDurationScale)
	if !exists || len(data) != 4 {
		return 1
	}

	return math.Float32frombits(binary.BigEndian.Uint32(data))
}

func (stack *ItemStack) SetPotionContents(contents PotionContents) {
	stack.replaceComponent(ItemComponentPotionContents, appendPotionContents(nil, contents))
}

func (stack *ItemStack) SetPotionDurationScale(scale float32) {
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, math.Float32bits(scale))

	stack.replaceComponent(ItemComponentPotionDurationScale, data)
}

func ParsePotionContents(data []byte) (PotionContents, error) {
	decoder := potionComponentDecoder{data: data}

	contents, err := decoder.potionContents()
	if err != nil {
		return PotionContents{}, err
	}

	if decoder.offset != len(data) {
		return PotionContents{}, fmt.Errorf("trailing potion contents data")
	}

	return contents, nil
}

func (decoder *potionComponentDecoder) potionContents() (PotionContents, error) {
	var contents PotionContents

	hasPotion, err := decoder.boolean()
	if err != nil {
		return PotionContents{}, err
	}

	if hasPotion {
		holder, holderErr := decoder.varInt()
		if holderErr != nil {
			return PotionContents{}, holderErr
		}

		potion := Potion(holder)
		if !potion.Valid() {
			return PotionContents{}, fmt.Errorf("invalid potion registry ID %d", holder)
		}

		contents.Potion = potion
		contents.HasPotion = true
	}

	hasColor, err := decoder.boolean()
	if err != nil {
		return PotionContents{}, err
	}

	if hasColor {
		color, colorErr := decoder.int32()
		if colorErr != nil {
			return PotionContents{}, colorErr
		}

		contents.CustomColor = color
		contents.HasCustomColor = true
	}

	effectCount, err := decoder.varInt()
	if err != nil {
		return PotionContents{}, err
	}

	if effectCount < 0 || effectCount > maxPotionCustomEffects {
		return PotionContents{}, fmt.Errorf("invalid custom potion effect count %d", effectCount)
	}

	contents.CustomEffects = make([]MobEffectInstance, effectCount)

	for index := range contents.CustomEffects {
		instance, instanceErr := decoder.mobEffectInstance()
		if instanceErr != nil {
			return PotionContents{}, instanceErr
		}

		contents.CustomEffects[index] = instance
	}

	hasName, err := decoder.boolean()
	if err != nil {
		return PotionContents{}, err
	}

	if hasName {
		name, nameErr := decoder.stringUTF8()
		if nameErr != nil {
			return PotionContents{}, nameErr
		}

		contents.CustomName = name
		contents.HasCustomName = true
	}

	return contents, nil
}

func (decoder *potionComponentDecoder) mobEffectInstance() (MobEffectInstance, error) {
	holder, err := decoder.varInt()
	if err != nil {
		return MobEffectInstance{}, err
	}

	effect := MobEffect(holder)
	if !effect.Valid() {
		return MobEffectInstance{}, fmt.Errorf("invalid mob effect registry ID %d", holder)
	}

	return decoder.mobEffectDetails(effect, 0)
}

func (decoder *potionComponentDecoder) mobEffectDetails(effect MobEffect, depth int) (MobEffectInstance, error) {
	if depth >= maxPotionHiddenEffectDepth {
		return MobEffectInstance{}, fmt.Errorf("custom potion hidden effect chain exceeds %d", maxPotionHiddenEffectDepth)
	}

	amplifier, err := decoder.varInt()
	if err != nil {
		return MobEffectInstance{}, err
	}

	duration, err := decoder.varInt()
	if err != nil {
		return MobEffectInstance{}, err
	}

	ambient, err := decoder.boolean()
	if err != nil {
		return MobEffectInstance{}, err
	}

	visible, err := decoder.boolean()
	if err != nil {
		return MobEffectInstance{}, err
	}

	showIcon, err := decoder.boolean()
	if err != nil {
		return MobEffectInstance{}, err
	}

	instance := NewMobEffectInstance(effect, duration, amplifier, ambient, visible, showIcon)

	hasHidden, err := decoder.boolean()
	if err != nil {
		return MobEffectInstance{}, err
	}

	if hasHidden {
		hidden, hiddenErr := decoder.mobEffectDetails(effect, depth+1)
		if hiddenErr != nil {
			return MobEffectInstance{}, hiddenErr
		}

		instance.HiddenEffect = &hidden
	}

	return instance, nil
}

func (decoder *potionComponentDecoder) boolean() (bool, error) {
	if decoder.offset >= len(decoder.data) {
		return false, fmt.Errorf("truncated potion contents boolean")
	}

	value := decoder.data[decoder.offset] != 0
	decoder.offset++

	return value, nil
}

func (decoder *potionComponentDecoder) int32() (int32, error) {
	if len(decoder.data)-decoder.offset < 4 {
		return 0, fmt.Errorf("truncated potion contents integer")
	}

	value := int32(binary.BigEndian.Uint32(decoder.data[decoder.offset:]))
	decoder.offset += 4

	return value, nil
}

func (decoder *potionComponentDecoder) varInt() (int32, error) {
	value, next, valid := readComponentVarInt(decoder.data, decoder.offset)
	if !valid {
		return 0, fmt.Errorf("invalid potion contents VarInt")
	}

	decoder.offset = next

	return value, nil
}

func (decoder *potionComponentDecoder) stringUTF8() (string, error) {
	length, err := decoder.varInt()
	if err != nil {
		return "", err
	}

	if length < 0 || length > maxPotionCustomNameBytes || int(length) > len(decoder.data)-decoder.offset {
		return "", fmt.Errorf("invalid custom potion name length %d", length)
	}

	raw := decoder.data[decoder.offset : decoder.offset+int(length)]
	decoder.offset += int(length)

	if !utf8.Valid(raw) || utf8.RuneCount(raw) > maxPotionCustomNameRunes {
		return "", fmt.Errorf("invalid custom potion name")
	}

	return string(raw), nil
}

func appendPotionContents(data []byte, contents PotionContents) []byte {
	data = appendComponentBool(data, contents.HasPotion)

	if contents.HasPotion {
		data = appendComponentVarInt(data, int32(contents.Potion))
	}

	data = appendComponentBool(data, contents.HasCustomColor)

	if contents.HasCustomColor {
		var color [4]byte
		binary.BigEndian.PutUint32(color[:], uint32(contents.CustomColor))
		data = append(data, color[:]...)
	}

	data = appendComponentVarInt(data, int32(len(contents.CustomEffects)))

	for _, instance := range contents.CustomEffects {
		data = appendMobEffectInstance(data, instance)
	}

	data = appendComponentBool(data, contents.HasCustomName)

	if contents.HasCustomName {
		data = appendComponentVarInt(data, int32(len(contents.CustomName)))
		data = append(data, contents.CustomName...)
	}

	return data
}

func appendMobEffectInstance(data []byte, instance MobEffectInstance) []byte {
	data = appendComponentVarInt(data, int32(instance.Effect))
	return appendMobEffectDetails(data, instance)
}

func appendMobEffectDetails(data []byte, instance MobEffectInstance) []byte {
	data = appendComponentVarInt(data, instance.Amplifier)
	data = appendComponentVarInt(data, instance.Duration)
	data = appendComponentBool(data, instance.Ambient)
	data = appendComponentBool(data, instance.Visible)
	data = appendComponentBool(data, instance.ShowIcon)
	data = appendComponentBool(data, instance.HiddenEffect != nil)

	if instance.HiddenEffect != nil {
		data = appendMobEffectDetails(data, *instance.HiddenEffect)
	}

	return data
}

func appendComponentBool(data []byte, value bool) []byte {
	if value {
		return append(data, 1)
	}

	return append(data, 0)
}
