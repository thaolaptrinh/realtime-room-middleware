package realtime

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/bridge"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/handler"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
	"github.com/vmihailenco/msgpack/v5"
)

type roomTickDispatchHarness struct {
	room      *room.Room
	enqueuer  *recordingRoomEnqueuer
	processor *PacketProcessor
	adapter   *ReceiveLoopAdapter
	kcp       *e2eSession
	wss       *e2eSession
}

type e2eSession struct {
	mu        sync.Mutex
	id        string
	transport transport.TransportType
	packets   [][]byte
}

func newE2EKCPSession(id string) *e2eSession {
	return &e2eSession{id: id, transport: transport.KCP}
}

func newE2EWSSSession(id string) *e2eSession {
	return &e2eSession{id: id, transport: transport.WebSocket}
}

func (s *e2eSession) ID() string                         { return s.id }
func (s *e2eSession) UserID() string                     { return "" }
func (s *e2eSession) Transport() transport.TransportType { return s.transport }
func (s *e2eSession) Close() error                       { return nil }
func (s *e2eSession) Send(packet []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(packet))
	copy(cp, packet)
	s.packets = append(s.packets, cp)
	return nil
}

func (s *e2eSession) Packets() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.packets))
	copy(out, s.packets)
	return out
}

func (s *e2eSession) PacketCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.packets)
}

func (s *e2eSession) ClearPackets() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets = nil
}

var _ transport.RealtimeSession = (*e2eSession)(nil)

type recordingRoomEnqueuer struct {
	mu       sync.Mutex
	room     *room.Room
	commands []room.RoomCommand
}

func (r *recordingRoomEnqueuer) Enqueue(cmd room.RoomCommand) error {
	r.mu.Lock()
	r.commands = append(r.commands, cmd)
	r.mu.Unlock()
	return r.room.Enqueue(cmd)
}

func (r *recordingRoomEnqueuer) Commands() []room.RoomCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]room.RoomCommand, len(r.commands))
	copy(out, r.commands)
	return out
}

func (r *recordingRoomEnqueuer) CommandCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.commands)
}

var _ handler.RoomEnqueuer = (*recordingRoomEnqueuer)(nil)

func newRoomTickDispatchHarness(t *testing.T) *roomTickDispatchHarness {
	t.Helper()

	kcpSession := newE2EKCPSession("kcp-e2e-1")
	wssSession := newE2EWSSSession("wss-e2e-1")
	sessions := map[string]transport.RealtimeSession{
		kcpSession.ID(): kcpSession,
		wssSession.ID(): wssSession,
	}

	dispatcher := bridge.NewDeltaBroadcaster(func(sessionID string) transport.RealtimeSession {
		return sessions[sessionID]
	})
	broadcaster := NewRoomDeltaBroadcaster(dispatcher)

	cfg := room.DefaultRoomConfig()
	cfg.BroadcastRateHz = cfg.TickRateHz
	cfg.ClusterConfig.Enabled = true
	cfg.ClusterConfig.ReclusterIntervalTicks = 1

	mgr := room.NewRoomManager(room.NewInMemoryRoomRegistry(), cfg, slog.Default())
	r, err := mgr.CreateRoomWithBroadcaster(context.Background(), "e2e-dispatch-room", broadcaster)
	if err != nil {
		t.Fatalf("CreateRoomWithBroadcaster: %v", err)
	}
	t.Cleanup(r.Stop)
	enqueuer := &recordingRoomEnqueuer{room: r}

	roomResolver := func(sessionID string) (handler.RoomEnqueuer, error) {
		return enqueuer, nil
	}
	sessionResolver := func(sessionID string) (string, string) {
		switch sessionID {
		case kcpSession.ID():
			return "player-kcp", "user-kcp"
		case wssSession.ID():
			return "player-wss", "user-wss"
		default:
			return "", ""
		}
	}

	h := handler.NewRealtimePacketHandler(roomResolver)
	router := NewSessionPacketRouter(sessionResolver)
	processor := NewPacketProcessor(h, router, slog.Default())
	adapter := NewReceiveLoopAdapter(processor, slog.Default())

	harness := &roomTickDispatchHarness{
		room:      r,
		enqueuer:  enqueuer,
		processor: processor,
		adapter:   adapter,
		kcp:       kcpSession,
		wss:       wssSession,
	}
	harness.joinPlayers(t)
	return harness
}

