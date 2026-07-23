---
title: Short links
description: Custom slugs, routing rules, expiry, click limits, UTM, QR, and analytics.
---

Short links are octarq's root namespace: every path that isn't `/admin` or `/api` is
a potential slug, so `https://go.example.com/abc` redirects without ever colliding
with the dashboard.

## Creating a link

- **Slug** — custom or random.
- **Host** — picked from your link-enabled domains.
- **Destination** — the target URL; one-click **title fetch** pulls the page title.

## Controls

| Feature | What it does |
| --- | --- |
| **Password protection** | Visitors must enter a password before redirecting. |
| **Expiry + fallback** | Expire a link on a date and optionally redirect to a fallback URL afterward. |
| **Click limits** | Cap total redirects; over the cap the link stops resolving. |
| **Advanced routing** | Send visitors to different destinations by **geo, device, OS, or language**. |
| **Tags + archive** | Organize and retire links without deleting them. |
| **UTM builder** | Compose campaign parameters inline. |
| **QR codes** | Generate a QR for any link. |

## Analytics

Each link records a time series plus breakdowns by **referer, country, device, and
browser**, with **bot detection** so automated hits don't pollute your numbers.
Click events are written asynchronously so redirects stay fast.
