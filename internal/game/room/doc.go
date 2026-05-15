// Package room implements the Room struct, room loop tick, and RoomManager
// for multi-room lifecycle management.
//
// Room loop is the single authority for room state mutation.
// Network goroutines must not mutate room state directly.
//
// Not yet implemented.
package room
