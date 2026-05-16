package player

// Vector3 is a 3D position or direction with float32 components.
type Vector3 struct {
	X float32
	Y float32
	Z float32
}

// Quaternion represents a rotation as a unit quaternion (X, Y, Z, W).
// Unity uses quaternion representation natively.
type Quaternion struct {
	X float32
	Y float32
	Z float32
	W float32
}

// IdentityQuaternion is the no-rotation quaternion (W=1, all others 0).
var IdentityQuaternion = Quaternion{W: 1}

// PlayerTransform holds the position and rotation of a player at a point in time.
type PlayerTransform struct {
	Position Vector3
	Rotation Quaternion
	Tick     uint32 // server tick at which this transform was recorded
}

// PlayerInput is a movement update submitted by the client.
// Seq allows the room loop to detect and discard out-of-order inputs.
// The room loop validates the embedded Transform before applying it.
type PlayerInput struct {
	Seq       uint32          // client-assigned sequence number (monotonic)
	Transform PlayerTransform // desired transform from client
	Timestamp int64           // client timestamp (epoch ms)
}
