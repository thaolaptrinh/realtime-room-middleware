package protocol

import (
	"bytes"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Envelope encode/decode roundtrip ---

func TestEncodeDecodeEnvelope(t *testing.T) {
	body := []byte("test payload")
	env := &Envelope{
		Version: CurrentVersion,
		Type:    TypePing,
		Seq:     42,
		Tick:    100,
		Body:    body,
	}

	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	got, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}

	if got.Version != env.Version {
		t.Errorf("Version: got %d, want %d", got.Version, env.Version)
	}
	if got.Type != env.Type {
		t.Errorf("Type: got %d, want %d", got.Type, env.Type)
	}
	if got.Seq != env.Seq {
		t.Errorf("Seq: got %d, want %d", got.Seq, env.Seq)
	}
	if got.Tick != env.Tick {
		t.Errorf("Tick: got %d, want %d", got.Tick, env.Tick)
	}
	if !bytes.Equal(got.Body, env.Body) {
		t.Errorf("Body: got %x, want %x", got.Body, env.Body)
	}
}

func TestEncodeDecodeEnvelopeZeroFields(t *testing.T) {
	env := &Envelope{
		Version: CurrentVersion,
		Type:    TypePing,
		Seq:     0,
		Tick:    0,
		Body:    nil,
	}

	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	got, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}

	if got.Version != CurrentVersion {
		t.Errorf("Version: got %d, want %d", got.Version, CurrentVersion)
	}
	if got.Type != TypePing {
		t.Errorf("Type: got %d, want %d", got.Type, TypePing)
	}
}

// --- Version validation ---

func TestRejectUnsupportedVersionLow(t *testing.T) {
	env := &Envelope{
		Version: 0,
		Type:    TypePing,
		Seq:     1,
		Tick:    1,
		Body:    []byte("x"),
	}
	_, err := EncodeEnvelope(env)
	if err == nil {
		t.Fatal("expected error for version 0")
	}
}

func TestRejectUnsupportedVersionHigh(t *testing.T) {
	env := &Envelope{
		Version: 99,
		Type:    TypePing,
		Seq:     1,
		Tick:    1,
		Body:    []byte("x"),
	}
	_, err := EncodeEnvelope(env)
	if err == nil {
		t.Fatal("expected error for version 99")
	}
}

func TestRejectDecodeUnsupportedVersion(t *testing.T) {
	env := &Envelope{
		Version: 0,
		Type:    TypePing,
		Seq:     1,
		Tick:    1,
		Body:    []byte("x"),
	}
	// Bypass EncodeEnvelope validation by encoding raw.
	data, err := encodeRawEnvelope(env)
	if err != nil {
		t.Fatalf("raw encode: %v", err)
	}
	_, err = DecodeEnvelope(data)
	if err == nil {
		t.Fatal("expected error for decoding version 0")
	}
}

func TestValidateVersionCurrent(t *testing.T) {
	if err := ValidateVersion(CurrentVersion); err != nil {
		t.Errorf("CurrentVersion %d should be valid: %v", CurrentVersion, err)
	}
}

func TestValidateVersionBounds(t *testing.T) {
	cases := []struct {
		v  uint16
		ok bool
	}{
		{MinVersion - 1, false},
		{MinVersion, true},
		{MaxVersion, true},
		{MaxVersion + 1, false},
	}
	for _, tc := range cases {
		err := ValidateVersion(tc.v)
		if (err == nil) != tc.ok {
			t.Errorf("ValidateVersion(%d): ok=%v, want ok=%v", tc.v, err == nil, tc.ok)
		}
	}
}

// --- Packet/payload size validation ---

func TestRejectOversizedPayload(t *testing.T) {
	body := make([]byte, MaxPayloadSize+1)
	env := &Envelope{
		Version: CurrentVersion,
		Type:    TypePing,
		Seq:     1,
		Tick:    1,
		Body:    body,
	}
	_, err := EncodeEnvelope(env)
	if err == nil {
		t.Fatal("expected error for oversized payload")
	}
}

func TestAcceptMaxPayloadSize(t *testing.T) {
	body := make([]byte, MaxPayloadSize)
	env := &Envelope{
		Version: CurrentVersion,
		Type:    TypePing,
		Seq:     1,
		Tick:    1,
		Body:    body,
	}
	_, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("max payload should be accepted: %v", err)
	}
}

