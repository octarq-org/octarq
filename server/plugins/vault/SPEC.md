# Module Specification: Credential & Secrets Vault (`server/plugins/vault`)

## 1. Overview & Architectural Boundaries

`server/plugins/vault` is the single authoritative credential and secret management base for the entire Octarq platform (Core OSS, Community Plugins, and Pro Modules).

- **Module identifier**: `vault` (`Core: true`, `EnabledByDefault: true`)
- **Services provided**:
  - `plugin.ServiceVault` (`plugin.VaultService`)
  - `plugin.ServiceCredentialResolver` (`plugin.CredentialResolver`)
  - `vault.purge` (`plugin.PurgeFunc`)
  - `vault.export` (`plugin.ExportFunc`)

## 2. Data Model

- `vault_collections`: Organizes items into workspaces/projects (`kind`: `integration`, `project`, `vault`, `system`) scoped by `(org_id, scope, slug)`.
- `vault_items`: Stores searchable non-secret metadata (`secret_id`, `org_id`, `scope`, `owner_user_id`, `collection_id`, `environment`, `folder_path`, `item_type`, `provider_type`, `provider`, `key_name`, `name`, `tags_json`, `is_default`, `current_version`, `expires_at`, `rotation_interval_days`, `last_rotated_at`, `metadata_json`).
- `vault_item_versions`: Immutable append-only encrypted snapshots (`item_id`, `version`, `ciphertext`, `field_schema_json`, `created_by`, `change_note`).
- `vault_service_tokens`: Scoped machine tokens (`token_prefix`, `token_hash`, `collection_id`, `environment`, `folder_prefix`, `permissions`, `expires_at`, `revoked_at`).

## 3. Cryptographic Isolation & Fail-Closed Guards

- **Per-Item Subject DEK**: Every item seals its versions using `host.Crypto().SubjectVault(orgID, "vault.item." + secretID)` with AAD `vault:v1:<orgID>:<secretID>:<version>`.
- **Cryptographic Shredding**: Deleting an item or purging a workspace hard-deletes `vault_item_versions` and `vault_items` rows in a transaction, then calls `ShredSubjectKey`.
- **Startup Guards**:
  - Implements `plugin.EncryptedDataReporter` to halt boot if a fresh envelope DEK would orphan existing `vault_item_versions` ciphertext.
  - Implements `plugin.Validator` to verify `host.Crypto() != nil` and `HasSubjectKey` for every active encrypted item.

## 4. Scope & Tenant Isolation

- `scope = "org"` (`org_id > 0`): Strict workspace isolation; `orgID == 0` is always rejected.
- `scope = "user"` (`org_id > 0`, `owner_user_id > 0`): Personal workspace items isolated to the owning user.
- `scope = "instance"` (`org_id = 0`): System-level credentials (OAuth client secrets, metrics token, system SMTP, instance S3) accessible only via `ResolveInstanceSecret` / `PutInstanceSecret` and `/api/instance/vault/*` (`IsInstanceAdmin`).
