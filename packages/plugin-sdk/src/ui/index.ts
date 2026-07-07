// @led/plugin-sdk/ui — the shared UI surface (the `./ui` subpath export).
//
// Every component here is pure: it depends only on React, Base UI, and the
// glass theme classes (glass/glass-strong/label, defined in the host app's
// Tailwind stylesheet). It imports NOTHING app-internal, so it publishes
// cleanly. App-coupled components (those needing the app's i18n/brand context —
// e.g. Code, LockedFeature) deliberately stay in the app.
export { cn } from "./cn";
export * from "./base";
export * from "./primitives";
