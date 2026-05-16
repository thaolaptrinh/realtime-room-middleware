package object_test

import (
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/object"
)

// ---- ObjectManager tests -------------------------------------------------------

func TestObjectManager_Create(t *testing.T) {
	mgr := object.NewObjectManager()
	obj, err := mgr.Create("obj-1", "chair", object.ObjectTransform{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if obj.ID != "obj-1" {
		t.Errorf("ID = %q, want %q", obj.ID, "obj-1")
	}
	if obj.Kind != "chair" {
		t.Errorf("Kind = %q, want %q", obj.Kind, "chair")
	}
	if obj.Status != object.ObjectStatusActive {
		t.Errorf("Status = %v, want active", obj.Status)
	}
	if obj.Version != 0 {
		t.Errorf("Version = %d, want 0", obj.Version)
	}
}

func TestObjectManager_Create_DuplicateRejected(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-1", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := mgr.Create("obj-1", "table", object.ObjectTransform{}); err == nil {
		t.Fatal("expected error for duplicate object ID, got nil")
	}
}

func TestObjectManager_Get(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-1", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	obj, ok := mgr.Get("obj-1")
	if !ok {
		t.Fatal("Get returned false for existing object")
	}
	if obj.ID != "obj-1" {
		t.Errorf("Get ID = %q, want %q", obj.ID, "obj-1")
	}
}

func TestObjectManager_Get_NotFound(t *testing.T) {
	mgr := object.NewObjectManager()
	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Fatal("Get returned true for nonexistent object")
	}
}

func TestObjectManager_List(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-1", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create obj-1: %v", err)
	}
	if _, err := mgr.Create("obj-2", "table", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create obj-2: %v", err)
	}

	list := mgr.List()
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
}

func TestObjectManager_UpdateTransform(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-1", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newTransform := object.ObjectTransform{
		Position: object.Vec3{X: 10, Y: 0, Z: 5},
		Rotation: object.IdentityQuat,
	}
	if err := mgr.UpdateTransform("obj-1", newTransform); err != nil {
		t.Fatalf("UpdateTransform: %v", err)
	}

	obj, _ := mgr.Get("obj-1")
	if obj.Transform.Position.X != 10 {
		t.Errorf("Position.X = %.1f, want 10.0", obj.Transform.Position.X)
	}
	if obj.Version != 1 {
		t.Errorf("Version = %d, want 1 after update", obj.Version)
	}
}

func TestObjectManager_UpdateTransform_NotFound(t *testing.T) {
	mgr := object.NewObjectManager()
	err := mgr.UpdateTransform("nonexistent", object.ObjectTransform{})
	if err == nil {
		t.Fatal("expected error for nonexistent object, got nil")
	}
}

func TestObjectManager_MarkInactive(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-1", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.MarkInactive("obj-1"); err != nil {
		t.Fatalf("MarkInactive: %v", err)
	}

	obj, _ := mgr.Get("obj-1")
	if obj.Status != object.ObjectStatusInactive {
		t.Errorf("Status = %v, want inactive", obj.Status)
	}
	// Count should still include inactive objects.
	if mgr.Count() != 1 {
		t.Errorf("Count = %d, want 1 (inactive still tracked)", mgr.Count())
	}
	// ActiveObjects should not include it.
	if len(mgr.ActiveObjects()) != 0 {
		t.Errorf("ActiveObjects = %d, want 0 after marking inactive", len(mgr.ActiveObjects()))
	}
}

func TestObjectManager_Remove(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-1", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mgr.Remove("obj-1")

	if mgr.Count() != 0 {
		t.Errorf("Count after Remove = %d, want 0", mgr.Count())
	}
	if _, ok := mgr.Get("obj-1"); ok {
		t.Error("Get should return false after Remove")
	}
}

