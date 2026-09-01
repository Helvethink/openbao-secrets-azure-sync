package sync

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"openbao-kv-azure-sync/internal/policy"
	"openbao-kv-azure-sync/internal/transform"
)

// sortedKeys returns deterministic field order for writes, deletes, and tests.
func sortedKeys(data map[string]any) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// buildEntryPayload converts one OpenBao KV field into a destination payload.
func buildEntryPayload(value any, format string) (transform.Payload, error) {
	if format == policy.FormatRaw {
		stringValue, ok := value.(string)
		if !ok {
			return transform.Payload{}, fmt.Errorf("raw format requires a string value, got %T", value)
		}
		return transform.Payload{ContentType: "text/plain", Value: []byte(stringValue)}, nil
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return transform.Payload{}, err
	}
	return transform.Payload{
		ContentType: "application/json",
		Value:       encoded,
	}, nil
}

// buildCertificatePayload extracts certificate material from supported field shapes.
func buildCertificatePayload(data map[string]any) ([]byte, string, error) {
	certificate, ok := firstString(data, "certificate", "pfx", "value")
	if ok {
		password, _ := firstString(data, "password")
		return []byte(certificate), password, nil
	}

	cert, certOK := firstString(data, "cert")
	key, keyOK := firstString(data, "key")
	if !certOK || !keyOK {
		return nil, "", fmt.Errorf("certificate sync requires certificate/pfx/value or cert and key fields")
	}

	parts := []string{cert, key}
	if chain, ok := firstString(data, "chain"); ok {
		parts = append(parts, chain)
	}
	password, _ := firstString(data, "password")
	return []byte(strings.Join(parts, "\n")), password, nil
}

// firstString returns the first non-empty string field from data.
func firstString(data map[string]any, fields ...string) (string, bool) {
	for _, field := range fields {
		value, ok := data[field]
		if !ok {
			continue
		}
		stringValue, ok := value.(string)
		if ok && stringValue != "" {
			return stringValue, true
		}
	}
	return "", false
}
