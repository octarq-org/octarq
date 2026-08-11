---
title: API Reference
description: Interactive OpenAPI 3.0 specification and endpoint documentation for Octarq.
sidebar:
  order: 5
  group:
    label: "API Reference"
---

Octarq exposes a REST API generated from Go backend handlers via Huma OpenAPI v3.

Management operations (authentication, workspace settings, short links, mailboxes, and DNS zones) are defined in the OpenAPI 3.0 spec.

This specification covers the open-source core of Octarq. Octarq Pro instances expose the same `/openapi.json` with their plugin endpoints (finance, infrastructure, and AI) included.

## OpenAPI 3.0 Spec File

The canonical OpenAPI JSON specification is downloadable directly:
- **OpenAPI 3.0 Specification**: [`/openapi.json`](/openapi.json)

## Interactive API Explorer

Below is the live, interactive API reference generated from the specification:
