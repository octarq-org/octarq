# DNS Plugin Specification (`plugins/dns/SPEC.md`)

## 1. Architectural Overview & System Role

The `dns` plugin (`octarq/plugins/dns`) is a core infrastructure plugin responsible for:
1. **Domain Lifecycle Management**: Managing custom domains and subdomains tied to DNS provider zones.
2. **Provider Credentials Management**: Secure storage of third-party DNS provider API credentials (Cloudflare, DNSPod) with AES-GCM encryption.
3. **Live DNS Record Operations**: Creating, updating, listing, and deleting live DNS records across providers.
4. **DNS Posture Verification**: Automated DNS probing of SPF, DKIM, DMARC, and CNAME resolution for mail and short-link hostnames.
5. **Email DNS Blueprint**: One-click generation, validation, and auto-provisioning of standard email DNS routing records (MX, SPF, DMARC).
6. **Dynamic DNS (DDNS) Engine**: Dyndns2-compatible dynamic DNS update service authenticated via SHA-256 token hashing.
7. **Cross-Plugin Seam Provider**: Providing the `plugin.DNSManager` ("dns.manager") service for Pro modules and AI MCP tools.

- **Plugin Identifier**: `"dns"` (`plugin.go:82`)
- **Plugin Metadata**:
  - Title: `"Domains & DNS"`
  - Description: `"DNS records, verification, and dynamic DNS."`
  - Category: `plugin.CategoryInfrastructure` (`"Infrastructure"`)
  - Tags: `["dns", "domains", "records", "ddns"]`
  - Enabled By Default: `true`
- **Implemented Interfaces**:
  - `plugin.Plugin` (`plugin.go:58`)
  - `plugin.Describer` (`plugin.go:59`)
  - `plugin.MenuProvider` (`plugin.go:60`)
  - `plugin.HelpDocsFS` (`plugin.go:61`)
  - `plugin.ExportFunc` (`plugin.go:68`)
  - `plugin.PurgeFunc` (`plugin.go:69`)
  - `plugin.OverviewFunc` (`plugin.go:70`)
  - `plugin.MCPExporter` (`plugin.go:71`)
  - `plugin.DNSManager` (`manager.go:19`)
- **Binary Embedding Guarantee**: Co-located at `plugins/dns/SPEC.md`. Strictly excluded from binary embedding (`//go:embed docs` in `plugin.go:122` only bundles user help docs under `docs/`). Zero binary bloat, zero secret leak.

---

## 2. Core Go Types & Compile-Time Assertions

### 2.1 Plugin Struct & Dependencies (`plugin.go:40-54`)

```go
type Plugin struct {
	db          *gorm.DB
	orgID       func(*http.Request) uint
	audit       func(r *http.Request, action, targetType string, targetID uint, meta map[string]any)
	encrypt     func(plaintext []byte) (string, error)
	decrypt     func(encoded string) ([]byte, error)
	requireRole func(r *http.Request, min string) bool

	publishEvent func(orgID uint, event string, data any)
	ctx          *plugin.Context

	// DNS resolvers, injectable so tests can stub them; default to net.
	lookupTXT   func(string) ([]string, error)
	lookupCNAME func(string) (string, error)
}
```

### 2.2 Compile-Time Interface Assertions (`plugin.go:57-72` & `manager.go:19`)

```go
var (
	_ plugin.Plugin       = (*Plugin)(nil)
	_ plugin.Describer    = (*Plugin)(nil)
	_ plugin.MenuProvider = (*Plugin)(nil)
	_ plugin.HelpDocsFS   = (*Plugin)(nil)
)

var (
	_ plugin.ExportFunc   = (*Plugin)(nil).exportData
	_ plugin.PurgeFunc    = (*Plugin)(nil).purge
	_ plugin.OverviewFunc = (*Plugin)(nil).overview
	_ plugin.MCPExporter  = (*Plugin)(nil).mcpExportDomains
	_ plugin.DNSManager   = (*Plugin)(nil).DNSManager()
)
```

