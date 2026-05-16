package cluster

import (
	"errors"
	"math"
	"sort"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"
)

// KMeansClusterAllocator implements ClusterAllocator using K-Means on player XZ positions.
//
// It is stateful: it retains the previous ClusterOutput to apply membership
// hysteresis on each successive Compute call. The state is valid only when
// Compute is called exclusively from the room loop goroutine (no mutex needed).
type KMeansClusterAllocator struct {
	prevOutput *ClusterOutput // retained for hysteresis; nil before the first Compute
}

// NewKMeansClusterAllocator creates a new KMeansClusterAllocator with no prior state.
func NewKMeansClusterAllocator() *KMeansClusterAllocator {
	return &KMeansClusterAllocator{}
}

// Reset clears the retained previous output. Call this when the room empties
// or when a clean slate is needed (e.g., after a major membership change where
// hysteresis from the previous state would be meaningless).
func (a *KMeansClusterAllocator) Reset() {
	a.prevOutput = nil
}

// Compute runs K-Means on the input player positions and returns cluster assignments.
//
// Steps:
//  1. Validate config. Return error for invalid config.
//  2. Filter out players with invalid (NaN/Inf) positions. Invalid player IDs are skipped.
//  3. Return empty output for empty valid input.
//  4. Compute K = ceil(n / TargetClusterSize), clamped to [1, n].
//  5. Initialise centroids deterministically (sorted by PlayerID, first K chosen).
//  6. Iterate K-Means up to MaxIterations until assignments stabilise.
//  7. Apply hysteresis post-pass against the previous ClusterOutput (if available).
//  8. Build and return ClusterOutput. Retain output for the next call's hysteresis pass.
//
// Must be called only from the room loop goroutine.
func (a *KMeansClusterAllocator) Compute(input ClusterInput, config ClusterConfig) (ClusterOutput, error) {
	if err := validateConfig(config); err != nil {
		return ClusterOutput{}, err
	}

	valid := filterValidPlayers(input.Players, config)
	n := len(valid)

	if n == 0 {
		out := emptyOutput()
		a.prevOutput = &out
		return out, nil
	}

	k := computeK(n, config.TargetClusterSize)
	centroids := initCentroids(valid, k)
	assignments := make([]int, n)

	for iter := 0; iter < config.MaxIterations; iter++ {
		changed := assignNearest(valid, centroids, assignments)
		recomputeCentroids(valid, assignments, centroids, k)
		if !changed {
			break
		}
	}

	if a.prevOutput != nil && config.MembershipHysteresis > 0 {
		applyHysteresis(valid, assignments, centroids, a.prevOutput, config.MembershipHysteresis)
	}

	out := buildOutput(valid, assignments, centroids, k)
	a.prevOutput = &out
	return out, nil
}

// --- Config validation -------------------------------------------------------

var (
	errTargetClusterSizeZero = errors.New("cluster: TargetClusterSize must be >= 1")
	errMaxIterationsZero     = errors.New("cluster: MaxIterations must be >= 1")
	errMaxPlayersZero        = errors.New("cluster: MaxPlayersPerRoom must be >= 1")
)

func validateConfig(cfg ClusterConfig) error {
	if cfg.TargetClusterSize <= 0 {
		return errTargetClusterSizeZero
	}
	if cfg.MaxIterations <= 0 {
		return errMaxIterationsZero
	}
	if cfg.MaxPlayersPerRoom <= 0 {
		return errMaxPlayersZero
	}
	return nil
}

// --- Input filtering ---------------------------------------------------------

// filterValidPlayers returns the subset of players with valid positions and non-empty IDs.
// Players beyond MaxPlayersPerRoom are silently truncated.
func filterValidPlayers(players []ClusterPlayer, cfg ClusterConfig) []ClusterPlayer {
	out := make([]ClusterPlayer, 0, len(players))
	for _, p := range players {
		if p.PlayerID == player.PlayerID("") {
			continue
		}
		if isInvalidPos(p.Position) {
			continue
		}
		out = append(out, p)
		if cfg.MaxPlayersPerRoom > 0 && len(out) >= cfg.MaxPlayersPerRoom {
			break
		}
	}
	return out
}

func isInvalidPos(pos spatial.EntityPosition) bool {
	return math.IsNaN(float64(pos.X)) || math.IsInf(float64(pos.X), 0) ||
		math.IsNaN(float64(pos.Z)) || math.IsInf(float64(pos.Z), 0)
}

// --- K computation -----------------------------------------------------------

// computeK returns ceil(n / targetSize), clamped to [1, n].
func computeK(n, targetSize int) int {
	k := (n + targetSize - 1) / targetSize
	if k < 1 {
		k = 1
	}
	if k > n {
		k = n
	}
	return k
}

// --- Centroid initialisation --------------------------------------------------

// initCentroids selects K initial centroids deterministically.
// Players are sorted by PlayerID (lexicographic) and the first K are chosen.
// This guarantees the same input always produces the same initial centroids.
func initCentroids(players []ClusterPlayer, k int) []spatial.EntityPosition {
	sorted := make([]ClusterPlayer, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].PlayerID < sorted[j].PlayerID
	})

	centroids := make([]spatial.EntityPosition, k)
	for i := range k {
		centroids[i] = sorted[i].Position
	}
	return centroids
}

