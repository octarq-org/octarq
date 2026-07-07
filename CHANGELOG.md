# Changelog

All notable changes to this project are documented here.

## [Unreleased]

### 🔒 Security

- **security**: Store only the SHA-256 hash of session tokens
- **security**: Ignore X-Forwarded-For unless LED_TRUST_PROXY is set
- **security**: Enforce LED_SECRET_KEY minimum length
- **security**: Constant-time secret comparisons and escape password-gate path
- **security**: Prevent SMTP header injection and bound inbound-mail parsing
- **security**: Scope abuse reports and their notifications per org
- **security**: Scope inbound-mail notifications to the mailbox's org
- **security**: Require owner/admin role to update instance settings
- **security**: Enforce org ownership on link analytics, QR, and provider accounts
- **security**: Scope DNS records provider to owner_id to prevent cross-tenant IDOR
- **security**: Harden P2 bounce webhook and stop leaking the 2FA secret
- Add request body size limit, move idParam helper, and enforce HTTP security headers
- **security**: Secure attribute on the session cookie
- **security**: Force SMTP From + rate-limit outbound mail
- **security**: SSRF guard on server-side URL fetches (title preview)

### 🚀 Features

- **billing**: Show org-scoped claim success URL in settings
- **finance**: Surface pending OCR transactions with confirm action
- Migrate Redis task queue backend to hibiken/asynq
- Integrate optional Redis cache & task queue with memory/DB fallbacks
- Migrate to stateful DB-backed sessions with per-session revocation
- Active session listing and per-session revoke
- **web**: Co-locate and embed settings directly inside Links, Mail, AI Inbox, and DNS feature views
- **web**: Re-architect sidebar layout and settings panel for solopreneur and self-media use cases
- **web**: Remove user-customizable sidebar menu and support automatic grouping based on plugin category
- **app**: Warn at startup when secure cookies are on but base URL is not https
- **compliance**: Add CSP headers, API versioning rewrite, email bounce webhook, audit logging, and EU cookie consent
- **auth**: Operator 2FA (TOTP), session revocation, and invite emails
- **server**: Security headers, global rate limiting, metrics, request IDs
- **app**: Add Notify method to compositions root App struct
- Implement OpenAPI specification generator and Makefile target
- **webhook**: Unify inbound email under /api/webhook/{orgSlug}/... with per-org token
- **plugin**: Add SendMail seam for plugin transactional email
- **frontend**: Implement onboarding, GDPR danger zone, DNS status, and accept invite pages
- **frontend**: Declare API client endpoints and route mapping for user activation
- **backend**: Add /api/health database ping check and tests
- **backend**: Implement SPF/DKIM/DMARC DNS record lookup and health verification
- **backend**: Implement invited-member password activation flow
- Support client portal embedding and dual Vite build pipeline
- **account**: Data export + account purge (GDPR/CCPA portability)
- **crypto**: Envelope encryption (DEK/KEK) for painless key rotation
- **web**: Gate Finance page behind 402 LockedFeature
- **storefront**: Add Licenses, Storefront, Billing, and refactor VPS/SSH pages with upsell gating
- **settings**: Refactor settings pages, webhooks UI, and add LLM Providers registry
- **api**: Define API types and client methods for license, storefront, billing, and LLM providers
- **ui**: Add LockedFeature component and update ProPill
- Implement Webhook Event Bus in core, add autoWrapLinks outbound email tracking, and build webhooks management settings UI
- **mcp**: Close the DNS write loop; fix plugin deps in mcp mode
- Implement dynamic MCP tool registration, refactor SQL guardrails, and update frontend Finance page to use real Transaction APIs
- Implement AI roadmap P5 foundation (multi-provider support, MCP query DB, email hook/classification/OTP, AI audit logs)
- Support 2-level sidebar customization, expand static Assets with storage and databases, and add flow filter to Finance page
- Support recurring income and editing individual occurrences of recurring transactions
- Merge subscriptions and ledger transactions into a unified FinanceWorkspace layout with cycle filters
- Connect SaaS subscriptions and transaction ledger data in FinanceWorkspace
- Align settings inner layout with general pages sidebar (AreaPanel), resolve form alignment, implement closed-loop finance ledger, and add React body portal to Modal
- Support workspace rename, integrate Billing & Plan demo, and optimize sidebar layout groups
- Migrate to magicpatterns design system with aurora gradient, glassmorphism components, and double-sidebar layout
- Implement multi-tenant switcher, dynamic plugin menus, and personal settings redesign
- **plugin**: 在 plugin.Context 中提供 Audit 审计日志记录方法
- 补全所有模型的 audit 覆盖 (link/domain update, smtp/sshkey/vps/notify 全 CRUD)
- Complete goal items (auth, audit, abuse, rate limits)
- **audit**: AuditLog — actor/action/target/meta trail for sensitive operations
- **email**: SPF/DKIM/DMARC authentication result badges
- **abuse**: Public /abuse report endpoint + admin list/update
- **retention**: Data retention cron — auto-purge old click events
- **privacy**: Anonymize IP before storing LinkEvent (GDPR/CCPA)
- **analytics**: Bot traffic toggle — retain events, filter by tag
- **multi-tenant**: Org-scoped data isolation, OAuth login, security tests
- **finance**: Add Finance UI — subscription list, summary cards, CRUD modal

