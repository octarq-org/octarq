// The registry seam rendered: turns composed UIPlugins into <Route> elements
// for App's <Routes>, and provides the neutral fallback the OSS build shows for
// any path a plugin would own but isn't composed in.
//
// Kept out of App.tsx so the routing shell doesn't grow a plugin-plumbing bulge.
import { Suspense } from "react";
import { Route } from "react-router-dom";
import { GlassCard, PageHeader, ScreenWrap, useTranslation } from "@octarq-org/plugin-sdk";
import { uiPlugins } from "@octarq-org/plugin-sdk";
import { ProGate } from "./ProGate";

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

// The composed plugin routes, as an array of <Route> for <Routes>. Empty when
// the registry is empty (OSS build) — then every such path falls to App's
// catch-all neutral fallback. Every element is wrapped in ProGate — the
// centralized degrade boundary (402 ⇒ upsell, 404/chunk failure ⇒ neutral
// note) — so pages degrade uniformly even without per-page handling.
export function pluginRouteElements() {
  return uiPlugins().flatMap((plugin) =>
    plugin.routes.map((route) => {
      const Page = route.Component;
      return (
        <Route
          key={route.path}
          path={route.path}
          element={
            <ProGate plugin={plugin} route={route}>
              <Suspense fallback={null}>
                <Page />
              </Suspense>
            </ProGate>
          }
        />
      );
    }),
  );
}
