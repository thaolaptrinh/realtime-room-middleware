package main

import (
	"fmt"
	"os"

	"github.com/thaonguyen/realtime-room-middleware/internal/config"
)

func main() {
	paths := []string{
		"config/dev.example.yaml",
		"config/single-vps.example.yaml",
		"config/distributed-k3s.example.yaml",
		"deployments/dev/config/dev.yaml",
		"deployments/single-vps/config/production.single-vps.yaml",
		"deployments/distributed-k3s/config/production.distributed-k3s.yaml",
	}

	if len(os.Args) > 1 {
		paths = os.Args[1:]
	}

	ok := true
	for _, p := range paths {
		cfg, err := config.Load(p)
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", p, err)
			ok = false
			continue
		}
		if err := config.ValidateModeConstraints(cfg); err != nil {
			fmt.Printf("FAIL %s: mode constraint: %v\n", p, err)
			ok = false
			continue
		}
		fmt.Printf("OK   %s (mode=%s resolver=%s registry=%s)\n", p, cfg.Deployment.Mode, cfg.Resolver.Type, cfg.Registry.Type)
	}

	if !ok {
		os.Exit(1)
	}
}
