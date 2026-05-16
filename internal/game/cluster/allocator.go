// Package cluster implements player position cluster allocation for Phase 1
// delta broadcast interest management.
//
// The ClusterAllocator interface groups players by XZ position into stable
// clusters. In Phase 1, K-Means is the sole implementation behind the interface.
// The cluster output drives which players receive each other's PlayerDelta updates.
//
// Design rules:
//   - ClusterAllocator.Compute must be called only by the room loop goroutine.
//   - Transport goroutines must not call Compute.
//   - Cluster input contains no transport metadata (KCP/WSS are invisible here).
//   - Cluster output is read-only at broadcast time; only the room loop writes it.
//   - Object sync, voice grouping, and object locking are deferred future scope.
package cluster

// ClusterAllocator is the interface for player position cluster grouping.
//
// Implementations must be deterministic for the same input, must not mutate
// room state, and must not import transport packages.
//
// Compute is called by the room loop on a cadence defined by ClusterConfig
// (interval, movement threshold, or membership change triggers). It must not
// be called from transport goroutines.
type ClusterAllocator interface {
	Compute(input ClusterInput, config ClusterConfig) (ClusterOutput, error)
}
