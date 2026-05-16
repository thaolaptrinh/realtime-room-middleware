package realtime

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/handler"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

type fakeSession struct {
	id        string
	transport transport.TransportType
	sendErr   error
}

func (f *fakeSession) ID() string                         { return f.id }
func (f *fakeSession) UserID() string                     { return "" }
func (f *fakeSession) Transport() transport.TransportType { return f.transport }
func (f *fakeSession) Close() error                       { return nil }
func (f *fakeSession) Send(packet []byte) error           { return f.sendErr }

type mockRoom struct {
	commands []room.RoomCommand
	enqErr   error
}

func (m *mockRoom) Enqueue(cmd room.RoomCommand) error {
	if m.enqErr != nil {
		return m.enqErr
	}
	m.commands = append(m.commands, cmd)
	return nil
}

func testResolver(roomMap map[string]*mockRoom) handler.SessionRoomResolver {
	return func(sessionID string) (handler.RoomEnqueuer, error) {
		r, ok := roomMap[sessionID]
		if !ok {
			return nil, nil
		}
		return r, nil
	}
}

func testSessionResolver(known map[string][2]string) SessionResolver {
	return func(sessionID string) (string, string) {
		if ids, ok := known[sessionID]; ok {
			return ids[0], ids[1]
		}
		return "", ""
	}
}

func encodePlayerInput(t *testing.T, msg protocol.PlayerInput) []byte {
	t.Helper()
	data, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerInput, 1, 0, &msg)
	if err != nil {
		t.Fatalf("encode PlayerInput: %v", err)
	}
	return data
}

func encodePlayerTransformUpdate(t *testing.T, msg protocol.PlayerTransformUpdate) []byte {
	t.Helper()
	data, err := protocol.EncodeAndWrap(protocol.CurrentVersion, protocol.TypePlayerTransformUpdate, 1, 0, &msg)
	if err != nil {
		t.Fatalf("encode PlayerTransformUpdate: %v", err)
	}
	return data
}

func newTestProcessor(t *testing.T, roomMap map[string]*mockRoom, sessionMap map[string][2]string) *PacketProcessor {
	t.Helper()
	h := handler.NewRealtimePacketHandler(testResolver(roomMap))
	router := NewSessionPacketRouter(testSessionResolver(sessionMap))
	return NewPacketProcessor(h, router, nil)
}

func TestPacketProcessor_ValidPlayerInput(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)

	msg := protocol.PlayerInput{Seq: 1, MoveX: 0.5, MoveZ: -0.5, Yaw: 1.0}
	data := encodePlayerInput(t, msg)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	if err := proc.Process(sess, data); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rm.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(rm.commands))
	}
	if rm.commands[0].Kind != room.CmdPlayerInput {
		t.Errorf("Kind = %d, want CmdPlayerInput", rm.commands[0].Kind)
	}
	if rm.commands[0].PlayerID != room.PlayerID("player-1") {
		t.Errorf("PlayerID = %q, want player-1", rm.commands[0].PlayerID)
	}
}

func TestPacketProcessor_ValidPlayerTransformUpdate(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 10.0, Z: 20.0, Yaw: 1.5}
	data := encodePlayerTransformUpdate(t, msg)
	sess := &fakeSession{id: "sess-1", transport: transport.WebSocket}

	if err := proc.Process(sess, data); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rm.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(rm.commands))
	}
	if rm.commands[0].Kind != room.CmdUpdatePlayerTransform {
		t.Errorf("Kind = %d, want CmdUpdatePlayerTransform", rm.commands[0].Kind)
	}
}

func TestPacketProcessor_InvalidBytes(t *testing.T) {
	proc := newTestProcessor(t, nil, nil)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	err := proc.Process(sess, []byte{0xFF, 0xFE, 0xFD})
	if err == nil {
		t.Fatal("expected error for garbage bytes")
	}
}

func TestPacketProcessor_EmptyBytes(t *testing.T) {
	proc := newTestProcessor(t, nil, nil)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	err := proc.Process(sess, []byte{})
	if err == nil {
		t.Fatal("expected error for empty bytes")
	}
}

func TestPacketProcessor_UnsupportedMessageType(t *testing.T) {
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": {}},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)

	helloBody, _ := protocol.EncodeMessage(&protocol.Hello{Version: protocol.CurrentVersion})
	env := &protocol.Envelope{
		Version: protocol.CurrentVersion,
		Type:    protocol.TypeHello,
		Seq:     1,
		Body:    helloBody,
	}
	data, err := protocol.EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	sess := &fakeSession{id: "sess-1", transport: transport.KCP}
	err = proc.Process(sess, data)
	if err == nil {
		t.Fatal("expected error for unsupported message type")
	}
}

