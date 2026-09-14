# Mail Plugin Living Specification (`plugins/mail/SPEC.md`)

## 1. Overview & Architectural Role

The `mail` plugin (`octarq/plugins/mail`) is the core messaging subsystem of Octarq. It provides an enterprise-grade, multi-tenant mail engine supporting:
- **Inbound Ingestion**: High-throughput public webhook ingress for standard RFC 822 MIME messages and Cloudflare Email Routing payloads, featuring constant-time tenant token verification, streaming MIME parsing, attachment extraction, deduplication, and automatic contact indexing.
- **Pluggable Blob Storage**: Separation of lightweight email metadata from multi-megabyte RFC 822 raw payloads via the `StorageProvider` extension seam, offloading payloads from the hot `emails` table into `mail_raw_blobs` (OSS default) or external S3/R2/MinIO object stores (Octarq Pro).
- **Outbound Transactional Delivery**: Hardened SMTP transmission featuring kernel-level dial-time SSRF guards, rate limiting, quota enforcement, suppression list checks, automated link tracking wrapping, and sent mail archiving.
- **Reputation & Bounce Management**: Automated ingestion and normalization of webhook bounces and complaints (AWS SES, Mailgun, SendGrid) with intelligent classification: permanent bounces and complaints are automatically added to the organization's suppression list, while transient bounces are logged without suppressing delivery.
- **Contacts & In-App Organization**: Automated contact interaction tracking, draft management, and multi-folder mailboxes (`inbox`, `sent`, `drafts`, `trash`, `spam`).

### Zero Binary Embedding Contract
This specification is a **Tier 2 Module Engineering Specification**. In strict accordance with `plugins/mail/lifecycle.go:106-107`:
```go
//go:embed docs
var docs embed.FS
```
Only the `docs/` directory is compiled into binary distributions (`plugin.HelpDocsFS`). `SPEC.md` is strictly excluded from Go binary compilation and is never exposed through `/api/help/` or public HTTP endpoints. Zero binary bloat, zero secret leak.

---

## 2. Go Types, Structs & Interfaces

All models and types are located in `plugins/mail/` and `octarq/plugin/`.

### 2.1 Domain Models (`plugins/mail/models.go` & `storage.go`)

```go
// Mailbox represents an address configured to receive email (prefix@domain).
// File: plugins/mail/models.go:8-17
type Mailbox struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id;index:idx_mailbox_owner_address,unique;default:1" json:"-"`
	Address   string    `gorm:"size:320;index:idx_mailbox_owner_address,unique" json:"address"`
	Note      string    `gorm:"type:text" json:"note"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Unread    int64     `gorm:"-" json:"unread"` // Computed at query time
}

// Email represents an ingested or outbound email record.
// File: plugins/mail/models.go:20-41
type Email struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	MailboxID      uint      `gorm:"index" json:"mailboxId"`
	MessageID      string    `gorm:"size:512;index" json:"messageId"`
	FromAddr       string    `gorm:"size:320" json:"from"`
	ToAddr         string    `gorm:"size:320" json:"to"`
	Subject        string    `gorm:"type:text" json:"subject"`
	Text           string    `gorm:"type:text" json:"text"`
	HTML           string    `gorm:"type:text" json:"html"`
	Raw            []byte    `json:"-"` // Legacy/fallback storage column
	StorageKey     string    `gorm:"size:255;index" json:"-"`
	Read           bool      `gorm:"default:false" json:"read"`
	Note           string    `gorm:"type:text" json:"note"`
	Attachments    string    `gorm:"type:text" json:"attachments"` // JSON array of Attachment descriptors
	AuthSPF        string    `gorm:"size:16" json:"authSpf"`       // pass|fail|softfail|neutral|none
	AuthDKIM       string    `gorm:"size:16" json:"authDkim"`      // pass|fail|none
	AuthDMARC      string    `gorm:"size:16" json:"authDmarc"`     // pass|fail|none
	Folder         string    `gorm:"size:32;index;default:'inbox'" json:"folder"` // inbox, sent, drafts, trash, spam
	UnsubscribeURL string    `gorm:"size:1024" json:"unsubscribeUrl"`
	ReceivedAt     time.Time `gorm:"index" json:"receivedAt"`
}

// SMTPSender holds outbound relay configuration and credentials.
// File: plugins/mail/models.go:43-58
type SMTPSender struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	OrgID     uint      `gorm:"column:owner_id;index" json:"-"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user"`
	Pass      string    `json:"-"` // Encrypted at rest via p.encrypt
	FromEmail string    `json:"fromEmail"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	PassSet   bool      `gorm:"-" json:"passSet"` // Virtual indicator for UI
}

// MailRawBlob stores raw RFC 822 EML blobs for the default database storage provider.
// File: plugins/mail/storage.go:13-20
type MailRawBlob struct {
	Key       string    `gorm:"primaryKey;size:255" json:"key"`
	Data      []byte    `json:"-"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MailSuppression records addresses blocked from outbound delivery.
