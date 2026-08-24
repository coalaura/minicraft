package main

import (
	"context"
	"errors"
	"net"
	"os/signal"
	"syscall"

	"github.com/coalaura/minicraft/internal/config"
	"github.com/coalaura/minicraft/internal/game"
	"github.com/coalaura/minicraft/internal/generator"
	_ "github.com/coalaura/minicraft/internal/generator/catalog"
	"github.com/coalaura/minicraft/internal/protocol"
	"github.com/coalaura/minicraft/internal/server"
	"github.com/coalaura/plain"
)

var log = plain.New(plain.WithDate(plain.RFC3339Local))

func main() {
	cfg, err := config.LoadConfig()
	log.MustFail(err)

	log.SetLevel(cfg.GetLogLevel())

	worldGenerator, err := generator.New(cfg.WorldGenerator)
	log.MustFail(err)

	runtime := server.NewRuntime(game.NewOverworld(worldGenerator))

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

		go handleConnection(ctx, cfg, runtime, conn)
	}
}

func handleConnection(ctx context.Context, cfg *config.Config, runtime *server.Runtime, conn net.Conn) {
	defer conn.Close()

	session := server.NewSession(protocol.NewConnection(conn, log), cfg, runtime, log)

	err := session.Run(ctx)
	if err != nil {
		log.Warnf("[session] %s - %v\n", conn.RemoteAddr(), err)
	}
}
