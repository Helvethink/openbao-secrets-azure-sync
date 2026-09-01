package azure

import (
	"context"
	"fmt"
)

type RoutingDestination struct {
	DryRun Destination
	Azure  Destination
}

// NewRoutingDestination returns a destination that dispatches by sync metadata.
func NewRoutingDestination(dryRun Destination, azure Destination) *RoutingDestination {
	return &RoutingDestination{
		DryRun: dryRun,
		Azure:  azure,
	}
}

// PutSecret writes a secret through the destination selected by sync_target_type.
func (d *RoutingDestination) PutSecret(ctx context.Context, payload SecretPayload) error {
	if payload.Metadata.IsDryRun() {
		return d.DryRun.PutSecret(ctx, payload)
	}
	if payload.Metadata.UsesAzure() {
		return d.Azure.PutSecret(ctx, payload)
	}
	return fmt.Errorf("unsupported secret sync_target_type %q", payload.Metadata.TargetType)
}

// DeleteSecret deletes a secret through the destination selected by sync_target_type.
func (d *RoutingDestination) DeleteSecret(ctx context.Context, payload SecretPayload) error {
	if payload.Metadata.IsDryRun() {
		return d.DryRun.DeleteSecret(ctx, payload)
	}
	if payload.Metadata.UsesAzure() {
		return d.Azure.DeleteSecret(ctx, payload)
	}
	return fmt.Errorf("unsupported secret sync_target_type %q", payload.Metadata.TargetType)
}

// PutCertificate writes a certificate through the destination selected by sync_target_type.
func (d *RoutingDestination) PutCertificate(ctx context.Context, payload CertificatePayload) error {
	if payload.Metadata.IsDryRun() {
		return d.DryRun.PutCertificate(ctx, payload)
	}
	if payload.Metadata.UsesAzure() {
		return d.Azure.PutCertificate(ctx, payload)
	}
	return fmt.Errorf("unsupported certificate sync_target_type %q", payload.Metadata.TargetType)
}
