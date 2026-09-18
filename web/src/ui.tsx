// Barrel: UI component library, split into ./ui/* modules. Import paths
// ("../ui" / "./ui") are unchanged for all consumers.
//
// Nothing here may shadow a name the SDK already exports: an explicit named
// re-export beats `export *`, which is how an app-local Button silently replaced
// the SDK's one for every "../ui" importer while plugin packages kept the SDK
// Button. One definition per name — web/scripts/lint-ui-surface.mjs enforces it.
export * from "./ui/primitives";
export * from "./ui/HostList";
export * from "./ui/charts";
export * from "./ui/time";

export { SetupStep, type SetupStepProps } from "./components/SetupStep";

