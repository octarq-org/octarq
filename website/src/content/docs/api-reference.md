---
title: API Reference
description: Interactive OpenAPI 3.0 specification and endpoint documentation for Octarq & Octarq Pro.
sidebar:
  order: 5
  group:
    label: "API Reference"
---

Octarq exposes a strongly-typed REST API generated directly from the Go backend handlers (via Huma OpenAPI v3).

All management operations — including authentication, workspace settings, link shorteners, mailboxes, DNS zones, and Pro modules (SSO, Audit, Finance, Infra, Billing, AI) — are fully described by the OpenAPI 3.0 spec.

## OpenAPI 3.0 Spec File

The canonical OpenAPI JSON specification is downloadable directly:
- **OpenAPI 3.0 Specification**: [`/openapi.json`](/openapi.json)

## Interactive API Explorer

Below is the live, interactive API reference generated from the specification:

<script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>

<rapi-doc 
  spec-url="/openapi.json" 
  theme="dark" 
  render-style="view" 
  show-header="false" 
  allow-authentication="true"
  allow-server-selection="true"
  style="height: 800px; width: 100%; border: 1px solid rgba(255,255,255,0.1); border-radius: 8px;"
>
</rapi-doc>
