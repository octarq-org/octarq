# Changelog

All notable changes to this project are documented here.

## [0.3.0] - 2026-07-20

### 🚀 Features

- **links**: Expose a links.create service for plugins (#30)
- **build**: Xcaddy-style plugin composition (make plugin-build) (#26)

### 🐛 Bug Fixes

- **build**: Compose plugins as &Plugin{} so stateful/MCP plugins work (#29)

### 📚 Documentation

- Sync README_ZH with the reframed English README (#28)
- Reframe README as a plugin framework for one-person companies (#27)
- Reframe plugin system as community-first (Pro is just one plugin set) (#25)

## [0.2.0] - 2026-07-20

### 🔒 Security

- **security**: Instance-admin flag, public-route metadata, redirect/MCP hardening
- SSRF-guard outbound webhook and notification delivery
- Port missing hardening from security-hardening to main
- **security**: Restrict instance-level settings to instance admin
- **security**: Bump pgx to v5.9.2 (GO-2026-5004 SQL injection)
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

- **web**: Visual polish pass — brand-gradient actions, glass depth, contrast & tactility (#18)
- **webhooks**: Generic plugin-extensible event registry with grouped picker UI (#14)
- **plugin**: Enrich plugin descriptions, declare menu order, reactive toggle, and member status (#13)
- **plugin**: EnabledByDefault (opt-out features); hello example on by default (#12)
- **web**: Advisory role gating — requiredRole on routes/menus, 403 AccessDenied
- **geo**: Auto-download GeoLite2-City with OCTARQ_MAXMIND_LICENSE_KEY
- **plugin**: Inter-plugin service registry (Provide/Lookup) on Context
- **web**: Plugin widgets via ExtensionSlot, ProGate route boundary, data-driven areas
- **app**: Preflight table-collision check before the delayed AutoMigrate
- **web**: Resolve @xterm/* in dev-from-source via web devDependencies
- **web**: OCTARQ_DEV_ALIASES to resolve an edition's plugin deps in dev-from-source
- **web**: Support composing plugins from external source for dev-from-source HMR
- **web**: Install manifest plugins at build time; manifest is the source of truth
- **web**: Compose @octarq-org/plugin-infra; drop local vps/ssh-keys pages
- Migrate API to Huma OpenAPI schema (#3)
- **web**: Manifest-driven plugin composition + editions
- **plugin-sdk**: Self-contained i18n, brand, and locked-state UI
- **plugin-sdk**: Back Field with Base UI + a11y fixes
- **app**: Add WithWebFS to override the embedded dashboard
- **web**: Theme tokens, tw-animate-css wiring, a11y audit
- **plugin-sdk**: Extract @led/plugin-sdk workspace package
- **web**: Back shared UI primitives with shadcn/Base UI
- **web**: Wire app to the plugin registry; move licenses page into a UIPlugin
- **web**: Add frontend plugin SDK contract, registry, and injection seam
- **openapi**: Generate openapi spec at build time via subcommand
- Split instance settings and workspace settings in API and frontend
- **ai**: Single-step AI assists — slug suggestions + on-demand email summary
- **web**: Gate workspace switcher to Pro + i18n shell (en/zh, auto-detect)
- **web**: Top area switcher + ⌘K command palette
- **web**: Collapsible second-level area panel
- **dns**: Drop global Cloudflare token, sync onboarding, link-host verify
- **dns**: Verify SPF/DKIM/DMARC per mail host, fix always-green status
- Per-workspace plugin management (opt-in, routes + menus gated)
- Env-driven app name (LED_APP_NAME) and unified Pro feature mask
- Self-serve registration, unified Pro-lock UI, device fingerprint dedup
- Optimize overview layout, unify feature gates, update billing page, separate export workspace data, and refactor customer portal forms
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

### 📦 Other

- Base UI CommandPalette + migrate all native selects; SDK toast changeset
- Toast system, in-app workspace switch, Base UI menus, mobile, a11y
- **shell**: Smooth collapsing rail + stable scroll gutter
- Translate login, coming-soon, and workspace shell strings
- Translate all remaining pages + shared ui (en/zh)
- Wire Domains + Mail pages, stage 8 more page namespaces

### 🐛 Bug Fixes

- **members**: Only an unredeemed invite marks a member pending (#16)
- **web**: Fix plugin toggle switch state and styling compilation
- **web**: Self-heal stale .manifest-bak snapshots left by a killed install
- **web**: Add -w so pnpm add targets the web workspace root
- **web**: Correct pnpm add flags and fail loudly on plugin install errors
- **web**: Plugin-infra published as 0.2.0, widen optional dep range
- **docker**: Copy example plugin workspace member into web build
- **sdk**: Remove nested pnpm-workspace/lock from plugin-sdk
- **test**: Correct stale Jungley8/led import in safehttp_test
- Address security compliance issues (P0/P1)
- **examples**: Point plugin-hello at the octarq-org/led module path
- **api**: Fix missing closing braces in API handlers
- **settings**: Fix wrong confirmation dialog translation key for clearing metrics token
- **i18n, audit**: Unify abuse nav labels and record redacted metadata for updates
- **web**: Guard verify-dns render against missing hosts array
- **web**: Stop seeding mock transactions into the Finance DB
- **web**: Hide empty sidebar areas when a feature is disabled
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

### ⚡ Performance

- Route-level code splitting + vendor chunks (#6)

### ♻️ Refactor

- **core**: UIArea.groups seam; drop Commerce shell + VPS/issued api dead weight (#20)
- **core**: Decouple Overview, portal, commercial api & Pro nav from OSS core (#19)
- **plugins**: Move feature menus, settings pages, and descriptions out of the core (#17)
- **core**: Remove Pro/commercial surface, backend-driven menus, compose hello demo (#11)
- **core**: Unify Core plugin composition with Pro (opt-in, drop build tags) (#10)
- **core**: Plugin composability — deps contract, build-tag exclusion, frontend self-containment (Phase 3) (#9)
- **core**: Move models, redirect engine, and mail webhooks into plugins (Phase 2) (#8)
- **core**: Extract links, mail, and dns into core plugins (Phase 1) (#7)
- Rename led_ wire formats to octarq branding (clean cut)
- **web**: Demote core business pages to UIPlugins; shell owns no business routes
- **web**: Audit is a core page, not a Pro plugin
- **web**: Consume @led/plugin-sdk, inverting the UI dependency
- Remove LED_SECRET_KEY_OLD and old master key rotation logic
- Split org-level settings into workspace_settings table
- **config**: Migrate runtime-tunable env keys to DB settings
- **web**: Split ui.tsx into ui/ modules behind a barrel
- **web**: Split Mail, Finance, and Links pages into modules
- **web**: Split Domains.tsx into pages/domains/ modules
- **web**: Split Settings.tsx into pages/settings/ modules
- **web**: Split App.tsx shell into src/shell/ modules
- **plugins**: Group features + core plumbing; license-independent menus
- **config**: Make default app name a build-time overridable var
- Address UI and copy tweaks, unify labels, resolve locations in sessions
- Replace custom DNS HTTP clients with official Cloudflare and Tencent Cloud Go SDKs
- Replace custom telegram notify and rate limiting with nikoksr/notify and ulule/limiter
- **web**: Restructure settings — global settings trimmed to 3 groups, business settings as module tabs
- **web**: Plain-language section descriptions across settings
- **web**: Restore "Danger Zone" heading (a standard destructive-action pattern)
- **web**: Plain-language copy for data, privacy, and security settings
- **core**: 将 VPS 和 SSH key 迁移至 pro plugins，并在前端支持优雅降级/未激活状态

### 📚 Documentation

- Update README, add Chinese translation, Mermaid architecture diagram and OSS credits
- Fix stale plugin refs (ProGate→PluginGate, dropped vite.portal.config.ts) (#23)
- Stale-logic audit — align comments, docs and examples with the plugin architecture
- Plugin architecture + commercialization design & status handoff
- Add plugin development guide and CONTRIBUTING
- **examples**: Add plugin-hello, a full-stack community plugin template
- **mail**: Update inbound email webhook URL to use the versioned /api/v1 prefix in docs and settings
- **web**: Remove legally unnecessary cookie consent component and rebuild frontend assets
- **web**: Make dashboard menu and page headers more concise
- **web**: Rewrite dashboard copy with benefit-oriented language and industry-standard terms
- GeoIP sourcing + Docker/k8s deploy, Pro-page note, P5 roadmap
- 修正 README 中关于 bootstrap org 的错误描述

### 🧪 Testing

- **plugin-sdk**: Vitest coverage for the plugin registry and ExtensionSlot
- Update settings api tests and openapi-gen schemas for settings split
- **api**: Cover tokens CRUD, domain sync, and CRUD error branches
- **api**: Cover event-webhook CRUD and notification-channel test send
- **api**: Cover read endpoints, link stats/QR, and DNS records CRUD
- **backend**: Integrate API routing and add tests for invitation flow and DNS verification
- Add eventbus delivery and signature tests, and mail outbound link wrapping parser tests
- Add comprehensive tests and fix code issues found in code review

### ♿ Accessibility & Privacy

- Raise the faintest muted text to the AA contrast floor
- Reduced-motion, keyboard-operable Code, Guide aria-expanded
- Add htmlFor/id to login form labels and inputs
- Mask IP server-side before JSON response; raw IP never leaves server
- Hide IP from sessions/portal devices; mask IP fallback when no geo location

### ⚙️ CI & Build

- **sdk**: Enable changesets release on push to main
- Fix Docker web build for the plugin-sdk workspace; gate SDK publish
- **web**: Make dashboard outDir overridable via OCTARQ_WEBEMBED_OUT
- **sdk**: Add changesets + GitHub Packages publish pipeline for @led/plugin-sdk
- Auto-generate changelog and release notes via git-cliff
- **frontend**: Recompile bundle with server-side 2FA QR
- **frontend**: Compile production bundle and update embedded assets
- **frontend**: Compile production bundle and update embedded assets
- Add vulnerability scanning job and vulncheck target

### 🧹 Chores

- **web**: Refresh embedded dashboard build [auto]
- **core**: Finish the commercial api.ts sweep; mark audit items done (#22)
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Drop ProGate compat aliases and commercial copy from OSS core; add decoupling audit doc (#15)
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **sdk**: Tailwind v4 data-attribute variant syntax in Switch/Tabs
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Consume Pro UI plugins as published packages, drop local copies
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **sdk**: Add initial-release changeset for @octarq-org/plugin-sdk
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- Fix stray led -> octarq in ci.yml auto-commit comment
- **web**: Refresh embedded dashboard build [auto]
- Gofmt files touched by recent commits (CI gofmt gate)
- Update translations, minor fixes, and test updates
- **rebrand**: Fix ghcr owner to octarq-org and rebuild embedded dashboard
- **rebrand**: Rename led -> octarq across the repo
- Gofmt import ordering after octarq-org module migration
- **sdk**: Add publish fields to @led/plugin-sdk + root lockfile
- **examples**: Tidy plugin-hello go.mod/go.sum after rebase
- **migration**: Migrate repository and module paths to octarq-org
- **web**: Rebuild embedded bundle for grouped plugin manager
- Gofmt
- **dist**: Rebuild webembed after settings refactor
- Add CLAUDE.md dev conventions and bind dev servers to --host
- Unify all pages styling using ui.tsx design system components (ScreenWrap, PageHeader, GlassCard, Badge, Button, StatCard)
- Group default navigation menus into 4 categories from Scheme 1
- Apply default navigation grouping categories based on Scheme 1

## [0.1.1] - 2026-06-26

### 🐛 Bug Fixes

- Correct module path to github.com/Jungley8/led

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


