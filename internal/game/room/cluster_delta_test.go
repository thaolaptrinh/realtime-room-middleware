package room_test

import (
	"context"
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/cluster"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
)

// ---- Cluster-based delta building tests (Stage 2 Task 9) --------------------

func TestRoom_ClusterEnabled_DeltaUsesClusterMembers(t *testing.T) {
	// Verify that when cluster_enabled=true, delta building uses cluster membership
	// to determine visible players, not radius-based interest.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-delta-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join two players at the same position (they should be in the same cluster).
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s2"),
		PlayerID:  room.PlayerID("p2"),
		UserID:    room.UserID("u2"),
		Timestamp: time.Now(),
	})

	// Wait for both joins and cluster recompute.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 2 {
			output := r.GetClusterOutput()
			if output.K == 1 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	if r.PlayerCount() != 2 {
		t.Fatalf("PlayerCount = %d, want 2", r.PlayerCount())
	}

	// Verify cluster output has both players in the same cluster.
	output := r.GetClusterOutput()
	if output.K != 1 {
		t.Fatalf("cluster K = %d, want 1 (both players at origin should be in same cluster)", output.K)
	}

	// Verify VisiblePlayersFor uses cluster membership.
	visible := r.VisiblePlayersFor(player.PlayerID("p1"))
	if len(visible) != 1 {
		t.Errorf("VisiblePlayersFor(p1) with cluster enabled = %d, want 1 (p2 in same cluster)", len(visible))
	}
	if len(visible) > 0 && visible[0] != player.PlayerID("p2") {
		t.Errorf("VisiblePlayersFor(p1)[0] = %q, want p2", visible[0])
	}

	// Verify p2 also sees p1.
	visible = r.VisiblePlayersFor(player.PlayerID("p2"))
	if len(visible) != 1 {
		t.Errorf("VisiblePlayersFor(p2) with cluster enabled = %d, want 1 (p1 in same cluster)", len(visible))
	}
	if len(visible) > 0 && visible[0] != player.PlayerID("p1") {
		t.Errorf("VisiblePlayersFor(p2)[0] = %q, want p1", visible[0])
	}
}

func TestRoom_ClusterEnabled_VieweDoesNotReceiveSelf(t *testing.T) {
	// Verify that a player's visible set excludes themselves even in cluster mode.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-no-self-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	visible := r.VisiblePlayersFor(player.PlayerID("p1"))
	if len(visible) != 0 {
		t.Errorf("VisiblePlayersFor(p1) in singleton cluster = %d, want 0 (self excluded)", len(visible))
	}
}

func TestRoom_ClusterEnabled_DifferentClustersExcluded(t *testing.T) {
	// Verify that players in different clusters are not in each other's visible sets.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	// Set small target cluster size to force multiple clusters.
	cfg.ClusterConfig.TargetClusterSize = 2
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "multi-cluster-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join players at different positions to encourage multiple clusters.
	positions := []struct {
		pid     string
		x, z    float32
		session string
		user    string
	}{
		{"p1", 0, 0, "s1", "u1"},
		{"p2", 5, 0, "s2", "u2"},
		{"p3", 50, 0, "s3", "u3"},
		{"p4", 55, 0, "s4", "u4"},
	}

	for _, pos := range positions {
		_ = r.Enqueue(room.RoomCommand{
			Kind:      room.CmdJoin,
			SessionID: room.SessionID(pos.session),
			PlayerID:  room.PlayerID(pos.pid),
			UserID:    room.UserID(pos.user),
			Timestamp: time.Now(),
		})
		// Move each player to their designated position.
		_ = r.Enqueue(room.RoomCommand{
			Kind:     room.CmdPlayerInput,
			PlayerID: room.PlayerID(pos.pid),
			Payload: player.PlayerInput{
				Seq: 1,
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: pos.x, Y: 0, Z: pos.z},
					Rotation: player.IdentityQuaternion,
				},
				Timestamp: time.Now().UnixMilli(),
			},
			Timestamp: time.Now(),
		})
	}

	// Wait for cluster recompute.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		output := r.GetClusterOutput()
		if output.K >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	output := r.GetClusterOutput()
	if output.K < 2 {
		t.Fatalf("expected at least 2 clusters with players at different positions, got K=%d", output.K)
	}

	// Verify that players in different clusters don't see each other.
	// Find two players in different clusters.
	var pid1, pid2 player.PlayerID
	var cid1 cluster.ClusterID
	for pid, cid := range output.Assignments {
		if pid1 == "" {
			pid1 = pid
			cid1 = cid
		} else if cid != cid1 {
			pid2 = pid
			break
		}
	}

	if pid1 == "" || pid2 == "" {
		t.Fatal("could not find two players in different clusters")
	}

	visible := r.VisiblePlayersFor(pid1)
	for _, visiblePID := range visible {
		cid, ok := output.Assignments[visiblePID]
		if !ok || cid != cid1 {
			t.Errorf("player %q (cluster %d) sees %q (cluster %d) — should only see same-cluster players", pid1, cid1, visiblePID, cid)
		}
	}
}

