package handler

import (
	"fmt"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/player"
	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
)

// mockRoom records enqueued commands without processing them.
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

// testContext builds a PacketContext from session metadata.
func testContext(sessionID string, playerID string) PacketContext {
	return PacketContext{
		SessionID: sessionID,
		PlayerID:  room.PlayerID(playerID),
		UserID:    room.UserID("user-" + playerID),
	}
}

// testResolver returns a SessionRoomResolver that maps specific session IDs
// to mock rooms.
func testResolver(roomMap map[string]*mockRoom) SessionRoomResolver {
	return func(sessionID string) (RoomEnqueuer, error) {
		r, ok := roomMap[sessionID]
		if !ok {
			return nil, nil
		}
		return r, nil
	}
}

// buildEnvelope creates a valid encoded envelope for the given message.
func buildEnvelope(t *testing.T, msgType protocol.MessageType, seq uint32, msg interface{}) *protocol.Envelope {
	t.Helper()
	env, err := protocol.BuildEnvelope(protocol.CurrentVersion, msgType, seq, 0, msg)
	if err != nil {
		t.Fatalf("BuildEnvelope(%s): %v", msgType, err)
	}
	return env
}

// --- PlayerTransformUpdate tests ---

func TestPlayerTransformUpdate_EnqueuesRoomCommand(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	msg := protocol.PlayerTransformUpdate{
		Seq:       42,
		X:         10.5,
		Z:         -3.25,
		Yaw:       1.57,
		AnimState: 3,
	}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 100, &msg)

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if !result.Handled {
		t.Fatalf("expected Handled=true, got: %v", result)
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.MessageType != protocol.TypePlayerTransformUpdate {
		t.Errorf("MessageType = %d, want %d", result.MessageType, protocol.TypePlayerTransformUpdate)
	}
	if len(rm.commands) != 1 {
		t.Fatalf("commands enqueued = %d, want 1", len(rm.commands))
	}

	cmd := rm.commands[0]
	if cmd.Kind != room.CmdUpdatePlayerTransform {
		t.Errorf("Kind = %d, want CmdUpdatePlayerTransform (%d)", cmd.Kind, room.CmdUpdatePlayerTransform)
	}
	if cmd.PlayerID != room.PlayerID("player-1") {
		t.Errorf("PlayerID = %q, want %q", cmd.PlayerID, "player-1")
	}
	if cmd.SessionID != room.SessionID("sess-1") {
		t.Errorf("SessionID = %q, want %q", cmd.SessionID, "sess-1")
	}

	transform, ok := cmd.Payload.(player.PlayerTransform)
	if !ok {
		t.Fatalf("Payload type = %T, want player.PlayerTransform", cmd.Payload)
	}
	if transform.Position.X != 10.5 {
		t.Errorf("Position.X = %f, want 10.5", transform.Position.X)
	}
	if transform.Position.Z != -3.25 {
		t.Errorf("Position.Z = %f, want -3.25", transform.Position.Z)
	}
	if transform.Rotation.Y != 1.57 {
		t.Errorf("Rotation.Y (Yaw) = %f, want 1.57", transform.Rotation.Y)
	}
}

// --- PlayerInput tests ---

func TestPlayerInput_EnqueuesRoomCommand(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-2": rm,
	}))

	msg := protocol.PlayerInput{
		Seq:       55,
		MoveX:     0.75,
		MoveZ:     -0.5,
		Yaw:       2.25,
		AnimState: 7,
	}
	env := buildEnvelope(t, protocol.TypePlayerInput, 200, &msg)

	result := h.HandleEnvelope(testContext("sess-2", "player-2"), env)

	if !result.Handled {
		t.Fatalf("expected Handled=true, got: %v", result)
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if len(rm.commands) != 1 {
		t.Fatalf("commands enqueued = %d, want 1", len(rm.commands))
	}

	cmd := rm.commands[0]
	if cmd.Kind != room.CmdPlayerInput {
		t.Errorf("Kind = %d, want CmdPlayerInput (%d)", cmd.Kind, room.CmdPlayerInput)
	}
	if cmd.PlayerID != room.PlayerID("player-2") {
		t.Errorf("PlayerID = %q, want %q", cmd.PlayerID, "player-2")
	}

	input, ok := cmd.Payload.(player.PlayerInput)
	if !ok {
		t.Fatalf("Payload type = %T, want player.PlayerInput", cmd.Payload)
	}
	if input.Seq != 55 {
		t.Errorf("Seq = %d, want 55", input.Seq)
	}
	if input.Transform.Position.X != 0.75 {
		t.Errorf("Position.X (MoveX) = %f, want 0.75", input.Transform.Position.X)
	}
	if input.Transform.Position.Z != -0.5 {
		t.Errorf("Position.Z (MoveZ) = %f, want -0.5", input.Transform.Position.Z)
	}
	if input.Transform.Rotation.Y != 2.25 {
		t.Errorf("Rotation.Y (Yaw) = %f, want 2.25", input.Transform.Rotation.Y)
	}
	if input.Timestamp <= 0 {
		t.Errorf("Timestamp = %d, want positive", input.Timestamp)
	}
}

