// The registry seam rendered: turns composed UIPlugins into <Route> elements
// for App's <Routes>, and provides the neutral fallback the OSS build shows for
// any path a plugin would own but isn't composed in.
//
// Kept out of App.tsx so the routing shell doesn't grow a plugin-plumbing bulge.
import { Component, ReactNode, Suspense } from "react";
import { Route } from "react-router-dom";
import { GlassCard, PageHeader, ScreenWrap, useTranslation } from "@octarq-org/plugin-sdk";
import { uiPlugins } from "@octarq-org/plugin-sdk";
import type { LockedFallbackType } from "@octarq-org/plugin-sdk";

// Neutral "this feature isn't part of this build" note — the frontend mirror of
// the backend answering 404 for a plugin that isn't mounted. Shown for any
// unmatched path (empty registry ⇒ Pro routes land here) and as the default
// degrade when a plugin page chunk fails to load.
export function PluginUnavailable() {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <GlassCard className="mx-auto mt-12 flex max-w-md flex-col items-center gap-3 px-6 py-14 text-center">
        <PageHeader
          title={t("uiCommon.routeUnavailableTitle")}
          description={t("uiCommon.routeUnavailableBody")}
        />
      </GlassCard>
    </ScreenWrap>
  );
}

// Boundary that degrades a plugin page to its lockedFallback (or the neutral
// note) if its lazy chunk throws — so a half-composed build never white-screens.
class PluginBoundary extends Component<
  { fallback: LockedFallbackType | undefined; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    if (this.state.failed) {
      const Fallback = this.props.fallback;
      // 404 status: the page couldn't be composed/loaded in this build.
      return Fallback ? <Fallback status={404} /> : <PluginUnavailable />;
    }
    return this.props.children;
  }
}

// The composed plugin routes, as an array of <Route> for <Routes>. Empty when
// the registry is empty (OSS build) — then every such path falls to App's
// catch-all neutral fallback.
export function pluginRouteElements() {
  return uiPlugins().flatMap((plugin) =>
    plugin.routes.map((route) => {
      const Page = route.Component;
      return (
        <Route
          key={route.path}
          path={route.path}
          element={
            <PluginBoundary fallback={plugin.lockedFallback}>
              <Suspense fallback={null}>
                <Page />
              </Suspense>
            </PluginBoundary>
          }
        />
      );
    }),
  );
}
