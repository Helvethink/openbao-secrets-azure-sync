package azure

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
)

// AuthConfig describes one supported Azure credential source.
type AuthConfig struct {
	Type            string
	TenantID        string
	ClientID        string
	ClientSecret    string
	ClientSecretEnv string
}

// ResourceGroupConfig provides optional Azure auth scoped by resource group.
type ResourceGroupConfig struct {
	Name           string
	SubscriptionID string
	Auth           AuthConfig
}

// KeyVaultConfig provides optional URL and auth overrides for a Key Vault.
type KeyVaultConfig struct {
	Name          string
	URL           string
	ResourceGroup string
	Auth          AuthConfig
}

// KeyVaultDestinationConfig configures Azure Key Vault clients and credentials.
type KeyVaultDestinationConfig struct {
	DefaultAuth    AuthConfig
	ResourceGroups []ResourceGroupConfig
	KeyVaults      []KeyVaultConfig
	Logger         *slog.Logger
}

// KeyVaultDestination writes OpenBao-derived values to Azure Key Vault.
type KeyVaultDestination struct {
	defaultAuth    AuthConfig
	resourceGroups map[string]ResourceGroupConfig
	keyVaults      map[string]KeyVaultConfig
	logger         *slog.Logger

	mu                 sync.Mutex
	credentials        map[string]azcore.TokenCredential
	secretClients      map[string]*azsecrets.Client
	certificateClients map[string]*azcertificates.Client
}

// ProvenanceCollisionError reports that a remote Azure object already belongs
// to another OpenBao origin.
type ProvenanceCollisionError struct {
	RemoteName string
}

func (e ProvenanceCollisionError) Error() string {
	return fmt.Sprintf("azure secret %q has different OpenBao provenance", e.RemoteName)
}

func (e ProvenanceCollisionError) ErrorType() string {
	return "collision"
}

// NewKeyVaultDestination creates a Key Vault destination with lazy client setup.
func NewKeyVaultDestination(config KeyVaultDestinationConfig) (*KeyVaultDestination, error) {
	destination := &KeyVaultDestination{
		defaultAuth:        config.DefaultAuth,
		resourceGroups:     make(map[string]ResourceGroupConfig, len(config.ResourceGroups)),
		keyVaults:          make(map[string]KeyVaultConfig, len(config.KeyVaults)),
		logger:             config.Logger,
		credentials:        make(map[string]azcore.TokenCredential),
		secretClients:      make(map[string]*azsecrets.Client),
		certificateClients: make(map[string]*azcertificates.Client),
	}
	for _, group := range config.ResourceGroups {
		destination.resourceGroups[normalizeName(group.Name)] = group
	}
	for _, vault := range config.KeyVaults {
		destination.keyVaults[normalizeName(vault.Name)] = vault
	}
	return destination, nil
}

// PutSecret creates or updates one Azure Key Vault secret.
func (d *KeyVaultDestination) PutSecret(ctx context.Context, payload SecretPayload) error {
	client, target, err := d.secretClient(payload.Metadata.TargetName)
	if err != nil {
		return err
	}

	if err := d.ensureSecretOwnership(ctx, client, payload); err != nil {
		return err
	}

	value := string(payload.Transform.Value)
	_, err = client.SetSecret(ctx, payload.RemoteName, azsecrets.SetSecretParameters{
		Value:       to.Ptr(value),
		ContentType: to.Ptr(payload.Transform.ContentType),
		Tags:        provenanceTags(payload.SourceInstance, payload.Namespace, payload.Mount, payload.Path, payload.Key, payload.Version),
	}, nil)
	if err != nil {
		if !isDeletedButRecoverable(err) {
			return err
		}
		if err := d.recoverDeletedSecret(ctx, client, payload.RemoteName); err != nil {
			return err
		}
		if err := d.ensureSecretOwnership(ctx, client, payload); err != nil {
			return err
		}
		_, err = client.SetSecret(ctx, payload.RemoteName, azsecrets.SetSecretParameters{
			Value:       to.Ptr(value),
			ContentType: to.Ptr(payload.Transform.ContentType),
			Tags:        provenanceTags(payload.SourceInstance, payload.Namespace, payload.Mount, payload.Path, payload.Key, payload.Version),
		}, nil)
		if err != nil {
			return err
		}
	}

	d.logger.Info("azure_key_vault_put_secret",
		"mount", payload.Mount,
		"path", payload.Path,
		"version", payload.Version,
		"target_type", payload.Metadata.TargetType,
		"key_vault", target.Name,
		"resource_group", target.ResourceGroup,
		"remote_name", payload.RemoteName,
		"content_type", payload.Transform.ContentType,
	)
	return nil
}