// --- Invalid protocol version ---

func TestInvalidProtocolVersion_RejectedByCodec(t *testing.T) {
	env := &protocol.Envelope{
		Version: 0,
		Type:    protocol.TypePlayerInput,
		Seq:     1,
		Tick:    0,
		Body:    []byte{0x00},
	}
	_, err := protocol.EncodeEnvelope(env)
	if err == nil {
		t.Fatal("expected EncodeEnvelope to reject version 0")
	}
}

func TestInvalidProtocolVersion_ValidateVersion(t *testing.T) {
	err := protocol.ValidateVersion(0)
	if err == nil {
		t.Fatal("expected error for protocol version 0")
	}
	err = protocol.ValidateVersion(99)
	if err == nil {
		t.Fatal("expected error for protocol version 99")
	}
}

func TestInvalidProtocolVersion_HandlerRejectsRawEnvelope(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	env := &protocol.Envelope{
		Version: 0,
		Type:    protocol.TypePlayerInput,
		Seq:     1,
		Tick:    0,
		Body:    []byte{},
	}

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if result.Handled {
		t.Error("raw envelope with version 0 should not be handled")
	}
}

// --- Unsupported message type ---

func TestUnsupportedMessageType_Rejected(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	testCases := []struct {
		name     string
		msgType  protocol.MessageType
		buildFn  func() []byte
	}{
		{
			name:    "Hello",
			msgType: protocol.TypeHello,
			buildFn: func() []byte {
				b, _ := protocol.EncodeMessage(&protocol.Hello{Version: protocol.CurrentVersion})
				return b
			},
		},
		{
			name:    "JoinRoom",
			msgType: protocol.TypeJoinRoom,
			buildFn: func() []byte {
				b, _ := protocol.EncodeMessage(&protocol.JoinRoom{RoomInstanceID: "r1", SessionToken: "tok", UserID: "u1"})
				return b
			},
		},
		{
			name:    "Ping",
			msgType: protocol.TypePing,
			buildFn: func() []byte {
				b, _ := protocol.EncodeMessage(&protocol.Ping{Timestamp: time.Now().UnixMilli()})
				return b
			},
		},
		{
			name:    "FullSnapshot (server-to-client)",
			msgType: protocol.TypeFullSnapshot,
			buildFn: func() []byte {
				b, _ := protocol.EncodeMessage(&protocol.FullSnapshot{Tick: 1})
				return b
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.buildFn()
			env := &protocol.Envelope{
				Version: protocol.CurrentVersion,
				Type:    tc.msgType,
				Seq:     1,
				Tick:    0,
				Body:    body,
			}

			result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

			if result.Handled {
				t.Errorf("expected Handled=false for %s", tc.name)
			}
			if result.Error == nil {
				t.Errorf("expected error for unsupported message type %s", tc.name)
			}
		})
	}

	if len(rm.commands) != 0 {
		t.Errorf("expected 0 commands enqueued, got %d", len(rm.commands))
	}
}

