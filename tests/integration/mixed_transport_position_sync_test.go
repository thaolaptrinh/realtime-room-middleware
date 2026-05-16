package integration

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/bridge"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

type fakeRealtimeSession struct {
	id        string
	transport transport.TransportType
	packets   [][]byte
	sendErr   error
	closed    bool
}

func (f *fakeRealtimeSession) ID() string                         { return f.id }
func (f *fakeRealtimeSession) UserID() string                     { return "" }
func (f *fakeRealtimeSession) Transport() transport.TransportType { return f.transport }
func (f *fakeRealtimeSession) Close() error                       { f.closed = true; return nil }
func (f *fakeRealtimeSession) Send(packet []byte) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	cp := make([]byte, len(packet))
	copy(cp, packet)
	f.packets = append(f.packets, cp)
	return nil
}

func waitFor(f func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestMixedTransport_KCPAndWSSInSameRoom(t *testing.T) {
	reg := room.NewInMemoryRoomRegistry()
	mgr := room.NewRoomManager(reg, room.DefaultRoomConfig(), slog.Default())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "mixed-transport-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	kcpSession := &fakeRealtimeSession{id: "kcp-session-1", transport: transport.KCP}
	wssSession := &fakeRealtimeSession{id: "wss-session-1", transport: transport.WebSocket}

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(kcpSession.ID()),
		PlayerID:  "player-kcp",
		UserID:    "user-kcp",
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(wssSession.ID()),
		PlayerID:  "player-wss",
		UserID:    "user-wss",
		Timestamp: time.Now(),
	})

	if !waitFor(func() bool { return r.PlayerCount() == 2 }, 500*time.Millisecond) {
		t.Fatalf("PlayerCount = %d, want 2", r.PlayerCount())
	}

	_ = r.Enqueue(room.RoomCommand{
		Kind:     room.CmdPlayerInput,
		PlayerID: "player-kcp",
		Payload: player.PlayerInput{
			Seq: 1,
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 10, Y: 0, Z: 20},
				Rotation: player.Quaternion{X: 0, Y: 0.5, Z: 0, W: 1},
			},
			Timestamp: time.Now().UnixMilli(),
		},
		Timestamp: time.Now(),
	})

	_ = r.Enqueue(room.RoomCommand{
		Kind:     room.CmdPlayerInput,
		PlayerID: "player-wss",
		Payload: player.PlayerInput{
			Seq: 1,
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 30, Y: 0, Z: 40},
				Rotation: player.Quaternion{X: 0, Y: 1.0, Z: 0, W: 1},
			},
			Timestamp: time.Now().UnixMilli(),
		},
		Timestamp: time.Now(),
	})

	if !waitFor(func() bool {
		_, v, _ := r.GetPlayerState("player-kcp")
		_, v2, _ := r.GetPlayerState("player-wss")
		return v > 0 && v2 > 0
	}, 500*time.Millisecond) {
		t.Error("player transforms not updated")
	}

	kcpPlayerView := bridge.PlayerStateView{
		PlayerID: "player-kcp",
		Transform: player.PlayerTransform{
			Position: player.Vector3{X: 10, Y: 0, Z: 20},
			Rotation: player.Quaternion{X: 0, Y: 0.5, Z: 0, W: 1},
		},
		Version: 1,
	}
	wssPlayerView := bridge.PlayerStateView{
		PlayerID: "player-wss",
		Transform: player.PlayerTransform{
			Position: player.Vector3{X: 30, Y: 0, Z: 40},
			Rotation: player.Quaternion{X: 0, Y: 1.0, Z: 0, W: 1},
		},
		Version: 1,
	}

	err = bridge.SendFullSnapshot(kcpSession, r.CurrentTick(), []bridge.PlayerStateView{kcpPlayerView, wssPlayerView})
	if err != nil {
		t.Fatalf("SendFullSnapshot to KCP: %v", err)
	}
	err = bridge.SendFullSnapshot(wssSession, r.CurrentTick(), []bridge.PlayerStateView{kcpPlayerView, wssPlayerView})
	if err != nil {
		t.Fatalf("SendFullSnapshot to WSS: %v", err)
	}

	if len(kcpSession.packets) != 1 {
		t.Fatalf("KCP packets = %d, want 1", len(kcpSession.packets))
	}
	if len(wssSession.packets) != 1 {
		t.Fatalf("WSS packets = %d, want 1", len(wssSession.packets))
	}

	kcpEnv, err := protocol.DecodeEnvelope(kcpSession.packets[0])
	if err != nil {
		t.Fatalf("DecodeEnvelope KCP: %v", err)
	}
	wssEnv, err := protocol.DecodeEnvelope(wssSession.packets[0])
	if err != nil {
		t.Fatalf("DecodeEnvelope WSS: %v", err)
	}

	if kcpEnv.Type != wssEnv.Type {
		t.Errorf("KCP message type = %d, WSS = %d — must be identical", kcpEnv.Type, wssEnv.Type)
	}
	if kcpEnv.Type != protocol.TypeFullSnapshot {
		t.Errorf("expected TypeFullSnapshot (%d), got %d", protocol.TypeFullSnapshot, kcpEnv.Type)
	}

	if kcpEnv.Version != wssEnv.Version {
		t.Errorf("KCP version = %d, WSS = %d — must be identical", kcpEnv.Version, wssEnv.Version)
	}

	if string(kcpEnv.Body) != string(wssEnv.Body) {
		t.Error("KCP and WSS body content differs — same snapshot must produce identical wire payload")
	}

	var kcpSnap, wssSnap protocol.FullSnapshot
	if err := protocol.DecodeMessage(kcpEnv.Body, &kcpSnap); err != nil {
		t.Fatalf("decode KCP FullSnapshot: %v", err)
	}
	if err := protocol.DecodeMessage(wssEnv.Body, &wssSnap); err != nil {
		t.Fatalf("decode WSS FullSnapshot: %v", err)
	}

	if kcpSnap.Tick != wssSnap.Tick {
		t.Errorf("tick mismatch: KCP=%d, WSS=%d", kcpSnap.Tick, wssSnap.Tick)
	}
	if len(kcpSnap.Players) != 2 || len(wssSnap.Players) != 2 {
		t.Fatalf("Players count: KCP=%d, WSS=%d, want 2", len(kcpSnap.Players), len(wssSnap.Players))
	}
}

