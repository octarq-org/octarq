---
"@octarq/plugin-sdk": patch
---

Surface `UIPlugin` name collisions instead of dropping the second registration
in silence. A duplicate name now throws in development and logs an error in
production, mirroring the backend's `preflightNameCollisions`, which refuses to
start on duplicate plugin names.

Silence was how the Pro audit page went unrendered since its first release: it
shared a name with the core audit plugin, and the registry returned early
without a word. A plugin name is an identity, and two plugins claiming one is a
composition mistake the author needs to see.
