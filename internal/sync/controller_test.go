package sync

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"openbao-kv-azure-sync/internal/azure"
	"openbao-kv-azure-sync/internal/metrics"
	"openbao-kv-azure-sync/internal/openbao"
	"openbao-kv-azure-sync/internal/watcher"
)

func TestReconcileSkipsDisabledSync(t *testing.T) {
	source := &fakeSource{
		metadata: openbao.Metadata{Custom: map[string]string{
			"sync_target_enabled": "false",
		}},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Namespace: "finance",
		Operation: "update",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if source.readDataCalls != 0 {
		t.Fatalf("ReadData calls = %d, want 0", source.readDataCalls)
	}
	if len(destination.secrets) != 0 || len(destination.certificates) != 0 {
		t.Fatalf("destination received writes: secrets=%d certificates=%d", len(destination.secrets), len(destination.certificates))
	}
}

func TestRunAcknowledgesEventAfterReconcile(t *testing.T) {
	source := &fakeSource{
		metadata: openbao.Metadata{Custom: map[string]string{
			"sync_target_enabled": "true",
			"sync_target_type":    "dryrun",
		}},
		data: openbao.SecretData{
			Version: 1,
			Data: map[string]any{
				"username": "demo",
			},
		},
	}
	destination := &fakeDestination{}
	fakeWatcher := newFakeWatcher()
	controller := New(Options{
		Watcher:        fakeWatcher,
		Source:         source,
		SourceInstance: "test-openbao",
		Destination:    destination,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func() {
		errs <- controller.Run(ctx)
	}()

	ack := make(chan error, 1)
	fakeWatcher.events <- watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Operation: "delete",
		Ack:       ack,
	}

	select {
	case err := <-ack:
		if err != nil {
			t.Fatalf("ack error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ack")
	}

	cancel()
	close(fakeWatcher.events)
	close(fakeWatcher.errs)
	if err := <-errs; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(destination.deletes) != 1 {
		t.Fatalf("secret deletes = %d, want 1", len(destination.deletes))
	}
}

func TestReconcilePutsOneSecretPerKVEntry(t *testing.T) {
	source := &fakeSource{
		metadata: syncMetadata("azure_key_vault/secret", ""),
		data: openbao.SecretData{
			Version: 7,
			Data: map[string]any{
				"password": "plain-secret",
				"retries":  float64(3),
			},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Namespace: "finance",
		Operation: "update",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(destination.secrets) != 2 {
		t.Fatalf("secret writes = %d, want 2", len(destination.secrets))
	}

	payload := destination.secrets[0]
	if payload.Mount != "secret" || payload.Path != "app/db" || payload.Version != 7 {
		t.Fatalf("payload identity = (%q, %q, %d), want (secret, app/db, 7)", payload.Mount, payload.Path, payload.Version)
	}
	if payload.Key != "password" {
		t.Fatalf("Key = %q, want password", payload.Key)
	}
	if payload.RemoteName != "synced-app-db-password" {
		t.Fatalf("RemoteName = %q, want synced-app-db-password", payload.RemoteName)
	}
	if payload.Metadata.TargetName != "team-vault" {
		t.Fatalf("TargetName = %q, want team-vault", payload.Metadata.TargetName)
	}
	if string(payload.Transform.Value) != `"plain-secret"` {
		t.Fatalf("Transform.Value = %q, want JSON string", string(payload.Transform.Value))
	}
	if payload.Transform.ContentType != "application/json" {
		t.Fatalf("ContentType = %q, want application/json", payload.Transform.ContentType)
	}
	if payload.SourceInstance != "test-openbao" {
		t.Fatalf("SourceInstance = %q, want test-openbao", payload.SourceInstance)
	}
	if payload.Namespace != "finance" {
		t.Fatalf("Namespace = %q, want finance", payload.Namespace)
	}

	jsonPayload := destination.secrets[1]
	if jsonPayload.Key != "retries" {
		t.Fatalf("Key = %q, want retries", jsonPayload.Key)
	}
	if string(jsonPayload.Transform.Value) != "3" {
		t.Fatalf("Transform.Value = %q, want 3", string(jsonPayload.Transform.Value))
	}
	if jsonPayload.Transform.ContentType != "application/json" {
		t.Fatalf("ContentType = %q, want application/json", jsonPayload.Transform.ContentType)
	}
}

func TestReconcileUsesRawFormatForStringEntries(t *testing.T) {
	source := &fakeSource{
		metadata: syncMetadata("azure_key_vault/secret", "raw"),
		data: openbao.SecretData{
			Version: 1,
			Data:    map[string]any{"password": "plain-secret"},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	if err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Namespace: "finance",
		Operation: "update",
	}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := string(destination.secrets[0].Transform.Value); got != "plain-secret" {
		t.Fatalf("raw value = %q, want plain-secret", got)
	}
	if got := destination.secrets[0].Transform.ContentType; got != "text/plain" {
		t.Fatalf("raw content type = %q, want text/plain", got)
	}
}

func TestReconcilePutsDryRunSecret(t *testing.T) {
	source := &fakeSource{
		metadata: openbao.Metadata{Custom: map[string]string{
			"sync_target_enabled": "true",
			"sync_target_type":    "dryrun",
		}},
		data: openbao.SecretData{
			Version: 2,
			Data: map[string]any{
				"username": "demo",
			},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Operation: "update",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(destination.secrets) != 1 {
		t.Fatalf("secret writes = %d, want 1", len(destination.secrets))
	}
	if destination.secrets[0].Metadata.TargetProvider != "dryrun" {
		t.Fatalf("TargetProvider = %q, want dryrun", destination.secrets[0].Metadata.TargetProvider)
	}
	if destination.secrets[0].RemoteName != "app-db-username" {
		t.Fatalf("RemoteName = %q, want app-db-username", destination.secrets[0].RemoteName)
	}
}

func TestReconcileDeletesOneSecretPerKVEntry(t *testing.T) {
	source := &fakeSource{
		metadata: syncMetadata("azure_key_vault/secret", ""),
		data: openbao.SecretData{
			Version: 8,
			Data: map[string]any{
				"password": "plain-secret",
				"username": "demo",
			},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Operation: "delete",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(destination.secrets) != 0 {
		t.Fatalf("secret writes = %d, want 0", len(destination.secrets))
	}
	if len(destination.deletes) != 2 {
		t.Fatalf("secret deletes = %d, want 2", len(destination.deletes))
	}
	if destination.deletes[0].RemoteName != "synced-app-db-password" {
		t.Fatalf("first RemoteName = %q, want synced-app-db-password", destination.deletes[0].RemoteName)
	}
	if destination.deletes[1].RemoteName != "synced-app-db-username" {
		t.Fatalf("second RemoteName = %q, want synced-app-db-username", destination.deletes[1].RemoteName)
	}
}

func TestReconcileDeletesRemovedKVEntryOnUpdate(t *testing.T) {
	source := &fakeSource{
		metadata: syncMetadata("azure_key_vault/secret", ""),
		data: openbao.SecretData{
			Version: 3,
			Data: map[string]any{
				"username": "demo",
			},
		},
		versions: map[int]openbao.SecretData{
			2: {
				Version: 2,
				Data: map[string]any{
					"password": "plain-secret",
					"username": "demo",
				},
			},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Operation: "update",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(destination.deletes) != 1 {
		t.Fatalf("secret deletes = %d, want 1", len(destination.deletes))
	}
	if destination.deletes[0].RemoteName != "synced-app-db-password" {
		t.Fatalf("RemoteName = %q, want synced-app-db-password", destination.deletes[0].RemoteName)
	}
	if len(destination.secrets) != 1 {
		t.Fatalf("secret writes = %d, want 1", len(destination.secrets))
	}
	if destination.secrets[0].RemoteName != "synced-app-db-username" {
		t.Fatalf("written RemoteName = %q, want synced-app-db-username", destination.secrets[0].RemoteName)
	}
}

func TestReconcilePutsCertificate(t *testing.T) {
	source := &fakeSource{
		metadata: syncMetadata("azure_key_vault/certificate", ""),
		data: openbao.SecretData{
			Version: 3,
			Data: map[string]any{
				"cert":     "-----BEGIN CERTIFICATE-----",
				"key":      "-----BEGIN PRIVATE KEY-----",
				"chain":    "-----BEGIN CHAIN-----",
				"password": "pfx-password",
			},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "tls/api",
		Operation: "update",
	})
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(destination.certificates) != 1 {
		t.Fatalf("certificate writes = %d, want 1", len(destination.certificates))
	}

	payload := destination.certificates[0]
	wantCertificate := "-----BEGIN CERTIFICATE-----\n-----BEGIN PRIVATE KEY-----\n-----BEGIN CHAIN-----"
	if string(payload.Certificate) != wantCertificate {
		t.Fatalf("Certificate = %q, want %q", string(payload.Certificate), wantCertificate)
	}
	if payload.Password != "pfx-password" {
		t.Fatalf("Password = %q, want pfx-password", payload.Password)
	}
	if payload.RemoteName != "synced-tls-api" {
		t.Fatalf("RemoteName = %q, want synced-tls-api", payload.RemoteName)
	}
}

func TestReconcileReturnsCertificatePayloadError(t *testing.T) {
	source := &fakeSource{
		metadata: syncMetadata("azure_key_vault/certificate", ""),
		data: openbao.SecretData{
			Version: 1,
			Data: map[string]any{
				"cert": "-----BEGIN CERTIFICATE-----",
			},
		},
	}
	destination := &fakeDestination{}
	controller := newTestController(source, destination)

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "tls/incomplete",
		Operation: "update",
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want error")
	}
	if len(destination.certificates) != 0 {
		t.Fatalf("certificate writes = %d, want 0", len(destination.certificates))
	}
}

func TestReconcileMetricsIncludeErrorType(t *testing.T) {
	collector := metrics.NewCollector()
	source := &fakeSource{
		metadataErr: fmt.Errorf("openbao unavailable"),
	}
	controller := New(Options{
		Source:         source,
		SourceInstance: "test-openbao",
		Destination:    &fakeDestination{},
		Metrics:        collector,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	err := controller.Reconcile(context.Background(), watcher.Event{
		Mount:     "secret",
		Path:      "app/db",
		Operation: "update",
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want error")
	}

	rendered := collector.Render()
	if !strings.Contains(rendered, `openbao_sync_reconcile_total{error_type="openbao_read",operation="update",result="error",target="unknown"} 1`) {
		t.Fatalf("metrics missing openbao_read error type:\n%s", rendered)
	}
}

func newTestController(source *fakeSource, destination *fakeDestination) *Controller {
	return New(Options{
		Source:         source,
		SourceInstance: "test-openbao",
		Destination:    destination,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func syncMetadata(targetType, format string) openbao.Metadata {
	return openbao.Metadata{Custom: map[string]string{
		"sync_target_enabled": "true",
		"sync_target_type":    targetType,
		"sync_target_name":    "team-vault",
		"sync_target_prefix":  "synced",
		"sync.format":         format,
	}}
}

type fakeSource struct {
	metadata      openbao.Metadata
	metadataErr   error
	data          openbao.SecretData
	versions      map[int]openbao.SecretData
	dataErr       error
	readDataCalls int
}

func (s *fakeSource) ReadMetadata(_ context.Context, _, _ string) (openbao.Metadata, error) {
	if s.metadataErr != nil {
		return openbao.Metadata{}, s.metadataErr
	}
	return s.metadata, nil
}

func (s *fakeSource) ReadData(_ context.Context, _, _ string) (openbao.SecretData, error) {
	s.readDataCalls++
	if s.dataErr != nil {
		return openbao.SecretData{}, s.dataErr
	}
	return s.data, nil
}

func (s *fakeSource) ReadDataVersion(_ context.Context, _, _ string, version int) (openbao.SecretData, error) {
	if s.dataErr != nil {
		return openbao.SecretData{}, s.dataErr
	}
	if data, ok := s.versions[version]; ok {
		return data, nil
	}
	return openbao.SecretData{}, fmt.Errorf("version %d not found", version)
}

type fakeDestination struct {
	secrets      []azure.SecretPayload
	deletes      []azure.SecretPayload
	secretErr    error
	certificates []azure.CertificatePayload
	certErr      error
}

func (d *fakeDestination) PutSecret(_ context.Context, payload azure.SecretPayload) error {
	if d.secretErr != nil {
		return d.secretErr
	}
	d.secrets = append(d.secrets, payload)
	return nil
}

func (d *fakeDestination) DeleteSecret(_ context.Context, payload azure.SecretPayload) error {
	d.deletes = append(d.deletes, payload)
	return nil
}

func (d *fakeDestination) PutCertificate(_ context.Context, payload azure.CertificatePayload) error {
	if d.certErr != nil {
		return d.certErr
	}
	d.certificates = append(d.certificates, payload)
	return nil
}

var (
	_ Source            = (*fakeSource)(nil)
	_ azure.Destination = (*fakeDestination)(nil)
)

type fakeWatcher struct {
	events chan watcher.Event
	errs   chan error
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{
		events: make(chan watcher.Event, 1),
		errs:   make(chan error, 1),
	}
}

func (w *fakeWatcher) Watch(context.Context) (<-chan watcher.Event, <-chan error) {
	return w.events, w.errs
}
