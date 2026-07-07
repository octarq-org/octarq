// ─── Frontend plugin injection point (OSS / default edition) ─────────────────
//
// This is the OSS injection module: it composes NOTHING. The registry stays
// empty, so every Pro route 404-degrades through the app's neutral plugin
// fallback and no Pro page bytes are shipped.
//
// A commercial build swaps this module for ./index.pro.ts via the `#led-plugins`
// alias (see vite.config.ts, keyed on VITE_LED_PLUGINS=pro). That is the
// frontend analog of a led-pro binary calling `app.App.Use(...)` for its Go
// plugins: the composition happens at build time, entirely outside the OSS core.
//
// This file intentionally has no imports — being empty is the whole point.
export {};
