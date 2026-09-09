package server

import (
	"math"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	entityPositionChangeTolerance = 7.6293945e-6
	entityVelocityChangeTolerance = 1e-7
	entityPositionRefreshTicks    = 60
	entityFullSyncDelay           = 400
)

const (
	runtimeEntityMovementNone = iota
	runtimeEntityMovementPosition
	runtimeEntityMovementPositionRotation
	runtimeEntityMovementRotation
	runtimeEntityMovementTeleport
)

type runtimeEntityMovementKind byte

type runtimeEntityTracker struct {
	PositionBase  game.Position
	LastVelocity  game.Velocity
	LastYaw       byte
	LastPitch     byte
	LastHeadYaw   byte
	WasOnGround   bool
	UpdateTick    int32
	TeleportDelay int32
}

type runtimeEntityPacket struct {
	ID      int32
	Encoder PacketEncoder
}

type runtimeEntitySynchronization struct {
	motion           protocol.SetEntityMotion
	position         protocol.UpdateEntityPosition
	positionRotation protocol.UpdateEntityPositionRotation
	rotation         protocol.UpdateEntityRotation
	teleport         protocol.SynchronizeEntityPosition
	headRotation     protocol.SetHeadRotation
	movement         runtimeEntityMovementKind
	sendMotion       bool
	sendHeadRotation bool
}

type runtimeEntityLockedViewer interface {
	runtimeEntityViewLocked() runtimeEntityView
}

func (r *Runtime) synchronizeRuntimeEntity(entity RuntimeEntity) {
	tracked, trackable := entity.(RuntimeEntityTracker)
	lockedViewer, lockable := entity.(runtimeEntityLockedViewer)

	if !trackable || !lockable {
		return
	}

	configuration := tracked.RuntimeEntityTrackingConfig()
	if configuration.UpdateInterval <= 0 {
		return
	}

	state := entity.RuntimeEntityState()

	state.mu.Lock()
	view := lockedViewer.runtimeEntityViewLocked()

	tracker := &state.tracker

	forcedMovementSync := state.movementSyncDirty
	eligible := tracker.UpdateTick%configuration.UpdateInterval == 0 || forcedMovementSync || state.metadataDirty

	var synchronization runtimeEntitySynchronization

	if eligible && !view.Removed {
		synchronization = runtimeEntitySynchronizationForView(view, tracker, configuration, forcedMovementSync)
		state.movementSyncDirty = false
	}

	tracker.UpdateTick++
	state.mu.Unlock()

	r.synchronizeRuntimeEntityMovement(view.ID, synchronization)

	r.synchronizeDirtyRuntimeEntityMetadataIfPresent(entity)
}

func (r *Runtime) synchronizeRuntimeEntityMovement(entityID int32, synchronization runtimeEntitySynchronization) {
	if !synchronization.sendMotion && synchronization.movement == runtimeEntityMovementNone && !synchronization.sendHeadRotation {
		return
	}

	viewers := r.sessionView()

	for _, session := range viewers {
		if !session.tracksRuntimeEntity(entityID) {
			continue
		}

		if synchronization.sendMotion {
			writeRuntimeEntitySynchronizationPacket(session, protocol.ClientboundSetEntityMotionID, synchronization.motion)
		}

		switch synchronization.movement {
		case runtimeEntityMovementPosition:
			writeRuntimeEntitySynchronizationPacket(session, protocol.ClientboundUpdateEntityPositionID, synchronization.position)
		case runtimeEntityMovementPositionRotation:
			writeRuntimeEntitySynchronizationPacket(session, protocol.ClientboundUpdateEntityPositionRotationID, synchronization.positionRotation)
		case runtimeEntityMovementRotation:
			writeRuntimeEntitySynchronizationPacket(session, protocol.ClientboundUpdateEntityRotationID, synchronization.rotation)
		case runtimeEntityMovementTeleport:
			writeRuntimeEntitySynchronizationPacket(session, protocol.ClientboundSynchronizeEntityPositionID, synchronization.teleport)
		}

		if synchronization.sendHeadRotation {
			writeRuntimeEntitySynchronizationPacket(session, protocol.ClientboundSetHeadRotationID, synchronization.headRotation)
		}
	}
}

