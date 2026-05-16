package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/config"
	gatewayhttp "github.com/thaonguyen/realtime-room-middleware/internal/gateway/http"
	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/resolver"
	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/token"
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

	var nodeResolver resolver.NodeResolver
	switch cfg.Resolver.Type {
	case config.ResolverSingleNode:
		nodeResolver = resolver.NewSingleNodeResolver(
			cfg.Resolver.SingleNodeAddr,
			cfg.Resolver.SingleNodeWebSocketURL,
			cfg.Protocol.Version,
		)
	default:
		fmt.Fprintf(os.Stderr, "gateway: unsupported resolver type %q for single-vps/dev\n", cfg.Resolver.Type)
		os.Exit(1)
	}

	srv := gatewayhttp.NewServer(gatewayhttp.ServerConfig{
		Addr:           cfg.Gateway.HTTPAddr,
		Resolver:       nodeResolver,
		TokenGenerator: token.NewGenerator(),
		Logger:         logger,
	})

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			fmt.Fprintf(os.Stderr, "gateway: server error: %v\n", err)
			os.Exit(1)
		}
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "gateway: shutdown error: %v\n", err)
			os.Exit(1)
		}
	}

	logger.Info("gateway stopped")
}

func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	return config.LoadFromEnv()
}
