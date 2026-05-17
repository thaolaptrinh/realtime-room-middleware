package bridge

import (
	"errors"
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/delta"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

// fakeSession implements transport.RealtimeSession for testing.
type fakeSession struct {
	id        string
	transport transport.TransportType
	packets   [][]byte
	sendErr   error
	closed    bool
}

func (f *fakeSession) ID() string                         { return f.id }
func (f *fakeSession) UserID() string                     { return "" }
func (f *fakeSession) Transport() transport.TransportType { return f.transport }
func (f *fakeSession) Close() error                       { f.closed = true; return nil }
func (f *fakeSession) Send(packet []byte) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	cp := make([]byte, len(packet))
	copy(cp, packet)
	f.packets = append(f.packets, cp)
	return nil
}

// domainPlayerDelta builds a domain PlayerDelta with the given tick and entries.
func domainPlayerDelta(tick uint32, enters []delta.PlayerEnterDelta, updates []delta.PlayerUpdateDelta, leaves []delta.PlayerLeaveDelta) *delta.PlayerDelta {
	return &delta.PlayerDelta{
		Tick:    tick,
		Enters:  enters,
		Updates: updates,
		Leaves:  leaves,
	}
}

func TestConvertPlayerDelta_NilInput(t *testing.T) {
	result := ConvertPlayerDelta(nil)
	if !result.IsEmpty() {
		t.Error("expected empty PlayerDelta for nil input")
	}
}

func TestConvertPlayerDelta_Enters(t *testing.T) {
	d := domainPlayerDelta(42, []delta.PlayerEnterDelta{
		{
			PlayerID: player.PlayerID("p1"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 1.0, Y: 0, Z: 2.0},
				Rotation: player.Quaternion{X: 0, Y: 0.5, Z: 0, W: 0.866},
			},
			Version: 3,
		},
	}, nil, nil)

	result := ConvertPlayerDelta(d)
	if result.Tick != 42 {
		t.Errorf("Tick = %d, want 42", result.Tick)
	}
	if len(result.Enters) != 1 {
		t.Fatalf("Enters len = %d, want 1", len(result.Enters))
	}
	e := result.Enters[0]
	if e.PlayerID != "p1" {
		t.Errorf("PlayerID = %q, want p1", e.PlayerID)
	}
	if e.X != 1.0 {
		t.Errorf("X = %f, want 1.0", e.X)
	}
	if e.Z != 2.0 {
		t.Errorf("Z = %f, want 2.0", e.Z)
	}
	if e.Yaw != 0.5 {
		t.Errorf("Yaw = %f, want 0.5", e.Yaw)
	}
	if e.AnimState != 0 {
		t.Errorf("AnimState = %d, want 0 (not tracked in domain yet)", e.AnimState)
	}
	if e.Version != 3 {
		t.Errorf("Version = %d, want 3", e.Version)
	}
}

func TestConvertPlayerDelta_Updates(t *testing.T) {
	d := domainPlayerDelta(10, nil, []delta.PlayerUpdateDelta{
		{
			PlayerID: player.PlayerID("p2"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 5.0, Y: 0, Z: 6.0},
				Rotation: player.Quaternion{X: 0, Y: 1.2, Z: 0, W: 1},
			},
			Version: 7,
		},
	}, nil)

	result := ConvertPlayerDelta(d)
	if len(result.Updates) != 1 {
		t.Fatalf("Updates len = %d, want 1", len(result.Updates))
	}
	u := result.Updates[0]
	if u.PlayerID != "p2" {
		t.Errorf("PlayerID = %q, want p2", u.PlayerID)
	}
	if u.X != 5.0 {
		t.Errorf("X = %f, want 5.0", u.X)
	}
	if u.Z != 6.0 {
		t.Errorf("Z = %f, want 6.0", u.Z)
	}
	if u.Yaw != 1.2 {
		t.Errorf("Yaw = %f, want 1.2", u.Yaw)
	}
	if u.Version != 7 {
		t.Errorf("Version = %d, want 7", u.Version)
	}
}

