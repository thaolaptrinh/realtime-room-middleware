package cluster

import (
	"math"
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/spatial"
)

// cfg returns a default config for use in tests.
func cfg() ClusterConfig { return DefaultClusterConfig() }

// pos returns an EntityPosition from x and z.
func pos(x, z float32) spatial.EntityPosition { return spatial.Pos(x, z) }

// player helper builds a ClusterPlayer.
func cp(id string, x, z float32) ClusterPlayer {
	return ClusterPlayer{PlayerID: player.PlayerID(id), Position: pos(x, z)}
}

func TestComputeEmptyInput(t *testing.T) {
	a := NewKMeansClusterAllocator()
	out, err := a.Compute(ClusterInput{}, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 0 {
		t.Errorf("want K=0, got %d", out.K)
	}
	if len(out.Assignments) != 0 {
		t.Errorf("want empty assignments, got %d", len(out.Assignments))
	}
	if len(out.Clusters) != 0 {
		t.Errorf("want empty clusters, got %d", len(out.Clusters))
	}
}

func TestComputeSinglePlayer(t *testing.T) {
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{cp("p1", 0, 0)}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 1 {
		t.Fatalf("want K=1, got %d", out.K)
	}
	cid, ok := out.Assignments[player.PlayerID("p1")]
	if !ok {
		t.Fatal("p1 not in Assignments")
	}
	members := out.Clusters[cid]
	if len(members) != 1 || members[0] != player.PlayerID("p1") {
		t.Errorf("unexpected cluster members: %v", members)
	}
}

func TestComputeTwoNearbyPlayersSameCluster(t *testing.T) {
	// Two players 1m apart with TargetClusterSize=8 → K=ceil(2/8)=1 → same cluster.
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 0, 0),
		cp("p2", 1, 0),
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 1 {
		t.Fatalf("want K=1 for 2 players with target size 8, got %d", out.K)
	}
	c1 := out.Assignments[player.PlayerID("p1")]
	c2 := out.Assignments[player.PlayerID("p2")]
	if c1 != c2 {
		t.Errorf("nearby players should be in the same cluster: c1=%d c2=%d", c1, c2)
	}
}

func TestComputeTwoFarPlayersSeparateClusters(t *testing.T) {
	// Two players 200m apart with target size 1 → K=2 → separate clusters.
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = 1
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 0, 0),
		cp("p2", 200, 200),
	}}
	out, err := a.Compute(input, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 2 {
		t.Fatalf("want K=2, got %d", out.K)
	}
	c1 := out.Assignments[player.PlayerID("p1")]
	c2 := out.Assignments[player.PlayerID("p2")]
	if c1 == c2 {
		t.Error("far players should be in different clusters")
	}
}

func TestComputeMultipleClustersBeyondTargetSize(t *testing.T) {
	// 9 players with TargetClusterSize=4 → K=ceil(9/4)=3 clusters.
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = 4

	players := make([]ClusterPlayer, 9)
	for i := range 9 {
		// 3 groups of 3 players, each group 100m apart.
		players[i] = cp(
			string(rune('a'+i)),
			float32(i/3)*100,
			0,
		)
	}
	input := ClusterInput{Players: players}
	out, err := a.Compute(input, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 3 {
		t.Errorf("want K=3, got %d", out.K)
	}
	if len(out.Assignments) != 9 {
		t.Errorf("want 9 assignments, got %d", len(out.Assignments))
	}
}

func TestComputeKComputedCorrectly(t *testing.T) {
	cases := []struct {
		n, targetSize, wantK int
	}{
		{1, 8, 1},
		{8, 8, 1},
		{9, 8, 2},
		{16, 8, 2},
		{17, 8, 3},
		{200, 8, 25},
		{1, 1, 1},
		{5, 1, 5},
	}
	for _, tc := range cases {
		got := computeK(tc.n, tc.targetSize)
		if got != tc.wantK {
			t.Errorf("computeK(%d, %d) = %d, want %d", tc.n, tc.targetSize, got, tc.wantK)
		}
	}
}

func TestComputeDeterministicOutput(t *testing.T) {
	// Same input must produce the same output on separate allocators (no shared state).
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 10, 10),
		cp("p2", 10, 11),
		cp("p3", 200, 200),
		cp("p4", 201, 200),
	}}
	c := cfg()
	c.TargetClusterSize = 2

	a1 := NewKMeansClusterAllocator()
	a2 := NewKMeansClusterAllocator()
	out1, _ := a1.Compute(input, c)
	out2, _ := a2.Compute(input, c)

	for pid := range out1.Assignments {
		if out1.Assignments[pid] != out2.Assignments[pid] {
			t.Errorf("non-deterministic: player %q assigned to different clusters", pid)
		}
	}
}

