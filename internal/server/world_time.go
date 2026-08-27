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

	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	r.tickActiveChunks()

	r.lifecycleMu.Unlock()
	r.worldMutationMu.Unlock()

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
