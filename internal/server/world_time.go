package server

import (
	"context"
	"time"

	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	gameTickInterval    = 50 * time.Millisecond
	timeSyncTicks       = 20
	playerDeathDuration = 20
)

type playerDeathLifecycleUpdate struct {
	player     game.Player
	recipients []*Session
}

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
	deathUpdates := make([]playerDeathLifecycleUpdate, 0)

	r.worldMutationMu.Lock()
	r.lifecycleMu.Lock()

	r.tickActiveChunks()

	swimmingChanges := r.updateActivePlayerSwimmingLocked()

	for _, session := range r.snapshotSessions() {
		session.updatePlayerState(func(player *game.Player) bool {
			player.TickAttackStrength()

			return true
		})

		updates := r.tickPlayerSurvivalLocked(session)
		if len(updates) > 0 {
			survivalUpdates[session] = updates
		}

		deathUpdate, finished := r.tickPlayerDeathLocked(session)
		if finished {
			deathUpdates = append(deathUpdates, deathUpdate)
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

	for _, update := range deathUpdates {
		r.sendPlayerDeathLifecycleUpdate(update)
	}

	r.tickOpenMenus()

	return state
}

func (r *Runtime) tickPlayerDeathLocked(session *Session) (playerDeathLifecycleUpdate, bool) {
	player := session.snapshotPlayer()
	if !player.Dead || player.DeathEntityRemoved {
		return playerDeathLifecycleUpdate{}, false
	}

	var recipients []*Session

	if player.DeathTime+1 >= playerDeathDuration {
		for _, recipient := range r.snapshotSessions() {
			if recipient != session && !playersVisible(recipient.snapshotPlayer(), player, recipient.renderDistance()) {
				continue
			}

			recipients = append(recipients, recipient)
		}
	}

	player, _ = session.updatePlayerState(func(player *game.Player) bool {
		player.DeathTime++

		if player.DeathTime >= playerDeathDuration {
			player.DeathEntityRemoved = true
		}

		return true
	})

	if !player.DeathEntityRemoved {
		return playerDeathLifecycleUpdate{}, false
	}

	return playerDeathLifecycleUpdate{player: player, recipients: recipients}, true
}

func (r *Runtime) sendPlayerDeathLifecycleUpdate(update playerDeathLifecycleUpdate) {
	event := protocol.EntityEvent{EntityID: update.player.EntityID, Event: 60}

	for _, recipient := range update.recipients {
		err := recipient.writePacket(protocol.ClientboundEntityEventID, event)
		if err != nil && recipient.Log != nil {
			recipient.Log.Warnf("[play] failed to synchronize final player death event: %v\n", err)
		}

		if recipient.snapshotPlayer().EntityID == update.player.EntityID {
			continue
		}

		err = recipient.sendPlayerRemoval(update.player)
		if err != nil && recipient.Log != nil {
			recipient.Log.Warnf("[play] failed to remove dead player entity: %v\n", err)
		}
	}
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