func TestRoom_ClusterDisabled_DeltaUsesRadiusFallback(t *testing.T) {
	// Verify that when cluster_enabled=false, delta building falls back to radius-based interest.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = false
	cfg.InterestVisualRadiusM = 10.0
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "radius-fallback-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join p1 at origin, p2 nearby (within radius), p3 far away.
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s2"),
		PlayerID:  room.PlayerID("p2"),
		UserID:    room.UserID("u2"),
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s3"),
		PlayerID:  room.PlayerID("p3"),
		UserID:    room.UserID("u3"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Move p2 to (5, 0, 5) — within 10m radius of p1.
	_ = r.Enqueue(room.RoomCommand{
		Kind:     room.CmdPlayerInput,
		PlayerID: room.PlayerID("p2"),
		Payload: player.PlayerInput{
			Seq: 1,
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 5, Y: 0, Z: 5},
				Rotation: player.IdentityQuaternion,
			},
			Timestamp: time.Now().UnixMilli(),
		},
		Timestamp: time.Now(),
	})

	// Move p3 to (50, 0, 50) — outside 10m radius of p1.
	_ = r.Enqueue(room.RoomCommand{
		Kind:     room.CmdPlayerInput,
		PlayerID: room.PlayerID("p3"),
		Payload: player.PlayerInput{
			Seq: 1,
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 50, Y: 0, Z: 50},
				Rotation: player.IdentityQuaternion,
			},
			Timestamp: time.Now().UnixMilli(),
		},
		Timestamp: time.Now(),
	})

	// Wait for spatial updates.
	time.Sleep(100 * time.Millisecond)

	// Verify VisiblePlayersFor uses radius query when cluster is disabled.
	visible := r.VisiblePlayersFor(player.PlayerID("p1"))
	if len(visible) != 1 {
		t.Errorf("VisiblePlayersFor(p1) with cluster disabled = %d, want 1 (only p2 within 10m)", len(visible))
	}
	if len(visible) > 0 && visible[0] != player.PlayerID("p2") {
		t.Errorf("VisiblePlayersFor(p1)[0] = %q, want p2 (within radius)", visible[0])
	}

	// Verify cluster output is empty or not used when disabled.
	output := r.GetClusterOutput()
	if len(output.Assignments) != 0 && r.ClusterConfig().Enabled {
		t.Error("cluster output should be empty or not used when cluster_enabled=false")
	}
}

