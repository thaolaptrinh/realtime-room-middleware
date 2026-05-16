package room_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/thaonguyen/realtime-room-middleware/internal/game/room"
)

// ---- helpers ----------------------------------------------------------------

func newTestLogger() *slog.Logger {
	return slog.Default()
}

func newTestRegistry() *room.InMemoryRoomRegistry {
	return room.NewInMemoryRoomRegistry()
}

func newTestManager(reg room.RoomRegistry) *room.RoomManager {
	return room.NewRoomManager(reg, room.DefaultRoomConfig(), newTestLogger())
}

// ---- InMemoryRoomRegistry tests ---------------------------------------------

func TestRegistry_CreateAndGet(t *testing.T) {
	reg := newTestRegistry()
	ctx := context.Background()

	spec := room.RoomSpec{
		LogicalRoomID: "test-room",
		InstanceID:    "test-room-0001",
		Config:        room.DefaultRoomConfig(),
	}

	inst, err := reg.CreateRoom(ctx, spec)
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if inst.InstanceID != spec.InstanceID {
		t.Errorf("InstanceID: got %q, want %q", inst.InstanceID, spec.InstanceID)
	}
	if inst.LogicalRoomID != spec.LogicalRoomID {
		t.Errorf("LogicalRoomID: got %q, want %q", inst.LogicalRoomID, spec.LogicalRoomID)
	}
	if inst.Status != room.RoomStatusCreated {
		t.Errorf("Status: got %s, want created", inst.Status)
	}
	if inst.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if inst.ClosedAt != nil {
		t.Error("ClosedAt should be nil for a new room")
	}

	got, err := reg.GetRoom(ctx, spec.InstanceID)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if got.InstanceID != spec.InstanceID {
		t.Errorf("GetRoom InstanceID: got %q, want %q", got.InstanceID, spec.InstanceID)
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := newTestRegistry()
	_, err := reg.GetRoom(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent room, got nil")
	}
}

func TestRegistry_DuplicateCreate(t *testing.T) {
	reg := newTestRegistry()
	ctx := context.Background()

	spec := room.RoomSpec{
		LogicalRoomID: "test-room",
		InstanceID:    "test-room-0001",
		Config:        room.DefaultRoomConfig(),
	}

	if _, err := reg.CreateRoom(ctx, spec); err != nil {
		t.Fatalf("first CreateRoom: %v", err)
	}

	_, err := reg.CreateRoom(ctx, spec)
	if err == nil {
		t.Fatal("expected error for duplicate instance ID, got nil")
	}
}

func TestRegistry_ListInstances(t *testing.T) {
	reg := newTestRegistry()
	ctx := context.Background()

	// No instances yet.
	list, err := reg.ListInstances(ctx, "test-room")
	if err != nil {
		t.Fatalf("ListInstances (empty): %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 instances, got %d", len(list))
	}

	// Create two instances under the same logical ID.
	for _, id := range []room.RoomInstanceID{"test-room-0001", "test-room-0002"} {
		spec := room.RoomSpec{
			LogicalRoomID: "test-room",
			InstanceID:    id,
			Config:        room.DefaultRoomConfig(),
		}
		if _, err := reg.CreateRoom(ctx, spec); err != nil {
			t.Fatalf("CreateRoom %q: %v", id, err)
		}
	}

	list, err = reg.ListInstances(ctx, "test-room")
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 instances, got %d", len(list))
	}

	// Different logical ID should return empty.
	list, err = reg.ListInstances(ctx, "other-room")
	if err != nil {
		t.Fatalf("ListInstances (other): %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 instances for 'other-room', got %d", len(list))
	}
}

func TestRegistry_MarkClosed(t *testing.T) {
	reg := newTestRegistry()
	ctx := context.Background()

	spec := room.RoomSpec{
		LogicalRoomID: "test-room",
		InstanceID:    "test-room-0001",
		Config:        room.DefaultRoomConfig(),
	}
	if _, err := reg.CreateRoom(ctx, spec); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if err := reg.MarkClosed(ctx, spec.InstanceID); err != nil {
		t.Fatalf("MarkClosed: %v", err)
	}

	inst, err := reg.GetRoom(ctx, spec.InstanceID)
	if err != nil {
		t.Fatalf("GetRoom after close: %v", err)
	}
	if inst.Status != room.RoomStatusClosed {
		t.Errorf("Status: got %s, want closed", inst.Status)
	}
	if inst.ClosedAt == nil {
		t.Error("ClosedAt should be set after MarkClosed")
	}
}

func TestRegistry_MarkClosed_NotFound(t *testing.T) {
	reg := newTestRegistry()
	err := reg.MarkClosed(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent room, got nil")
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := newTestRegistry()
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			id := room.RoomInstanceID(fmt.Sprintf("room-%04d", n))
			spec := room.RoomSpec{
				LogicalRoomID: "shared-logical",
				InstanceID:    id,
				Config:        room.DefaultRoomConfig(),
			}
			if _, err := reg.CreateRoom(ctx, spec); err != nil {
				return // duplicate — acceptable in concurrent test
			}
			_, _ = reg.GetRoom(ctx, id)
			_, _ = reg.ListInstances(ctx, "shared-logical")
			_ = reg.MarkClosed(ctx, id)
		}(i)
	}

	wg.Wait()
}

