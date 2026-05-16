package cluster

import (
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"
)

// ClusterID identifies a cluster within a room for a single recompute cycle.
// ClusterIDs are not stable across recomputes — the room loop rebinds them
// after each ClusterAllocator.Compute call.
type ClusterID uint32

// ClusterConfig holds K-Means cluster allocator configuration.
// All fields are stored in RoomConfig and passed to Compute on each recompute.
type ClusterConfig struct {
	// Enabled controls whether cluster-based interest is active.
	// When false, the broadcast path falls back to radius-based interest.
	Enabled bool

	// TargetClusterSize is the target number of players per cluster.
	// K = ceil(n / TargetClusterSize), clamped to [1, n].
	// Must be >= 1. Default: 8.
	TargetClusterSize int

	// MaxClusterRadius is the maximum distance in meters from the centroid
	// that a cluster member may reach. Players beyond this distance are still
	// assigned (no player is ever left unassigned), but it bounds expected
	// cluster radius at design time. Default: 30.0 m.
	MaxClusterRadius float32

	// ReclusterIntervalTicks is the room tick interval between periodic full
	// recomputes (independent of movement or membership triggers). Default: 10.
	ReclusterIntervalTicks int

	// MovementThreshold is the minimum single-player movement (meters) since
	// the last recompute that triggers an early recompute. Default: 2.0 m.
	MovementThreshold float32

	// MembershipHysteresis is the distance margin (meters) a player must
	// move past a cluster boundary before being reassigned. Prevents
	// membership flicker near centroid boundaries. Default: 5.0 m.
	MembershipHysteresis float32

	// MaxIterations caps K-Means iterations per recompute. Default: 20.
	MaxIterations int

	// MaxPlayersPerRoom is the upper bound used to validate input size.
	// Default: 200.
	MaxPlayersPerRoom int
}

// DefaultClusterConfig returns a ClusterConfig with production defaults.
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		Enabled:                true,
		TargetClusterSize:      8,
		MaxClusterRadius:       30.0,
		ReclusterIntervalTicks: 10,
		MovementThreshold:      2.0,
		MembershipHysteresis:   5.0,
		MaxIterations:          20,
		MaxPlayersPerRoom:      200,
	}
}

// ClusterPlayer is a single player entry supplied to the cluster allocator.
// Transport type is intentionally absent — clusters are transport-agnostic.
// KCP and WSS players are indistinguishable at this layer.
type ClusterPlayer struct {
	PlayerID player.PlayerID
	Position spatial.EntityPosition // XZ only; Y (vertical) is ignored.
}

// ClusterInput is the input to ClusterAllocator.Compute.
// It contains only player positions. No transport metadata, no room state.
type ClusterInput struct {
	Players []ClusterPlayer
}

// ClusterOutput is the result of a single ClusterAllocator.Compute call.
// The room loop stores this and reads it at broadcast tick to build interest sets.
// ClusterIDs are local to this output and not stable across recomputes.
type ClusterOutput struct {
	// Assignments maps each player ID to its cluster ID.
	Assignments map[player.PlayerID]ClusterID

	// Clusters maps each cluster ID to its member player IDs.
	// Players in the same cluster receive each other's PlayerDelta updates.
	Clusters map[ClusterID][]player.PlayerID

	// Centroids maps each cluster ID to its centroid position (XZ plane).
	// Used for diagnostics and by the hysteresis pass.
	Centroids map[ClusterID]spatial.EntityPosition

	// K is the number of clusters produced.
	K int
}