### 🐛 Bug Fixes

- **race**: Guard InMemoryQueue.handlers with a mutex
- Unconditionally delete legacy empty-UA sessions on startup
- SwitchOrg uses SetSessionFromRequest to retain IP/UA and deduplicate sessions; add periodic session cleanup
- Add name attributes to login inputs so Enter triggers native form submit
- Explicit onKeyDown Enter handler on all login inputs
- Show only location name in sessions list, fall back to IP
- Login Enter submit, deduplicate sessions by IP+UA fingerprint, hide IP for Localhost
- Resolve IPv6 loopback brackets in active sessions location mapping
- Rename Session table name to user_sessions to avoid conflict with customer.Session
- Resolve SQLite no such column sessions.id migration limitation
- **web**: Unify sidebar fallback names, fix search placeholder overlap, and clean up duplicate plus symbol in mailbox action
- **ratelimit**: Normalize /api/v1/ prefix before tier classification; add API tests
- **mail**: Handle AWS SNS wrapped SES payloads and auto-confirm subscription
- **web**: Resolve pre-existing type errors so the frontend builds
- **openapi**: Make output path relative and portable
- **web**: Remove legacy duplicate Telegram Alerts settings and align sidebar menu item selector width via grid
- **tenant**: Return empty array instead of null for members and add fallback to prevent page crash
- **web**: Increase workspace switcher popup opacity for readability
- **web**: Align logo box center when expanded
- **web**: Apply navigation sidebar layout fix and build assets
- **tenant**: Enforce role checks on org member management
- Improve menu customizer layout parsing safety in App and PersonalSettings
- 移除 Abuse.tsx 中不存在的 Button import，改用原生 button
- BootstrapOrgID 按 slug 精确查找 admin org，防止 OAuth 用户先登录导致 admin org 错误绑定

### ♻️ Refactor

- Address UI and copy tweaks, unify labels, resolve locations in sessions
- Replace custom DNS HTTP clients with official Cloudflare and Tencent Cloud Go SDKs
- Replace custom telegram notify and rate limiting with nikoksr/notify and ulule/limiter
- **web**: Restructure settings — global settings trimmed to 3 groups, business settings as module tabs
- **web**: Plain-language section descriptions across settings
- **web**: Restore "Danger Zone" heading (a standard destructive-action pattern)
- **web**: Plain-language copy for data, privacy, and security settings
- **core**: 将 VPS 和 SSH key 迁移至 pro plugins，并在前端支持优雅降级/未激活状态

### 📚 Documentation

