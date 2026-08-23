package main

import (
	"context"
	"errors"
	"net"
	"os/signal"
	"syscall"

	"github.com/coalaura/minicraft/config"
	"github.com/coalaura/minicraft/protocol"
	"github.com/coalaura/plain"
)

var log = plain.New(plain.WithDate(plain.RFC3339Local))

func init() {
	protocol.SetLogger(log)
}

func main() {
	cfg, err := config.LoadConfig()
	log.MustFail(err)

	listener, err := net.Listen("tcp", cfg.ListenAddr())
	log.MustFail(err)

	defer listener.Close()

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	defer cancel()

	go func() {
		<-ctx.Done()

		log.Println("shutting down")

		err := listener.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warnf("failed to close listener: %v", err)
		}
	}()

	log.Printf("Listening on %s (online-mode=%v)\n", cfg.ListenAddr(), cfg.OnlineMode)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}

			log.Warnf("failed to accept: %v", err)

			continue
		}

		go protocol.HandleConnection(ctx, conn, cfg)
	}
}
