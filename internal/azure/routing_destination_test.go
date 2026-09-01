package azure

import (
	"context"
	"testing"

	"openbao-kv-azure-sync/internal/policy"
)

func TestRoutingDestinationRoutesDryRunSecret(t *testing.T) {
	dryRun := &recordingDestination{}
	azureDestination := &recordingDestination{}
	router := NewRoutingDestination(dryRun, azureDestination)

	err := router.PutSecret(context.Background(), SecretPayload{
		Metadata: policy.SyncMetadata{TargetEnabled: true, TargetProvider: policy.TargetDryRun},
	})
	if err != nil {
		t.Fatalf("PutSecret returned error: %v", err)
	}
	if dryRun.secretCalls != 1 {
		t.Fatalf("dryrun secret calls = %d, want 1", dryRun.secretCalls)
	}
	if azureDestination.secretCalls != 0 {
		t.Fatalf("azure secret calls = %d, want 0", azureDestination.secretCalls)
	}
}

func TestRoutingDestinationRoutesAzureSecret(t *testing.T) {
	dryRun := &recordingDestination{}
	azureDestination := &recordingDestination{}
	router := NewRoutingDestination(dryRun, azureDestination)

	err := router.PutSecret(context.Background(), SecretPayload{
		Metadata: policy.SyncMetadata{TargetEnabled: true, TargetProvider: policy.TargetProviderAzure, TargetType: policy.TargetTypeSecret},
	})
	if err != nil {
		t.Fatalf("PutSecret returned error: %v", err)
	}
	if dryRun.secretCalls != 0 {
		t.Fatalf("dryrun secret calls = %d, want 0", dryRun.secretCalls)
	}
	if azureDestination.secretCalls != 1 {
		t.Fatalf("azure secret calls = %d, want 1", azureDestination.secretCalls)
	}
}

func TestRoutingDestinationRoutesAzureDeleteSecret(t *testing.T) {
	dryRun := &recordingDestination{}
	azureDestination := &recordingDestination{}
	router := NewRoutingDestination(dryRun, azureDestination)

	err := router.DeleteSecret(context.Background(), SecretPayload{
		Metadata: policy.SyncMetadata{TargetEnabled: true, TargetProvider: policy.TargetProviderAzure, TargetType: policy.TargetTypeSecret},
	})
	if err != nil {
		t.Fatalf("DeleteSecret returned error: %v", err)
	}
	if dryRun.deleteSecretCalls != 0 {
		t.Fatalf("dryrun delete secret calls = %d, want 0", dryRun.deleteSecretCalls)
	}
	if azureDestination.deleteSecretCalls != 1 {
		t.Fatalf("azure delete secret calls = %d, want 1", azureDestination.deleteSecretCalls)
	}
}

type recordingDestination struct {
	secretCalls       int
	deleteSecretCalls int
	certificateCalls  int
}

func (d *recordingDestination) PutSecret(context.Context, SecretPayload) error {
	d.secretCalls++
	return nil
}

func (d *recordingDestination) DeleteSecret(context.Context, SecretPayload) error {
	d.deleteSecretCalls++
	return nil
}

func (d *recordingDestination) PutCertificate(context.Context, CertificatePayload) error {
	d.certificateCalls++
	return nil
}