- **mail**: Update inbound email webhook URL to use the versioned /api/v1 prefix in docs and settings
- **web**: Remove legally unnecessary cookie consent component and rebuild frontend assets
- **web**: Make dashboard menu and page headers more concise
- **web**: Rewrite dashboard copy with benefit-oriented language and industry-standard terms
- GeoIP sourcing + Docker/k8s deploy, Pro-page note, P5 roadmap
- 修正 README 中关于 bootstrap org 的错误描述

### 🧪 Testing

- **api**: Cover tokens CRUD, domain sync, and CRUD error branches
- **api**: Cover event-webhook CRUD and notification-channel test send
- **api**: Cover read endpoints, link stats/QR, and DNS records CRUD
- **backend**: Integrate API routing and add tests for invitation flow and DNS verification
- Add eventbus delivery and signature tests, and mail outbound link wrapping parser tests
- Add comprehensive tests and fix code issues found in code review

### ♿ Accessibility & Privacy

- Add htmlFor/id to login form labels and inputs
- Mask IP server-side before JSON response; raw IP never leaves server
- Hide IP from sessions/portal devices; mask IP fallback when no geo location

### ⚙️ CI & Build

- **frontend**: Recompile bundle with server-side 2FA QR
- **frontend**: Compile production bundle and update embedded assets
- **frontend**: Compile production bundle and update embedded assets
- Add vulnerability scanning job and vulncheck target

### 🧹 Chores

- **dist**: Rebuild webembed after settings refactor
- Add CLAUDE.md dev conventions and bind dev servers to --host
- Unify all pages styling using ui.tsx design system components (ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard)
- Group default navigation menus into 4 categories from Scheme 1
- Apply default navigation grouping categories based on Scheme 1

## [0.1.1] - 2026-06-26

### 🐛 Bug Fixes

- Correct module path to github.com/octarq-org/led

## [0.1.0] - 2026-06-26

### 🚀 Features

- **plugin**: Expose Notify hook and Starter interface in plugin.Context
- Add vps and ssh keys management with web terminal
- **ui**: Integrate new APIs, settings, and pagination
- Migrate settings to DB and add API pagination
- Implement database-backed notification channels
- Add advanced shortlink routing rules and bot detection
- Support configuring multiple SMTP senders
- Change settings to a multi-page layout with a sub-menu
- Normalize DNS providers to ProviderAccount model
- Mount dashboard under /admin; runtime settings (reserved slugs/mailboxes, CF token)
- Overview dashboard with charts + DNS/Cloudflare setup guides
- Multiple link/mail hosts per domain (subdomains)
- **links**: Per-domain short-link host (subdomain), expressed end-to-end
- Cloudflare zone sync, service-aware pickers, deeper link/mail/dns
- **p4**: API tokens, DNSPod provider, Telegram notify, unit tests
- Led MVP — self-hosted link/email/domain service (P0–P3)

### 🐛 Bug Fixes

- **analytics**: Add region support and fix empty stats crash
- **dns**: Surface provider errors as 400 (not 502); add MX priority; clearer host UI
- **geo**: Load mmdb via FromBytes to avoid mmap ENODEV on mounted volumes
- **db**: Drop sqlite-only blob type on Email.Raw so Postgres migrates

### ♻️ Refactor

- Plugin architecture with deferred AutoMigrate (Core-as-Library)
- **domains**: Drop the link/mail toggles — host lists are always visible

### 📚 Documentation

- Prepare documentation for open source release and clean up deprecated env vars
- **compose**: Clearer GeoIP mount (absolute host path, /geoip target, must exist)
- **compose**: Document GeoIP mmdb mount; make DB driver/DSN env-overridable

### ⚙️ CI & Build

- Pin pnpm version in action-setup (packageManager lives in web/, not root)
- Use pnpm 9 to avoid the pnpm-10+ ignored-builds gate
- GitHub Actions for test/lint and multi-arch image publish
- Verifiable Docker images; binary-only scratch image

### 🧹 Chores

- Gofmt -w (fix CI gofmt check)
- Update web build artifacts
- Optimize UI and interactions according to modern web guidance
- Drop LED_BASE_URL, make docker host port configurable, trim README