// ---- RoomManager tests ------------------------------------------------------

func TestManager_CreateAndGet(t *testing.T) {
	reg := newTestRegistry()
	mgr := newTestManager(reg)
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "expo-room-a")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if r.LogicalRoomID() != "expo-room-a" {
		t.Errorf("LogicalRoomID: got %q, want %q", r.LogicalRoomID(), "expo-room-a")
	}
	if r.Status() != room.RoomStatusRunning {
		t.Errorf("Status: got %s, want running", r.Status())
	}

	got, err := mgr.GetRoom(r.InstanceID())
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if got.InstanceID() != r.InstanceID() {
		t.Errorf("InstanceID mismatch: got %q, want %q", got.InstanceID(), r.InstanceID())
	}
}

func TestManager_GetNotFound(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	_, err := mgr.GetRoom("nonexistent-0001")
	if err == nil {
		t.Fatal("expected error for nonexistent room, got nil")
	}
}

func TestManager_MultipleRooms_NoSingleRoomHardcode(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	const count = 3
	rooms := make([]*room.Room, count)
	for i := range rooms {
		r, err := mgr.CreateRoom(ctx, "multi-room")
		if err != nil {
			t.Fatalf("CreateRoom[%d]: %v", i, err)
		}
		rooms[i] = r
	}
	defer func() {
		for _, r := range rooms {
			r.Stop()
		}
	}()

	if mgr.ActiveRoomCount() != count {
		t.Errorf("ActiveRoomCount: got %d, want %d", mgr.ActiveRoomCount(), count)
	}

	// All instance IDs must be unique.
	seen := make(map[room.RoomInstanceID]bool)
	for _, r := range rooms {
		if seen[r.InstanceID()] {
			t.Errorf("duplicate instance ID: %q", r.InstanceID())
		}
		seen[r.InstanceID()] = true
	}
}

func TestManager_CloseRoom(t *testing.T) {
	reg := newTestRegistry()
	mgr := newTestManager(reg)
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "close-test")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	instanceID := r.InstanceID()

	if err := mgr.CloseRoom(ctx, instanceID); err != nil {
		t.Fatalf("CloseRoom: %v", err)
	}

	if mgr.ActiveRoomCount() != 0 {
		t.Errorf("ActiveRoomCount after close: got %d, want 0", mgr.ActiveRoomCount())
	}

	if _, err := mgr.GetRoom(instanceID); err == nil {
		t.Error("expected error after CloseRoom, got nil")
	}

	// Registry must reflect closed status.
	inst, err := reg.GetRoom(ctx, instanceID)
	if err != nil {
		t.Fatalf("registry.GetRoom: %v", err)
	}
	if inst.Status != room.RoomStatusClosed {
		t.Errorf("registry status: got %s, want closed", inst.Status)
	}
}

func TestManager_CloseRoom_NotFound(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	err := mgr.CloseRoom(context.Background(), "nonexistent-0001")
	if err == nil {
		t.Fatal("expected error for nonexistent room, got nil")
	}
}

func TestManager_Shutdown(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	const count = 3
	for i := 0; i < count; i++ {
		if _, err := mgr.CreateRoom(ctx, "shutdown-room"); err != nil {
			t.Fatalf("CreateRoom[%d]: %v", i, err)
		}
	}

	if mgr.ActiveRoomCount() != count {
		t.Fatalf("before shutdown: got %d rooms, want %d", mgr.ActiveRoomCount(), count)
	}

	mgr.Shutdown(ctx)

	if mgr.ActiveRoomCount() != 0 {
		t.Errorf("after shutdown: got %d rooms, want 0", mgr.ActiveRoomCount())
	}
}

// ---- Room lifecycle tests ---------------------------------------------------

func TestRoom_StartStop(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "lifecycle-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if r.Status() != room.RoomStatusRunning {
		t.Errorf("after Start: got %s, want running", r.Status())
	}

	r.Stop()

	if r.Status() != room.RoomStatusClosed {
		t.Errorf("after Stop: got %s, want closed", r.Status())
	}
}

func TestRoom_StopIdempotent(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "idempotent-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	r.Stop()
	r.Stop() // must not panic or deadlock
	r.Stop()

	if r.Status() != room.RoomStatusClosed {
		t.Errorf("after repeated Stop: got %s, want closed", r.Status())
	}
}

