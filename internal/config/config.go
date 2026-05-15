package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Deployment DeploymentConfig `yaml:"deployment"`
	Gateway    GatewayConfig    `yaml:"gateway"`
	Game       GameConfig       `yaml:"game"`
	Protocol   ProtocolConfig   `yaml:"protocol"`
	Resolver   ResolverConfig   `yaml:"resolver"`
	Registry   RegistryConfig   `yaml:"registry"`
	Spatial    SpatialConfig    `yaml:"spatial"`
	Interest   InterestConfig   `yaml:"interest"`
	Voice      VoiceConfig      `yaml:"voice"`
	ObjectLock ObjectLockConfig `yaml:"object_lock"`
	Metrics    MetricsConfig    `yaml:"metrics"`
}

type DeploymentConfig struct {
	Mode string `yaml:"mode"`
}

type GatewayConfig struct {
	HTTPAddr string `yaml:"http_addr"`
}

type GameConfig struct {
	KCPAddr           string `yaml:"kcp_addr"`
	TickRateHz        int    `yaml:"tick_rate_hz"`
	BroadcastRateHz   int    `yaml:"broadcast_rate_hz"`
	MaxPlayersPerRoom int    `yaml:"max_players_per_room"`
}

type ProtocolConfig struct {
	Version       int    `yaml:"version"`
	Serialization string `yaml:"serialization"`
	Transport     string `yaml:"transport"`
}

type ResolverConfig struct {
	Type           string `yaml:"type"`
	SingleNodeAddr string `yaml:"single_node_addr"`
	RedisAddr      string `yaml:"redis_addr"`
}

type RegistryConfig struct {
	Type      string `yaml:"type"`
	RedisAddr string `yaml:"redis_addr"`
}

type SpatialConfig struct {
	CellSizeM float32 `yaml:"cell_size_m"`
}

type InterestConfig struct {
	VisualRadiusM     float32 `yaml:"visual_radius_m"`
	ObjectRadiusM     float32 `yaml:"object_radius_m"`
	VoiceRadiusM      float32 `yaml:"voice_radius_m"`
	FullAvatarRadiusM float32 `yaml:"full_avatar_radius_m"`
	LowLodRadiusM     float32 `yaml:"low_lod_radius_m"`
}

type VoiceConfig struct {
	Allocator               string `yaml:"allocator"`
	MaxParticipantsPerGroup int    `yaml:"max_participants_per_group"`
	RecomputeIntervalMs     int    `yaml:"recompute_interval_ms"`
}

type ObjectLockConfig struct {
	LeaseTTLMs        int `yaml:"lease_ttl_ms"`
	RefreshIntervalMs int `yaml:"refresh_interval_ms"`
	MaxLocksPerUser   int `yaml:"max_locks_per_user"`
}

type MetricsConfig struct {
	Type string `yaml:"type"`
}

const (
	ModeDev            = "dev"
	ModeSingleVPS      = "single-vps"
	ModeDistributedK3s = "distributed-k3s"

	ResolverSingleNode = "single-node"
	ResolverRedis      = "redis"

	RegistryMemory = "memory"
	RegistryRedis  = "redis"

	MetricsLog        = "log"
	MetricsPrometheus = "prometheus"
)

func validModes() map[string]bool {
	return map[string]bool{
		ModeDev:            true,
		ModeSingleVPS:      true,
		ModeDistributedK3s: true,
	}
}

func validResolverTypes() map[string]bool {
	return map[string]bool{
		ResolverSingleNode: true,
		ResolverRedis:      true,
	}
}

func validRegistryTypes() map[string]bool {
	return map[string]bool{
		RegistryMemory: true,
		RegistryRedis:  true,
	}
}

