package protocol

import (
	"fmt"

	"github.com/coalaura/plain"
)

var log *plain.Plain

func SetLogger(l *plain.Plain) {
	log = l
}

func (c *MCConnection) Print(state, msg string, a ...any) {
	if len(a) > 0 {
		msg = fmt.Sprintf(msg, a...)
	}

	log.Printf(
		"[%s] %s - %s\n",
		state,
		c.conn.RemoteAddr(),
		msg,
	)
}

func (c *MCConnection) Warn(state, msg string, a ...any) {
	if len(a) > 0 {
		msg = fmt.Sprintf(msg, a...)
	}

	log.Warnf(
		"[%s] %s - %s\n",
		state,
		c.conn.RemoteAddr(),
		msg,
	)
}
