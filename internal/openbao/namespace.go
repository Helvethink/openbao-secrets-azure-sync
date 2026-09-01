package openbao

import (
	"context"
	"strings"
)

type namespaceContextKey struct{}

// ContextWithNamespace returns a context carrying an OpenBao namespace override.
func ContextWithNamespace(ctx context.Context, namespace string) context.Context {
	namespace = strings.Trim(namespace, "/")
	if namespace == "" {
		return ctx
	}
	return context.WithValue(ctx, namespaceContextKey{}, namespace)
}

// NamespaceFromContext returns the namespace override or fallback.
func NamespaceFromContext(ctx context.Context, fallback string) string {
	if namespace, ok := ctx.Value(namespaceContextKey{}).(string); ok && namespace != "" {
		return strings.Trim(namespace, "/")
	}
	return strings.Trim(fallback, "/")
}
