package realtime

import (
	"sync"

	"github.com/thaonguyen/realtime-room-middleware/internal/transport"
)

type recordingSession struct {
	mu        sync.Mutex
	id        string
	transport transport.TransportType
	userID    string
	packets   [][]byte
	sendErr   error
	closed    bool
}

func newFakeKCPSession(id string) *recordingSession {
	return &recordingSession{id: id, transport: transport.KCP}
}

func newFakeWSSSession(id string) *recordingSession {
	return &recordingSession{id: id, transport: transport.WebSocket}
}

func (s *recordingSession) ID() string                         { return s.id }
func (s *recordingSession) UserID() string                     { return s.userID }
func (s *recordingSession) Transport() transport.TransportType { return s.transport }
func (s *recordingSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
func (s *recordingSession) Send(packet []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sendErr != nil {
		return s.sendErr
	}
	cp := make([]byte, len(packet))
	copy(cp, packet)
	s.packets = append(s.packets, cp)
	return nil
}

func (s *recordingSession) Packets() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.packets))
	copy(out, s.packets)
	return out
}

func (s *recordingSession) PacketCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.packets)
}

var _ transport.RealtimeSession = (*recordingSession)(nil)
