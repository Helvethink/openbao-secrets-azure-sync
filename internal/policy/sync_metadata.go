package policy

import "strings"

const (
	// TargetDryRun routes the event to logging only.
	TargetDryRun = "dryrun"
	// TargetAzureKeyVaultSecret maps one OpenBao KV field to one Azure secret.
	TargetAzureKeyVaultSecret = "azure_key_vault/secret"
	// TargetAzureKeyVaultCertificate imports certificate material into Key Vault.
	TargetAzureKeyVaultCertificate = "azure_key_vault/certificate"
	// TargetProviderAzure is the sync.target.provider value for Azure targets.
	TargetProviderAzure = "azure"
	// TargetTypeSecret is the sync.target.type value for secret targets.
	TargetTypeSecret = "secret"
	// TargetTypeCertificate is the sync.target.type value for certificate targets.
	TargetTypeCertificate = "certificate"

	// FormatJSON serializes a complete object as JSON.
	FormatJSON = "json"
	// FormatRaw expects a string value and writes it directly.
	FormatRaw = "raw"
)

// SyncMetadata is the normalized synchronization policy stored in OpenBao KV v2
// custom_metadata.
type SyncMetadata struct {
	TargetEnabled  bool
	TargetProvider string
	TargetType     string
	TargetName     string
	TargetPrefix   string
	Format         string
	Raw            map[string]string
}

// IsAzureKeyVaultSecret reports whether this policy targets Azure Key Vault secrets.
func (m SyncMetadata) IsAzureKeyVaultSecret() bool {
	return m.TargetEnabled && m.TargetProvider == TargetProviderAzure && m.TargetType == TargetTypeSecret
}

// IsAzureKeyVaultCertificate reports whether this policy targets Azure Key Vault certificates.
func (m SyncMetadata) IsAzureKeyVaultCertificate() bool {
	return m.TargetEnabled && m.TargetProvider == TargetProviderAzure && m.TargetType == TargetTypeCertificate
}

// IsDryRun reports whether this policy should only log intended writes.
func (m SyncMetadata) IsDryRun() bool {
	return m.TargetEnabled && m.TargetProvider == TargetDryRun
}

// UsesAzure reports whether this target type should be routed to Azure.
func (m SyncMetadata) UsesAzure() bool {
	return m.TargetEnabled && m.TargetProvider == TargetProviderAzure
}

// RemoteObjectName returns the base Azure object name for an OpenBao secret path.
func (m SyncMetadata) RemoteObjectName(secretPath string) string {
	base := sanitizeKeyVaultName(secretPath)
	prefix := sanitizeKeyVaultName(m.TargetPrefix)
	if prefix == "" {
		return base
	}
	if base == "" {
		return prefix
	}
	return prefix + "-" + base
}

// RemoteObjectEntryName returns the Azure secret name for one OpenBao KV field.
func RemoteObjectEntryName(secretName, key string) string {
	return joinKeyVaultNameParts(secretName, sanitizeKeyVaultName(key))
}

// joinKeyVaultNameParts joins logical name parts into an Azure-safe name.
func joinKeyVaultNameParts(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeKeyVaultName(part)
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, "-")
}

// sanitizeKeyVaultName normalizes a string to Azure Key Vault object-name rules.
func sanitizeKeyVaultName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastWasHyphen := false

	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			builder.WriteRune(r)
			lastWasHyphen = false
			continue
		}
		if !lastWasHyphen {
			builder.WriteByte('-')
			lastWasHyphen = true
		}
	}

	return strings.Trim(builder.String(), "-")
}
