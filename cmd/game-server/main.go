package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/thaonguyen/realtime-room-middleware/internal/config"
	kcptransport "github.com/thaonguyen/realtime-room-middleware/internal/transport/kcp"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "game-server: config error: %v\n", err)
		os.Exit(1)
	}

	logger := slog.Default()

	logger.Info("game-server starting",
		slog.String("mode", cfg.Deployment.Mode),
		slog.String("kcp_addr", cfg.Game.KCPAddr),
		slog.Int("tick_rate_hz", cfg.Game.TickRateHz),
		slog.Int("broadcast_rate_hz", cfg.Game.BroadcastRateHz),
		slog.Int("max_players_per_room", cfg.Game.MaxPlayersPerRoom),
		slog.String("resolver", cfg.Resolver.Type),
		slog.String("registry", cfg.Registry.Type),
		slog.String("metrics", cfg.Metrics.Type),
	)

	handler := kcptransport.HandlerFunc(func(sess kcptransport.Session, data []byte) {
		logger.Debug("kcp packet received",
			slog.String("session_id", sess.ID()),
			slog.Int("bytes", len(data)),
		)
	})

	kcpServer, err := kcptransport.NewServer(kcptransport.ServerConfig{
		ListenAddr: cfg.Game.KCPAddr,
		Logger:     logger,
	}, handler)
	if err != nil {
		logger.Error("kcp server init failed", slog.String("err", err.Error()))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := kcpServer.Start(ctx); err != nil {
		logger.Error("kcp server start failed", slog.String("err", err.Error()))
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("game-server ready",
		slog.String("kcp_addr", kcpServer.Addr().String()),
	)

	<-sigCh
	logger.Info("game-server shutting down")
	kcpServer.Stop()
	logger.Info("game-server stopped")
}

func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	return config.LoadFromEnv()
}
