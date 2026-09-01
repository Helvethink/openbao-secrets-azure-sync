package watcher

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"openbao-kv-azure-sync/internal/httpauth"
)

const (
	maxAuditRequestBodySize = 10 << 20
	maxAuditRecordSize      = 1 << 20
)

type HTTPDeviceOptions struct {
	ListenAddress string
	Path          string
	AuthToken     string
	Mounts        []string
	Namespaces    []string
	Logger        *slog.Logger
}

// HTTPDeviceWatcher exposes an HTTP endpoint compatible with OpenBao's HTTP
// audit device.
type HTTPDeviceWatcher struct {
	listenAddress string
	path          string
	authToken     string
	logger        *slog.Logger
	parser        auditRecordParser
	events        chan Event
}

// NewHTTPDeviceWatcher creates an HTTP audit-device watcher.
func NewHTTPDeviceWatcher(options HTTPDeviceOptions) *HTTPDeviceWatcher {
	return &HTTPDeviceWatcher{
		listenAddress: options.ListenAddress,
		path:          options.Path,
		authToken:     options.AuthToken,
		logger:        options.Logger,
		parser:        newAuditRecordParser(options.Mounts, options.Namespaces, options.Logger),
		events:        make(chan Event, 128),
	}
}

// Watch starts the HTTP server and returns the audit events it accepts.
func (w *HTTPDeviceWatcher) Watch(ctx context.Context) (<-chan Event, <-chan error) {
	errs := make(chan error, 1)
	mux := http.NewServeMux()
	mux.Handle(w.path, w)

	server := &http.Server{
		Addr:              w.listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		defer close(errs)
		w.logger.Info("http audit device watcher listening", "address", w.listenAddress, "path", w.path)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			w.logger.Error("shutdown http audit device watcher", "error", err)
		}
		close(w.events)
	}()

	return w.events, errs
}

// ServeHTTP receives audit records from OpenBao.
//
// It waits for the controller ACK before responding so OpenBao's synchronous
// HTTP audit device remains blocked until reconciliation is complete.
func (w *HTTPDeviceWatcher) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost && request.Method != http.MethodPut {
		response.Header().Set("Allow", "POST, PUT")
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !w.authorized(request) {
		http.Error(response, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer request.Body.Close()
	request.Body = http.MaxBytesReader(response, request.Body, maxAuditRequestBodySize)

	accepted, err := w.accept(request.Context(), request.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(response, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.logger.Error("process audit request", "error", err)
		http.Error(response, "audit processing failed", http.StatusInternalServerError)
		return
	}
	if accepted == 0 {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	response.WriteHeader(http.StatusAccepted)
}

// accept parses newline-delimited audit records and waits for each event ACK.
func (w *HTTPDeviceWatcher) accept(ctx context.Context, body io.Reader) (int, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), maxAuditRecordSize)
	accepted := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.logger.Info("received audit record")
		event, ok := w.parser.parseLine(line)
		if !ok {
			continue
		}
		event.Ack = make(chan error, 1)
		select {
		case w.events <- event:
		case <-ctx.Done():
			return accepted, ctx.Err()
		default:
			return accepted, errors.New("event buffer is full")
		}
		select {
		case err := <-event.Ack:
			if err != nil {
				return accepted, err
			}
			accepted++
		case <-ctx.Done():
			return accepted, ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		return accepted, err
	}
	return accepted, nil
}

// authorized checks the optional shared secret used by the local audit device.
func (w *HTTPDeviceWatcher) authorized(request *http.Request) bool {
	if w.authToken == "" {
		return true
	}
	if httpauth.TokenMatches(request.Header.Get("X-OpenBao-Audit-Token"), w.authToken) {
		return true
	}
	const bearerPrefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	return strings.HasPrefix(authorization, bearerPrefix) &&
		httpauth.TokenMatches(strings.TrimPrefix(authorization, bearerPrefix), w.authToken)
}