// ensureSecretOwnership prevents overwriting a Key Vault secret owned by another
// OpenBao path/key.
func (d *KeyVaultDestination) ensureSecretOwnership(ctx context.Context, client *azsecrets.Client, payload SecretPayload) error {
	secret, err := client.GetSecret(ctx, payload.RemoteName, "", nil)
	if err != nil {
		var responseErr *azcore.ResponseError
		if errors.As(err, &responseErr) && responseErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}

	if sameTag(secret.Tags, "openbao_instance", payload.SourceInstance) &&
		sameTag(secret.Tags, "openbao_namespace", payload.Namespace) &&
		sameTag(secret.Tags, "openbao_path", payload.Path) &&
		sameTag(secret.Tags, "openbao_key", payload.Key) {
		return nil
	}

	return ProvenanceCollisionError{RemoteName: payload.RemoteName}
}

// recoverDeletedSecret recovers a soft-deleted secret and waits until it is readable.
func (d *KeyVaultDestination) recoverDeletedSecret(ctx context.Context, client *azsecrets.Client, name string) error {
	if _, err := client.RecoverDeletedSecret(ctx, name, nil); err != nil && !isConflict(err) {
		return err
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := client.GetSecret(ctx, name, "", nil); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for recovered azure secret %q", name)
		case <-ticker.C:
		}
	}
}

// DeleteSecret starts deletion of one Azure Key Vault secret.
func (d *KeyVaultDestination) DeleteSecret(ctx context.Context, payload SecretPayload) error {
	client, target, err := d.secretClient(payload.Metadata.TargetName)
	if err != nil {
		return err
	}
	if err := d.ensureSecretOwnership(ctx, client, payload); err != nil {
		return err
	}

	_, err = client.DeleteSecret(ctx, payload.RemoteName, nil)
	if err != nil {
		return err
	}

	d.logger.Info("azure_key_vault_delete_secret",
		"mount", payload.Mount,
		"path", payload.Path,
		"key", payload.Key,
		"version", payload.Version,
		"target_type", payload.Metadata.TargetType,
		"key_vault", target.Name,
		"resource_group", target.ResourceGroup,
		"remote_name", payload.RemoteName,
	)
	return nil
}

// PutCertificate imports certificate material into Azure Key Vault.
func (d *KeyVaultDestination) PutCertificate(ctx context.Context, payload CertificatePayload) error {
	client, target, err := d.certificateClient(payload.Metadata.TargetName)
	if err != nil {
		return err
	}
	if len(payload.Certificate) == 0 {
		return errors.New("certificate payload is empty")
	}

	encoded := string(payload.Certificate)
	if !looksLikeBase64(encoded) {
		encoded = base64.StdEncoding.EncodeToString(payload.Certificate)
	}

	parameters := azcertificates.ImportCertificateParameters{
		Base64EncodedCertificate: to.Ptr(encoded),
		Tags:                     provenanceTags(payload.SourceInstance, payload.Namespace, payload.Mount, payload.Path, "", payload.Version),
	}
	if payload.Password != "" {
		parameters.Password = to.Ptr(payload.Password)
	}

	_, err = client.ImportCertificate(ctx, payload.RemoteName, parameters, nil)
	if err != nil {
		return err
	}

	d.logger.Info("azure_key_vault_import_certificate",
		"mount", payload.Mount,
		"path", payload.Path,
		"version", payload.Version,
		"target_type", payload.Metadata.TargetType,
		"key_vault", target.Name,
		"resource_group", target.ResourceGroup,
		"remote_name", payload.RemoteName,
	)
	return nil
}

// secretClient returns a cached Azure Key Vault secret client for the target.
func (d *KeyVaultDestination) secretClient(keyVaultName string) (*azsecrets.Client, KeyVaultConfig, error) {
	target, credential, err := d.resolveTarget(keyVaultName)
	if err != nil {
		return nil, KeyVaultConfig{}, err
	}
	cacheKey := cacheKey(target, credential)

	d.mu.Lock()
	defer d.mu.Unlock()
	if client, ok := d.secretClients[cacheKey]; ok {
		return client, target, nil
	}
	client, err := azsecrets.NewClient(target.URL, credential, nil)
	if err != nil {
		return nil, KeyVaultConfig{}, err
	}
	d.secretClients[cacheKey] = client
	return client, target, nil
}

// certificateClient returns a cached Azure Key Vault certificate client.
func (d *KeyVaultDestination) certificateClient(keyVaultName string) (*azcertificates.Client, KeyVaultConfig, error) {
	target, credential, err := d.resolveTarget(keyVaultName)
	if err != nil {
		return nil, KeyVaultConfig{}, err
	}
	cacheKey := cacheKey(target, credential)

	d.mu.Lock()
	defer d.mu.Unlock()
	if client, ok := d.certificateClients[cacheKey]; ok {
		return client, target, nil
	}
	client, err := azcertificates.NewClient(target.URL, credential, nil)
	if err != nil {
		return nil, KeyVaultConfig{}, err
	}
	d.certificateClients[cacheKey] = client
	return client, target, nil
}

