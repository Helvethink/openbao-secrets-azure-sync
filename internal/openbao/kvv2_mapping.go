package openbao

import baoapi "github.com/openbao/openbao/api/v2"

func sdkKVData(secret *baoapi.KVSecret) SecretData {
	version := 0
	if secret.VersionMetadata != nil {
		version = secret.VersionMetadata.Version
	}

	data := secret.Data
	if data == nil {
		data = map[string]any{}
	}

	return SecretData{
		Version: version,
		Data:    data,
	}
}

func sdkCustomMetadata(data map[string]any) map[string]string {
	result := make(map[string]string, len(data))
	for key, value := range data {
		stringValue, ok := value.(string)
		if !ok {
			continue
		}
		result[key] = stringValue
	}
	return result
}
