package natural

import (
	"math"
	"math/bits"
)

func fractalNoise(seed int64, x, z float64, octaves int, salt uint64) float64 {
	amplitude := 1.0
	frequency := 1.0
	total := 0.0
	weight := 0.0

	for octave := range octaves {
		octaveSalt := salt + uint64(octave)*0x9e3779b97f4a7c15
		total += valueNoise(seed, x*frequency, z*frequency, octaveSalt) * amplitude
		weight += amplitude
		frequency *= 2
		amplitude *= 0.5
	}

	return total / weight
}

func valueNoise(seed int64, x, z float64, salt uint64) float64 {
	x0 := int64(math.Floor(x))
	z0 := int64(math.Floor(z))

	x1 := x0 + 1
	z1 := z0 + 1

	tx := fade(x - float64(x0))
	tz := fade(z - float64(z0))

	a := latticeValue(seed, x0, z0, salt)
	b := latticeValue(seed, x1, z0, salt)
	c := latticeValue(seed, x0, z1, salt)
	d := latticeValue(seed, x1, z1, salt)

	top := lerp(a, b, tx)
	bottom := lerp(c, d, tx)

	return lerp(top, bottom, tz)
}

func latticeValue(seed int64, x, z int64, salt uint64) float64 {
	hash := coordinateHash(seed, x, 0, z, salt)
	unit := float64(hash>>11) * (1.0 / (1 << 53))

	return unit*2 - 1
}

func coordinateHash(seed int64, x, y, z int64, salt uint64) uint64 {
	hash := uint64(seed) ^ salt
	hash ^= uint64(x) * 0x9e3779b97f4a7c15
	hash = bits.RotateLeft64(hash, 21)
	hash ^= uint64(y) * 0xbf58476d1ce4e5b9
	hash = bits.RotateLeft64(hash, 17)
	hash ^= uint64(z) * 0x94d049bb133111eb

	return mix64(hash)
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}

func fade(value float64) float64 {
	return value * value * value * (value*(value*6-15) + 10)
}

func smoothstep(edge0, edge1, value float64) float64 {
	if edge0 == edge1 {
		return 0
	}

	t := clamp01((value - edge0) / (edge1 - edge0))

	return t * t * (3 - 2*t)
}

func lerp(first, second, amount float64) float64 {
	return first + (second-first)*amount
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func floorDiv(value, divisor int32) int32 {
	quotient := value / divisor

	if value%divisor < 0 {
		quotient--
	}

	return quotient
}

func abs32(value int32) int32 {
	if value < 0 {
		return -value
	}

	return value
}