func TestObjectManager_ActiveObjects(t *testing.T) {
	mgr := object.NewObjectManager()
	if _, err := mgr.Create("obj-active", "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create active: %v", err)
	}
	if _, err := mgr.Create("obj-inactive", "table", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create inactive: %v", err)
	}
	_ = mgr.MarkInactive("obj-inactive")

	active := mgr.ActiveObjects()
	if len(active) != 1 {
		t.Fatalf("ActiveObjects len = %d, want 1", len(active))
	}
	if active[0].ID != "obj-active" {
		t.Errorf("ActiveObjects[0].ID = %q, want %q", active[0].ID, "obj-active")
	}
}

// ---- LockManager tests ---------------------------------------------------------

func newTestLockManager() (*object.ObjectManager, *object.LockManager) {
	objs := object.NewObjectManager()
	lease := object.LockLease{TTL: 10 * time.Second, MaxLocksPerUser: 3}
	locks := object.NewLockManager(objs, lease)
	return objs, locks
}

func mustCreate(t *testing.T, mgr *object.ObjectManager, id object.ObjectID) {
	t.Helper()
	if _, err := mgr.Create(id, "chair", object.ObjectTransform{}); err != nil {
		t.Fatalf("Create %q: %v", id, err)
	}
}

func TestLockManager_AcquireLock_Succeeds(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	result := locks.AcquireLock("obj-1", "alice", "sess-1", now)
	if !result.Granted {
		t.Fatalf("AcquireLock failed: %s", result.Reason)
	}

	obj, _ := objs.Get("obj-1")
	if obj.Lock.LockedBy != "alice" {
		t.Errorf("LockedBy = %q, want %q", obj.Lock.LockedBy, "alice")
	}
	if !obj.Lock.IsLocked(now) {
		t.Error("lock should be active immediately after acquire")
	}
	if obj.Version != 1 {
		t.Errorf("Version = %d, want 1 after lock", obj.Version)
	}
	if locks.UserLockCount("alice") != 1 {
		t.Errorf("UserLockCount = %d, want 1", locks.UserLockCount("alice"))
	}
}

func TestLockManager_AcquireLock_RejectWhenLockedByOther(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	// Alice acquires first.
	if r := locks.AcquireLock("obj-1", "alice", "sess-a", now); !r.Granted {
		t.Fatalf("alice AcquireLock failed: %s", r.Reason)
	}

	// Bob tries to acquire — must be rejected.
	result := locks.AcquireLock("obj-1", "bob", "sess-b", now)
	if result.Granted {
		t.Fatal("bob AcquireLock should have been rejected while alice holds the lock")
	}
}

func TestLockManager_AcquireLock_SameOwnerReAcquireExtendsLease(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	locks.AcquireLock("obj-1", "alice", "sess-a", now)
	obj, _ := objs.Get("obj-1")
	firstExpiry := obj.Lock.LockUntil

	// Re-acquire as same owner with a later timestamp.
	later := now.Add(2 * time.Second)
	result := locks.AcquireLock("obj-1", "alice", "sess-a", later)
	if !result.Granted {
		t.Fatalf("re-acquire failed: %s", result.Reason)
	}
	if !obj.Lock.LockUntil.After(firstExpiry) {
		t.Error("re-acquire should extend the lock expiry")
	}
	// User lock count must not double-count.
	if locks.UserLockCount("alice") != 1 {
		t.Errorf("UserLockCount after re-acquire = %d, want 1", locks.UserLockCount("alice"))
	}
}

func TestLockManager_AcquireLock_RejectWhenMaxLocksReached(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	mustCreate(t, objs, "obj-2")
	mustCreate(t, objs, "obj-3")
	mustCreate(t, objs, "obj-4")
	now := time.Now()

	// Acquire up to the limit (3).
	for _, id := range []object.ObjectID{"obj-1", "obj-2", "obj-3"} {
		if r := locks.AcquireLock(id, "alice", "sess-a", now); !r.Granted {
			t.Fatalf("acquire %q failed: %s", id, r.Reason)
		}
	}
	if locks.UserLockCount("alice") != 3 {
		t.Fatalf("UserLockCount = %d, want 3", locks.UserLockCount("alice"))
	}

	// Fourth acquire must be rejected.
	result := locks.AcquireLock("obj-4", "alice", "sess-a", now)
	if result.Granted {
		t.Fatal("fourth AcquireLock should be rejected (max locks reached)")
	}
}