// File: plugins/mail/models.go:61-70
type MailSuppression struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	OrgID     uint      `gorm:"column:owner_id;index:idx_mail_suppression_owner_address,unique;default:1" json:"-"`
	Address   string    `gorm:"size:320;index:idx_mail_suppression_owner_address,unique" json:"address"`
	Reason    string    `gorm:"size:32" json:"reason"` // hard_bounce, complaint, manual
	Source    string    `gorm:"type:text" json:"source"`
	Count     int       `gorm:"default:1" json:"count"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MailContact records interaction statistics for auto-completion and communication graphs.
// File: plugins/mail/models.go:73-82
type MailContact struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	OrgID            uint      `gorm:"column:owner_id;index:idx_contact_org_addr,unique;default:1" json:"-"`
	Address          string    `gorm:"size:320;index:idx_contact_org_addr,unique" json:"address"`
	Name             string    `gorm:"size:255" json:"name"`
	InteractionCount int       `gorm:"default:1" json:"interactionCount"`
	LastSeenAt       time.Time `gorm:"index" json:"lastSeenAt"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
```

### 2.2 Storage Provider Contract (`plugin/plugin.go:170-175`)

```go
type StorageProvider interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (int64, error)
}
```

### 2.3 MIME Parser Data Structures (`internal/mail/parse.go:39-70`)

```go
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
	Inline      bool   `json:"inline,omitempty"`
	ContentID   string `json:"contentId,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type AuthResults struct {
	SPF   string `json:"spf"`
	DKIM  string `json:"dkim"`
	DMARC string `json:"dmarc"`
}

type Parsed struct {
	MessageID       string
	From            string
	To              string
	Subject         string
	Text            string
	HTML            string
	Attachments     []Attachment
	ReceivedAt      time.Time
	Raw             []byte
	Auth            AuthResults
	PartErrors      int
	MaxPartsReached bool
}
```

---

## 3. State Transitions & Execution Pipelines

### 3.1 Inbound Email Reception Pipeline