func validMetricsTypes() map[string]bool {
	return map[string]bool{
		MetricsLog:        true,
		MetricsPrometheus: true,
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func LoadFromEnv() (*Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		return nil, fmt.Errorf("CONFIG_PATH environment variable not set")
	}
	return Load(path)
}

func Validate(cfg *Config) error {
	if err := validateDeployment(cfg); err != nil {
		return err
	}
	if err := validateGateway(cfg); err != nil {
		return err
	}
	if err := validateGame(cfg); err != nil {
		return err
	}
	if err := validateResolver(cfg); err != nil {
		return err
	}
	if err := validateRegistry(cfg); err != nil {
		return err
	}
	if err := validateMetrics(cfg); err != nil {
		return err
	}
	if err := validateSpatial(cfg); err != nil {
		return err
	}
	if err := validateInterest(cfg); err != nil {
		return err
	}
	if err := validateVoice(cfg); err != nil {
		return err
	}
	if err := validateObjectLock(cfg); err != nil {
		return err
	}
	return nil
}

func validateDeployment(cfg *Config) error {
	mode := cfg.Deployment.Mode
	if !validModes()[mode] {
		return fmt.Errorf("invalid deployment mode %q: must be one of dev, single-vps, distributed-k3s", mode)
	}
	return nil
}

func validateGateway(cfg *Config) error {
	if cfg.Gateway.HTTPAddr == "" {
		return fmt.Errorf("gateway.http_addr is required")
	}
	if !strings.HasPrefix(cfg.Gateway.HTTPAddr, ":") && !strings.Contains(cfg.Gateway.HTTPAddr, ":") {
		return fmt.Errorf("gateway.http_addr %q is not a valid listen address", cfg.Gateway.HTTPAddr)
	}
	return nil
}

func validateGame(cfg *Config) error {
	if cfg.Game.KCPAddr == "" {
		return fmt.Errorf("game.kcp_addr is required")
	}
	if !strings.HasPrefix(cfg.Game.KCPAddr, ":") && !strings.Contains(cfg.Game.KCPAddr, ":") {
		return fmt.Errorf("game.kcp_addr %q is not a valid listen address", cfg.Game.KCPAddr)
	}
	if cfg.Game.TickRateHz <= 0 {
		return fmt.Errorf("game.tick_rate_hz must be > 0, got %d", cfg.Game.TickRateHz)
	}
	if cfg.Game.BroadcastRateHz <= 0 {
		return fmt.Errorf("game.broadcast_rate_hz must be > 0, got %d", cfg.Game.BroadcastRateHz)
	}
	if cfg.Game.BroadcastRateHz > cfg.Game.TickRateHz {
		return fmt.Errorf("game.broadcast_rate_hz (%d) must not exceed game.tick_rate_hz (%d)", cfg.Game.BroadcastRateHz, cfg.Game.TickRateHz)
	}
	if cfg.Game.MaxPlayersPerRoom <= 0 {
		return fmt.Errorf("game.max_players_per_room must be > 0, got %d", cfg.Game.MaxPlayersPerRoom)
	}
	return nil
}

func validateResolver(cfg *Config) error {
	if !validResolverTypes()[cfg.Resolver.Type] {
		return fmt.Errorf("invalid resolver type %q: must be one of single-node, redis", cfg.Resolver.Type)
	}

	switch cfg.Resolver.Type {
	case ResolverSingleNode:
		if cfg.Deployment.Mode != ModeDev && cfg.Resolver.SingleNodeAddr == "" {
			return fmt.Errorf("resolver.single_node_addr is required when resolver.type is single-node")
		}
	case ResolverRedis:
		if cfg.Resolver.RedisAddr == "" {
			return fmt.Errorf("resolver.redis_addr is required when resolver.type is redis")
		}
	}
	return nil
}

func validateRegistry(cfg *Config) error {
	if !validRegistryTypes()[cfg.Registry.Type] {
		return fmt.Errorf("invalid registry type %q: must be one of memory, redis", cfg.Registry.Type)
	}

	if cfg.Registry.Type == RegistryRedis && cfg.Registry.RedisAddr == "" {
		return fmt.Errorf("registry.redis_addr is required when registry.type is redis")
	}
	return nil
}

func validateMetrics(cfg *Config) error {
	if !validMetricsTypes()[cfg.Metrics.Type] {
		return fmt.Errorf("invalid metrics type %q: must be one of log, prometheus", cfg.Metrics.Type)
	}
	return nil
}

func validateSpatial(cfg *Config) error {
	if cfg.Spatial.CellSizeM <= 0 {
		return fmt.Errorf("spatial.cell_size_m must be > 0, got %f", cfg.Spatial.CellSizeM)
	}
	return nil
}

func validateInterest(cfg *Config) error {
	if cfg.Interest.VisualRadiusM <= 0 {
		return fmt.Errorf("interest.visual_radius_m must be > 0, got %f", cfg.Interest.VisualRadiusM)
	}
	if cfg.Interest.ObjectRadiusM <= 0 {
		return fmt.Errorf("interest.object_radius_m must be > 0, got %f", cfg.Interest.ObjectRadiusM)
	}
	if cfg.Interest.VoiceRadiusM <= 0 {
		return fmt.Errorf("interest.voice_radius_m must be > 0, got %f", cfg.Interest.VoiceRadiusM)
	}
	if cfg.Interest.FullAvatarRadiusM <= 0 {
		return fmt.Errorf("interest.full_avatar_radius_m must be > 0, got %f", cfg.Interest.FullAvatarRadiusM)
	}
	if cfg.Interest.LowLodRadiusM <= 0 {
		return fmt.Errorf("interest.low_lod_radius_m must be > 0, got %f", cfg.Interest.LowLodRadiusM)
	}
	return nil
}

func validateVoice(cfg *Config) error {
	if cfg.Voice.Allocator != "proximity" && cfg.Voice.Allocator != "kmeans" {
		return fmt.Errorf("invalid voice allocator %q: must be proximity or kmeans", cfg.Voice.Allocator)
	}
	if cfg.Voice.MaxParticipantsPerGroup <= 0 {
		return fmt.Errorf("voice.max_participants_per_group must be > 0, got %d", cfg.Voice.MaxParticipantsPerGroup)
	}
	if cfg.Voice.RecomputeIntervalMs <= 0 {
		return fmt.Errorf("voice.recompute_interval_ms must be > 0, got %d", cfg.Voice.RecomputeIntervalMs)
	}
	return nil
}

func validateObjectLock(cfg *Config) error {
	if cfg.ObjectLock.LeaseTTLMs <= 0 {
		return fmt.Errorf("object_lock.lease_ttl_ms must be > 0, got %d", cfg.ObjectLock.LeaseTTLMs)
	}
	if cfg.ObjectLock.RefreshIntervalMs <= 0 {
		return fmt.Errorf("object_lock.refresh_interval_ms must be > 0, got %d", cfg.ObjectLock.RefreshIntervalMs)
	}
	if cfg.ObjectLock.MaxLocksPerUser <= 0 {
		return fmt.Errorf("object_lock.max_locks_per_user must be > 0, got %d", cfg.ObjectLock.MaxLocksPerUser)
	}
	return nil
}

// ValidateModeConstraints enforces deployment-mode-specific rules.
func ValidateModeConstraints(cfg *Config) error {
	switch cfg.Deployment.Mode {
	case ModeSingleVPS:
		if cfg.Resolver.Type == ResolverRedis {
			return fmt.Errorf("single-vps mode must not use redis resolver")
		}
		if cfg.Registry.Type == RegistryRedis {
			return fmt.Errorf("single-vps mode must not use redis registry")
		}
	case ModeDistributedK3s:
		if cfg.Resolver.Type != ResolverRedis {
			return fmt.Errorf("distributed-k3s mode requires redis resolver, got %q", cfg.Resolver.Type)
		}
		if cfg.Registry.Type != RegistryRedis {
			return fmt.Errorf("distributed-k3s mode requires redis registry, got %q", cfg.Registry.Type)
		}
	}
	return nil
}
