package server

import (
	"math"
	"sort"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	explosionRayGridSize        = 16
	explosionRayStep            = 0.3
	explosionRayStepAttenuation = 0.22500001
	explosionPacketRangeSquared = 64 * 64
)

const (
	ExplosionKeepBlocks ExplosionBlockInteraction = iota
	ExplosionDestroyBlocks
	ExplosionDestroyBlocksWithDecay
)

type ExplosionBlockInteraction uint8

type RuntimeExplosion struct {
	Position         game.Position
	Radius           float32
	DirectEntityID   int32
	CauseEntityID    int32
	CauseIsPlayer    bool
	BlockInteraction ExplosionBlockInteraction
	Random           func() float32
}

type RuntimeExplosionResult struct {
	AffectedBlocks  []game.BlockPosition
	DestroyedBlocks []game.BlockPosition
	PlayerKnockback map[int32]game.Velocity
}

type explosionPlayerUpdate struct {
	session   *Session
	survival  playerSurvivalUpdate
	damaged   bool
	player    game.Player
	knockback game.Velocity
}

type explosionLivingUpdate struct {
	damage  runtimeLivingDamageUpdate
	applied bool
}

func (r *Runtime) Explode(explosion RuntimeExplosion) RuntimeExplosionResult {
	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	result, playerUpdates, livingUpdates := r.explodeLocked(explosion)

	mutations := r.takeRuntimeBlockMutationsLocked()

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

	r.sendExplosionEntityUpdates(playerUpdates, livingUpdates)
	r.completeRuntimeBlockMutations(mutations)

	return result
}

func (r *Runtime) explodeLocked(explosion RuntimeExplosion) (RuntimeExplosionResult, []explosionPlayerUpdate, []explosionLivingUpdate) {
	if explosion.Radius <= 0 || math.IsNaN(float64(explosion.Radius)) {
		return RuntimeExplosionResult{}, nil, nil
	}

	random := explosion.Random
	if random == nil {
		random = r.nextEntityRandom
	}

	affected := r.explosionAffectedBlocks(explosion.Position, explosion.Radius, random)

	result := RuntimeExplosionResult{AffectedBlocks: affected, PlayerKnockback: make(map[int32]game.Velocity)}

	playerUpdates, livingUpdates := r.damageExplosionEntities(explosion, &result)

	if explosion.BlockInteraction != ExplosionKeepBlocks {
		result.DestroyedBlocks = r.destroyExplosionBlocksLocked(affected, explosion, random)
	}

	r.sendExplosionPackets(explosion, result)

	return result, playerUpdates, livingUpdates
}

func (r *Runtime) explosionAffectedBlocks(center game.Position, radius float32, random func() float32) []game.BlockPosition {
	affected := make(map[game.BlockPosition]struct{})
	minimumY := int32(protocol.OverworldMinY)
	maximumY := minimumY + protocol.OverworldSectionCount*game.ChunkWidth - 1

	for rayX := range explosionRayGridSize {
		for rayY := range explosionRayGridSize {
			for rayZ := range explosionRayGridSize {
				if rayX != 0 && rayX != explosionRayGridSize-1 && rayY != 0 && rayY != explosionRayGridSize-1 && rayZ != 0 && rayZ != explosionRayGridSize-1 {
					continue
				}

				directionX := float64(rayX)/15*2 - 1
				directionY := float64(rayY)/15*2 - 1
				directionZ := float64(rayZ)/15*2 - 1

				length := math.Sqrt(directionX*directionX + directionY*directionY + directionZ*directionZ)

				directionX /= length
				directionY /= length
				directionZ /= length

				power := radius * (0.7 + random()*0.6)
				position := center

				for power > 0 {
					blockPosition := game.BlockPosition{X: int32(math.Floor(position.X)), Y: int32(math.Floor(position.Y)), Z: int32(math.Floor(position.Z))}
					if blockPosition.Y < minimumY || blockPosition.Y > maximumY {
						break
					}

					block := r.World.BlockAt(blockPosition)

					definition, defined := block.Definition()
					fluidResistance := r.World.FluidAt(blockPosition).ExplosionResistance()

					if defined || fluidResistance > 0 {
						blockResistance := float32(0)

						if defined {
							blockResistance = definition.ExplosionResistance
						}

						power -= (max(blockResistance, fluidResistance) + 0.3) * 0.3
					}

					if power > 0 && block != game.Air {
						affected[blockPosition] = struct{}{}
					}

					position.X += directionX * explosionRayStep
					position.Y += directionY * explosionRayStep
					position.Z += directionZ * explosionRayStep
					power -= explosionRayStepAttenuation
				}
			}
		}
	}

	positions := make([]game.BlockPosition, 0, len(affected))

	for position := range affected {
		positions = append(positions, position)
	}

	sort.Slice(positions, func(left, right int) bool {
		if positions[left].X != positions[right].X {
			return positions[left].X < positions[right].X
		}

		if positions[left].Y != positions[right].Y {
			return positions[left].Y < positions[right].Y
		}

		return positions[left].Z < positions[right].Z
	})

	return positions
}