// TestRoom_ContextCancellation verifies that cancelling the parent context
// causes the tick goroutine to exit, which allows Stop() to return promptly.
func TestRoom_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	reg := newTestRegistry()
	mgr := room.NewRoomManager(reg, room.DefaultRoomConfig(), newTestLogger())

	r, err := mgr.CreateRoom(ctx, "ctx-cancel-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	cancel() // cancel the parent context; tick loop should exit

	stopDone := make(chan struct{})
	go func() {
		r.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Stop returned promptly after context cancellation — expected.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return after context cancellation within 2s")
	}

	if r.Status() != room.RoomStatusClosed {
		t.Errorf("Status after context cancel + Stop: got %s, want closed", r.Status())
	}
}

// ---- Tick loop / command processing tests -----------------------------------

func TestRoom_TickLoop_ProcessesJoinCommand(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "cmd-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if err := r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: "sess-1",
		PlayerID:  "player-1",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("Enqueue join: %v", err)
	}

	// Poll until the tick loop processes the command (at most a few tick intervals).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("PlayerCount after join: got %d, want 1", r.PlayerCount())
}

func TestRoom_TickLoop_ProcessesLeaveCommand(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "leave-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// Join first.
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", Timestamp: time.Now()})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if r.PlayerCount() != 1 {
		t.Fatalf("PlayerCount after join: got %d, want 1", r.PlayerCount())
	}

	// Now leave.
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdLeave, SessionID: "s1", PlayerID: "p1", Timestamp: time.Now()})

	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("PlayerCount after leave: got %d, want 0", r.PlayerCount())
}

func TestRoom_Enqueue_RejectsWhenNotRunning(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "reject-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	r.Stop()

	err = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin})
	if err == nil {
		t.Fatal("expected Enqueue to fail on stopped room, got nil")
	}
}

// ---- Session tracking tests --------------------------------------------------

func TestRoom_HasSession_AfterJoin(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "session-track-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if r.HasSession("sess-abc") {
		t.Error("HasSession should return false before join")
	}

	_ = r.Enqueue(room.RoomCommand{
		Kind:      room.CmdJoin,
		SessionID: "sess-abc",
		PlayerID:  "player-1",
		UserID:    "user-42",
		Timestamp: time.Now(),
	})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasSession("sess-abc") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasSession should return true after CmdJoin is processed")
}

func TestRoom_HasSession_FalseAfterLeave(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "leave-track-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "sess-1", PlayerID: "p1", UserID: "u1", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasSession("sess-1") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !r.HasSession("sess-1") {
		t.Fatal("precondition: HasSession should be true after join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdLeave, SessionID: "sess-1", PlayerID: "p1", Timestamp: time.Now()})
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !r.HasSession("sess-1") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasSession should return false after CmdLeave is processed")
}

func TestRoom_HasUser_AfterJoinAndLeave(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "user-track-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if r.HasUser("user-99") {
		t.Error("HasUser should return false before join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "user-99", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasUser("user-99") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !r.HasUser("user-99") {
		t.Fatal("HasUser should return true after join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdLeave, SessionID: "s1", Timestamp: time.Now()})
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !r.HasUser("user-99") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasUser should return false after CmdLeave is processed")
}

func TestRoom_DuplicateUserJoinRejected(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "dup-user-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	// First join.
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "user-dupe", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.PlayerCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if r.PlayerCount() != 1 {
		t.Fatalf("PlayerCount after first join: got %d, want 1", r.PlayerCount())
	}

	// Second join with same UserID and different SessionID must be rejected.
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s2", PlayerID: "p2", UserID: "user-dupe", Timestamp: time.Now()})

	// Give the room loop time to process; count must stay at 1.
	time.Sleep(100 * time.Millisecond)

	if r.PlayerCount() != 1 {
		t.Errorf("PlayerCount after duplicate join: got %d, want 1 (second join must be rejected)", r.PlayerCount())
	}
	if r.HasSession("s2") {
		t.Error("HasSession(s2): duplicate session must not be attached")
	}
}

func TestRoom_ActiveSessions(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "active-sess-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	if len(r.ActiveSessions()) != 0 {
		t.Fatalf("ActiveSessions before join: got %d, want 0", len(r.ActiveSessions()))
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "u1", Timestamp: time.Now()})
	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s2", PlayerID: "p2", UserID: "u2", Timestamp: time.Now()})

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(r.ActiveSessions()) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(r.ActiveSessions()) != 2 {
		t.Errorf("ActiveSessions after two joins: got %d, want 2", len(r.ActiveSessions()))
	}
}

func TestRoom_DisconnectRemovesSession(t *testing.T) {
	mgr := newTestManager(newTestRegistry())
	ctx := context.Background()

	r, err := mgr.CreateRoom(ctx, "disconnect-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	defer r.Stop()

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdJoin, SessionID: "s1", PlayerID: "p1", UserID: "u1", Timestamp: time.Now()})
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r.HasSession("s1") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !r.HasSession("s1") {
		t.Fatal("precondition: HasSession should be true after join")
	}

	_ = r.Enqueue(room.RoomCommand{Kind: room.CmdDisconnect, SessionID: "s1", Timestamp: time.Now()})
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !r.HasSession("s1") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("HasSession should return false after CmdDisconnect")
}