func TestPacketProcessor_SessionNotAttachedToRoom(t *testing.T) {
	proc := newTestProcessor(t,
		map[string]*mockRoom{},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)

	msg := protocol.PlayerInput{Seq: 1, MoveX: 0.5, MoveZ: 0.0, Yaw: 0.0}
	data := encodePlayerInput(t, msg)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	err := proc.Process(sess, data)
	if err == nil {
		t.Fatal("expected error for session not attached to room")
	}
}

func TestPacketProcessor_EmptyPlayerID(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"", ""}},
	)

	msg := protocol.PlayerInput{Seq: 1, MoveX: 0.5, MoveZ: 0.0, Yaw: 0.0}
	data := encodePlayerInput(t, msg)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	err := proc.Process(sess, data)
	if err == nil {
		t.Fatal("expected error for empty player ID")
	}
	if len(rm.commands) != 0 {
		t.Errorf("commands = %d, want 0", len(rm.commands))
	}
}

func TestPacketProcessor_HandlerError(t *testing.T) {
	rm := &mockRoom{enqErr: fmt.Errorf("room command queue full")}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 5.0, Z: 3.0, Yaw: 0.5}
	data := encodePlayerTransformUpdate(t, msg)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	err := proc.Process(sess, data)
	if err == nil {
		t.Fatal("expected error when handler fails")
	}
}

func TestPacketProcessor_NoPanic(t *testing.T) {
	proc := newTestProcessor(t, nil, nil)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	for _, input := range [][]byte{nil, {}, {0x00}, {0xFF, 0xFF, 0xFF, 0xFF}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Process panicked on input %v: %v", input, r)
				}
			}()
			_ = proc.Process(sess, input)
		}()
	}
}

func TestReceiveLoopAdapter_NoPanicOnError(t *testing.T) {
	proc := newTestProcessor(t, nil, nil)
	adapter := NewReceiveLoopAdapter(proc, nil)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("HandlePacket panicked: %v", r)
			}
		}()
		adapter.HandlePacket(sess, []byte{0xFF, 0xFE})
	}()
}

func TestReceiveLoopAdapter_SuccessNoError(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)
	adapter := NewReceiveLoopAdapter(proc, nil)

	msg := protocol.PlayerInput{Seq: 1, MoveX: 1.0, MoveZ: 0.0, Yaw: 0.0}
	data := encodePlayerInput(t, msg)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("HandlePacket panicked: %v", r)
			}
		}()
		adapter.HandlePacket(sess, data)
	}()

	if len(rm.commands) != 1 {
		t.Errorf("commands = %d, want 1", len(rm.commands))
	}
}

func TestMixedTransport_SameReceiveBehavior(t *testing.T) {
	kcpRoom := &mockRoom{}
	wssRoom := &mockRoom{}

	proc := newTestProcessor(t,
		map[string]*mockRoom{"kcp-sess": kcpRoom, "wss-sess": wssRoom},
		map[string][2]string{"kcp-sess": {"p1", "u1"}, "wss-sess": {"p1", "u1"}},
	)

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 10.0, Z: 20.0, Yaw: 1.5}
	data := encodePlayerTransformUpdate(t, msg)

	kcpSess := &fakeSession{id: "kcp-sess", transport: transport.KCP}
	wssSess := &fakeSession{id: "wss-sess", transport: transport.WebSocket}

	kcpErr := proc.Process(kcpSess, data)
	wssErr := proc.Process(wssSess, data)

	if kcpErr != nil || wssErr != nil {
		t.Fatalf("KCP error=%v, WSS error=%v — both should be nil", kcpErr, wssErr)
	}
	if len(kcpRoom.commands) != 1 || len(wssRoom.commands) != 1 {
		t.Fatalf("KCP commands=%d, WSS commands=%d — both want 1", len(kcpRoom.commands), len(wssRoom.commands))
	}
	if kcpRoom.commands[0].Kind != wssRoom.commands[0].Kind {
		t.Errorf("KCP Kind=%d, WSS Kind=%d — must be identical", kcpRoom.commands[0].Kind, wssRoom.commands[0].Kind)
	}
	if kcpRoom.commands[0].PlayerID != wssRoom.commands[0].PlayerID {
		t.Errorf("KCP PlayerID=%q, WSS PlayerID=%q — must be identical", kcpRoom.commands[0].PlayerID, wssRoom.commands[0].PlayerID)
	}
}

func TestSessionPacketRouter_ResolveContext(t *testing.T) {
	router := NewSessionPacketRouter(func(sessionID string) (string, string) {
		return "player-42", "user-42"
	})

	sess := &fakeSession{id: "sess-1", transport: transport.KCP}
	ctx := router.ResolveContext(sess)

	if ctx.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", ctx.SessionID)
	}
	if ctx.PlayerID != room.PlayerID("player-42") {
		t.Errorf("PlayerID = %q, want player-42", ctx.PlayerID)
	}
	if ctx.UserID != room.UserID("user-42") {
		t.Errorf("UserID = %q, want user-42", ctx.UserID)
	}
}

