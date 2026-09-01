package openbao

import "context"

// SecretData is the current secret data and version.
type SecretData struct {
	Version int
	Data    map[string]any
}

// ReadMetadata reads synchronization metadata for a secret.
func (c *Client) ReadMetadata(ctx context.Context, mount, secretPath string) (Metadata, error) {
	client, err := c.sdkClient(ctx)
	if err != nil {
		return Metadata{}, err
	}

	metadata, err := client.KVv2(mount).GetMetadata(ctx, secretPath)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		CurrentVersion: metadata.CurrentVersion,
		Custom:         sdkCustomMetadata(metadata.CustomMetadata),
	}, nil
}

// ReadData reads the current data version for a secret.
func (c *Client) ReadData(ctx context.Context, mount, secretPath string) (SecretData, error) {
	return c.ReadDataVersion(ctx, mount, secretPath, 0)
}

// ReadDataVersion reads a specific secret data version. Version 0 means latest.
func (c *Client) ReadDataVersion(ctx context.Context, mount, secretPath string, version int) (SecretData, error) {
	client, err := c.sdkClient(ctx)
	if err != nil {
		return SecretData{}, err
	}

	kv := client.KVv2(mount)
	if version > 0 {
		secret, err := kv.GetVersion(ctx, secretPath, version)
		if err != nil {
			return SecretData{}, err
		}
		return sdkKVData(secret), nil
	}

	secret, err := kv.Get(ctx, secretPath)
	if err != nil {
		return SecretData{}, err
	}
	return sdkKVData(secret), nil
}
