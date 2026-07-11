// @octarq-org/plugin-sdk/ui — the shared UI surface (the `./ui` subpath export).
//
// Every component here is pure: it depends only on React, Base UI, and the
// glass theme classes (glass/glass-strong/label, defined in the host app's
// Tailwind stylesheet). It imports NOTHING app-internal, so it publishes
// cleanly. `LockedFeature`/`LockedFallback` used to be app-coupled but are now
// driven by the SDK's own i18n + brand context (see ./locked), so they publish
// cleanly too. `Code`/`Guide` (copy-to-clipboard affordances) remain app-side.
export { cn } from "./cn";
export * from "./base";
export * from "./primitives";
export * from "./locked";
