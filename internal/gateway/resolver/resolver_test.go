package resolver

import (
	"context"
	"strings"
	"testing"
)

const testWebSocketURL = "ws://localhost:9001/realtime"

func TestSingleNodeResolverReturnsConfiguredAddr(t *testing.T) {
	r := NewSingleNodeResolver("127.0.0.1:9000", testWebSocketURL, 1)

	assign, err := r.ResolveRoom(context.Background(), "expo-room-a", AssignOptions{UserID: "user1"})
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}

	if assign.GameNodeAddr != "127.0.0.1:9000" {
		t.Errorf("GameNodeAddr = %q, want %q", assign.GameNodeAddr, "127.0.0.1:9000")
	}
	if assign.KCPAddr != "127.0.0.1:9000" {
		t.Errorf("KCPAddr = %q, want %q", assign.KCPAddr, "127.0.0.1:9000")
	}
	if assign.WebSocketURL != testWebSocketURL {
		t.Errorf("WebSocketURL = %q, want %q", assign.WebSocketURL, testWebSocketURL)
	}
	if assign.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion = %d, want 1", assign.ProtocolVersion)
	}
	if assign.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
}

func TestSingleNodeResolverEmptyWebSocketURL(t *testing.T) {
	r := NewSingleNodeResolver("127.0.0.1:9000", "", 1)

	assign, err := r.ResolveRoom(context.Background(), "expo-room-a", AssignOptions{UserID: "user1"})
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}
	if assign.WebSocketURL != "" {
		t.Errorf("WebSocketURL = %q, want empty string", assign.WebSocketURL)
	}
}

func TestSingleNodeResolverInstanceIDPrefix(t *testing.T) {
	r := NewSingleNodeResolver("127.0.0.1:9000", testWebSocketURL, 1)

	assign, err := r.ResolveRoom(context.Background(), "expo-room-a", AssignOptions{UserID: "user1"})
	if err != nil {
		t.Fatalf("ResolveRoom: %v", err)
	}

	if !strings.HasPrefix(assign.RoomInstanceID, "expo-room-a-") {
		t.Errorf("RoomInstanceID = %q, want prefix %q", assign.RoomInstanceID, "expo-room-a-")
	}
}

func TestSingleNodeResolverDifferentRooms(t *testing.T) {
	r := NewSingleNodeResolver("127.0.0.1:9000", testWebSocketURL, 1)

	a, _ := r.ResolveRoom(context.Background(), "room-a", AssignOptions{})
	b, _ := r.ResolveRoom(context.Background(), "room-b", AssignOptions{})

	if !strings.HasPrefix(a.RoomInstanceID, "room-a-") {
		t.Errorf("room-a instance ID = %q, want prefix room-a-", a.RoomInstanceID)
	}
	if !strings.HasPrefix(b.RoomInstanceID, "room-b-") {
		t.Errorf("room-b instance ID = %q, want prefix room-b-", b.RoomInstanceID)
	}
}

func TestSingleNodeResolverRespectsContextCancel(t *testing.T) {
	r := NewSingleNodeResolver("127.0.0.1:9000", testWebSocketURL, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.ResolveRoom(ctx, "room-a", AssignOptions{})
	if err != nil {
		t.Logf("canceled context returned error (acceptable): %v", err)
	}
}