func TestMixedTransport_PlayerDeltaBothReceive(t *testing.T) {
	reg := room.NewInMemoryRoomRegistry()
	mgr := room.NewRoomManager(reg, room.DefaultRoomConfig(), slog.Default())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "delta-both-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	kcpSession := &fakeRealtimeSession{id: "kcp-delta-1", transport: transport.KCP}
	wssSession := &fakeRealtimeSession{id: "wss-delta-1", transport: transport.WebSocket}

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(kcpSession.ID()),
		PlayerID:  "p1",
		UserID:    "u1",
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(wssSession.ID()),
		PlayerID:  "p2",
		UserID:    "u2",
		Timestamp: time.Now(),
	})

	if !waitFor(func() bool { return r.PlayerCount() == 2 }, 500*time.Millisecond) {
		t.Fatalf("PlayerCount = %d, want 2", r.PlayerCount())
	}

	sessions := map[string]*fakeRealtimeSession{
		kcpSession.ID(): kcpSession,
		wssSession.ID(): wssSession,
	}
	lookup := func(id string) transport.RealtimeSession { return sessions[id] }

	kcpDelta := &delta.PlayerDelta{
		Tick: r.CurrentTick(),
		Enters: []delta.PlayerEnterDelta{
			{
				PlayerID: player.PlayerID("new-p1"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 5, Y: 0, Z: 10},
					Rotation: player.Quaternion{X: 0, Y: 0.7, Z: 0, W: 1},
				},
				Version: 1,
			},
		},
		Updates: []delta.PlayerUpdateDelta{
			{
				PlayerID: player.PlayerID("p1"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 15, Y: 0, Z: 25},
					Rotation: player.IdentityQuaternion,
				},
				Version: 3,
			},
		},
		Leaves: []delta.PlayerLeaveDelta{},
	}

	wssDelta := &delta.PlayerDelta{
		Tick: r.CurrentTick(),
		Enters: []delta.PlayerEnterDelta{
			{
				PlayerID: player.PlayerID("new-p2"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 8, Y: 0, Z: 12},
					Rotation: player.Quaternion{X: 0, Y: 0.9, Z: 0, W: 1},
				},
				Version: 1,
			},
		},
		Updates: []delta.PlayerUpdateDelta{
			{
				PlayerID: player.PlayerID("p2"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 20, Y: 0, Z: 35},
					Rotation: player.Quaternion{X: 0, Y: 1.2, Z: 0, W: 1},
				},
				Version: 5,
			},
		},
		Leaves: []delta.PlayerLeaveDelta{},
	}

	if err := bridge.SendPlayerDelta(kcpSession, kcpDelta); err != nil {
		t.Fatalf("SendPlayerDelta to KCP: %v", err)
	}
	if err := bridge.SendPlayerDelta(wssSession, wssDelta); err != nil {
		t.Fatalf("SendPlayerDelta to WSS: %v", err)
	}

	kcpEnv, err := protocol.DecodeEnvelope(kcpSession.packets[0])
	if err != nil {
		t.Fatalf("Decode KCP: %v", err)
	}
	wssEnv, err := protocol.DecodeEnvelope(wssSession.packets[0])
	if err != nil {
		t.Fatalf("Decode WSS: %v", err)
	}

	if kcpEnv.Type != protocol.TypePlayerDelta {
		t.Errorf("KCP type = %d, want %d", kcpEnv.Type, protocol.TypePlayerDelta)
	}
	if wssEnv.Type != protocol.TypePlayerDelta {
		t.Errorf("WSS type = %d, want %d", wssEnv.Type, protocol.TypePlayerDelta)
	}

	if kcpEnv.Type != wssEnv.Type {
		t.Error("both sessions should receive same message type")
	}

	firstByteKCP := kcpSession.packets[0][0]
	firstByteWSS := wssSession.packets[0][0]
	isMsgpackKCP := (firstByteKCP >= 0x80 && firstByteKCP <= 0x8f) || firstByteKCP == 0xde || firstByteKCP == 0xdf
	isMsgpackWSS := (firstByteWSS >= 0x80 && firstByteWSS <= 0x8f) || firstByteWSS == 0xde || firstByteWSS == 0xdf
	if !isMsgpackKCP {
		t.Errorf("KCP packet does not start with MessagePack map header: 0x%02x", firstByteKCP)
	}
	if !isMsgpackWSS {
		t.Errorf("WSS packet does not start with MessagePack map header: 0x%02x", firstByteWSS)
	}

	if firstByteKCP == 0x7b {
		t.Error("KCP packet looks like JSON")
	}
	if firstByteWSS == 0x7b {
		t.Error("WSS packet looks like JSON")
	}

	_ = lookup
}

