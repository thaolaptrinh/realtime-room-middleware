package config

import (
	"os"
	"path/filepath"
	"testing"
)

func validDevConfig() *Config {
	return &Config{
		Deployment: DeploymentConfig{Mode: ModeDev},
		Gateway:    GatewayConfig{HTTPAddr: ":8080"},
		Game: GameConfig{
			KCPAddr:           ":9000",
			TickRateHz:        20,
			BroadcastRateHz:   10,
			MaxPlayersPerRoom: 200,
		},
		Protocol: ProtocolConfig{
			Version:       1,
			Serialization: "msgpack",
			Transport:     "kcp",
		},
		Resolver: ResolverConfig{
			Type:           ResolverSingleNode,
			SingleNodeAddr: "127.0.0.1:9000",
		},
		Registry: RegistryConfig{Type: RegistryMemory},
		Spatial:  SpatialConfig{CellSizeM: 10},
		Interest: InterestConfig{
			VisualRadiusM:     30,
			ObjectRadiusM:     30,
			VoiceRadiusM:      30,
			FullAvatarRadiusM: 30,
			LowLodRadiusM:     30,
		},
		Voice: VoiceConfig{
			Allocator:               "proximity",
			MaxParticipantsPerGroup: 8,
			RecomputeIntervalMs:     250,
		},
		ObjectLock: ObjectLockConfig{
			LeaseTTLMs:        10000,
			RefreshIntervalMs: 3000,
			MaxLocksPerUser:   3,
		},
		Metrics: MetricsConfig{Type: MetricsLog},
	}
}

func validSingleVPSConfig() *Config {
	cfg := validDevConfig()
	cfg.Deployment.Mode = ModeSingleVPS
	cfg.Metrics.Type = MetricsPrometheus
	return cfg
}

func validDistributedK3sConfig() *Config {
	cfg := validDevConfig()
	cfg.Deployment.Mode = ModeDistributedK3s
	cfg.Resolver = ResolverConfig{
		Type:      ResolverRedis,
		RedisAddr: "redis:6379",
	}
	cfg.Registry = RegistryConfig{
		Type:      RegistryRedis,
		RedisAddr: "redis:6379",
	}
	cfg.Metrics.Type = MetricsPrometheus
	return cfg
}

func TestValidateValidDevConfig(t *testing.T) {
	if err := Validate(validDevConfig()); err != nil {
		t.Fatalf("valid dev config should pass: %v", err)
	}
}

func TestValidateValidSingleVPSConfig(t *testing.T) {
	if err := Validate(validSingleVPSConfig()); err != nil {
		t.Fatalf("valid single-vps config should pass: %v", err)
	}
}

func TestValidateValidDistributedK3sConfig(t *testing.T) {
	if err := Validate(validDistributedK3sConfig()); err != nil {
		t.Fatalf("valid distributed-k3s config should pass: %v", err)
	}
}

func TestValidateInvalidDeploymentMode(t *testing.T) {
	cfg := validDevConfig()
	cfg.Deployment.Mode = "invalid"
	assertValidation(t, cfg, "invalid deployment mode")
}

func TestValidateInvalidResolverType(t *testing.T) {
	cfg := validDevConfig()
	cfg.Resolver.Type = "invalid"
	assertValidation(t, cfg, "invalid resolver type")
}

func TestValidateInvalidRegistryType(t *testing.T) {
	cfg := validDevConfig()
	cfg.Registry.Type = "invalid"
	assertValidation(t, cfg, "invalid registry type")
}

func TestValidateInvalidMetricsType(t *testing.T) {
	cfg := validDevConfig()
	cfg.Metrics.Type = "invalid"
	assertValidation(t, cfg, "invalid metrics type")
}

func TestValidateInvalidTickRate(t *testing.T) {
	cfg := validDevConfig()
	cfg.Game.TickRateHz = 0
	assertValidation(t, cfg, "tick_rate_hz")

	cfg.Game.TickRateHz = -1
	assertValidation(t, cfg, "tick_rate_hz")
}