### 2.3 Domain Models (`models.go:10-89`)

The DNS plugin defines three GORM models exclusively in `plugins/dns/models.go`. None of these models are mirrored in or managed by `internal/models`.

1. **`ProviderAccount`** (`models.go:12-24`):
   - Represents a configured DNS provider account.
   - Fields:
     - `ID uint`: Primary key.
     - `OrgID uint`: Tenant foreign key (`column:owner_id;index;default:1`).
     - `Name string`: Descriptive account name (e.g. "Personal Cloudflare").
     - `Type string`: Provider type (`"cloudflare"` or `"dnspod"`).
     - `Config string`: Encrypted JSON credentials payload (`type:text; json:"-"`).
     - `CreatedAt time.Time`, `UpdatedAt time.Time`.
     - `HasCredentials bool`: Computed unpersisted flag (`gorm:"-" json:"hasCredentials"`).
2. **`Domain`** (`models.go:27-45`):
   - Represents a domain managed within a tenant workspace.
   - Fields:
     - `ID uint`: Primary key.
     - `OrgID uint`: Tenant foreign key (`column:owner_id;index;default:1`).
     - `Name string`: Fully qualified domain name (`uniqueIndex;size:255`).
     - `ProviderAccountID uint`: Foreign key to `provider_accounts.id` (`index`).
     - `ZoneID string`: DNS provider zone identifier (`size:64`).
     - `Note string`: Optional tenant note.
     - `ForMail bool`: Master toggle for inbound email reception.
     - `ForLink bool`: Master toggle for short-link redirection.
     - `LinkHosts models.HostList`: Enabled hostnames for short links (`type:text`), imported from `internal/models`.
     - `MailHosts models.HostList`: Enabled hostnames for mailboxes (`type:text`), imported from `internal/models`.
     - `CreatedAt time.Time`, `UpdatedAt time.Time`.
   - Methods:
     - `EffectiveLinkHosts() []string`: Returns `d.LinkHosts.Enabled()`.
     - `EffectiveMailHosts() []string`: Returns `d.MailHosts.Enabled()`.
3. **`DDNSToken`** (`models.go:75-88`):
   - Dynamic DNS authentication token.
   - Table: `dns_ddns_tokens` (`TableName() string { return "dns_ddns_tokens" }`).
   - Fields:
     - `ID uint`: Primary key.
     - `OrgID uint`: Tenant foreign key (`column:owner_id;index;default:1`).
     - `DomainID uint`: Foreign key to `domains.id` (`index`).
     - `RecordName string`: FQDN to update (`size:255`).
     - `RecordType string`: Record type (`"A"` or `"AAAA"`, `size:8`).
     - `TokenHash string`: Hex-encoded SHA-256 hash of secret token (`uniqueIndex;size:64; json:"-"`).
     - `Label string`: Friendly identifier.
     - `LastIP string`: Last updated client IP address (`size:64`).
     - `LastSeenAt *time.Time`: Timestamp of last successful update.
     - `CreatedAt time.Time`.

### 2.4 Shared Model: `models.HostList` (`internal/models/models.go:178-244`)

The `Domain` model co-owns and imports `models.HostList` from core `internal/models/models.go:183`. It provides typed hostname list storage with soft-enable capabilities:

```go
type Host struct {
    Host    string `json:"host"`
    Enabled bool   `json:"enabled"`
}

type HostList []Host
```

- **Persistence & Serialization**:
  - `Value() (driver.Value, error)` (`models.go:186`): Serializes `[]Host` to a JSON array string. An empty list yields `"[]"`.
  - `Scan(v any) error` (`models.go:194`): Implements `sql.Scanner`, deserializing JSON byte slices or strings into `[]Host`. Returns an error on unsupported types.
- **Methods**:
  - `Enabled() []string` (`models.go:221`): Filters and returns only hostnames where `Enabled == true`.
  - `Blocks(host string) bool` (`models.go:233`): Reports whether `host` is present in the list but has every entry disabled (`Enabled == false`), signalling reverse proxies to drop traffic.
