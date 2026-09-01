package azure

import (
	"os"
	"strings"
	"testing"
)

func TestResolveTargetUsesKeyVaultThenResourceGroupAuth(t *testing.T) {
	t.Setenv("TEST_RG_SECRET", "secret")

	destination, err := NewKeyVaultDestination(KeyVaultDestinationConfig{
		DefaultAuth: AuthConfig{Type: "environment"},
		ResourceGroups: []ResourceGroupConfig{
			{
				Name: "shared-rg",
				Auth: AuthConfig{
					Type:            "client_secret",
					TenantID:        "tenant",
					ClientID:        "client",
					ClientSecretEnv: "TEST_RG_SECRET",
				},
			},
		},
		KeyVaults: []KeyVaultConfig{
			{Name: "team-vault", URL: "https://team-vault.vault.azure.net/", ResourceGroup: "shared-rg"},
		},
	})
	if err != nil {
		t.Fatalf("NewKeyVaultDestination returned error: %v", err)
	}

	target, _, err := destination.resolveTarget("team-vault")
	if err != nil {
		t.Fatalf("resolveTarget returned error: %v", err)
	}
	if target.URL != "https://team-vault.vault.azure.net/" {
		t.Fatalf("unexpected target URL: %s", target.URL)
	}
}

func TestResolveTargetUsesKeyVaultAuthBeforeResourceGroupAuth(t *testing.T) {
	t.Setenv("TEST_RG_SECRET", "rg-secret")
	t.Setenv("TEST_KV_SECRET", "kv-secret")

	destination, err := NewKeyVaultDestination(KeyVaultDestinationConfig{
		ResourceGroups: []ResourceGroupConfig{
			{
				Name: "shared-rg",
				Auth: AuthConfig{
					Type:            "client_secret",
					TenantID:        "tenant",
					ClientID:        "rg-client",
					ClientSecretEnv: "TEST_RG_SECRET",
				},
			},
		},
		KeyVaults: []KeyVaultConfig{
			{
				Name:          "team-vault",
				ResourceGroup: "shared-rg",
				Auth: AuthConfig{
					Type:            "client_secret",
					TenantID:        "tenant",
					ClientID:        "kv-client",
					ClientSecretEnv: "TEST_KV_SECRET",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewKeyVaultDestination returned error: %v", err)
	}

	_, err = destination.credential(AuthConfig{
		Type:            "client_secret",
		TenantID:        "tenant",
		ClientID:        "kv-client",
		ClientSecretEnv: "TEST_KV_SECRET",
	})
	if err != nil {
		t.Fatalf("credential returned error: %v", err)
	}

	if _, exists := destination.credentials[authCacheKey(AuthConfig{
		Type:            "client_secret",
		TenantID:        "tenant",
		ClientID:        "rg-client",
		ClientSecretEnv: "TEST_RG_SECRET",
	})]; exists {
		t.Fatal("resource group credential should not be created before it is needed")
	}

	_, _, err = destination.resolveTarget("team-vault")
	if err != nil {
		t.Fatalf("resolveTarget returned error: %v", err)
	}
	if os.Getenv("TEST_KV_SECRET") == "" {
		t.Fatal("test setup lost key vault secret env")
	}
}

func TestProvenanceTags(t *testing.T) {
	tags := provenanceTags("openbao-dev", "finance", "secret", "app/db", "username", 12)

	assertTag := func(key, want string) {
		t.Helper()
		value, ok := tags[key]
		if !ok {
			t.Fatalf("missing tag %s", key)
		}
		if value == nil || *value != want {
			t.Fatalf("tag %s = %v, want %q", key, value, want)
		}
	}

	assertTag("openbao_instance", "openbao-dev")
	assertTag("openbao_namespace", "finance")
	assertTag("openbao_mount", "secret")
	assertTag("openbao_path", "app/db")
	assertTag("openbao_key", "username")
	assertTag("openbao_version", "12")
	assertTag("openbao_origin", "openbao-dev/finance/secret/app/db/username")
}

func TestResolveTargetRejectsUnsafeDynamicKeyVaultName(t *testing.T) {
	destination, err := NewKeyVaultDestination(KeyVaultDestinationConfig{})
	if err != nil {
		t.Fatalf("NewKeyVaultDestination returned error: %v", err)
	}

	_, _, err = destination.resolveTarget("attacker.example#")
	if err == nil || !strings.Contains(err.Error(), "invalid azure key vault name") {
		t.Fatalf("resolveTarget error = %v, want invalid name", err)
	}
}

func TestNewKeyVaultDestinationRejectsUnsafeConfiguredURL(t *testing.T) {
	_, err := NewKeyVaultDestination(KeyVaultDestinationConfig{
		KeyVaults: []KeyVaultConfig{{Name: "team-vault", URL: "http://example.test/"}},
	})
	if err == nil {
		t.Fatal("NewKeyVaultDestination returned nil error")
	}
}

func TestSecretProvenanceIncludesMount(t *testing.T) {
	payload := SecretPayload{
		SourceInstance: "openbao-dev",
		Namespace:      "finance",
		Mount:          "secret-a",
		Path:           "app/db",
		Key:            "username",
	}
	tags := provenanceTags(payload.SourceInstance, payload.Namespace, payload.Mount, payload.Path, payload.Key, 1)
	if !hasSecretProvenance(tags, payload) {
		t.Fatal("hasSecretProvenance() = false for matching origin")
	}
	payload.Mount = "secret-b"
	if hasSecretProvenance(tags, payload) {
		t.Fatal("hasSecretProvenance() = true for a different mount")
	}
}

func TestCertificateProvenanceIncludesMount(t *testing.T) {
	payload := CertificatePayload{
		SourceInstance: "openbao-dev",
		Namespace:      "finance",
		Mount:          "secret-a",
		Path:           "tls/api",
	}
	tags := provenanceTags(payload.SourceInstance, payload.Namespace, payload.Mount, payload.Path, "", 1)
	if !hasCertificateProvenance(tags, payload) {
		t.Fatal("hasCertificateProvenance() = false for matching origin")
	}
	payload.Mount = "secret-b"
	if hasCertificateProvenance(tags, payload) {
		t.Fatal("hasCertificateProvenance() = true for a different mount")
	}
}
