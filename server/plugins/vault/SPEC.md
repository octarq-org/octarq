# Module Specification: Credential & Secrets Vault (`server/plugins/vault`)

## 1. Overview & Architectural Boundaries

`server/plugins/vault` is the single authoritative credential and secret management base for the entire Octarq platform (Core OSS, Community Plugins, and downstream modules).

- **Module identifier**: `vault` (`Core: true`, `EnabledByDefault: true`)
- **Services provided**:
  - `plugin.ServiceVault` (`plugin.VaultService`)
  - `plugin.ServiceCredentialResolver` (`plugin.CredentialResolver`)
  - `vault.purge` (`plugin.PurgeFunc`)
  - `vault.export` (`plugin.ExportFunc`)
- **Binary embedding invariant**: This `SPEC.md` is strictly excluded from `//go:embed docs` (`HelpDocsFS`), which embeds only `server/plugins/vault/docs/`.

## 2. Data Model

- `vault_collections`: Organizes items into workspaces/projects (`kind`: `integration`, `project`, `vault`, `system`) uniquely scoped by `(org_id, scope, slug)`.
- `vault_items`: Stores searchable non-secret metadata (`secret_id`, `org_id`, `scope`, `owner_user_id`, `collection_id`, `environment`, `folder_path`, `item_type`, `provider_type`, `provider`, `key_name`, `name`, `tags_json`, `is_default`, `current_version`, `expires_at`, `rotation_interval_days`, `last_rotated_at`, `metadata_json`).
- `vault_item_versions`: Immutable append-only encrypted snapshots (`org_id`, `item_id`, `version`, `ciphertext`, `field_schema_json`, `created_by`, `change_note`) uniquely indexed on `(item_id, version)`.
- `vault_service_tokens`: Scoped read-only machine tokens (`org_id`, `name`, `token_prefix`, `token_hash`, `collection_id`, `environment`, `folder_prefix`, `permissions`, `expires_at`, `last_used_at`, `revoked_at`, `created_by`).

## 3. Cryptographic Isolation & Fail-Closed Guards

- **Per-Item Subject DEK**: Every item seals its versions using `host.Crypto().SubjectVault(orgID, "vault.item." + secretID)` with AAD `vault:v1:<orgID>:<secretID>:<version>`.
- **Cryptographic Shredding**: Deleting an item (`DeleteSecret`, `DeleteInstanceSecret`) or purging a workspace (`purge`) hard-deletes `vault_item_versions` and `vault_items` rows inside a transaction, then calls `ShredSubjectKey`.
- **Startup Guards**:
  - Implements `plugin.EncryptedDataReporter` (`HasEncryptedData`) to halt boot if a fresh envelope DEK would orphan existing `vault_item_versions` ciphertext.
  - Implements `plugin.Validator` (`Validate`) to verify `host.Crypto() != nil` and `HasSubjectKey` for every active encrypted item (`current_version > 0`), then executes `MigrateLegacyCoreSecrets`.
- **Workspace Metadata Export (`vault.export`)**: Returns `VaultItemMeta` only; plaintext secrets and ciphertexts are never included in workspace metadata exports.

## 4. Scope & Tenant Isolation

- `scope = "org"` (`org_id > 0`): Strict workspace isolation; `orgID == 0` is always rejected. This is the only scope the vault HTTP API serves.
- `scope = "user"` (`org_id > 0`, `owner_user_id > 0`): Service-level private storage for feature plugins (2FA, pt). The vault HTTP API never lists, creates, or addresses these items. `PutSecret` requires `OwnerUserID`; a missing owner fails closed.
- `scope = "instance"` (`org_id = 0`): System-level credentials (OAuth client secrets, metrics token, system SMTP, instance S3) accessible only via `ResolveInstanceSecret` / `PutInstanceSecret` / `DeleteInstanceSecret` and `/api/instance/vault/*` (`IsInstanceAdmin`).
- **Search Query Isolation**: `ListItems` parenthesizes `?q=` `OR` predicates (`(LOWER(name) LIKE ? OR LOWER(key_name) LIKE ? OR LOWER(provider) LIKE ? OR LOWER(tags_json) LIKE ?)`) so `org_id`, `scope`, and `owner_user_id` filters always bind across all search branches.

## 5. HTTP Endpoints & RBAC Matrix

### Workspace Endpoints (`/api/vault/*`)

The HTTP API exposes one workspace vault. `GET /api/vault/items` and `POST /api/vault/items` accept an empty `scope` or `scope=org`. Every other non-empty value returns `400` (`Invalid vault scope.`). All endpoints require an authenticated user (`401`) and an active workspace (`400` if `orgID == 0`), plus `RequirePerm`:

