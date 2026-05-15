// Package object defines ObjectState and the object lock manager.
//
// Object locking uses a server-authoritative command queue with lease TTL.
// This package covers:
//   - Object state (position, type, custom state bytes, lock info, version)
//   - Lock acquisition, refresh, release, and expiration
//   - Disconnect release
//   - Max locks per user enforcement
//
// Not yet implemented.
package object