```
Inbound HTTP Request (POST /api/webhook/{orgSlug}/email/inbound[/raw]/{token})
  │
  ├── 1. Tenant Authentication (Constant-Time Token Check)
  │      - Resolves org by slug
  │      - subtle.ConstantTimeCompare([]byte(input.Token), []byte(org.InboundToken))
  │      - On mismatch: audit recordInboundAuthFailure, returns 401 Unauthorized
  │
  ├── 2. Payload Buffering & Extraction
  │      - Capped at 25 MiB via io.LimitReader(r.Body, 25<<20)
  │      - If multipart/form-data: searches ["email", "raw", "body-mime", "message", "eml", "body"]
  │
  ├── 3. Resilient MIME Parsing (internal/mail/parse.go)
  │      - Limits: maxPartBytes = 10 MiB, maxParts = 200
  │      - Non-destructive parsing: bad parts logged; parses proceed to EOF
  │      - Inline image tracking: preserves Content-ID & Inline=true in Attachments
  │      - Duplicate HTML protection: keeps first non-empty HTML part
  │      - Fallback: unparseable raw bytes produce placeholder record (no email dropped)
  │
  ├── 4. Mailbox Resolution & Catch-All (plugins/mail/resolve.go)
  │      - Checks exact match in mailboxes table (owner_id = orgID, address = to, enabled = true)
  │      - Checks host status (mailHostDisabled); drops message if disabled
  │      - Catch-All Evaluation: if mail.catch_all == "true" AND address NOT in reserved list
  │        (admin, administrator, hostmaster, postmaster, webmaster) AND ownsMailHost(orgID, addr):
  │        -> auto-creates Mailbox{OrgID, Address: to, Enabled: true, Note: "auto (catch-all)"}
  │      - If unresolved: message dropped cleanly (returns {ok: true, stored: false})
  │
  ├── 5. Deduplication & List-Unsubscribe Extraction
  │      - Checks (mailbox_id, message_id); returns {stored: true, duplicate: true} if exists
  │      - Parses List-Unsubscribe header (RFC 2369 / RFC 8058), prioritizing HTTP(S) over mailto
  │
  ├── 6. Metadata Persistence & Storage Provider Offload
  │      - Inserts row into emails table (Folder: "inbox", Read: false)
  │      - Resolves StorageProvider via getStorageProvider()
  │      - Offloads raw RFC 822 EML to key: "mail/{orgID}/{emailID}.eml"
  │      - Fail-Closed Resilience: if storage provider Put fails, saves raw to emails.raw directly
  │
  ├── 7. Metering, EventBus & Pro Dispatches
  │      - Records usage: usagemetric.RawBytes and usagemetric.MailIn (1 unit)
  │      - Publishes "email.receive" via eventbus.Publish
  │      - Invokes async plugin.EmailEvent handlers (Inbox AI summarizer / OTP extractor)
  │      - Asynchronously notifies configured NotificationChannels via safego.Go
  │
  └── Returns HTTP 200 {ok: true, stored: true, id: email.ID}
```

### 3.2 Outbound Transactional Sending Pipeline

```
Outbound Send Request (POST /api/emails/send or mail.send / mail.send.system)
  │
  ├── 1. Recipient Suppression Enforcement
  │      - Checks mail_suppressions table for (owner_id = orgID, address = recipient)
  │      - Fails immediately with HTTP 400 if recipient is suppressed
  │
  ├── 2. Outbound Rate Limiting & Plan Quotas
  │      - Evaluates sendLimiter (100 emails/hour per organization)
  │      - Calls plugin.CheckQuota(ctx, orgID, "mailOutPerMonth", count)
  │      - Returns HTTP 402 Payment Required if plan lacks mail, or HTTP 429 if quota exceeded
  │
  ├── 3. SMTPSender Resolution & Credential Decryption
  │      - Selects SMTPSender for org (or system sender for system emails)
  │      - Decrypts sender password via p.decrypt(s.Pass)
  │      - Enforces From header: overrides From to sender.FromEmail (prevents relay spoofing)
  │
  ├── 4. Optional Link Tracking Wrapping (plugins/mail/send.go)
  │      - If TrackLinks=true, scans body URLs, filters out localhost/own host
  │      - Creates short links in links table and wraps hrefs with tracking redirects
  │
  ├── 5. Kernel-Level Dial-Time SMTP SSRF Guard (internal/mail/send.go & plugin/safehttp)
  │      - Joins host and port via net.JoinHostPort(s.host, s.port)
  │      - Net dialer configured with Control: safehttp.SMTPControl hook
  │      - Fires AFTER DNS resolution with the final IP address
  │      - Rejects loopback, private RFC 1918, CGNAT (100.64.0.0/10), IPv6 ULA, and metadata IPs
  │      - Relaxable ONLY via instance-wide OCTARQ_ALLOW_PRIVATE_SMTP=true
  │
  ├── 6. SMTP Transport Protocol Execution
  │      - Executes STARTTLS if server advertises capability
  │      - Requires AUTH if credentials exist (fails closed if server strips AUTH)
  │      - Delivers RFC 822 payload with header CRLF injection neutralization (stripCRLF)
  │
  ├── 7. Post-Send Archiving & Usage Accounting
  │      - Records usage: usagemetric.MailOut
  │      - Upserts recipients into mail_contacts table
  │      - Inserts sent email record into emails table (Folder: "sent", Read: true)
  │      - On failure: publishes webhook event "email.send_failed"
  │
  └── Returns HTTP 200 {ok: true}
```

