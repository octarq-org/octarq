---
"@octarq/plugin-sdk": minor
---

Add `UIPlugin.replaces?: string[]` — the plugin-market "enhanced edition" seam:
a plugin declares which other plugins it supersedes, and the registry excludes
the replaced plugins wholesale (routes, widgets, areas, i18n all stop applying)
so an enhanced edition can take over a feature from the OSS/vanilla plugin.

Replacement is derived at read time from the full registry — never applied at
registration — so the composed result is order-independent. Invalid
declarations are composition errors surfaced like name collisions: two plugins
replacing the same target throws in dev / console.errors in prod (neither
wins), a `replaces` name matching no composed plugin warns in dev and is
silently ignored in prod, self-replacement and replace chains are rejected.
