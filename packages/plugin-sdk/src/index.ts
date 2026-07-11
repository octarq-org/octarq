// @octarq-org/plugin-sdk — package root (the `.` export).
//
// Carries the full plugin surface so a plugin package has ONE import specifier:
//   - contract + registry (the UIPlugin type, registerUIPlugin, uiPluginI18n…)
//   - ui (glass-themed components, incl. LockedFeature/LockedFallback)
//   - i18n (I18nProvider + useTranslation, host feeds resources)
//   - brand (BrandProvider + useAppName, host feeds the product name)
// The pure component set is still also available under the `./ui` subpath for a
// consumer that wants only components. ESM tree-shaking prunes anything unused
// (e.g. the build-time injection module that imports only registerUIPlugin).
export * from "./contract";
export * from "./ui";
export * from "./i18n";
export * from "./brand";
