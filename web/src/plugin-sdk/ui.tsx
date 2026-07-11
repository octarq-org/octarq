// Facade extras — the ONLY components that stay app-side.
//
// The shared UI (primitives), i18n (useTranslation/I18nProvider), brand
// (useAppName), and the gated-state UI (LockedFeature/LockedFallback) now all
// come from the published package and are re-exported by ../plugin-sdk (index).
// What remains here is `Code`/`Guide` — copy-to-clipboard affordances that reach
// past the SDK's context — and `timeAgo`, a host-side date util. Kept app-side so
// the published package needs neither.
export { Code, Guide, timeAgo } from "../ui";
