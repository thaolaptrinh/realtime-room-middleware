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
		fmt.Fprintf(os.Stderr, "gateway: config error: %v\n", err)
		os.Exit(1)
	}

	logger := observability.InitDefaultLogger("info")

	logger.Info("gateway starting",
		slog.String("mode", cfg.Deployment.Mode),
		slog.String("http_addr", cfg.Gateway.HTTPAddr),
		slog.String("resolver", cfg.Resolver.Type),
		slog.String("registry", cfg.Registry.Type),
		slog.String("metrics", cfg.Metrics.Type),
	)

	// TODO: start HTTP gateway in Stage 1
	logger.Info("gateway: realtime logic not yet implemented")
}

func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	return config.LoadFromEnv()
}