func TestValidatePacketSize(t *testing.T) {
	data := make([]byte, MaxPacketSize+1)
	if err := ValidatePacketSize(data); err == nil {
		t.Fatal("expected error for oversized packet")
	}
	data = make([]byte, MaxPacketSize)
	if err := ValidatePacketSize(data); err != nil {
		t.Fatalf("max packet should be accepted: %v", err)
	}
}

// --- MessageType helpers ---

func TestMessageTypeDirection(t *testing.T) {
	if !TypeHello.IsClientToServer() {
		t.Error("Hello should be client-to-server")
	}
	if !TypePing.IsClientToServer() {
		t.Error("Ping should be client-to-server")
	}
	if !TypePlayerInput.IsClientToServer() {
		t.Error("PlayerInput should be client-to-server")
	}
	if !TypePlayerTransformUpdate.IsClientToServer() {
		t.Error("PlayerTransformUpdate should be client-to-server")
	}
	if TypeHello.IsServerToClient() {
		t.Error("Hello should not be server-to-client")
	}

	if !TypeWelcome.IsServerToClient() {
		t.Error("Welcome should be server-to-client")
	}
	if !TypePong.IsServerToClient() {
		t.Error("Pong should be server-to-client")
	}
	if !TypeFullSnapshot.IsServerToClient() {
		t.Error("FullSnapshot should be server-to-client")
	}
	if !TypePlayerDelta.IsServerToClient() {
		t.Error("PlayerDelta should be server-to-client")
	}
	if !TypeClusterMembershipDelta.IsServerToClient() {
		t.Error("ClusterMembershipDelta should be server-to-client")
	}
	if TypeWelcome.IsClientToServer() {
		t.Error("Welcome should not be client-to-server")
	}
}

func TestRejectInvalidMessageType(t *testing.T) {
	msg := Ping{Timestamp: 1700000000}
	_, err := EncodeAndWrap(CurrentVersion, MessageType(9999), 1, 0, &msg)
	if err == nil {
		t.Fatal("expected error for unknown message type")
	}
}

func TestRejectDecodeInvalidMessageType(t *testing.T) {
	env := &Envelope{
		Version: CurrentVersion,
		Type:    MessageType(9999),
		Seq:     1,
		Tick:    1,
		Body:    []byte("x"),
	}
	data, err := encodeRawEnvelope(env)
	if err != nil {
		t.Fatalf("raw encode: %v", err)
	}
	_, err = DecodeEnvelope(data)
	if err == nil {
		t.Fatal("expected error for decoding unknown message type")
	}
}

func TestRejectDeferredMessageTypes(t *testing.T) {
	deferred := []MessageType{
		TypeReconnect,
		TypeReconnectAccepted,
		TypeReconnectRejected,
		TypeObjectDelta,
		TypeVoiceGroupDelta,
		TypeLockAccepted,
		TypeLockRejected,
		TypeClusterMembershipDelta,
	}
	for _, mt := range deferred {
		if err := ValidateMessageType(mt); err == nil {
			t.Fatalf("expected deferred message type %s to be rejected", mt)
		}
	}
}

func TestPhase1MessageTypeIDs(t *testing.T) {
	cases := []struct {
		name string
		got  MessageType
		want MessageType
	}{
		{name: "PlayerInput", got: TypePlayerInput, want: 4},
		{name: "PlayerTransformUpdate", got: TypePlayerTransformUpdate, want: 6},
		{name: "FullSnapshot", got: TypeFullSnapshot, want: 1005},
		{name: "PlayerDelta", got: TypePlayerDelta, want: 1006},
		{name: "ClusterMembershipDelta", got: TypeClusterMembershipDelta, want: 1011},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s type ID = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestMessageTypeString(t *testing.T) {
	cases := []struct {
		mt   MessageType
		want string
	}{
		{TypeHello, "Hello"},
		{TypeJoinRoom, "JoinRoom"},
		{TypePlayerInput, "PlayerInput"},
		{TypePing, "Ping"},
		{TypePlayerTransformUpdate, "PlayerTransformUpdate"},
		{TypeWelcome, "Welcome"},
		{TypeJoinAccepted, "JoinAccepted"},
		{TypeFullSnapshot, "FullSnapshot"},
		{TypePlayerDelta, "PlayerDelta"},
		{TypeClusterMembershipDelta, "ClusterMembershipDelta"},
		{TypeError, "Error"},
		{TypePong, "Pong"},
		{MessageType(9999), "Unknown(9999)"},
	}
	for _, tc := range cases {
		got := tc.mt.String()
		if got != tc.want {
			t.Errorf("MessageType(%d).String() = %q, want %q", tc.mt, got, tc.want)
		}
	}
}

// --- Phase 1 position cluster sync messages ---

func TestPlayerInputRoundtrip(t *testing.T) {
	msg := PlayerInput{Seq: 101, MoveX: 0.25, MoveZ: -0.75, Yaw: 1.5, AnimState: 3}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerInput: %v", err)
	}

	var got PlayerInput
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerInput: %v", err)
	}
	if got != msg {
		t.Errorf("PlayerInput: got %+v, want %+v", got, msg)
	}
}

