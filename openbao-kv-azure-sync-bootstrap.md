# Historical bootstrap notes

> This document records the original project proposal. The implementation and
> supported configuration are documented in [README.md](README.md). In
> particular, the older `sync.provider`, `sync.type`, and `sync.destination`
> metadata names shown below are not the current schema.

## Objectif

Créer un projet Go permettant de synchroniser certains secrets stockés dans un moteur **OpenBao KV v2** vers Azure, en s'appuyant sur les **custom metadata** du KV v2 pour décider quoi synchroniser, vers quelle destination, sous quel format et avec quel type de ressource Azure.

L'idée n'est pas de remplacer KV v2 ni de créer une extension native du backend KV v2, mais de conserver le KV v2 standard OpenBao comme source de vérité, puis d'ajouter un contrôleur externe Go.

---

## Décision d'architecture

### Ce qu'on ne fait pas

On ne crée pas un nouveau type de plugin OpenBao arbitraire.

OpenBao supporte des plugins externes de types classiques comme :

- `auth`
- `secret`
- `database`

Un plugin externe OpenBao est monté comme son propre engine. Il ne s'accroche pas automatiquement à un KV v2 existant pour intercepter ses écritures.

Donc créer un plugin secret engine externe qui "étend" directement `kv-v2` n'est pas la voie la plus simple.

### Ce qu'on fait

On garde un KV v2 standard :

```text
secret/data/app/db
secret/metadata/app/db
```

Et on ajoute des `custom_metadata` sur les secrets à synchroniser :

```text
sync.enabled=true
sync.provider=azure
sync.type=azure-keyvault-secret
sync.destination=https://my-vault.vault.azure.net/
sync.name=my-app-db
sync.format=json
```

Puis un service Go externe observe les changements, lit les métadonnées, lit la dernière version du secret, applique la transformation nécessaire, puis pousse vers Azure.

---

## Architecture cible

```text
OpenBao KV v2
  ├── secret/data/<path>
  └── secret/metadata/<path>
        custom_metadata:
          sync.enabled=true
          sync.provider=azure
          sync.type=azure-keyvault-secret | azure-keyvault-certificate | azure-keyvault-key | azure-app-configuration
          sync.destination=<destination>
          sync.name=<remote-name>
          sync.format=raw | json | certificate | key

Audit log OpenBao
  ↓
openbao-kv-sync-controller
  ├── watcher/
  │   └── auditlog
  ├── openbao/
  │   └── kvv2 client
  ├── policy/
  │   └── metadata parser
  ├── transform/
  │   ├── raw
  │   ├── json
  │   ├── certificate
  │   └── key
  ├── azure/
  │   ├── keyvault secret
  │   ├── keyvault certificate
  │   ├── keyvault key
  │   └── app configuration
  └── state/
      └── synced version / checksum / last error
```

---

## Exemple d'utilisation côté OpenBao

### Écrire un secret

```bash
bao kv put secret/app/db \
  username=my-app \
  password='super-secret'
```

### Ajouter les métadonnées de synchronisation

```bash
bao kv metadata patch \
  -mount=secret \
  -custom-metadata=sync.enabled=true \
  -custom-metadata=sync.provider=azure \
  -custom-metadata=sync.type=azure-keyvault-secret \
  -custom-metadata=sync.destination=https://my-vault.vault.azure.net/ \
  -custom-metadata=sync.name=my-app-db \
  -custom-metadata=sync.format=json \
  app/db
```

### Exemple certificat

```bash
bao kv put secret/tls/api \
  cert=@tls.crt \
  key=@tls.key \
  chain=@ca.crt
```

```bash
bao kv metadata patch \
  -mount=secret \
  -custom-metadata=sync.enabled=true \
  -custom-metadata=sync.provider=azure \
  -custom-metadata=sync.type=azure-keyvault-certificate \
  -custom-metadata=sync.destination=https://my-vault.vault.azure.net/ \
  -custom-metadata=sync.name=tls-api \
  -custom-metadata=sync.format=certificate \
  -custom-metadata=sync.cert_field=cert \
  -custom-metadata=sync.key_field=key \
  -custom-metadata=sync.chain_field=chain \
  tls/api
```