func TestSessionPacketRouter_UnresolvedSession(t *testing.T) {
	router := NewSessionPacketRouter(func(sessionID string) (string, string) {
		return "", ""
	})

	sess := &fakeSession{id: "unknown", transport: transport.WebSocket}
	ctx := router.ResolveContext(sess)

	if ctx.SessionID != "unknown" {
		t.Errorf("SessionID = %q, want unknown", ctx.SessionID)
	}
	if ctx.PlayerID != "" {
		t.Errorf("PlayerID = %q, want empty", ctx.PlayerID)
	}
	if ctx.UserID != "" {
		t.Errorf("UserID = %q, want empty", ctx.UserID)
	}
}

func TestMultiplePacketsInSequence(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)
	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	for i := 0; i < 5; i++ {
		msg := protocol.PlayerInput{Seq: uint32(i), MoveX: float32(i) * 0.1, MoveZ: 0, Yaw: 0}
		data := encodePlayerInput(t, msg)
		if err := proc.Process(sess, data); err != nil {
			t.Errorf("packet %d: %v", i, err)
		}
	}

	if len(rm.commands) != 5 {
		t.Errorf("commands = %d, want 5", len(rm.commands))
	}
}

func TestKCPReceivePath_PlayerTransformUpdate(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"kcp-sess": rm},
		map[string][2]string{"kcp-sess": {"player-kcp", "user-kcp"}},
	)

	msg := protocol.PlayerTransformUpdate{Seq: 7, X: 15.0, Z: -25.0, Yaw: 2.1}
	data := encodePlayerTransformUpdate(t, msg)
	sess := &fakeSession{id: "kcp-sess", transport: transport.KCP}

	if err := proc.Process(sess, data); err != nil {
		t.Fatalf("KCP Process PlayerTransformUpdate: %v", err)
	}
	if len(rm.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(rm.commands))
	}
	if rm.commands[0].Kind != room.CmdUpdatePlayerTransform {
		t.Errorf("Kind = %d, want CmdUpdatePlayerTransform", rm.commands[0].Kind)
	}
	if rm.commands[0].PlayerID != room.PlayerID("player-kcp") {
		t.Errorf("PlayerID = %q, want player-kcp", rm.commands[0].PlayerID)
	}
	if rm.commands[0].SessionID != room.SessionID("kcp-sess") {
		t.Errorf("SessionID = %q, want kcp-sess", rm.commands[0].SessionID)
	}
}

func TestWSSReceivePath_PlayerInput(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"wss-sess": rm},
		map[string][2]string{"wss-sess": {"player-wss", "user-wss"}},
	)

	msg := protocol.PlayerInput{Seq: 3, MoveX: -0.75, MoveZ: 0.3, Yaw: 0.8}
	data := encodePlayerInput(t, msg)
	sess := &fakeSession{id: "wss-sess", transport: transport.WebSocket}

	if err := proc.Process(sess, data); err != nil {
		t.Fatalf("WSS Process PlayerInput: %v", err)
	}
	if len(rm.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(rm.commands))
	}
	if rm.commands[0].Kind != room.CmdPlayerInput {
		t.Errorf("Kind = %d, want CmdPlayerInput", rm.commands[0].Kind)
	}
	if rm.commands[0].PlayerID != room.PlayerID("player-wss") {
		t.Errorf("PlayerID = %q, want player-wss", rm.commands[0].PlayerID)
	}
	if rm.commands[0].SessionID != room.SessionID("wss-sess") {
		t.Errorf("SessionID = %q, want wss-sess", rm.commands[0].SessionID)
	}
}

func TestInvalidPacket_NoRoomStateMutation(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)

	garbageInputs := [][]byte{
		{0xFF, 0xFE, 0xFD, 0xFC},
		{},
		nil,
		{0xDE, 0xAD, 0xBE, 0xEF},
	}

	sess := &fakeSession{id: "sess-1", transport: transport.KCP}
	for i, input := range garbageInputs {
		err := proc.Process(sess, input)
		if err == nil {
			t.Errorf("input %d: expected error for garbage bytes", i)
		}
	}

	if len(rm.commands) != 0 {
		t.Errorf("room state mutated: commands = %d, want 0", len(rm.commands))
	}
}

func TestInvalidPacket_WSSNoRoomStateMutation(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"wss-sess": rm},
		map[string][2]string{"wss-sess": {"player-1", "user-1"}},
	)

	sess := &fakeSession{id: "wss-sess", transport: transport.WebSocket}
	err := proc.Process(sess, []byte{0xFF, 0xFE, 0xFD})
	if err == nil {
		t.Fatal("expected error for invalid packet")
	}
	if len(rm.commands) != 0 {
		t.Errorf("room state mutated: commands = %d, want 0", len(rm.commands))
	}
}

