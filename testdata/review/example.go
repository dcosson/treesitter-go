package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Server handles HTTP requests with middleware support.
type Server struct {
	mu       sync.RWMutex
	handlers map[string]http.HandlerFunc
	port     int
}

// Option configures a Server.
type Option func(*Server)

// WithPort sets the server port.
func WithPort(port int) Option {
	return func(s *Server) {
		s.port = port
	}
}

// New creates a Server with the given options.
func New(opts ...Option) *Server {
	s := &Server{
		handlers: make(map[string]http.HandlerFunc),
		port:     8080,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Handle registers a handler for the given path.
func (s *Server) Handle(path string, handler http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[path] = handler
}

// Start begins listening. It blocks until the context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{Addr: addr}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	return srv.ListenAndServe()
}
