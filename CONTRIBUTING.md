# Contributing to OpenBao KV Azure Sync

Thank you for helping improve OpenBao KV Azure Sync. Contributions of all
sizes are welcome, including bug reports, documentation fixes, tests,
operational improvements, and new synchronization capabilities.

This controller moves sensitive data across a trust boundary from OpenBao KV
v2 to Azure. Correctness, predictable reconciliation, secret safety, and
operator visibility take priority over adding features quickly.

## Before you start

For a small bug fix or documentation improvement, feel free to open a pull
request directly. Open an issue before starting work that introduces:

- a new destination or authentication method;
- a configuration or sync-metadata compatibility change;
- persistent state, retries, parallel reconciliation, or event reordering;
- destructive Azure or OpenBao behavior;
- a new dependency or a breaking change.

Before implementing a change:

- search existing issues and pull requests for related work;
- keep the contribution focused on one problem;
- read the relevant package and its tests;
- review the limitations in [TODO.md](TODO.md);
- confirm version-sensitive OpenBao or Azure behavior against official
  documentation.

## Reporting security issues

Do not open a public issue for a suspected vulnerability. Use
[GitHub private vulnerability reporting](https://github.com/Helvethink/openbao-secrets-azure-sync/security/advisories/new)
and allow the maintainers time to investigate before public disclosure.

Never include any of the following in an issue, pull request, commit, test
fixture, screenshot, or log excerpt:

- OpenBao tokens or Azure credentials;
- authorization headers or shared audit/API tokens;
- secret values, private keys, PFX files, or certificate passwords;
- raw, unsanitized audit records;
- tenant identifiers, subscription identifiers, or production endpoints when
  they are sensitive.

Use explicit placeholders such as `<redacted>`, `example-vault`, and
`00000000-0000-0000-0000-000000000000`.

## Development setup

You will need:

- Git;
- Go 1.25 or the version declared in [go.mod](go.mod);
- `make`;
- `golangci-lint` for `make lint` and `make check`;
- Docker Compose for the local OpenBao demonstration, when needed;
- `govulncheck` for dependency or security-sensitive changes.

Fork the repository, clone your fork, and download dependencies:

```sh
git clone https://github.com/<your-account>/openbao-secrets-azure-sync.git
cd openbao-secrets-azure-sync
go mod download
```

Run the baseline checks before making changes:

```sh
make check
make race
make build
```

Run the controller in dry-run mode with:

```sh
go run ./cmd/openbao-kv-azure-sync -config configs/config.example.yaml
```

The complete local OpenBao, Prometheus, and Grafana demonstration is available
through:

```sh
docker compose up --build
```

The default Compose environment uses development credentials and must not be
exposed or reused outside local development.

## Repository structure

- `cmd/openbao-kv-azure-sync` contains the executable entry point and
  dependency wiring.
- `internal/config` loads, normalizes, and validates configuration.
- `internal/watcher` receives audit events from a file or HTTP audit device.
- `internal/openbao` wraps namespace-aware OpenBao KV v2 reads.
- `internal/policy` parses sync metadata and derives Azure-safe object names.
- `internal/sync` reconciles current OpenBao state into destination
  operations.
- `internal/azure` implements dry-run routing and Azure Key Vault operations.
- `internal/api` exposes health, metrics, and manual reconciliation.
- `internal/metrics` renders the controller's Prometheus-style metrics.
- `configs` contains examples and the JSON configuration schema.
- `deployments` contains Prometheus and Grafana development configuration.
- `testdata` contains sanitized test fixtures.

Keep implementation details under `internal`. Define interfaces in the
package that consumes them, and keep the executable focused on configuration,
wiring, lifecycle, and exit status.

## Design and safety principles

### OpenBao remains the source of truth

Synchronization behavior is selected through KV v2 custom metadata. Do not
introduce destination state that silently overrides current OpenBao data or
policy.

Preserve namespace, mount, path, key, and version information throughout
reconciliation. Changes to metadata parsing must retain documented legacy
`sync_target_*` compatibility unless the change is explicitly breaking.

### Secret values must remain secret

Never log payload values, tokens, client secrets, private certificate material,
or passwords. Structured logs may contain sanitized operational identifiers
such as mount, logical path, version, target type, and remote object name.

Return generic errors from HTTP endpoints and keep detailed errors in internal
structured logs. Validate and limit all data entering through HTTP or metadata
trust boundaries.

### Azure writes must preserve ownership

Azure objects carry provenance tags. Any write or delete change must continue
to prevent objects from being overwritten across different OpenBao instances,
namespaces, mounts, paths, or keys.

Treat Key Vault names and configured URLs as security-sensitive inputs.
Unconfigured dynamic targets must resolve only to valid Azure Key Vault
endpoints. Real Azure tests must use dedicated, non-production resources.

### Concurrency must preserve reconciliation correctness

Every goroutine needs a clear owner, cancellation path, and termination
condition. Propagate `context.Context` through OpenBao and Azure operations,
keep channel ownership explicit, and verify shutdown behavior.

Do not parallelize reconciliation without defining ordering for events that
refer to the same OpenBao object. A higher throughput implementation must not
allow an older event to overwrite newer state.

### Deletion requires special care

KV v2 delete, destroy, undelete, and metadata-delete workflows do not all
expose the same source data. The controller is currently stateless, and full
deletion handling remains documented future work. Do not claim complete
deletion support without tests for each affected workflow and a design that
accounts for unavailable historical keys.

## Making a change

Create a topic branch from the latest default branch. Keep unrelated
formatting, generated files, and refactors out of the change.

When changing Go code:

- propagate `context.Context` as the first parameter of blocking operations;
- wrap errors with useful context and `%w` when callers may inspect them;
- use `errors.Is` and `errors.As` for error-chain inspection;
- keep operational logs structured with `log/slog`;
- handle an error once: return it or log it at the responsible boundary;
- add Go documentation for exported identifiers;
- avoid new dependencies unless they materially improve correctness or
  maintainability;
- preserve caller-owned data and avoid sharing mutable values across
  goroutines.

When changing configuration:

- update the structs, defaults, normalization, and validation in
  `internal/config`;
- update [configs/config.schema.json](configs/config.schema.json);
- update every affected example under `configs`;
- document migration or compatibility effects in the pull request and README.

When changing sync metadata or Azure behavior:

- test current and legacy metadata forms when applicable;
- test name normalization and provenance collisions;
- cover dry-run routing separately from real Azure side effects;
- consider repeated delivery, cancellation, partial completion, and errors;
- document required Azure permissions without including real identities.

Run `go mod tidy` only when imports or module requirements change, and review
the complete `go.mod` and `go.sum` diff.

## Tests

Every behavior change needs tests for its observable contract and relevant
failure modes. Prefer deterministic, table-driven tests with named subtests.
Use `httptest` for HTTP behavior and small interfaces or fakes at OpenBao and
Azure boundaries.

Depending on the change, cover:

- valid and invalid sync metadata;
- namespace, mount, path, and operation parsing;
- authentication and sanitized HTTP errors;
- malformed, oversized, or unsupported input;
- context cancellation, shutdown, and goroutine termination;
- provenance matches and collisions;
- dry-run and destination routing;
- partial failures and repeated events;
- configuration defaults, validation, and migration behavior.

Run focused tests while iterating:

```sh
go test ./internal/watcher
go test ./internal/policy
go test ./internal/sync
go test ./internal/azure
```

Before submitting a pull request, run:

```sh
make check
make race
make build
```

For dependency updates or security-sensitive changes, also run:

```sh
govulncheck ./...
```

`make check` verifies formatting, runs `go vet`, runs `golangci-lint`,
and executes the complete test suite. Do not disable a linter, suppress an
error, or weaken a test merely to make a check pass. If a tool cannot run in
your environment, explain why in the pull request.

Integration tests must use dedicated development resources, obtain credentials
from the environment, and leave no sensitive data in output or fixtures.
Prefer dry-run verification unless a real Key Vault operation is essential to
the behavior under test.

## Documentation

Update user-facing documentation whenever observable behavior changes:

- keep [README.md](README.md) aligned with supported metadata and operations;
- update configuration examples and their schema together;
- update [TODO.md](TODO.md) when a documented limitation is resolved or a new
  limitation is intentionally introduced;
- keep commands executable and examples free of real credentials;
- describe operational effects, required permissions, and migration steps.

## Commits and pull requests

Use a short, descriptive commit subject and keep commits logically focused.
Pull request titles should describe the resulting behavior.

Prefixes such as `fix:`, `feat:`, `docs:`, `test:`, `ci:`, and
`chore:` are welcome when they make the intent clearer, but no prefix should
hide a user-visible or security-relevant change.

Complete the repository's pull request template. Before requesting review,
confirm that:

- the problem, behavior, and compatibility impact are explained;
- tests cover new behavior and relevant failures;
- formatting, tests, vet, race detection, and applicable security checks pass;
- configuration, schema, README, and examples are updated when needed;
- provenance, cancellation, ordering, and deletion effects were considered;
- no credentials, secret values, certificates, tenant data, or production
  endpoints appear anywhere in the contribution;
- skipped checks, unverified assumptions, and follow-up work are explicit.

Contributions are provided under the repository's [MIT License](LICENSE).