func TestMixedTransport_SendErrorDoesNotAffectOther(t *testing.T) {
	reg := room.NewInMemoryRoomRegistry()
	mgr := room.NewRoomManager(reg, room.DefaultRoomConfig(), slog.Default())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "error-isolation-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	sendErr := errors.New("connection closed")
	kcpSession := &fakeRealtimeSession{id: "kcp-fail-1", transport: transport.KCP, sendErr: sendErr}
	wssSession := &fakeRealtimeSession{id: "wss-ok-1", transport: transport.WebSocket}

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(kcpSession.ID()),
		PlayerID:  "p1",
		UserID:    "u1",
		Timestamp: time.Now(),
	})
	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(wssSession.ID()),
		PlayerID:  "p2",
		UserID:    "u2",
		Timestamp: time.Now(),
	})

	if !waitFor(func() bool { return r.PlayerCount() == 2 }, 500*time.Millisecond) {
		t.Fatalf("PlayerCount = %d, want 2", r.PlayerCount())
	}

	sessions := map[string]*fakeRealtimeSession{
		kcpSession.ID(): kcpSession,
		wssSession.ID(): wssSession,
	}
	lookup := func(id string) transport.RealtimeSession { return sessions[id] }

	pd := &delta.PlayerDelta{
		Tick: r.CurrentTick(),
		Enters: []delta.PlayerEnterDelta{
			{
				PlayerID: player.PlayerID("p1"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 1, Y: 0, Z: 2},
					Rotation: player.IdentityQuaternion,
				},
				Version: 1,
			},
		},
	}

	results := bridge.DispatchBatches(map[string]*delta.DeltaBatch{
		kcpSession.ID(): {Tick: pd.Tick, PlayerDelta: pd},
		wssSession.ID(): {Tick: pd.Tick, PlayerDelta: pd},
	}, lookup)

	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}

	var kcpResult, wssResult *bridge.SendResult
	for i := range results {
		if results[i].SessionID == kcpSession.ID() {
			kcpResult = &results[i]
		}
		if results[i].SessionID == wssSession.ID() {
			wssResult = &results[i]
		}
	}

	if kcpResult == nil || kcpResult.Error == nil {
		t.Error("KCP session should report send error")
	}
	if wssResult == nil {
		t.Error("WSS result should exist")
	}
	if wssResult.Error != nil {
		t.Errorf("WSS session should succeed, got: %v", wssResult.Error)
	}

	if len(wssSession.packets) != 1 {
		t.Errorf("WSS packets = %d, want 1 (should still receive even if KCP fails)", len(wssSession.packets))
	}
}

