package server

import (
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const maximumVehiclePassengers = 2

type passengerCapacity interface {
	maximumPassengers() int
}

type passengerList struct {
	ids   [maximumVehiclePassengers]int32
	count int
}

// PassengerView identifies a player or runtime entity in the shared entity-ID namespace.
type PassengerView struct {
	EntityID  int32
	Position  game.Position
	VehicleID int32
}

// MountPassenger adds passenger to vehicle. An entity may have one vehicle; concrete vehicles define their capacity.
func (r *Runtime) MountPassenger(vehicle, passenger any) bool {
	vehicleID, validVehicle := r.passengerIDFor(vehicle)
	passengerID, validPassenger := r.passengerIDFor(passenger)

	if !validVehicle || !validPassenger || vehicleID == passengerID {
		return false
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	return r.mountPassengerLocked(vehicleID, passengerID)
}

// mountPassengerLocked adds a passenger while lifecycleMu is already held.
func (r *Runtime) mountPassengerLocked(vehicleID, passengerID int32) bool {
	if !r.passengerExists(vehicleID) || !r.passengerExists(passengerID) {
		return false
	}

	capacity := 1
	vehicle := r.passengerForID(vehicleID)

	if configured, valid := vehicle.(passengerCapacity); valid {
		capacity = configured.maximumPassengers()
	}

	r.passengerMu.Lock()

	if r.passengerVehicles[passengerID] != 0 || r.passengerWouldCycleLocked(vehicleID, passengerID) {
		r.passengerMu.Unlock()

		return false
	}

	passengers := r.vehiclePassengers[vehicleID]
	if passengers.count >= capacity {
		r.passengerMu.Unlock()

		return false
	}

	insertAt := passengers.count

	if r.playerPassengerID(passengerID) && passengers.count > 0 && !r.playerPassengerID(passengers.ids[0]) {
		copy(passengers.ids[1:], passengers.ids[:passengers.count])

		insertAt = 0
	}

	passengers.ids[insertAt] = passengerID
	passengers.count++

	r.vehiclePassengers[vehicleID] = passengers
	r.passengerVehicles[passengerID] = vehicleID
	r.passengerMu.Unlock()

	r.suspendMountedPassengerTicker(passengerID)
	r.synchronizePassengerVehicle(vehicleID)

	return true
}

// DismountPassenger removes passenger from its vehicle.
func (r *Runtime) DismountPassenger(passenger any) bool {
	passengerID, valid := r.passengerIDFor(passenger)
	if !valid {
		return false
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	vehicleID := r.passengerVehicleID(passengerID)

	r.entityMu.RLock()
	boat, boatVehicle := r.entities[vehicleID].(*runtimeBoatEntity)
	r.entityMu.RUnlock()

	var (
		playerSession  *Session
		previousPlayer game.Player
	)

	if boatVehicle {
		passenger := r.passengerForID(passengerID)
		if passenger != nil {
			playerSession, _ = passenger.(*Session)
			if playerSession != nil {
				previousPlayer = playerSession.snapshotPlayer()
			}

			r.updatePassengerPositionLocked(passenger, boat.safeDismountPosition(r, passenger))
		}
	}

	dismounted := r.dismountPassengerLocked(passengerID)
	if !dismounted {
		return false
	}

	if playerSession != nil {
		currentPlayer := playerSession.snapshotPlayer()

		err := playerSession.sendPlayerPosition()
		if err != nil && playerSession.Log != nil {
			playerSession.Log.Warnf("[play] failed to synchronize dismount position: %v\n", err)
		}

		for _, viewer := range r.sessionView() {
			if viewer == playerSession || !viewer.seesPlayerEntity(passengerID) {
				continue
			}

			err = viewer.sendPlayerMovement(previousPlayer, currentPlayer)
			if err != nil && viewer.Log != nil {
				viewer.Log.Warnf("[play] failed to synchronize dismounted player: %v\n", err)
			}
		}

		r.updateMountedPlayerChunks(playerSession, false)
	}

	return true
}

// UpdatePassengerPosition updates the authoritative position of a mounted player or runtime entity.
func (r *Runtime) UpdatePassengerPosition(passenger any, position game.Position) bool {
	passengerID, valid := r.passengerIDFor(passenger)
	if !valid {
		return false
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.passengerMu.RLock()
	_, mounted := r.passengerVehicles[passengerID]
	r.passengerMu.RUnlock()

	if !mounted {
		return false
	}

	return r.updatePassengerPositionLocked(passenger, position)
}

// UpdatePassengerPositions calls update in passenger order and applies its returned positions.
func (r *Runtime) UpdatePassengerPositions(vehicle any, update func(PassengerView) game.Position) {
	vehicleID, valid := r.passengerIDFor(vehicle)
	if !valid || update == nil {
		return
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.passengerMu.RLock()
	passengers := r.vehiclePassengers[vehicleID]
	r.passengerMu.RUnlock()

	for index := 0; index < passengers.count; index++ {
		passenger := r.passengerForID(passengers.ids[index])
		if passenger == nil {
			continue
		}

		view, present := r.passengerView(passenger)
		if !present {
			continue
		}

		view.VehicleID = vehicleID
		r.updatePassengerPositionLocked(passenger, update(view))
	}
}

// VehicleID returns the ID of session's current vehicle, or zero when it is not mounted.
func (s *Session) VehicleID() int32 {
	if s.Runtime == nil {
		return 0
	}

	return s.Runtime.passengerVehicleID(s.passengerID())
}

func (r *Runtime) removePassenger(entityID int32) bool {
	r.passengerMu.Lock()

	vehicleID := r.passengerVehicles[entityID]
	passengers := r.vehiclePassengers[entityID]

	if vehicleID == 0 && passengers.count == 0 {
		r.passengerMu.Unlock()

		return false
	}

	delete(r.passengerVehicles, entityID)

	if vehicleID != 0 {
		r.removePassengerFromVehicleLocked(vehicleID, entityID)
	}

	delete(r.vehiclePassengers, entityID)

	for index := 0; index < passengers.count; index++ {
		delete(r.passengerVehicles, passengers.ids[index])
	}

	r.passengerMu.Unlock()

	if vehicleID != 0 {
		r.synchronizePassengerVehicle(vehicleID)
	}

	if passengers.count != 0 {
		r.synchronizePassengerVehicle(entityID)

		for index := 0; index < passengers.count; index++ {
			r.resumeDismountedPassengerTicker(passengers.ids[index])
		}
	}

	return true
}

func (r *Runtime) dismountPassengerLocked(passengerID int32) bool {
	r.passengerMu.Lock()
	vehicleID := r.passengerVehicles[passengerID]

	if vehicleID == 0 {
		r.passengerMu.Unlock()

		return false
	}

	delete(r.passengerVehicles, passengerID)
	r.removePassengerFromVehicleLocked(vehicleID, passengerID)
	r.passengerMu.Unlock()

	r.resumeDismountedPassengerTicker(passengerID)
	r.synchronizePassengerVehicle(vehicleID)

	return true
}

func (r *Runtime) passengerIDFor(passenger any) (int32, bool) {
	switch entity := passenger.(type) {
	case *Session:
		return entity.passengerID(), entity != nil && entity.Runtime == r
	case RuntimeEntity:
		state := entity.RuntimeEntityState()

		state.mu.RLock()
		entityID := state.ID
		removed := state.Removed
		state.mu.RUnlock()

		return entityID, entityID != 0 && !removed
	default:
		return 0, false
	}
}

func (s *Session) passengerID() int32 {
	if s == nil {
		return 0
	}

	s.playerMx.RLock()
	defer s.playerMx.RUnlock()

	if s.Player == nil {
		return 0
	}

	return s.Player.EntityID
}

func (r *Runtime) passengerExists(entityID int32) bool {
	for _, session := range r.sessionView() {
		if session.passengerID() == entityID {
			return true
		}
	}

	r.entityMu.RLock()
	entity := r.entities[entityID]
	r.entityMu.RUnlock()

	return entity != nil
}

func (r *Runtime) passengerWouldCycleLocked(vehicleID, passengerID int32) bool {
	for vehicleID != 0 {
		if vehicleID == passengerID {
			return true
		}

		vehicleID = r.passengerVehicles[vehicleID]
	}

	return false
}

func (r *Runtime) removePassengerFromVehicleLocked(vehicleID, passengerID int32) {
	passengers := r.vehiclePassengers[vehicleID]

	for index := 0; index < passengers.count; index++ {
		if passengers.ids[index] != passengerID {
			continue
		}

		copy(passengers.ids[index:], passengers.ids[index+1:passengers.count])
		passengers.count--
		passengers.ids[passengers.count] = 0

		if passengers.count == 0 {
			delete(r.vehiclePassengers, vehicleID)
		} else {
			r.vehiclePassengers[vehicleID] = passengers
		}

		return
	}
}

func (r *Runtime) passengerVehicleID(entityID int32) int32 {
	r.passengerMu.RLock()
	defer r.passengerMu.RUnlock()

	return r.passengerVehicles[entityID]
}

func (r *Runtime) passengerForID(entityID int32) any {
	for _, session := range r.sessionView() {
		if session.passengerID() == entityID {
			return session
		}
	}

	r.entityMu.RLock()
	entity := r.entities[entityID]
	r.entityMu.RUnlock()

	return entity
}

func (r *Runtime) passengerView(passenger any) (PassengerView, bool) {
	switch entity := passenger.(type) {
	case *Session:
		player := entity.playerView()

		return PassengerView{EntityID: player.EntityID, Position: player.Position}, true
	case RuntimeEntity:
		state := entity.RuntimeEntityState()

		state.mu.RLock()
		view := PassengerView{EntityID: state.ID, Position: state.Position}
		removed := state.Removed
		state.mu.RUnlock()

		return view, !removed
	default:
		return PassengerView{}, false
	}
}

func (r *Runtime) updatePassengerPositionLocked(passenger any, position game.Position) bool {
	switch entity := passenger.(type) {
	case *Session:
		entity.playerMx.Lock()
		entity.Player.Position = position
		entity.playerMx.Unlock()

		return true
	case RuntimeEntity:
		state := entity.RuntimeEntityState()

		state.mu.Lock()

		if state.Removed {
			state.mu.Unlock()

			return false
		}

		previous := state.Position
		state.Position = position
		state.mu.Unlock()

		r.runtimeEntityMoved(entity, previous)

		return true
	default:
		return false
	}
}

func (r *Runtime) playerPassengerID(entityID int32) bool {
	for _, session := range r.sessionView() {
		if session.passengerID() == entityID {
			return true
		}
	}

	return false
}

func (r *Runtime) suspendMountedPassengerTicker(passengerID int32) {
	r.entityMu.RLock()
	entity := r.entities[passengerID]
	r.entityMu.RUnlock()

	if entity == nil {
		return
	}

	state := entity.RuntimeEntityState()

	state.mu.RLock()
	chunkPosition := state.Chunk
	state.mu.RUnlock()

	chunk, active := r.ActiveChunk(chunkPosition)
	if active {
		chunk.setMountedEntity(passengerID, entity)
	}
}

func (r *Runtime) resumeDismountedPassengerTicker(passengerID int32) {
	r.entityMu.RLock()
	entity := r.entities[passengerID]
	r.entityMu.RUnlock()

	if entity == nil {
		return
	}

	state := entity.RuntimeEntityState()

	state.mu.RLock()
	chunkPosition := state.Chunk
	removed := state.Removed
	state.mu.RUnlock()

	if removed {
		return
	}

	chunk, active := r.ActiveChunk(chunkPosition)
	if active {
		chunk.SetEntity(passengerID, entity)
	}
}

func (r *Runtime) synchronizePassengerRelationsFor(session *Session, entityID int32) {
	r.passengerMu.RLock()
	vehicleID := r.passengerVehicles[entityID]
	_, vehicle := r.vehiclePassengers[entityID]
	r.passengerMu.RUnlock()

	if vehicle {
		r.sendPassengerVehicle(session, entityID)
	}

	if vehicleID != 0 {
		r.sendPassengerVehicle(session, vehicleID)
	}
}

func (r *Runtime) synchronizePassengerVehicle(vehicleID int32) {
	for _, session := range r.sessionView() {
		r.sendPassengerVehicle(session, vehicleID)
	}
}

func (r *Runtime) sendPassengerVehicle(session *Session, vehicleID int32) {
	r.passengerMu.RLock()
	passengers := r.vehiclePassengers[vehicleID]
	r.passengerMu.RUnlock()

	if !r.passengerVisibleTo(session, vehicleID) {
		return
	}

	for index := 0; index < passengers.count; index++ {
		if !r.passengerVisibleTo(session, passengers.ids[index]) {
			return
		}
	}

	packet := protocol.SetPassengers{VehicleID: vehicleID, Passengers: passengers.ids[:passengers.count]}

	err := session.writePacket(protocol.ClientboundSetPassengersID, packet)
	if err != nil && session.Log != nil {
		session.Log.Warnf("[play] failed to synchronize passengers: %v\n", err)
	}
}

func (r *Runtime) passengerVisibleTo(session *Session, entityID int32) bool {
	if session.passengerID() == entityID {
		return true
	}

	for _, other := range r.sessionView() {
		if other.passengerID() != entityID {
			continue
		}

		return session.seesPlayerEntity(entityID)
	}

	return session.tracksRuntimeEntity(entityID)
}

func (s *Session) markPlayerVisible(entityID int32) {
	s.entityTrackMu.Lock()
	defer s.entityTrackMu.Unlock()

	if s.visiblePlayerEntities == nil {
		s.visiblePlayerEntities = make(map[int32]struct{})
	}

	s.visiblePlayerEntities[entityID] = struct{}{}
}

func (s *Session) markPlayerHidden(entityID int32) {
	s.entityTrackMu.Lock()
	defer s.entityTrackMu.Unlock()

	delete(s.visiblePlayerEntities, entityID)
}

func (s *Session) seesPlayerEntity(entityID int32) bool {
	s.entityTrackMu.Lock()
	defer s.entityTrackMu.Unlock()

	_, visible := s.visiblePlayerEntities[entityID]
	return visible
}