func (h *roomTickDispatchHarness) joinPlayers(t *testing.T) {
	t.Helper()
	now := time.Now()
	if err := h.room.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(h.kcp.ID()),
		PlayerID:  "player-kcp",
		UserID:    "user-kcp",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("enqueue KCP join: %v", err)
	}
	if err := h.room.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: room.SessionID(h.wss.ID()),
		PlayerID:  "player-wss",
		UserID:    "user-wss",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("enqueue WSS join: %v", err)
	}

	if !waitUntil(500*time.Millisecond, func() bool {
		return h.room.PlayerCount() == 2 && len(h.room.VisiblePlayersFor(player.PlayerID("player-kcp"))) == 1
	}) {
		t.Fatalf("players did not join same visible cluster: count=%d visible=%v", h.room.PlayerCount(), h.room.VisiblePlayersFor(player.PlayerID("player-kcp")))
	}

	if !waitUntil(500*time.Millisecond, func() bool {
		return h.kcp.PacketCount() > 0 && h.wss.PacketCount() > 0
	}) {
		t.Fatalf("initial delta dispatch did not reach both fake sessions: kcp=%d wss=%d", h.kcp.PacketCount(), h.wss.PacketCount())
	}
	h.kcp.ClearPackets()
	h.wss.ClearPackets()
}

func TestRoomTickDispatchE2E_KCPTransformReachesWSSAsPlayerDelta(t *testing.T) {
	h := newRoomTickDispatchHarness(t)

	inbound := protocol.PlayerTransformUpdate{Seq: 10, X: 12.5, Z: -3.25, Yaw: 0.75, AnimState: 2}
	packet, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerTransformUpdate, 10, 0, &inbound)
	if err != nil {
		t.Fatalf("EncodeAndWrap PlayerTransformUpdate: %v", err)
	}

	h.adapter.HandlePacket(h.kcp, packet)
	assertLastEnqueuedTransformCommand(t, h.enqueuer, room.SessionID(h.kcp.ID()), room.PlayerID("player-kcp"))

	if !waitUntil(500*time.Millisecond, func() bool {
		transform, version, ok := h.room.GetPlayerState(player.PlayerID("player-kcp"))
		return ok && version > 0 && transform.Position.X == inbound.X && transform.Position.Z == inbound.Z && transform.Rotation.Y == inbound.Yaw
	}) {
		transform, version, ok := h.room.GetPlayerState(player.PlayerID("player-kcp"))
		t.Fatalf("KCP transform was not processed by room tick: ok=%v version=%d transform=%+v", ok, version, transform)
	}
	if visible := h.room.VisiblePlayersFor(player.PlayerID("player-wss")); !containsPlayer(visible, player.PlayerID("player-kcp")) {
		t.Fatalf("cluster visibility for WSS viewer = %v, want player-kcp visible", visible)
	}

	if !waitUntil(500*time.Millisecond, func() bool { return h.wss.PacketCount() > 0 }) {
		t.Fatalf("WSS fake session did not receive PlayerDelta after KCP transform update")
	}

	env, body := decodeSinglePlayerDeltaPacket(t, h.wss.Packets())
	if err := body.Validate(); err != nil {
		t.Fatalf("decoded PlayerDelta is invalid: %v", err)
	}
	if env.Version != protocol.CurrentVersion {
		t.Fatalf("protocol version = %d, want %d", env.Version, protocol.CurrentVersion)
	}
	if env.Type != protocol.TypePlayerDelta {
		t.Fatalf("message type = %s, want %s", env.Type, protocol.TypePlayerDelta)
	}
	if len(body.Updates) != 1 {
		t.Fatalf("PlayerDelta updates = %d, want 1: %+v", len(body.Updates), body)
	}
	update := body.Updates[0]
	if update.PlayerID != "player-kcp" {
		t.Fatalf("update player = %q, want player-kcp", update.PlayerID)
	}
	if update.X != inbound.X || update.Z != inbound.Z || update.Yaw != inbound.Yaw {
		t.Fatalf("update transform = (%f,%f,%f), want (%f,%f,%f)", update.X, update.Z, update.Yaw, inbound.X, inbound.Z, inbound.Yaw)
	}
}

