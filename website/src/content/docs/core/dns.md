---
title: DNS
description: One-click zone sync and full record CRUD behind a provider abstraction.
---

octarq manages DNS behind a **provider abstraction** — Cloudflare and DNSPod today,
with Aliyun and Route53 slotting into the same interface.

## Zone sync

**One-click sync** pulls every zone and its records from your provider into octarq, so
you manage them from one dashboard instead of juggling provider tabs.

## Records

- Full **record CRUD** with **type and text filtering**.
- **Subdomain presets** for short-link and email setups, so wiring a new link host
  or mail domain is a couple of clicks.
- Record notes map to the provider's native **comment / remark** field, so they
  stay visible in the provider console too.

:::note
The provider interface is the extension point: adding a registrar means
implementing one interface, not touching the rest of octarq.
:::
