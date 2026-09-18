// Forwarder to the SDK's `./ui` subpath. Reached by relative path, not by the
// `@octarq/plugin-sdk` package name: that name is aliased to the app-side facade
// (web/src/plugin-sdk), which re-exports this package — going by name here would
// close the loop the other way.
export * from "../../../packages/plugin-sdk/src/ui";