func (r *Runtime) damageExplosionEntities(explosion RuntimeExplosion, result *RuntimeExplosionResult) ([]explosionPlayerUpdate, []explosionLivingUpdate) {
	diameter := float64(explosion.Radius * 2)
	damageType := r.explosionDamageType(explosion)

	playerUpdates := make([]explosionPlayerUpdate, 0)

	for _, session := range r.sessionView() {
		player := session.snapshotPlayer()

		if (explosion.DirectEntityID != 0 && player.EntityID == explosion.DirectEntityID) || player.GameMode == game.GameModeSpectator {
			continue
		}

		knockback, damage, hit := r.explosionImpact(explosion.Position, diameter, player.Position, player.EyePosition(), player.CollisionBox(), player.ArmorAttributes().KnockbackResistance)
		if !hit {
			continue
		}

		survival, damaged := r.damagePlayerLocked(session, game.Damage{Type: damageType, Amount: damage, CauseEntityID: explosion.CauseEntityID, DirectEntityID: explosion.DirectEntityID, SourcePosition: &explosion.Position})

		player, _ = session.updatePlayerState(func(player *game.Player) bool {
			player.Velocity.X += knockback.X
			player.Velocity.Y += knockback.Y
			player.Velocity.Z += knockback.Z

			return knockback != (game.Velocity{})
		})

		result.PlayerKnockback[player.EntityID] = knockback
		playerUpdates = append(playerUpdates, explosionPlayerUpdate{session: session, survival: survival, damaged: damaged, player: player, knockback: knockback})
	}

	livingUpdates := make([]explosionLivingUpdate, 0)

	entities := r.appendRuntimeEntities(nil)

	for _, entity := range entities {
		living, valid := entity.(RuntimeLivingEntity)
		if !valid {
			continue
		}

		state := entity.RuntimeEntityState()
		livingState := living.RuntimeLivingState()

		state.mu.RLock()
		entityID := state.ID
		position := state.Position
		removed := state.Removed
		state.mu.RUnlock()

		if (explosion.DirectEntityID != 0 && entityID == explosion.DirectEntityID) || removed {
			continue
		}

		origin := position

		if eye, available := entity.(runtimeLivingEyeHeight); available {
			origin.Y += eye.RuntimeLivingEyeHeight()
		} else {
			origin.Y += livingState.Height * 0.85
		}

		knockback, damage, hit := r.explosionImpact(explosion.Position, diameter, position, origin, livingState.CollisionBox(position), livingState.KnockbackResistance)
		if !hit {
			continue
		}

		update, applied := r.damageRuntimeLivingEntityLocked(living, game.Damage{Type: damageType, Amount: damage, CauseEntityID: explosion.CauseEntityID, DirectEntityID: explosion.DirectEntityID, SourcePosition: &explosion.Position})

		state.mu.Lock()

		if !state.Removed {
			livingState.Velocity.X += knockback.X
			livingState.Velocity.Y += knockback.Y
			livingState.Velocity.Z += knockback.Z

			state.movementSyncDirty = knockback != (game.Velocity{}) || state.movementSyncDirty
		}

		state.mu.Unlock()

		livingUpdates = append(livingUpdates, explosionLivingUpdate{damage: update, applied: applied})
	}

	return playerUpdates, livingUpdates
}