func TestComputeAllSamePosition(t *testing.T) {
	// 3 players at the exact same position with TargetClusterSize=8 → K=1.
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 5, 5),
		cp("p2", 5, 5),
		cp("p3", 5, 5),
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 1 {
		t.Errorf("want K=1 when all players at same position, got %d", out.K)
	}
}

func TestComputeAllPlayersAssigned(t *testing.T) {
	// Every valid player must appear in Assignments (fallback rule: no player left unassigned).
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = 3

	players := make([]ClusterPlayer, 10)
	for i := range 10 {
		players[i] = cp(string(rune('a'+i)), float32(i)*50, 0)
	}
	input := ClusterInput{Players: players}
	out, err := a.Compute(input, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Assignments) != 10 {
		t.Errorf("want all 10 players assigned, got %d", len(out.Assignments))
	}
}

func TestComputeMaxIterationsRespected(t *testing.T) {
	// Setting MaxIterations=1 must not panic and must produce a valid output.
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.MaxIterations = 1
	c.TargetClusterSize = 2
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 0, 0),
		cp("p2", 100, 0),
		cp("p3", 200, 0),
		cp("p4", 300, 0),
	}}
	out, err := a.Compute(input, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Assignments) != 4 {
		t.Errorf("want 4 assignments, got %d", len(out.Assignments))
	}
}

// --- Invalid input tests ------------------------------------------------------

func TestComputeInvalidPositionNaN(t *testing.T) {
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", float32(math.NaN()), 0),
		cp("p2", 0, 0), // valid
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// p1 is skipped; only p2 is clustered.
	if _, ok := out.Assignments[player.PlayerID("p1")]; ok {
		t.Error("player with NaN position should not appear in Assignments")
	}
	if _, ok := out.Assignments[player.PlayerID("p2")]; !ok {
		t.Error("valid player p2 should appear in Assignments")
	}
}

func TestComputeInvalidPositionInf(t *testing.T) {
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", float32(math.Inf(1)), 0),
		cp("p2", 5, 5), // valid
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := out.Assignments[player.PlayerID("p1")]; ok {
		t.Error("player with Inf position should not appear in Assignments")
	}
	if _, ok := out.Assignments[player.PlayerID("p2")]; !ok {
		t.Error("valid player p2 should appear in Assignments")
	}
}

func TestComputeInvalidPlayerIDEmpty(t *testing.T) {
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		{PlayerID: player.PlayerID(""), Position: pos(0, 0)},
		cp("p1", 0, 0),
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only p1 should be assigned.
	if len(out.Assignments) != 1 {
		t.Errorf("want 1 assignment (p1 only), got %d", len(out.Assignments))
	}
	if _, ok := out.Assignments[player.PlayerID("p1")]; !ok {
		t.Error("valid player p1 should be in Assignments")
	}
}

func TestComputeAllInvalidPositions(t *testing.T) {
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", float32(math.NaN()), 0),
		cp("p2", 0, float32(math.Inf(-1))),
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.K != 0 {
		t.Errorf("want K=0 for all-invalid input, got %d", out.K)
	}
}

