// App-side facade for `@octarq/plugin-sdk`.
//
// The `@octarq/plugin-sdk` alias (vite.config.ts + tsconfig paths) resolves HERE,
// not to the published package, on purpose: a plugin needs ONE import surface
// that unions two things a published package alone can't provide together —
//
//   1. the pure, publishable package (`packages/plugin-sdk`): the UIPlugin
//      contract + registry, and the shared glass-themed UI components; and
//   2. the app-COUPLED helpers that must run inside the app (they read the app's
//      i18n/brand React context): `useTranslation`, `Code`, `Guide`,
//      `LockedFeature`, and the `LockedFallback` convenience wrapper.
//
// Because the package name is aliased to this facade, this file reaches the real
// package by its source path (`../../../packages/plugin-sdk/src`) rather than by
// name (which would resolve back here). The dependency now points the RIGHT way:
// the app depends on the package, never the reverse.
export * from "../../../packages/plugin-sdk/src"; // contract + registry
export * from "./ui"; // shared UI (package) + app-coupled UI (facade)
