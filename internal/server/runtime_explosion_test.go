package server

import (
	"math"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

func TestExplosionAffectedBlocksAreDeterministicAndSynchronizeClients(t *testing.T) {
	world := &game.World{}

	position := game.BlockPosition{Y: 64}

	world.SetBlock(position, game.Stone)

	runtime := NewRuntime(world)

	session, connection := newBlockMutationTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Observer", game.GameModeSpectator)

	session.Player.Position = game.Position{X: 0.5, Y: 64.5, Z: 0.5}

	markChunkLoaded(session, position)

	joinTestSession(t, runtime, session)

	connection.reset()

	explosion := RuntimeExplosion{
		Position:         game.Position{X: 0.5, Y: 64.5, Z: 0.5},
		Radius:           2,
		BlockInteraction: ExplosionDestroyBlocks,
		Random: func() float32 {
			return 1
		},
	}

	results := make(chan RuntimeExplosionResult, 1)

	go func() {
		result := runtime.Explode(explosion)
		results <- result
	}()

	var result RuntimeExplosionResult

	select {
	case result = <-results:
	case <-time.After(time.Second):
		t.Fatal("explode did not complete while applying authoritative block mutation")
	}

	if len(result.AffectedBlocks) != 1 || result.AffectedBlocks[0] != position {
		t.Fatalf("affected blocks = %+v, want [%+v]", result.AffectedBlocks, position)
	}

	if len(result.DestroyedBlocks) != 1 || result.DestroyedBlocks[0] != position || world.BlockAt(position) != game.Air {
		t.Fatalf("destroyed blocks = %+v, block after explosion = %d", result.DestroyedBlocks, world.BlockAt(position))
	}

	packetIDs := connection.packetIDs(t)
	if len(packetIDs) < 3 {
		t.Fatalf("packet ids = %v, want explosion and block synchronization packets", packetIDs)
	}

	assertPacketIDs(t, packetIDs[:3], []int32{
		protocol.ClientboundExplodeID,
		protocol.ClientboundBlockUpdateID,
		protocol.ClientboundLevelEventID,
	})

	assertBlockUpdate(t, connection.packets(t)[1], position, protocol.AirBlockState)
	assertLevelEvent(t, connection.packets(t)[2], protocol.LevelEventBlockBreak, position, protocol.StoneBlockState, false)
}

func TestExplosionDamagesAndKnocksBackLivingEntityByDistance(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	entity := spawnTestRuntimeLivingEntity(runtime, game.Position{X: 1}, 20)

	runtime.Explode(RuntimeExplosion{Position: game.Position{}, Radius: 2, Random: func() float32 {
		return 0
	}})

	entity.State.mu.RLock()
	health := entity.Living.Health
	velocity := entity.Living.Velocity
	entity.State.mu.RUnlock()

	if math.Abs(float64(health-0.625)) > 1e-4 {
		t.Fatalf("living health after explosion = %v, want 0.625", health)
	}

	if velocity.X <= 0 || velocity.Y <= 0 || velocity.Z != 0 {
		t.Fatalf("living explosion knockback = %+v, want positive X and Y", velocity)
	}
}

func TestExplosionSolidBlockReducesExposure(t *testing.T) {
	world := &game.World{}

	runtime := NewRuntime(world)

	center := game.Position{X: 0.5, Y: 0.5, Z: 0.5}
	box := game.AABB{MinX: 2.7, MinY: 0, MinZ: -0.3, MaxX: 3.3, MaxY: 1.8, MaxZ: 0.3}

	unobstructed := runtime.explosionExposure(center, box)

	world.SetBlock(game.BlockPosition{X: 1, Y: 0, Z: 0}, game.Stone)

	obstructed := runtime.explosionExposure(center, box)

	if unobstructed != 1 {
		t.Fatalf("unobstructed exposure = %v, want 1", unobstructed)
	}

	if obstructed >= unobstructed {
		t.Fatalf("obstructed exposure = %v, want less than %v", obstructed, unobstructed)
	}
}
