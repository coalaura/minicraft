package babel

func checker(x, z, scale int64) bool {
	return (floorDiv(x, scale)+floorDiv(z, scale))&1 == 0
}

func centerDistance(value int64) int64 {
	return min(absolute(value-23), absolute(value-24))
}

func cityOrigins(seed int64) (int64, int64) {
	mixed := mix64(uint64(seed) ^ 0x243f6a8885a308d3)
	other := mix64(mixed ^ 0x13198a2e03707344)

	return int64(mixed % uint64(districtScale)), int64(other % uint64(districtScale))
}

func hashCoordinates(seed int64, x, z int64, salt uint64) uint64 {
	value := mix64(uint64(seed) ^ salt)
	value ^= mix64(uint64(x) + 0x9e3779b97f4a7c15)
	value = mix64(value)
	value ^= mix64(uint64(z) + 0xbf58476d1ce4e5b9)

	return mix64(value)
}

func mix64(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	value ^= value >> 31

	return value
}

func nearestGridIndex(value, scale int64) int64 {
	cell := floorDiv(value, scale)
	local := positiveRemainder(value, scale)
	if local > scale/2 {
		cell++
	}

	return cell
}

func gridSignedOffset(value, scale int64) int64 {
	local := positiveRemainder(value, scale)
	if local > scale/2 {
		return local - scale
	}

	return local
}

func floorDiv(value, divisor int64) int64 {
	quotient := value / divisor
	if value%divisor < 0 {
		quotient--
	}

	return quotient
}

func positiveRemainder(value, divisor int64) int64 {
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}

	return remainder
}

func absolute(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
