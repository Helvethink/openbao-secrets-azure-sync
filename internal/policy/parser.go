package policy

import (
	"errors"
	"fmt"
	"strconv"
)

// ErrSyncDisabled indicates that metadata explicitly disables synchronization.
var ErrSyncDisabled = errors.New("sync disabled")

// ParseSyncMetadata validates OpenBao custom_metadata and returns sync policy.
func ParseSyncMetadata(custom map[string]string) (SyncMetadata, error) {
	enabledValue := firstMetadataValue(custom, "sync.enabled", "sync_target_enabled")
	enabled, err := strconv.ParseBool(enabledValue)
	if err != nil || !enabled {
		return SyncMetadata{TargetEnabled: false, Raw: custom}, ErrSyncDisabled
	}

	metadata := SyncMetadata{
		TargetEnabled:  true,
		TargetProvider: firstMetadataValue(custom, "sync.target.provider"),
		TargetType:     firstMetadataValue(custom, "sync.target.type"),
		TargetName:     firstMetadataValue(custom, "sync.target.name", "sync_target_name"),
		TargetPrefix:   firstMetadataValue(custom, "sync.prefix", "sync_target_prefix"),
		Format:         custom["sync.format"],
		Raw:            custom,
	}

	if legacyType := custom["sync_target_type"]; metadata.TargetProvider == "" && legacyType != "" {
		switch legacyType {
		case TargetDryRun:
			metadata.TargetProvider = TargetDryRun
		case TargetAzureKeyVaultSecret:
			metadata.TargetProvider = TargetProviderAzure
			metadata.TargetType = TargetTypeSecret
		case TargetAzureKeyVaultCertificate:
			metadata.TargetProvider = TargetProviderAzure
			metadata.TargetType = TargetTypeCertificate
		default:
			metadata.TargetType = legacyType
		}
	}

	switch metadata.TargetProvider {
	case TargetDryRun:
	case TargetProviderAzure:
		switch metadata.TargetType {
		case TargetTypeSecret, TargetTypeCertificate:
		default:
			return metadata, fmt.Errorf("unsupported sync.target.type %q", metadata.TargetType)
		}
	default:
		return metadata, fmt.Errorf("unsupported sync.target.provider %q", metadata.TargetProvider)
	}
	if metadata.UsesAzure() && metadata.TargetName == "" {
		return metadata, errors.New("sync.target.name is required")
	}
	if metadata.Format == "" {
		metadata.Format = FormatJSON
	}
	switch metadata.Format {
	case FormatJSON, FormatRaw:
	default:
		return metadata, fmt.Errorf("unsupported sync.format %q", metadata.Format)
	}

	return metadata, nil
}

func firstMetadataValue(custom map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := custom[key]; value != "" {
			return value
		}
	}
	return ""
}
