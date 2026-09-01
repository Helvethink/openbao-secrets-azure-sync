package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"openbao-kv-azure-sync/internal/httpauth"
	"openbao-kv-azure-sync/internal/metrics"
	controller "openbao-kv-azure-sync/internal/sync"
	"openbao-kv-azure-sync/internal/watcher"
)

const maxSyncRequestBodySize = 1 << 20

// Server exposes operational endpoints for metrics and manual reconciliation.
type Server struct {
	listenAddress string
	authToken     string
	controller    *controller.Controller
	metrics       *metrics.Collector
	logger        *slog.Logger
}

// Options configures the operational API server.
type Options struct {
	ListenAddress string
	AuthToken     string
	Controller    *controller.Controller
	Metrics       *metrics.Collector
	Logger        *slog.Logger
}

// NewServer creates an operational API server.
func NewServer(options Options) *Server {
	return &Server{
		listenAddress: options.ListenAddress,
		authToken:     options.AuthToken,
		controller:    options.Controller,
		metrics:       options.Metrics,
		logger:        options.Logger,
	}
}

// Run starts the API server until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleHealth)
	mux.Handle("/metrics", s.metrics)
	mux.HandleFunc("/sync", s.handleSync)

	server := &http.Server{
		Addr:              s.listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errs := make(chan error, 1)
	go func() {
		s.logger.Info("api server listening", "address", s.listenAddress)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
		close(errs)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errs:
		return err
	}
}

func (s *Server) handleHealth(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", "GET")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(response).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleSync(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", "POST")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorized(request) {
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer request.Body.Close()
	request.Body = http.MaxBytesReader(response, request.Body, maxSyncRequestBodySize)

	var body struct {
		Mount     string `json:"mount"`
		Path      string `json:"path"`
		Namespace string `json:"namespace"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(response, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(response, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(response, "invalid request body", http.StatusBadRequest)
		return
	}
	body.Mount = strings.Trim(body.Mount, "/")
	body.Path = strings.Trim(body.Path, "/")
	body.Namespace = strings.Trim(body.Namespace, "/")
	if body.Mount == "" || body.Path == "" {
		http.Error(response, "mount and path are required", http.StatusBadRequest)
		return
	}

	err := s.controller.Reconcile(request.Context(), watcher.Event{
		Mount:     body.Mount,
		Path:      body.Path,
		Namespace: body.Namespace,
		Operation: "api_resync",
	})
	if err != nil {
		s.logger.Error("manual reconciliation failed",
			"namespace", body.Namespace,
			"mount", body.Mount,
			"path", body.Path,
			"error", err,
		)
		http.Error(response, "reconciliation failed", http.StatusInternalServerError)
		return
	}
	response.WriteHeader(http.StatusAccepted)
}

func (s *Server) authorized(request *http.Request) bool {
	if s.authToken == "" {
		return true
	}
	if httpauth.TokenMatches(request.Header.Get("X-OpenBao-Sync-Token"), s.authToken) {
		return true
	}
	const bearerPrefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	return strings.HasPrefix(authorization, bearerPrefix) &&
		httpauth.TokenMatches(strings.TrimPrefix(authorization, bearerPrefix), s.authToken)
}
