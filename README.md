# OpenBao KV Azure Sync

[![PkgGoDev](https://pkg.go.dev/badge/github.com/Helvethink/openbao-secrets-azure-sync)](https://pkg.go.dev/mod/github.com/Helvethink/openbao-secrets-azure-sync)
[![Maintainability](https://qlty.sh/badges/91c1035f-d5ff-464d-a5bd-7794ed447c73/maintainability.svg)](https://qlty.sh/gh/Helvethink/projects/openbao-secrets-azure-sync)
[![Docker Pulls](https://badgen.net/docker/pulls/helvethink/openbao-secrets-azure-sync?icon=docker&label=pulls)](https://hub.docker.com/r/helvethink/openbao-secrets-azure-sync/)
[![Docker Stars](https://badgen.net/docker/stars/helvethink/openbao-secrets-azure-sync?icon=docker&label=stars)](https://hub.docker.com/r/helvethink/openbao-secrets-azure-sync/)
[![test](https://github.com/Helvethink/openbao-secrets-azure-sync/actions/workflows/CI.yml/badge.svg)](https://github.com/Helvethink/openbao-secrets-azure-sync/actions/workflows/CI.yml)
[![Coverage Status](https://coveralls.io/repos/github/Helvethink/openbao-secrets-azure-sync/badge.svg?branch=main)](https://coveralls.io/github/Helvethink/openbao-secrets-azure-sync?branch=main)
[![release](https://github.com/Helvethink/openbao-secrets-azure-sync/actions/workflows/release.yml/badge.svg)](https://github.com/Helvethink/openbao-secrets-azure-sync/actions/workflows/release.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/Helvethink/openbao-secrets-azure-sync/blob/main/LICENSE)

An external Go controller that synchronizes selected OpenBao KV v2 secrets to
Azure Key Vault. OpenBao remains the source of truth. Synchronization is
enabled per secret with KV v2 `custom_metadata`; the controller watches an
OpenBao audit log, reads the current metadata and data, transforms the values,
and applies them to the selected destination.

Secret values are never written to logs.

## What is implemented

- OpenBao KV v2 reads, including namespace-aware reads.
- Audit-log tailing from a file or an OpenBao HTTP audit device.
- Per-secret `dryrun` and Azure Key Vault routing.
- Azure Key Vault secrets, with one Azure secret generated per KV field.
- Azure Key Vault certificate import from a single certificate/PFX/value field
  or from `cert`, `key`, and optional `chain` fields.
- Provenance tags and collision protection before overwriting Azure secrets.
- Manual resynchronization and Prometheus-style metrics endpoints.
- Docker Compose development environments with OpenBao, Prometheus, and
  Grafana.

The configured state store is reserved for future use; synchronization is
currently stateless.

## Quick start

Requirements: Go 1.25 or newer for local builds, or Docker Compose for the
containerized demo.

Run the dry-run controller against the example file watcher:

```bash
go run ./cmd/openbao-kv-azure-sync -config configs/config.example.yaml
```

Run the complete local demo:

```bash
docker compose up --build
```

The demo uses an OpenBao development server and does not write to Azure. It
exposes:

| Service | URL | Credentials |
| --- | --- | --- |
| OpenBao | http://127.0.0.1:28200 | `dev-token` |
| Controller ingress | http://127.0.0.1:18080/ingress | `dev-audit-token` |
| Controller API and metrics | http://127.0.0.1:19090 | none in demo config |
| Prometheus | http://127.0.0.1:19091 | none |
| Grafana | http://127.0.0.1:13000 | `admin` / `admin` |

The bootstrap container creates `secret/app/db` and enables dry-run sync. To
trigger another event:

```bash
docker compose exec openbao bao kv put secret/app/db username=demo password=changed
```

## Sync metadata

A secret must have `sync.enabled=true`. The supported metadata keys are:

| Key | Values | Meaning |
| --- | --- | --- |
| `sync.enabled` | `true` / `false` | Enable or disable synchronization. |
| `sync.target.provider` | `dryrun` / `azure` | Select the destination. |
| `sync.target.type` | `secret` / `certificate` | Azure resource type. Required for Azure. |
| `sync.target.name` | Key Vault name | Required for Azure. |
| `sync.prefix` | Azure-safe name prefix | Optional. |
| `sync.format` | `json` / `raw` | Field serialization. Defaults to `json`. |

For Azure secrets, each field becomes a separate Azure secret. For example,
`secret/app/db` with fields `username` and `password` becomes
`<prefix>-app-db-username` and `<prefix>-app-db-password`. Names are lowercased
and characters outside `a-z` and `0-9` are converted to hyphens to satisfy
Azure Key Vault naming rules.

Example:

```bash
bao kv metadata patch \
  -mount=secret \
  -custom-metadata=sync.enabled=true \
  -custom-metadata=sync.target.provider=azure \
  -custom-metadata=sync.target.type=secret \
  -custom-metadata=sync.target.name=my-key-vault \
  -custom-metadata=sync.prefix=from-bao \
  app/db
```

Legacy `sync_target_*` metadata names are accepted for compatibility, but new
secrets should use the names above.

## Configuration

Start from one of the files in `configs/`:

- `config.example.yaml` uses the local file watcher and dry-run destination.
- `config.docker.yaml` is used by the Compose demo.
- `config.azure.example.yaml` shows the HTTP watcher and client-secret auth.

The main configuration sections are `openbao`, `watcher`, `api`,
`destination`, `azure`, and `sync`. `configs/config.schema.json` documents the
YAML shape for editors and external validation.

### Azure authentication

Supported auth types are `environment`, `managed_identity`, and
`client_secret`. Client secrets should be supplied with `client_secret_env`,
not committed in YAML. Resolution order is Key Vault override, resource-group
override, then `azure.auth`.

For real Azure use, set the required environment variables and run the Azure
Compose file:

```bash
export AZURE_TENANT_ID="<tenant-id>"
export AZURE_CLIENT_ID="<client-id>"
export AZURE_CLIENT_SECRET="<client-secret>"
export AZURE_KEY_VAULT_NAME="<key-vault-name>"
docker compose -f docker-compose.azure.yml up --build
```

Grant the identity permission to read, write, and delete Key Vault secrets.
The controller reads existing tags to prevent overwriting an object owned by a
different OpenBao instance, namespace, path, or key.

Do not use development credentials outside a local environment. If the local
`.env` file in this checkout contains a live Azure credential, rotate it before
using the repository or pushing it anywhere.

## Watchers and API

The file watcher tails `watcher.audit_log_path`. The HTTP watcher accepts
newline-delimited OpenBao audit records at the configured path and supports
the `X-OpenBao-Audit-Token` header or a Bearer token. It waits for
reconciliation to finish before acknowledging an audit request.

The operational API exposes:

```text
GET  /healthz
GET  /readyz
GET  /metrics
POST /sync
```

`POST /sync` accepts `mount`, `path`, and an optional `namespace`:

```bash
curl -X POST http://127.0.0.1:19090/sync \
  -H 'Content-Type: application/json' \
  -d '{"mount":"secret","path":"app/db"}'
```

Set `api.auth_token` and send it as `X-OpenBao-Sync-Token` or
`Authorization: Bearer ...` when exposing the API beyond localhost.

## Development

Run formatting, tests, and static checks:

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
```

The project layout is:

```text
cmd/openbao-kv-azure-sync/   CLI entrypoint
internal/config/             configuration loading and validation
internal/watcher/            audit log and HTTP watchers
internal/openbao/             OpenBao KV v2 client
internal/policy/              metadata parsing and name mapping
internal/transform/           destination payload types
internal/azure/               dry-run, routing, and Key Vault destinations
internal/sync/                 reconciliation controller
internal/metrics/              metrics collector
configs/                      example configuration and JSON schema
deployments/                  Prometheus and Grafana configuration
testdata/                     audit-log fixture
```

## Scope and limitations

- Synchronization is stateless; the `state` configuration is reserved.
- Azure Key Vault keys and Azure App Configuration are not implemented.
- HTTP audit ingress has shared-token authentication but no TLS/mTLS support.
- Retries with backoff are not implemented.
- The Compose files are development examples and use predictable credentials.
