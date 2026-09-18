// App-side facade for `@octarq/plugin-sdk`.
//
// The `@octarq/plugin-sdk` alias (vite.config.ts + tsconfig paths) resolves HERE,
// not to the published package. It is a single hop now: everything a plugin
// touches — the UIPlugin contract + registry, the shared UI components, i18n and
// brand — already lives in the package. This file used to union that with
// app-coupled helpers (Code/Guide/timeAgo); those were promoted into the package
// too, so there is no second surface left to union.
//
// The package is reached by source path rather than by name, because the name
// resolves back to this file.
export * from "../../../packages/plugin-sdk/src";
