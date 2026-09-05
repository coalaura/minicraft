package game

import "math"

const (
	LivingHurtCooldownTicks     = 20
	LivingHurtCooldownThreshold = 10
	LivingDeathDurationTicks    = 20
	LivingKnockbackMinDirection = 1.0e-5
	LivingKnockbackRandomScale  = 0.01
)

const (
	DamageFall DamageType = iota
	DamageDrown
	DamageInFire
	DamageLava
	DamageOnFire
	DamageOutOfWorld
	DamageGenericKill
	DamageStarve
	DamageMagic
	DamagePlayerAttack
)

type DamageType uint8

type LivingState struct {
	Health             float32
	MaxHealth          float32
	Absorption         float32
	InvulnerableTime   int32
	LastHurt           float32
	ActiveEffects      ActiveMobEffects
	RemainingFireTicks int32
	Dead               bool
	DeathTime          int32
}

type Damage struct {
	Type           DamageType
	Amount         float32
	CauseEntityID  int32
	DirectEntityID int32
	SourcePosition *Position
}

type DamageTraits struct {
	RegistryID              int32
	Fire                    bool
	BypassesArmor           bool
	DamagesArmor            bool
	BypassesEffects         bool
	BypassesResistance      bool
	BypassesInvulnerability bool
	BypassesCooldown        bool
}

type LivingDefense struct {
	Armor     int32
	Toughness float32
}

type LivingDamageResult struct {
	Applied      bool
	FullHurt     bool
	Died         bool
	HealthDamage float32
	Absorbed     float32
}

type ArmorDamageHandler func(float32)

type LivingDefenseProvider func() LivingDefense

type KnockbackRandom func() float32

func (state *LivingState) Reset(maxHealth float32) {
	state.Health = maxHealth
	state.MaxHealth = maxHealth
	state.Absorption = 0
	state.InvulnerableTime = 0
	state.LastHurt = 0
	state.ActiveEffects.Clear()
	state.RemainingFireTicks = 0
	state.Dead = false
	state.DeathTime = 0
}

func (state *LivingState) TickHurtCooldown() {
	if state.InvulnerableTime > 0 {
		state.InvulnerableTime--
	}
}

func (state *LivingState) TickDeath() bool {
	if !state.Dead {
		return false
	}

	state.DeathTime++

	return state.DeathTime >= LivingDeathDurationTicks
}

func (damageType DamageType) Traits() DamageTraits {
	switch damageType {
	case DamageDrown:
		return DamageTraits{RegistryID: 6, BypassesArmor: true}
	case DamageFall:
		return DamageTraits{RegistryID: 10, BypassesArmor: true}
	case DamageGenericKill:
		return DamageTraits{RegistryID: 19, BypassesArmor: true, BypassesResistance: true, BypassesInvulnerability: true}
	case DamageInFire:
		return DamageTraits{RegistryID: 21, Fire: true, DamagesArmor: true}
	case DamageLava:
		return DamageTraits{RegistryID: 24, Fire: true, DamagesArmor: true}
	case DamageOnFire:
		return DamageTraits{RegistryID: 31, Fire: true, BypassesArmor: true}
	case DamageOutOfWorld:
		return DamageTraits{RegistryID: 32, BypassesArmor: true, BypassesResistance: true, BypassesInvulnerability: true}
	case DamageStarve:
		return DamageTraits{RegistryID: 40, BypassesArmor: true, BypassesEffects: true}
	case DamageMagic:
		return DamageTraits{RegistryID: 27, BypassesArmor: true}
	case DamagePlayerAttack:
		return DamageTraits{RegistryID: 34, DamagesArmor: true}
	default:
		return DamageTraits{RegistryID: 18, BypassesArmor: true}
	}
}

func ResolveLivingDamage(state *LivingState, damage Damage, defense LivingDefenseProvider, damageArmor ArmorDamageHandler) LivingDamageResult {
	if state.Dead || damage.Amount <= 0 {
		return LivingDamageResult{}
	}

	traits := damage.Type.Traits()

	if traits.Fire {
		_, fireResistance := state.ActiveEffects.Find(MobEffectFireResistance)
		if fireResistance {
			return LivingDamageResult{}
		}
	}

	amount := damage.Amount
	if math.IsNaN(float64(amount)) || math.IsInf(float64(amount), 0) {
		amount = math.MaxFloat32
	}

	incomingAmount := amount

	result := LivingDamageResult{Applied: true}

	if state.InvulnerableTime > LivingHurtCooldownThreshold && !traits.BypassesCooldown {
		if amount <= state.LastHurt {
			return LivingDamageResult{}
		}

		amount -= state.LastHurt
		state.LastHurt = incomingAmount
	} else {
		state.LastHurt = incomingAmount
		state.InvulnerableTime = LivingHurtCooldownTicks
		result.FullHurt = true
	}

	if traits.DamagesArmor && damageArmor != nil {
		damageArmor(amount)
	}

	if !traits.BypassesArmor {
		attributes := LivingDefense{}

		if defense != nil {
			attributes = defense()
		}

		amount = DamageAfterArmorAbsorb(amount, attributes.Armor, attributes.Toughness)
	}

	if !traits.BypassesEffects && !traits.BypassesResistance {
		resistance, active := state.ActiveEffects.Find(MobEffectResistance)
		if active {
			multiplier := max(float32(25-5*(resistance.Amplifier+1))/25, 0)
			amount *= multiplier
		}
	}

	result.Absorbed = min(amount, state.Absorption)
	state.Absorption -= result.Absorbed
	amount -= result.Absorbed

	previousHealth := state.Health
	state.Health = max(0, state.Health-amount)
	result.HealthDamage = previousHealth - state.Health

	if state.Health == 0 {
		state.Dead = true
		result.Died = true
	}

	return result
}

func ApplyLivingKnockback(velocity *Velocity, onGround bool, resistance float32, directionX, directionZ, strength float64, random KnockbackRandom) bool {
	resistance = min(max(resistance, 0), 1)
	strength *= 1 - float64(resistance)

	if strength <= 0 {
		return false
	}

	for directionX*directionX+directionZ*directionZ < LivingKnockbackMinDirection {
		directionX = float64(random()-random()) * LivingKnockbackRandomScale
		directionZ = float64(random()-random()) * LivingKnockbackRandomScale
	}

	length := math.Hypot(directionX, directionZ)
	directionX /= length
	directionZ /= length

	velocity.X = velocity.X/2 - directionX*strength

	if onGround {
		velocity.Y = min(0.4, velocity.Y/2+strength)
	}

	velocity.Z = velocity.Z/2 - directionZ*strength

	return true
}
