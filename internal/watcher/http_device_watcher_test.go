package watcher

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPDeviceWatcherAcceptsAuditRecord(t *testing.T) {
	w := NewHTTPDeviceWatcher(HTTPDeviceOptions{
		ListenAddress: "127.0.0.1:0",
		Path:          "/ingress",
		AuthToken:     "token",
		Mounts:        []string{"secret"},
		Namespaces:    []string{"*"},
		Logger:        slog.Default(),
	})

	request := httptest.NewRequest(http.MethodPost, "/ingress", strings.NewReader(`{"type":"request","request":{"operation":"update","path":"secret/data/app/db"}}`))
	request.Header.Set("X-OpenBao-Audit-Token", "token")
	response := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.ServeHTTP(response, request)
	}()

	event := <-w.events
	if event.Mount != "secret" || event.Path != "app/db" || event.Operation != "update" {
		t.Fatalf("unexpected event: %+v", event)
	}
	event.Ack <- nil
	<-done

	if response.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}

func TestHTTPDeviceWatcherRejectsInvalidToken(t *testing.T) {
	w := NewHTTPDeviceWatcher(HTTPDeviceOptions{
		ListenAddress: "127.0.0.1:0",
		Path:          "/ingress",
		AuthToken:     "token",
		Mounts:        []string{"secret"},
		Namespaces:    []string{"*"},
		Logger:        slog.Default(),
	})

	request := httptest.NewRequest(http.MethodPost, "/ingress", strings.NewReader(`{"request":{"operation":"update","path":"secret/data/app/db"}}`))
	response := httptest.NewRecorder()

	w.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", response.Code)
	}
}
