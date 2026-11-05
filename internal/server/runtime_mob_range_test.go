package server

import (
	"math"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type mobRangeTestCase struct {
	Name       string
	Base       float64
	Bonus      float64
	Follow     float64
	Navigation float64
}

func TestMobFollowAndNavigationRanges(t *testing.T) {
	tests := []mobRangeTestCase{
		{Name: "skeleton negative", Base: skeletonFollowRange, Bonus: -0.1, Follow: 14.4, Navigation: 16},
		{Name: "animal negative", Base: animalFollowRange, Bonus: -0.1, Follow: 14.4, Navigation: 16},
		{Name: "aquatic negative", Base: aquaticFollowRange, Bonus: -0.1, Follow: 14.4, Navigation: 16},
		{Name: "skeleton positive", Base: skeletonFollowRange, Bonus: 0.1, Follow: 17.6, Navigation: 17.6},
		{Name: "animal positive", Base: animalFollowRange, Bonus: 0.1, Follow: 17.6, Navigation: 17.6},
		{Name: "aquatic positive", Base: aquaticFollowRange, Bonus: 0.1, Follow: 17.6, Navigation: 17.6},
		{Name: "creeper negative", Base: creeperFollowRange, Bonus: -0.1, Follow: 28.8, Navigation: 28.8},
		{Name: "zombie negative", Base: zombieFollowRange, Bonus: -0.1, Follow: 31.5, Navigation: 31.5},
		{Name: "creeper positive", Base: creeperFollowRange, Bonus: 0.1, Follow: 35.2, Navigation: 35.2},
		{Name: "zombie positive", Base: zombieFollowRange, Bonus: 0.1, Follow: 38.5, Navigation: 38.5},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			state := RuntimeMobState{FollowRangeBonus: test.Bonus}

			follow := state.FollowRange(test.Base)
			navigation := state.NavigationRange(test.Base)

			if math.Abs(follow-test.Follow) > 1e-12 || math.Abs(navigation-test.Navigation) > 1e-12 {
				t.Fatalf("follow/navigation = %g/%g, want %g/%g", follow, navigation, test.Follow, test.Navigation)
			}
		})
	}
}

func TestSkeletonSpawnBonusTargetAndNavigation(t *testing.T) {
	runtime := NewRuntime(&game.World{Generator: spawnTestGenerator{Floor: game.Stone, Roof: true}})

	runtime.entityRandom = func() float32 {
		return 0
	}

	start := game.Position{X: 0.5, Y: -63, Z: 0.5}
	target := game.Position{X: 15.5, Y: -63, Z: 0.5}
	player := addRuntimeMobTestPlayer(t, runtime, target, game.GameModeSurvival)
	skeleton := runtime.SpawnSkeleton(start)

	skeleton.tickTarget(runtime, true)

	if skeleton.Target.Target.session != player {
		t.Fatal("base FOLLOW_RANGE did not acquire player 15 blocks away")
	}

	skeleton.FollowRangeBonus = -0.1
	skeleton.tickTarget(runtime, true)

	if skeleton.Target.Target.present() {
		t.Fatal("negative spawn bonus did not reduce target acquisition below base range")
	}

	target.X = 16.5
	skeleton.moveToTarget(runtime, target, 1)

	if len(skeleton.Navigation.Path) == 0 || skeleton.Navigation.Path[len(skeleton.Navigation.Path)-1] != target {
		t.Fatal("negative spawn bonus prevented navigation to the 16-block floor")
	}

	skeleton.Navigation.Stop()
	target.X = 17.5
	skeleton.moveToTarget(runtime, target, 1)

	if len(skeleton.Navigation.Path) != 0 {
		t.Fatal("negative spawn bonus navigated beyond the 16-block floor")
	}

	skeleton.FollowRangeBonus = 0.1
	skeleton.moveToTarget(runtime, target, 1)

	if len(skeleton.Navigation.Path) == 0 || skeleton.Navigation.Path[len(skeleton.Navigation.Path)-1] != target {
		t.Fatal("positive spawn bonus did not extend navigation beyond 16 blocks")
	}
}