func TestPlayerTransformUpdateRoundtrip(t *testing.T) {
	msg := PlayerTransformUpdate{Seq: 102, X: 12.5, Z: -4.25, Yaw: 2.25, AnimState: 7}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerTransformUpdate: %v", err)
	}

	var got PlayerTransformUpdate
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerTransformUpdate: %v", err)
	}
	if got != msg {
		t.Errorf("PlayerTransformUpdate: got %+v, want %+v", got, msg)
	}
}

func TestPlayerSnapshotRoundtrip(t *testing.T) {
	msg := PlayerSnapshot{PlayerID: "player_42", X: 1.25, Z: 2.5, Yaw: 0.75, AnimState: 5, Version: 9}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerSnapshot: %v", err)
	}

	var got PlayerSnapshot
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerSnapshot: %v", err)
	}
	if got != msg {
		t.Errorf("PlayerSnapshot: got %+v, want %+v", got, msg)
	}
}

func TestFullSnapshotRoundtrip(t *testing.T) {
	msg := FullSnapshot{
		Tick: 77,
		Players: []PlayerSnapshot{
			{PlayerID: "p1", X: 1, Z: 2, Yaw: 0.1, AnimState: 1, Version: 10},
			{PlayerID: "p2", X: 3, Z: 4, Yaw: 0.2, AnimState: 2, Version: 11},
		},
	}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage FullSnapshot: %v", err)
	}

	var got FullSnapshot
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage FullSnapshot: %v", err)
	}
	if got.Tick != msg.Tick || len(got.Players) != len(msg.Players) || got.Players[0] != msg.Players[0] || got.Players[1] != msg.Players[1] {
		t.Errorf("FullSnapshot: got %+v, want %+v", got, msg)
	}
}

func TestPlayerDeltaRoundtrip(t *testing.T) {
	msg := PlayerDelta{
		Tick: 88,
		Enters: []PlayerEnterDelta{
			{PlayerID: "p1", X: 1, Z: 2, Yaw: 0.1, AnimState: 1, Version: 10},
		},
		Updates: []PlayerUpdateDelta{
			{PlayerID: "p2", X: 3, Z: 4, Yaw: 0.2, AnimState: 2, Version: 11},
		},
		Leaves: []PlayerLeaveDelta{
			{PlayerID: "p3"},
		},
	}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerDelta: %v", err)
	}

	var got PlayerDelta
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerDelta: %v", err)
	}
	if got.Tick != msg.Tick || got.Enters[0] != msg.Enters[0] || got.Updates[0] != msg.Updates[0] || got.Leaves[0] != msg.Leaves[0] {
		t.Errorf("PlayerDelta: got %+v, want %+v", got, msg)
	}
}

func TestPlayerEnterDeltaRoundtrip(t *testing.T) {
	msg := PlayerEnterDelta{PlayerID: "p1", X: 1, Z: 2, Yaw: 0.1, AnimState: 1, Version: 10}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerEnterDelta: %v", err)
	}

	var got PlayerEnterDelta
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerEnterDelta: %v", err)
	}
	if got != msg {
		t.Errorf("PlayerEnterDelta: got %+v, want %+v", got, msg)
	}
}

func TestPlayerUpdateDeltaRoundtrip(t *testing.T) {
	msg := PlayerUpdateDelta{PlayerID: "p2", X: 3, Z: 4, Yaw: 0.2, AnimState: 2, Version: 11}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerUpdateDelta: %v", err)
	}

	var got PlayerUpdateDelta
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerUpdateDelta: %v", err)
	}
	if got != msg {
		t.Errorf("PlayerUpdateDelta: got %+v, want %+v", got, msg)
	}
}

