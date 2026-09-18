---
"@octarq/plugin-sdk": minor
---

Design tokens are now enumerable as data (`TOKENS`, `SEED_TOKENS`,
`tokensByGroup`, `tokenByName`, `themeAlias`, `readTokenValues`).

The token set previously existed only as CSS custom properties in the host
stylesheet, so nothing could enumerate it: picking the right token meant reading
`styles.css`, and building any kind of token inspector meant scraping it or
reading computed styles off the DOM.

The inventory carries **names, groups and roles — not values**:

- `TokenDef.root` / `.dark` / `.themed` record which of the three blocks
  (`:root`, `.dark`, `@theme inline`) declares the token, so a consumer can tell
  theme-invariant tokens from ones with a dark override.
- `seed` marks the three literals a white-label rebrand overrides — the entire
  rebrand write surface; everything brand-tinted derives from `--primary`.
- `readTokenValues()` resolves live values from the DOM, which is the only source
  that reflects a runtime rebrand and the active light/dark mode.

Values are deliberately not copied here: a duplicated brand hex is what the
colour lint forbids, and a static copy would go stale against a rebrand. The set
is held in lockstep with the stylesheet by a fail-closed parity test in the host
(`web/src/test/tokensParity.test.ts`) asserting set equality in both directions
for all three blocks.