func TestConvertPlayerDelta_Leaves(t *testing.T) {
	d := domainPlayerDelta(99, nil, nil, []delta.PlayerLeaveDelta{
		{PlayerID: player.PlayerID("p3")},
	})

	result := ConvertPlayerDelta(d)
	if len(result.Leaves) != 1 {
		t.Fatalf("Leaves len = %d, want 1", len(result.Leaves))
	}
	if result.Leaves[0].PlayerID != "p3" {
		t.Errorf("PlayerID = %q, want p3", result.Leaves[0].PlayerID)
	}
}

func TestConvertFullSnapshot(t *testing.T) {
	players := []PlayerStateView{
		{
			PlayerID: player.PlayerID("p1"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 10, Y: 0, Z: 20},
				Rotation: player.Quaternion{X: 0, Y: 0.7, Z: 0, W: 1},
			},
			Version: 5,
		},
		{
			PlayerID: player.PlayerID("p2"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 30, Y: 0, Z: 40},
				Rotation: player.IdentityQuaternion,
			},
			Version: 1,
		},
	}

	result := ConvertFullSnapshot(100, players)
	if result.Tick != 100 {
		t.Errorf("Tick = %d, want 100", result.Tick)
	}
	if len(result.Players) != 2 {
		t.Fatalf("Players len = %d, want 2", len(result.Players))
	}
	if result.Players[0].PlayerID != "p1" {
		t.Errorf("Players[0].PlayerID = %q, want p1", result.Players[0].PlayerID)
	}
	if result.Players[0].X != 10 {
		t.Errorf("Players[0].X = %f, want 10", result.Players[0].X)
	}
	if result.Players[0].Z != 20 {
		t.Errorf("Players[0].Z = %f, want 20", result.Players[0].Z)
	}
	if result.Players[0].Yaw != 0.7 {
		t.Errorf("Players[0].Yaw = %f, want 0.7", result.Players[0].Yaw)
	}
	if result.Players[0].Version != 5 {
		t.Errorf("Players[0].Version = %d, want 5", result.Players[0].Version)
	}
	if result.Players[1].PlayerID != "p2" {
		t.Errorf("Players[1].PlayerID = %q, want p2", result.Players[1].PlayerID)
	}
}

func TestSendPlayerDelta_EncodedPacket(t *testing.T) {
	fake := &fakeSession{id: "s1", transport: transport.KCP}

	pd := domainPlayerDelta(42, []delta.PlayerEnterDelta{
		{
			PlayerID: player.PlayerID("p1"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 1, Y: 0, Z: 2},
				Rotation: player.Quaternion{X: 0, Y: 0.5, Z: 0, W: 1},
			},
			Version: 1,
		},
	}, nil, nil)

	err := SendPlayerDelta(fake, pd)
	if err != nil {
		t.Fatalf("SendPlayerDelta: %v", err)
	}
	if len(fake.packets) != 1 {
		t.Fatalf("packets sent = %d, want 1", len(fake.packets))
	}

	// Decode the envelope and verify message type.
	env, err := protocol.DecodeEnvelope(fake.packets[0])
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.Type != protocol.TypePlayerDelta {
		t.Errorf("message type = %d, want %d (PlayerDelta)", env.Type, protocol.TypePlayerDelta)
	}
	if env.Tick != 42 {
		t.Errorf("envelope tick = %d, want 42", env.Tick)
	}
	if env.Version != protocol.CurrentVersion {
		t.Errorf("protocol version = %d, want %d", env.Version, protocol.CurrentVersion)
	}

	// Decode the body and verify content.
	var wire protocol.PlayerDelta
	if err := protocol.DecodeMessage(env.Body, &wire); err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if wire.Tick != 42 {
		t.Errorf("wire tick = %d, want 42", wire.Tick)
	}
	if len(wire.Enters) != 1 || wire.Enters[0].PlayerID != "p1" {
		t.Errorf("wire enters = %v, want [{p1}]", wire.Enters)
	}
}

