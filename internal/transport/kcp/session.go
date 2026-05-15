package kcp

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	kcp "github.com/xtaci/kcp-go"
)

// kcpSession wraps a KCP connection with safe lifecycle management.
type kcpSession struct {
	id     string
	conn   *kcp.UDPSession
	server *KCPServer
	sendCh chan []byte
	closed chan struct{}
	closeOnce sync.Once
	logger *slog.Logger
}

func (s *kcpSession) ID() string       { return s.id }
func (s *kcpSession) RemoteAddr() net.Addr { return s.conn.RemoteAddr() }

// IsClosed reports whether the session has been closed.
func (s *kcpSession) IsClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

// Send queues data for delivery to the client.
// Returns an error if the session is closed or the send queue is full.
func (s *kcpSession) Send(data []byte) error {
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

// Close terminates the session. Idempotent.
func (s *kcpSession) Close() error {
	s.close()
	return nil
}

func (s *kcpSession) close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.conn.Close()
		s.server.removeSession(s.id)
		s.logger.Debug("kcp session closed")
	})
}

// readLoop reads packets from the KCP connection and dispatches them to the
// server's handler. It exits when the context is cancelled, the connection
// is closed, or a fatal read error occurs.
func (s *kcpSession) readLoop(ctx context.Context, srv *KCPServer) {
	defer srv.wg.Done()
	defer s.close()

	buf := make([]byte, srv.cfg.maxPacketSize())

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
			s.logger.Debug("kcp set read deadline failed", slog.String("err", err.Error()))
			return
		}

		n, err := s.conn.Read(buf)
		if err != nil {
			if isTimeoutOrClosed(err) {
				continue
			}
			s.logger.Debug("kcp read error", slog.String("err", err.Error()))
			return
		}

		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			srv.handler.HandlePacket(s, data)
		}
	}
}

// writeLoop drains the send channel and writes packets to the KCP connection.
func (s *kcpSession) writeLoop(ctx context.Context, srv *KCPServer) {
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
			if _, err := s.conn.Write(data); err != nil {
				s.logger.Debug("kcp write error", slog.String("err", err.Error()))
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

func isTimeoutOrClosed(err error) bool {
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	return false
}
