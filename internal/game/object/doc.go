// Package object manages room object state and server-authoritative lease-based locking.
//
// ObjectManager handles object creation, retrieval, updates, and lifecycle.
// LockManager enforces the command queue + lease TTL locking model:
//   - Acquire: granted if the object is unlocked and the owner is under their lock limit.
//   - Refresh: extends TTL; only the current owner may refresh.
//   - Release: cleared immediately; only the current owner may release.
//   - Expire: locks past their TTL are cleared each room tick by ReleaseExpired.
//   - Disconnect: all locks held by a session are released by ReleaseBySession.
//
// Neither ObjectManager nor LockManager is goroutine-safe.
// Both must be accessed exclusively from the room loop goroutine.
package object
