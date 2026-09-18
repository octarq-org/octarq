// App-coupled UI.
//
// These read THIS app's own data and context — the api types, the brand
// provider, the i18n dictionary, the home setup checklist — so they cannot live
// in the plugin SDK, which must publish with no app-internal imports. Everything
// else the product renders comes from `@octarq/plugin-sdk` by name; this barrel
// is the only other place app code takes a component from.
export * from "./HostList";
export * from "./charts";
export * from "./SetupStep";
