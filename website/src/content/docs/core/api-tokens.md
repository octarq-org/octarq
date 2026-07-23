---
title: API tokens
description: Bearer tokens for the JSON API, stored as SHA-256 hashes.
---

Every data endpoint in octarq accepts either a session cookie or a bearer token, so
you can script against the same API the dashboard uses.

## Issuing a token

Create a token in the dashboard. The raw value is shown **once** at creation —
copy it then. octarq stores only a **SHA-256 hash**, so a database leak never exposes
a usable token.

## Using a token

```bash
curl https://octarq.example.com/api/v1/links \
  -H "Authorization: Bearer oct_xxxxxxxxxxxxxxxxxxxx"
```

Any endpoint that works with a session cookie works with `Authorization: Bearer
oct_…`.
