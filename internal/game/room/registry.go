package room

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RoomRegistry stores and retrieves room instance metadata.
//
// Phase 1 uses InMemoryRoomRegistry (no external dependencies).
// Phase 2 will provide a Redis-backed implementation in internal/adapters/registry/.
type RoomRegistry interface {
	// CreateRoom registers a new room instance. Returns an error if the instance
	// ID already exists.
	CreateRoom(ctx context.Context, spec RoomSpec) (*RoomInstance, error)

	// GetRoom retrieves a room instance by its physical instance ID.
	GetRoom(ctx context.Context, instanceID RoomInstanceID) (*RoomInstance, error)

	// ListInstances returns all known instances for a given logical room ID.
	// Returns a nil slice (not an error) when no instances exist.
	ListInstances(ctx context.Context, logicalRoomID LogicalRoomID) ([]*RoomInstance, error)

	// MarkClosed transitions an instance to RoomStatusClosed in the registry.
	MarkClosed(ctx context.Context, instanceID RoomInstanceID) error
}

// InMemoryRoomRegistry is the Phase 1 single-vps room registry.
// It has no external dependencies and is safe for concurrent access.
type InMemoryRoomRegistry struct {
	mu        sync.Mutex
	instances map[RoomInstanceID]*RoomInstance
	byLogical map[LogicalRoomID][]*RoomInstance
}

// NewInMemoryRoomRegistry returns a ready-to-use in-memory registry.
func NewInMemoryRoomRegistry() *InMemoryRoomRegistry {
	return &InMemoryRoomRegistry{
		instances: make(map[RoomInstanceID]*RoomInstance),
		byLogical: make(map[LogicalRoomID][]*RoomInstance),
	}
}

func (r *InMemoryRoomRegistry) CreateRoom(_ context.Context, spec RoomSpec) (*RoomInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.instances[spec.InstanceID]; exists {
		return nil, fmt.Errorf("room instance %q already exists", spec.InstanceID)
	}

	inst := &RoomInstance{
		InstanceID:    spec.InstanceID,
		LogicalRoomID: spec.LogicalRoomID,
		Status:        RoomStatusCreated,
		CreatedAt:     time.Now(),
	}

	r.instances[spec.InstanceID] = inst
	r.byLogical[spec.LogicalRoomID] = append(r.byLogical[spec.LogicalRoomID], inst)

	return inst, nil
}

func (r *InMemoryRoomRegistry) GetRoom(_ context.Context, instanceID RoomInstanceID) (*RoomInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.instances[instanceID]
	if !ok {
		return nil, fmt.Errorf("room instance %q not found", instanceID)
	}
	return inst, nil
}

func (r *InMemoryRoomRegistry) ListInstances(_ context.Context, logicalRoomID LogicalRoomID) ([]*RoomInstance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	list := r.byLogical[logicalRoomID]
	if len(list) == 0 {
		return nil, nil
	}
	// Return a copy to prevent callers from mutating the internal slice.
	out := make([]*RoomInstance, len(list))
	copy(out, list)
	return out, nil
}

func (r *InMemoryRoomRegistry) MarkClosed(_ context.Context, instanceID RoomInstanceID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.instances[instanceID]
	if !ok {
		return fmt.Errorf("room instance %q not found", instanceID)
	}

	now := time.Now()
	inst.Status = RoomStatusClosed
	inst.ClosedAt = &now
	return nil
}
