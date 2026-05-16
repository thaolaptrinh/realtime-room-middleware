package protocol

import (
	"fmt"
	"math"
)

// Client → Server messages.
// Object, voice, reconnect, and leave messages remain deferred.

// Hello is the first message from client after KCP session opens.
type Hello struct {
	Version uint16 `msgpack:"v"`
}

// JoinRoom requests to join a specific room instance.
type JoinRoom struct {
	RoomInstanceID string `msgpack:"ri"`
	SessionToken   string `msgpack:"st"`
	UserID         string `msgpack:"uid"`
}

// Ping is a keep-alive probe from the client.
type Ping struct {
	Timestamp int64 `msgpack:"ts"`
}

// PlayerInput carries movement intent for Phase 1 player position sync.
type PlayerInput struct {
	Seq       uint32  `msgpack:"s"`
	MoveX     float32 `msgpack:"mx"`
	MoveZ     float32 `msgpack:"mz"`
	Yaw       float32 `msgpack:"y"`
	AnimState uint16  `msgpack:"a"`
}

// Validate checks finite movement and rotation values.
func (m PlayerInput) Validate() error {
	return validateFiniteFields("PlayerInput", map[string]float32{
		"MoveX": m.MoveX,
		"MoveZ": m.MoveZ,
		"Yaw":   m.Yaw,
	})
}

// PlayerTransformUpdate carries client-reported transform state for Phase 1.
type PlayerTransformUpdate struct {
	Seq       uint32  `msgpack:"s"`
	X         float32 `msgpack:"x"`
	Z         float32 `msgpack:"z"`
	Yaw       float32 `msgpack:"y"`
	AnimState uint16  `msgpack:"a"`
}

// Validate checks finite position and rotation values.
func (m PlayerTransformUpdate) Validate() error {
	return validateFiniteTransform("PlayerTransformUpdate", m.X, m.Z, m.Yaw)
}

// Server → Client messages.
// Object, voice, and lock messages remain deferred.

// Welcome is the server response to Hello.
type Welcome struct {
	Version   uint16 `msgpack:"v"`
	ServerID  string `msgpack:"sid"`
	Timestamp int64  `msgpack:"ts"`
}

// JoinAccepted confirms room join and provides initial room info.
type JoinAccepted struct {
	RoomInstanceID string `msgpack:"ri"`
	LogicalRoomID  string `msgpack:"li"`
	PlayerID       string `msgpack:"pid"`
	Tick           uint32 `msgpack:"tk"`
}

// ServerError is a structured error sent to the client.
type ServerError struct {
	Code    uint16 `msgpack:"code"`
	Message string `msgpack:"msg"`
}

// Pong is the server response to Ping.
type Pong struct {
	Timestamp  int64  `msgpack:"ts"`
	ServerTick uint32 `msgpack:"tk"`
}

// FullSnapshot is the full visible player state sent on join or resync.
type FullSnapshot struct {
	Tick    uint32           `msgpack:"tk"`
	Players []PlayerSnapshot `msgpack:"pl"`
}

// Validate checks all contained player snapshots.
func (m FullSnapshot) Validate() error {
	for i := range m.Players {
		if err := m.Players[i].Validate(); err != nil {
			return fmt.Errorf("FullSnapshot.Players[%d]: %w", i, err)
		}
	}
	return nil
}

// PlayerSnapshot is a wire-level player transform snapshot.
type PlayerSnapshot struct {
	PlayerID  string  `msgpack:"pid"`
	X         float32 `msgpack:"x"`
	Z         float32 `msgpack:"z"`
	Yaw       float32 `msgpack:"y"`
	AnimState uint16  `msgpack:"a"`
	Version   uint32  `msgpack:"v"`
}

// Validate checks required ID and finite transform fields.
func (m PlayerSnapshot) Validate() error {
	if m.PlayerID == "" {
		return fmt.Errorf("PlayerSnapshot.PlayerID is required")
	}
	return validateFiniteTransform("PlayerSnapshot", m.X, m.Z, m.Yaw)
}

// PlayerDelta aggregates player visibility changes for one viewer at one tick.
type PlayerDelta struct {
	Tick    uint32              `msgpack:"tk"`
	Enters  []PlayerEnterDelta  `msgpack:"en"`
	Updates []PlayerUpdateDelta `msgpack:"up"`
	Leaves  []PlayerLeaveDelta  `msgpack:"lv"`
}

// IsEmpty reports whether the delta has no visible changes.
func (m PlayerDelta) IsEmpty() bool {
	return len(m.Enters) == 0 && len(m.Updates) == 0 && len(m.Leaves) == 0
}

// Validate checks all contained delta entries.
func (m PlayerDelta) Validate() error {
	if m.IsEmpty() {
		return fmt.Errorf("PlayerDelta must not be empty")
	}
	for i := range m.Enters {
		if err := m.Enters[i].Validate(); err != nil {
			return fmt.Errorf("PlayerDelta.Enters[%d]: %w", i, err)
		}
	}
	for i := range m.Updates {
		if err := m.Updates[i].Validate(); err != nil {
			return fmt.Errorf("PlayerDelta.Updates[%d]: %w", i, err)
		}
	}
	for i := range m.Leaves {
		if err := m.Leaves[i].Validate(); err != nil {
			return fmt.Errorf("PlayerDelta.Leaves[%d]: %w", i, err)
		}
	}
	return nil
}

// PlayerEnterDelta is emitted when a player becomes visible to a viewer.
type PlayerEnterDelta struct {
	PlayerID  string  `msgpack:"pid"`
	X         float32 `msgpack:"x"`
	Z         float32 `msgpack:"z"`
	Yaw       float32 `msgpack:"y"`
	AnimState uint16  `msgpack:"a"`
	Version   uint32  `msgpack:"v"`
}

// Validate checks required ID and finite transform fields.
func (m PlayerEnterDelta) Validate() error {
	if m.PlayerID == "" {
		return fmt.Errorf("PlayerEnterDelta.PlayerID is required")
	}
	return validateFiniteTransform("PlayerEnterDelta", m.X, m.Z, m.Yaw)
}

// PlayerUpdateDelta is emitted when a visible player's transform changes.
type PlayerUpdateDelta struct {
	PlayerID  string  `msgpack:"pid"`
	X         float32 `msgpack:"x"`
	Z         float32 `msgpack:"z"`
	Yaw       float32 `msgpack:"y"`
	AnimState uint16  `msgpack:"a"`
	Version   uint32  `msgpack:"v"`
}

// Validate checks required ID and finite transform fields.
func (m PlayerUpdateDelta) Validate() error {
	if m.PlayerID == "" {
		return fmt.Errorf("PlayerUpdateDelta.PlayerID is required")
	}
	return validateFiniteTransform("PlayerUpdateDelta", m.X, m.Z, m.Yaw)
}

// PlayerLeaveDelta is emitted when a player leaves a viewer's interest set.
type PlayerLeaveDelta struct {
	PlayerID string `msgpack:"pid"`
}

// Validate checks required ID fields.
func (m PlayerLeaveDelta) Validate() error {
	if m.PlayerID == "" {
		return fmt.Errorf("PlayerLeaveDelta.PlayerID is required")
	}
	return nil
}

func validateFiniteTransform(owner string, x float32, z float32, yaw float32) error {
	return validateFiniteFields(owner, map[string]float32{
		"X":   x,
		"Z":   z,
		"Yaw": yaw,
	})
}

func validateFiniteFields(owner string, fields map[string]float32) error {
	for name, value := range fields {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("%s.%s must be finite", owner, name)
		}
	}
	return nil
}