func TestValidateInvalidBroadcastRate(t *testing.T) {
	cfg := validDevConfig()
	cfg.Game.BroadcastRateHz = 0
	assertValidation(t, cfg, "broadcast_rate_hz")

	cfg = validDevConfig()
	cfg.Game.BroadcastRateHz = 30
	cfg.Game.TickRateHz = 20
	assertValidation(t, cfg, "broadcast_rate_hz")
}

func TestValidateInvalidGatewayAddr(t *testing.T) {
	cfg := validDevConfig()
	cfg.Gateway.HTTPAddr = ""
	assertValidation(t, cfg, "http_addr")

	cfg.Gateway.HTTPAddr = "no-colon"
	assertValidation(t, cfg, "http_addr")
}

func TestValidateInvalidKCPAddr(t *testing.T) {
	cfg := validDevConfig()
	cfg.Game.KCPAddr = ""
	assertValidation(t, cfg, "kcp_addr")

	cfg.Game.KCPAddr = "no-colon"
	assertValidation(t, cfg, "kcp_addr")
}

func TestValidateResolverRedisMissingAddr(t *testing.T) {
	cfg := validDevConfig()
	cfg.Resolver.Type = ResolverRedis
	cfg.Resolver.RedisAddr = ""
	assertValidation(t, cfg, "redis_addr")
}

func TestValidateRegistryRedisMissingAddr(t *testing.T) {
	cfg := validDevConfig()
	cfg.Registry.Type = RegistryRedis
	cfg.Registry.RedisAddr = ""
	assertValidation(t, cfg, "redis_addr")
}

func TestModeConstraintsSingleVPSNoRedis(t *testing.T) {
	cfg := validSingleVPSConfig()
	if err := ValidateModeConstraints(cfg); err != nil {
		t.Fatalf("single-vps with single-node/memory should pass: %v", err)
	}
}

func TestModeConstraintsSingleVPSRejectsRedisResolver(t *testing.T) {
	cfg := validSingleVPSConfig()
	cfg.Resolver.Type = ResolverRedis
	cfg.Resolver.RedisAddr = "redis:6379"
	assertModeConstraint(t, cfg, "redis resolver")
}

func TestModeConstraintsSingleVPSRejectsRedisRegistry(t *testing.T) {
	cfg := validSingleVPSConfig()
	cfg.Registry.Type = RegistryRedis
	cfg.Registry.RedisAddr = "redis:6379"
	assertModeConstraint(t, cfg, "redis registry")
}

func TestModeConstraintsDistributedRequiresRedisResolver(t *testing.T) {
	cfg := validDistributedK3sConfig()
	cfg.Resolver.Type = ResolverSingleNode
	assertModeConstraint(t, cfg, "redis resolver")
}

