package sync

import (
	"context"

	"openbao-kv-azure-sync/internal/openbao"
)

// Source reads the source state needed to reconcile one audit event.
type Source interface {
	ReadMetadata(ctx context.Context, mount, secretPath string) (openbao.Metadata, error)
	ReadData(ctx context.Context, mount, secretPath string) (openbao.SecretData, error)
}

// VersionedSource can read historical source versions when the backend supports it.
type VersionedSource interface {
	ReadDataVersion(ctx context.Context, mount, secretPath string, version int) (openbao.SecretData, error)
}