func TestRoomTickDispatchE2E_WSSUsesSameProtocolSemantics(t *testing.T) {
	h := newRoomTickDispatchHarness(t)

	inbound := protocol.PlayerTransformUpdate{Seq: 11, X: -8, Z: 4, Yaw: 1.25}
	packet, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerTransformUpdate, 11, 0, &inbound)
	if err != nil {
		t.Fatalf("EncodeAndWrap PlayerTransformUpdate: %v", err)
	}

	h.adapter.HandlePacket(h.wss, packet)
	assertLastEnqueuedTransformCommand(t, h.enqueuer, room.SessionID(h.wss.ID()), room.PlayerID("player-wss"))

	if !waitUntil(500*time.Millisecond, func() bool {
		transform, version, ok := h.room.GetPlayerState(player.PlayerID("player-wss"))
		return ok && version > 0 && transform.Position.X == inbound.X && transform.Position.Z == inbound.Z && transform.Rotation.Y == inbound.Yaw
	}) {
		transform, version, ok := h.room.GetPlayerState(player.PlayerID("player-wss"))
		t.Fatalf("WSS transform was not processed by room tick: ok=%v version=%d transform=%+v", ok, version, transform)
	}
	if visible := h.room.VisiblePlayersFor(player.PlayerID("player-kcp")); !containsPlayer(visible, player.PlayerID("player-wss")) {
		t.Fatalf("cluster visibility for KCP viewer = %v, want player-wss visible", visible)
	}

	if !waitUntil(500*time.Millisecond, func() bool { return h.kcp.PacketCount() > 0 }) {
		t.Fatalf("KCP fake session did not receive PlayerDelta after WSS transform update")
	}

	env, body := decodeSinglePlayerDeltaPacket(t, h.kcp.Packets())
	if err := body.Validate(); err != nil {
		t.Fatalf("decoded PlayerDelta is invalid: %v", err)
	}
	if env.Version != protocol.CurrentVersion || env.Type != protocol.TypePlayerDelta {
		t.Fatalf("decoded envelope = version %d type %s, want Protocol v1 PlayerDelta", env.Version, env.Type)
	}
	if len(body.Updates) != 1 || body.Updates[0].PlayerID != "player-wss" {
		t.Fatalf("decoded updates = %+v, want one player-wss update", body.Updates)
	}
	update := body.Updates[0]
	if update.X != inbound.X || update.Z != inbound.Z || update.Yaw != inbound.Yaw {
		t.Fatalf("update transform = (%f,%f,%f), want (%f,%f,%f)", update.X, update.Z, update.Yaw, inbound.X, inbound.Z, inbound.Yaw)
	}
}

func TestRoomTickDispatchE2E_FailurePathsReturnErrorsAndDoNotMutateRoom(t *testing.T) {
	h := newRoomTickDispatchHarness(t)
	baseTransform, baseVersion, ok := h.room.GetPlayerState(player.PlayerID("player-kcp"))
	if !ok {
		t.Fatal("player-kcp missing from harness room")
	}
	baseCommandCount := h.enqueuer.CommandCount()

	cases := []struct {
		name string
		data []byte
	}{
		{name: "invalid MessagePack", data: []byte{0xff, 0xfe, 0xfd}},
		{name: "unsupported Protocol v1 type", data: encodeUnsupportedHelloPacket(t)},
		{name: "invalid protocol version", data: encodeInvalidVersionPacket(t)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := h.processor.Process(h.kcp, tc.data)
			if err == nil {
				t.Fatal("Process returned nil error")
			}
			transform, version, ok := h.room.GetPlayerState(player.PlayerID("player-kcp"))
			if !ok {
				t.Fatal("player-kcp missing after failed packet")
			}
			if version != baseVersion || transform.Position.X != baseTransform.Position.X || transform.Position.Z != baseTransform.Position.Z || transform.Rotation.Y != baseTransform.Rotation.Y {
				t.Fatalf("invalid packet mutated room state: version %d->%d transform %+v->%+v", baseVersion, version, baseTransform, transform)
			}
			if got := h.enqueuer.CommandCount(); got != baseCommandCount {
				t.Fatalf("invalid packet enqueued room command: count %d, want %d", got, baseCommandCount)
			}
		})
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("HandlePacket panicked on invalid packet: %v", r)
			}
		}()
		h.adapter.HandlePacket(h.wss, []byte{0xff, 0xfe, 0xfd})
	}()
}

