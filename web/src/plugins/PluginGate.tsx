// Centralized degrade boundary for plugin routes (402 upsell, 403 forbidden, 404 unavailable/crash).
import {
  Component,
  ReactNode,
  createContext,
  useContext,
  useMemo,
  useState,
} from "react";
import { LockedFeature, PluginGateContext } from "@octarq/plugin-sdk";
import type { UIPlugin, UIRoute, PluginGateContextValue } from "@octarq/plugin-sdk";
import { AccessDenied, PluginUnavailable, PluginDisabled } from "./PluginRoutes";
import { roleSatisfies, useCurrentRole } from "../shell/role";

export { PluginGateContext };
export type { PluginGateContextValue };

export interface PluginRouteGateContextValue {
  // Degrade the current route to the standard gated state for `status`
  // (402 ⇒ upsell, anything else ⇒ neutral note).
  degrade: (status: number) => void;
  // Advisory tier metadata from the route (UIRoute.requiredTier). Enforcement
  // stays server-side (the backend answers 402); this is for display only.
  requiredTier?: string;
}

const PluginRouteGateContext = createContext<PluginRouteGateContextValue | null>(null);

// Safe anywhere: outside a gate (e.g. a core page) `degrade` is a no-op, so a
// shared component may call it unconditionally.
export function usePluginGate(): PluginRouteGateContextValue {
  return useContext(PluginRouteGateContext) ?? { degrade: () => {} };
}

// The standard degraded rendering, shared by the declarative (`degrade`) and
// exceptional (error boundary) paths.
function GateFallback({ status, plugin }: { status: number; plugin: UIPlugin }) {
  if (status === 403) return <AccessDenied />;
  const Fallback = plugin.lockedFallback;
  if (Fallback) return <Fallback status={status} />;
  if (status === 402) return <LockedFeature status={402} feature={plugin.name} />;
  const isDisabled = useIsPluginDisabled(plugin);
  if (status === 404 && isDisabled) return <PluginDisabled />;
  return <PluginUnavailable />;
}

function useIsPluginDisabled(plugin: UIPlugin): boolean {
  const { disabledPlugins } = useContext(PluginGateContext);
  return disabledPlugins.has(plugin.name) ||
    (plugin.name === "domains" && disabledPlugins.has("dns"));
}

// Error-boundary half: catches chunk-load failures and render-time throws. An
// error carrying a numeric `status` (ApiError does) keeps it; anything else is
// a 404 — "this page couldn't be composed/loaded in this build".
class GateBoundary extends Component<
  { plugin: UIPlugin; children: ReactNode },
  { status: number | null }
> {
  state: { status: number | null } = { status: null };
  static getDerivedStateFromError(error: unknown) {
    const status = (error as { status?: unknown })?.status;
    return { status: typeof status === "number" ? status : 404 };
  }
  render() {
    if (this.state.status !== null) {
      return <GateFallback status={this.state.status} plugin={this.props.plugin} />;
    }
    return this.props.children;
  }
}

export function PluginGate({
  plugin,
  route,
  children,
}: {
  plugin: UIPlugin;
  route: UIRoute;
  children: ReactNode;
}) {
  const [status, setStatus] = useState<number | null>(null);
  const { role, isInstanceAdmin } = useCurrentRole();
  const { disabledPlugins, disabledPaths, loaded } = useContext(PluginGateContext);

  const ctx = useMemo<PluginRouteGateContextValue>(
    () => ({ degrade: setStatus, requiredTier: route.requiredTier }),
    [route.requiredTier],
  );
  if (status !== null) return <GateFallback status={status} plugin={plugin} />;

  if (loaded) {
    const isPluginDisabled =
      disabledPlugins.has(plugin.name) ||
      (plugin.name === "domains" && disabledPlugins.has("dns")) ||
      disabledPaths.has(route.path);
    if (isPluginDisabled) {
      return <PluginDisabled />;
    }
  }
  // Declarative pre-check: a route announcing a requiredRole the current user
  // doesn't meet renders access-denied WITHOUT mounting the page. Same ranking
  // as the sidebar filter (roleSatisfies) — and still just UX; the backend's
  // own 403 lands in the exact same fallback via degrade()/the boundary.
  if (!roleSatisfies(route.requiredRole, role, isInstanceAdmin)) {
    return <GateFallback status={403} plugin={plugin} />;
  }
  return (
    <PluginRouteGateContext.Provider value={ctx}>
      <GateBoundary plugin={plugin}>{children}</GateBoundary>
    </PluginRouteGateContext.Provider>
  );
}
