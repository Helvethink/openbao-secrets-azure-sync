package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openbao-kv-azure-sync/internal/openbao"
	controller "openbao-kv-azure-sync/internal/sync"
)

type failingSource struct{}

func (failingSource) ReadMetadata(context.Context, string, string) (openbao.Metadata, error) {
	return openbao.Metadata{}, errors.New("sensitive upstream detail")
}

func (failingSource) ReadData(context.Context, string, string) (openbao.SecretData, error) {
	return openbao.SecretData{}, errors.New("unexpected data read")
}

func TestServerAuthorized(t *testing.T) {
	server := NewServer(Options{AuthToken: "secret-token"})
	tests := []struct {
		name          string
		header        string
		authorization string
		want          bool
	}{
		{name: "custom header", header: "secret-token", want: true},
		{name: "bearer token", authorization: "Bearer secret-token", want: true},
		{name: "missing", want: false},
		{name: "wrong custom header", header: "other-token", want: false},
		{name: "wrong bearer scheme", authorization: "Basic secret-token", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/sync", nil)
			request.Header.Set("X-OpenBao-Sync-Token", test.header)
			request.Header.Set("Authorization", test.authorization)
			if got := server.authorized(request); got != test.want {
				t.Fatalf("authorized() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestHandleSyncRejectsInvalidRequestBody(t *testing.T) {
	server := NewServer(Options{AuthToken: "token"})
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "invalid JSON", body: `{`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"mount":"secret","path":"app/db","extra":true}`, wantStatus: http.StatusBadRequest},
		{name: "trailing JSON", body: `{"mount":"secret","path":"app/db"}{}`, wantStatus: http.StatusBadRequest},
		{name: "too large", body: `{"mount":"` + strings.Repeat("a", maxSyncRequestBodySize) + `"}`, wantStatus: http.StatusRequestEntityTooLarge},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/sync", strings.NewReader(test.body))
			request.Header.Set("X-OpenBao-Sync-Token", "token")
			response := httptest.NewRecorder()

			server.handleSync(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestHandleSyncDoesNotExposeReconciliationError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctrl := controller.New(controller.Options{Source: failingSource{}, Logger: logger})
	server := NewServer(Options{AuthToken: "token", Controller: ctrl, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/sync", strings.NewReader(`{"mount":"secret","path":"app/db"}`))
	request.Header.Set("X-OpenBao-Sync-Token", "token")
	response := httptest.NewRecorder()

	server.handleSync(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := response.Body.String(); got != "reconciliation failed\n" {
		t.Fatalf("body = %q, want generic reconciliation error", got)
	}
}