- **Cross-Subsystem Reuse**: `HostList` is shared between `plugins/dns` (`Domain`), `origin` (`origin.go:183` host routing), and consumers in `plugins/links` and `plugins/mail`.

### 2.5 Model Boundaries & Migration Co-Ownership

- **Exclusive Plugin Ownership**: `Domain`, `ProviderAccount`, and `DDNSToken` are defined solely in `plugins/dns/models.go`. They are **NOT** mirrored in `internal/models` and are excluded from `internal/models.AllModels()` (`internal/models/models.go:376-382`).
- **Migration Ownership**: The plugin owns the database schema for its tables. It registers them via `(p *Plugin) Models() []any` (`plugin.go:98-100`):
  ```go
  func (p *Plugin) Models() []any {
      return []any{&Domain{}, &ProviderAccount{}, &DDNSToken{}}
  }
  ```
  The core runtime automatically collects and migrates these models alongside core models during startup before HTTP listeners bind.
- **Boundary Contract**: Core `internal/models` owns system primitives (`Org`, `User`, `Setting`, `HostList`). Domain and DNS credentials tables are isolated strictly within `plugins/dns`.

---

## 3. Cryptographic Storage & Security Architecture

### 3.1 AES-GCM Credentials Encryption (`providers.go:15-24` & `plugin.go:255-272`)
- Credentials maps (`{"apiToken": "..."}`) are serialized to JSON and encrypted via `p.encrypt` (AES-GCM with unique nonce) before storing in `provider_accounts.config`.
- Cleartext credentials never leave the host:
  - In API responses, `Config` is serialized as `json:"-"`.
  - The computed field `HasCredentials = (Config != "")` informs the frontend without leaking secrets.
  - In audit log payloads, config maps are sanitized so all keys map to `"[REDACTED]"` (`providers.go:157-159`).
  - Decryption via `p.decrypt` occurs exclusively in `providerFor(dom Domain)` immediately before initializing `dnsprovider.Provider`.

### 3.2 Tenant Isolation & Anti-Oracle Protections (`providers.go:191-205`)
- All database queries strictly enforce `owner_id = orgID`.
- When deleting a provider account (`deleteProviderAccount`):
  - Ownership is verified **first** via `Where("id = ? AND owner_id = ?", id, orgID)`.
  - If not owned, returns `404 Not Found`.
  - Only after confirming ownership does it count referencing domains. This prevents cross-tenant existence enumeration attacks where a tenant tests if an ID exists in another org by observing `409 Conflict` vs `404 Not Found`.

### 3.3 Role-Based Access Control (RBAC) Gates
- All mutating endpoints enforce `hasRole(r, "admin")`.
- If `p.requireRole` is unwired by the host, `p.hasRole` defaults to `false` (fail-closed) to prevent unauthorized access (`plugin.go:280-285`).

### 3.4 Reserved Zone Security Architecture & Anti-Hijacking (`models.BaseDomain`)

To prevent tenant domain squatting and cross-tenant traffic hijacking, the DNS plugin enforces strict reserved zone validation backed by `models.BaseDomain(p.db)` (`internal/models/base_domain.go:32`).

- **Dynamic Base Domain Resolution**:
  - Setting: `models.BaseDomainSetting` (`"base_domain"` in `settings` table, configured via Settings → Instance).
  - Fallback: `models.BaseDomainEnv` (`OCTARQ_BASE_DOMAIN` environment variable).
  - Normalization: Strips port, scheme, and trailing dot. Returns `""` if unconfigured.
  - Automatic tenant subdomains are provisioned as `<org_slug>.<base_domain>`.
