package sync

import (
	"context"

	"openbao-kv-azure-sync/internal/azure"
	"openbao-kv-azure-sync/internal/openbao"
	"openbao-kv-azure-sync/internal/policy"
	"openbao-kv-azure-sync/internal/watcher"
)

func (r *Reconciler) putCertificate(ctx context.Context, event watcher.Event, secret openbao.SecretData, syncMetadata policy.SyncMetadata, remoteName string) error {
	certificate, password, err := buildCertificatePayload(secret.Data)
	if err != nil {
		return withErrorType(errorTypeTransform, err)
	}

	err = r.destination.PutCertificate(ctx, azure.CertificatePayload{
		SourceInstance: r.sourceInstance,
		Namespace:      event.Namespace,
		Mount:          event.Mount,
		Path:           event.Path,
		Version:        secret.Version,
		RemoteName:     remoteName,
		Metadata:       syncMetadata,
		Certificate:    certificate,
		Password:       password,
	})
	if err != nil {
		err = withErrorType(errorTypeDestination, err)
		r.recordDestination(syncMetadata, "put_certificate", err)
		return err
	}
	r.recordDestination(syncMetadata, "put_certificate", nil)
	return nil
}
