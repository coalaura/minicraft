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

type runtimeEntityTracker struct {
	PositionBase  game.Position
	LastVelocity  game.Velocity
	LastYaw       byte
	LastPitch     byte
	WasOnGround   bool
	UpdateTick    int32
	TeleportDelay int32
}

type runtimeEntityPacket struct {
	ID      int32
	Encoder PacketEncoder
}

type runtimeEntityLockedViewer interface {
	runtimeEntityViewLocked() runtimeEntityView
}

func newRuntimeEntityTracker(view runtimeEntityView) runtimeEntityTracker {
	return runtimeEntityTracker{
		PositionBase: view.Position,
		LastVelocity: view.Velocity,
		LastYaw:      protocolAngle(view.Rotation.Yaw),
		LastPitch:    protocolAngle(view.Rotation.Pitch),
		WasOnGround:  view.OnGround,
	}
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

	eligible := tracker.UpdateTick%configuration.UpdateInterval == 0 || state.metadataDirty

	var packets []runtimeEntityPacket

	if eligible && !view.Removed {
		packets = runtimeEntityPackets(view, tracker, configuration)
	}

	tracker.UpdateTick++
	state.mu.Unlock()

	for _, packet := range packets {
		r.broadcastRuntimeEntityPacket(view.ID, packet)
	}

	r.synchronizeDirtyRuntimeEntityMetadataIfPresent(entity)
}

func runtimeEntityPackets(view runtimeEntityView, tracker *runtimeEntityTracker, configuration RuntimeEntityTrackingConfig) []runtimeEntityPacket {
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

	sentPosition := false
	sentRotation := false

	packets := make([]runtimeEntityPacket, 0, 2)
	if configuration.TrackDeltas {
		velocityDifference := velocityDistanceSquared(view.Velocity, tracker.LastVelocity)
		velocityStopped := velocityDifference > 0 && velocityLengthSquared(view.Velocity) == 0

		if velocityDifference > entityVelocityChangeTolerance || velocityStopped {
			tracker.LastVelocity = view.Velocity

			packets = append(packets, runtimeEntityPacket{
				ID: protocol.ClientboundSetEntityMotionID,
				Encoder: protocol.SetEntityMotion{
					EntityID:  view.ID,
					VelocityX: view.Velocity.X,
					VelocityY: view.Velocity.Y,
					VelocityZ: view.Velocity.Z,
				},
			})
		}
	}

	switch {
	case fullSync:
		tracker.WasOnGround = view.OnGround
		tracker.TeleportDelay = 0

		packets = append(packets, runtimeEntityPacket{
			ID: protocol.ClientboundSynchronizeEntityPositionID,
			Encoder: protocol.SynchronizeEntityPosition{
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
			},
		})

		sentPosition = true
		sentRotation = true
	case sendPosition && rotationChanged:
		packets = append(packets, runtimeEntityPacket{
			ID: protocol.ClientboundUpdateEntityPositionRotationID,
			Encoder: protocol.UpdateEntityPositionRotation{
				EntityID: view.ID,
				DeltaX:   deltaX,
				DeltaY:   deltaY,
				DeltaZ:   deltaZ,
				Yaw:      yaw,
				Pitch:    pitch,
				OnGround: view.OnGround,
			},
		})

		sentPosition = true
		sentRotation = true
	case sendPosition:
		packets = append(packets, runtimeEntityPacket{
			ID: protocol.ClientboundUpdateEntityPositionID,
			Encoder: protocol.UpdateEntityPosition{
				EntityID: view.ID,
				DeltaX:   deltaX,
				DeltaY:   deltaY,
				DeltaZ:   deltaZ,
				OnGround: view.OnGround,
			},
		})

		sentPosition = true
	case rotationChanged:
		packets = append(packets, runtimeEntityPacket{
			ID: protocol.ClientboundUpdateEntityRotationID,
			Encoder: protocol.UpdateEntityRotation{
				EntityID: view.ID,
				Yaw:      yaw,
				Pitch:    pitch,
				OnGround: view.OnGround,
			},
		})

		sentRotation = true
	}

	if sentPosition {
		tracker.PositionBase = view.Position
	}

	if sentRotation {
		tracker.LastYaw = yaw
		tracker.LastPitch = pitch
	}

	return packets
}

func (r *Runtime) broadcastRuntimeEntityPacket(entityID int32, packet runtimeEntityPacket) {
	for _, session := range r.snapshotSessions() {
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
