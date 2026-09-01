package config

import "testing"

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := Load("../../configs/config.example.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Destination.Type != "dryrun" {
		t.Fatalf("unexpected destination type: %s", cfg.Destination.Type)
	}
	if len(cfg.Azure.ResourceGroups) != 1 {
		t.Fatalf("unexpected resource group count: %d", len(cfg.Azure.ResourceGroups))
	}
	if len(cfg.Azure.KeyVaults) != 2 {
		t.Fatalf("unexpected key vault count: %d", len(cfg.Azure.KeyVaults))
	}
}

func TestLoadAzureExampleConfig(t *testing.T) {
	cfg, err := Load("../../configs/config.azure.example.yaml")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Destination.Type != "azure" {
		t.Fatalf("unexpected destination type: %s", cfg.Destination.Type)
	}
	if cfg.Azure.Auth.Type != "client_secret" {
		t.Fatalf("unexpected azure auth type: %s", cfg.Azure.Auth.Type)
	}
	if cfg.Azure.Auth.ClientSecretEnv != "AZURE_CLIENT_SECRET" {
		t.Fatalf("unexpected azure client secret env: %s", cfg.Azure.Auth.ClientSecretEnv)
	}
}

func TestValidateAzureRequiresClientSecretIdentity(t *testing.T) {
	cfg := defaultConfig()
	cfg.Destination.Type = "azure"
	cfg.Azure.Auth = AzureAuthConfig{Type: "client_secret"}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