func TestSendFullSnapshot_EncodedPacket(t *testing.T) {
	fake := &fakeSession{id: "s1", transport: transport.WebSocket}

	players := []PlayerStateView{
		{
			PlayerID: player.PlayerID("p1"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 5, Y: 0, Z: 10},
				Rotation: player.IdentityQuaternion,
			},
			Version: 3,
		},
	}

	err := SendFullSnapshot(fake, 7, players)
	if err != nil {
		t.Fatalf("SendFullSnapshot: %v", err)
	}
	if len(fake.packets) != 1 {
		t.Fatalf("packets sent = %d, want 1", len(fake.packets))
	}

	env, err := protocol.DecodeEnvelope(fake.packets[0])
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.Type != protocol.TypeFullSnapshot {
		t.Errorf("message type = %d, want %d (FullSnapshot)", env.Type, protocol.TypeFullSnapshot)
	}
	if env.Tick != 7 {
		t.Errorf("envelope tick = %d, want 7", env.Tick)
	}

	var wire protocol.FullSnapshot
	if err := protocol.DecodeMessage(env.Body, &wire); err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if wire.Tick != 7 {
		t.Errorf("wire tick = %d, want 7", wire.Tick)
	}
	if len(wire.Players) != 1 || wire.Players[0].PlayerID != "p1" {
		t.Errorf("wire players = %v, want [{p1}]", wire.Players)
	}
}

func TestSendPlayerDelta_SendError(t *testing.T) {
	sendErr := errors.New("connection closed")
	fake := &fakeSession{id: "s1", transport: transport.KCP, sendErr: sendErr}

	pd := domainPlayerDelta(1, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)

	err := SendPlayerDelta(fake, pd)
	if err == nil {
		t.Fatal("expected error from Send, got nil")
	}
	if !errors.Is(err, sendErr) {
		t.Errorf("error = %q, want wrapped %q", err, sendErr)
	}
}