- **Threat Model**:
  - An attacker must not be able to manually register `base_domain` itself (e.g. `app.octarq.org`).
  - An attacker must not register an existing, unprovisioned, or retired tenant slug subdomain (e.g. `victim.app.octarq.org` or `oldslug.app.octarq.org` from `OrgSlugHistory`).
  - An attacker must not bypass apex checks by placing reserved subdomains into `LinkHosts` or `MailHosts` on an unrelated apex domain (e.g. `evil.com` with link host `victim.app.octarq.org`).
- **Triple-Gate Enforcement**:
  1. **Apex Registration Gate (`domains.go:97-99` & `hosts.go:30-37`)**:
     `underBaseZone(p.db, name)` checks whether `name == base || strings.HasSuffix(name, "."+base)`. If true, returns `HTTP 400 Bad Request: "that hostname is reserved for automatic tenant subdomains"`.
  2. **Host List Reservation Gate (`domains.go:133-135, 224-226` & `hosts.go:42-56`)**:
     `reservedInHostLists(p.db, dom.LinkHosts, dom.MailHosts)` scans all entries in `LinkHosts` and `MailHosts` against `models.BaseDomain(p.db)`. Rejects both create and update requests with `HTTP 400 Bad Request` if any host matches or is a subdomain of `base`.
  3. **Zone Sync Filter (`sync.go:61-75`)**:
     When syncing zones from external providers (`syncZones`), any zone matching or under `models.BaseDomain(p.db)` is dropped from discovery results.
- **Zone Boundary Secondary Guard (`hosts.go:58-75`)**:
  `hostsOutsideZone(dom.Name, dom.LinkHosts, dom.MailHosts)` verifies that every host in `LinkHosts` and `MailHosts` is either `dom.Name` or a strict subdomain of `dom.Name`. This prevents cross-tenant domain collisions outside the base zone.
- **Regression Tests**: Verified by `base_zone_test.go:28,134,174` (`TestCreateDomainRejectsReservedBaseZone`, `TestCreateDomainRejectsReservedHostLists`, `TestUpdateDomainRejectsReservedHostLists`).

---

## 4. State Machines & Core Workflows

### 4.1 Domain Provisioning & Validation State Machine (`domains.go:82-172`)

```
[Inbound Request]
       │
       ▼
[Admin Role & Org Check] ──(Fail)──► HTTP 401/403
       │ (Pass)
       ▼
[Reserved Base Zone Check] ──(In Base Zone)──► HTTP 400 (Reserved for tenant subdomains)
       │ (Pass)
       ▼
[Provider Account Ownership] ──(Not Owned)──► HTTP 404 (Provider account not found)
       │ (Pass)
       ▼
[Zone Boundary Verification] ──(Outside Zone)──► HTTP 400 (Host list outside domain zone)
       │ (Pass)
       ▼
[Quota Verification] ──(ErrQuotaUnavailable)──► HTTP 402 Payment Required
       │              ──(ErrQuotaExceeded)─────► HTTP 429 Too Many Requests
       │ (Pass)
       ▼
[Transactional DB Insert] ──(Duplicate Name)──► HTTP 409 Conflict
       │ (Success)
       ▼
[Cache Invalidation & Audit] ──► forgetOrigin(dom.Name) + audit("domain.create") + emit("domain.create")
```

### 4.2 DNS Posture Verification Engine (`records_verify.go`)
- **Probing Targets**:
  - Probes all enabled mail hosts (`dom.MailHosts.Enabled()`), falling back to apex `dom.Name`.
  - Probes all enabled link hosts (`dom.LinkHosts.Enabled()`).
- **Validation Rules**:
  - **SPF**: Probes TXT on host. Valid if contains `v=spf1`. Healthy if starts with `v=spf1`.
  - **DMARC**: Probes TXT on `_dmarc.<host>`. Valid if contains `v=dmarc1`. Healthy if policy `p=none|quarantine|reject` is specified.
  - **DKIM**: Concurrently queries 6 standard selectors (`default`, `octarq`, `google`, `mail`, `k1`, `sig1`) at `<selector>._domainkey.<host>`. Healthy if public key `p=` is present.
  - **Link CNAME**: Resolves CNAME. Healthy if resolved target equals apex domain or sub-zone (`.<apex>`).
