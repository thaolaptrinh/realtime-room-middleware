// Package resolver defines the NodeResolver interface for resolving logical rooms
// to physical game-node assignments. Single-vps mode uses SingleNodeResolver.
// Distributed mode will use RedisNodeResolver (future, no Redis dependency here).
package resolver

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

// NodeAssignment is the result of resolving a logical room to a game node.
type NodeAssignment struct {
	RoomInstanceID  string // physical room instance ID
	GameNodeAddr    string // game server host address
	KCPAddr         string // KCP/UDP address (host:port); empty if not configured
	WebSocketURL    string // public WSS URL (e.g. wss://host/realtime); empty if not configured
	ProtocolVersion uint16
	ExpiresAt       time.Time // when the assignment token expires
}

// AssignOptions carries optional parameters for room assignment.
type AssignOptions struct {
	UserID string
}

// NodeResolver resolves a logical room ID to a physical game-node assignment.
type NodeResolver interface {
	// ResolveRoom resolves a logical room to a node assignment.
	// If no instance exists yet, it may assign one.
	ResolveRoom(ctx context.Context, logicalRoomID string, opts AssignOptions) (NodeAssignment, error)
}

// SingleNodeResolver returns a fixed local game server address for all rooms.
// Used in dev and single-vps modes where there is one game server process.
type SingleNodeResolver struct {
	kcpAddr         string
	websocketURL    string
	protocolVersion uint16
	tokenTTL        time.Duration
}

// NewSingleNodeResolver creates a resolver that always returns the configured single node addresses.
// websocketURL is the public-facing WSS URL returned to WebGL clients; pass empty string to
// disable WebSocket transport in join responses.
func NewSingleNodeResolver(kcpAddr, websocketURL string, protocolVersion int) *SingleNodeResolver {
	return &SingleNodeResolver{
		kcpAddr:         kcpAddr,
		websocketURL:    websocketURL,
		protocolVersion: uint16(protocolVersion),
		tokenTTL:        5 * time.Minute,
	}
}

// ResolveRoom assigns the single configured node for any logical room.
func (r *SingleNodeResolver) ResolveRoom(ctx context.Context, logicalRoomID string, opts AssignOptions) (NodeAssignment, error) {
	instanceID := generateInstanceID(logicalRoomID)
	return NodeAssignment{
		RoomInstanceID:  instanceID,
		GameNodeAddr:    r.kcpAddr,
		KCPAddr:         r.kcpAddr,
		WebSocketURL:    r.websocketURL,
		ProtocolVersion: r.protocolVersion,
		ExpiresAt:       time.Now().Add(r.tokenTTL),
	}, nil
}

// generateInstanceID creates a deterministic-looking instance ID for a logical room.
// For single-vps mode this is sufficient since there is only one process.
// Distributed mode will use a different scheme with Redis-backed tracking.
func generateInstanceID(logicalRoomID string) string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%s-%04d", logicalRoomID, n)
}
