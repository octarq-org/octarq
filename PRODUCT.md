# Product — Octarq Core (OSS)

Octarq Core is a self-hosted, single-binary operational back office for indie founders, one-person companies, and small AI-native teams. Core supplies shared identity, workspace tenancy, encrypted secrets storage, audit logging, webhooks, notifications, job queues, and an agent-safe MCP surface.

## Target Personas

1. **Workspace Owner (`owner`)**: Boots and operates the self-hosted Octarq workspace, configures billing/domain/third-party integrations, delegates access to admins/members, and controls destructive actions (deleting credentials, exporting `.env` bundles, purging workspace data).
2. **Workspace Admin (`admin`)**: Configures day-to-day operational integrations (LLM providers, payment rails, object storage, communication relays, DNS keys), rotates credentials, imports `.env` secrets, and issues scoped read-only CI service tokens.
3. **Workspace Member (`member`)**: Uses approved workspace integrations, checks connection health and TOTP codes, and stores their own isolated Personal Vault credentials (personal API keys, SSH keys, logins, 2FA seeds) without touching shared workspace secrets.
4. **Instance Administrator (`IsInstanceAdmin`)**: Manages host-wide infrastructure secrets (OAuth client secrets, metrics bearer token, instance SMTP/storage defaults) across the entire Octarq deployment.

## Core User Stories & End-to-End Paths

### 1. Workspace Credential & Integrations Hub (`/settings/integrations`, Workspace scope)

- **Who**: Workspace Owner or Admin (create/update/reveal/rollback/set default/import/service tokens) and Workspace Member (view metadata, test connection, view TOTP, view version history).
- **Path**:
  1. Sign in and select an active workspace.
  2. Navigate to **Settings › Integrations** (`/settings/integrations`) with the **Workspace** scope active.
  3. Filter by collection, environment (`default`, `prod`, `staging`, `dev`), category (`Integrations`, `Secrets & Infra`, `Logins & 2FA`), or search query (`?q=`).
  4. Click **New item**, choose a schema template (or custom KV fields), enter secret values, and save. Octarq seals the payload under a per-item key and assigns a `vault://` reference.
  5. Admins can **Reveal** (audit-logged), **Edit** (creates a new immutable version), **Rollback** to an earlier version, **Set default** for a provider type, **Import** `.env`/JSON secrets in bulk, or mint read-only **Service Tokens** (`octvq_...`) scoped to a collection, environment, and folder prefix for CI pipelines (`GET /api/vault/raw`).
  6. Owners can additionally **Export** workspace secrets (`.env` or JSON) and permanently **Delete** items (which shreds the item's encryption key).

### 2. Personal Vault Self-Service (`/settings/integrations`, Personal scope)

- **Who**: Any authenticated workspace Member, Admin, or Owner.
- **Path**:
  1. Open **Settings › Integrations** (`/settings/integrations`).
  2. Switch the scope selector from **Workspace** to **Personal**.
  3. Click **New item** to store private API tokens, SSH private keys, database credentials, logins, or 2FA TOTP secrets isolated to `(org_id, scope = "user", owner_user_id)`.
  4. Reveal, edit, view/roll back versions, generate live TOTP codes, test credentials, or delete personal items without requiring workspace admin/owner privileges. Other workspace members and admins cannot see or access another user's personal items.

### 3. Instance System Vault (`/instance/settings`, Instance Vault section)

- **Who**: Instance Administrator (`IsInstanceAdmin`).
- **Path**:
  1. Open **Instance Settings** (`/instance/settings`).
  2. Scroll to the **Instance Vault** section.
  3. Click **Add instance secret** to store or rotate system-wide secrets (`org_id = 0`, `scope = "instance"`, e.g. `instance.oauth.google`, `instance.oauth.github`, `instance.metrics_token`).
  4. Reveal or delete instance secrets with instance-level audit logging. Workspace users cannot list or resolve instance secrets.

### 4. Agent-Safe Credential Operations over MCP

- **Who**: AI agents connected to Octarq's MCP endpoint with a workspace token.
- **Path**:
  1. Agent calls `octarq_vault__list_items` or `octarq_vault__get_item_meta` to discover `vault://` references and non-secret metadata.
  2. Admin/Owner agents can call `octarq_vault__set_secret` to provision or rotate workspace secrets.
  3. Agents can verify health via `octarq_vault__test_integration` or obtain a time-bound 6-digit code via `octarq_vault__get_totp` — plaintext secrets are never returned into LLM context.
