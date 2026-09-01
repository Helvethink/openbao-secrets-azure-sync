package watcher

import (
	"log/slog"
	"testing"
)

func TestAuditLogWatcherParsePath(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	event, ok := w.parser.parsePath("update", "secret/data/app/db")
	if !ok {
		t.Fatal("expected path to be parsed")
	}
	if event.Mount != "secret" || event.Path != "app/db" || event.Operation != "update" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestAuditLogWatcherIgnoreUnknownMount(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	if _, ok := w.parser.parsePath("update", "other/data/app/db"); ok {
		t.Fatal("expected unknown mount to be ignored")
	}
}

func TestAuditLogWatcherIgnoresReadResponses(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	line := `{"type":"response","request":{"operation":"read","path":"secret/metadata/app"}}`
	if event, ok := w.parser.parseLine(line); ok {
		t.Fatalf("expected read response to be ignored, got %+v", event)
	}
}

func TestAuditLogWatcherIgnoresReadRequests(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	line := `{"type":"request","request":{"operation":"read","path":"secret/metadata/app"}}`
	if event, ok := w.parser.parseLine(line); ok {
		t.Fatalf("expected read request to be ignored, got %+v", event)
	}
}

func TestAuditLogWatcherAcceptsDeleteRequests(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	line := `{"type":"request","request":{"operation":"delete","path":"secret/metadata/app/db"}}`
	event, ok := w.parser.parseLine(line)
	if !ok {
		t.Fatal("expected delete request to be parsed")
	}
	if event.Mount != "secret" || event.Path != "app/db" || event.Operation != "delete" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestAuditLogWatcherParsesNamespace(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	line := `{"type":"request","request":{"operation":"update","path":"secret/data/app/db","namespace":{"id":"ns_123","path":"finance/"}}}`
	event, ok := w.parser.parseLine(line)
	if !ok {
		t.Fatal("expected namespaced request to be parsed")
	}
	if event.Namespace != "finance" {
		t.Fatalf("Namespace = %q, want finance", event.Namespace)
	}
}

func TestAuditLogWatcherParsesNamespaceFromPath(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"secret"}, []string{"*"}, slog.Default())

	line := `{"type":"request","request":{"operation":"update","path":"test/secret/data/app/db"}}`
	event, ok := w.parser.parseLine(line)
	if !ok {
		t.Fatal("expected namespaced path request to be parsed")
	}
	if event.Namespace != "test" {
		t.Fatalf("Namespace = %q, want test", event.Namespace)
	}
	if event.Mount != "secret" || event.Path != "app/db" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestAuditLogWatcherWildcardMountAndNamespace(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"*"}, []string{"*"}, slog.Default())

	line := `{"type":"request","request":{"operation":"update","path":"test/kv/metadata/app/azure"}}`
	event, ok := w.parser.parseLine(line)
	if !ok {
		t.Fatal("expected wildcard mount request to be parsed")
	}
	if event.Namespace != "test" || event.Mount != "kv" || event.Path != "app/azure" {
		t.Fatalf("unexpected event: %+v", event)
	}
}

func TestAuditLogWatcherFiltersNamespace(t *testing.T) {
	w := NewAuditLogWatcher("/dev/null", []string{"*"}, []string{"prod"}, slog.Default())

	line := `{"type":"request","request":{"operation":"update","path":"test/kv/metadata/app/azure"}}`
	if event, ok := w.parser.parseLine(line); ok {
		t.Fatalf("expected namespace to be filtered, got %+v", event)
	}
}
