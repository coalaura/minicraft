package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
)

const (
	groundMovementInputScale           = float32(0.21600002)
	groundMovementInputMinimumSquared  = 1e-7
	groundMovementPositionEpsilonSq    = 2.5000003e-7
	groundMovementWaypointVertical     = 1.0
	groundMovementOvershootDistanceSq  = 4.0
	groundMovementOvershootNearNodeSq  = 0.5
	groundMovementWantedMinimumSquared = 2.5000003e-7
	groundMovementLiquidJumpImpulse    = 0.04
)

type groundMobControlConfig struct {
	MovementSpeed        float32
	FlyingSpeed          float32
	GroundFriction       float32
	AirFriction          float32
	Gravity              float64
	VerticalDrag         float64
	JumpStrength         float64
	StepHeight           float64
	StepDistanceScale    float64
	EyeHeight            float64
	MoveMaximumTurn      float32
	IdleLookMaximumYaw   float32
	IdleLookMaximumPitch float32
	MaximumHeadYaw       float32
}

type groundNavigationState struct {
	Path          []game.Position
	Index         int
	SpeedModifier float64
}

type groundMoveControlState struct {
	Wanted        game.Position
	SpeedModifier float64
	Moving        bool
	ForwardInput  float32
	SidewaysInput float32
	Jump          bool
	JumpRequested bool
}

type groundLookControlState struct {
	Wanted       game.Position
	Cooldown     int32
	MaximumYaw   float32
	MaximumPitch float32
}

type groundBodyRotationState struct {
	LastStableHead float32
	StableTicks    int32
}

func (navigation *groundNavigationState) Done() bool {
	return navigation.Index >= len(navigation.Path)
}

func (navigation *groundNavigationState) Stop() {
	navigation.Path = nil
	navigation.Index = 0
	navigation.SpeedModifier = 0
}

func (navigation *groundNavigationState) MoveTo(path []game.Position, speedModifier float64) bool {
	if len(path) == 0 {
		navigation.Stop()

		return false
	}

	navigation.Path = path
	navigation.Index = 0
	navigation.SpeedModifier = speedModifier

	return true
}

func (navigation *groundNavigationState) Tick(position game.Position, width float64, control *groundMoveControlState) {
	for !navigation.Done() && groundNavigationNodeReached(position, navigation.Path, navigation.Index, width) {
		navigation.Index++
	}

	if navigation.Done() {
		control.Moving = false

		return
	}

	control.Wanted = navigation.Path[navigation.Index]
	control.SpeedModifier = navigation.SpeedModifier
	control.Moving = true
}

func (control *groundMoveControlState) Tick(position game.Position, rotation *game.Rotation) {
	control.TickConfigured(position, rotation, zombieGroundControlConfig())
}

func (control *groundMoveControlState) TickConfigured(position game.Position, rotation *game.Rotation, configuration groundMobControlConfig) {
	control.ForwardInput = 0
	control.SidewaysInput = 0
	control.Jump = control.JumpRequested
	control.JumpRequested = false

	if !control.Moving {
		return
	}

	deltaX := control.Wanted.X - position.X
	deltaY := control.Wanted.Y - position.Y
	deltaZ := control.Wanted.Z - position.Z

	distanceSquared := deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ
	if distanceSquared < groundMovementWantedMinimumSquared {
		control.Moving = false

		return
	}

	wantedYaw := float32(math.Atan2(deltaZ, deltaX)*180/math.Pi - 90)
	rotation.Yaw = rotateTowards(rotation.Yaw, wantedYaw, configuration.MoveMaximumTurn)

	speed := float32(control.SpeedModifier) * configuration.MovementSpeed
	control.ForwardInput = speed

	horizontalDistanceSquared := deltaX*deltaX + deltaZ*deltaZ
	if deltaY > configuration.StepHeight && horizontalDistanceSquared < 1 {
		control.Jump = true
	}
}

func (control *groundLookControlState) SetWanted(position game.Position, maximumYaw, maximumPitch float32) {
	control.Wanted = position
	control.Cooldown = 2
	control.MaximumYaw = maximumYaw
	control.MaximumPitch = maximumPitch
}