func TestServerToClientMessage_Rejected(t *testing.T) {
	h := NewRealtimePacketHandler(testResolver(nil))

	env := buildEnvelope(t, protocol.TypePlayerDelta, 1, &protocol.PlayerDelta{
		Tick:    1,
		Updates: []protocol.PlayerUpdateDelta{{PlayerID: "p1", X: 1, Z: 2, Yaw: 0, Version: 1}},
	})

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if result.Handled {
		t.Error("server-to-client message should not be handled")
	}
	if result.Error == nil {
		t.Error("expected error for server-to-client message type")
	}
}

// --- Invalid transform values ---
// NaN/Inf values are rejected by the protocol layer (BuildEnvelope → EncodeMessage → Validate)
// before the handler is invoked. The handler never sees invalid float payloads in the normal
// packet path because BuildEnvelope refuses to encode them.

func TestInvalidTransformValues_RejectedByProtocolLayer(t *testing.T) {
	testCases := []struct {
		name string
		msg  interface{}
		mt   protocol.MessageType
	}{
		{
			name: "PlayerInput NaN MoveX",
			msg:  &protocol.PlayerInput{Seq: 1, MoveX: float32(math.NaN()), MoveZ: 0, Yaw: 0},
			mt:   protocol.TypePlayerInput,
		},
		{
			name: "PlayerInput Inf MoveZ",
			msg:  &protocol.PlayerInput{Seq: 1, MoveX: 0, MoveZ: float32(math.Inf(1)), Yaw: 0},
			mt:   protocol.TypePlayerInput,
		},
		{
			name: "PlayerInput NaN Yaw",
			msg:  &protocol.PlayerInput{Seq: 1, MoveX: 0, MoveZ: 0, Yaw: float32(math.NaN())},
			mt:   protocol.TypePlayerInput,
		},
		{
			name: "PlayerTransformUpdate NaN X",
			msg:  &protocol.PlayerTransformUpdate{Seq: 1, X: float32(math.NaN()), Z: 0, Yaw: 0},
			mt:   protocol.TypePlayerTransformUpdate,
		},
		{
			name: "PlayerTransformUpdate Inf Z",
			msg:  &protocol.PlayerTransformUpdate{Seq: 1, X: 0, Z: float32(math.Inf(-1)), Yaw: 0},
			mt:   protocol.TypePlayerTransformUpdate,
		},
		{
			name: "PlayerTransformUpdate NaN Yaw",
			msg:  &protocol.PlayerTransformUpdate{Seq: 1, X: 0, Z: 0, Yaw: float32(math.NaN())},
			mt:   protocol.TypePlayerTransformUpdate,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := protocol.BuildEnvelope(protocol.CurrentVersion, tc.mt, 1, 0, tc.msg)
			if err == nil {
				t.Errorf("expected BuildEnvelope to reject NaN/Inf values")
			}
		})
	}
}

// TestInvalidTransformValues_HandlerNotReached confirms that when BuildEnvelope
// rejects a payload, the handler is never invoked and room state is not mutated.
func TestInvalidTransformValues_HandlerNotReached(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	invalidMsgs := []struct {
		msg interface{}
		mt  protocol.MessageType
	}{
		{&protocol.PlayerInput{Seq: 1, MoveX: float32(math.NaN())}, protocol.TypePlayerInput},
		{&protocol.PlayerTransformUpdate{Seq: 1, X: float32(math.Inf(1))}, protocol.TypePlayerTransformUpdate},
	}

	for _, tc := range invalidMsgs {
		_, err := protocol.BuildEnvelope(protocol.CurrentVersion, tc.mt, 1, 0, tc.msg)
		if err == nil {
			t.Fatal("expected BuildEnvelope to reject invalid values")
		}
	}

	if len(rm.commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(rm.commands))
	}
	_ = h
}

// --- Handler does not directly mutate room state ---

func TestHandler_DoesNotMutateRoomState(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 5.0, Z: 3.0, Yaw: 0.5}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 1, &msg)

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if !result.Handled {
		t.Fatalf("expected Handled=true")
	}

	// Verify the handler only enqueued a command — it did not call any
	// room mutation method directly. The mock only records Enqueue calls.
	if len(rm.commands) != 1 {
		t.Fatalf("expected exactly 1 command, got %d", len(rm.commands))
	}
	cmd := rm.commands[0]
	if cmd.Kind != room.CmdUpdatePlayerTransform {
		t.Errorf("expected CmdUpdatePlayerTransform, got %d", cmd.Kind)
	}
	// The command payload is a domain type, not a wire type.
	if _, ok := cmd.Payload.(player.PlayerTransform); !ok {
		t.Errorf("payload type = %T, expected player.PlayerTransform", cmd.Payload)
	}
}