// --- Invalid config tests ----------------------------------------------------

func TestComputeInvalidConfigTargetClusterSizeZero(t *testing.T) {
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = 0
	_, err := a.Compute(ClusterInput{Players: []ClusterPlayer{cp("p1", 0, 0)}}, c)
	if err == nil {
		t.Error("expected error for TargetClusterSize=0")
	}
}

func TestComputeInvalidConfigNegativeTargetClusterSize(t *testing.T) {
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = -1
	_, err := a.Compute(ClusterInput{Players: []ClusterPlayer{cp("p1", 0, 0)}}, c)
	if err == nil {
		t.Error("expected error for TargetClusterSize=-1")
	}
}

func TestComputeInvalidConfigMaxIterationsZero(t *testing.T) {
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.MaxIterations = 0
	_, err := a.Compute(ClusterInput{Players: []ClusterPlayer{cp("p1", 0, 0)}}, c)
	if err == nil {
		t.Error("expected error for MaxIterations=0")
	}
}

func TestComputeInvalidConfigMaxPlayersZero(t *testing.T) {
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.MaxPlayersPerRoom = 0
	_, err := a.Compute(ClusterInput{Players: []ClusterPlayer{cp("p1", 0, 0)}}, c)
	if err == nil {
		t.Error("expected error for MaxPlayersPerRoom=0")
	}
}

// --- Transport-agnostic tests -------------------------------------------------

func TestComputeTransportTypeNotInInput(t *testing.T) {
	// ClusterPlayer has no transport field — KCP and WSS players look identical.
	// Two players at the same position but "different transports" (only difference
	// is their ID suffix convention used by the test) must be in the same cluster.
	a := NewKMeansClusterAllocator()
	input := ClusterInput{Players: []ClusterPlayer{
		cp("kcp-player-1", 0, 0),
		cp("wss-player-1", 1, 0),
	}}
	out, err := a.Compute(input, cfg())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With TargetClusterSize=8 and n=2, K=1 — both must land in the same cluster.
	if out.K != 1 {
		t.Fatalf("want K=1, got %d", out.K)
	}
	c1 := out.Assignments[player.PlayerID("kcp-player-1")]
	c2 := out.Assignments[player.PlayerID("wss-player-1")]
	if c1 != c2 {
		t.Error("KCP and WSS players at the same position must be in the same cluster")
	}
}

// --- Hysteresis tests ---------------------------------------------------------
//
// Setup used by all three hysteresis/reset tests:
//   4 players, TargetClusterSize=2 → K=2
//   base positions:  p1=(5,0), p2=(0,0), p3=(20,0), p4=(21,0)
//   First compute:   p1,p2 → cluster A (centroid≈(2.5,0))
//                    p3,p4 → cluster B (centroid≈(20.5,0))
//   Cluster boundary ≈ midpoint (11.5,0)
//
// Key positions for p1 in the second compute:
//   (12,0) — just past boundary: K-Means assigns to B, but hysteresis=5 reverts → stays with p2
//   (19,0) — far past boundary:  K-Means assigns to B, improvement 15.5m > 5m → reassignment kept

