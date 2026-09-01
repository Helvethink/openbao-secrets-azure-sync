package openbao

import (
	"context"
	"strings"
	"time"

	baoapi "github.com/openbao/openbao/api/v2"
)

// Client wraps the OpenBao SDK with the subset used by the controller.
// This is useful because it allows us to use namespace-scoped clients.
type Client struct {
	client    *baoapi.Client
	namespace string
	initErr   error
}

// NewClient creates a Client for one OpenBao address and token, eventually a namespace.
func NewClient(address, token string, namespace ...string) *Client {
	var ns string
	if len(namespace) > 0 {
		ns = strings.Trim(namespace[0], "/")
	}

	config := baoapi.DefaultConfig()
	config.Address = strings.TrimRight(address, "/")
	config.Timeout = 10 * time.Second

	client, err := baoapi.NewClient(config)
	if err != nil {
		return &Client{namespace: ns, initErr: err}
	}
	client.SetToken(token)
	if ns != "" {
		client.SetNamespace(ns)
	}
	return &Client{client: client, namespace: ns}
}

func (c *Client) sdkClient(ctx context.Context) (*baoapi.Client, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	namespace := NamespaceFromContext(ctx, c.namespace)
	return c.client.WithNamespace(namespace), nil
}