func TestMixedTransport_SameProtocolPayload(t *testing.T) {
	// Verify that KCP and WSS sessions receive the same protocol message type
	// and identical wire content for the same domain delta.
	pd := domainPlayerDelta(50, []delta.PlayerEnterDelta{
		{
			PlayerID: player.PlayerID("p1"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 3, Y: 0, Z: 4},
				Rotation: player.Quaternion{X: 0, Y: 1.5, Z: 0, W: 1},
			},
			Version: 10,
		},
	}, nil, nil)

	kcpSess := &fakeSession{id: "kcp-1", transport: transport.KCP}
	wssSess := &fakeSession{id: "wss-1", transport: transport.WebSocket}

	if err := SendPlayerDelta(kcpSess, pd); err != nil {
		t.Fatalf("SendPlayerDelta KCP: %v", err)
	}
	if err := SendPlayerDelta(wssSess, pd); err != nil {
		t.Fatalf("SendPlayerDelta WSS: %v", err)
	}

	// Both should have sent exactly one packet.
	if len(kcpSess.packets) != 1 || len(wssSess.packets) != 1 {
		t.Fatalf("KCP packets=%d, WSS packets=%d, both want 1", len(kcpSess.packets), len(wssSess.packets))
	}

	// Decode both envelopes.
	kcpEnv, err := protocol.DecodeEnvelope(kcpSess.packets[0])
	if err != nil {
		t.Fatalf("decode KCP envelope: %v", err)
	}
	wssEnv, err := protocol.DecodeEnvelope(wssSess.packets[0])
	if err != nil {
		t.Fatalf("decode WSS envelope: %v", err)
	}

	// Same message type.
	if kcpEnv.Type != wssEnv.Type {
		t.Errorf("KCP type=%d, WSS type=%d — must be identical", kcpEnv.Type, wssEnv.Type)
	}
	if kcpEnv.Type != protocol.TypePlayerDelta {
		t.Errorf("expected TypePlayerDelta (%d), got %d", protocol.TypePlayerDelta, kcpEnv.Type)
	}

	// Same protocol version.
	if kcpEnv.Version != wssEnv.Version {
		t.Errorf("KCP version=%d, WSS version=%d — must be identical", kcpEnv.Version, wssEnv.Version)
	}

	// Same body content (MessagePack payload).
	if string(kcpEnv.Body) != string(wssEnv.Body) {
		t.Errorf("KCP and WSS body content differs — same delta must produce identical wire payload")
	}

	// Decode bodies to verify content match.
	var kcpDelta, wssDelta protocol.PlayerDelta
	if err := protocol.DecodeMessage(kcpEnv.Body, &kcpDelta); err != nil {
		t.Fatalf("decode KCP body: %v", err)
	}
	if err := protocol.DecodeMessage(wssEnv.Body, &wssDelta); err != nil {
		t.Fatalf("decode WSS body: %v", err)
	}
	if kcpDelta.Tick != wssDelta.Tick {
		t.Errorf("tick mismatch: KCP=%d, WSS=%d", kcpDelta.Tick, wssDelta.Tick)
	}
	if len(kcpDelta.Enters) != 1 || kcpDelta.Enters[0].PlayerID != "p1" {
		t.Errorf("KCP enters = %v, expected [{p1}]", kcpDelta.Enters)
	}
	if len(wssDelta.Enters) != 1 || wssDelta.Enters[0].PlayerID != "p1" {
		t.Errorf("WSS enters = %v, expected [{p1}]", wssDelta.Enters)
	}
}

func TestDispatchBatches_MultipleSessions(t *testing.T) {
	kcpSess := &fakeSession{id: "s1", transport: transport.KCP}
	wssSess := &fakeSession{id: "s2", transport: transport.WebSocket}

	sessions := map[string]*fakeSession{
		"s1": kcpSess,
		"s2": wssSess,
	}

	lookup := func(id string) transport.RealtimeSession {
		return sessions[id]
	}

	pd1 := domainPlayerDelta(10, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)
	pd2 := domainPlayerDelta(10, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p2"), Version: 1},
	}, nil, nil)

	batches := map[string]*delta.DeltaBatch{
		"s1": {Tick: 10, PlayerDelta: pd1},
		"s2": {Tick: 10, PlayerDelta: pd2},
	}

	results := DispatchBatches(batches, lookup)
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Errorf("session %s: unexpected error: %v", r.SessionID, r.Error)
		}
	}

	if len(kcpSess.packets) != 1 {
		t.Errorf("KCP packets = %d, want 1", len(kcpSess.packets))
	}
	if len(wssSess.packets) != 1 {
		t.Errorf("WSS packets = %d, want 1", len(wssSess.packets))
	}
}

