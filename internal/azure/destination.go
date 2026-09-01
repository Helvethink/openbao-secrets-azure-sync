package azure

import (
	"context"

	"openbao-kv-azure-sync/internal/policy"
	"openbao-kv-azure-sync/internal/transform"
)

// SecretPayload describes one Azure Key Vault secret derived from one OpenBao
// KV field.
type SecretPayload struct {
	SourceInstance string
	Namespace      string
	Mount          string
	Path           string
	Key            string
	Version        int
	RemoteName     string
	Metadata       policy.SyncMetadata
	Transform      transform.Payload
}

// CertificatePayload describes one Azure Key Vault certificate import.
type CertificatePayload struct {
	SourceInstance string
	Namespace      string
	Mount          string
	Path           string
	Version        int
	RemoteName     string
	Metadata       policy.SyncMetadata
	Certificate    []byte
	Password       string
}

// Destination is the side-effect boundary for remote secret managers.
type Destination interface {
	PutSecret(ctx context.Context, payload SecretPayload) error
	DeleteSecret(ctx context.Context, payload SecretPayload) error
	PutCertificate(ctx context.Context, payload CertificatePayload) error
}