func TestPlayerLeaveDeltaRoundtrip(t *testing.T) {
	msg := PlayerLeaveDelta{PlayerID: "p3"}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage PlayerLeaveDelta: %v", err)
	}

	var got PlayerLeaveDelta
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage PlayerLeaveDelta: %v", err)
	}
	if got != msg {
		t.Errorf("PlayerLeaveDelta: got %+v, want %+v", got, msg)
	}
}

func TestRejectInvalidTransformValues(t *testing.T) {
	cases := []struct {
		name string
		msg  interface{}
	}{
		{name: "player input NaN", msg: &PlayerInput{Seq: 1, MoveX: float32(math.NaN()), MoveZ: 0, Yaw: 0}},
		{name: "transform update Inf", msg: &PlayerTransformUpdate{Seq: 1, X: float32(math.Inf(1)), Z: 0, Yaw: 0}},
		{name: "snapshot NaN", msg: &PlayerSnapshot{PlayerID: "p1", X: 0, Z: 0, Yaw: float32(math.NaN()), Version: 1}},
		{name: "enter Inf", msg: &PlayerEnterDelta{PlayerID: "p1", X: 0, Z: float32(math.Inf(-1)), Yaw: 0, Version: 1}},
		{name: "update NaN", msg: &PlayerUpdateDelta{PlayerID: "p1", X: 0, Z: 0, Yaw: float32(math.NaN()), Version: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EncodeMessage(tc.msg)
			if err == nil {
				t.Fatal("expected invalid transform value to be rejected")
			}
		})
	}
}

func TestRejectInvalidTransformValuesForValueMessages(t *testing.T) {
	_, err := EncodeMessage(PlayerTransformUpdate{Seq: 1, X: float32(math.NaN()), Z: 0, Yaw: 0})
	if err == nil {
		t.Fatal("expected value-form message validation to reject invalid transform")
	}
}

func TestRejectMissingRequiredPlayerIDs(t *testing.T) {
	cases := []struct {
		name string
		msg  interface{}
	}{
		{name: "snapshot", msg: &PlayerSnapshot{X: 1, Z: 2, Yaw: 0, Version: 1}},
		{name: "enter", msg: &PlayerEnterDelta{X: 1, Z: 2, Yaw: 0, Version: 1}},
		{name: "update", msg: &PlayerUpdateDelta{X: 1, Z: 2, Yaw: 0, Version: 1}},
		{name: "leave", msg: &PlayerLeaveDelta{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := EncodeMessage(tc.msg)
			if err == nil {
				t.Fatal("expected missing player ID to be rejected")
			}
		})
	}
}

func TestPlayerDeltaIsEmpty(t *testing.T) {
	if !((&PlayerDelta{}).IsEmpty()) {
		t.Fatal("zero PlayerDelta should be empty")
	}
	if (&PlayerDelta{Updates: []PlayerUpdateDelta{{PlayerID: "p1", Version: 1}}}).IsEmpty() {
		t.Fatal("PlayerDelta with an update should not be empty")
	}
}

func TestRejectEmptyPlayerDelta(t *testing.T) {
	_, err := EncodeMessage(&PlayerDelta{})
	if err == nil {
		t.Fatal("expected empty PlayerDelta to be rejected")
	}
}

// --- Hello message ---

func TestHelloRoundtrip(t *testing.T) {
	msg := Hello{Version: CurrentVersion}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage Hello: %v", err)
	}

	var got Hello
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage Hello: %v", err)
	}
	if got.Version != msg.Version {
		t.Errorf("Hello.Version: got %d, want %d", got.Version, msg.Version)
	}
}

// --- JoinRoom message ---

func TestJoinRoomRoundtrip(t *testing.T) {
	msg := JoinRoom{
		RoomInstanceID: "room-expo-a-1",
		SessionToken:   "tok_abc123",
		UserID:         "user_42",
	}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage JoinRoom: %v", err)
	}

	var got JoinRoom
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage JoinRoom: %v", err)
	}
	if got.RoomInstanceID != msg.RoomInstanceID {
		t.Errorf("RoomInstanceID: got %q, want %q", got.RoomInstanceID, msg.RoomInstanceID)
	}
	if got.SessionToken != msg.SessionToken {
		t.Errorf("SessionToken: got %q, want %q", got.SessionToken, msg.SessionToken)
	}
	if got.UserID != msg.UserID {
		t.Errorf("UserID: got %q, want %q", got.UserID, msg.UserID)
	}
}