- **Drift Alerting**: If mail hosts lack healthy SPF/DMARC or link hosts lack valid CNAMEs, dispatches webhook event `domain.verify_failed`.

### 4.3 Email DNS Blueprint Automation (`email_blueprint.go`)
- **Blueprint Template**:
  - `MX @ route1.mx.cloudflare.net` (Priority 10, TTL 1)
  - `MX @ route2.mx.cloudflare.net` (Priority 53, TTL 1)
  - `TXT @ v=spf1 include:_spf.mx.cloudflare.net ~all` (TTL 1)
  - `TXT _dmarc v=DMARC1; p=none; sp=none;` (TTL 1)
- **Status Computation**:
  - Normalizes record names against apex domain.
  - Compares with live provider records: `ok` (matches), `missing` (record absent), `mismatch` (name exists with differing content).
- **One-Click Apply (`applyEmailBlueprint`)**:
  - Enforces `hasRole(r, "admin")`.
  - Skips `ok` records (idempotent).
  - Calls `prov.CreateRecord` for missing or mismatched records.
  - Dispatches audit event `email_blueprint_applied` and webhook event `domain.email_blueprint_applied`.

### 4.4 Dynamic DNS (DDNS) Engine (`ddns_crud.go`, `ddns_update.go`, `ddns_secret.go`)
- **Secret Security**: Generates 24-byte crypto/rand secret. Persists only hex-encoded SHA-256 hash. Secret returned exactly once upon creation.
- **Protocol Endpoint**: `GET /api/dns/ddns/update` and `POST /api/dns/ddns/update`.
  - Public route bypasses session authentication (`Metadata: map[string]any{"public": true}`).
  - Authenticates by matching SHA-256 hash of query parameter `token` against `dns_ddns_tokens.token_hash`.
  - Resolves IP from query `ip`, form value `ip`, or caller remote IP (`clientIP`).
  - Respects proxy configuration: Reads `X-Forwarded-For` or `X-Real-IP` if `trustProxy == true`.
  - Response protocol (`text/plain; charset=utf-8`):
    - `badauth`: Token is missing or invalid.
    - `dnserr`: Domain/provider lookup failed or provider update error.
    - `nochg <ip>`: Record already matches IP.
    - `good <ip>`: Record created or updated successfully.
  - Updates `last_ip` and `last_seen_at` on every successful request.

---

## 5. Database Tables & Schemas

```sql
-- Provider Accounts Table
CREATE TABLE provider_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER DEFAULT 1,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(32) NOT NULL,
    config TEXT,
    created_at DATETIME,
    updated_at DATETIME
);
CREATE INDEX idx_provider_accounts_owner_id ON provider_accounts(owner_id);

-- Domains Table
CREATE TABLE domains (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER DEFAULT 1,
    name VARCHAR(255) UNIQUE NOT NULL,
    provider_account_id INTEGER,
    zone_id VARCHAR(64),
    note TEXT,
    for_mail BOOLEAN DEFAULT FALSE,
    for_link BOOLEAN DEFAULT FALSE,
    link_hosts TEXT,
    mail_hosts TEXT,
    created_at DATETIME,
    updated_at DATETIME
);
CREATE INDEX idx_domains_owner_id ON domains(owner_id);
CREATE INDEX idx_domains_provider_account_id ON domains(provider_account_id);

-- DDNS Tokens Table
CREATE TABLE dns_ddns_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER DEFAULT 1,
    domain_id INTEGER,
    record_name VARCHAR(255) NOT NULL,
    record_type VARCHAR(8) NOT NULL,
    token_hash VARCHAR(64) UNIQUE NOT NULL,
    label VARCHAR(255),
    last_ip VARCHAR(64),
    last_seen_at DATETIME,
    created_at DATETIME
);
CREATE INDEX idx_dns_ddns_tokens_owner_id ON dns_ddns_tokens(owner_id);
CREATE INDEX idx_dns_ddns_tokens_domain_id ON dns_ddns_tokens(domain_id);
```