func TestRoomTickDispatchE2E_KCPAndWSSOutboundGameplaySemanticsMatch(t *testing.T) {
	h := newRoomTickDispatchHarness(t)

	kcpInbound := protocol.PlayerTransformUpdate{Seq: 20, X: 3, Z: 4, Yaw: 0.25}
	wssInbound := protocol.PlayerTransformUpdate{Seq: 21, X: 3, Z: 4, Yaw: 0.25}
	kcpPacket, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerTransformUpdate, kcpInbound.Seq, 0, &kcpInbound)
	if err != nil {
		t.Fatalf("EncodeAndWrap KCP PlayerTransformUpdate: %v", err)
	}
	wssPacket, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerTransformUpdate, wssInbound.Seq, 0, &wssInbound)
	if err != nil {
		t.Fatalf("EncodeAndWrap WSS PlayerTransformUpdate: %v", err)
	}

	h.adapter.HandlePacket(h.kcp, kcpPacket)
	if !waitUntil(500*time.Millisecond, func() bool { return h.wss.PacketCount() > 0 }) {
		t.Fatal("WSS did not receive delta from KCP update")
	}
	_, fromKCP := decodeSinglePlayerDeltaPacket(t, h.wss.Packets())
	h.wss.ClearPackets()

	h.adapter.HandlePacket(h.wss, wssPacket)
	if !waitUntil(500*time.Millisecond, func() bool { return h.kcp.PacketCount() > 0 }) {
		t.Fatal("KCP did not receive delta from WSS update")
	}
	_, fromWSS := decodeSinglePlayerDeltaPacket(t, h.kcp.Packets())

	if len(fromKCP.Updates) != 1 || len(fromWSS.Updates) != 1 {
		t.Fatalf("updates from KCP=%+v WSS=%+v, want one update each", fromKCP.Updates, fromWSS.Updates)
	}
	kcpUpdate := fromKCP.Updates[0]
	wssUpdate := fromWSS.Updates[0]
	if kcpUpdate.X != wssUpdate.X || kcpUpdate.Z != wssUpdate.Z || kcpUpdate.Yaw != wssUpdate.Yaw {
		t.Fatalf("gameplay transform semantics differ: KCP %+v WSS %+v", kcpUpdate, wssUpdate)
	}
	if kcpUpdate.PlayerID == wssUpdate.PlayerID {
		t.Fatalf("expected deltas for opposite players, got same player %q", kcpUpdate.PlayerID)
	}
}

func decodeSinglePlayerDeltaPacket(t *testing.T, packets [][]byte) (*protocol.Envelope, protocol.PlayerDelta) {
	t.Helper()
	if len(packets) != 1 {
		t.Fatalf("packets = %d, want 1", len(packets))
	}
	data := packets[0]
	if len(data) == 0 {
		t.Fatal("empty outbound packet")
	}
	firstByte := data[0]
	isMsgpackMap := (firstByte >= 0x80 && firstByte <= 0x8f) || firstByte == 0xde || firstByte == 0xdf
	if !isMsgpackMap {
		t.Fatalf("first byte = 0x%02x, want MessagePack map header", firstByte)
	}
	if firstByte == 0x7b {
		t.Fatal("outbound gameplay packet looks like JSON")
	}

	env, err := protocol.DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	var body protocol.PlayerDelta
	if err := protocol.DecodeMessage(env.Body, &body); err != nil {
		t.Fatalf("DecodeMessage PlayerDelta: %v", err)
	}
	return env, body
}

func assertLastEnqueuedTransformCommand(t *testing.T, enqueuer *recordingRoomEnqueuer, sessionID room.SessionID, playerID room.PlayerID) {
	t.Helper()
	commands := enqueuer.Commands()
	if len(commands) == 0 {
		t.Fatal("no room commands recorded")
	}
	cmd := commands[len(commands)-1]
	if cmd.Kind != room.CmdUpdatePlayerTransform {
		t.Fatalf("last command kind = %d, want CmdUpdatePlayerTransform", cmd.Kind)
	}
	if cmd.SessionID != sessionID {
		t.Fatalf("last command session = %q, want %q", cmd.SessionID, sessionID)
	}
	if cmd.PlayerID != playerID {
		t.Fatalf("last command player = %q, want %q", cmd.PlayerID, playerID)
	}
}

func encodeUnsupportedHelloPacket(t *testing.T) []byte {
	t.Helper()
	body, err := protocol.EncodeMessage(&protocol.Hello{Version: protocol.CurrentVersion})
	if err != nil {
		t.Fatalf("EncodeMessage Hello: %v", err)
	}
	data, err := protocol.EncodeEnvelope(&protocol.Envelope{
		Version: protocol.CurrentVersion,
		Type:    protocol.TypeHello,
		Seq:     1,
		Body:    body,
	})
	if err != nil {
		t.Fatalf("EncodeEnvelope Hello: %v", err)
	}
	return data
}

func encodeInvalidVersionPacket(t *testing.T) []byte {
	t.Helper()
	body, err := protocol.EncodeMessage(&protocol.PlayerTransformUpdate{Seq: 1, X: 1, Z: 2, Yaw: 3})
	if err != nil {
		t.Fatalf("EncodeMessage PlayerTransformUpdate: %v", err)
	}
	data, err := msgpack.Marshal(&protocol.Envelope{
		Version: protocol.CurrentVersion + 1,
		Type:    protocol.TypePlayerTransformUpdate,
		Seq:     1,
		Body:    body,
	})
	if err != nil {
		t.Fatalf("marshal invalid-version envelope: %v", err)
	}
	return data
}

func containsPlayer(players []player.PlayerID, want player.PlayerID) bool {
	for _, got := range players {
		if got == want {
			return true
		}
	}
	return false
}

func waitUntil(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}
