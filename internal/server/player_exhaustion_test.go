package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type playerExhaustionModeTestCase struct {
	name string
	mode game.GameMode
}

func TestSurvivalBlockBreakAddsExhaustion(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	world := &game.World{Generator: blockMutationTestGenerator{block: game.Dirt}}

	runtime := NewRuntime(world)

	session, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemAir)

	startMining(t, session, position, 1)

	for range 10 {
		runtime.Tick()
	}

	stopMining(t, session, position, 2)

	player := session.snapshotPlayer()
	if player.Exhaustion != float32(0.005) {
		t.Fatalf("exhaustion after successful survival block break = %v, want 0.005", player.Exhaustion)
	}
}

func TestRejectedAndAbortedMiningDoesNotAddExhaustion(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	t.Run("rejected", func(t *testing.T) {
		world := &game.World{Generator: blockMutationTestGenerator{block: game.Dirt}}

		runtime := NewRuntime(world)

		runtime.AllowBlockBreaking = false

		session, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemAir)

		startMining(t, session, position, 1)

		player := session.snapshotPlayer()
		if player.Exhaustion != 0 {
			t.Fatalf("exhaustion after rejected mining = %v, want 0", player.Exhaustion)
		}
	})

	t.Run("aborted", func(t *testing.T) {
		world := &game.World{Generator: blockMutationTestGenerator{block: game.Dirt}}

		runtime := NewRuntime(world)

		session, _ := newMiningTestSession(t, runtime, position, game.GameModeSurvival, game.ItemAir)

		startMining(t, session, position, 1)

		runtime.abortDestroyingBlock(session)

		for range 10 {
			runtime.Tick()
		}

		player := session.snapshotPlayer()
		if player.Exhaustion != 0 {
			t.Fatalf("exhaustion after aborted mining = %v, want 0", player.Exhaustion)
		}
	})
}

func TestPlayerMovementExhaustionUsesRoundedDistances(t *testing.T) {
	t.Run("sprinting uses rounded horizontal distance", func(t *testing.T) {
		runtime := NewRuntime(&game.World{})

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		session.Player.Sprinting = true

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.X = 1.234
		})

		player := session.snapshotPlayer()
		if player.Exhaustion != float32(123)*0.001 {
			t.Fatalf("sprint exhaustion = %v, want 0.123", player.Exhaustion)
		}
	})

	t.Run("swimming uses rounded full distance", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 0}, game.Water)
		world.SetBlock(game.BlockPosition{Y: 1}, game.Water)
		world.SetBlock(game.BlockPosition{X: 3, Y: 4}, game.Water)
		world.SetBlock(game.BlockPosition{X: 3, Y: 5}, game.Water)

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		session.Player.Position = game.Position{X: 0.5, Y: 0.1, Z: 0.5}
		session.Player.Sprinting = true

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position = game.Position{X: 3.5, Y: 4.1, Z: 0.5}
		})

		player := session.snapshotPlayer()
		if !player.Swimming || player.Exhaustion != float32(500)*0.0001 {
			t.Fatalf("swimming state = %t, exhaustion = %v, want true and 0.05", player.Swimming, player.Exhaustion)
		}
	})

	t.Run("in water uses rounded distance", func(t *testing.T) {
		world := &game.World{}

		world.SetBlock(game.BlockPosition{Y: 0}, game.Water)
		world.SetBlock(game.BlockPosition{X: 1, Y: 0}, game.Water)

		runtime := NewRuntime(world)

		session, _ := newMovementTestSession(runtime, "00010203-0405-0607-0809-0a0b0c0d0e0f", "Player")

		session.Player.Position = game.Position{X: 0.5, Y: 0.1, Z: 0.5}

		runtime.updatePlayerMovement(session, func(player *game.Player) {
			player.Position.X = 1.734
		})

		player := session.snapshotPlayer()
		if player.Swimming || player.Exhaustion != float32(123)*0.0001 {
			t.Fatalf("in-water state = swimming %t, exhaustion %v, want false and 0.0123", player.Swimming, player.Exhaustion)
		}
	})
}

func TestCreativeAndSpectatorActionsDoNotAddExhaustion(t *testing.T) {
	position := game.BlockPosition{Y: 70}

	modes := []playerExhaustionModeTestCase{
		{name: "creative", mode: game.GameModeCreative},
		{name: "spectator", mode: game.GameModeSpectator},
	}

	for _, test := range modes {
		t.Run(test.name, func(t *testing.T) {
			world := &game.World{Generator: blockMutationTestGenerator{block: game.Dirt}}

			runtime := NewRuntime(world)

			session, _ := newMiningTestSession(t, runtime, position, test.mode, game.ItemAir)

			session.Player.Sprinting = true

			runtime.updatePlayerMovement(session, func(player *game.Player) {
				player.Position.X += 1.234
			})

			startMining(t, session, position, 1)

			player := session.snapshotPlayer()
			if player.Exhaustion != 0 {
				t.Fatalf("exhaustion after movement and mining = %v, want 0", player.Exhaustion)
			}
		})
	}
}
