package protocol

import "fmt"

const (
	// CurrentVersion is the current protocol version.
	// Breaking wire-format changes require a major bump.
	CurrentVersion uint16 = 1

	// MinVersion is the minimum protocol version the server accepts.
	MinVersion uint16 = 1

	// MaxVersion is the maximum protocol version the server accepts.
	MaxVersion uint16 = 1
)

const (
	// MaxPacketSize is the maximum allowed size for a complete encoded packet.
	// 64 KB gives headroom for FullSnapshot with 200 CCU without risking
	// memory abuse. Tunable after load testing.
	MaxPacketSize int = 64 * 1024

	// MaxPayloadSize is the maximum allowed size for the envelope body.
	MaxPayloadSize int = 60 * 1024
)

// MessageType identifies the kind of message inside an Envelope.
type MessageType uint16

const (
	// Client → Server (1-99)
	TypeHello       MessageType = 1
	TypeJoinRoom    MessageType = 2
	TypeReconnect   MessageType = 3
	TypePlayerInput MessageType = 4
	TypePing        MessageType = 5
	// TypePlayerTransformUpdate carries client-reported transform state for Phase 1 position sync.
	TypePlayerTransformUpdate MessageType = 6

	// Server → Client
	TypeWelcome           MessageType = 1001
	TypeJoinAccepted      MessageType = 1002
	TypeReconnectAccepted MessageType = 1003
	TypeReconnectRejected MessageType = 1004
	TypeFullSnapshot      MessageType = 1005
	TypePlayerDelta       MessageType = 1006
	TypeObjectDelta       MessageType = 1007
	TypeVoiceGroupDelta   MessageType = 1008
	TypeLockAccepted      MessageType = 1009
	TypeLockRejected      MessageType = 1010
	// TypeClusterMembershipDelta is reserved for optional Phase 1 diagnostics.
	TypeClusterMembershipDelta MessageType = 1011
	TypeError                  MessageType = 1100
	TypePong                   MessageType = 1101
)

// IsClientToServer reports whether a message type originates from the client.
func (mt MessageType) IsClientToServer() bool {
	return mt >= 1 && mt <= 99
}

// IsServerToClient reports whether a message type originates from the server.
func (mt MessageType) IsServerToClient() bool {
	return mt >= 1000 && mt <= 1999
}

// String returns a human-readable name for the message type.
func (mt MessageType) String() string {
	switch mt {
	case TypeHello:
		return "Hello"
	case TypeJoinRoom:
		return "JoinRoom"
	case TypeReconnect:
		return "Reconnect"
	case TypePlayerInput:
		return "PlayerInput"
	case TypePing:
		return "Ping"
	case TypePlayerTransformUpdate:
		return "PlayerTransformUpdate"
	case TypeWelcome:
		return "Welcome"
	case TypeJoinAccepted:
		return "JoinAccepted"
	case TypeReconnectAccepted:
		return "ReconnectAccepted"
	case TypeReconnectRejected:
		return "ReconnectRejected"
	case TypeFullSnapshot:
		return "FullSnapshot"
	case TypePlayerDelta:
		return "PlayerDelta"
	case TypeObjectDelta:
		return "ObjectDelta"
	case TypeVoiceGroupDelta:
		return "VoiceGroupDelta"
	case TypeLockAccepted:
		return "LockAccepted"
	case TypeLockRejected:
		return "LockRejected"
	case TypeClusterMembershipDelta:
		return "ClusterMembershipDelta"
	case TypeError:
		return "Error"
	case TypePong:
		return "Pong"
	default:
		return fmt.Sprintf("Unknown(%d)", mt)
	}
}

// IsKnown reports whether a message type has a stable Protocol v1 ID.
func (mt MessageType) IsKnown() bool {
	switch mt {
	case TypeHello,
		TypeJoinRoom,
		TypeReconnect,
		TypePlayerInput,
		TypePing,
		TypePlayerTransformUpdate,
		TypeWelcome,
		TypeJoinAccepted,
		TypeReconnectAccepted,
		TypeReconnectRejected,
		TypeFullSnapshot,
		TypePlayerDelta,
		TypeObjectDelta,
		TypeVoiceGroupDelta,
		TypeLockAccepted,
		TypeLockRejected,
		TypeClusterMembershipDelta,
		TypeError,
		TypePong:
		return true
	default:
		return false
	}
}

// IsImplemented reports whether a message type has a Protocol v1 wire struct in this package.
func (mt MessageType) IsImplemented() bool {
	switch mt {
	case TypeHello,
		TypeJoinRoom,
		TypePlayerInput,
		TypePing,
		TypePlayerTransformUpdate,
		TypeWelcome,
		TypeJoinAccepted,
		TypeFullSnapshot,
		TypePlayerDelta,
		TypeError,
		TypePong:
		return true
	default:
		return false
	}
}

// Envelope is the wire-level wrapper for every packet.
type Envelope struct {
	Version uint16      `msgpack:"v"`
	Type    MessageType `msgpack:"t"`
	Seq     uint32      `msgpack:"s"`
	Tick    uint32      `msgpack:"k"`
	Body    []byte      `msgpack:"b"`
}

// ProtocolError represents a protocol-level error returned to the client.
type ProtocolError struct {
	Code    uint16 `msgpack:"code"`
	Message string `msgpack:"msg"`
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error %d: %s", e.Code, e.Message)
}

// Error codes returned in TypeError messages.
const (
	ErrCodeInvalidVersion  uint16 = 1
	ErrCodeInvalidType     uint16 = 2
	ErrCodeAuthFailed      uint16 = 3
	ErrCodeRoomFull        uint16 = 4
	ErrCodeRoomNotFound    uint16 = 5
	ErrCodePayloadTooLarge uint16 = 6
	ErrCodeInternal        uint16 = 99
)

// ValidateVersion checks that a protocol version is within the supported range.
func ValidateVersion(v uint16) error {
	if v < MinVersion || v > MaxVersion {
		return fmt.Errorf("unsupported protocol version %d: supported range is [%d, %d]",
			v, MinVersion, MaxVersion)
	}
	return nil
}

// ValidateMessageType checks that a message type is implemented in Protocol v1.
func ValidateMessageType(mt MessageType) error {
	if !mt.IsImplemented() {
		return fmt.Errorf("unimplemented or unknown message type %d", mt)
	}
	return nil
}

// ValidatePacketSize checks that raw packet bytes are within the allowed limit.
func ValidatePacketSize(data []byte) error {
	if len(data) > MaxPacketSize {
		return fmt.Errorf("packet size %d exceeds maximum %d bytes", len(data), MaxPacketSize)
	}
	return nil
}

// ValidatePayloadSize checks that an envelope body is within the allowed limit.
func ValidatePayloadSize(body []byte) error {
	if len(body) > MaxPayloadSize {
		return fmt.Errorf("payload size %d exceeds maximum %d bytes", len(body), MaxPayloadSize)
	}
	return nil
}
