// Barrel: UI component library, split into ./ui/* modules. Import paths
// ("../ui" / "./ui") are unchanged for all consumers.
//
// No explicit Button/buttonVariants re-export here on purpose: an explicit named
// export beats `export *`, so the app-local Button this file used to re-export
// silently replaced the SDK's one for every "../ui" importer while plugins kept
// the SDK Button. There is one Button now, via ./ui/primitives below.
export * from "./ui/primitives";
export * from "./ui/HostList";
export * from "./ui/charts";
export * from "./ui/time";

// App-coupled; the first three move into the SDK next (Alert's cn is the last
// consumer of web/src/lib/utils.ts).
export { Alert, alertVariants } from "./components/ui/Alert";
export { FormError } from "./components/ui/FormError";
export { RouteFallback } from "./components/ui/RouteFallback";
export { SetupStep, type SetupStepProps } from "./components/SetupStep";

