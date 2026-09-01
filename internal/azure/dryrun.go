package azure

import (
	"context"
	"log/slog"
)

type DryRunDestination struct {
	logger *slog.Logger
}

// NewDryRunDestination creates a destination that logs intended operations only.
func NewDryRunDestination(logger *slog.Logger) *DryRunDestination {
	return &DryRunDestination{logger: logger}
}

// PutSecret logs the Azure secret write that would have happened.
func (d *DryRunDestination) PutSecret(ctx context.Context, payload SecretPayload) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d.logger.Info("dry_run_put_secret",
		"source_instance", payload.SourceInstance,
		"mount", payload.Mount,
		"path", payload.Path,
		"key", payload.Key,
		"version", payload.Version,
		"target_type", payload.Metadata.TargetType,
		"key_vault", payload.Metadata.TargetName,
		"remote_name", payload.RemoteName,
		"content_type", payload.Transform.ContentType,
	)
	return nil
}

// DeleteSecret logs the Azure secret deletion that would have happened.
func (d *DryRunDestination) DeleteSecret(ctx context.Context, payload SecretPayload) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d.logger.Info("dry_run_delete_secret",
		"source_instance", payload.SourceInstance,
		"mount", payload.Mount,
		"path", payload.Path,
		"key", payload.Key,
		"version", payload.Version,
		"target_type", payload.Metadata.TargetType,
		"key_vault", payload.Metadata.TargetName,
		"remote_name", payload.RemoteName,
	)
	return nil
}

// PutCertificate logs the Azure certificate import that would have happened.
func (d *DryRunDestination) PutCertificate(ctx context.Context, payload CertificatePayload) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	d.logger.Info("dry_run_put_certificate",
		"source_instance", payload.SourceInstance,
		"mount", payload.Mount,
		"path", payload.Path,
		"version", payload.Version,
		"target_type", payload.Metadata.TargetType,
		"key_vault", payload.Metadata.TargetName,
		"remote_name", payload.RemoteName,
	)
	return nil
}
