## Summary

<!-- Explain what this pull request changes, why it is needed, and the observable behavior after the change. -->

## Related issue

<!-- Link the relevant issue, for example: Closes #123. Use "N/A" when no issue is required. -->

Closes #

## Type of change

<!-- Select all that apply. -->

- [ ] Bug fix
- [ ] New feature
- [ ] Security hardening
- [ ] Refactoring or maintenance
- [ ] Tests
- [ ] Documentation
- [ ] Configuration or deployment
- [ ] Dependency update
- [ ] Breaking change

## Affected areas

<!-- Select all that apply. -->

- [ ] OpenBao client, namespaces, or KV v2 metadata
- [ ] Audit-log or HTTP audit-device watcher
- [ ] Synchronization policy or transformation
- [ ] Azure Key Vault secrets or certificates
- [ ] Operational API, metrics, or logging
- [ ] Configuration or JSON schema
- [ ] Docker, Compose, Prometheus, or Grafana
- [ ] Documentation only

## Behavior and compatibility

<!--
Describe changes to sync metadata, configuration, HTTP contracts, error
semantics, Azure side effects, or migration steps. State "No compatibility
impact" when applicable.
-->

### OpenBao validation

<!--
State the OpenBao version, watcher type, KV v2 operation, mount, and namespace
behavior tested. Use "N/A" when the change does not affect OpenBao behavior.
-->

### Azure validation

<!--
State whether this was tested with dry-run or a real Key Vault, including the
authentication mode and affected object type. Never include credentials,
tokens, secret values, tenant data, or production endpoints.
-->

## Testing

<!-- List the commands and scenarios used to verify this change. Remove commands that are not applicable. -->

```text
gofmt -d cmd internal
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
```

<!-- Add any Docker Compose, manual resynchronization, or sanitized integration testing performed. -->

## Security and operational impact

<!--
Describe changes involving secrets, authentication, authorization, logs,
network listeners, destination URLs, provenance tags, concurrency, retries, or
deletion behavior. Write "No additional impact" when none apply.
-->

## Checklist

- [ ] The change is focused and preserves unrelated work.
- [ ] New or changed behavior has meaningful tests, including error paths.
- [ ] Go code is formatted and `go test ./...` and `go vet ./...` pass.
- [ ] `go test -race ./...` passes when concurrency, watchers, shared state, or shutdown behavior changed.
- [ ] `govulncheck ./...` passes when dependencies or security-sensitive code changed.
- [ ] Exported identifiers have appropriate Go documentation.
- [ ] Configuration examples and `configs/config.schema.json` are updated when the configuration changes.
- [ ] README examples and operational documentation are updated when user-visible behavior changes.
- [ ] No credentials, tokens, secret values, private certificates, tenant data, or production endpoints are included.
- [ ] Logs and HTTP responses do not expose secret values or unnecessary internal details.
- [ ] Azure writes preserve provenance and collision protection, or the impact is explained above.
- [ ] Context cancellation, goroutine lifetime, channel ownership, and graceful shutdown were considered where applicable.
- [ ] KV v2 delete, destroy, undelete, and metadata-delete behavior was considered where applicable.
- [ ] Any skipped check, unverified assumption, breaking change, or follow-up work is explained below.

## Reviewer notes

<!-- Highlight risky areas, design trade-offs, skipped checks, and specific code paths reviewers should examine closely. -->
