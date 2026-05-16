package room

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

// RoomManager creates, tracks, and destroys Room instances.
// It owns the RoomRegistry and the live room map.
//
// Phase 1 single-vps: backed by InMemoryRoomRegistry, no Redis dependency.
type RoomManager struct {
	logger   *slog.Logger
	registry RoomRegistry
	config   RoomConfig

	mu    sync.Mutex
	rooms map[RoomInstanceID]*Room

	// instanceCounter generates monotonically increasing instance ID suffixes.
	instanceCounter atomic.Uint64
}

// NewRoomManager constructs a RoomManager backed by the given registry.
func NewRoomManager(registry RoomRegistry, cfg RoomConfig, logger *slog.Logger) *RoomManager {
	return &RoomManager{
		logger:   logger,
		registry: registry,
		config:   cfg,
		rooms:    make(map[RoomInstanceID]*Room),
	}
}

// CreateRoom creates, registers, and starts a new room instance for the given
// logical room ID. The caller receives a running Room ready to accept commands.
func (m *RoomManager) CreateRoom(ctx context.Context, logicalID LogicalRoomID) (*Room, error) {
	instanceID := m.generateInstanceID(logicalID)

	spec := RoomSpec{
		LogicalRoomID: logicalID,
		InstanceID:    instanceID,
		Config:        m.config,
	}

	if _, err := m.registry.CreateRoom(ctx, spec); err != nil {
		return nil, fmt.Errorf("register room %q: %w", instanceID, err)
	}

	room := newRoom(spec, m.logger)
	if err := room.Start(ctx); err != nil {
		// Roll back: mark registry entry closed on start failure.
		_ = m.registry.MarkClosed(ctx, instanceID)
		return nil, fmt.Errorf("start room %q: %w", instanceID, err)
	}

	m.mu.Lock()
	m.rooms[instanceID] = room
	m.mu.Unlock()

	m.logger.Info("room created",
		slog.String("logical_room_id", string(logicalID)),
		slog.String("room_instance_id", string(instanceID)),
	)

	return room, nil
}

// GetRoom retrieves a live room by instance ID.
// Returns an error if the room is not found in the active rooms map.
func (m *RoomManager) GetRoom(instanceID RoomInstanceID) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, ok := m.rooms[instanceID]
	if !ok {
		return nil, fmt.Errorf("room instance %q not found", instanceID)
	}
	return room, nil
}

// CloseRoom stops the room tick loop, removes it from the active rooms map,
// and marks it closed in the registry.
func (m *RoomManager) CloseRoom(ctx context.Context, instanceID RoomInstanceID) error {
	m.mu.Lock()
	room, ok := m.rooms[instanceID]
	if ok {
		delete(m.rooms, instanceID)
	}
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("room instance %q not found", instanceID)
	}

	// Stop the tick loop (blocking; called without the manager mutex held).
	room.Stop()

	if err := m.registry.MarkClosed(ctx, instanceID); err != nil {
		m.logger.Warn("failed to mark room closed in registry",
			slog.String("room_instance_id", string(instanceID)),
			slog.String("err", err.Error()),
		)
	}

	m.logger.Info("room closed",
		slog.String("room_instance_id", string(instanceID)),
	)
	return nil
}

// Shutdown stops all active rooms. Call on server shutdown.
func (m *RoomManager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	rooms := make(map[RoomInstanceID]*Room, len(m.rooms))
	for id, r := range m.rooms {
		rooms[id] = r
	}
	m.rooms = make(map[RoomInstanceID]*Room)
	m.mu.Unlock()

	for id, r := range rooms {
		r.Stop()
		_ = m.registry.MarkClosed(ctx, id)
	}

	m.logger.Info("room manager shutdown complete",
		slog.Int("rooms_closed", len(rooms)),
	)
}

// ActiveRoomCount returns the number of currently running room instances.
func (m *RoomManager) ActiveRoomCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rooms)
}

// generateInstanceID creates a unique RoomInstanceID.
// Format: "<logicalID>-<zero-padded-counter>", e.g., "expo-room-a-0001".
func (m *RoomManager) generateInstanceID(logicalID LogicalRoomID) RoomInstanceID {
	n := m.instanceCounter.Add(1)
	return RoomInstanceID(fmt.Sprintf("%s-%04d", logicalID, n))
}
