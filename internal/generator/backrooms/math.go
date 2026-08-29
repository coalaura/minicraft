package backrooms

func rectanglePerimeterAt(x, z, x0, x1, z0, z1 int64) bool {
	if x < x0 || x > x1 || z < z0 || z > z1 {
		return false
	}

	return x == x0 || x == x1 || z == z0 || z == z1
}

func mergeStructure(first, second structure) structure {
	if first > second {
		return first
	}

	return second
}

func zoneCoordinate(value int64) (int64, int64) {
	shifted := value + zoneSize/2
	zone := floorDiv(shifted, zoneSize)
	local := shifted - zone*zoneSize

	return zone, local
}

func floorDiv(value, divisor int64) int64 {
	quotient := value / divisor

	if value%divisor < 0 {
		quotient--
	}

	return quotient
}

func floorMod(value, divisor int64) int64 {
	remainder := value % divisor
	if remainder < 0 {
		remainder += divisor
	}

	return remainder
}

func coordinateHash(seed, x, z int64, salt uint64) uint64 {
	hash := uint64(seed) ^ salt
	hash ^= mix64(uint64(x) + 0x9e3779b97f4a7c15)
	hash ^= mix64(uint64(z) + 0xbf58476d1ce4e5b9)

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

func between(value, first, second int64) bool {
	minimum := min(first, second)
	maximum := max(first, second)

	return value >= minimum && value <= maximum
}

func clamp(value, minimum, maximum int64) int64 {
	return max(minimum, min(value, maximum))
}

func abs32(value int32) int32 {
	if value < 0 {
		return -value
	}

	return value
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}