func TestLockManager_AcquireLock_ObjectNotFound(t *testing.T) {
	_, locks := newTestLockManager()
	result := locks.AcquireLock("nonexistent", "alice", "sess-a", time.Now())
	if result.Granted {
		t.Fatal("AcquireLock should fail for nonexistent object")
	}
}

func TestLockManager_AcquireLock_InactiveObjectRejected(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	_ = objs.MarkInactive("obj-1")
	result := locks.AcquireLock("obj-1", "alice", "sess-a", time.Now())
	if result.Granted {
		t.Fatal("AcquireLock should fail for inactive object")
	}
}

func TestLockManager_RefreshLock_Succeeds(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	locks.AcquireLock("obj-1", "alice", "sess-a", now)
	obj, _ := objs.Get("obj-1")
	versionBefore := obj.Version
	expiryBefore := obj.Lock.LockUntil

	later := now.Add(3 * time.Second)
	result := locks.RefreshLock("obj-1", "alice", later)
	if !result.Granted {
		t.Fatalf("RefreshLock failed: %s", result.Reason)
	}
	if !obj.Lock.LockUntil.After(expiryBefore) {
		t.Error("RefreshLock should extend expiry")
	}
	if obj.Version != versionBefore {
		t.Errorf("Version changed on refresh: got %d, want %d (no state change)", obj.Version, versionBefore)
	}
}

func TestLockManager_RefreshLock_RejectNonOwner(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	locks.AcquireLock("obj-1", "alice", "sess-a", now)

	result := locks.RefreshLock("obj-1", "bob", now)
	if result.Granted {
		t.Fatal("RefreshLock by non-owner should be rejected")
	}
}

func TestLockManager_RefreshLock_RejectExpired(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	// Acquire and then advance time past the TTL.
	locks.AcquireLock("obj-1", "alice", "sess-a", now)
	expired := now.Add(20 * time.Second) // past 10s TTL

	result := locks.RefreshLock("obj-1", "alice", expired)
	if result.Granted {
		t.Fatal("RefreshLock should be rejected after lock has expired")
	}
}

func TestLockManager_ReleaseLock_Succeeds(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	locks.AcquireLock("obj-1", "alice", "sess-a", now)
	obj, _ := objs.Get("obj-1")
	versionBefore := obj.Version

	result := locks.ReleaseLock("obj-1", "alice", now)
	if !result.Granted {
		t.Fatalf("ReleaseLock failed: %s", result.Reason)
	}
	if obj.Lock.LockedBy != "" {
		t.Errorf("LockedBy = %q after release, want empty", obj.Lock.LockedBy)
	}
	if obj.Version != versionBefore+1 {
		t.Errorf("Version = %d after release, want %d", obj.Version, versionBefore+1)
	}
	if locks.UserLockCount("alice") != 0 {
		t.Errorf("UserLockCount after release = %d, want 0", locks.UserLockCount("alice"))
	}
}

func TestLockManager_ReleaseLock_RejectNonOwner(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	locks.AcquireLock("obj-1", "alice", "sess-a", now)

	result := locks.ReleaseLock("obj-1", "bob", now)
	if result.Granted {
		t.Fatal("ReleaseLock by non-owner should be rejected")
	}

	// Alice's lock should still be active.
	obj, _ := objs.Get("obj-1")
	if !obj.Lock.IsLocked(now) {
		t.Error("lock should still be active after failed release by non-owner")
	}
}

