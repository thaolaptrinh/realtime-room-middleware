package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/thaonguyen/realtime-room-middleware/internal/config"
	"github.com/thaonguyen/realtime-room-middleware/internal/observability"
)

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "game-server: config error: %v\n", err)
		os.Exit(1)
	}

	logger := observability.InitDefaultLogger("info")

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

	// TODO: start KCP server in Stage 1
	logger.Info("game-server: realtime logic not yet implemented")
}

func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	return config.LoadFromEnv()
}