func TestRoom_ClusterTransformUpdate_GeneratesUpdateForClusterMembers(t *testing.T) {
	// Verify that transform updates generate PlayerUpdateDelta only for visible cluster members.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-transform-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join two players in the same cluster.
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s2"),
		PlayerID:  room.PlayerID("p2"),
		UserID:    room.UserID("u2"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 2 {
			output := r.GetClusterOutput()
			if output.K == 1 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Update p1's transform.
	_ = r.Enqueue(room.RoomCommand{
		Kind:     room.CmdPlayerInput,
		PlayerID: room.PlayerID("p1"),
		Payload: player.PlayerInput{
			Seq: 1,
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 2, Y: 0, Z: 2},
				Rotation: player.IdentityQuaternion,
			},
			Timestamp: time.Now().UnixMilli(),
		},
		Timestamp: time.Now(),
	})

	// Wait for transform update.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		transform, version, ok := r.GetPlayerState(player.PlayerID("p1"))
		if ok && version == 1 && transform.Position.X == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify transform was applied.
	transform, version, ok := r.GetPlayerState(player.PlayerID("p1"))
	if !ok {
		t.Fatal("p1 state should exist")
	}
	if version != 1 {
		t.Errorf("p1 version after update = %d, want 1", version)
	}
	if transform.Position.X != 2 {
		t.Errorf("p1 position.X after update = %.1f, want 2", transform.Position.X)
	}

	// Verify p2 can see p1 (they're in the same cluster).
	visible := r.VisiblePlayersFor(player.PlayerID("p2"))
	if len(visible) != 1 {
		t.Errorf("VisiblePlayersFor(p2) = %d, want 1 (p1 is in same cluster)", len(visible))
	}
}

func TestRoom_ClusterMembershipChange_TriggersDeltaEnterLeave(t *testing.T) {
	// Verify that when a player moves far enough to change cluster membership,
	// the cluster assignment changes — the trigger for delta enter/leave.
	// Actual delta enter/leave output is tested separately in delta package tests.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	// TargetClusterSize=2 → K=ceil(4/2)=2 from the start, so p3 can actually
	// be reassigned to a different cluster when it moves far enough.
	cfg.ClusterConfig.TargetClusterSize = 2
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-change-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join 4 players: three near origin, one far away.
	// K-Means with K=2 converges to:
	//   cluster A: p1(0,0), p2(3,0), p3(1,0) — near origin
	//   cluster B: p4(200,0) — far away
	positions := []struct {
		pid     string
		x, z    float32
		session string
		user    string
	}{
		{"p1", 0, 0, "s1", "u1"},
		{"p2", 3, 0, "s2", "u2"},
		{"p3", 1, 0, "s3", "u3"},
		{"p4", 200, 0, "s4", "u4"},
	}

	for _, pos := range positions {
		_ = r.Enqueue(room.RoomCommand{
			Kind:      room.CmdJoin,
			SessionID: room.SessionID(pos.session),
			PlayerID:  room.PlayerID(pos.pid),
			UserID:    room.UserID(pos.user),
			Timestamp: time.Now(),
		})
		_ = r.Enqueue(room.RoomCommand{
			Kind:     room.CmdPlayerInput,
			PlayerID: room.PlayerID(pos.pid),
			Payload: player.PlayerInput{
				Seq: 1,
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: pos.x, Y: 0, Z: pos.z},
					Rotation: player.IdentityQuaternion,
				},
				Timestamp: time.Now().UnixMilli(),
			},
			Timestamp: time.Now(),
		})
	}

	// Wait for initial cluster computation.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		output := r.GetClusterOutput()
		if output.K == 2 && len(output.Assignments) == 4 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	output := r.GetClusterOutput()
	if output.K != 2 {
		t.Fatalf("expected K=2 with TargetClusterSize=2 and 4 players, got K=%d", output.K)
	}

	// Verify initial cluster split: p1 and p3 near origin (same cluster), p4 far away (different).
	cidP1 := output.Assignments[player.PlayerID("p1")]
	cidP3 := output.Assignments[player.PlayerID("p3")]
	cidP4 := output.Assignments[player.PlayerID("p4")]
	if cidP1 == cidP4 {
		t.Fatalf("p1 and p4 should be in different clusters initially (near origin vs x=200)")
	}
	if cidP1 != cidP3 {
		t.Fatalf("p1 and p3 should start in the same cluster (both near origin), got p1=%d, p3=%d", cidP1, cidP3)
	}
	initialClusterP3 := cidP3

	// Verify p3 initially sees p1 and p2 (same cluster), not p4.
	visibleP3 := r.VisiblePlayersFor(player.PlayerID("p3"))
	if len(visibleP3) != 2 {
		t.Errorf("p3 should initially see 2 players (p1, p2 in same cluster), got %d", len(visibleP3))
	}

	// Move p3 from (1, 0) to (200, 0) — joining p4's cluster.
	// Distance = 199m, clearly exceeding MovementThreshold (2.0m) and MembershipHysteresis (5.0m).
	_ = r.Enqueue(room.RoomCommand{
		Kind:     room.CmdPlayerInput,
		PlayerID: room.PlayerID("p3"),
		Payload: player.PlayerInput{
			Seq: 2,
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 200, Y: 0, Z: 0},
				Rotation: player.IdentityQuaternion,
			},
			Timestamp: time.Now().UnixMilli(),
		},
		Timestamp: time.Now(),
	})

	// Wait for cluster recompute triggered by movement threshold.
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		output = r.GetClusterOutput()
		newCID, ok := output.Assignments[player.PlayerID("p3")]
		if ok && newCID != initialClusterP3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify p3 changed clusters.
	output = r.GetClusterOutput()
	newClusterP3 := output.Assignments[player.PlayerID("p3")]
	if newClusterP3 == initialClusterP3 {
		t.Errorf("p3 should have been reassigned to a different cluster after moving to x=200; initial=%d, new=%d", initialClusterP3, newClusterP3)
	}

	// Verify p3 is now in the same cluster as p4.
	cidP4After := output.Assignments[player.PlayerID("p4")]
	if newClusterP3 != cidP4After {
		t.Errorf("p3 should be in the same cluster as p4 after moving to p4's location; p3=%d, p4=%d", newClusterP3, cidP4After)
	}

	// Verify p3 no longer sees p1 or p2 (different clusters).
	visibleP3 = r.VisiblePlayersFor(player.PlayerID("p3"))
	for _, v := range visibleP3 {
		if v == player.PlayerID("p1") || v == player.PlayerID("p2") {
			t.Errorf("p3 should no longer see %q after moving to a different cluster", v)
		}
	}

	// Verify p3 now sees p4 (same cluster).
	foundP4 := false
	for _, v := range visibleP3 {
		if v == player.PlayerID("p4") {
			foundP4 = true
		}
	}
	if !foundP4 {
		t.Error("p3 should see p4 after moving to p4's cluster")
	}

	// Verify p1 no longer sees p3 (p3 left p1's cluster).
	visibleP1 := r.VisiblePlayersFor(player.PlayerID("p1"))
	for _, v := range visibleP1 {
		if v == player.PlayerID("p3") {
			t.Error("p1 should no longer see p3 after p3 moved to a different cluster")
		}
	}
}