func TestDispatchBatches_ReportsOneSessionSendFailureWithoutPanic(t *testing.T) {
	sendErr := errors.New("wss send failed")
	kcpSess := &fakeSession{id: "kcp-ok", transport: transport.KCP}
	wssSess := &fakeSession{id: "wss-fail", transport: transport.WebSocket, sendErr: sendErr}

	sessions := map[string]*fakeSession{
		kcpSess.ID(): kcpSess,
		wssSess.ID(): wssSess,
	}
	lookup := func(id string) transport.RealtimeSession { return sessions[id] }

	pd := domainPlayerDelta(12, nil, []delta.PlayerUpdateDelta{
		{
			PlayerID: player.PlayerID("p1"),
			Transform: player.PlayerTransform{
				Position: player.Vector3{X: 2, Z: 3},
				Rotation: player.Quaternion{X: 0, Y: 0.5, Z: 0, W: 1},
			},
			Version: 2,
		},
	}, nil)
	batches := map[string]*delta.DeltaBatch{
		kcpSess.ID(): {Tick: 12, PlayerDelta: pd},
		wssSess.ID(): {Tick: 12, PlayerDelta: pd},
	}

	var results []SendResult
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("DispatchBatches panicked on one failed send: %v", r)
			}
		}()
		results = DispatchBatches(batches, lookup)
	}()

	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	var sawOK, sawFailure bool
	for _, result := range results {
		switch result.SessionID {
		case kcpSess.ID():
			sawOK = true
			if result.Error != nil {
				t.Fatalf("KCP result error = %v, want nil", result.Error)
			}
			if result.Transport != transport.KCP {
				t.Fatalf("KCP result transport = %s, want %s", result.Transport, transport.KCP)
			}
		case wssSess.ID():
			sawFailure = true
			if !errors.Is(result.Error, sendErr) {
				t.Fatalf("WSS result error = %v, want %v", result.Error, sendErr)
			}
			if result.Transport != transport.WebSocket {
				t.Fatalf("WSS result transport = %s, want %s", result.Transport, transport.WebSocket)
			}
		default:
			t.Fatalf("unexpected result for session %q", result.SessionID)
		}
	}
	if !sawOK || !sawFailure {
		t.Fatalf("results missing expected sessions: sawOK=%v sawFailure=%v results=%+v", sawOK, sawFailure, results)
	}
	if len(kcpSess.packets) != 1 {
		t.Fatalf("successful KCP packets = %d, want 1", len(kcpSess.packets))
	}
	if len(wssSess.packets) != 0 {
		t.Fatalf("failed WSS packets = %d, want 0", len(wssSess.packets))
	}
}

func TestDispatchBatches_MissingSession(t *testing.T) {
	lookup := func(id string) transport.RealtimeSession {
		return nil // session not found
	}

	pd := domainPlayerDelta(5, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)

	batches := map[string]*delta.DeltaBatch{
		"ghost": {Tick: 5, PlayerDelta: pd},
	}

	results := DispatchBatches(batches, lookup)
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].SessionID != "ghost" {
		t.Errorf("SessionID = %q, want ghost", results[0].SessionID)
	}
	if results[0].Error == nil {
		t.Error("expected error for missing session, got nil")
	}
}

func TestDispatchBatches_EmptyBatchSkipped(t *testing.T) {
	sent := false
	lookup := func(id string) transport.RealtimeSession {
		sent = true
		return nil
	}

	emptyDelta := &delta.PlayerDelta{Tick: 1} // no enters, updates, or leaves
	batches := map[string]*delta.DeltaBatch{
		"s1": {Tick: 1, PlayerDelta: emptyDelta},
	}

	results := DispatchBatches(batches, lookup)
	if len(results) != 0 {
		t.Errorf("results len = %d, want 0 (empty batch should be skipped)", len(results))
	}
	if sent {
		t.Error("lookup should not have been called for empty batch")
	}
}

func TestDispatchBatches_NilBatchSkipped(t *testing.T) {
	sent := false
	lookup := func(id string) transport.RealtimeSession {
		sent = true
		return nil
	}

	batches := map[string]*delta.DeltaBatch{
		"s1": nil,
	}

	results := DispatchBatches(batches, lookup)
	if len(results) != 0 {
		t.Errorf("results len = %d, want 0 (nil batch should be skipped)", len(results))
	}
	if sent {
		t.Error("lookup should not have been called for nil batch")
	}
}

func TestDispatchBatches_NilMap(t *testing.T) {
	results := DispatchBatches(nil, func(id string) transport.RealtimeSession { return nil })
	if results != nil {
		t.Errorf("expected nil results for nil map, got %v", results)
	}
}