func TestHysteresisPreventsMembershipFlicker(t *testing.T) {
	// p1 crosses the cluster boundary by only ~0.5m past the midpoint.
	// With hysteresis=5m, the small improvement does not justify reassignment
	// and p1 is reverted to its previous cluster (with p2).
	c := cfg()
	c.TargetClusterSize = 2
	c.MembershipHysteresis = 5.0

	a := NewKMeansClusterAllocator()

	// First compute — establish two well-separated groups.
	_, err := a.Compute(ClusterInput{Players: []ClusterPlayer{
		cp("p1", 5, 0),
		cp("p2", 0, 0),
		cp("p3", 20, 0),
		cp("p4", 21, 0),
	}}, c)
	if err != nil {
		t.Fatalf("first compute: %v", err)
	}

	// Second compute — p1 moves to (12,0), just past the ~11.5 boundary.
	// K-Means assigns p1 to cluster B (centroid≈(17.67,0)).
	// distNew≈5.67m, distOld≈9.5m, improvement≈3.83m < hysteresis=5m → revert.
	out2, err := a.Compute(ClusterInput{Players: []ClusterPlayer{
		cp("p1", 12, 0),
		cp("p2", 0, 0),
		cp("p3", 20, 0),
		cp("p4", 21, 0),
	}}, c)
	if err != nil {
		t.Fatalf("second compute: %v", err)
	}

	c1 := out2.Assignments[player.PlayerID("p1")]
	c2 := out2.Assignments[player.PlayerID("p2")]
	if c1 != c2 {
		t.Errorf("hysteresis should keep p1 with p2 after a small boundary crossing; "+
			"p1 cluster=%d, p2 cluster=%d", c1, c2)
	}
}

func TestHysteresisAllowsReassignmentOnLargeMovement(t *testing.T) {
	// p1 moves far past the cluster boundary to (19,0) — deep into cluster B territory.
	// distNew≈1m, distOld≈16.5m, improvement≈15.5m > hysteresis=5m → reassignment kept.
	c := cfg()
	c.TargetClusterSize = 2
	c.MembershipHysteresis = 5.0

	a := NewKMeansClusterAllocator()

	// First compute — establish two well-separated groups.
	_, err := a.Compute(ClusterInput{Players: []ClusterPlayer{
		cp("p1", 5, 0),
		cp("p2", 0, 0),
		cp("p3", 20, 0),
		cp("p4", 21, 0),
	}}, c)
	if err != nil {
		t.Fatalf("first compute: %v", err)
	}

	// Second compute — p1 moves deep into cluster B territory.
	out2, err := a.Compute(ClusterInput{Players: []ClusterPlayer{
		cp("p1", 19, 0),
		cp("p2", 0, 0),
		cp("p3", 20, 0),
		cp("p4", 21, 0),
	}}, c)
	if err != nil {
		t.Fatalf("second compute: %v", err)
	}

	c1 := out2.Assignments[player.PlayerID("p1")]
	c3 := out2.Assignments[player.PlayerID("p3")]
	c2 := out2.Assignments[player.PlayerID("p2")]
	if c1 != c3 {
		t.Errorf("p1 moved far and should join p3's cluster; "+
			"p1 cluster=%d, p3 cluster=%d", c1, c3)
	}
	if c1 == c2 {
		t.Error("p1 should no longer be in p2's cluster after moving far away")
	}
}

// --- Output structure tests ---------------------------------------------------

func TestOutputClustersAndAssignmentsConsistent(t *testing.T) {
	// Every player in Assignments[pid]=cid must appear in Clusters[cid].
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = 3
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 0, 0),
		cp("p2", 1, 0),
		cp("p3", 100, 0),
		cp("p4", 101, 0),
		cp("p5", 200, 0),
		cp("p6", 201, 0),
	}}
	out, err := a.Compute(input, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for pid, cid := range out.Assignments {
		members := out.Clusters[cid]
		found := false
		for _, m := range members {
			if m == pid {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("player %q assigned to cluster %d but not in Clusters[%d]", pid, cid, cid)
		}
	}

	// Total members across all clusters == total assignments.
	total := 0
	for _, members := range out.Clusters {
		total += len(members)
	}
	if total != len(out.Assignments) {
		t.Errorf("total cluster members %d != assignments %d", total, len(out.Assignments))
	}
}

func TestOutputCentroidsCountMatchesK(t *testing.T) {
	a := NewKMeansClusterAllocator()
	c := cfg()
	c.TargetClusterSize = 2
	input := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 0, 0),
		cp("p2", 100, 0),
		cp("p3", 200, 0),
	}}
	out, err := a.Compute(input, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Centroids) != out.K {
		t.Errorf("Centroids count %d != K %d", len(out.Centroids), out.K)
	}
}

