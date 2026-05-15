package protocol

// Client → Server messages.
// Only foundation messages are implemented.
// PlayerInput, Reconnect, and other game messages will be added in later milestones.

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

// Server → Client messages.
// Only foundation messages are implemented.
// FullSnapshot, PlayerDelta, ObjectDelta, VoiceGroupDelta, and lock messages
// will be added in later milestones.

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
	Tick            uint32 `msgpack:"tk"`
}

// ServerError is a structured error sent to the client.
type ServerError struct {
	Code    uint16 `msgpack:"code"`
	Message string `msgpack:"msg"`
}

// Pong is the server response to Ping.
type Pong struct {
	Timestamp int64 `msgpack:"ts"`
	ServerTick uint32 `msgpack:"tk"`
}
