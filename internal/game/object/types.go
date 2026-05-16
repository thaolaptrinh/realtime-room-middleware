package object

import "time"

// ObjectID identifies a room object.
type ObjectID string

// ObjectKind classifies a room object (e.g., "chair", "speaker", "projector").
type ObjectKind string

// ObjectStatus is the lifecycle state of a room object.
type ObjectStatus int

const (
	ObjectStatusActive   ObjectStatus = iota // In the room; synchronized to nearby clients.
	ObjectStatusInactive                      // Removed or unavailable; not synchronized.
)

// Vec3 is a 3D position with float32 components.
type Vec3 struct {
	X, Y, Z float32
}

// Quat is a rotation as a unit quaternion (X, Y, Z, W).
type Quat struct {
	X, Y, Z, W float32
}

// IdentityQuat is the no-rotation quaternion.
var IdentityQuat = Quat{W: 1}

// ObjectTransform holds the position and rotation of a room object.
type ObjectTransform struct {
	Position Vec3
	Rotation Quat
}

// LockState describes the current lock status of an object.
// Zero value means unlocked.
type LockState struct {
	LockedBy  string    // UserID of the current lock owner; empty if unlocked.
	SessionID string    // SessionID of the lock owner; used for disconnect release.
	LockUntil time.Time // Expiry of the lock; zero if unlocked.
}

// IsLocked returns true if the object has a non-expired active lock.
func (ls LockState) IsLocked(now time.Time) bool {
	return ls.LockedBy != "" && ls.LockUntil.After(now)
}

// IsOwnedBy returns true if the active lock is held by the given user.
func (ls LockState) IsOwnedBy(userID string, now time.Time) bool {
	return ls.IsLocked(now) && ls.LockedBy == userID
}

// LockLease holds TTL and per-user limit configuration.
type LockLease struct {
	TTL             time.Duration // Lock validity from acquisition or last refresh.
	MaxLocksPerUser int           // Maximum concurrent locks a single user may hold.
}

// DefaultLockLease returns production-default lease settings from the blueprint.
func DefaultLockLease() LockLease {
	return LockLease{
		TTL:             10 * time.Second,
		MaxLocksPerUser: 3,
	}
}

// ObjectState is the authoritative runtime record for a room object.
// Only the room loop goroutine may mutate ObjectState fields.
type ObjectState struct {
	ID          ObjectID
	Kind        ObjectKind
	Transform   ObjectTransform
	CustomState []byte       // Arbitrary serialized state; opaque to the lock manager.
	Status      ObjectStatus
	Lock        LockState
	Version     uint32 // Incremented on every state or lock change.
}
