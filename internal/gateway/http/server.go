package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/resolver"
	"github.com/thaonguyen/realtime-room-middleware/internal/gateway/token"
	"github.com/thaonguyen/realtime-room-middleware/internal/protocol"
)

// Server is the Gateway HTTP control-plane server.
type Server struct {
	httpServer *http.Server
	resolver   resolver.NodeResolver
	tokenGen   *token.Generator
	logger     *slog.Logger
}

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Addr            string
	Resolver        resolver.NodeResolver
	TokenGenerator  *token.Generator
	Logger          *slog.Logger
}

// NewServer creates a new Gateway HTTP server.
func NewServer(cfg ServerConfig) *Server {
	s := &Server{
		resolver: cfg.Resolver,
		tokenGen: cfg.TokenGenerator,
		logger:   cfg.Logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("POST /join", s.handleJoin)

	s.httpServer = &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}

	return s
}

// Start begins serving HTTP requests. Blocks until the server exits.
func (s *Server) Start() error {
	s.logger.Info("gateway http server starting", slog.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("gateway http server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the server's listen address. Useful after Start with ":0".
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// Handler returns the HTTP handler for testing with httptest.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// supportedVersionRange returns the protocol version range this gateway accepts.
func supportedVersionRange() (uint16, uint16) {
	return protocol.MinVersion, protocol.MaxVersion
}
