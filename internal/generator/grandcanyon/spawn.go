package grandcanyon

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	spawnSearchRadius = int32(384)
	spawnSearchStep   = int32(16)
	spawnRefineRadius = int32(15)
)

type spawnCandidate struct {
	x     int32
	z     int32
	score float64
	valid bool
}

func (Generator) Spawn(seed int64) game.Position {
	best := spawnCandidate{
		score: math.Inf(-1),
	}

	for z := -spawnSearchRadius; z <= spawnSearchRadius; z += spawnSearchStep {
		for x := -spawnSearchRadius; x <= spawnSearchRadius; x += spawnSearchStep {
			candidate := scoreSpawn(
				seed,
				x,
				z,
			)

			if candidate.valid && candidate.score > best.score {
				best = candidate
			}
		}
	}

	if !best.valid {
		terrain := columnAt(seed, 0, 0)

		return game.Position{
			X: 0.5,
			Y: float64(terrain.height + 1),
			Z: 0.5,
		}
	}

	refined := best

	for z := best.z - spawnRefineRadius; z <= best.z+spawnRefineRadius; z++ {
		for x := best.x - spawnRefineRadius; x <= best.x+spawnRefineRadius; x++ {
			candidate := scoreSpawn(
				seed,
				x,
				z,
			)

			if candidate.valid && candidate.score > refined.score {
				refined = candidate
			}
		}
	}

	terrain := columnAt(
		seed,
		refined.x,
		refined.z,
	)

	return game.Position{
		X: float64(refined.x) + 0.5,
		Y: float64(terrain.height + 1),
		Z: float64(refined.z) + 0.5,
	}
}

func scoreSpawn(seed int64, x, z int32) spawnCandidate {
	terrain := columnAt(seed, x, z)

	if terrain.height < 155 || terrain.canyonStrength > 0.16 || terrain.riverStrength > 0.04 {
		return spawnCandidate{}
	}

	north := columnAt(seed, x, z-2).height
	south := columnAt(seed, x, z+2).height
	west := columnAt(seed, x-2, z).height
	east := columnAt(seed, x+2, z).height

	localSlope := max(
		abs32(terrain.height-north),
		abs32(terrain.height-south),
		abs32(terrain.height-west),
		abs32(terrain.height-east),
	)

	if localSlope > 2 {
		return spawnCandidate{}
	}

	viewNorth := columnAt(seed, x, z-16).height
	viewSouth := columnAt(seed, x, z+16).height
	viewWest := columnAt(seed, x-16, z).height
	viewEast := columnAt(seed, x+16, z).height

	minView := min(viewNorth, viewSouth, viewWest, viewEast)
	viewDrop := float64(terrain.height - minView)

	distance := math.Hypot(float64(x), float64(z))

	score := float64(terrain.height)*0.40 -
		float64(localSlope)*10.0 -
		distance*0.020 +
		viewDrop*0.85

	return spawnCandidate{
		x:     x,
		z:     z,
		score: score,
		valid: true,
	}
}
