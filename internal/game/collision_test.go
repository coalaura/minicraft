package game

import (
	"math"
	"testing"
)

type blockCollisionTestCase struct {
	name  string
	block Block
	empty bool
}

type playerEyePositionTestCase struct {
	pose      PlayerPose
	eyeHeight float64
}

func TestBlockCollisionFamilies(t *testing.T) {
	openGate, valid := OakFenceGate.WithProperties(BlockPropertyValue{Name: "open", Value: "true"})
	if !valid {
		t.Fatal("resolve open fence gate")
	}

	tests := []blockCollisionTestCase{
		{name: "full block", block: Stone},
		{name: "slab", block: OakSlab},
		{name: "stairs", block: OakStairs},
		{name: "door", block: OakDoor},
		{name: "iron door independent of behavior", block: IronDoor},
		{name: "trapdoor", block: OakTrapdoor},
		{name: "fence", block: OakFence},
		{name: "closed fence gate", block: OakFenceGate},
		{name: "open fence gate", block: openGate, empty: true},
		{name: "pane", block: GlassPane},
		{name: "wall", block: CobblestoneWall},
		{name: "air", block: Air, empty: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boxes := test.block.CollisionBoxes(BlockPosition{})
			if (len(boxes) == 0) != test.empty {
				t.Fatalf("collision box count = %d, empty = %t", len(boxes), test.empty)
			}
		})
	}
}

func TestSlabCollisionUsesResolvedState(t *testing.T) {
	bottom, valid := OakSlab.WithProperties(BlockPropertyValue{Name: "type", Value: "bottom"})
	if !valid {
		t.Fatal("resolve bottom slab")
	}

	top, valid := OakSlab.WithProperties(BlockPropertyValue{Name: "type", Value: "top"})
	if !valid {
		t.Fatal("resolve top slab")
	}

	double, valid := OakSlab.WithProperties(BlockPropertyValue{Name: "type", Value: "double"})
	if !valid {
		t.Fatal("resolve double slab")
	}

	if box := bottom.CollisionBoxes(BlockPosition{})[0]; box.MinY != 0 || box.MaxY != 0.5 {
		t.Fatalf("bottom slab box = %+v", box)
	}

	if box := top.CollisionBoxes(BlockPosition{})[0]; box.MinY != 0.5 || box.MaxY != 1 {
		t.Fatalf("top slab box = %+v", box)
	}

	if box := double.CollisionBoxes(BlockPosition{})[0]; box.MinY != 0 || box.MaxY != 1 {
		t.Fatalf("double slab box = %+v", box)
	}
}

func TestPlayerCollisionBoxUsesPose(t *testing.T) {
	player := Player{Position: Position{X: 4, Y: 5, Z: 6}}

	heights := map[PlayerPose]float64{
		PlayerPoseStanding:  1.8,
		PlayerPoseCrouching: 1.5,
		PlayerPoseCrawling:  0.6,
	}

	for pose, height := range heights {
		player.Pose = pose

		box := player.CollisionBox()

		if math.Abs(box.MaxY-box.MinY-height) > 1e-9 || math.Abs(box.MaxX-box.MinX-0.6) > 1e-9 || math.Abs(box.MaxZ-box.MinZ-0.6) > 1e-9 {
			t.Fatalf("pose %d collision box = %+v", pose, box)
		}
	}
}

func TestPlayerEyePositionUsesPose(t *testing.T) {
	player := Player{Position: Position{X: 4, Y: 5, Z: 6}}

	tests := []playerEyePositionTestCase{
		{pose: PlayerPoseStanding, eyeHeight: 1.62},
		{pose: PlayerPoseCrouching, eyeHeight: 1.27},
		{pose: PlayerPoseCrawling, eyeHeight: 0.4},
	}

	for _, test := range tests {
		player.Pose = test.pose

		eyePosition := player.EyePosition()
		if player.EyeHeight() != test.eyeHeight || eyePosition != (Position{X: 4, Y: 5 + test.eyeHeight, Z: 6}) {
			t.Fatalf("pose %d eye position = %+v, height = %f", test.pose, eyePosition, player.EyeHeight())
		}
	}
}

func TestStraightWallCollisionOmitsCenterPost(t *testing.T) {
	wall, valid := CobblestoneWall.WithProperties(
		BlockPropertyValue{Name: "north", Value: "low"},
		BlockPropertyValue{Name: "south", Value: "low"},
		BlockPropertyValue{Name: "west", Value: "none"},
		BlockPropertyValue{Name: "east", Value: "none"},
		BlockPropertyValue{Name: "up", Value: "false"},
	)
	if !valid {
		t.Fatal("resolve straight wall")
	}

	boxes := wall.CollisionBoxes(BlockPosition{})
	if len(boxes) != 2 {
		t.Fatalf("straight wall collision box count = %d, want 2", len(boxes))
	}

	if boxes[0].MaxZ != 0.5 || boxes[1].MinZ != 0.5 {
		t.Fatalf("straight wall boxes = %+v", boxes)
	}
}