// --- No-import-of-transport test (compile-time enforcement) -------------------
// The cluster package must not import transport packages.
// This is enforced by the package layout — any such import causes a compile error
// due to the circular dependency rules in this project.
// No runtime test is needed; the absence of transport imports in kmeans.go
// and types.go is sufficient.

func TestDefaultClusterConfigValues(t *testing.T) {
	c := DefaultClusterConfig()
	if !c.Enabled {
		t.Error("Enabled should default to true")
	}
	if c.TargetClusterSize != 8 {
		t.Errorf("TargetClusterSize: want 8, got %d", c.TargetClusterSize)
	}
	if c.MaxClusterRadius != 30.0 {
		t.Errorf("MaxClusterRadius: want 30.0, got %f", c.MaxClusterRadius)
	}
	if c.ReclusterIntervalTicks != 10 {
		t.Errorf("ReclusterIntervalTicks: want 10, got %d", c.ReclusterIntervalTicks)
	}
	if c.MovementThreshold != 2.0 {
		t.Errorf("MovementThreshold: want 2.0, got %f", c.MovementThreshold)
	}
	if c.MembershipHysteresis != 5.0 {
		t.Errorf("MembershipHysteresis: want 5.0, got %f", c.MembershipHysteresis)
	}
	if c.MaxIterations != 20 {
		t.Errorf("MaxIterations: want 20, got %d", c.MaxIterations)
	}
	if c.MaxPlayersPerRoom != 200 {
		t.Errorf("MaxPlayersPerRoom: want 200, got %d", c.MaxPlayersPerRoom)
	}
}

func TestResetClearsPreviousOutput(t *testing.T) {
	// With hysteresis=100m (very large), p1 crossing the boundary at (12,0) is
	// reverted back to p2's cluster. After Reset(), the previous state is gone
	// and K-Means freely assigns p1 to p3's cluster.
	c := cfg()
	c.TargetClusterSize = 2
	c.MembershipHysteresis = 100.0

	base := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 5, 0),
		cp("p2", 0, 0),
		cp("p3", 20, 0),
		cp("p4", 21, 0),
	}}
	moved := ClusterInput{Players: []ClusterPlayer{
		cp("p1", 12, 0), // just past boundary
		cp("p2", 0, 0),
		cp("p3", 20, 0),
		cp("p4", 21, 0),
	}}

	// Without Reset: large hysteresis keeps p1 in p2's cluster.
	a1 := NewKMeansClusterAllocator()
	if _, err := a1.Compute(base, c); err != nil {
		t.Fatalf("first compute (no reset): %v", err)
	}
	out1, err := a1.Compute(moved, c)
	if err != nil {
		t.Fatalf("second compute (no reset): %v", err)
	}
	if out1.Assignments[player.PlayerID("p1")] != out1.Assignments[player.PlayerID("p2")] {
		t.Error("without Reset and hysteresis=100, p1 should remain with p2")
	}

	// With Reset: previous state is gone; K-Means runs fresh and assigns p1 to p3's cluster.
	a2 := NewKMeansClusterAllocator()
	if _, err := a2.Compute(base, c); err != nil {
		t.Fatalf("first compute (with reset): %v", err)
	}
	a2.Reset()
	out2, err := a2.Compute(moved, c)
	if err != nil {
		t.Fatalf("second compute (after reset): %v", err)
	}
	if out2.Assignments[player.PlayerID("p1")] == out2.Assignments[player.PlayerID("p2")] {
		t.Error("after Reset, p1 at (12,0) should join p3's cluster, not stay with p2")
	}
	if out2.Assignments[player.PlayerID("p1")] != out2.Assignments[player.PlayerID("p3")] {
		t.Errorf("after Reset, p1 should be in p3's cluster; "+
			"p1=%d, p3=%d", out2.Assignments[player.PlayerID("p1")], out2.Assignments[player.PlayerID("p3")])
	}
}
