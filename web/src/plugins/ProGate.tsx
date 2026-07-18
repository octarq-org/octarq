// ProGate — the centralized degrade boundary wrapped around EVERY plugin route
// element (see PluginRoutes.tsx). It standardizes octarq's gated-state
// convention in one place so plugin pages degrade uniformly:
//
//   402 (unlicensed)          → the upsell (the plugin's lockedFallback, or the
//                               SDK's LockedFeature as the default)
//   403 (forbidden)           → the neutral AccessDenied note ("you lack
//                               permission") — also pre-rendered, without
//                               mounting the page, when the route declares a
//                               requiredRole the current user doesn't meet
//                               (member < admin < owner; instance admin
//                               bypasses). UX only — the server stays
//                               authoritative.
//   404 (not in this build)   → the neutral PluginUnavailable note
//   chunk-load / render crash → treated as 404 (the page couldn't be composed)
//
// Pages that already handle 402/404 themselves keep doing so — the gate is the
// safety net, not a replacement. It catches two things a page can't: a lazy
// chunk that fails to load, and an uncaught throw during render (including a
// thrown ApiError, whose `status` is honored). It also provides a context
// helper (`useProGate`) so a page can degrade declaratively —
// `gate.degrade(err.status)` from a data-fetch catch — instead of each page
// re-implementing the locked-state branch.
import {
  Component,
  ReactNode,
  createContext,
  useContext,
  useMemo,
  useState,
} from "react";
import { LockedFeature } from "@octarq-org/plugin-sdk";
import type { UIPlugin, UIRoute } from "@octarq-org/plugin-sdk";
import { AccessDenied, PluginUnavailable } from "./PluginRoutes";
import { roleSatisfies, useCurrentRole } from "../shell/role";

export interface PluginGateContextValue {
  disabledPlugins: Set<string>;
  disabledPaths: Set<string>;
  loaded: boolean;
}

export const PluginGateContext = createContext<PluginGateContextValue>({
  disabledPlugins: new Set(),
  disabledPaths: new Set(),
  loaded: false,
});

export interface ProGateContextValue {
  // Degrade the current route to the standard gated state for `status`
  // (402 ⇒ upsell, anything else ⇒ neutral note).
  degrade: (status: number) => void;
  // Advisory tier metadata from the route (UIRoute.requiredTier). Enforcement
  // stays server-side (the backend answers 402); this is for display only.
  requiredTier?: string;
}

const ProGateContext = createContext<ProGateContextValue | null>(null);

// Safe anywhere: outside a gate (e.g. a core page) `degrade` is a no-op, so a
// shared component may call it unconditionally.
export function useProGate(): ProGateContextValue {
  return useContext(ProGateContext) ?? { degrade: () => {} };
}

// The standard degraded rendering, shared by the declarative (`degrade`) and
// exceptional (error boundary) paths.
function GateFallback({ status, plugin }: { status: number; plugin: UIPlugin }) {
  // 403 is a role problem, not a licensing/build problem — it always renders
  // the neutral access-denied note (lockedFallback is the 402/404 seam).
  if (status === 403) return <AccessDenied />;
  const Fallback = plugin.lockedFallback;
  if (Fallback) return <Fallback status={status} />;
  // 402 without a plugin-supplied fallback still upsells — never a raw error.
  if (status === 402) return <LockedFeature status={402} feature={plugin.name} />;
  return <PluginUnavailable />;
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

export function ProGate({
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

  const ctx = useMemo<ProGateContextValue>(
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
      return <PluginUnavailable />;
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
    <ProGateContext.Provider value={ctx}>
      <GateBoundary plugin={plugin}>{children}</GateBoundary>
    </ProGateContext.Provider>
  );
}