// --- Ping/Pong ---

func TestPingRoundtrip(t *testing.T) {
	msg := Ping{Timestamp: 1700000000}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage Ping: %v", err)
	}

	var got Ping
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage Ping: %v", err)
	}
	if got.Timestamp != msg.Timestamp {
		t.Errorf("Ping.Timestamp: got %d, want %d", got.Timestamp, msg.Timestamp)
	}
}

func TestPongRoundtrip(t *testing.T) {
	msg := Pong{Timestamp: 1700000000, ServerTick: 500}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage Pong: %v", err)
	}

	var got Pong
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage Pong: %v", err)
	}
	if got.Timestamp != msg.Timestamp {
		t.Errorf("Pong.Timestamp: got %d, want %d", got.Timestamp, msg.Timestamp)
	}
	if got.ServerTick != msg.ServerTick {
		t.Errorf("Pong.ServerTick: got %d, want %d", got.ServerTick, msg.ServerTick)
	}
}

// --- Welcome message ---

func TestWelcomeRoundtrip(t *testing.T) {
	msg := Welcome{
		Version:   CurrentVersion,
		ServerID:  "srv-01",
		Timestamp: 1700000000,
	}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage Welcome: %v", err)
	}

	var got Welcome
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage Welcome: %v", err)
	}
	if got.Version != msg.Version {
		t.Errorf("Welcome.Version: got %d, want %d", got.Version, msg.Version)
	}
	if got.ServerID != msg.ServerID {
		t.Errorf("Welcome.ServerID: got %q, want %q", got.ServerID, msg.ServerID)
	}
}

// --- JoinAccepted message ---

func TestJoinAcceptedRoundtrip(t *testing.T) {
	msg := JoinAccepted{
		RoomInstanceID: "room-expo-a-1",
		LogicalRoomID:  "expo-room-a",
		PlayerID:       "player_42",
		Tick:           1234,
	}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage JoinAccepted: %v", err)
	}

	var got JoinAccepted
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage JoinAccepted: %v", err)
	}
	if got.RoomInstanceID != msg.RoomInstanceID {
		t.Errorf("RoomInstanceID: got %q, want %q", got.RoomInstanceID, msg.RoomInstanceID)
	}
	if got.LogicalRoomID != msg.LogicalRoomID {
		t.Errorf("LogicalRoomID: got %q, want %q", got.LogicalRoomID, msg.LogicalRoomID)
	}
	if got.PlayerID != msg.PlayerID {
		t.Errorf("PlayerID: got %q, want %q", got.PlayerID, msg.PlayerID)
	}
	if got.Tick != msg.Tick {
		t.Errorf("Tick: got %d, want %d", got.Tick, msg.Tick)
	}
}

// --- Error message ---

func TestServerErrorRoundtrip(t *testing.T) {
	msg := ServerError{
		Code:    ErrCodeRoomFull,
		Message: "room is full",
	}
	body, err := EncodeMessage(&msg)
	if err != nil {
		t.Fatalf("EncodeMessage ServerError: %v", err)
	}

	var got ServerError
	if err := DecodeMessage(body, &got); err != nil {
		t.Fatalf("DecodeMessage ServerError: %v", err)
	}
	if got.Code != msg.Code {
		t.Errorf("ServerError.Code: got %d, want %d", got.Code, msg.Code)
	}
	if got.Message != msg.Message {
		t.Errorf("ServerError.Message: got %q, want %q", got.Message, msg.Message)
	}
}

func TestProtocolErrorImplementsError(t *testing.T) {
	err := &ProtocolError{Code: ErrCodeInternal, Message: "something broke"}
	_ = error(err) // compile-time check
	if err.Error() != "protocol error 99: something broke" {
		t.Errorf("Error() = %q, want %q", err.Error(), "protocol error 99: something broke")
	}
}

// --- Full flow: BuildEnvelope + DecodeAndUnwrap ---

