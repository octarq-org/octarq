// Barrel: UI component library, split into ./ui/* modules. Import paths
// ("../ui" / "./ui") are unchanged for all consumers.
// LockedFeature/LockedFallback now come from the SDK (re-exported via
// ./ui/primitives) rather than an app-local module.
export * from "./ui/primitives";
export * from "./ui/HostList";
export * from "./ui/charts";
export * from "./ui/time";

// Semantic UI components
export { Alert, alertVariants } from "./components/ui/Alert";
export { Badge, badgeVariants, type BadgeProps } from "./components/ui/Badge";
export { Button, buttonVariants, type ButtonProps } from "./components/ui/Button";
