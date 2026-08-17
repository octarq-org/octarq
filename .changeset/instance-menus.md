---
"@octarq/plugin-sdk": minor
---

Add `UIPlugin.instanceRoutes?: UIRoute[]` and `uiInstanceRoutes()` — the
frontend half of the instance-scope plugin seam. Routes registered here render
under the `/instance` console basename instead of `/admin`, paired with the Go
`plugin.InstanceMenuProvider` interface whose entries the instance-admin-gated
`GET /api/instance/menus` endpoint serves. The console renders a plugin rail
entry only when the backend announces it AND the frontend registers a route for
the same path, mirroring the tenant sidebar's trust in `/api/menus`.

`instanceRoutes` goes through the same `replaces` filtering as `routes`: a
replaced plugin's instance pages disappear with it, so a superseded plugin can
never keep a page in the instance console.