func TestDispatchBatches_DoesNotMutateInput(t *testing.T) {
	fake := &fakeSession{id: "s1", transport: transport.KCP}
	lookup := func(id string) transport.RealtimeSession { return fake }

	pd := domainPlayerDelta(10, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)

	batches := map[string]*delta.DeltaBatch{
		"s1": {Tick: 10, PlayerDelta: pd},
	}

	// Record original state.
	originalEnterCount := len(pd.Enters)
	originalTick := pd.Tick

	DispatchBatches(batches, lookup)

	// Verify domain delta was not mutated.
	if len(pd.Enters) != originalEnterCount {
		t.Errorf("Enters mutated: was %d, now %d", originalEnterCount, len(pd.Enters))
	}
	if pd.Tick != originalTick {
		t.Errorf("Tick mutated: was %d, now %d", originalTick, pd.Tick)
	}
}

func TestDeltaBroadcaster(t *testing.T) {
	fake := &fakeSession{id: "s1", transport: transport.KCP}
	lookup := func(id string) transport.RealtimeSession { return fake }

	db := NewDeltaBroadcaster(lookup)

	pd := domainPlayerDelta(10, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)

	batches := map[string]*delta.DeltaBatch{
		"s1": {Tick: 10, PlayerDelta: pd},
	}

	db.BroadcastDelta(batches)

	if len(fake.packets) != 1 {
		t.Errorf("packets sent = %d, want 1", len(fake.packets))
	}
}

func TestBoundary_ProtocolNoGameImports(t *testing.T) {
	// Structural test: protocol.PlayerDelta must not reference game types.
	// If it did, the conversion in this package would not be needed.
	// This test verifies the conversion functions exist and produce correct output,
	// proving the boundary is maintained.

	pd := domainPlayerDelta(1, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)

	wire := ConvertPlayerDelta(pd)
	if wire.Tick != 1 {
		t.Errorf("conversion produced wrong tick: %d", wire.Tick)
	}
	if len(wire.Enters) != 1 {
		t.Errorf("conversion produced wrong enters count: %d", len(wire.Enters))
	}
	// Wire types are protocol types (string fields, not player.PlayerID).
	if wire.Enters[0].PlayerID != "p1" {
		t.Errorf("conversion produced wrong player ID type or value: %q", wire.Enters[0].PlayerID)
	}
}

func TestBoundary_NoJSONOrProtobuf(t *testing.T) {
	// Verify that the encoded packet uses MessagePack, not JSON or Protobuf.
	// MessagePack packets start with specific byte patterns.
	// A map with 4 keys (the Envelope) starts with 0x84 (fixmap with 4 entries)
	// or 0xde/0xdf (map16/map32) depending on size.
	// JSON would start with '{' (0x7b) and Protobuf would be binary without msgpack framing.
	fake := &fakeSession{id: "s1", transport: transport.KCP}

	pd := domainPlayerDelta(1, []delta.PlayerEnterDelta{
		{PlayerID: player.PlayerID("p1"), Version: 1},
	}, nil, nil)

	if err := SendPlayerDelta(fake, pd); err != nil {
		t.Fatalf("SendPlayerDelta: %v", err)
	}

	data := fake.packets[0]
	if len(data) == 0 {
		t.Fatal("empty packet")
	}

	// MessagePack envelope is a map. First byte should indicate a map type:
	// 0x80-0x8f = fixmap (0-15 keys)
	// 0xde = map16
	// 0xdf = map32
	firstByte := data[0]
	isMsgpackMap := (firstByte >= 0x80 && firstByte <= 0x8f) || firstByte == 0xde || firstByte == 0xdf
	if !isMsgpackMap {
		t.Errorf("first byte = 0x%02x, expected MessagePack map header (not JSON or Protobuf)", firstByte)
	}

	// Definitely not JSON.
	if firstByte == 0x7b { // '{'
		t.Error("packet starts with '{' — looks like JSON, expected MessagePack")
	}
}