func writeRuntimeEntitySynchronizationPacket[T PacketEncoder](session *Session, packetID int32, encoder T) {
	buffered := session.Runtime != nil && session.Runtime.entityPacketBatch.Load()

	err := writeSessionPacketMode(session, packetID, encoder, buffered)

	if err != nil && session.Log != nil {
		session.Log.Warnf("[play] failed to synchronize entity: %v\n", err)
	}
}

func (r *Runtime) broadcastRuntimeEntityPacket(entityID int32, packet runtimeEntityPacket) {
	for _, session := range r.sessionView() {
		if !session.tracksRuntimeEntity(entityID) {
			continue
		}

		err := session.writePacket(packet.ID, packet.Encoder)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize entity: %v\n", err)
		}
	}
}

func (r *Runtime) synchronizeDirtyRuntimeEntityMetadataIfPresent(entity RuntimeEntity) {
	metadata, present := entity.(RuntimeEntityMetadata)
	if present {
		r.synchronizeDirtyRuntimeEntityMetadata(metadata)
	}
}

func newRuntimeEntityTracker(view runtimeEntityView) runtimeEntityTracker {
	return runtimeEntityTracker{
		PositionBase: view.Position,
		LastVelocity: view.Velocity,
		LastYaw:      protocolAngle(view.Rotation.Yaw),
		LastPitch:    protocolAngle(view.Rotation.Pitch),
		LastHeadYaw:  protocolAngle(view.Rotation.HeadYaw),
		WasOnGround:  view.OnGround,
	}
}

func runtimeEntitySpawnSnapshotLocked(view runtimeEntityView, tracker runtimeEntityTracker) runtimeEntitySpawnSnapshot {
	return runtimeEntitySpawnSnapshot{
		ID:       view.ID,
		UUID:     view.UUID,
		Position: tracker.PositionBase,
		Velocity: tracker.LastVelocity,
		Yaw:      tracker.LastYaw,
		Pitch:    tracker.LastPitch,
		HeadYaw:  tracker.LastHeadYaw,
	}
}