---

## Déclenchement de la synchronisation

### Version 1 recommandée

Utiliser les **audit logs OpenBao**.

Le controller lit les événements d'audit et détecte les changements sur :

```text
<mount>/data/<path>
<mount>/metadata/<path>
<mount>/delete/<path>
<mount>/undelete/<path>
<mount>/destroy/<path>
```

À chaque événement pertinent :

1. extraire le mount et le path logique
2. lire `metadata/<path>`
3. vérifier `custom_metadata.sync.enabled == true`
4. lire `data/<path>`
5. vérifier la version courante
6. comparer avec l'état local
7. synchroniser vers Azure
8. enregistrer le résultat

### Alternative

Faire du polling périodique, mais c'est moins élégant et potentiellement coûteux.

---

## État de synchronisation

Pour éviter les resync inutiles, stocker localement :

```text
mount
path
version
content_checksum
metadata_checksum
destination
remote_name
last_sync_at
last_status
last_error
```

Backend possible pour la v1 :

- SQLite
- fichier BoltDB
- PostgreSQL plus tard

Pour un prototype GoLand, SQLite est simple et suffisant.

---

## Structure de projet proposée

```text
openbao-kv-azure-sync/
├── cmd/
│   └── openbao-kv-azure-sync/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── watcher/
│   │   ├── watcher.go
│   │   └── auditlog_watcher.go
│   ├── openbao/
│   │   ├── client.go
│   │   ├── kvv2.go
│   │   └── metadata.go
│   ├── policy/
│   │   ├── sync_metadata.go
│   │   └── parser.go
│   ├── transform/
│   │   ├── transformer.go
│   │   ├── raw.go
│   │   ├── json.go
│   │   ├── certificate.go
│   │   └── key.go
│   ├── azure/
│   │   ├── destination.go
│   │   ├── keyvault_secret.go
│   │   ├── keyvault_certificate.go
│   │   ├── keyvault_key.go
│   │   └── app_configuration.go
│   ├── state/
│   │   ├── store.go
│   │   └── sqlite_store.go
│   └── sync/
│       ├── controller.go
│       └── reconciler.go
├── configs/
│   └── config.example.yaml
├── deployments/
│   ├── docker-compose.yaml
│   └── openbao.hcl
├── testdata/
│   └── audit.log
├── go.mod
├── README.md
└── TODO.md
```

---

## Configuration proposée

`configs/config.example.yaml`

```yaml
openbao:
  address: "http://127.0.0.1:8200"
  token: "dev-token"
  mounts:
    - "secret"

watcher:
  type: "auditlog"
  audit_log_path: "./testdata/audit.log"

state:
  type: "sqlite"
  dsn: "./sync-state.db"

azure:
  auth:
    type: "environment"
  defaults:
    key_vault_url: "https://my-vault.vault.azure.net/"

sync:
  dry_run: true
  worker_count: 2
```

---

## Types de synchronisation

### `azure-keyvault-secret`

Transforme les données KV en secret Azure Key Vault.

Formats possibles :

```text
raw
json
```

Exemple :

```text
sync.type=azure-keyvault-secret
sync.name=my-secret
sync.format=json
```

### `azure-keyvault-certificate`

Transforme les champs `cert`, `key`, `chain` en certificat importable dans Azure Key Vault.

Exemple :

```text
sync.type=azure-keyvault-certificate
sync.cert_field=cert
sync.key_field=key
sync.chain_field=chain
```

### `azure-keyvault-key`

Pour synchroniser une clef vers Azure Key Vault Keys.

À traiter prudemment : selon le cas, il peut être préférable de créer la clef directement dans Azure et de ne jamais exporter de private key depuis OpenBao.

### `azure-app-configuration`

Pour synchroniser certains secrets ou paramètres applicatifs vers Azure App Configuration.

---

## Règles de sécurité

