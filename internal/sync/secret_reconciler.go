package sync

import (
	"context"
	"fmt"

	"openbao-kv-azure-sync/internal/azure"
	"openbao-kv-azure-sync/internal/openbao"
	"openbao-kv-azure-sync/internal/policy"
	"openbao-kv-azure-sync/internal/watcher"
)

// deleteRemovedSecretEntries deletes destination secrets for fields removed since the previous KV version.
func (r *Reconciler) deleteRemovedSecretEntries(ctx context.Context, event watcher.Event, current openbao.SecretData, currentVersion int, syncMetadata policy.SyncMetadata, remoteName string) error {
	previousVersion := current.Version - 1
	if previousVersion <= 0 && currentVersion > 1 {
		previousVersion = currentVersion - 1
	}
	if previousVersion <= 0 {
		return nil
	}

	versionedSource, ok := r.source.(VersionedSource)
	if !ok {
		r.logger.Debug("skip removed key cleanup: source does not support historical versions",
			"mount", event.Mount,
			"path", event.Path,
			"version", previousVersion,
		)
		return nil
	}

	previous, err := versionedSource.ReadDataVersion(ctx, event.Mount, event.Path, previousVersion)
	if err != nil {
		r.logger.Debug("skip removed key cleanup: previous version is not readable",
			"mount", event.Mount,
			"path", event.Path,
			"version", previousVersion,
			"error", err,
		)
		return nil
	}

	for _, key := range sortedKeys(previous.Data) {
		if _, exists := current.Data[key]; exists {
			continue
		}
		err := r.destination.DeleteSecret(ctx, azure.SecretPayload{
			SourceInstance: r.sourceInstance,
			Namespace:      event.Namespace,
			Mount:          event.Mount,
			Path:           event.Path,
			Key:            key,
			Version:        current.Version,
			RemoteName:     policy.RemoteObjectEntryName(remoteName, key),
			Metadata:       syncMetadata,
		})
		r.recordDestination(syncMetadata, "delete_removed_secret", err)
		if err != nil {
			return withErrorType(errorTypeDestination, err)
		}
	}
	return nil
}

// deleteSecretEntries deletes one destination secret for each OpenBao KV field.
func (r *Reconciler) deleteSecretEntries(ctx context.Context, event watcher.Event, secret openbao.SecretData, syncMetadata policy.SyncMetadata, remoteName string) error {
	for _, key := range sortedKeys(secret.Data) {
		err := r.destination.DeleteSecret(ctx, azure.SecretPayload{
			SourceInstance: r.sourceInstance,
			Namespace:      event.Namespace,
			Mount:          event.Mount,
			Path:           event.Path,
			Key:            key,
			Version:        secret.Version,
			RemoteName:     policy.RemoteObjectEntryName(remoteName, key),
			Metadata:       syncMetadata,
		})
		r.recordDestination(syncMetadata, "delete_secret", err)
		if err != nil {
			return withErrorType(errorTypeDestination, err)
		}
	}
	return nil
}

// putSecretEntries writes one destination secret for each OpenBao KV field.
func (r *Reconciler) putSecretEntries(ctx context.Context, event watcher.Event, secret openbao.SecretData, syncMetadata policy.SyncMetadata, remoteName string) error {
	for _, key := range sortedKeys(secret.Data) {
		payload, err := buildEntryPayload(secret.Data[key], syncMetadata.Format)
		if err != nil {
			return withErrorType(errorTypeTransform, fmt.Errorf("build payload for key %q: %w", key, err))
		}
		err = r.destination.PutSecret(ctx, azure.SecretPayload{
			SourceInstance: r.sourceInstance,
			Namespace:      event.Namespace,
			Mount:          event.Mount,
			Path:           event.Path,
			Key:            key,
			Version:        secret.Version,
			RemoteName:     policy.RemoteObjectEntryName(remoteName, key),
			Metadata:       syncMetadata,
			Transform:      payload,
		})
		r.recordDestination(syncMetadata, "put_secret", err)
		if err != nil {
			return withErrorType(errorTypeDestination, err)
		}
	}
	return nil
}
