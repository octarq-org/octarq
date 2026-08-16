# Pre-launch checklist (self-hosted)

Everything here has one property in common: **skipping it does not produce an
error.** The instance starts, the logs stay clean, and something is quietly
wrong — links that never arrive, cookies that never stick, a database nobody is
backing up. That is why this is a checklist rather than a section of the README.

Work through it once before you point real users at an instance. Each item says
what actually happens if you skip it, so you can decide what applies to you.

Cloud/multi-tenant operators: this list applies to you too, plus the extra
steps in `octarq-pro`'s `deploy/cloud/PRE-LAUNCH.md`.

---

## 1. Secrets

### `OCTARQ_SECRET_KEY` — set it explicitly

It seeds both the session-cookie HMAC and the AES-GCM that encrypts stored
credentials (OAuth client secrets, DNS provider keys, the metrics token).

- Absent → octarq **generates one and writes it next to the database**
  (`.octarq_secret`) so `docker run` works with no configuration. That file is
  now the only thing that can decrypt your stored credentials. **Losing it loses
  them.** If your deployment recreates its volume, you lose it.
- Shorter than 16 bytes → **fatal** when pointed at provisioned infrastructure
  (external Postgres or Redis), a log warning otherwise. A warning at boot is
  not something anyone reads on day 400.

```bash
OCTARQ_SECRET_KEY=$(openssl rand -hex 32)
```

- [ ] Set explicitly, at least 32 bytes, stored in your secret manager
- [ ] If you are relying on the generated file instead, it is on persistent
      storage and included in backups

### `OCTARQ_ADMIN_PASSWORD`

Absent → generated and written next to the database, and **printed to the log
in cleartext** on first boot. Fine for a laptop, not for a host whose logs are
shipped somewhere.

- [ ] Set explicitly, or first-boot logs rotated/purged after you change it

---

## 2. The reverse proxy

### `OCTARQ_TRUST_PROXY`

Almost every real deployment terminates TLS at a proxy, so octarq sees plain
HTTP and `X-Forwarded-Proto: https`.

- **Behind a proxy and not set** → octarq believes the connection is insecure
  and does **not** mark session cookies `Secure`.
- **Set with no proxy in front** → the header is client-supplied. Anyone can
  send `X-Forwarded-Proto: https` over plain HTTP, octarq marks the cookie
  `Secure`, and the browser then refuses to send it back over that same plain
  connection — locking the user out.

It is not a hardening toggle you turn on for safety. It is a statement of fact
about your topology, and it must be true.

- [ ] `true` if and only if a proxy you control sits in front

### `OCTARQ_SHARED_HOSTS`

Absolute URLs (password reset, email verification, workspace invites, OAuth
redirect) are built from the **request host**, checked against hosts somebody
has proven they own — the domains registered through the dns plugin.

A fresh instance has registered nothing, so it falls back to using the request
host as-is. **That fallback switches off the moment ANY domain is registered.**
From then on, a request arriving on a hostname that is not registered and not
declared here produces no absolute URL, and the mails that need one stop
working. Nothing logs an error that names this setting.

- [ ] Set to the dashboard's own hostname(s), comma-separated, before or at the
      same time as registering your first domain

---

## 3. Database and backups

- [ ] **Postgres, not SQLite**, for anything with concurrent users
      (`OCTARQ_DB_DRIVER=postgres`, `OCTARQ_DB_DSN=...`). SQLite is the
      zero-config default so a first run needs nothing; it is not the
      production answer.
- [ ] **Backups are scheduled by you.** The dashboard offers an on-demand
      full-database download (Settings → Instance). Nothing runs on a timer —
      there is no built-in scheduler, so "we have backups" is only true once you
      have written the cron job and restored from one.
- [ ] **Restore has been tested.** An untested backup is a hypothesis.
- [ ] The secret key (§1) is backed up **separately** from the database.
      Backing up an encrypted database next to nothing that can decrypt it is a
      common and complete loss.

`octarq backup` / `octarq restore` and the container restrictions (Postgres
needs host-side `pg_dump`) are documented in
[`website/src/content/docs/backup-restore.md`](website/src/content/docs/backup-restore.md).

---

## 4. Who can get in

- [ ] **Public sign-up**: Settings → open registration is **on by default**.
      If this instance is for your team only, turn it off, or anyone with the
      URL gets an account.
- [ ] **Email verification**: on by default. Leaving it on requires working
      outbound mail — otherwise nobody can complete sign-up. Configure mail
      first, or turn verification off deliberately.
- [ ] **Reserved slugs/mailboxes**: reserve the names you do not want a user to
      take (`admin`, `support`, `billing`, your brand).
- [ ] **Rate limits**: auth / API / redirect limits have defaults
      (60 / 600 / 6000 per minute per IP). Confirm they suit your traffic
      before a launch spike rather than during one.

---

## 5. Network egress

Both of these default to **off**, which is the safe setting. They exist for
deployments where the target genuinely is on a private network.

- [ ] `OCTARQ_ALLOW_PRIVATE_WEBHOOKS` — on lets a user's webhook URL point at
      your internal network (SSRF). Leave off unless you need it.
- [ ] `OCTARQ_ALLOW_PRIVATE_SMTP` — same reasoning for mail servers.

---

## 6. Cross-origin reads (only if something needs them)

Public GET endpoints can be read cross-origin by an **exact-origin allowlist**.
Empty allowlist = no CORS headers at all = the default, and the right setting if
nothing external reads your API from a browser.

If you run a separate marketing or docs site that fetches from this instance:

- [ ] Allowlist the exact origin — `https://example.com`, **not**
      `example.com`, no trailing slash, no path. A near-miss silently fails:
      the endpoint answers 200, the browser discards the response, and the page
      renders nothing with no error anywhere.
- [ ] Runtime setting (Settings) is the source of truth; `OCTARQ_CORS_ORIGINS`
      is only a bootstrap fallback for an instance that has not saved the
      setting yet.

Credentials are never allowed cross-origin, by design: these endpoints serve
public data, and allowing credentials would let any compromised allowlisted
origin act as your users against every API on the instance.

---

## 7. Observability

- [ ] **`/metrics`**: with no token configured it is **loopback-only**. Set a
      token if you scrape from another host; do not expose it unauthenticated.
- [ ] Log destination decided, and first-boot logs (which may contain the
      generated admin password, §1) handled.
- [ ] **Data retention**: 0 means keep forever. Set it if you have a retention
      obligation.

---

## 8. Before you announce

- [ ] Sign up as a new user end to end, on the real hostname, over HTTPS:
      registration → verification mail arrives → password reset mail arrives →
      the links in both open the right instance.

That single walk-through exercises §1, §2, and §4 together, and it is the
fastest way to catch a `SHARED_HOSTS` or `TRUST_PROXY` mistake — both of which
look completely fine from the dashboard you are already logged into.