func TestReceiveLoopAdapter_InvalidPacketNoRoomMutation(t *testing.T) {
	rm := &mockRoom{}
	proc := newTestProcessor(t,
		map[string]*mockRoom{"sess-1": rm},
		map[string][2]string{"sess-1": {"player-1", "user-1"}},
	)
	adapter := NewReceiveLoopAdapter(proc, nil)

	sess := &fakeSession{id: "sess-1", transport: transport.KCP}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("HandlePacket panicked on invalid data: %v", r)
			}
		}()
		adapter.HandlePacket(sess, []byte{0xFF, 0xFE, 0xFD})
	}()

	if len(rm.commands) != 0 {
		t.Errorf("room state mutated via adapter: commands = %d, want 0", len(rm.commands))
	}
}

func TestMixedTransport_IdenticalProtocolSemantics_PlayerInput(t *testing.T) {
	kcpRoom := &mockRoom{}
	wssRoom := &mockRoom{}

	proc := newTestProcessor(t,
		map[string]*mockRoom{"kcp-sess": kcpRoom, "wss-sess": wssRoom},
		map[string][2]string{"kcp-sess": {"p1", "u1"}, "wss-sess": {"p1", "u1"}},
	)

	msg := protocol.PlayerInput{Seq: 42, MoveX: 0.5, MoveZ: -0.5, Yaw: 1.0}
	data := encodePlayerInput(t, msg)

	kcpSess := &fakeSession{id: "kcp-sess", transport: transport.KCP}
	wssSess := &fakeSession{id: "wss-sess", transport: transport.WebSocket}

	kcpErr := proc.Process(kcpSess, data)
	wssErr := proc.Process(wssSess, data)

	if kcpErr != nil || wssErr != nil {
		t.Fatalf("KCP error=%v, WSS error=%v — both should be nil", kcpErr, wssErr)
	}
	if kcpRoom.commands[0].Kind != wssRoom.commands[0].Kind {
		t.Errorf("KCP Kind=%d, WSS Kind=%d — must match", kcpRoom.commands[0].Kind, wssRoom.commands[0].Kind)
	}
	if kcpRoom.commands[0].PlayerID != wssRoom.commands[0].PlayerID {
		t.Errorf("KCP PlayerID=%q, WSS PlayerID=%q — must match", kcpRoom.commands[0].PlayerID, wssRoom.commands[0].PlayerID)
	}
}

// --- Boundary: transport packages do not import internal/game ---

func TestBoundary_TransportPackagesDoNotImportGame(t *testing.T) {
	transportDirs := []string{
		filepath.Join("..", "..", "transport", "kcp"),
		filepath.Join("..", "..", "transport", "websocket"),
	}

	for _, dir := range transportDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			filePath := filepath.Join(dir, entry.Name())
			f, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("ParseFile %s: %v", filePath, err)
			}
			for _, imp := range f.Imports {
				path := strings.Trim(imp.Path.Value, "\"")
				if strings.Contains(path, "/internal/game") {
					t.Errorf("transport package %s imports game package: %s", filePath, path)
				}
			}
		}
	}
}

// --- Boundary: protocol package does not import internal/game ---

func TestBoundary_ProtocolPackageDoesNotImportGame(t *testing.T) {
	protoDir := filepath.Join("..", "..", "protocol")
	entries, err := os.ReadDir(protoDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", protoDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		filePath := filepath.Join(protoDir, entry.Name())
		f, err := parser.ParseFile(token.NewFileSet(), filePath, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", filePath, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			if strings.Contains(path, "/internal/game") {
				t.Errorf("protocol package %s imports game package: %s", filePath, path)
			}
		}
	}
}

// --- Boundary: no JSON/protobuf in realtime runtime paths ---

func TestBoundary_RealtimeRuntimeNoJSONNoProtobuf(t *testing.T) {
	runtimeDirs := []string{
		".",
		filepath.Join("..", "..", "transport", "kcp"),
		filepath.Join("..", "..", "transport", "websocket"),
		filepath.Join("..", "..", "game", "handler"),
		filepath.Join("..", "..", "protocol"),
	}

	for _, dir := range runtimeDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			filePath := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("ReadFile %s: %v", filePath, err)
			}
			contents := string(data)
			if strings.Contains(contents, "encoding/json") {
				t.Errorf("JSON import in realtime runtime: %s", filePath)
			}
			if strings.Contains(contents, "google.golang.org/protobuf") || strings.Contains(contents, ".proto") {
				t.Errorf("protobuf in realtime runtime: %s", filePath)
			}
		}
	}
}
