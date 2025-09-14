package protocol

import (
	"context"
	"errors"

	"github.com/coalaura/minicraft/config"
)

func HandlePlay(ctx context.Context, c *MCConnection, cfg *config.Config, uuid, name string) error {
	return errors.New("HandlePlay not implemented")
}
