# Changelog

All notable changes to this project are documented here.

## [0.4.1] - 2026-08-29

### 🚀 Features

- **web**: Add post-login 12-screen onboarding questionnaire flow
- **web**: Add AI agent MCP connection card and polish onboarding DX (#468)
- **settings**: Add UI for public CORS origins, system SMTP sender, and reserved slugs (#464)
- **eventbus**: Add declarative EventReactor registry and worker pool (#436)
- **web**: Add CommandPalette chat mode, thinking chain collapsible, and A2UI container (#434)
- **harness,api**: Add streaming consumption and SSE chat stream endpoint (#433)
- **tenantsql**: Implement tenant view layer engine and core views (#431)
- **llmprovider**: Add StreamProvider interface and streaming support (#430)
- **webhooks,mail**: Test-verification endpoints for webhook endpoints and SMTP senders (#429)
- **tenantsql**: Add secure tenant SQL validation layer (#428)
- **plugin**: Add RiskLevel and RequireApproval to EndpointSpec with MCP metadata (#426)
- **eventbus**: Add in-process event spine — Envelope, Subscribe, backpressure (#425)
- **harness**: Add micro-kernel skeleton (types, profile, runner, guard, tracer) (#424)

### 📦 Other

- Consolidated batch (14 PRs) — perf, security & code health (#460)

### 🐛 Bug Fixes

- **e2e**: Only fallback-dismiss onboarding when UI was present (#471)
- **e2e**: Make onboarding dismiss robust via storage fallback
- **e2e**: Auto-dismiss post-login onboarding in test helpers
- **links**: Prioritize title over slug, split tags into badges, and include title/tags in topLinks (#470)
- **ci**: Resolve cloudflare account id dynamically if secret is unset in cleanup steps
- **ci**: Use dynamic deployment URL for preview comments (#465)
- **harness,api**: Halt on ErrApprovalRequired, ignore client system prompt, enforce read-only web chat tools (#442)
- **auth,eventbus**: Implement HTTP RequireRole check and non-blocking reactor dispatcher (#441)
- **tenantsql**: Wrap view materialization with redacted values to prevent alias bypass (#440)
- **core**: Resolve recent review findings (concurrency, cache, postgres tx, tracer) (#438)
- **lint**: Address QF1003 and QF1008 staticcheck findings (#437)
- **ci**: Remove go.mod cache-bust comment that broke workflow parsing (#432)
- **db**: Silence record-not-found SQL logs in production; drop stale landing-page MCP tag (#421)
- **release**: Set up buildx containerd builder before goreleaser (#420)

### ♻️ Refactor

- **web**: Migrate onboarding to Pro plugin hook (5-screen) (#472)

### 📚 Documentation

- **website**: Publish pre-launch checklist, un-orphan backup-restore (#422)

### ⚙️ CI & Build

- **website**: Remove old deployments cleanup step
- **website**: Loop-delete all pages of old deployments
- **website**: Avoid # in wrangler --message to prevent shell comment truncation
- **website**: Remove redundant cleanup-preview job and delete trigger
- **website**: Switch preview deploy to wrangler versions upload (#467)
- **website**: Cleanup preview on branch delete (#463)
- **release**: Keep latest/major.minor image tags on stable releases only (#419)
- **release**: Replace hand-rolled release jobs with goreleaser (#418)

### 🧹 Chores

- **web**: Refresh embedded dashboard build [auto] (#469)
- **web**: Refresh embedded dashboard build [auto] (#466)
- **web**: Refresh embedded dashboard build [auto] (#461)
- **web**: Refresh embedded dashboard build [auto] (#435)
- Remove stale research working notes from repo root (#423)

## [0.4.0] - 2026-08-24

### 🔒 Security

- **security**: Suppress internal error leaks, wire trust proxy, and add mail idempotency (#371)
- **security**: Hash invite tokens, rate-limit the completion endpoints, audit sensitive actions (#324)
- **security**: Verify DNS provider-account ownership before usage probe (#66)
- **security**: Harden MCP auth against org-0 fail-open + cross-org regression tests (#65)

### 🚀 Features

- **oct-1**: Architecture evolution — 6 borrowings (contract/EventBus/cache/CLI/config) (#417)
- **config**: Export LoadDotEnv for env loading before app.New (#403)
- **web**: Refactor DNS, links, and mail detail layouts for streamlined UX (#401)
- **web, settings**: Unify toasts, format buildinfo short sha, and add sharedHosts settings (#398)
- **sdk**: Integrate sonner as app-wide toast system and format validation errors (#396)
- **settings**: Support configuring sharedHosts in instance settings and origin resolver (#395)
- **plugin,endpoint**: Add unified declarative dual endpoints (HTTP + MCP) (#388)
- **plugin,cache**: Add AgentError and ScopedCache for plugin isolation and agent guidance (#386)
- **telemetry**: Integrate OpenTelemetry distributed tracing and metrics (#385)
- **website**: Add X profile link to landing footer (#379)
- **db**: Add MySQL 8 driver support, backup/restore, and documentation (#377)
- **docs**: Generate openapi at build time and bind website deploy to release tags (#372)
- **eventbus,app**: Make outbound webhooks replay-safe and reliably delivered (#368)
- **web**: Carry the workspace brand colour into the favicon and theme-color (#346)
- **plugin**: Notify member-removal hooks when a member is removed (#343)
- **web**: Broaden i18n audit to copy attributes and JSX expr literals (#341)
- Instance-scope plugin routes and menus (InstanceMenuProvider + instanceRoutes) (#332)
- **instance**: Standalone operator console with a first-run setup wizard (#321)
- **plugin-sdk**: Add UIPlugin.replaces — the enhanced-edition replacement seam (#317)
- **instance**: System mail sender, readiness API, instance-admin gating (#316)
- **api**: Authenticated instance build info endpoint (#274)
- **cors**: Allowlist-based CORS for public GET endpoints (#263)
- **tenant**: Auto-provision each org's <slug>.<base> subdomain (#261)
- **auth**: Fix signup entry — /signup & /login routes, email verification on by default, optional orgName (#258)
- **origin**: Declare instance-wide shared hosts so tenants on a shared entry host get an origin (#257)
- **website**: Plausible analytics + cross-host campaign forwarding (#249)
- **website**: Give the API explorer the whole screen (#248)
- **website**: Destinations on the left, tools on the right (#247)
- **website**: Make the API explorer actually render, and stop calling a CDN (#243)
- **links**: Stop counting clicks once a free org's monthly quota is up — never stop the redirect (#234)
- **plugin**: Add a quota-checker seam for the Cloud build to enforce (#233)
- **website**: Rebuild the landing page on the Nocturne design, with motion per section (#232)
- **security**: Guard every cookie-authed write, and add a CSRF token (#230)
- **account**: Self-serve account deletion (GDPR) (#217)
- **settings**: Add settings-workspace extension slot on the workspace settings page (#211)
- **origin**: Add OwnerOf to resolve host ownership by org (#206)
- **config**: Validate OCTARQ_BASE_URL and report startup readiness (#199)
- **web**: Open a topbar-right extension slot (#196)
- Reserve retired org slugs in OrgSlugHistory to prevent squatting (#194)
- **org**: Let an owner choose the workspace address (#188)
- **auth**: Key external logins on (issuer, subject), not on email (#187)
- **org**: Allocate org slugs at random through one entry point (#186)
- Handle plugin disabled state (#184)
- **mail**: Suppress hard bounces and complaints with tenant isolation (#178)
- **links**: Add UTM tracking, referrer channel classification, and PV/UV analytics depth (#174)
- **links**: Weighted A/B split routing with sticky assignment (#172)
- **coreschema**: Expose the core schema so plugins can check their mirrors (#171)
- **website**: Serve the site on octarq.org instead of docs.octarq.org (#170)
- **mail**: Route raw originals through a storage seam (#165)
- **links**: Add click pipeline batching, rate limiting, and quick create link endpoint (#164)
- **mail**: Add provider-agnostic generic inbound email webhook (#163)
- **help**: Align help categories with navigation areas and rename docs (#161)
- **plugin**: Permission seam so Pro RBAC can reach core resources (#160)
- **web**: Persist filter conditions in URL query for Links and DNS records (#158)
- **ui**: Table density preference, and four hand-rolled tables onto the shared primitives (#157)
- **ui**: Global create in the top bar, actions in the command palette (#153)
- **plugin**: ActionProvider contract and GET /api/actions (#152)
- **website**: Build a real landing page from the pitch nobody could read (#129)
- **app**: Refuse to start when two plugins share a name (#127)
- **auth**: Unify account identity on email, add self-service email change (#115)
- **docs**: Add interactive OpenAPI API Reference page
- **tokens**: Let an API token's scope be changed without rotating its secret (#105)
- **auth**: Make API tokens act as their holder instead of as the workspace (#101)
- **login**: Render the errors that redirects have always been sending (#100)
- **auth**: Scope API bearer tokens to a role (P2-18) (#93)
- **authz**: Give member a role that cannot delete the workspace's data (#91)
- **notify**: Encrypt notification channel configs at rest (#89)
- **core**: Make LLM resolver org-aware with SetLLMResolverForOrg (#85)
- **usage**: Add RecordUsage context seam and wiring for metered consumption (#84)
- **api**: Make account purge and export actually complete (#82)
- **auth**: Revoke sessions on member removal, add per-org branding + authz hooks (#80)
- **notify**: Unify alert channels into one provider list (#77)
- **help**: Make in-app docs a core capability plugins contribute to (#75)
- **brand**: Adopt the Keystone Arch mark (#74)
- **plugins**: Separate instance registration from workspace enablement, guard dependencies (#70)
- **brand**: Give Octarq a real mark, and let white-label override it (#71)
- Extension seams (auth-method registry + 3 settings/login slots) (#62)
- **ui**: Semantic component layer (fixes light-mode readability) + color-lint guard (#59)
- **status**: Public status page + /api/status subsystem health (#58)
- **i18n**: Add Portuguese (pt) and Japanese (ja) locales (#57)
- **i18n**: Upgrade SDK i18n infrastructure to Partial<Resources> with English fallback and add Spanish (es) localization (#54)
- **onboarding**: Add setup checklist and dismissal persistence on Overview (#53)
- **db**: Add database backup and restore commands and admin backup endpoint (#52)
- **auth**: Add password reset flow and registration email verification (#51)
- **dns**: Add Dynamic DNS (DDNS) support with dyndns2 update protocol (#50)
- **notify**: Pluggable notification channel providers (#45)
- **brand**: Runtime white-label logo + accent colors (#44)
- **plugin**: LoginByEmail hook on plugin.Context (T7 core foundation) (#39)
- **cli**: `octarq plugin new <name>` scaffolds a plugin skeleton (T3) (#37)
- **config**: Zero-config boot — auto-generate & persist secret key + admin password (T2) (#35)

### 📦 Other

- **docs**: Remove editions comparison table from open source readmes (#382)
- Make the docs directory the convention, not a Go literal (#122)
- Bring the platform docs into the OSS build (#121)

### 🐛 Bug Fixes

- **auth**: Invalidate same-org sessions on member role change (#413)
- **links**: Isolate instance-scope link settings and routing boundaries (#411)
- **plugins**: Close gaps in mail storage cleanup, link security, and quota enforcement (#405)
- **dns**: Return 404 for missing domain records and backfill legacy owner_id (#404)
- **dns**: Preserve domain metadata on host updates and improve error handling (#400)
- **router**: Exempt instance administration and public routes from per-workspace plugin gate
- **i18n**: Handle hardening and secret-key detail translations in instance console (#394)
- **links**: Fix detail view horizontal overflow and adopt UI SDK tabs (#392)
- **i18n**: Localize instance console readiness check details (#391)
- **db**: Drop sqlite-only blob type tags on MailRawBlob and Record for Postgres compatibility (#387)
- **links,db**: Make SQL tag filter match tagsContain and cap SQLite to one conn (#383)
- **links,db**: Push tag filter down to SQL and configure SQLite WAL resilience (#380)
- **ui**: Harmonize StatCard delta colors and remove misleading positive/danger states (#378)
- **website**: Require Go toolchain for openapi generation with strict error exit (#373)
- **plugin**: Clamp over-limit pagination, collapse the duplicate DNS service name, and finish the named-contract pattern (#363)
- **auth**: Stop bearer tokens minting sessions, invite-token leak, SSO password reset, and webhook secrets in logs (#365)
- **openapi**: Generate the published spec from the live handler registrations (#362)
- **api**: Retire active workspace slug during account purge (#361)
- **mcp**: Remove raw-SQL tool that read across tenants (#357)
- **purge,mail,dns**: Propagate plugin purge errors, add cross-org tests, and fix slug exhaustion fallback (#360)
- **settings**: Report real join dates, and give display preferences a home (#359)
- **ui,api**: Remove duplicate plus in workspace menu and report unconfigured mail on verification resend (#347)
- **links**: Give each cloud tenant its own short-link host namespace (#344)
- **help**: Remove spinner animation from doc viewer loading state (#340)
- **console**: Consolidate instance console overview and wizard into single home page (#339)
- **auth**: Make token role cap a hard ceiling on the perm seam; revoke tokens on member removal (#338)
- **web**: Consolidate empty states and surface backend errors (#326)
- **ops**: Configurable log level, and a CSP that actually mitigates XSS (#325)
- **auth**: Require 2FA on OAuth callbacks, carry the challenge in a cookie (#318)
- **mcp**: Guard the mcp_export service contract (#315)
- **deps**: Upgrade react-router-dom to v7.18 (#310)
- **plugin-sdk**: Surface UIPlugin name collisions instead of dropping them (#309)
- **plugin**: Guard the remaining 5 cross-plugin service contracts (#300 follow-up) (#307)
- **web**: Close UX consistency gaps in loading, i18n and copy (#306)
- **auth**: Unblock sign-up and invites on mail-less instances (#304)
- **deps**: Bump go 1.25.13 and grpc v1.82.1 to clear govulncheck (#305)
- **release**: Correct ship-blocking docs, image tags and /data perms (#303)
- **plugin**: Guard cross-plugin service contracts at compile time (#300)
- **abuse**: Attribute reported slugs by host, not by slug alone (#298)
- **web,api**: Mobile OS detection in the session list, and simpler geo joining (#297)
- **links**: Resolve redirect hosts via origin, refuse contested hosts (#296)
- **web**: Move settings auth route under /instance (#294)
- **web**: Don't claim geo is unavailable when there are simply no clicks (#292)
- **web**: Re-read the brand when the workspace changes (#289)
- **web**: Make white-label branding reach the whole dashboard (#285)
- **sdk**: Single Badge implementation in SDK — restore children rendering, merge app styles (#280)
- **server**: Serve public status page at /status (#281)
- **api**: Stop minting a duplicate org per admin login and attribute login/register audits (#282)
- **colors**: Stop ui-color-ok markers from leaking into rendered text (#269)
- **cors**: Match concrete request paths against parameterized public routes (#265)
- **mail**: Align usage metric names with quota keys (#259)
- **website**: Reserve the header's full height on /api-reference/ to stop 1px page scroll (#256)
- **website**: Publish the current core spec, and keep it current (#253)
- **website**: Fold the landing subnav into the shared header (#242)
- **sdk**: Use brand css variables for primary colors and fix lint violations (#241)
- **openapi-gen**: Stop writing into a sibling private repo (#240)
- **website**: Serve this site on docs.octarq.org, and let the deploy see its token (#239)
- **website**: Restore the path to pricing, and say what actually costs money (#237)
- **admin**: Four places the back office sent people into a wall (#236)
- **api**: Give every huma response body its own schema name (#235)
- **web**: Distinguish plugin-disabled from plugin-unavailable on 404 (#223)
- **origin**: Namespace the resolver cache and wire domain invalidation (#224)
- GDPR, caching, cleanup hardening and legal pages (#222)
- **settings**: Mask the inbound webhook token (#219)
- **cleanup**: Prune audit logs by retention setting (#218)
- **mail**: Scope email handlers by org in the query itself (#216)
- Harden org scoping (abuse update, settings org guard, links event cleanup, session revocation on role change) (#215)
- **sdk**: Point locked-feature upsell at the license page (#213)
- **mail**: Delete raw eml blobs from storage provider on mail purge (#210)
- **i18n**: Update email auth provider description to reflect email registration (#209)
- **config**: A registered domain makes the secret-key floor fatal again (#207)
- **auth**: Don't hand out a session when sign-up needs email verification (#202)
- **api**: Stop swallowing write errors in the recovery flows (#197)
- **api**: Restore /api/v1/ as an alias, with its rate-limit normalization (#195)
- **auth**: An identity provider cannot hand out the owner role (#191)
- **mail**: Require admin role for createMailbox and updateMailbox (#183)
- **dns**: Require admin to change DNS records, and stop echoing provider errors (#182)
- **i18n**: Exempt the S3 region and bucket examples from the copy audit (#181)
- **mail**: Put the generic inbound token in the path, and audit rejections (#179)
- **mail**: Stop the MIME parser from silently dropping parts (#175)
- **api**: Inventory the endpoints reachable without a session, and stop one leaking in by prefix (#176)
- **panic-isolation**: Isolate plugin and worker panics with safego (#173)
- **i18n**: Translate untranslated area titles/nav items and extend i18n audit gate (#162)
- **web**: Setup checklist counted only half its steps, and one step our ICP can never finish (#154)
- **nav**: An area's title is display text, not a routing key (#148)
- **mobile**: Make admin pages mobile responsive and adapt dialog modal layout (#138)
- **help,account**: Repair in-app doc links, and make account settings self-service (#132)
- **i18n**: Shorten the sidebar footer links and stop two of them colliding (#128)
- **dev**: Make OCTARQ_PORT actually move the backend (#126)
- **help**: Key the docs cache by plugin, not by plugin name (#125)
- **shell**: Stop rendering Help twice in the sidebar footer (#123)
- **plugins**: List core plumbing in the plugin manager as locked-on (#118)
- **web**: Skip rendering widgets from disabled plugins in ExtensionSlot and defer mail setup checks (#114)
- **mail,notify**: Append config flag hint to SSRF guard errors
- **mail**: Guard outbound SMTP at dial time, not just at write time (#108)
- **dns**: Scope the DNSManager seam to the org it acts for (#107)
- **mail,links**: Stop one tenant's host config affecting another's (#106)
- Deliver mail notifications again, and erase a workspace's namespaced settings on purge (#104)
- Keep legacy DNS migration per-tenant, and gate audit/abuse reads at admin (#103)
- **links**: Enforce host ownership validation for short links (#102)
- **auth**: End an outstanding reset link when the password is changed (#99)
- **settings**: Gate inboundToken through callerHoldsRole (P2-10) (#94)
- **mcp**: Stop defaulting an unknown org to the bootstrap tenant (#90)
- **authz**: Fail closed when a request carries no org (#88)
- **brand**: Make the brand glyph a single source of truth in the SDK (#86)
- **mcp**: Stop one tenant's MCP connection from repointing every tenant's requests (#83)
- **auth**: Rate-limit password reset, and stop it starving the login budget (#81)
- **changeset**: Name the workspace package, not Pro's dependency alias (#78)
- **help**: Place Help in the footer rail, not a new nav group (#76)
- **i18n**: Translate sidebar group headings and plugin menu labels (#73)
- **auth**: Persist the active org when switch-org refreshes an existing session (#72)
- **ui**: Localize verify-email banner keys, center thin-banner alignment, un-cramp language switcher (#61)
- **ci**: Add tag guard to release docker job and concurrency group to ci.yml (#55)
- **web**: Alias published @octarq-org/plugin-sdk to the app SDK copy (#48)
- **deps**: Bump golang.org/x/text to v0.39.0 (GO-2026-5970) (#38)

### ⚡ Performance

- **origin**: Cache OwnerOf answers on the Resolver (#208)

### ♻️ Refactor

- **links**: Split plugin.go and stats.go to eliminate SIZE_OK exemptions (rebased onto 411) (#412)
- Split 6 oversized modules to meet 250 pure LOC limit (#409)
- Deslop for PR #405 — deduplicate helpers, name magic numbers, mark debt (#408)
- Modularize large files across core and plugins (#375)
- **safehttp**: Promote the SSRF guard to plugin/safehttp (#364)
- **links**: Scope reserved-slug settings to the instance console (#336)
- **web**: Flatten auth pages, strip decorative glows (#267)
- Promote origin to an importable package (#205)
- Derive absolute URLs from the request, drop three env vars (#204)
- **org**: Drop the email-derived slug warning (#192)
- **help**: Two-tier doc navigation with the category taxonomy owned by the backend (#116)
- **overview**: Move links/mail panels out of the shell into plugin slots (#69)
- **core**: Remove infra asset placeholders (moved to Pro infra plugin) (#64)
- De-led core Overview + copy audit (pure-core P2) (#63)

### 📚 Documentation

- **research**: Go frameworks for octarq — rebuild stages 1-2, add Stage 3 guard (#416)
- Remove License MIT badge and trim license description (#390)
- Add test coverage badge to readme (#389)
- Add editions comparison table and align open-core commercial funnel (#381)
- **readme**: Lead with the logo, a screenshot and the pitch (#358)
- Correct claims the code does not implement (#331)
- Retire repo-root docs/, consolidate onto two surfaces (#329)
- **env**: Classify every OCTARQ_* var and document backup/restore (#323)
- **mcp**: Stdio always runs as the bootstrap tenant (#322)
- **plugin**: Stop enumerating cross-repo providers in contract comments (#311)
- Add a self-hosted pre-launch checklist (#266)
- **website**: Track code copy button clicks with Plausible (#254)
- Add privacy policy links to 404 and api reference pages (#250)
- **plugins**: Settings pages belong under /settings/<menu id> (#229)
- Refresh stale documentation (account deletion, token masking, retention) (#221)
- Drop four refactor journals, repoint their references (#146)
- De-jargon the docs site, and fix a env var that never existed (#142)
- **help**: Rewrite help documentation to align with UI paths and remove AI tone (#141)
- Update stale plugin SDK registry and plugin repo references (#130)
- **app**: Separate the core-mount rule from the org-0 public-route rule (#120)
- **guides**: Add launch article 'Why I Rewrote a SaaS Stack as a Go Plugin Framework'
- Add OCTARQ_ALLOW_PRIVATE_SMTP documentation for self-hosted mail relays
- **ci**: Correct the stale claim that main is a protected branch (#98)
- **website**: Order the sidebar explicitly, put Quickstart second (#68)
- **website**: Replace Nimbus scaffold homepage with real Octarq landing
- Adopt Nimbus, replace Starlight docs site (#56)
- **website**: Purify open-source docs (remove pricing, buy CTAs, and sales pitch) (#47)
- **packages**: Add README.md for npm packages (#46)
- **website**: Add What is Octarq + Configuration to the Start section (#42)
- **website**: Migrate core feature docs into the public docs site (#41)
- **website**: Add Architecture + Guides sections (system & plugin design) (#40)
- **readme**: Embed agent-native demo GIF at the top of both READMEs (T1) (#34)
- Astro Starlight documentation site (T4) (#36)

### 🧪 Testing

- **mail**: Cover getAttachment/extractAttachment to restore 90% threshold (#407)
- **infra**: Raise statement coverage to >= 90% across infra packages (#356)
- **plugin,origin,server,config,models**: Raise coverage to 90% (#354)
- **app,auth,cmd**: Raise coverage to 90% (#352)
- Assert the outcomes the new coverage tests only called (#353)
- **dns,db,geo**: Raise coverage to 90% (#351)
- **links,mail**: Raise plugin coverage to 90% (#350)
- **api**: Raise coverage to 90% (#349)
- **api**: Lock tenant/instance scope boundary (#333)
- **web**: Block real network in vitest so dry mocks fail loudly, not by hanging (#301)
- **web**: Self-booting Playwright e2e harness, wired into CI (#271)
- **web**: Guard the core settings paths that plugin packages link to (#264)
- Raise coverage to 70.1% and ratchet CI against regressions (#166)
- **app,api**: Pin two implicit contracts (P3-21, P3-19) (#95)

### ⚙️ CI & Build

- Path-filter jobs, drop docs-surface, run e2e in parallel (#384)
- **deploy**: Isolate production deployment, add preview deployment, and update DB docs (#376)
- Raise the coverage floor from 69% to 90% (#355)
- Add production url to deploy-website environment (#255)
- Stop stale main pushes from opening an empty dist PR (#201)
- Deploy website/ to Cloudflare from GitHub Actions (#169)
- Refresh webembed/dist after the merge instead of on the PR branch (#117)
- Refresh webembed/dist on the PR branch instead of pushing to main (#92)
- Move govulncheck from per-PR to a weekly schedule (#67)

### 🧹 Chores

- **ecosystem**: Neutralize OSS docs for plugin ecosystem (#415)
- **web**: Refresh embedded dashboard build [auto] (#414)
- **web**: Refresh embedded dashboard build [auto] (#406)
- Exempt links plugin/stats with SIZE_OK (360/256 cohesive, split would fragment) (#410)
- **web**: Refresh embedded dashboard build [auto] (#402)
- **web**: Refresh embedded dashboard build [auto] (#397)
- **web**: Refresh embedded dashboard build [auto] (#393)
- **website**: Untrack generated openapi.json and add to gitignore (#374)
- **web**: Refresh embedded dashboard build [auto] (#348)
- **web**: Refresh embedded dashboard build [auto] (#345)
- **web**: Refresh embedded dashboard build [auto] (#342)
- **web**: Refresh embedded dashboard build [auto] (#337)
- **web**: Refresh embedded dashboard build [auto] (#335)
- **web**: Refresh embedded dashboard build [auto] (#328)
- **comments**: Drop restating comments, fix ones that no longer match the code (#330)
- **web**: Refresh embedded dashboard build [auto] (#320)
- **web**: Refresh embedded dashboard build [auto] (#319)
- **web**: Refresh embedded dashboard build [auto] (#313)
- **web**: Refresh embedded dashboard build [auto] (#308)
- Strip AI-slop comments and doc noise (#302)
- **web**: Refresh embedded dashboard build [auto] (#299)
- **web**: Refresh embedded dashboard build [auto] (#295)
- **web**: Refresh embedded dashboard build [auto] (#293)
- **web**: Refresh embedded dashboard build [auto] (#291)
- **web**: Align the colour lint with octarq-pro (#288)
- **web**: Refresh embedded dashboard build [auto] (#287)
- **web**: Refresh embedded dashboard build [auto] (#284)
- **web**: Refresh embedded dashboard build [auto] (#268)
- **web**: Human copy for empty/error states — five-locale i18n, build stamp, request-id error surfaces (#279)
- **web**: Font role split — machine values go mono; Badge gains shape slot (#278)
- **web**: De-AI public status page and plugin fallbacks (#277)
- **web**: De-AI plugin pages — flat help surfaces, token AI boxes, tighter empty states (#276)
- **sdk**: Flatten SDK surfaces, inline brand halo, tighten density (#275)
- **web,sdk**: De-AI pages, shell, and locked state (Batch 5) (#273)
- **web**: De-AI shell surfaces — shared popup styles, solid topbar, token skip link (#272)
- **web**: Align design tokens — flat surfaces, 4px radius, Inter only (#270)
- **web**: Refresh embedded dashboard build [auto] (#262)
- **web**: Refresh embedded dashboard build [auto] (#260)
- **web**: Refresh embedded dashboard build [auto] (#252)
- Remove empty privacy policy and terms of service pages (#251)
- **website**: Upgrade nimbus-docs to 0.9.0 (#246)
- **web**: Refresh embedded dashboard build [auto] (#245)
- **web**: Refresh embedded dashboard build [auto] (#238)
- **web**: Refresh embedded dashboard build [auto] (#231)
- **web**: Refresh embedded dashboard build [auto] (#228)
- **settings**: Name the instance slot System Settings, not Infrastructure (#227)
- **web**: Refresh embedded dashboard build [auto] (#225)
- Remove legacy compatibility shims (OSS) (#226)
- **web**: Refresh embedded dashboard build [auto] (#220)
- **web**: Refresh embedded dashboard build [auto] (#214)
- **web**: Refresh embedded dashboard build [auto] (#212)
- **web**: Refresh embedded dashboard build [auto] (#203)
- **web**: Refresh embedded dashboard build [auto] (#198)
- **web**: Refresh embedded dashboard build [auto] (#193)
- **web**: Refresh embedded dashboard build [auto] (#190)
- **web**: Refresh embedded dashboard build [auto] (#189)
- **web**: Refresh embedded dashboard build [auto] (#185)
- **web**: Refresh embedded dashboard build [auto] (#180)
- Drop three aliases that were duplicates of routes that already exist (#177)
- **web**: Refresh embedded dashboard build [auto] (#167)
- **web**: Refresh embedded dashboard build [auto] (#159)
- **web**: Refresh embedded dashboard build [auto] (#156)
- **web**: Refresh embedded dashboard build [auto] (#155)
- Cleanup navigation key discrepancies and docs (#151)
- **web**: Refresh embedded dashboard build [auto] (#149)
- **web**: Refresh embedded dashboard build [auto] (#147)
- **web**: Refresh embedded dashboard build [auto] (#139)
- **comments**: Cut narrative from frontend comments, keep the contract (#145)
- **comments**: Cut narrative from Go comments, keep the reasons (#144)
- **i18n**: Unify ui copy across 5 languages (#143)
- Drop committed agent scratch files (#140)
- **web**: Refresh embedded dashboard build [auto] (#136)
- **web**: Refresh embedded dashboard build [auto] (#131)
- **web**: Refresh embedded dashboard build [auto] (#124)
- **web**: Refresh embedded dashboard build [auto] (#119)
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **sdk**: Changeset — publish i18n locales (es/pt/ja)
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]
- **web**: Refresh embedded dashboard build [auto]

## [sdk-v0.4.0] - 2026-07-21

### 🚀 Features

- **sdk**: Rename to @octarq/plugin-sdk and publish to public npm (#33)

### 📚 Documentation

- Point plugin links to the octarq-plugins monorepo (#32)

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
- **finance**: Surface pending PDF text extraction transactions with confirm action (Invoice PDF text extraction & AI filing; image/scanned PDFs not yet supported)
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


