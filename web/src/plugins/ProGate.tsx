// ProGate — the centralized degrade boundary wrapped around EVERY plugin route
// element (see PluginRoutes.tsx). It standardizes octarq's gated-state
// convention in one place so plugin pages degrade uniformly:
//
//   402 (unlicensed)          → the upsell (the plugin's lockedFallback, or the
//                               SDK's LockedFeature as the default)
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
import { PluginUnavailable } from "./PluginRoutes";

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
  const ctx = useMemo<ProGateContextValue>(
    () => ({ degrade: setStatus, requiredTier: route.requiredTier }),
    [route.requiredTier],
  );
  if (status !== null) return <GateFallback status={status} plugin={plugin} />;
  return (
    <ProGateContext.Provider value={ctx}>
      <GateBoundary plugin={plugin}>{children}</GateBoundary>
    </ProGateContext.Provider>
  );
}