---

## 6. Extension Seams & External Integration Points

### 6.1 Provided Services (`Mount` in `plugin.go:199-204`)
1. **`plugin.ServiceDNSManager` ("dns.manager")**:
   - Backed by `dnsManager` (`manager.go:15-35`).
   - Pure Go interface implementation:
     - `List(ctx context.Context, orgID, domainID uint) ([]plugin.DNSRecord, error)`
     - `Set(ctx context.Context, orgID, domainID uint, r plugin.DNSRecord) (plugin.DNSRecord, error)`
     - `Delete(ctx context.Context, orgID, domainID uint, recordID string) error`
   - Scoped strictly by `orgID` to prevent tenant crossing.
2. **`plugin.OverviewServiceName("dns")` (`"dns.overview"`)**: Returns counts for total domains, link domains, and mail domains.
3. **`plugin.PurgeServiceName("dns")` (`"dns.purge"`)**: Purges domains, DDNS tokens, and provider accounts for `orgID`; calls `forgetOrigin` on all domain names.
4. **`plugin.ExportServiceName("dns")` (`"dns.export"`)**: Exports domains and provider accounts for tenant data portability.
5. **`plugin.MCPExportServiceName("domains")` (`"domains.mcp_export"`)**: Returns tenant domains for MCP AI context.
6. **`"dns.trust_proxy"`**: Exposes `SetTrustProxy(bool)` for reverse proxy IP resolution.

### 6.2 Consumed Core Facilities
- `ctx.DB`: SQLite / PostgreSQL database connection.
- `ctx.OrgID`: Tenant context resolution.
- `ctx.Audit`: Structured audit logging.
- `ctx.Encrypt` / `ctx.Decrypt`: AES-GCM cipher suite.
- `ctx.PublishEvent`: Tenant webhook and lifecycle event dispatch.
- `models.BaseDomain(p.db)` (`internal/models/base_domain.go:32`): Instance base domain resolution for reserved zone validation.
- `models.HostList` (`internal/models/models.go:183-244`): Typed JSON array driver for short link and mailbox hostname persistence.
- `plugin.CheckQuota`: Tenant resource ceiling checks for `customDomains`.
- `origin.ClearDomainCache`: Core reverse-proxy domain routing cache eviction.

---

## 7. Invariant Guarantees & Fail-Closed Behaviors

1. **Anti-Oracle Provider Deletion**: Ownership of provider accounts is validated before counting referencing domains, preventing cross-tenant existence enumeration attacks.
2. **Fail-Closed Unwired RBAC**: When `requireRole` is nil, `hasRole` immediately returns `false`, preventing unauthorized admin actions when authorization infrastructure is offline.
3. **Encrypted at Rest**: API keys and tokens for Cloudflare and DNSPod are always AES-GCM encrypted and never logged or exposed via API JSON responses.
4. **Tenant Scoped DNSManager**: The `dns.manager` service requires a non-zero `orgID` (`errOrgRequired`), failing closed if a caller omits tenant boundaries.
5. **Zero Binary Embedding**: `SPEC.md` resides in the plugin root and is strictly omitted from binary compilation (`//go:embed docs`), preserving security boundaries.
6. **Reserved Base Zone Invariant**: The instance base domain (resolved dynamically via `models.BaseDomain(p.db)`) and all of its subdomains are reserved. The DNS plugin rejects apex registrations, host list entries, and zone synchronizations that match or fall under this base, preventing tenant subdomain hijacking and squatting.
7. **Plugin Model Ownership Isolation**: `Domain`, `ProviderAccount`, and `DDNSToken` are exclusively owned and migrated by `plugins/dns` via `Models() []any`. They are excluded from core `models.AllModels()`, guaranteeing modular decoupling and zero core schema pollution.