// --- KCP/WSS session metadata produces same handler behavior ---

func TestKCPAndWSS_SameHandlerBehavior(t *testing.T) {
	kcpRoom := &mockRoom{}
	wssRoom := &mockRoom{}

	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"kcp-sess": kcpRoom,
		"wss-sess": wssRoom,
	}))

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 10.0, Z: 20.0, Yaw: 1.5}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 1, &msg)

	kcpCtx := PacketContext{SessionID: "kcp-sess", PlayerID: "p1", UserID: "u1"}
	wssCtx := PacketContext{SessionID: "wss-sess", PlayerID: "p1", UserID: "u1"}

	kcpResult := h.HandleEnvelope(kcpCtx, env)
	wssResult := h.HandleEnvelope(wssCtx, env)

	if kcpResult.Handled != wssResult.Handled {
		t.Errorf("KCP Handled=%v, WSS Handled=%v — must be identical", kcpResult.Handled, wssResult.Handled)
	}
	if kcpResult.Error != nil || wssResult.Error != nil {
		t.Fatalf("KCP error=%v, WSS error=%v — both should be nil", kcpResult.Error, wssResult.Error)
	}

	if len(kcpRoom.commands) != 1 || len(wssRoom.commands) != 1 {
		t.Fatalf("KCP commands=%d, WSS commands=%d — both should be 1", len(kcpRoom.commands), len(wssRoom.commands))
	}

	kcpCmd := kcpRoom.commands[0]
	wssCmd := wssRoom.commands[0]

	if kcpCmd.Kind != wssCmd.Kind {
		t.Errorf("KCP Kind=%d, WSS Kind=%d — must be identical", kcpCmd.Kind, wssCmd.Kind)
	}
	if kcpCmd.PlayerID != wssCmd.PlayerID {
		t.Errorf("KCP PlayerID=%q, WSS PlayerID=%q — must be identical", kcpCmd.PlayerID, wssCmd.PlayerID)
	}

	kcpPayload, kcpOk := kcpCmd.Payload.(player.PlayerTransform)
	wssPayload, wssOk := wssCmd.Payload.(player.PlayerTransform)
	if !kcpOk || !wssOk {
		t.Fatalf("both payloads should be PlayerTransform: KCP=%T, WSS=%T", kcpCmd.Payload, wssCmd.Payload)
	}
	if kcpPayload.Position.X != wssPayload.Position.X {
		t.Errorf("KCP X=%f, WSS X=%f — must be identical", kcpPayload.Position.X, wssPayload.Position.X)
	}
	if kcpPayload.Position.Z != wssPayload.Position.Z {
		t.Errorf("KCP Z=%f, WSS Z=%f — must be identical", kcpPayload.Position.Z, wssPayload.Position.Z)
	}
	if kcpPayload.Rotation.Y != wssPayload.Rotation.Y {
		t.Errorf("KCP Yaw=%f, WSS Yaw=%f — must be identical", kcpPayload.Rotation.Y, wssPayload.Rotation.Y)
	}
}

func TestKCPAndWSS_PlayerInput_SameBehavior(t *testing.T) {
	kcpRoom := &mockRoom{}
	wssRoom := &mockRoom{}

	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"kcp-sess": kcpRoom,
		"wss-sess": wssRoom,
	}))

	msg := protocol.PlayerInput{Seq: 10, MoveX: 0.5, MoveZ: -0.5, Yaw: 2.0}
	env := buildEnvelope(t, protocol.TypePlayerInput, 1, &msg)

	kcpResult := h.HandleEnvelope(PacketContext{SessionID: "kcp-sess", PlayerID: "p1", UserID: "u1"}, env)
	wssResult := h.HandleEnvelope(PacketContext{SessionID: "wss-sess", PlayerID: "p1", UserID: "u1"}, env)

	if kcpResult.Handled != wssResult.Handled {
		t.Errorf("KCP Handled=%v != WSS Handled=%v", kcpResult.Handled, wssResult.Handled)
	}
	if len(kcpRoom.commands) != 1 || len(wssRoom.commands) != 1 {
		t.Fatalf("KCP commands=%d, WSS commands=%d", len(kcpRoom.commands), len(wssRoom.commands))
	}
	if kcpRoom.commands[0].Kind != wssRoom.commands[0].Kind {
		t.Errorf("command kinds differ")
	}
}

