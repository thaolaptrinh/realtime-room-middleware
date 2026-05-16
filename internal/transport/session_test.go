package transport

import "testing"

func TestTransportTypeConstants(t *testing.T) {
	if string(KCP) != "kcp" {
		t.Fatalf("expected KCP == %q, got %q", "kcp", string(KCP))
	}
	if string(WebSocket) != "websocket" {
		t.Fatalf("expected WebSocket == %q, got %q", "websocket", string(WebSocket))
	}
}

func TestTransportTypeDistinct(t *testing.T) {
	if KCP == WebSocket {
		t.Fatal("KCP and WebSocket transport types must be distinct")
	}
}

func TestPacketReceiverFuncAdapter(t *testing.T) {
	var called bool
	var recv PacketReceiver = PacketReceiverFunc(func(sess RealtimeSession, data []byte) {
		called = true
	})
	recv.HandlePacket(nil, nil)
	if !called {
		t.Fatal("PacketReceiverFunc did not call the wrapped function")
	}
}
