package websocket

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	gorillaws "github.com/gorilla/websocket"
)

const (
	defaultReadTimeout    = 10 * time.Second
	defaultWriteTimeout   = 5 * time.Second
	defaultSendQueueSize  = 256
	defaultMaxPayloadSize = int64(60 * 1024) // 60 KB per docs/protocol.md MaxPayloadSize
)

// ServerConfig holds WebSocket server configuration.
//
// WSS (TLS) is required for all public production endpoints. When DevMode is true,
// unencrypted WS is permitted and origin checking is relaxed. In single-vps production,
// TLS termination is typically handled by a reverse proxy (nginx, caddy, etc.).
type ServerConfig struct {
	// ListenAddr is the TCP address to listen on (e.g. ":9001"). Required.
	ListenAddr string

	// TLSCertFile and TLSKeyFile enable WSS directly without a reverse proxy.
	// Optional: production deployments may instead terminate TLS upstream.
	// When DevMode is false and these are empty, the server accepts unencrypted WS
	// but this MUST only be used behind a TLS-terminating reverse proxy in production.
	TLSCertFile string
	TLSKeyFile  string

	// AllowedOrigins is an allowlist of permitted WebSocket origins.
	// Connections from unlisted origins are rejected during the HTTP upgrade.
	// Ignored in DevMode or when empty.
	AllowedOrigins []string

	// MaxPayloadSize is the maximum inbound binary frame payload in bytes.
	// Defaults to 60 KB per docs/protocol.md MaxPayloadSize.
	// Frames exceeding this limit cause the session to be closed.
	MaxPayloadSize int64

	// ReadTimeout is the per-read deadline (max time between incoming frames).
	// Defaults to 10s. Expiry closes the session.
	ReadTimeout time.Duration

	// WriteTimeout is the per-write deadline. Defaults to 5s.
	WriteTimeout time.Duration

	// SendQueueSize is the buffered channel depth per session for outbound packets.
	// Defaults to 256. Full queue causes movement delta packets to be dropped;
	// control messages cause the session to be considered unhealthy.
	SendQueueSize int

	// Logger is used for structured logging. nil means slog.Default().
	Logger *slog.Logger

	// DevMode allows unencrypted WS and relaxes origin checking.
	// Must be false in all production deployments.
	DevMode bool
}

// Validate checks the server config for basic correctness.
func (c *ServerConfig) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("websocket: ListenAddr is required")
	}
	if _, err := net.ResolveTCPAddr("tcp", c.ListenAddr); err != nil {
		return fmt.Errorf("websocket: invalid listen address %q: %w", c.ListenAddr, err)
	}
	return nil
}

func (c *ServerConfig) maxPayloadSize() int64 {
	if c.MaxPayloadSize <= 0 {
		return defaultMaxPayloadSize
	}
	return c.MaxPayloadSize
}

func (c *ServerConfig) readTimeout() time.Duration {
	if c.ReadTimeout <= 0 {
		return defaultReadTimeout
	}
	return c.ReadTimeout
}

func (c *ServerConfig) writeTimeout() time.Duration {
	if c.WriteTimeout <= 0 {
		return defaultWriteTimeout
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

var wssSessionCount atomic.Uint64

func nextWSSSessionID() string {
	return fmt.Sprintf("wss-%d", wssSessionCount.Add(1))
}

// WSSServer listens for WebSocket connections and dispatches binary frame payloads
// to a PacketHandler.
//
// All realtime packets use binary WebSocket frames carrying MessagePack Protocol v1
// bytes. Text frames are rejected and the session is closed immediately.
// The server never mutates room state directly.
type WSSServer struct {
	cfg      ServerConfig
	handler  PacketHandler
	upgrader gorillaws.Upgrader
	httpSrv  *http.Server
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	sessions map[string]*wssSession
	mu       sync.Mutex
	wg       sync.WaitGroup
	logger   *slog.Logger
	running  bool
}

// NewServer creates a new WebSocket server with the given config and packet handler.
func NewServer(cfg ServerConfig, handler PacketHandler) (*WSSServer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, fmt.Errorf("websocket: handler is required")
	}

	s := &WSSServer{
		cfg:      cfg,
		handler:  handler,
		sessions: make(map[string]*wssSession),
		logger:   cfg.logger(),
	}
	s.upgrader = gorillaws.Upgrader{
		ReadBufferSize:  int(cfg.maxPayloadSize()),
		WriteBufferSize: int(cfg.maxPayloadSize()),
		CheckOrigin:     s.checkOrigin,
	}
	return s, nil
}

// checkOrigin validates the request origin against the configured allowlist.
// In DevMode or with an empty allowlist, all origins are permitted.
func (s *WSSServer) checkOrigin(r *http.Request) bool {
	if s.cfg.DevMode || len(s.cfg.AllowedOrigins) == 0 {
		return true
	}
	origin := r.Header.Get("Origin")
	for _, allowed := range s.cfg.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	s.logger.Warn("websocket: origin rejected", slog.String("origin", origin))
	return false
}

// Addr returns the server's listen address. Only valid after Start.
func (s *WSSServer) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// SessionCount returns the number of active WebSocket sessions.
func (s *WSSServer) SessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// Start binds the listener and begins accepting WebSocket connections.
// It returns once the listener is ready; the serve loop runs in the background.
func (s *WSSServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("websocket: server already running")
	}

	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("websocket: listen on %s: %w", s.cfg.ListenAddr, err)
	}

	s.listener = ln
	s.running = true
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/realtime", s.handleUpgrade)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	s.httpSrv = &http.Server{Handler: mux}
	s.logger.Info("websocket server listening", slog.String("addr", ln.Addr().String()))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			select {
			case <-s.ctx.Done():
			default:
				s.logger.Error("websocket: serve error", slog.String("err", err.Error()))
			}
		}
	}()

	return nil
}

// Stop gracefully shuts down the server: cancels the session context, closes all
// active sessions, shuts down the HTTP listener, and waits for all goroutines.
func (s *WSSServer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false

	if s.cancel != nil {
		s.cancel()
	}

	sessions := make([]*wssSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.mu.Unlock()

	for _, sess := range sessions {
		sess.close()
	}

	if s.httpSrv != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutCtx)
	}

	s.wg.Wait()
	s.logger.Info("websocket: server stopped")
}

// handleUpgrade upgrades the HTTP connection to a WebSocket session and starts
// the session read/write goroutines.
func (s *WSSServer) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Debug("websocket: upgrade failed", slog.String("err", err.Error()))
		return
	}
	conn.SetReadLimit(s.cfg.maxPayloadSize())

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
	go sess.readLoop(s.ctx, s)
}

func (s *WSSServer) newSession(conn *gorillaws.Conn) *wssSession {
	return &wssSession{
		id:     nextWSSSessionID(),
		conn:   conn,
		server: s,
		sendCh: make(chan []byte, s.cfg.sendQueueSize()),
		closed: make(chan struct{}),
		logger: s.logger.With(slog.String("session", conn.RemoteAddr().String())),
	}
}

func (s *WSSServer) removeSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}