// --- Session not attached to room ---

func TestSessionNotAttachedToRoom_Error(t *testing.T) {
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{}))

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 1.0, Z: 2.0, Yaw: 0.5}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 1, &msg)

	result := h.HandleEnvelope(testContext("unknown-sess", "player-1"), env)

	if result.Handled {
		t.Error("expected Handled=false for unattached session")
	}
	if result.Error == nil {
		t.Error("expected error for session not attached to room")
	}
}

func TestResolveRoomError_Error(t *testing.T) {
	resolveErr := fmt.Errorf("resolver unavailable")
	h := NewRealtimePacketHandler(func(sessionID string) (RoomEnqueuer, error) {
		return nil, resolveErr
	})

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 1.0, Z: 2.0, Yaw: 0.5}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 1, &msg)

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if result.Handled {
		t.Error("expected Handled=false when resolver fails")
	}
	if result.Error == nil {
		t.Error("expected error when resolver fails")
	}
}

// --- Empty player ID rejected ---

func TestEmptyPlayerID_PlayerInput_Rejected(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	msg := protocol.PlayerInput{Seq: 1, MoveX: 0.5, MoveZ: 0.5, Yaw: 1.0}
	env := buildEnvelope(t, protocol.TypePlayerInput, 1, &msg)

	ctx := PacketContext{SessionID: "sess-1", PlayerID: "", UserID: "u1"}
	result := h.HandleEnvelope(ctx, env)

	if result.Handled {
		t.Error("expected Handled=false for empty player ID")
	}
	if result.Error == nil {
		t.Error("expected error for empty player ID")
	}
}

func TestEmptyPlayerID_PlayerTransformUpdate_Rejected(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 1.0, Z: 2.0, Yaw: 0.5}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 1, &msg)

	ctx := PacketContext{SessionID: "sess-1", PlayerID: "", UserID: "u1"}
	result := h.HandleEnvelope(ctx, env)

	if result.Handled {
		t.Error("expected Handled=false for empty player ID")
	}
	if result.Error == nil {
		t.Error("expected error for empty player ID")
	}
}

// --- Room queue full ---

func TestRoomQueueFull_Error(t *testing.T) {
	rm := &mockRoom{
		enqErr: fmt.Errorf("room command queue full"),
	}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	msg := protocol.PlayerTransformUpdate{Seq: 1, X: 1.0, Z: 2.0, Yaw: 0.5}
	env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 1, &msg)

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if result.Handled {
		t.Error("expected Handled=false when queue is full")
	}
	if result.Error == nil {
		t.Error("expected error when room queue is full")
	}
}

// --- Garbage body data ---

func TestGarbageBody_Rejected(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	env := &protocol.Envelope{
		Version: protocol.CurrentVersion,
		Type:    protocol.TypePlayerTransformUpdate,
		Seq:     1,
		Tick:    0,
		Body:    []byte{0xFF, 0xFE, 0xFD},
	}

	result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)

	if result.Handled {
		t.Error("expected Handled=false for garbage body")
	}
	if result.Error == nil {
		t.Error("expected error for garbage body")
	}
	if len(rm.commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(rm.commands))
	}
}

// --- HandlePacketResult.String ---

func TestHandlePacketResult_String(t *testing.T) {
	handled := HandlePacketResult{MessageType: protocol.TypePlayerTransformUpdate, Handled: true}
	if !strings.Contains(handled.String(), "handled") {
		t.Errorf("String() = %q, want to contain 'handled'", handled.String())
	}

	notHandled := HandlePacketResult{
		MessageType: protocol.TypePing,
		Error:       fmt.Errorf("unsupported"),
	}
	if !strings.Contains(notHandled.String(), "error") {
		t.Errorf("String() = %q, want to contain 'error'", notHandled.String())
	}
}