// resolveTarget resolves Key Vault URL and credential from KV, RG, then default auth.
func (d *KeyVaultDestination) resolveTarget(keyVaultName string) (KeyVaultConfig, azcore.TokenCredential, error) {
	target := d.keyVaults[normalizeName(keyVaultName)]
	if target.Name == "" {
		target.Name = keyVaultName
	}
	if target.URL == "" {
		target.URL = fmt.Sprintf("https://%s.vault.azure.net/", target.Name)
	}

	auth := target.Auth
	if auth.Type == "" && target.ResourceGroup != "" {
		if group, ok := d.resourceGroups[normalizeName(target.ResourceGroup)]; ok {
			auth = group.Auth
		}
	}
	if auth.Type == "" {
		auth = d.defaultAuth
	}

	credential, err := d.credential(auth)
	if err != nil {
		return KeyVaultConfig{}, nil, err
	}
	return target, credential, nil
}

// credential returns a cached Azure credential for the given auth config.
func (d *KeyVaultDestination) credential(auth AuthConfig) (azcore.TokenCredential, error) {
	key := authCacheKey(auth)
	d.mu.Lock()
	if credential, ok := d.credentials[key]; ok {
		d.mu.Unlock()
		return credential, nil
	}
	d.mu.Unlock()

	credential, err := newCredential(auth)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.credentials[key] = credential
	d.mu.Unlock()
	return credential, nil
}

// newCredential builds an Azure SDK credential from static configuration.
func newCredential(auth AuthConfig) (azcore.TokenCredential, error) {
	switch auth.Type {
	case "", "environment":
		return azidentity.NewDefaultAzureCredential(nil)
	case "managed_identity":
		options := &azidentity.ManagedIdentityCredentialOptions{}
		if auth.ClientID != "" {
			options.ID = azidentity.ClientID(auth.ClientID)
		}
		return azidentity.NewManagedIdentityCredential(options)
	case "client_secret":
		secret := auth.ClientSecret
		if secret == "" && auth.ClientSecretEnv != "" {
			secret = os.Getenv(auth.ClientSecretEnv)
		}
		if secret == "" {
			return nil, fmt.Errorf("azure client_secret auth requires a client secret value")
		}
		return azidentity.NewClientSecretCredential(auth.TenantID, auth.ClientID, secret, nil)
	default:
		return nil, fmt.Errorf("unsupported azure auth type %q", auth.Type)
	}
}

// cacheKey identifies one client cache entry by vault and credential instance.
func cacheKey(target KeyVaultConfig, credential azcore.TokenCredential) string {
	return fmt.Sprintf("%s:%p", normalizeName(target.Name), credential)
}

// authCacheKey identifies equivalent Azure auth configurations.
func authCacheKey(auth AuthConfig) string {
	return strings.Join([]string{
		auth.Type,
		auth.TenantID,
		auth.ClientID,
		auth.ClientSecretEnv,
		auth.ClientSecret,
	}, "\x00")
}

// normalizeName canonicalizes configured Azure resource names for lookup.
func normalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// looksLikeBase64 reports whether a certificate payload is already base64.
func looksLikeBase64(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "-----BEGIN ") {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(value)
	return err == nil
}

// provenanceTags records the OpenBao origin on Azure Key Vault objects.
func provenanceTags(sourceInstance, namespace, mount, secretPath, key string, version int) map[string]*string {
	tags := map[string]*string{
		"openbao_instance": to.Ptr(sourceInstance),
		"openbao_mount":    to.Ptr(mount),
		"openbao_path":     to.Ptr(secretPath),
		"openbao_origin":   to.Ptr(strings.Trim(strings.Join([]string{sourceInstance, namespace, mount, secretPath}, "/"), "/")),
	}
	if version > 0 {
		tags["openbao_version"] = to.Ptr(fmt.Sprintf("%d", version))
	}
	if namespace != "" {
		tags["openbao_namespace"] = to.Ptr(namespace)
	}
	if key != "" {
		tags["openbao_key"] = to.Ptr(key)
		tags["openbao_origin"] = to.Ptr(strings.Trim(strings.Join([]string{sourceInstance, namespace, mount, secretPath, key}, "/"), "/"))
	}
	return tags
}

func sameTag(tags map[string]*string, key, want string) bool {
	value := tags[key]
	if want == "" && value == nil {
		return true
	}
	return value != nil && *value == want
}

func isDeletedButRecoverable(err error) bool {
	var responseErr *azcore.ResponseError
	if !errors.As(err, &responseErr) {
		return false
	}
	return responseErr.StatusCode == http.StatusConflict &&
		(responseErr.ErrorCode == "Conflict" || strings.Contains(responseErr.Error(), "ObjectIsDeletedButRecoverable"))
}

func isConflict(err error) bool {
	var responseErr *azcore.ResponseError
	return errors.As(err, &responseErr) && responseErr.StatusCode == http.StatusConflict
}
