---
"@octarq-org/plugin-sdk": minor
---

Add optional `groups?: string[]` to `UIArea`. A plugin-declared area can now
carry ordered group shells, and a menu whose `category` matches one of those
group labels routes into the area (in addition to matching the area's id or
title). This lets a Pro edition own a whole multi-group area — e.g. Commerce
with Sales/Billing/Finance — after the OSS core stopped shipping an empty shell
and keyword routing for it.
