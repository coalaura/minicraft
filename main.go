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

	worldGenerator, err := generator.New(cfg.World.Generator)
	log.MustFail(err)

	world := game.NewOverworld(worldGenerator, cfg.World.Seed)

	world.SetLightingMode(game.ParseLightingMode(cfg.World.Lighting))
	world.SetTime(cfg.WorldTime(), cfg.DayCycle())

	if cfg.World.Spawn != nil {
		world.Spawn = game.Position{X: cfg.World.Spawn.X, Y: cfg.World.Spawn.Y, Z: cfg.World.Spawn.Z}
	}

	runtime := server.NewRuntime(world)

	runtime.AllowBlockBreaking = cfg.AllowBlockBreaking()
	runtime.AllowBlockPlacing = cfg.AllowBlockPlacing()
	runtime.ChatEnabled = cfg.ChatEnabled()
	runtime.ChatFormat = cfg.ChatFormat()
	runtime.ChatJoinMessage = cfg.ChatJoinMessage()
	runtime.ChatLeaveMessage = cfg.ChatLeaveMessage()

	err = runtime.InitializeSecureChat(context.Background(), cfg.Server.OnlineMode, nil)
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

	go runtime.RunGameLoop(ctx)

	go func() {
		<-ctx.Done()
		cancel()

		err := listener.Close()
		if err != nil && !errors.Is(err, net.ErrClosed) {
			log.Warnf("failed to close listener: %v", err)
		}
	}()

	log.Printf("Listening on %s (online-mode=%v)\n", cfg.ListenAddr(), cfg.Server.OnlineMode)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}

			log.Warnf("failed to accept: %v", err)

			continue
		}

		go handleConnection(ctx, cfg, runtime, conn)
	}

	log.Warnln("shutting down")

	err = runtime.DisconnectAll("Server shutting down")
	if err != nil {
		log.Warnf("failed to disconnect all clients: %v", err)
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
