package server

import (
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

func TestArrowDamagesBoat(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{})
	arrow := runtime.SpawnArrow(game.Position{X: -1, Y: boatHeight / 2}, game.Velocity{X: 1}, 0)

	arrow.Tick(runtime, nil)

	if !arrow.State.Removed {
		t.Fatal("arrow remained after damaging boat")
	}

	boat.State.mu.RLock()
	hurtDirection := boat.HurtDirection
	hurtTime := boat.HurtTime
	damage := boat.Damage
	removed := boat.State.Removed
	boat.State.mu.RUnlock()

	if removed || hurtDirection != -1 || hurtTime != 10 || damage != 20 {
		t.Fatalf("boat arrow damage = removed %t direction %d time %d damage %v", removed, hurtDirection, hurtTime, damage)
	}
}

func TestExplosionDestroysBoatAndDropsItem(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	boat := runtime.SpawnBoat(game.EntityBambooRaft, game.Position{X: 1})

	runtime.Explode(RuntimeExplosion{Position: game.Position{}, Radius: 2, BlockInteraction: ExplosionKeepBlocks, Random: func() float32 {
		return 0
	}})

	if !boat.State.Removed {
		t.Fatal("explosion did not destroy boat")
	}

	for _, entity := range runtime.snapshotRuntimeEntities() {
		item, itemEntity := entity.(*runtimeItemEntity)
		if itemEntity && item.Stack == (game.ItemStack{Item: game.ItemBambooRaft, Count: 1}) {
			return
		}
	}

	t.Fatal("explosion-destroyed raft did not drop its item")
}

func TestExplosionPushesSurvivingBoat(t *testing.T) {
	runtime := NewRuntime(&game.World{})

	boat := runtime.SpawnBoat(game.EntityOakBoat, game.Position{X: 3.8})

	runtime.Explode(RuntimeExplosion{Position: game.Position{}, Radius: 2, BlockInteraction: ExplosionKeepBlocks, Random: func() float32 {
		return 0
	}})

	boat.State.mu.RLock()
	velocity := boat.Velocity
	removed := boat.State.Removed
	boat.State.mu.RUnlock()

	if removed {
		t.Fatal("surviving boat was destroyed")
	}

	if velocity.X <= 0 || velocity.Y <= 0 || velocity.Z != 0 {
		t.Fatalf("explosion boat velocity = %+v", velocity)
	}
}
