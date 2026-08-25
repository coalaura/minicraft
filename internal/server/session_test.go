package server

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/coalaura/minicraft/internal/protocol"
)

type deadlineConnection struct {
	readDeadline time.Time
}

func (c *deadlineConnection) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *deadlineConnection) Write(data []byte) (int, error) {
	return len(data), nil
}

func (c *deadlineConnection) Close() error {
	return nil
}

func (c *deadlineConnection) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *deadlineConnection) RemoteAddr() net.Addr {
	return &net.TCPAddr{}
}

func (c *deadlineConnection) SetDeadline(time.Time) error {
	return nil
}

func (c *deadlineConnection) SetReadDeadline(deadline time.Time) error {
	c.readDeadline = deadline

	return nil
}

func (c *deadlineConnection) SetWriteDeadline(time.Time) error {
	return nil
}

func TestReadPacketSetsIdleDeadline(t *testing.T) {
	connection := &deadlineConnection{}

	session := NewSession(protocol.NewConnection(connection, nil), nil, nil, nil)

	startedAt := time.Now()

	_, err := session.readPacket()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("read packet error = %v; want EOF", err)
	}

	minimumDeadline := startedAt.Add(connectionReadTimeout)
	maximumDeadline := time.Now().Add(connectionReadTimeout)

	if connection.readDeadline.Before(minimumDeadline) || connection.readDeadline.After(maximumDeadline) {
		t.Fatalf("read deadline = %v; want between %v and %v", connection.readDeadline, minimumDeadline, maximumDeadline)
	}
}
