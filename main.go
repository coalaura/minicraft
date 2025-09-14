package main

import (
	"context"
	"net"
	"os"
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

	log.Printf("Listening on %s (online-mode=%v)\n", cfg.ListenAddr(), cfg.OnlineMode)

	ctx, cancel := SignalContext()
	defer cancel()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				log.Println("shutting down")

				return
			default:
			}

			log.Warnf("failed to accept: %v", err)

			continue
		}

		go protocol.HandleConnection(ctx, conn, cfg)
	}
}

func SignalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	ch := make(chan os.Signal, 1)

	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-ch
		cancel()

		// hard exit
		<-ch
		os.Exit(1)
	}()

	return ctx, cancel
}