func (r *Runtime) explosionDamageType(explosion RuntimeExplosion) game.DamageType {
	if explosion.DirectEntityID == 0 || explosion.CauseEntityID == 0 || !explosion.CauseIsPlayer {
		return game.DamageExplosion
	}

	return game.DamagePlayerExplosion
}

func (r *Runtime) explosionImpact(center game.Position, diameter float64, position, origin game.Position, box game.AABB, resistance float32) (game.Velocity, float32, bool) {
	distanceX := position.X - center.X
	distanceY := position.Y - center.Y
	distanceZ := position.Z - center.Z

	distance := math.Sqrt(distanceX*distanceX+distanceY*distanceY+distanceZ*distanceZ) / diameter
	if distance > 1 {
		return game.Velocity{}, 0, false
	}

	directionX := origin.X - center.X
	directionY := origin.Y - center.Y
	directionZ := origin.Z - center.Z

	directionLength := math.Sqrt(directionX*directionX + directionY*directionY + directionZ*directionZ)
	if directionLength == 0 {
		return game.Velocity{}, 0, false
	}

	exposure := r.explosionExposure(center, box)
	power := (1 - distance) * exposure
	damage := float32((power*power+power)/2*7*diameter + 1)
	knockbackPower := power * (1 - float64(resistance))
	knockback := game.Velocity{
		X: directionX / directionLength * knockbackPower,
		Y: directionY / directionLength * knockbackPower,
		Z: directionZ / directionLength * knockbackPower,
	}

	return knockback, damage, true
}

func (r *Runtime) explosionExposure(center game.Position, box game.AABB) float64 {
	incrementX := 1 / ((box.MaxX-box.MinX)*2 + 1)
	incrementY := 1 / ((box.MaxY-box.MinY)*2 + 1)
	incrementZ := 1 / ((box.MaxZ-box.MinZ)*2 + 1)

	offsetX := (1 - math.Floor(1/incrementX)*incrementX) / 2
	offsetZ := (1 - math.Floor(1/incrementZ)*incrementZ) / 2

	var (
		hits  int
		count int
	)

	for sampleX := 0.0; sampleX <= 1; sampleX += incrementX {
		for sampleY := 0.0; sampleY <= 1; sampleY += incrementY {
			for sampleZ := 0.0; sampleZ <= 1; sampleZ += incrementZ {
				point := game.Position{
					X: box.MinX + (box.MaxX-box.MinX)*sampleX + offsetX,
					Y: box.MinY + (box.MaxY-box.MinY)*sampleY,
					Z: box.MinZ + (box.MaxZ-box.MinZ)*sampleZ + offsetZ,
				}
				count++

				if !r.explosionSegmentBlocked(point, center) {
					hits++
				}
			}
		}
	}

	if count == 0 {
		return 0
	}

	return float64(hits) / float64(count)
}

func (r *Runtime) explosionSegmentBlocked(start, end game.Position) bool {
	minimumX := int32(math.Floor(min(start.X, end.X)))
	minimumY := int32(math.Floor(min(start.Y, end.Y)))
	minimumZ := int32(math.Floor(min(start.Z, end.Z)))
	maximumX := int32(math.Floor(max(start.X, end.X)))
	maximumY := int32(math.Floor(max(start.Y, end.Y)))
	maximumZ := int32(math.Floor(max(start.Z, end.Z)))

	for x := minimumX; x <= maximumX; x++ {
		for y := minimumY; y <= maximumY; y++ {
			for z := minimumZ; z <= maximumZ; z++ {
				position := game.BlockPosition{X: x, Y: y, Z: z}

				var boxBuffer [7]game.AABB

				for _, box := range r.World.BlockAt(position).AppendCollisionBoxes(boxBuffer[:0], position) {
					if segmentIntersectsBox(start, end, box) {
						return true
					}
				}
			}
		}
	}

	return false
}