func (control *groundLookControlState) Tick(position game.Position, rotation *game.Rotation, navigating bool) {
	control.TickConfigured(position, rotation, navigating, zombieGroundControlConfig())
}

func (control *groundLookControlState) TickConfigured(position game.Position, rotation *game.Rotation, navigating bool, configuration groundMobControlConfig) {
	if control.Cooldown > 0 {
		control.Cooldown--

		deltaX := control.Wanted.X - position.X
		deltaY := control.Wanted.Y - (position.Y + configuration.EyeHeight)
		deltaZ := control.Wanted.Z - position.Z
		horizontalDistance := math.Hypot(deltaX, deltaZ)

		wantedYaw := float32(math.Atan2(deltaZ, deltaX)*180/math.Pi - 90)
		wantedPitch := float32(-math.Atan2(deltaY, horizontalDistance) * 180 / math.Pi)

		rotation.HeadYaw = rotateTowards(rotation.HeadYaw, wantedYaw, control.MaximumYaw)
		rotation.Pitch = rotateTowards(rotation.Pitch, wantedPitch, control.MaximumPitch)
	} else {
		rotation.HeadYaw = rotateTowards(rotation.HeadYaw, rotation.Yaw, configuration.IdleLookMaximumYaw)
		rotation.Pitch = rotateTowards(rotation.Pitch, 0, configuration.IdleLookMaximumPitch)
	}

	if navigating {
		rotation.HeadYaw = clampAngleAround(rotation.HeadYaw, rotation.Yaw, configuration.MaximumHeadYaw)
	}
}

func (control *groundBodyRotationState) Tick(previous, position game.Position, rotation *game.Rotation, maximumHeadYaw float32) {
	deltaX := position.X - previous.X
	deltaZ := position.Z - previous.Z
	moving := deltaX*deltaX+deltaZ*deltaZ > groundMovementPositionEpsilonSq

	if moving {
		rotation.HeadYaw = clampAngleAround(rotation.HeadYaw, rotation.Yaw, maximumHeadYaw)
		control.LastStableHead = rotation.HeadYaw
		control.StableTicks = 0

		return
	}

	if math.Abs(float64(wrapDegrees(rotation.HeadYaw-control.LastStableHead))) > 15 {
		control.StableTicks = 0
		control.LastStableHead = rotation.HeadYaw

		rotation.Yaw = clampAngleAround(rotation.Yaw, rotation.HeadYaw, maximumHeadYaw)

		return
	}

	control.StableTicks++

	if control.StableTicks <= 10 {
		return
	}

	progress := min(float32(control.StableTicks-10)/10, 1)
	maximumDifference := maximumHeadYaw * (1 - progress)

	rotation.Yaw = clampAngleAround(rotation.Yaw, rotation.HeadYaw, maximumDifference)
}

