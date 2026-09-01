package sync

import (
	"context"
	"errors"
	"log/slog"

	"openbao-kv-azure-sync/internal/azure"
	"openbao-kv-azure-sync/internal/metrics"
	"openbao-kv-azure-sync/internal/openbao"
	"openbao-kv-azure-sync/internal/policy"
	"openbao-kv-azure-sync/internal/watcher"
)

// ReconcilerOptions wires the stateless reconciliation logic to OpenBao,
// remote destinations, metrics, and logging.
type ReconcilerOptions struct {
	Source         Source
	SourceInstance string
	Destination    azure.Destination
	Metrics        *metrics.Collector
	Logger         *slog.Logger
}

// Reconciler applies OpenBao state to configured sync destinations.
type Reconciler struct {
	source         Source
	sourceInstance string
	destination    azure.Destination
	metrics        *metrics.Collector
	logger         *slog.Logger
}

// NewReconciler creates a reconciler from explicit dependencies.
func NewReconciler(options ReconcilerOptions) *Reconciler {
	return &Reconciler{
		source:         options.Source,
		sourceInstance: options.SourceInstance,
		destination:    options.Destination,
		metrics:        options.Metrics,
		logger:         defaultLogger(options.Logger),
	}
}

// Reconcile applies the current OpenBao state for a single audit event.
func (r *Reconciler) Reconcile(ctx context.Context, event watcher.Event) error {
	r.logger.Info("reconcile event received",
		"namespace", event.Namespace,
		"mount", event.Mount,
		"path", event.Path,
		"operation", event.Operation,
	)
	if event.Namespace != "" {
		ctx = openbao.ContextWithNamespace(ctx, event.Namespace)
	}

	tracker := newReconcileMetricsTracker(r.metrics, event.Operation)
	defer tracker.finish()

	metadata, err := r.readMetadata(ctx, event)
	if err != nil {
		tracker.fail(err)
		return err
	}

	syncMetadata, err := policy.ParseSyncMetadata(metadata.Custom)
	tracker.setTarget(syncMetadata)
	if errors.Is(err, policy.ErrSyncDisabled) {
		tracker.targetType = "disabled"
		r.logger.Debug("sync disabled", "mount", event.Mount, "path", event.Path)
		return nil
	}
	if err != nil {
		err = withErrorType(errorTypePolicy, err)
		tracker.fail(err)
		return err
	}

	secret, err := r.readData(ctx, event)
	if err != nil {
		tracker.fail(err)
		return err
	}

	err = r.apply(ctx, event, metadata, syncMetadata, secret)
	if err != nil {
		tracker.fail(err)
		return err
	}
	return nil
}

func (r *Reconciler) readMetadata(ctx context.Context, event watcher.Event) (openbao.Metadata, error) {
	metadata, err := r.source.ReadMetadata(ctx, event.Mount, event.Path)
	if err != nil {
		return openbao.Metadata{}, withErrorType(errorTypeOpenBaoRead, err)
	}
	return metadata, nil
}

func (r *Reconciler) readData(ctx context.Context, event watcher.Event) (openbao.SecretData, error) {
	secret, err := r.source.ReadData(ctx, event.Mount, event.Path)
	if err != nil {
		return openbao.SecretData{}, withErrorType(errorTypeOpenBaoRead, err)
	}
	return secret, nil
}

func (r *Reconciler) apply(ctx context.Context, event watcher.Event, metadata openbao.Metadata, syncMetadata policy.SyncMetadata, secret openbao.SecretData) error {
	remoteName := syncMetadata.RemoteObjectName(event.Path)

	if event.Operation == "delete" && (syncMetadata.IsAzureKeyVaultSecret() || syncMetadata.IsDryRun()) {
		return r.deleteSecretEntries(ctx, event, secret, syncMetadata, remoteName)
	}
	if event.Operation == "update" && (syncMetadata.IsAzureKeyVaultSecret() || syncMetadata.IsDryRun()) {
		if err := r.deleteRemovedSecretEntries(ctx, event, secret, metadata.CurrentVersion, syncMetadata, remoteName); err != nil {
			return err
		}
	}

	if syncMetadata.IsAzureKeyVaultCertificate() {
		return r.putCertificate(ctx, event, secret, syncMetadata, remoteName)
	}
	if !syncMetadata.IsAzureKeyVaultSecret() && !syncMetadata.IsDryRun() {
		return nil
	}
	return r.putSecretEntries(ctx, event, secret, syncMetadata, remoteName)
}

func (r *Reconciler) recordDestination(syncMetadata policy.SyncMetadata, operation string, err error) {
	if r.metrics == nil {
		return
	}
	result := "success"
	failureType := errorTypeNone
	if err != nil {
		result = "error"
		failureType = errorType(err)
		if failureType == errorTypeUnknown {
			failureType = errorTypeDestination
		}
	}
	r.metrics.IncDestination(syncMetadata.TargetProvider, operation, result, failureType)
}