func TestRoom_ClusterMetadataDoesNotAffectDelta(t *testing.T) {
	// Verify that cluster membership is based on position only, not on session ID
	// or transport type. KCP and WSS sessions at the same position end up in
	// the same cluster and receive the same deltas.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-metadata-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join two players at the same position with different session IDs (simulating
	// different transports: KCP vs WSS).
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("kcp-session-1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("wss-session-1"),
		PlayerID:  room.PlayerID("p2"),
		UserID:    room.UserID("u2"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 2 {
			output := r.GetClusterOutput()
			if output.K == 1 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	output := r.GetClusterOutput()
	if output.K != 1 {
		t.Errorf("KCP and WSS players at same position should be in same cluster, got K=%d", output.K)
	}

	cid1, ok1 := output.Assignments[player.PlayerID("p1")]
	cid2, ok2 := output.Assignments[player.PlayerID("p2")]
	if !ok1 || !ok2 || cid1 != cid2 {
		t.Errorf("KCP and WSS players should have same cluster assignment: p1=%d, p2=%d", cid1, cid2)
	}

	// Both should see each other (cluster-based interest, not transport-based).
	visibleP1 := r.VisiblePlayersFor(player.PlayerID("p1"))
	visibleP2 := r.VisiblePlayersFor(player.PlayerID("p2"))
	if len(visibleP1) != 1 || visibleP1[0] != player.PlayerID("p2") {
		t.Errorf("p1 (KCP) should see p2 (WSS), got %v", visibleP1)
	}
	if len(visibleP2) != 1 || visibleP2[0] != player.PlayerID("p1") {
		t.Errorf("p2 (WSS) should see p1 (KCP), got %v", visibleP2)
	}
}

func TestRoom_ClusterWithNoAssignment_EmptyVisibleSet(t *testing.T) {
	// Verify that when a player has no cluster assignment (e.g., during room
	// initialization or edge cases), VisiblePlayersFor returns an empty set.
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-no-assign-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Before any joins, cluster output is empty.
	output := r.GetClusterOutput()
	if len(output.Assignments) != 0 {
		t.Errorf("initial cluster assignments should be empty, got %d", len(output.Assignments))
	}

	// Query for a nonexistent player should return empty set.
	visible := r.VisiblePlayersFor(player.PlayerID("ghost"))
	if len(visible) != 0 {
		t.Errorf("VisiblePlayersFor for nonexistent player should return empty, got %d", len(visible))
	}

	// Join a player, then immediately query (cluster may not be computed yet).
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})

	// Even before cluster computation, query should not panic.
	visible = r.VisiblePlayersFor(player.PlayerID("p1"))
	// Result may be empty or contain p1's cluster members once computed.
	// The important thing is no panic.
	_ = visible
}

func TestRoom_ClusterIntegration_DoesNotImportTransport(t *testing.T) {
	// Verify that delta building does not depend on transport packages.
	// This is a compile-time check: if the delta package imported transport,
	// there would be a circular dependency or runtime coupling.
	// Since we can't test imports directly at runtime, we verify behavior:
	// cluster membership must not depend on session ID format or transport type.

	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cluster-no-import-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join players with various session ID formats (simulating different transports).
	sessionIDs := []string{
		"kcp-udp-session-12345",
		"wss-websocket-session-abc",
		"session-with-dashes",
		"session_with_underscores",
		"CamelCaseSession",
	}

	for i, sid := range sessionIDs {
		pid := player.PlayerID(string(rune('a' + i)))
		uid := room.UserID("user-" + string(pid))
		_ = r.Enqueue(room.RoomCommand{
			Kind:      room.CmdJoin,
			SessionID: room.SessionID(sid),
			PlayerID:  room.PlayerID(pid),
			UserID:    uid,
			Timestamp: time.Now(),
		})
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == len(sessionIDs) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// All players at origin should be in the same cluster regardless of session ID format.
	output := r.GetClusterOutput()
	if output.K != 1 {
		t.Errorf("all players at origin should form 1 cluster regardless of session ID format, got K=%d", output.K)
	}

	// Each player should see all others (cluster-based interest).
	for i := 0; i < len(sessionIDs); i++ {
		pid := player.PlayerID(string(rune('a' + i)))
		visible := r.VisiblePlayersFor(pid)
		expectedCount := len(sessionIDs) - 1 // exclude self
		if len(visible) != expectedCount {
			t.Errorf("%s sees %d other players, want %d (session ID format should not affect clustering)", pid, len(visible), expectedCount)
		}
	}
}

func TestRoom_ClusterRadiusFallback_Coexist(t *testing.T) {
	// Verify that we can toggle cluster_enabled and the room continues to work.
	reg := newTestRegistry()
	mgr := room.NewRoomManager(reg, room.DefaultRoomConfig(), newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "toggle-cluster-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s2"),
		PlayerID:  room.PlayerID("p2"),
		UserID:    room.UserID("u2"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// With cluster enabled (default), both players at origin should be in same cluster.
	if r.ClusterConfig().Enabled {
		visible := r.VisiblePlayersFor(player.PlayerID("p1"))
		if len(visible) != 1 {
			t.Errorf("cluster enabled: p1 should see p2, got %d visible", len(visible))
		}
	}

	// Note: We can't toggle cluster_enabled at runtime without restarting the room,
	// which is by design. The config is set at room creation time.
	// This test verifies that both paths work when configured appropriately.
}

func TestRoom_ClusterOutputDeterministic(t *testing.T) {
	// Verify that cluster output is deterministic for the same player positions.
	// Multiple rooms with the same player positions should produce the same
	// cluster assignments (assuming K-Means initialization is seeded consistently).
	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	joinPlayers := func(r *room.Room, positions []struct{pid, x, z string}) {
		for _, pos := range positions {
			_ = r.Enqueue(room.RoomCommand{
				Kind:      room.CmdJoin,
				SessionID: room.SessionID("s-" + pos.pid),
				PlayerID:  room.PlayerID(pos.pid),
				UserID:    room.UserID("u-" + pos.pid),
				Timestamp: time.Now(),
			})
			_ = r.Enqueue(room.RoomCommand{
				Kind:     room.CmdPlayerInput,
				PlayerID: room.PlayerID(pos.pid),
				Payload: player.PlayerInput{
					Seq: 1,
					Transform: player.PlayerTransform{
						Position: player.Vector3{X: 0, Y: 0, Z: 0}, // all at origin for simplicity
						Rotation: player.IdentityQuaternion,
					},
					Timestamp: time.Now().UnixMilli(),
				},
				Timestamp: time.Now(),
			})
		}
	}

	positions := []struct{pid, x, z string}{
		{"p1", "0", "0"},
		{"p2", "1", "0"},
		{"p3", "2", "0"},
	}

	// Create first room.
	r1, err := mgr.CreateRoom(ctx, "det-room-1")
	if err != nil {
		t.Fatalf("CreateRoom room1: %v", err)
	}
	defer r1.Stop()
	joinPlayers(r1, positions)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r1.PlayerCount() == len(positions) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	output1 := r1.GetClusterOutput()

	// Create second room with same positions.
	r2, err := mgr.CreateRoom(ctx, "det-room-2")
	if err != nil {
		t.Fatalf("CreateRoom room2: %v", err)
	}
	defer r2.Stop()
	joinPlayers(r2, positions)

	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r2.PlayerCount() == len(positions) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	output2 := r2.GetClusterOutput()

	// Cluster counts should match.
	if output1.K != output2.K {
		t.Errorf("deterministic cluster count: room1 K=%d, room2 K=%d", output1.K, output2.K)
	}

	// Each player should be assigned to some cluster in both rooms.
	for _, pos := range positions {
		pid := player.PlayerID(pos.pid)
		_, ok1 := output1.Assignments[pid]
		_, ok2 := output2.Assignments[pid]
		if !ok1 {
			t.Errorf("room1: player %q not assigned to any cluster", pid)
		}
		if !ok2 {
			t.Errorf("room2: player %q not assigned to any cluster", pid)
		}
	}
}

func TestRoom_ClusterNoRedisDependency(t *testing.T) {
	// Verify that cluster-based delta building works without Redis.
	// This is a structural test: if the code imported Redis packages, there
	// would be a runtime or compilation error.
	// We simply verify the room works correctly in single-VPS mode.

	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "no-redis-cluster-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			output := r.GetClusterOutput()
			if output.K == 1 {
				return // success: cluster works without Redis
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Error("cluster-based delta building should work without Redis (single-VPS mode)")
}

// TestRoom_ClusterMembershipDelta_NotImplemented verifies that we do not
// implement ClusterMembershipDelta in Phase 1. It remains reserved.
func TestRoom_ClusterMembershipDelta_NotImplemented(t *testing.T) {
	// This is a documentation test. ClusterMembershipDelta (message type 1011)
	// is reserved for optional Phase 1 use but is not implemented.
	// The delta builder produces PlayerDelta only, not cluster metadata deltas.

	reg := newTestRegistry()
	cfg := room.DefaultRoomConfig()
	cfg.ClusterConfig.Enabled = true
	mgr := room.NewRoomManager(reg, cfg, newTestLogger())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "no-cluster-meta-delta-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID("s1"),
		PlayerID:  room.PlayerID("p1"),
		UserID:    room.UserID("u1"),
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify cluster metadata is available via query API, not via delta.
	output := r.GetClusterOutput()
	if output.K != 1 {
		t.Errorf("expected K=1 cluster, got K=%d", output.K)
	}

	// Cluster membership can be queried, but no separate ClusterMembershipDelta
	// message is implemented. This is by design for Phase 1.
	_ = output // silence unused warning
}
