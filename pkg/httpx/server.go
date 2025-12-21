// Package httpx provides HTTP server utilities and helpers for Kedastral services.
package httpx

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	kedastraltls "github.com/HatiCode/kedastral/pkg/tls"
)

// Server wraps http.Server with graceful shutdown capabilities.
// It provides a clean API for starting and stopping HTTP servers.
type Server struct {
	server *http.Server
	logger *slog.Logger
}

// NewServer creates a new HTTP server that listens on the specified address.
// The handler can be nil to use http.DefaultServeMux.
func NewServer(addr string, handler http.Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	return &Server{
		server: &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: logger,
	}
}

// SetTLSConfig configures the server to use TLS with the provided configuration.
// Must be called before Start() or StartTLS().
func (s *Server) SetTLSConfig(config *tls.Config) {
	s.server.TLSConfig = config
}

// Start begins serving HTTP requests. This method blocks until the server is stopped.
// Returns an error if the server fails to start or is stopped ungracefully.
func (s *Server) Start() error {
	s.logger.Info("starting HTTP server", "addr", s.server.Addr)
	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

// StartTLS begins serving HTTPS requests with the provided cert and key files.
// This method blocks until the server is stopped.
// Returns an error if the server fails to start or is stopped ungracefully.
func (s *Server) StartTLS(certFile, keyFile string) error {
	s.logger.Info("starting HTTPS server", "addr", s.server.Addr)
	err := s.server.ListenAndServeTLS(certFile, keyFile)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the server.
// It waits up to the specified timeout for active connections to complete.
func (s *Server) Stop(timeout time.Duration) error {
	s.logger.Info("stopping HTTP server", "timeout", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	s.logger.Info("HTTP server stopped gracefully")
	return nil
}

// ErrorResponse represents a JSON error response.
// This provides consistent error formatting across all endpoints.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteJSON writes a JSON response with the specified status code.
// The value v is marshaled to JSON and written to the response writer.
// If marshaling fails, a 500 Internal Server Error is written instead.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// WriteError writes a JSON error response with the specified status code.
// The error message is extracted from the error and wrapped in an ErrorResponse.
// Error responses should use format: {"error":"<msg>"}
func WriteError(w http.ResponseWriter, status int, err error) {
	resp := ErrorResponse{
		Error: err.Error(),
	}
	if jsonErr := WriteJSON(w, status, resp); jsonErr != nil {
		slog.Error("failed to write error response", "error", jsonErr, "original_error", err)
	}
}

// WriteErrorMessage writes a JSON error response with a custom message.
func WriteErrorMessage(w http.ResponseWriter, status int, message string) {
	resp := ErrorResponse{
		Error: message,
	}
	if err := WriteJSON(w, status, resp); err != nil {
		slog.Error("failed to write error message", "error", err, "message", message)
	}
}

// HealthHandler returns an http.Handler that always responds with 200 OK.
// This can be used for basic health checks that don't require any logic.
//
// For more complex health checks (e.g., checking staleness), create a custom handler.
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			slog.Error("failed to write health response", "error", err)
		}
	}
}

// HealthHandlerWithCheck returns an http.Handler that calls a check function.
// If the check returns an error, a 503 Service Unavailable is returned.
// Otherwise, returns 200 OK.
func HealthHandlerWithCheck(check func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := check(); err != nil {
			WriteError(w, http.StatusServiceUnavailable, err)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			slog.Error("failed to write health response", "error", err)
		}
	}
}

// LoggingMiddleware returns middleware that logs HTTP requests.
// It logs the method, path, status code, and duration of each request.
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			logger.Info("HTTP request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// RecoveryMiddleware returns middleware that recovers from panics in HTTP handlers.
// If a panic occurs, it logs the error and returns a 500 Internal Server Error.
func RecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered",
						"error", err,
						"method", r.Method,
						"path", r.URL.Path,
					)
					WriteErrorMessage(w, http.StatusInternalServerError, "internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// NewClient creates an HTTP client with optional TLS configuration.
// If tlsCfg.Enabled is false, a standard HTTP client is created.
// If tlsCfg.Enabled is true, the client will use mTLS for HTTPS connections.
func NewClient(tlsCfg kedastraltls.Config, timeout time.Duration) (*http.Client, error) {
	var cryptoTLSConfig *tls.Config
	var err error

	if tlsCfg.Enabled {
		cryptoTLSConfig, err = kedastraltls.NewClientTLSConfig(tlsCfg.CertFile, tlsCfg.KeyFile, tlsCfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("create TLS config: %w", err)
		}
	}

	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig:     cryptoTLSConfig,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}