func TestModeConstraintsDistributedRequiresRedisRegistry(t *testing.T) {
	cfg := validDistributedK3sConfig()
	cfg.Registry.Type = RegistryMemory
	assertModeConstraint(t, cfg, "redis registry")
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
deployment:
  mode: dev
gateway:
  http_addr: ":9090"
game:
  kcp_addr: ":9100"
  tick_rate_hz: 20
  broadcast_rate_hz: 10
  max_players_per_room: 100
protocol:
  version: 1
  serialization: msgpack
  transport: kcp
resolver:
  type: single-node
  single_node_addr: "127.0.0.1:9100"
registry:
  type: memory
spatial:
  cell_size_m: 10
interest:
  visual_radius_m: 30
  object_radius_m: 30
  voice_radius_m: 30
  full_avatar_radius_m: 30
  low_lod_radius_m: 30
voice:
  allocator: proximity
  max_participants_per_group: 8
  recompute_interval_ms: 250
object_lock:
  lease_ttl_ms: 10000
  refresh_interval_ms: 3000
  max_locks_per_user: 3
metrics:
  type: log
`)
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Deployment.Mode != ModeDev {
		t.Errorf("mode = %q, want %q", cfg.Deployment.Mode, ModeDev)
	}
	if cfg.Gateway.HTTPAddr != ":9090" {
		t.Errorf("http_addr = %q, want %q", cfg.Gateway.HTTPAddr, ":9090")
	}
	if cfg.Game.KCPAddr != ":9100" {
		t.Errorf("kcp_addr = %q, want %q", cfg.Game.KCPAddr, ":9100")
	}
	if cfg.Game.MaxPlayersPerRoom != 100 {
		t.Errorf("max_players = %d, want 100", cfg.Game.MaxPlayersPerRoom)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFromEnvNotSet(t *testing.T) {
	os.Unsetenv("CONFIG_PATH")
	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when CONFIG_PATH not set")
	}
}

func TestLoadFromEnvSet(t *testing.T) {
	dir := t.TempDir()
	content := []byte(`
deployment:
  mode: dev
gateway:
  http_addr: ":8080"
game:
  kcp_addr: ":9000"
  tick_rate_hz: 20
  broadcast_rate_hz: 10
  max_players_per_room: 200
protocol:
  version: 1
  serialization: msgpack
  transport: kcp
resolver:
  type: single-node
  single_node_addr: "127.0.0.1:9000"
registry:
  type: memory
spatial:
  cell_size_m: 10
interest:
  visual_radius_m: 30
  object_radius_m: 30
  voice_radius_m: 30
  full_avatar_radius_m: 30
  low_lod_radius_m: 30
voice:
  allocator: proximity
  max_participants_per_group: 8
  recompute_interval_ms: 250
object_lock:
  lease_ttl_ms: 10000
  refresh_interval_ms: 3000
  max_locks_per_user: 3
metrics:
  type: log
`)
	path := filepath.Join(dir, "env.yaml")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG_PATH", path)
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}
	if cfg.Deployment.Mode != ModeDev {
		t.Errorf("mode = %q, want %q", cfg.Deployment.Mode, ModeDev)
	}
}

func TestLoadActualExampleConfigs(t *testing.T) {
	files := []string{
		"../../config/dev.example.yaml",
		"../../config/single-vps.example.yaml",
		"../../config/distributed-k3s.example.yaml",
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			cfg, err := Load(f)
			if err != nil {
				t.Fatalf("Load %s failed: %v", f, err)
			}
			if err := ValidateModeConstraints(cfg); err != nil {
				t.Fatalf("mode constraints for %s: %v", f, err)
			}
		})
	}
}

func TestLoadActualDeploymentConfigs(t *testing.T) {
	files := []string{
		"../../deployments/dev/config/dev.yaml",
		"../../deployments/single-vps/config/production.single-vps.yaml",
		"../../deployments/distributed-k3s/config/production.distributed-k3s.yaml",
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			cfg, err := Load(f)
			if err != nil {
				t.Fatalf("Load %s failed: %v", f, err)
			}
			if err := ValidateModeConstraints(cfg); err != nil {
				t.Fatalf("mode constraints for %s: %v", f, err)
			}
		})
	}
}

func TestDevModeAllowsRedis(t *testing.T) {
	cfg := validDevConfig()
	cfg.Resolver = ResolverConfig{Type: ResolverRedis, RedisAddr: "redis:6379"}
	cfg.Registry = RegistryConfig{Type: RegistryRedis, RedisAddr: "redis:6379"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("dev with redis should validate: %v", err)
	}
	if err := ValidateModeConstraints(cfg); err != nil {
		t.Fatalf("dev allows redis: %v", err)
	}
}

func assertValidation(t *testing.T, cfg *Config, substr string) {
	t.Helper()
	err := Validate(cfg)
	if err == nil {
		t.Fatalf("expected validation error containing %q", substr)
	}
}

func assertModeConstraint(t *testing.T, cfg *Config, substr string) {
	t.Helper()
	err := ValidateModeConstraints(cfg)
	if err == nil {
		t.Fatalf("expected mode constraint error containing %q", substr)
	}
}
