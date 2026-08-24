package server

import (
	"context"
	"time"

	"github.com/coalaura/minicraft/internal/protocol"
)

const (
	KeepAlivePeriod = 10 * time.Second
)

func (s *Session) keepAliveLoop(ctx context.Context) {
	ticker := time.NewTicker(KeepAlivePeriod)
	defer ticker.Stop()

	var id int64

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			id++

			err := s.sendKeepAlive(id)
			if err != nil {
				s.Log.Warnf("[play] failed to send keep alive: %v\n", err)

				return
			}
		}
	}
}

func (s *Session) sendKeepAlive(id int64) error {
	var wr protocol.PacketWriter

	keepAlive := protocol.PlayKeepAlive{ID: id}

	keepAlive.Encode(&wr)

	err := wr.Err()
	if err != nil {
		return err
	}

	return s.writeRawPacket(protocol.Packet{
		ID:   protocol.ClientboundPlayKeepAliveID,
		Data: wr.Buffer.Bytes(),
	})
}

func (s *Session) handleKeepAlive(response protocol.PlayKeepAliveResponse) {
	// Keep-alive response; nothing to do.
	_ = response
}