- Ne jamais logger les valeurs des secrets.
- Logger uniquement mount, path, version, type, destination logique et statut.
- Supporter un mode `dry_run`.
- Prévoir une allowlist de mounts.
- Ne synchroniser que si `sync.enabled=true`.
- Prévoir un mécanisme de retry avec backoff.
- Prévoir une gestion claire des erreurs permanentes vs temporaires.
- Optionnel : signer ou hasher les payloads pour détecter les changements.
- Éviter de synchroniser des private keys sauf besoin explicite.

---

## Premier MVP

### Scope MVP

- Lire un audit log fichier.
- Détecter les écritures sur `secret/data/*`.
- Lire les metadata KV v2.
- Parser les `custom_metadata`.
- Si `sync.enabled=true` et `sync.type=azure-keyvault-secret`, lire la dernière version.
- Transformer en JSON.
- En mode `dry_run`, afficher l'action sans secret.
- En mode réel, pousser vers Azure Key Vault Secret.
- Stocker l'état dans SQLite.

### Hors scope MVP

- Certificats.
- Azure Key Vault Keys.
- Azure App Configuration.
- Reconciliation complète.
- Kubernetes operator.
- Plugin OpenBao.
- UI.

---

## TODO initial

1. Initialiser le module Go.
2. Créer la configuration YAML.
3. Créer un client OpenBao minimal.
4. Implémenter la lecture KV v2 data + metadata.
5. Implémenter le parser des custom metadata.
6. Implémenter le watcher audit log.
7. Implémenter le reconciler.
8. Implémenter le mode dry-run.
9. Implémenter Azure Key Vault Secret.
10. Ajouter les tests unitaires.
11. Ajouter un docker-compose OpenBao dev.
12. Ajouter un README de démo.

---

## Prompt Codex pour GoLand

Utilise ce prompt dans Codex :

```text
Tu es dans un projet Go appelé openbao-kv-azure-sync.

Objectif : créer un controller Go externe qui synchronise certains secrets OpenBao KV v2 vers Azure, piloté par les custom_metadata du KV v2.

Contraintes importantes :
- Ne pas créer de plugin OpenBao pour le MVP.
- OpenBao KV v2 reste la source de vérité.
- Le controller observe les changements via un audit log OpenBao.
- Il ne doit jamais logger les valeurs des secrets.
- Il doit supporter un mode dry-run.
- Il doit lire les metadata KV v2 pour savoir si un secret doit être synchronisé.
- Il doit synchroniser uniquement si custom_metadata["sync.enabled"] == "true".
- Premier type supporté : azure-keyvault-secret.
- Premier format supporté : json.
- Stocker l'état de synchronisation dans SQLite.
- Prévoir une architecture propre avec des packages internal/config, internal/watcher, internal/openbao, internal/policy, internal/transform, internal/azure, internal/state, internal/sync.

Commence par générer :
1. go.mod
2. cmd/openbao-kv-azure-sync/main.go
3. internal/config/config.go
4. internal/policy/sync_metadata.go
5. internal/policy/parser.go
6. internal/watcher/watcher.go
7. internal/watcher/auditlog_watcher.go
8. internal/openbao/client.go
9. internal/openbao/kvv2.go
10. internal/sync/controller.go
11. configs/config.example.yaml
12. README.md
13. TODO.md

Le code doit être idiomatique Go, testable, avec interfaces là où c'est utile.
Pour Azure, crée d'abord une interface Destination et une implémentation DryRunDestination. L'implémentation réelle Azure Key Vault viendra ensuite.
```

---

## Notes pour la suite

À garder en tête pour les évolutions :

- Ajouter un reconciler périodique pour vérifier que l'état Azure correspond toujours à OpenBao.
- Ajouter la gestion des suppressions : delete, destroy, metadata delete.
- Ajouter les certificats.
- Ajouter la gestion des clefs.
- Ajouter Azure App Configuration.
- Ajouter des métriques Prometheus.
- Ajouter une image Docker.
- Ajouter un Helm chart.
- Ajouter un mode Kubernetes avec ConfigMap/Secret pour la configuration.