### 3.3 Bounce & Complaint Processing Pipeline

```
Bounce Webhook Request (POST /api/webhook/{orgSlug}/email/bounce/{token})
  │
  ├── 1. Tenant Authentication (org.InboundToken verification)
  │
  ├── 2. AWS SNS Subscription Auto-Confirmation SSRF Guard
  │      - If Type == "SubscriptionConfirmation":
  │      - Validates SubscribeURL against isAWSSNSURL (must be https://sns.<region>.amazonaws.com)
  │      - Fetches confirmation using dial-guarded safehttp.NewClient(10s)
  │
  ├── 3. Event Extraction & Normalization (plugins/mail/bounces.go)
  │      - Normalizes formats from AWS SES, Mailgun, and SendGrid
  │      - Maps events into bounceEvent{Email, Event, BounceType, Details}
  │
  ├── 4. Intelligent Classification & Suppression
  │      - Hard Bounce (Permanent) or Complaint -> Inserts/Upserts into mail_suppressions
  │        (increments count, updates reason and source timestamp)
  │      - Soft Bounce (Transient / Undetermined) -> Logs audit record and fires alert notification;
  │        STRICTLY NEVER added to suppression list
  │
  └── 5. Audit Logging & Real-Time Reputation Alerts
         - Writes audit log entries (action: "email.bounce")
         - Dispatches alert message to enabled notification channels via safego.Go
```

---

## 4. Database Schema, Indexes & Tenant Views

The mail plugin registers 6 models in `p.Models()` and 1 Tenant SQL View in `RegisterViews(ctx)`:

### 4.1 Table Schemas & Index Definitions

#### 1. `mailboxes`
```sql
CREATE TABLE mailboxes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL DEFAULT 1,
    address VARCHAR(320) NOT NULL,
    note TEXT,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT idx_mailbox_owner_address UNIQUE (owner_id, address)
);
```

#### 2. `emails`
```sql
CREATE TABLE emails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    mailbox_id INTEGER NOT NULL,
    message_id VARCHAR(512),
    from_addr VARCHAR(320),
    to_addr VARCHAR(320),
    subject TEXT,
    text TEXT,
    html TEXT,
    raw BLOB,
    storage_key VARCHAR(255),
    read BOOLEAN NOT NULL DEFAULT 0,
    note TEXT,
    attachments TEXT,
    auth_spf VARCHAR(16),
    auth_dkim VARCHAR(16),
    auth_dmarc VARCHAR(16),
    folder VARCHAR(32) NOT NULL DEFAULT 'inbox',
    unsubscribe_url VARCHAR(1024),
    received_at DATETIME NOT NULL
);
CREATE INDEX idx_emails_mailbox_id ON emails(mailbox_id);
CREATE INDEX idx_emails_message_id ON emails(message_id);
CREATE INDEX idx_emails_storage_key ON emails(storage_key);
CREATE INDEX idx_emails_folder ON emails(folder);
CREATE INDEX idx_emails_received_at ON emails(received_at);
```

#### 3. `smtp_senders`
```sql
CREATE TABLE smtp_senders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    host VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL,
    user VARCHAR(255) NOT NULL,
    pass TEXT NOT NULL, -- Encrypted ciphertext
    from_email VARCHAR(320) NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX idx_smtp_senders_owner_id ON smtp_senders(owner_id);
```

#### 4. `mail_raw_blobs`
```sql
CREATE TABLE mail_raw_blobs (
    key VARCHAR(255) PRIMARY KEY,
    data BLOB NOT NULL,
    updated_at DATETIME NOT NULL
);
```