| Method & Path | Min Role | Notes |
| --- | --- | --- |
| `GET /api/vault/schemas` | Public / Any | Returns `SchemasCatalog()` metadata templates |
| `GET /api/vault/collections` | `member` | Auto-ensures `integrations` and `default` collections |
| `POST /api/vault/collections` | `admin` | Rejects empty name (`400`) or duplicate slug (`409`) |
| `DELETE /api/vault/collections/{id}` | `owner` | Rejects non-empty collection (`409`) |
| `GET /api/vault/items` | `member` | `scope` defaults to `org`; rejects `user` or unknown scope (`400`) |
| `POST /api/vault/items` | `admin` | Rejects `scope=user` or unknown scope (`400`) and invalid payload (`400`) |
| `GET /api/vault/items/{id}` | `member` | Returns `VaultItemMeta` only; non-org items read as `404` |
| `PUT /api/vault/items/{id}` | `admin` | Appends a new version if the payload changes |
| `DELETE /api/vault/items/{id}` | `owner` | Hard-deletes item + versions and shreds Subject DEK |
| `POST /api/vault/items/{id}/reveal` | `admin` | Decrypts active version and writes `vault.secret.revealed` audit |
| `GET /api/vault/items/{id}/versions` | `member` | Lists immutable version metadata (no ciphertext/plaintext) |
| `POST /api/vault/items/{id}/rollback/{version}` | `admin` | Appends a new version restored from a historical version |
| `POST /api/vault/items/{id}/default` | `admin` | Sets `is_default = true` for the org provider type |
| `POST /api/vault/items/{id}/test` | `member` | Validates credential/endpoint via `safehttp` SSRF guard |
| `GET /api/vault/items/{id}/totp` | `member` | Computes 6-digit RFC-6238 code without revealing the seed |
| `POST /api/vault/import` | `admin` | Bulk `.env` or JSON import |
| `GET /api/vault/export` | `owner` | Plaintext `.env` or JSON export, audit-logged |
| `GET /api/vault/tokens` | `admin` | Lists workspace service tokens (hashes never returned) |
| `POST /api/vault/tokens` | `admin` | Mints `octvq_` token (`read` only); plaintext returned once |
| `DELETE /api/vault/tokens/{id}` | `admin` | Revokes token (`revoked_at = now`) |

### Instance-Admin System Vault (`/api/instance/vault/*`)

All routes require authenticated `IsInstanceAdmin(r)` (`401` / `403`):

- `GET /api/instance/vault/items`: Lists `org_id = 0, scope = "instance"` metadata.
- `POST /api/instance/vault/items`: Upserts an instance secret via `PutInstanceSecret` (audited as `vault.instance.upserted`).
- `POST /api/instance/vault/items/{id}/reveal`: Decrypts and returns an instance secret (audited as `vault.instance.revealed`).
- `DELETE /api/instance/vault/items/{id}`: Hard-deletes and shreds an instance secret's DEK (audited as `vault.instance.deleted`).

## 6. Service Tokens & Raw Pull (`/api/vault/raw`)

- **Token Format**: `octvq_` + 48 hex chars (`crypto/rand`). Only the 14-char prefix and SHA-256 `token_hash` are persisted.
- **Permissions**: Only `"read"` is supported (`""` normalizes to `"read"`; any other value including `"read_write"` is rejected fail-closed).
- **Raw Pull (`GET /api/vault/raw`)**:
  - Authenticates `Authorization: Bearer octvq_...`, rejecting missing, unknown, revoked, or expired tokens (`401`).
  - Queries strictly `org_id = tok.OrgID AND scope = "org" AND collection_id = tok.CollectionID AND environment = tok.Environment`.
  - Enforces path-boundary folder matching: `tok.FolderPrefix == "/"` OR `row.FolderPath == tok.FolderPrefix` OR `strings.HasPrefix(row.FolderPath, tok.FolderPrefix+"/")` (so `/app` matches `/app` and `/app/db`, never `/apple`).
  - Updates `last_used_at` and records `vault.service_token.pulled` audit log (via `ctx.Audit` or direct `audit_logs` fallback if `ctx.Audit` is unset).
  - Supports `?format=env` (`text/plain`) and JSON (`{ "environment": ..., "secrets": ... }`).

## 7. `vault://` Reference Grammar

- `vault://id/<itemID>`: Resolves by numeric item ID within `org_id = orgID AND scope = "org"`.
- `vault://<collection>/<env>/<keyName>` (root folder `/`) or `vault://<collection>/<env>/<folder>/<keyName>`: Resolves within `org_id = orgID AND scope = "org"`.
- **Strict Scope Boundary**: `ResolveByRef` never resolves `VaultScopeUser` or `VaultScopeInstance` items.

## 8. MCP Tool Contracts (`octarq_vault__*`)

Implemented via `plugin.MCPProvider` (`MCPTier() == ""`, Core):

- `octarq_vault__list_items`: Lists `VaultScopeOrg` item metadata and `vault://` references for `OrgIDFromContext(ctx)`. Never returns plaintext secrets.
- `octarq_vault__get_item_meta`: Returns non-secret metadata for one `VaultScopeOrg` item (`ErrSecretNotFound` for user/instance/foreign-org items).
- `octarq_vault__set_secret`: Creates or rotates a `VaultScopeOrg` item. Requires `RoleFromContext(ctx)` to be `"owner"` or `"admin"`.
- `octarq_vault__test_integration`: Decrypts in memory and tests connectivity/integrity for a `VaultScopeOrg` item, returning `{ "ok": bool, "message": string }` without secret values.
- `octarq_vault__get_totp`: Computes the live 6-digit RFC-6238 code and remaining seconds for a `VaultScopeOrg` item (`ErrSecretNotFound` for `VaultScopeUser`).

## 9. Legacy Migration (`MigrateLegacyCoreSecrets`)

Runs at boot inside `Validate` to deterministically migrate pre-Vault encrypted rows into `vault_items` + `vault_item_versions` under per-item `SubjectVault` DEKs:

1. `llm_gateway_providers` (`api_key_encr` -> `VaultScopeOrg`, `integrations` collection, `provider_type = "llm"`, links `vault_item_id`, clears `api_key_encr`).
2. `webhooks` (`secret` -> `VaultScopeOrg`, `item_type = "webhook"`, links `vault_item_id`, clears `secret`).
3. `notification_channels` (`config` ciphertext -> splits secret-like keys into `VaultScopeOrg`, keeps non-secret JSON metadata in `config`, links `vault_item_id`).
4. `settings` (`google_client_secret`, `github_client_secret`, `metrics_token` -> `PutInstanceSecret` under `instance.oauth.google`, `instance.oauth.github`, `instance.metrics_token`, clears legacy `settings.value`).

Migration is idempotent and fail-closed: any decryption or storage error aborts migration before clearing legacy columns.