func TestMixedTransport_ProtocolV1MessagePack(t *testing.T) {
	kcpSession := &fakeRealtimeSession{id: "kcp-verify-1", transport: transport.KCP}
	wssSession := &fakeRealtimeSession{id: "wss-verify-1", transport: transport.WebSocket}

	pd := &delta.PlayerDelta{
		Tick: 100,
		Enters: []delta.PlayerEnterDelta{
			{
				PlayerID: player.PlayerID("verify-p1"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 100, Y: 0, Z: 200},
					Rotation: player.Quaternion{X: 0, Y: 1.5, Z: 0, W: 1},
				},
				Version: 42,
			},
		},
		Updates: []delta.PlayerUpdateDelta{
			{
				PlayerID: player.PlayerID("verify-p2"),
				Transform: player.PlayerTransform{
					Position: player.Vector3{X: 150, Y: 0, Z: 250},
					Rotation: player.Quaternion{X: 0, Y: 2.0, Z: 0, W: 1},
				},
				Version: 99,
			},
		},
		Leaves: []delta.PlayerLeaveDelta{
			{PlayerID: player.PlayerID("verify-p3")},
		},
	}

	if err := bridge.SendPlayerDelta(kcpSession, pd); err != nil {
		t.Fatalf("SendPlayerDelta to KCP: %v", err)
	}
	if err := bridge.SendPlayerDelta(wssSession, pd); err != nil {
		t.Fatalf("SendPlayerDelta to WSS: %v", err)
	}

	kcpEnv, err := protocol.DecodeEnvelope(kcpSession.packets[0])
	if err != nil {
		t.Fatalf("decode KCP envelope: %v", err)
	}
	wssEnv, err := protocol.DecodeEnvelope(wssSession.packets[0])
	if err != nil {
		t.Fatalf("decode WSS envelope: %v", err)
	}

	if kcpEnv.Version != protocol.CurrentVersion {
		t.Errorf("KCP version = %d, want %d (CurrentVersion)", kcpEnv.Version, protocol.CurrentVersion)
	}
	if wssEnv.Version != protocol.CurrentVersion {
		t.Errorf("WSS version = %d, want %d (CurrentVersion)", wssEnv.Version, protocol.CurrentVersion)
	}

	if kcpEnv.Type != wssEnv.Type {
		t.Errorf("KCP type = %s, WSS type = %s — must be identical", kcpEnv.Type, wssEnv.Type)
	}
	if kcpEnv.Type != protocol.TypePlayerDelta {
		t.Errorf("expected TypePlayerDelta, got %s", kcpEnv.Type)
	}

	if string(kcpEnv.Body) != string(wssEnv.Body) {
		t.Error("body content differs — same delta must produce identical wire payload")
	}

	var kcpDelta, wssDelta protocol.PlayerDelta
	if err := protocol.DecodeMessage(kcpEnv.Body, &kcpDelta); err != nil {
		t.Fatalf("decode KCP body: %v", err)
	}
	if err := protocol.DecodeMessage(wssEnv.Body, &wssDelta); err != nil {
		t.Fatalf("decode WSS body: %v", err)
	}

	if kcpDelta.Tick != 100 || wssDelta.Tick != 100 {
		t.Errorf("tick mismatch: KCP=%d, WSS=%d, want 100", kcpDelta.Tick, wssDelta.Tick)
	}
	if len(kcpDelta.Enters) != 1 || kcpDelta.Enters[0].PlayerID != "verify-p1" {
		t.Errorf("KCP enters = %v", kcpDelta.Enters)
	}
	if len(wssDelta.Enters) != 1 || wssDelta.Enters[0].PlayerID != "verify-p1" {
		t.Errorf("WSS enters = %v", wssDelta.Enters)
	}
	if len(kcpDelta.Updates) != 1 || kcpDelta.Updates[0].PlayerID != "verify-p2" {
		t.Errorf("KCP updates = %v", kcpDelta.Updates)
	}
	if len(wssDelta.Updates) != 1 || wssDelta.Updates[0].PlayerID != "verify-p2" {
		t.Errorf("WSS updates = %v", wssDelta.Updates)
	}
	if len(kcpDelta.Leaves) != 1 || kcpDelta.Leaves[0].PlayerID != "verify-p3" {
		t.Errorf("KCP leaves = %v", kcpDelta.Leaves)
	}
	if len(wssDelta.Leaves) != 1 || wssDelta.Leaves[0].PlayerID != "verify-p3" {
		t.Errorf("WSS leaves = %v", wssDelta.Leaves)
	}
}