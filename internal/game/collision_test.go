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

type chestCollisionTestCase struct {
	name  string
	block Block
	want  AABB
}

func TestCombinedFaceOccludes(t *testing.T) {
	bottom := collisionBlockForTest(t, StoneSlab, BlockPropertyValue{Name: "type", Value: "bottom"})
	top := collisionBlockForTest(t, StoneSlab, BlockPropertyValue{Name: "type", Value: "top"})

	if !CombinedFaceOccludes(bottom, top, BlockFaceEast) {
		t.Fatal("combined slab faces should occlude")
	}

	if CombinedFaceOccludes(bottom, Air, BlockFaceEast) {
		t.Fatal("single half slab face should not occlude")
	}

	if !Stone.CombinedFaceOccludes(Air, BlockFaceNorth) {
		t.Fatal("full block face should occlude")
	}
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
		{name: "hopper", block: Hopper},
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

func TestHopperCollisionLeavesInputOpenAndFollowsFacing(t *testing.T) {
	down, valid := Hopper.WithProperties(BlockPropertyValue{Name: "facing", Value: "down"})
	if !valid {
		t.Fatal("resolve downward hopper")
	}

	east, valid := Hopper.WithProperties(BlockPropertyValue{Name: "facing", Value: "east"})
	if !valid {
		t.Fatal("resolve eastward hopper")
	}

	downBoxes := down.CollisionBoxes(BlockPosition{})
	eastBoxes := east.CollisionBoxes(BlockPosition{})

	if len(downBoxes) != 7 || len(eastBoxes) != 7 {
		t.Fatalf("hopper collision box counts = %d, %d; want 7", len(downBoxes), len(eastBoxes))
	}

	input := AABB{MinX: 0.25, MinY: 11.0 / 16.0, MinZ: 0.25, MaxX: 0.75, MaxY: 1, MaxZ: 0.75}

	for _, box := range downBoxes {
		if box.Intersects(input) {
			t.Fatalf("hopper collision blocks input cavity: %+v", box)
		}
	}

	if downBoxes[6] != unitBox(6, 0, 6, 10, 4, 10) || eastBoxes[6] != unitBox(12, 4, 6, 16, 8, 10) {
		t.Fatalf("hopper spouts = down %+v east %+v", downBoxes[6], eastBoxes[6])
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

	box := bottom.CollisionBoxes(BlockPosition{})[0]
	if box.MinY != 0 || box.MaxY != 0.5 {
		t.Fatalf("bottom slab box = %+v", box)
	}

	box = top.CollisionBoxes(BlockPosition{})[0]
	if box.MinY != 0.5 || box.MaxY != 1 {
		t.Fatalf("top slab box = %+v", box)
	}

	box = double.CollisionBoxes(BlockPosition{})[0]
	if box.MinY != 0 || box.MaxY != 1 {
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

func TestNewPlacementFamilyCollisionShapes(t *testing.T) {
	carpetBoxes := WhiteCarpet.CollisionBoxes(BlockPosition{})
	if len(carpetBoxes) != 1 || carpetBoxes[0].MaxY != 1.0/16.0 {
		t.Fatalf("carpet collision boxes = %+v", carpetBoxes)
	}

	oneLayer, _ := Snow.WithProperties(BlockPropertyValue{Name: "layers", Value: "1"})
	eightLayers, _ := Snow.WithProperties(BlockPropertyValue{Name: "layers", Value: "8"})

	boxes := oneLayer.CollisionBoxes(BlockPosition{})
	if len(boxes) != 0 {
		t.Fatalf("one snow layer collision boxes = %+v", boxes)
	}

	boxes = eightLayers.CollisionBoxes(BlockPosition{})
	if len(boxes) != 1 || boxes[0].MaxY != 14.0/16.0 {
		t.Fatalf("eight snow layer collision boxes = %+v", boxes)
	}

	boxes = Candle.CollisionBoxes(BlockPosition{})
	if len(boxes) != 0 {
		t.Fatalf("candle collision boxes = %+v", boxes)
	}

	horizontalChain, _ := IronChain.WithProperties(BlockPropertyValue{Name: "axis", Value: "x"})

	chainBox := horizontalChain.CollisionBoxes(BlockPosition{})[0]
	if chainBox.MaxX != 1 || chainBox.MaxY-chainBox.MinY != 3.0/16.0 {
		t.Fatalf("horizontal chain collision box = %+v", chainBox)
	}

	tip, _ := PointedDripstone.WithProperties(
		BlockPropertyValue{Name: "thickness", Value: "tip"},
		BlockPropertyValue{Name: "vertical_direction", Value: "up"},
	)

	tipBox := tip.CollisionBoxes(BlockPosition{})[0]
	if tipBox.MaxY != 11.0/16.0 || tipBox.MinX != 5.0/16.0 {
		t.Fatalf("pointed dripstone tip collision box = %+v", tipBox)
	}

	cakeBox := Cake.CollisionBoxes(BlockPosition{})[0]
	if cakeBox.MinX != 1.0/16.0 || cakeBox.MaxY != 0.5 {
		t.Fatalf("cake collision box = %+v", cakeBox)
	}
}

func TestChestCollisionBoxes(t *testing.T) {
	position := BlockPosition{X: 4, Y: 5, Z: 6}
	singleChestBox := AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}

	tests := []chestCollisionTestCase{
		{name: "single chest", block: Chest, want: singleChestBox},
		{name: "single trapped chest", block: TrappedChest, want: singleChestBox},
		{name: "north left", block: chestStateForCollisionTest(t, Chest, "north", "left"), want: AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 5, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}},
		{name: "north right", block: chestStateForCollisionTest(t, Chest, "north", "right"), want: AABB{MinX: 4, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}},
		{name: "south left", block: chestStateForCollisionTest(t, Chest, "south", "left"), want: AABB{MinX: 4, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}},
		{name: "south right", block: chestStateForCollisionTest(t, Chest, "south", "right"), want: AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 5, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}},
		{name: "west left", block: chestStateForCollisionTest(t, Chest, "west", "left"), want: AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}},
		{name: "west right", block: chestStateForCollisionTest(t, Chest, "west", "right"), want: AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 7}},
		{name: "east left", block: chestStateForCollisionTest(t, Chest, "east", "left"), want: AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6 + 1.0/16.0, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 7}},
		{name: "east right", block: chestStateForCollisionTest(t, Chest, "east", "right"), want: AABB{MinX: 4 + 1.0/16.0, MinY: 5, MinZ: 6, MaxX: 4 + 15.0/16.0, MaxY: 5 + 14.0/16.0, MaxZ: 6 + 15.0/16.0}},
	}

	for _, chest := range copperChestBlocksForTest() {
		tests = append(tests, chestCollisionTestCase{name: "single " + chest.name + " copper chest", block: chest.block, want: singleChestBox})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boxes := test.block.CollisionBoxes(position)
			if len(boxes) != 1 || boxes[0] != test.want {
				t.Fatalf("collision boxes = %+v, want [%+v]", boxes, test.want)
			}
		})
	}
}

func chestStateForCollisionTest(t *testing.T, block Block, facing, chestType string) Block {
	t.Helper()

	state, valid := block.WithProperties(
		BlockPropertyValue{Name: "facing", Value: facing},
		BlockPropertyValue{Name: "type", Value: chestType},
	)

	if !valid {
		t.Fatalf("resolve %s %s chest", facing, chestType)
	}

	return state
}

func collisionBlockForTest(t *testing.T, block Block, property BlockPropertyValue) Block {
	t.Helper()

	state, valid := block.WithProperties(property)
	if !valid {
		t.Fatalf("resolve %d with %s=%s", block, property.Name, property.Value)
	}

	return state
}