func TestFullFlowHello(t *testing.T) {
	msg := Hello{Version: CurrentVersion}
	data, err := EncodeAndWrap(CurrentVersion, TypeHello, 1, 0, &msg)
	if err != nil {
		t.Fatalf("EncodeAndWrap: %v", err)
	}

	var got Hello
	env, err := DecodeAndUnwrap(data, &got)
	if err != nil {
		t.Fatalf("DecodeAndUnwrap: %v", err)
	}
	if env.Type != TypeHello {
		t.Errorf("Type: got %d, want %d", env.Type, TypeHello)
	}
	if env.Seq != 1 {
		t.Errorf("Seq: got %d, want 1", env.Seq)
	}
	if got.Version != CurrentVersion {
		t.Errorf("Hello.Version: got %d, want %d", got.Version, CurrentVersion)
	}
}

func TestFullFlowPingPong(t *testing.T) {
	ping := Ping{Timestamp: 1700000000}
	data, err := EncodeAndWrap(CurrentVersion, TypePing, 10, 500, &ping)
	if err != nil {
		t.Fatalf("EncodeAndWrap ping: %v", err)
	}

	env, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	if env.Type != TypePing {
		t.Errorf("Type: got %d, want %d", env.Type, TypePing)
	}
	if env.Tick != 500 {
		t.Errorf("Tick: got %d, want 500", env.Tick)
	}

	var gotPing Ping
	if err := DecodeMessage(env.Body, &gotPing); err != nil {
		t.Fatalf("DecodeMessage Ping: %v", err)
	}
	if gotPing.Timestamp != ping.Timestamp {
		t.Errorf("Ping.Timestamp: got %d, want %d", gotPing.Timestamp, ping.Timestamp)
	}
}

// --- Deterministic fixture test ---

func TestEnvelopeWireFormatDeterministic(t *testing.T) {
	env := &Envelope{
		Version: 1,
		Type:    TypePing,
		Seq:     1,
		Tick:    0,
		Body:    nil,
	}

	data1, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("encode 1: %v", err)
	}
	data2, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("encode 2: %v", err)
	}

	if !bytes.Equal(data1, data2) {
		t.Errorf("identical envelopes produced different wire bytes:\n%x\n%x", data1, data2)
	}
}

// --- Decode garbage data ---

func TestDecodeGarbage(t *testing.T) {
	_, err := DecodeEnvelope([]byte{0xFF, 0xFE, 0xFD})
	if err == nil {
		t.Fatal("expected error decoding garbage data")
	}
}

func TestDecodeEmptyData(t *testing.T) {
	// Empty input should produce an error: either msgpack decode fails,
	// or version validation rejects version=0.
	_, err := DecodeEnvelope([]byte{})
	if err == nil {
		t.Fatal("expected error decoding empty data")
	}
}

// --- ProtocolError codes ---

func TestProtocolErrorCodes(t *testing.T) {
	codes := []uint16{
		ErrCodeInvalidVersion,
		ErrCodeInvalidType,
		ErrCodeAuthFailed,
		ErrCodeRoomFull,
		ErrCodeRoomNotFound,
		ErrCodePayloadTooLarge,
		ErrCodeInternal,
	}
	seen := map[uint16]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Errorf("duplicate error code %d", c)
		}
		seen[c] = true
	}
}

func TestProtocolPackageDoesNotImportGamePackages(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir protocol package: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", entry.Name(), err)
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			if strings.Contains(path, "/internal/game") {
				t.Fatalf("protocol package must not import game packages: %s imports %s", entry.Name(), path)
			}
		}
	}
}

func TestProtocolPackageDoesNotUseJSONOrProtobuf(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir protocol package: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", entry.Name(), err)
		}
		contents := string(data)
		if strings.Contains(contents, "encoding/json") {
			t.Fatalf("protocol package must not use JSON realtime codecs: %s", entry.Name())
		}
		if strings.Contains(contents, "protobuf") || strings.Contains(contents, "google.golang.org/protobuf") || strings.Contains(contents, ".proto") {
			t.Fatalf("protocol package must not use protobuf realtime codecs: %s", entry.Name())
		}
	}
}

// --- encodeRawEnvelope bypasses validation for testing invalid data paths ---

func encodeRawEnvelope(env *Envelope) ([]byte, error) {
	return encodeRaw(env)
}
