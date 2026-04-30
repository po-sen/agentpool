package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/po-sen/agentpool/internal/runtime/logger"
)

// Server wraps the standard library HTTP server with lifecycle behavior.
type Server struct {
	server *http.Server
	logger *logger.Logger
}

// New creates an HTTP server with conservative timeouts.
func New(addr string, handler http.Handler, log *logger.Logger) *Server {
	return &Server{
		server: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: log,
	}
}

// Run starts the server and shuts it down when the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil && s.logger != nil {
			s.logger.Errorf("http server shutdown failed: %v", err)
		}
	}()

	if s.logger != nil {
		s.logger.Infof("http server listening on %s", s.server.Addr)
	}

	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}
