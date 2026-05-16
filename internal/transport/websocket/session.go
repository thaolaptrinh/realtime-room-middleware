package websocket

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

// wssSession wraps a WebSocket connection with safe lifecycle management.
//
// All realtime payloads use binary WebSocket frames carrying MessagePack Protocol v1
// bytes. wssSession never mutates room state directly; it passes raw bytes upward
// through the PacketHandler boundary.
type wssSession struct {
	id        string
	conn      *gorillaws.Conn
	server    *WSSServer
	sendCh    chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	logger    *slog.Logger
}

// ID returns the unique session identifier.
func (s *wssSession) ID() string { return s.id }

// RemoteAddr returns the remote network address of the WebSocket client.
func (s *wssSession) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }

// IsClosed reports whether the session has been closed.
func (s *wssSession) IsClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

// Send queues data for delivery to the client as a binary WebSocket frame.
// Returns an error if the session is closed or the send queue is full.
func (s *wssSession) Send(data []byte) error {
	if s.IsClosed() {
		return net.ErrClosed
	}
	select {
	case s.sendCh <- data:
		return nil
	default:
		return net.ErrClosed
	}
}

// Close terminates the session. Idempotent; safe to call multiple times.
func (s *wssSession) Close() error {
	s.close()
	return nil
}

func (s *wssSession) close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		_ = s.conn.Close()
		s.server.removeSession(s.id)
		s.logger.Debug("websocket: session closed")
	})
}

// readLoop reads binary frames from the WebSocket connection and dispatches them
// to the server's PacketHandler. Any error (including read deadline expiry) closes
// the session. Text frames are rejected with immediate close.
func (s *wssSession) readLoop(ctx context.Context, srv *WSSServer) {
	defer srv.wg.Done()
	defer s.close()

	srv.wg.Add(1)
	go s.writeLoop(ctx, srv)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closed:
			return
		default:
		}

		if err := s.conn.SetReadDeadline(time.Now().Add(srv.cfg.readTimeout())); err != nil {
			s.logger.Debug("websocket: set read deadline failed", slog.String("err", err.Error()))
			return
		}

		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			if !gorillaws.IsCloseError(err, gorillaws.CloseNormalClosure, gorillaws.CloseGoingAway) {
				s.logger.Debug("websocket: read error", slog.String("err", err.Error()))
			}
			return
		}

		// Text frames are invalid on the realtime data plane.
		// All gameplay packets must be binary MessagePack frames.
		if msgType == gorillaws.TextMessage {
			s.logger.Warn("websocket: text frame rejected; realtime packets must use binary frames")
			return
		}

		if msgType != gorillaws.BinaryMessage {
			continue
		}

		if len(data) > 0 {
			srv.handler.HandlePacket(s, data)
		}
	}
}

// writeLoop drains the send channel and writes each payload as a binary WebSocket frame.
func (s *wssSession) writeLoop(ctx context.Context, srv *WSSServer) {
	defer srv.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closed:
			drainSendCh(s.sendCh)
			return
		case data := <-s.sendCh:
			if err := s.conn.SetWriteDeadline(time.Now().Add(srv.cfg.writeTimeout())); err != nil {
				return
			}
			if err := s.conn.WriteMessage(gorillaws.BinaryMessage, data); err != nil {
				s.logger.Debug("websocket: write error", slog.String("err", err.Error()))
				return
			}
		}
	}
}

func drainSendCh(ch chan []byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