func (runtime *Runtime) tickGroundMobMovement(entity RuntimeEntity, living *RuntimeLivingState, rotation *game.Rotation, navigation *groundNavigationState, moveControl *groundMoveControlState, lookControl *groundLookControlState, bodyControl *groundBodyRotationState, configuration groundMobControlConfig, fallDamageImmune bool) (float32, bool) {
	state := entity.RuntimeEntityState()

	state.mu.Lock()

	previous := state.Position

	navigation.Tick(state.Position, living.Width, moveControl)

	moveControl.TickConfigured(state.Position, rotation, configuration)
	lookControl.TickConfigured(state.Position, rotation, !navigation.Done(), configuration)

	if moveControl.Jump {
		box := living.CollisionBox(state.Position)

		inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0
		inLava := runtime.fluidContact(box, game.FluidTypeLava, false).Depth > 0

		if inWater || inLava {
			living.Velocity.Y += groundMovementLiquidJumpImpulse
		} else if living.OnGround {
			living.Velocity.Y = max(living.Velocity.Y, configuration.JumpStrength)
		}
	}

	runtime.applyGroundLivingPhysics(state, living, rotation, moveControl, configuration)

	bodyControl.Tick(previous, state.Position, rotation, configuration.MaximumHeadYaw)

	deltaX := state.Position.X - previous.X
	deltaY := state.Position.Y - previous.Y
	deltaZ := state.Position.Z - previous.Z

	box := living.CollisionBox(state.Position)

	inWater := runtime.fluidContact(box, game.FluidTypeWater, false).Depth > 0

	resetFallDistance := inWater || runtime.runtimeLivingTouchesFallResettingBlock(box)
	if !resetFallDistance && living.FallDistance != 0 && deltaX*deltaX+deltaY*deltaY+deltaZ*deltaZ >= 1 {
		distance := math.Sqrt(deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ)
		traceDistance := min(distance, fallResetTraceMaximum)

		scale := traceDistance / distance

		traceEnd := game.Position{X: previous.X + deltaX*scale, Y: previous.Y + deltaY*scale, Z: previous.Z + deltaZ*scale}

		resetFallDistance = runtime.fallResetTraceHits(previous, traceEnd)
	}

	if resetFallDistance {
		living.FallDistance = 0
	} else if deltaY < 0 {
		living.FallDistance -= float32(deltaY)
	}

	fallDamage := float32(0)

	if living.OnGround {
		if !fallDamageImmune {
			fallDamage = calculateRuntimeLivingFallDamage(living.FallDistance)
		}

		living.FallDistance = 0
	}

	living.MoveDistance += float32(math.Hypot(deltaX, deltaZ) * configuration.StepDistanceScale)

	step := living.OnGround && living.MoveDistance > float32(living.NextStepDistance) && runtime.blockBelowItem(state.Position) != game.Air

	if step {
		living.NextStepDistance = int32(living.MoveDistance) + 1
	}

	state.mu.Unlock()

	runtime.runtimeEntityMoved(entity, previous)

	return fallDamage, step
}

func (runtime *Runtime) applyGroundLivingPhysics(state *RuntimeEntityState, living *RuntimeLivingState, rotation *game.Rotation, moveControl *groundMoveControlState, configuration groundMobControlConfig) {
	blockFriction := float32(1)
	acceleration := configuration.FlyingSpeed

	wasOnGround := living.OnGround
	if wasOnGround {
		blockFriction = runtime.blockFrictionBelow(state.Position)
		acceleration = configuration.MovementSpeed * (groundMovementInputScale / (blockFriction * blockFriction * blockFriction))
	}

	inputX := moveControl.SidewaysInput
	inputZ := moveControl.ForwardInput
	inputLengthSquared := inputX*inputX + inputZ*inputZ

	if inputLengthSquared >= groundMovementInputMinimumSquared {
		if inputLengthSquared > 1 {
			inverseLength := float32(1 / math.Sqrt(float64(inputLengthSquared)))
			inputX *= inverseLength
			inputZ *= inverseLength
		}

		inputX *= acceleration
		inputZ *= acceleration

		yaw := float64(rotation.Yaw) * math.Pi / 180
		sine := float64(minecraftSin(yaw))
		cosine := float64(minecraftCos(yaw))

		living.Velocity.X += float64(inputX)*cosine - float64(inputZ)*sine
		living.Velocity.Z += float64(inputZ)*cosine + float64(inputX)*sine
	}

	movement := runtime.moveGroundEntity(state.Position, living.Velocity, living.Width, living.Height, configuration.StepHeight, wasOnGround)

	state.Position = movement.Position
	living.OnGround = movement.OnGround

	if movement.HorizontalCollisionX {
		living.Velocity.X = 0
	}

	if movement.HorizontalCollisionZ {
		living.Velocity.Z = 0
	}

	if movement.VerticalCollision {
		living.Velocity.Y = 0
	}

	living.Velocity.Y -= configuration.Gravity
	horizontalDrag := configuration.AirFriction

	if wasOnGround {
		horizontalDrag = blockFriction * configuration.GroundFriction
	}

	living.Velocity.X *= float64(horizontalDrag)
	living.Velocity.Y *= configuration.VerticalDrag
	living.Velocity.Z *= float64(horizontalDrag)
}
