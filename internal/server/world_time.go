package server

import (
	"context"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	gameTickInterval = 50 * time.Millisecond
	timeSyncTicks    = 20
)

func (r *Runtime) RunGameLoop(ctx context.Context) {
	ticker := time.NewTicker(gameTickInterval)
	defer ticker.Stop()

	var ticksUntilSync int

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state := r.Tick()

			ticksUntilSync++

			if state.DayCycle && ticksUntilSync >= timeSyncTicks {
				r.broadcastTime(state)

				ticksUntilSync = 0
			}
		}
	}
}

func (r *Runtime) Tick() game.TimeState {
	state := r.World.AdvanceTime()
	survivalUpdates := make(map[*Session][]playerSurvivalUpdate)

	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	r.tickActiveChunks()

	swimmingChanges := r.updateActivePlayerSwimmingLocked()

	for _, session := range r.snapshotSessions() {
		updates := r.tickPlayerSurvivalLocked(session)
		if len(updates) > 0 {
			survivalUpdates[session] = updates
		}
	}

	r.lifecycleMu.Unlock()

	r.tickScheduledBlocksLocked()
	r.tickScheduledFluidsLocked()

	r.tickRandomBlocksLocked()

	miningMutations := r.tickMiningAttemptsLocked()

	r.runtimeBlockMutations = append(r.runtimeBlockMutations, miningMutations...)

	deliveries := r.takeRuntimeBlockMutationsLocked()

	r.worldMutationMu.Unlock()

	r.completeRuntimeBlockMutations(deliveries)
	r.sendPlayerMetadataUpdates(swimmingChanges)

	for session, updates := range survivalUpdates {
		r.sendPlayerSurvivalUpdates(session, updates)
	}

	r.tickOpenMenus()

	return state
}

func (r *Runtime) broadcastTime(state game.TimeState) {
	for _, session := range r.snapshotSessions() {
		err := session.sendTimeUpdate(state)
		if err != nil && session.Log != nil {
			session.Log.Warnf("[play] failed to synchronize world time: %v\n", err)
		}
	}
}

func (s *Session) sendTimeUpdate(state game.TimeState) error {
	var wr protocol.PacketWriter

	update := protocol.UpdateTime{
		Age:         state.Age,
		Time:        state.DayTime,
		TickDayTime: state.DayCycle,
	}

	update.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundUpdateTimeID,
		Data: wr.Buffer.Bytes(),
	})
}
