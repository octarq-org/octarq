---
title: "Why I Rewrote a SaaS Stack as a Go Plugin Framework"
description: "The technical story behind Octarq: replacing fragmented microservices and vendor lock-in with a single, open-core Go binary and modular plugin system."
sidebar:
  order: 3
  group:
    label: "Guides"
---

Most SaaS applications start out simple: a database, a backend server, and a frontend dashboard. As features scale — custom domains, mailbox routing, Stripe billing, SSO, role-based access control, notification dispatching, and audit logging — the architecture often becomes difficult to manage across separate microservices or locked into hosted infrastructure.

Octarq offers an open-core Go plugin framework that packs backoffice infrastructure (short links, mailboxes, DNS management, AI helpers, notifications) into a single binary, while allowing enterprise features (OIDC SSO, Audit Logs, Metered Billing) to be injected via decoupled plugins.

Here is the architectural story of why and how we built it.

---

## 1. The Core Philosophy: "Own the Stack"

When developers self-host software today, they shouldn't need a Kubernetes cluster just to run a backoffice. 

By building on **Go** and pure-Go **SQLite** (with Postgres driver support), Octarq compiles into a single static binary. 
- **Zero-config startup**: Running `docker run -p 8080:8080 -v octarq-data:/data octarq/octarq` auto-generates secret keys and admin credentials on first boot.
- **Low footprint**: Starts in under 15ms and consumes under 30MB of RAM.
- **Data Sovereignty**: All SQLite/Postgres data remains under the operator's physical control.

---

## 2. Decoupled Seams & `plugin.Context`

Rather than hardcoding backoffice features into a monolithic app struct, Octarq core provides a strict, decoupled seam system. Every module — whether built-in or loaded as a Pro plugin — mounts via `plugin.Context`:

```go
type Plugin interface {
    Name() string
    Init(ctx context.Context, pctx *plugin.Context) error
    Mount(router *http.ServeMux, pctx *plugin.Context) error
}
```

The `*plugin.Context` provides safe, isolated primitives:
- **`Huma`**: Strongly-typed OpenAPI v3 route registration.
- **`DB`**: Tenant-scoped database handles (`orgDB(r)`).
- **`RequireRole`**: Fine-grained role-based authorization gates (`rolegate`).
- **`Encrypt` / `Decrypt`**: Cryptographic storage at rest for API keys and capabilities.
- **`Audit`**: Structured audit event recording with IP & actor tracking.

### Plugin Registration Example

Because core features (like `links`, `mail`, and `dns`) use the exact same seam interfaces as Pro extensions (`sso`, `audit`, `slack`, `whitelabel`), there are no "hidden internal APIs". A plugin can register custom notification handlers, custom role permissions, or custom UI menus:

```go
// Registering a Slack notification provider from a plugin
pctx.RegisterNotifier("slack", func(ctx context.Context, cfg string, msg notify.Message) error {
    return slack.Send(ctx, cfg, msg)
})
```

---

## 3. OpenAPI-First with Huma v2

Rest APIs often suffer from documentation drift. Octarq resolves this by building every HTTP route using **Huma v2** over standard `http.ServeMux`.

Every handler defines typed input and output structs with automatic JSON schema validation and OpenAPI 3.0 generation:

```go
type CreateLinkInput struct {
    Body struct {
        Slug string `json:"slug" doc:"Shortlink slug"`
        URL  string `json:"url" format:"uri" doc:"Destination URL"`
    }
}
```

This guarantees that:
1. Invalid requests are rejected automatically with structured 400 Bad Request errors.
2. The entire API surface — core and plugins — is automatically documented and browsable via `/openapi.json` and interactive API docs.
3. Client SDKs (like `@octarq/plugin-sdk`) stay 100% in sync with backend Go definitions.

---

## 4. Defense-in-Depth & SSRF Protection

Security in a multi-tenant backoffice requires active defenses, not just password checks. Octarq implements multi-layered isolation:

- **Socket-Level SSRF Guards**: Outbound webhook and SMTP delivery pass through `safehttp.Control`, which inspects final resolved IP addresses at dial time (`net.Dialer.Control`). Attempts to reach loopback (`127.0.0.1`), RFC1918 private subnets, or cloud metadata (`169.254.169.254`) are blocked before the socket connects, shutting down DNS rebinding attacks.
- **Role Ceiling Authorization**: API tokens operate under an `effectiveRole = min(live_membership, token_cap)` security ceiling. Even if a token holds an `owner` role, revoking the user's workspace membership immediately revokes the token.
- **Host Ownership Verification**: Short links and mail domains enforce strict owner validation on creation, update, and resolution — preventing cross-tenant host squatting.

---

## 5. OSS Purity + Pro Business Model

Open-source projects often struggle when commercial features taint the open codebase with upsell prompts or incomplete stubs.

Octarq maintains strict **OSS Purity**:
- The open-source `octarq` repository contains zero pricing code, commercial copy, or license verification logic.
- Pro extensions (`octarq-pro`) live in clean, decoupled Go modules.
- Upgrades work by swapping or compiling in Pro plugin packages — without modifying core Go source code.

---

## Summary

Rewriting a SaaS stack into a Go plugin framework delivered the best of both worlds: the simplicity and performance of a compiled single-binary, combined with the modularity and extensibility of enterprise software.

Whether self-hosting on a $5/mo VPS or scaling across multi-tenant Cloud nodes, Octarq lets you **own your stack**.

- Explore the code on [GitHub](https://github.com/octarq-org/octarq)
- Read the [Documentation](/quickstart/)
- Check out the [API Reference](/api-reference/)
