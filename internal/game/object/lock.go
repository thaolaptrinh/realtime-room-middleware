package object

import (
	"fmt"
	"time"
)

// LockResult describes the outcome of a lock operation.
type LockResult struct {
	Granted bool   // true if the operation succeeded.
	Reason  string // non-empty when !Granted; human-readable rejection reason.
}

func lockGranted() LockResult              { return LockResult{Granted: true} }
func lockRejected(reason string) LockResult { return LockResult{Reason: reason} }

// LockManager enforces server-authoritative lease-based object locking.
// It reads and mutates the Lock field of ObjectState values owned by the ObjectManager.
//
// Not goroutine-safe — must be accessed only from the room loop goroutine.
type LockManager struct {
	objects       *ObjectManager
	lease         LockLease
	userLockCount map[string]int // userID → count of currently held (non-expired) locks
}

// NewLockManager creates a LockManager that operates on the given ObjectManager.
func NewLockManager(objects *ObjectManager, lease LockLease) *LockManager {
	return &LockManager{
		objects:       objects,
		lease:         lease,
		userLockCount: make(map[string]int),
	}
}

// AcquireLock attempts to grant a lock on objectID to ownerUserID/ownerSessionID.
//
// Succeeds if:
//   - The object exists and is active.
//   - The object is unlocked (or the lock has expired and been cleared by ReleaseExpired).
//   - The owner is below their MaxLocksPerUser limit.
//
// Special case: if the owner already holds the active lock, the TTL is extended
// and Granted is returned without counting a second lock.
func (lm *LockManager) AcquireLock(objectID ObjectID, ownerUserID, ownerSessionID string, now time.Time) LockResult {
	obj, ok := lm.objects.Get(objectID)
	if !ok {
		return lockRejected(fmt.Sprintf("object %q not found", objectID))
	}
	if obj.Status == ObjectStatusInactive {
		return lockRejected(fmt.Sprintf("object %q is inactive", objectID))
	}
	if obj.Lock.IsLocked(now) {
		if obj.Lock.LockedBy == ownerUserID {
			// Same owner re-acquiring: extend TTL and update session in place.
			obj.Lock.LockUntil = now.Add(lm.lease.TTL)
			obj.Lock.SessionID = ownerSessionID
			obj.Version++
			return lockGranted()
		}
		return lockRejected(fmt.Sprintf("object %q is locked by another user", objectID))
	}
	if lm.userLockCount[ownerUserID] >= lm.lease.MaxLocksPerUser {
		return lockRejected(fmt.Sprintf("user %q holds the maximum of %d locks", ownerUserID, lm.lease.MaxLocksPerUser))
	}
	obj.Lock = LockState{
		LockedBy:  ownerUserID,
		SessionID: ownerSessionID,
		LockUntil: now.Add(lm.lease.TTL),
	}
	obj.Version++
	lm.userLockCount[ownerUserID]++
	return lockGranted()
}

// RefreshLock extends the TTL of an active lock held by ownerUserID.
//
// Returns rejected if the object does not exist, the lock has expired, or the
// lock is owned by a different user.
func (lm *LockManager) RefreshLock(objectID ObjectID, ownerUserID string, now time.Time) LockResult {
	obj, ok := lm.objects.Get(objectID)
	if !ok {
		return lockRejected(fmt.Sprintf("object %q not found", objectID))
	}
	if !obj.Lock.IsOwnedBy(ownerUserID, now) {
		return lockRejected(fmt.Sprintf("object %q is not locked by %q", objectID, ownerUserID))
	}
	obj.Lock.LockUntil = now.Add(lm.lease.TTL)
	// Version is not incremented on refresh: a TTL extension is not a state change.
	return lockGranted()
}

// ReleaseLock clears the active lock on objectID if it is owned by ownerUserID.
//
// Returns rejected if the object does not exist or is not owned by ownerUserID.
func (lm *LockManager) ReleaseLock(objectID ObjectID, ownerUserID string, now time.Time) LockResult {
	obj, ok := lm.objects.Get(objectID)
	if !ok {
		return lockRejected(fmt.Sprintf("object %q not found", objectID))
	}
	if !obj.Lock.IsOwnedBy(ownerUserID, now) {
		return lockRejected(fmt.Sprintf("object %q is not locked by %q", objectID, ownerUserID))
	}
	lm.clearLock(obj)
	return lockGranted()
}

// ReleaseExpired scans all objects and clears any locks whose TTL has passed.
// Returns the IDs of objects whose locks were released.
// Called by the room loop on every tick before draining commands.
func (lm *LockManager) ReleaseExpired(now time.Time) []ObjectID {
	var released []ObjectID
	for _, obj := range lm.objects.List() {
		if obj.Lock.LockedBy != "" && !obj.Lock.LockUntil.After(now) {
			released = append(released, obj.ID)
			lm.clearLock(obj)
		}
	}
	return released
}

// ReleaseBySession releases all locks held by the given session.
// Called by the room loop when a session disconnects or leaves.
// Returns the IDs of objects whose locks were released.
func (lm *LockManager) ReleaseBySession(sessionID string, now time.Time) []ObjectID {
	var released []ObjectID
	for _, obj := range lm.objects.List() {
		if obj.Lock.SessionID == sessionID && obj.Lock.LockedBy != "" {
			released = append(released, obj.ID)
			lm.clearLock(obj)
		}
	}
	return released
}

// UserLockCount returns the number of active locks currently held by userID.
func (lm *LockManager) UserLockCount(userID string) int {
	return lm.userLockCount[userID]
}

// clearLock removes the lock from an object and updates the owner's lock count.
func (lm *LockManager) clearLock(obj *ObjectState) {
	if obj.Lock.LockedBy != "" && lm.userLockCount[obj.Lock.LockedBy] > 0 {
		lm.userLockCount[obj.Lock.LockedBy]--
		if lm.userLockCount[obj.Lock.LockedBy] == 0 {
			delete(lm.userLockCount, obj.Lock.LockedBy)
		}
	}
	obj.Lock = LockState{}
	obj.Version++
}