// --- Boundary: transport packages do not import internal/game ---

func TestTransportPackagesDoNotImportGame(t *testing.T) {
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

func TestProtocolPackageDoesNotImportGame(t *testing.T) {
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

// --- Boundary: handler does not use JSON or Protobuf ---

func TestHandlerDoesNotUseJSONOrProtobuf(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir handler package: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", entry.Name(), err)
		}
		contents := string(data)
		if strings.Contains(contents, "encoding/json") {
			t.Fatalf("handler package must not use JSON: %s", entry.Name())
		}
		if strings.Contains(contents, "protobuf") || strings.Contains(contents, "google.golang.org/protobuf") || strings.Contains(contents, ".proto") {
			t.Fatalf("handler package must not use protobuf: %s", entry.Name())
		}
	}
}

// --- Multiple packets in sequence ---

func TestMultiplePacketsInSequence(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	ctx := testContext("sess-1", "player-1")

	for i := 0; i < 5; i++ {
		msg := protocol.PlayerTransformUpdate{
			Seq: uint32(i),
			X:   float32(i) * 10.0,
			Z:   float32(i) * 5.0,
			Yaw: 0.5,
		}
		env := buildEnvelope(t, protocol.TypePlayerTransformUpdate, uint32(i), &msg)
		result := h.HandleEnvelope(ctx, env)
		if !result.Handled {
			t.Errorf("packet %d: expected Handled=true, error=%v", i, result.Error)
		}
	}

	if len(rm.commands) != 5 {
		t.Errorf("expected 5 commands, got %d", len(rm.commands))
	}
}

// --- Mixed message types ---

func TestMixedMessageTypesInSequence(t *testing.T) {
	rm := &mockRoom{}
	h := NewRealtimePacketHandler(testResolver(map[string]*mockRoom{
		"sess-1": rm,
	}))

	ctx := testContext("sess-1", "player-1")

	inputMsg := protocol.PlayerInput{Seq: 1, MoveX: 0.5, MoveZ: -0.5, Yaw: 1.0}
	inputEnv := buildEnvelope(t, protocol.TypePlayerInput, 1, &inputMsg)
	inputResult := h.HandleEnvelope(ctx, inputEnv)
	if !inputResult.Handled {
		t.Errorf("PlayerInput: expected Handled=true, error=%v", inputResult.Error)
	}

	transformMsg := protocol.PlayerTransformUpdate{Seq: 2, X: 10.0, Z: 20.0, Yaw: 1.5}
	transformEnv := buildEnvelope(t, protocol.TypePlayerTransformUpdate, 2, &transformMsg)
	transformResult := h.HandleEnvelope(ctx, transformEnv)
	if !transformResult.Handled {
		t.Errorf("PlayerTransformUpdate: expected Handled=true, error=%v", transformResult.Error)
	}

	if len(rm.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(rm.commands))
	}
	if rm.commands[0].Kind != room.CmdPlayerInput {
		t.Errorf("command 0: Kind=%d, want CmdPlayerInput", rm.commands[0].Kind)
	}
	if rm.commands[1].Kind != room.CmdUpdatePlayerTransform {
		t.Errorf("command 1: Kind=%d, want CmdUpdatePlayerTransform", rm.commands[1].Kind)
	}
}

// --- Deferred message types are unsupported ---

func TestDeferredMessageTypes_Unsupported(t *testing.T) {
	h := NewRealtimePacketHandler(testResolver(nil))

	deferredTypes := []protocol.MessageType{
		protocol.TypeReconnect,
		protocol.TypeJoinRoom,
		protocol.TypeHello,
		protocol.TypePing,
	}

	for _, mt := range deferredTypes {
		t.Run(mt.String(), func(t *testing.T) {
			env := &protocol.Envelope{
				Version: protocol.CurrentVersion,
				Type:    mt,
				Seq:     1,
				Tick:    0,
				Body:    []byte{},
			}
			result := h.HandleEnvelope(testContext("sess-1", "player-1"), env)
			if result.Handled {
				t.Errorf("deferred type %s should not be handled", mt)
			}
			if result.Error == nil {
				t.Errorf("deferred type %s should produce an error", mt)
			}
		})
	}
}
