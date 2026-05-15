package kcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	kcp "github.com/xtaci/kcp-go"
)

const (
	defaultReadDeadline  = 10 * time.Second
	defaultWriteDeadline = 5 * time.Second
	defaultSendQueueSize = 256
)

// ServerConfig holds KCP server configuration.
type ServerConfig struct {
	// ListenAddr is the UDP address to listen on (e.g. ":9000").
	ListenAddr string

	// MaxPacketSize is the maximum allowed inbound packet size in bytes.
	// Packets exceeding this are dropped. Defaults to protocol.MaxPacketSize (64 KB)
	// if zero.
	MaxPacketSize int

	// ReadTimeout sets the read deadline per KCP read operation.
	// Zero means the default (10s).
	ReadTimeout time.Duration

	// WriteTimeout sets the write deadline per KCP write operation.
	// Zero means the default (5s).
	WriteTimeout time.Duration

	// SendQueueSize is the buffered channel size per session for outbound packets.
	// Zero means the default (256).
	SendQueueSize int

	// Logger is used for structured logging. nil means slog.Default().
	Logger *slog.Logger
}

func (c *ServerConfig) maxPacketSize() int {
	if c.MaxPacketSize <= 0 {
		return 64 * 1024
	}
	return c.MaxPacketSize
}

func (c *ServerConfig) readTimeout() time.Duration {
	if c.ReadTimeout <= 0 {
		return defaultReadDeadline
	}
	return c.ReadTimeout
}

func (c *ServerConfig) writeTimeout() time.Duration {
	if c.WriteTimeout <= 0 {
		return defaultWriteDeadline
	}
	return c.WriteTimeout
}

func (c *ServerConfig) sendQueueSize() int {
	if c.SendQueueSize <= 0 {
		return defaultSendQueueSize
	}
	return c.SendQueueSize
}

func (c *ServerConfig) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// Validate checks the server config for errors.
func (c *ServerConfig) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("kcp: ListenAddr is required")
	}
	if _, err := net.ResolveUDPAddr("udp", c.ListenAddr); err != nil {
		return fmt.Errorf("kcp: invalid listen address %q: %w", c.ListenAddr, err)
	}
	return nil
}

// sessionCount is used to generate unique session IDs.
var sessionCount atomic.Uint64

// nextSessionID returns a unique session identifier.
func nextSessionID() string {
	return fmt.Sprintf("sess-%d", sessionCount.Add(1))
}

// KCPServer listens for KCP connections and dispatches packets to a handler.
type KCPServer struct {
	cfg      ServerConfig
	handler  PacketHandler
	listener *kcp.Listener
	sessions map[string]*kcpSession
	mu       sync.Mutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	logger   *slog.Logger
	running  bool
}

// NewServer creates a new KCP server with the given config and packet handler.
func NewServer(cfg ServerConfig, handler PacketHandler) (*KCPServer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, fmt.Errorf("kcp: handler is required")
	}
	return &KCPServer{
		cfg:      cfg,
		handler:  handler,
		sessions: make(map[string]*kcpSession),
		logger:   cfg.logger(),
	}, nil
}

// Addr returns the server's listen address. Only valid after Start.
func (s *KCPServer) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Start begins accepting KCP connections. It blocks until the listener is ready,
// then spawns the accept loop in a background goroutine.
func (s *KCPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("kcp: server already running")
	}

	ln, err := kcp.ListenWithOptions(s.cfg.ListenAddr, nil, 0, 0)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("kcp: listen on %s: %w", s.cfg.ListenAddr, err)
	}
	s.listener = ln
	s.running = true

	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	s.logger.Info("kcp server listening", slog.String("addr", ln.Addr().String()))

	s.wg.Add(1)
	go s.acceptLoop(ctx)

	return nil
}

// Stop gracefully shuts down the server. It closes all sessions, stops
// the listener, and waits for goroutines to finish.
func (s *KCPServer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false

	if s.cancel != nil {
		s.cancel()
	}

	// Close listener first so accept loop exits.
	if s.listener != nil {
		s.listener.Close()
	}

	// Snapshot sessions to close outside lock.
	sessions := make([]*kcpSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()

	for _, sess := range sessions {
		sess.close()
	}

	s.wg.Wait()
	s.logger.Info("kcp server stopped")
}

// SessionCount returns the number of active sessions.
func (s *KCPServer) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *KCPServer) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		conn, err := s.listener.AcceptKCP()
		if err != nil {
			// Context cancelled or listener closed — expected during shutdown.
			select {
			case <-ctx.Done():
				return
			default:
			}
			s.logger.Error("kcp accept error", slog.String("err", err.Error()))
			return
		}

		sess := s.newSession(conn)
		s.mu.Lock()
		if !s.running {
			s.mu.Unlock()
			sess.close()
			return
		}
		s.sessions[sess.id] = sess
		s.mu.Unlock()

		s.wg.Add(1)
		go sess.readLoop(ctx, s)
	}
}

func (s *KCPServer) newSession(conn *kcp.UDPSession) *kcpSession {
	return &kcpSession{
		id:       nextSessionID(),
		conn:     conn,
		server:   s,
		sendCh:   make(chan []byte, s.cfg.sendQueueSize()),
		closed:   make(chan struct{}),
		logger:   s.logger.With(slog.String("session", conn.RemoteAddr().String())),
	}
}

func (s *KCPServer) removeSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}