// --- Assignment and centroid update ------------------------------------------

// assignNearest assigns each player to the nearest centroid (by squared distance).
// Returns true if any assignment changed.
func assignNearest(players []ClusterPlayer, centroids []spatial.EntityPosition, assignments []int) bool {
	changed := false
	for i, p := range players {
		nearest := nearestCentroidIndex(p.Position, centroids)
		if assignments[i] != nearest {
			assignments[i] = nearest
			changed = true
		}
	}
	return changed
}

// recomputeCentroids updates each centroid to the mean XZ position of its members.
// Empty clusters (no members) retain their previous centroid.
func recomputeCentroids(players []ClusterPlayer, assignments []int, centroids []spatial.EntityPosition, k int) {
	sumX := make([]float64, k)
	sumZ := make([]float64, k)
	count := make([]int, k)

	for i, p := range players {
		c := assignments[i]
		sumX[c] += float64(p.Position.X)
		sumZ[c] += float64(p.Position.Z)
		count[c]++
	}

	for c := range k {
		if count[c] > 0 {
			centroids[c] = spatial.EntityPosition{
				X: float32(sumX[c] / float64(count[c])),
				Z: float32(sumZ[c] / float64(count[c])),
			}
		}
		// empty cluster: retain previous centroid
	}
}

// --- Hysteresis --------------------------------------------------------------

// applyHysteresis stabilises cluster membership near centroid boundaries.
//
// For each player that had a previous cluster assignment, it only reassigns
// the player to the new centroid if the new centroid is more than `hysteresis`
// meters closer than the previous centroid. Otherwise it maps the player back
// to the new cluster whose centroid is nearest to their previous centroid.
//
// This prevents membership flicker when a player moves slowly near a boundary.
//
// Skeleton note: centroids are not recomputed after hysteresis reverts.
// The Centroids in the output reflect K-Means convergence, not post-hysteresis
// membership. A future optimisation may recompute centroids after the hysteresis
// pass for tighter accuracy of the prevCentroid used in the next compute call.
func applyHysteresis(
	players []ClusterPlayer,
	assignments []int,
	centroids []spatial.EntityPosition,
	prev *ClusterOutput,
	hysteresis float32,
) {
	for i, p := range players {
		prevID, ok := prev.Assignments[p.PlayerID]
		if !ok {
			continue // new player since last compute; no hysteresis applies
		}
		prevCentroid, ok := prev.Centroids[prevID]
		if !ok {
			continue
		}

		newCentroid := centroids[assignments[i]]
		distNew := euclideanDist(p.Position, newCentroid)
		distOld := euclideanDist(p.Position, prevCentroid)

		// Reassign only if the new centroid is clearly closer.
		// Rule from spec: reassign if distNew < distOld - hysteresis
		if distNew >= distOld-hysteresis {
			// Snap player to the new cluster whose centroid is nearest to the
			// previous centroid. This approximates "stay in the old cluster"
			// using the new centroid set (IDs are not stable across recomputes).
			assignments[i] = nearestCentroidIndex(prevCentroid, centroids)
		}
	}
}

// --- Output construction -----------------------------------------------------

func emptyOutput() ClusterOutput {
	return ClusterOutput{
		Assignments: make(map[player.PlayerID]ClusterID),
		Clusters:    make(map[ClusterID][]player.PlayerID),
		Centroids:   make(map[ClusterID]spatial.EntityPosition),
		K:           0,
	}
}

func buildOutput(players []ClusterPlayer, assignments []int, centroids []spatial.EntityPosition, k int) ClusterOutput {
	out := ClusterOutput{
		Assignments: make(map[player.PlayerID]ClusterID, len(players)),
		Clusters:    make(map[ClusterID][]player.PlayerID, k),
		Centroids:   make(map[ClusterID]spatial.EntityPosition, k),
		K:           k,
	}

	for c := range k {
		out.Centroids[ClusterID(c)] = centroids[c]
	}

	for i, p := range players {
		cid := ClusterID(assignments[i])
		out.Assignments[p.PlayerID] = cid
		out.Clusters[cid] = append(out.Clusters[cid], p.PlayerID)
	}

	return out
}

// --- Geometry helpers --------------------------------------------------------

func euclideanDist(a, b spatial.EntityPosition) float32 {
	dx := float64(a.X - b.X)
	dz := float64(a.Z - b.Z)
	return float32(math.Sqrt(dx*dx + dz*dz))
}

// nearestCentroidIndex returns the index in centroids closest to pos.
// Ties are broken by lower index.
func nearestCentroidIndex(pos spatial.EntityPosition, centroids []spatial.EntityPosition) int {
	best := 0
	bestDist := squaredDist(pos, centroids[0])
	for i := 1; i < len(centroids); i++ {
		d := squaredDist(pos, centroids[i])
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func squaredDist(a, b spatial.EntityPosition) float64 {
	dx := float64(a.X - b.X)
	dz := float64(a.Z - b.Z)
	return dx*dx + dz*dz
}