func (r *Runtime) destroyExplosionBlocksLocked(affected []game.BlockPosition, explosion RuntimeExplosion, random func() float32) []game.BlockPosition {
	shuffled := append([]game.BlockPosition(nil), affected...)
	primedTnt := make(map[game.BlockPosition]struct{})

	for index := len(shuffled) - 1; index > 0; index-- {
		swap := int(random() * float32(index+1))
		swap = min(swap, index)

		shuffled[index], shuffled[swap] = shuffled[swap], shuffled[index]
	}

	changes := make([]game.BlockChange, 0, len(shuffled))

	for _, position := range shuffled {
		block := r.World.BlockAt(position)
		if sameBlockType(block, game.Tnt) {
			primedTnt[position] = struct{}{}
		}

		if block != game.Air {
			changes = append(changes, game.BlockChange{Position: position, Replacement: game.Air})
		}
	}

	if len(changes) == 0 {
		return nil
	}

	requiredChanges := len(changes)
	changes = r.withStructuralNeighborChanges(changes)

	result, delivery, err := r.mutateBlocksLocked(nil, blockMutationExplosion, changes, requiredChanges, true, false, true, true, true)
	if err != nil || !result.Changed {
		return nil
	}

	for index := range delivery.records {
		record := &delivery.records[index]
		if record.cause != blockMutationExplosionBreak {
			continue
		}

		if _, prime := primedTnt[record.change.Position]; prime {
			record.lootContext = blockLootNone

			continue
		}

		record.lootContext = blockLootNoBreaker

		if explosion.BlockInteraction == ExplosionDestroyBlocksWithDecay {
			record.lootContext = blockLootExplosion
			record.lootSurvival = 1 / explosion.Radius
			record.lootRandom = random
		}
	}

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, queuedBlockMutation{result: result, delivery: delivery})

	destroyed := make([]game.BlockPosition, 0, requiredChanges)

	for _, change := range result.Changes {
		if change.Replacement == game.Air {
			destroyed = append(destroyed, change.Position)

			if _, prime := primedTnt[change.Position]; prime {
				fuse := int32(random()*20) + 10
				fuse = min(fuse, 29)

				r.primeTnt(change.Position, explosion.CauseEntityID, explosion.CauseIsPlayer, fuse)
			}
		}
	}

	return destroyed
}

func (r *Runtime) sendExplosionPackets(explosion RuntimeExplosion, result RuntimeExplosionResult) {
	for _, session := range r.sessionView() {
		player := session.snapshotPlayer()

		distanceX := player.Position.X - explosion.Position.X
		distanceY := player.Position.Y - explosion.Position.Y
		distanceZ := player.Position.Z - explosion.Position.Z

		if distanceX*distanceX+distanceY*distanceY+distanceZ*distanceZ >= explosionPacketRangeSquared {
			continue
		}

		knockback, hasKnockback := result.PlayerKnockback[player.EntityID]
		particle := int32(22)

		if explosion.BlockInteraction == ExplosionKeepBlocks || explosion.Radius < 2 {
			particle = 23
		}

		packet := protocol.Explode{
			X: explosion.Position.X, Y: explosion.Position.Y, Z: explosion.Position.Z,
			Radius: explosion.Radius, Blocks: int32(len(result.AffectedBlocks)),
			PlayerKnockback: knockback, HasPlayerKnockback: hasKnockback,
			Particle: particle, Sound: protocol.SoundEventHolder{RegistryID: 668},
			BlockParticles: []protocol.ExplosionParticleInfo{
				{Particle: 57, Scaling: 0.5, Speed: 1, Weight: 1},
				{Particle: 60, Scaling: 1, Speed: 1, Weight: 1},
			},
		}

		err := session.writePacket(protocol.ClientboundExplodeID, packet)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize explosion: %v\n", err)
		}
	}
}

func (r *Runtime) sendExplosionEntityUpdates(players []explosionPlayerUpdate, living []explosionLivingUpdate) {
	for _, update := range players {
		if update.damaged {
			r.sendPlayerSurvivalUpdate(update.session, update.survival)
		}

		if update.knockback != (game.Velocity{}) {
			r.sendPlayerKnockback(update.player)
		}
	}

	for _, update := range living {
		if update.applied {
			r.sendRuntimeLivingDamageUpdate(update.damage)
		}
	}
}