#### 5. `mail_suppressions`
```sql
CREATE TABLE mail_suppressions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL DEFAULT 1,
    address VARCHAR(320) NOT NULL,
    reason VARCHAR(32) NOT NULL, -- hard_bounce, complaint, manual
    source TEXT,
    count INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT idx_mail_suppression_owner_address UNIQUE (owner_id, address)
);
```

#### 6. `mail_contacts`
```sql
CREATE TABLE mail_contacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id INTEGER NOT NULL DEFAULT 1,
    address VARCHAR(320) NOT NULL,
    name VARCHAR(255),
    interaction_count INTEGER NOT NULL DEFAULT 1,
    last_seen_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT idx_contact_org_addr UNIQUE (owner_id, address)
);
CREATE INDEX idx_mail_contacts_last_seen_at ON mail_contacts(last_seen_at);
```

### 4.2 Tenant SQL View & Column Redactions (`views.go:15-39`)

- **View Name**: `tenant_emails`
- **Underlying SQL Definition**:
  ```sql
  SELECT e.id, e.mailbox_id, e.message_id, e.from_addr, e.to_addr, e.subject,
         e.text, e.html, e.storage_key, e.read, e.note, e.attachments,
         e.auth_spf, e.auth_dkim, e.auth_dmarc, e.received_at
  FROM emails e
  INNER JOIN mailboxes m ON e.mailbox_id = m.id
  WHERE m.owner_id = %d
  ```
- **Redaction Policy**: The columns `["subject", "text", "html", "raw", "storage_key"]` are formally registered as `Sensitive`. Non-admin AI agents and untrusted callers querying the database through the NL2SQL tenant view layer receive redacted representations.

---

## 5. Extension Seams & Integration Points

### 5.1 Services Provided (`plugins/mail/lifecycle.go:165-175`)

| Service Coordinate | Contract Type | Interface Signature / Description |
|---|---|---|
| `plugin.ServiceMailSend` (`"mail.send"`) | `plugin.MailSender` | `func(orgID uint, to, subject, htmlBody, textBody string) error`<br/>Transactional send through the organization's SMTP sender. |
| `plugin.ServiceMailSendSystem` (`"mail.send.system"`) | `plugin.SystemMailSender` | `func(to, subject, htmlBody, textBody string) error`<br/>Sends instance-level system mail (verification, password reset, invites) via the instance system sender. |
| `plugin.ServiceMailReady` (`"mail.ready"`) | `plugin.MailReady` | `func() bool`<br/>Returns true if at least one SMTP sender exists on the instance. |
| `plugin.ServiceMailDispatcher` (`"mail.dispatcher"`) | `plugin.EmailDispatcher` | `func(handler func(plugin.EmailEvent))`<br/>Registers inbound email hooks for Pro extensions (Inbox AI summarizer, OTP extractor). |
| `plugin.ServiceMailEmailGet` (`"mail.email.get"`) | `plugin.EmailGetter` | `func(orgID uint, id uint) (from, subject, body string, ok bool)`<br/>Fetches sanitized email text for AI processing. |
| `plugin.OverviewServiceName("mail")` (`"mail.overview"`) | `plugin.OverviewFunc` | `func(orgID uint, includeBot bool) map[string]any`<br/>Supplies mailbox count, email count, unread count, and recent emails. |
| `plugin.PurgeServiceName("mail")` (`"mail.purge"`) | `plugin.PurgeFunc` | `func(orgID uint) error`<br/>Cascading tenant deletion: wipes emails, blobs (DB & S3), mailboxes, senders, suppressions. |
| `plugin.ExportServiceName("mail")` (`"mail.export"`) | `plugin.ExportFunc` | `func(orgID uint) map[string]any`<br/>Serializes tenant mail data for compliance export (`GET /api/account/export`). |
| `plugin.MCPExportServiceName("mailboxes")` (`"mailboxes.mcp_export"`) | `plugin.MCPExporter` | `func(ctx context.Context, orgID uint) (any, error)`<br/>Exports mailboxes to Model Context Protocol clients. |
| `plugin.MCPExportServiceName("emails")` (`"emails.mcp_export"`) | `plugin.MCPExporter` | `func(ctx context.Context, orgID uint) (any, error)`<br/>Exports emails to Model Context Protocol clients. |

