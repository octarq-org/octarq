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
  degrade: (status: number) => void;
  requiredTier?: string;
}

const PluginRouteGateContext = createContext<PluginRouteGateContextValue | null>(null);

export function usePluginGate(): PluginRouteGateContextValue {
  return useContext(PluginRouteGateContext) ?? { degrade: () => {} };
}

function isPluginDisabled(disabledPlugins: Set<string>, plugin: UIPlugin): boolean {
  return (
    disabledPlugins.has(plugin.name) ||
    (plugin.name === "domains" && disabledPlugins.has("dns"))
  );
}

function GateFallback({ status, plugin }: { status: number; plugin: UIPlugin }) {
  const { disabledPlugins } = useContext(PluginGateContext);

  if (status === 403) return <AccessDenied plugin={plugin} />;
  const Fallback = plugin.lockedFallback;
  if (Fallback) return <Fallback status={status} />;
  if (status === 402) return <LockedFeature status={402} feature={plugin.name} />;
  if (status === 404 && isPluginDisabled(disabledPlugins, plugin)) {
    return <PluginDisabled plugin={plugin} />;
  }
  return <PluginUnavailable plugin={plugin} />;
}

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

  if (loaded && (isPluginDisabled(disabledPlugins, plugin) || disabledPaths.has(route.path))) {
    return <PluginDisabled plugin={plugin} />;
  }
  if (!roleSatisfies(route.requiredRole, role, isInstanceAdmin)) {
    return <GateFallback status={403} plugin={plugin} />;
  }
  return (
    <PluginRouteGateContext.Provider value={ctx}>
      <GateBoundary plugin={plugin}>{children}</GateBoundary>
    </PluginRouteGateContext.Provider>
  );
}