func runtimeEntitySynchronizationForView(view runtimeEntityView, tracker *runtimeEntityTracker, configuration RuntimeEntityTrackingConfig, forcedMovementSync bool) runtimeEntitySynchronization {
	var synchronization runtimeEntitySynchronization

	tracker.TeleportDelay++

	deltaX, xRelative := protocolPositionDelta(tracker.PositionBase.X, view.Position.X)
	deltaY, yRelative := protocolPositionDelta(tracker.PositionBase.Y, view.Position.Y)
	deltaZ, zRelative := protocolPositionDelta(tracker.PositionBase.Z, view.Position.Z)

	distanceX := view.Position.X - tracker.PositionBase.X
	distanceY := view.Position.Y - tracker.PositionBase.Y
	distanceZ := view.Position.Z - tracker.PositionBase.Z

	positionChanged := distanceX*distanceX+distanceY*distanceY+distanceZ*distanceZ >= entityPositionChangeTolerance
	sendPosition := positionChanged || tracker.UpdateTick%entityPositionRefreshTicks == 0

	yaw := protocolAngle(view.Rotation.Yaw)
	pitch := protocolAngle(view.Rotation.Pitch)

	rotationChanged := yaw != tracker.LastYaw || pitch != tracker.LastPitch

	fullSync := !xRelative || !yRelative || !zRelative || tracker.TeleportDelay > entityFullSyncDelay || tracker.WasOnGround != view.OnGround

	var (
		sentPosition bool
		sentRotation bool
	)

	if configuration.TrackDeltas || forcedMovementSync {
		velocityDifference := velocityDistanceSquared(view.Velocity, tracker.LastVelocity)
		velocityStopped := velocityDifference > 0 && velocityLengthSquared(view.Velocity) == 0

		if velocityDifference > entityVelocityChangeTolerance || velocityStopped {
			tracker.LastVelocity = view.Velocity

			synchronization.motion = protocol.SetEntityMotion{
				EntityID:  view.ID,
				VelocityX: view.Velocity.X,
				VelocityY: view.Velocity.Y,
				VelocityZ: view.Velocity.Z,
			}
			synchronization.sendMotion = true
		}
	}

	switch {
	case fullSync:
		tracker.WasOnGround = view.OnGround
		tracker.TeleportDelay = 0

		synchronization.teleport = protocol.SynchronizeEntityPosition{
			EntityID:  view.ID,
			X:         view.Position.X,
			Y:         view.Position.Y,
			Z:         view.Position.Z,
			VelocityX: view.Velocity.X,
			VelocityY: view.Velocity.Y,
			VelocityZ: view.Velocity.Z,
			Yaw:       view.Rotation.Yaw,
			Pitch:     view.Rotation.Pitch,
			OnGround:  view.OnGround,
		}
		synchronization.movement = runtimeEntityMovementTeleport

		sentPosition = true
		sentRotation = true
	case sendPosition && rotationChanged:
		synchronization.positionRotation = protocol.UpdateEntityPositionRotation{
			EntityID: view.ID,
			DeltaX:   deltaX,
			DeltaY:   deltaY,
			DeltaZ:   deltaZ,
			Yaw:      yaw,
			Pitch:    pitch,
			OnGround: view.OnGround,
		}
		synchronization.movement = runtimeEntityMovementPositionRotation

		sentPosition = true
		sentRotation = true
	case sendPosition:
		synchronization.position = protocol.UpdateEntityPosition{
			EntityID: view.ID,
			DeltaX:   deltaX,
			DeltaY:   deltaY,
			DeltaZ:   deltaZ,
			OnGround: view.OnGround,
		}
		synchronization.movement = runtimeEntityMovementPosition

		sentPosition = true
	case rotationChanged:
		synchronization.rotation = protocol.UpdateEntityRotation{
			EntityID: view.ID,
			Yaw:      yaw,
			Pitch:    pitch,
			OnGround: view.OnGround,
		}
		synchronization.movement = runtimeEntityMovementRotation

		sentRotation = true
	}

	if sentPosition {
		tracker.PositionBase = view.Position
	}

	if sentRotation {
		tracker.LastYaw = yaw
		tracker.LastPitch = pitch
	}

	headYaw := protocolAngle(view.Rotation.HeadYaw)
	if headYaw != tracker.LastHeadYaw {
		tracker.LastHeadYaw = headYaw

		synchronization.headRotation = protocol.SetHeadRotation{
			EntityID: view.ID,
			HeadYaw:  headYaw,
		}
		synchronization.sendHeadRotation = true
	}

	return synchronization
}

func velocityDistanceSquared(first, second game.Velocity) float64 {
	deltaX := first.X - second.X
	deltaY := first.Y - second.Y
	deltaZ := first.Z - second.Z

	return deltaX*deltaX + deltaY*deltaY + deltaZ*deltaZ
}

func velocityLengthSquared(velocity game.Velocity) float64 {
	return velocity.X*velocity.X + velocity.Y*velocity.Y + velocity.Z*velocity.Z
}

func protocolPositionDelta(previous, current float64) (int16, bool) {
	previousEncoded := math.Round(previous * entityPositionScale)
	currentEncoded := math.Round(current * entityPositionScale)

	delta := currentEncoded - previousEncoded

	if math.IsNaN(delta) || math.IsInf(delta, 0) || delta < math.MinInt16 || delta > math.MaxInt16 {
		return 0, false
	}

	return int16(delta), true
}
