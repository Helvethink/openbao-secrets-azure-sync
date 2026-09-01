package policy

import (
	"errors"
	"testing"
)

func TestParseSyncMetadata(t *testing.T) {
	metadata, err := ParseSyncMetadata(map[string]string{
		"sync.enabled":         "true",
		"sync.target.provider": "azure",
		"sync.target.type":     "secret",
		"sync.target.name":     "team-vault",
		"sync.prefix":          "synced",
	})
	if err != nil {
		t.Fatalf("ParseSyncMetadata returned error: %v", err)
	}
	if !metadata.IsAzureKeyVaultSecret() {
		t.Fatal("expected enabled azure key vault secret metadata")
	}
	if got := metadata.RemoteObjectName("app/db"); got != "synced-app-db" {
		t.Fatalf("unexpected remote object name: %s", got)
	}
	if got := RemoteObjectEntryName(metadata.RemoteObjectName("app/db"), "username"); got != "synced-app-db-username" {
		t.Fatalf("unexpected remote entry name: %s", got)
	}
}

func TestParseSyncMetadataCertificate(t *testing.T) {
	metadata, err := ParseSyncMetadata(map[string]string{
		"sync.enabled":         "true",
		"sync.target.provider": "azure",
		"sync.target.type":     "certificate",
		"sync.target.name":     "team-vault",
		"sync.prefix":          "tls",
	})
	if err != nil {
		t.Fatalf("ParseSyncMetadata returned error: %v", err)
	}
	if !metadata.IsAzureKeyVaultCertificate() {
		t.Fatal("expected enabled azure key vault certificate metadata")
	}
}

func TestParseSyncMetadataDryRun(t *testing.T) {
	metadata, err := ParseSyncMetadata(map[string]string{
		"sync.enabled":         "true",
		"sync.target.provider": "dryrun",
	})
	if err != nil {
		t.Fatalf("ParseSyncMetadata returned error: %v", err)
	}
	if !metadata.IsDryRun() {
		t.Fatal("expected dryrun metadata")
	}
	if got := metadata.RemoteObjectName("app/db"); got != "app-db" {
		t.Fatalf("unexpected remote object name: %s", got)
	}
}

func TestParseSyncMetadataDisabled(t *testing.T) {
	_, err := ParseSyncMetadata(map[string]string{"sync.enabled": "false"})
	if !errors.Is(err, ErrSyncDisabled) {
		t.Fatalf("expected ErrSyncDisabled, got %v", err)
	}
}

func TestParseSyncMetadataRequiresPrefix(t *testing.T) {
	_, err := ParseSyncMetadata(map[string]string{
		"sync.enabled":         "true",
		"sync.target.provider": "azure",
		"sync.target.type":     "secret",
	})
	if err == nil {
		t.Fatal("expected error for missing sync_target_name")
	}
}

func TestParseSyncMetadataLegacyKeys(t *testing.T) {
	metadata, err := ParseSyncMetadata(map[string]string{
		"sync_target_enabled": "true",
		"sync_target_type":    "azure_key_vault/secret",
		"sync_target_name":    "team-vault",
		"sync_target_prefix":  "synced",
	})
	if err != nil {
		t.Fatalf("ParseSyncMetadata returned error: %v", err)
	}
	if !metadata.IsAzureKeyVaultSecret() {
		t.Fatal("expected enabled azure key vault secret metadata")
	}
}