### 5.2 Services Consumed

| Service / Dependency | Usage Context & Resolution Mechanism |
|---|---|
| `plugin.ServiceMailStorageProvider` (`"mail.storage_provider"`) | Resolved via `plugin.LookupAs[plugin.StorageProvider](ctx, ...)`. Implemented by Octarq Pro `modules/mailstorage` for remote S3/MinIO/R2 object storage. Defaults to OSS `DBStorageProvider`. |
| `idempotency.ServiceName` | Middleware attached to `POST /api/emails/send` to guarantee idempotent message dispatch. |
| `links.Link` & `plugin.LinkCreator` | Scans outbound emails for HTTP(S) links and creates short links under the workspace's link domain. |
| `dns.Domain` | Evaluates email domains (`for_mail = true`) and verifies `EffectiveMailHosts()` in catch-all resolution. |
| `plugin.Context` core services | `ctx.DB`, `ctx.OrgID`, `ctx.Audit`, `ctx.Encrypt` / `ctx.Decrypt`, `ctx.GetWorkspaceSetting`, `ctx.GetGlobalSetting`, `ctx.Notify`, `ctx.PublishEvent`, `ctx.RecordUsage`, `ctx.RequireRole`, `ctx.RegisterWebhookEvent`, `ctx.RegisterTenantView`. |

---

## 6. Invariant Guarantees & Fail-Closed Behaviors

1. **Zero Email Loss Invariant**:
   - Inbound MIME parsing errors never drop emails. Malformed headers, boundary defects, or syntax violations fall back to minimal records with Subject `"(unparseable message)"`.
   - If the active storage provider (e.g. S3) fails during `Put`, the system logs the error and falls back immediately to saving the raw bytes directly into the `emails.raw` column.
2. **Fail-Closed Dial-Time SSRF Guard**:
   - All outbound SMTP socket creations pass through `safehttp.SMTPControl`. Destination IP addresses are verified at dial time. Any connection resolving to loopback, private LAN (RFC 1918), link-local, multicast, IPv6 ULA, or cloud metadata endpoints (`169.254.169.254`) is immediately aborted before the TCP handshake.
3. **Fail-Closed SMTP Authentication Guard**:
   - If an SMTP sender has credentials configured but the remote server does not advertise the `AUTH` capability, `internal/mail/send.go:100` returns an error (`smtp: server doesn't support AUTH`) and refuses to send. Delivery never silently degrades to unauthenticated transmission.
4. **Cross-Tenant Squatting Guard**:
   - `createMailbox` enforces `mailAddressDomainNotAnotherTenants`: no tenant may claim a mailbox address whose domain is registered by another workspace.
   - Inbound catch-all resolution strictly enforces `ownsMailHost`: dynamic mailboxes are only created if the recipient domain is owned and enabled by the receiving organization.
5. **Strict Multi-Tenant Suppression Isolation**:
   - Suppressions are indexed and queried strictly by `(owner_id, address)`. A hard bounce in Organization A suppresses outbound delivery from Organization A only; Organization B remains completely unconstrained.
6. **Soft Bounce Protection Invariant**:
   - Transient bounces (`bounceType: Transient`, `Undetermined`, or temporary SMTP 4xx codes) generate audit entries and notifications but are NEVER inserted into `mail_suppressions`.
7. **Unwired Seam Security Invariant**:
   - If `ctx.RequireRole` is nil, `hasRole` returns `false` (fail-closed refusal). Administrative routes (`createMailbox`, `updateMailbox`, `deleteMailbox`, `createSMTPSender`, `deleteEmail`, `createSuppression`, etc.) strictly reject calls when authorization cannot be verified.
8. **Constant-Time Webhook Token Authentication**:
   - Public webhook endpoints (`inbound`, `inboundGeneric`, `emailBounceWebhook`) verify tokens via `subtle.ConstantTimeCompare`. Mismatched tokens log `recordInboundAuthFailure` without recording or leaking the attempted credential.