func TestLockManager_ReleaseExpired(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	mustCreate(t, objs, "obj-2")
	now := time.Now()

	// Acquire both.
	locks.AcquireLock("obj-1", "alice", "sess-a", now)
	locks.AcquireLock("obj-2", "bob", "sess-b", now)

	if locks.UserLockCount("alice") != 1 {
		t.Fatalf("alice count before expire = %d, want 1", locks.UserLockCount("alice"))
	}

	// Advance time past TTL.
	expired := now.Add(20 * time.Second)
	released := locks.ReleaseExpired(expired)

	if len(released) != 2 {
		t.Errorf("ReleaseExpired: got %d released, want 2", len(released))
	}
	if locks.UserLockCount("alice") != 0 {
		t.Errorf("alice count after expire = %d, want 0", locks.UserLockCount("alice"))
	}
	if locks.UserLockCount("bob") != 0 {
		t.Errorf("bob count after expire = %d, want 0", locks.UserLockCount("bob"))
	}

	// Objects should now be lockable again.
	if r := locks.AcquireLock("obj-1", "alice", "sess-a", expired); !r.Granted {
		t.Fatalf("re-acquire after expire failed: %s", r.Reason)
	}
}

func TestLockManager_ReleaseExpired_OnlyExpired(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	mustCreate(t, objs, "obj-2")
	now := time.Now()

	shortLease := object.LockLease{TTL: 2 * time.Second, MaxLocksPerUser: 3}
	shortLocks := object.NewLockManager(objs, shortLease)
	_ = shortLocks

	// Acquire obj-1 early, obj-2 later.
	locks.AcquireLock("obj-1", "alice", "sess-a", now)
	locks.AcquireLock("obj-2", "bob", "sess-b", now.Add(5*time.Second))

	// Advance 12s: obj-1 expired (10+2s margin), obj-2 not yet (10-5=5s remaining).
	atT := now.Add(12 * time.Second)
	released := locks.ReleaseExpired(atT)

	if len(released) != 1 {
		t.Errorf("expected 1 released (obj-1 only), got %d: %v", len(released), released)
	}
	if len(released) == 1 && released[0] != "obj-1" {
		t.Errorf("released[0] = %q, want %q", released[0], "obj-1")
	}
}

func TestLockManager_ReleaseBySession(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	mustCreate(t, objs, "obj-2")
	mustCreate(t, objs, "obj-3")
	now := time.Now()

	// alice holds obj-1 and obj-2; bob holds obj-3.
	locks.AcquireLock("obj-1", "alice", "sess-alice", now)
	locks.AcquireLock("obj-2", "alice", "sess-alice", now)
	locks.AcquireLock("obj-3", "bob", "sess-bob", now)

	if locks.UserLockCount("alice") != 2 {
		t.Fatalf("alice count before disconnect = %d, want 2", locks.UserLockCount("alice"))
	}

	// Alice's session disconnects.
	released := locks.ReleaseBySession("sess-alice", now)

	if len(released) != 2 {
		t.Errorf("ReleaseBySession: got %d released, want 2", len(released))
	}
	if locks.UserLockCount("alice") != 0 {
		t.Errorf("alice count after disconnect = %d, want 0", locks.UserLockCount("alice"))
	}
	// Bob's lock must remain.
	if locks.UserLockCount("bob") != 1 {
		t.Errorf("bob count after alice disconnect = %d, want 1", locks.UserLockCount("bob"))
	}
	obj3, _ := objs.Get("obj-3")
	if !obj3.Lock.IsLocked(now) {
		t.Error("bob's lock on obj-3 should remain after alice disconnects")
	}
}

func TestLockManager_LockExpiry_AllowsNewOwner(t *testing.T) {
	objs, locks := newTestLockManager()
	mustCreate(t, objs, "obj-1")
	now := time.Now()

	// Alice acquires.
	locks.AcquireLock("obj-1", "alice", "sess-a", now)

	// Simulate ReleaseExpired running after TTL.
	expired := now.Add(11 * time.Second)
	locks.ReleaseExpired(expired)

	// Bob can now acquire.
	result := locks.AcquireLock("obj-1", "bob", "sess-b", expired)
	if !result.Granted {
		t.Fatalf("bob should be able to acquire after alice's lock expired: %s", result.Reason)
	}
	obj, _ := objs.Get("obj-1")
	if obj.Lock.LockedBy != "bob" {
		t.Errorf("LockedBy = %q, want %q", obj.Lock.LockedBy, "bob")
	}
}
