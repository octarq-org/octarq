// Maps composed UIPlugins into <Route> elements wrapped in PluginGate, with fallback components for unserved routes.
import { Suspense } from "react";
import { Route, Link } from "react-router-dom";
import { useCurrentRole, roleSatisfies } from "../shell/role";
import { GlassCard, PageHeader, ScreenWrap, useTranslation } from "@octarq/plugin-sdk";
import { uiPlugins } from "@octarq/plugin-sdk";
import { PluginGate } from "./PluginGate";

// Fallback view for unserved or failed plugin routes (404).
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

// View for plugins that are disabled in the current workspace.
export function PluginDisabled() {
  const { t } = useTranslation();
  const roleCtx = useCurrentRole();
  const canEnable = roleSatisfies("admin", roleCtx.role, roleCtx.isInstanceAdmin);
  return (
    <ScreenWrap>
      <GlassCard className="mx-auto mt-12 flex max-w-md flex-col items-center gap-3 px-6 py-14 text-center">
        <PageHeader
          title={t("uiCommon.pluginDisabledTitle")}
          description={canEnable ? t("uiCommon.pluginDisabledAdminBody") : t("uiCommon.pluginDisabledBody")}
        />
        {canEnable && (
          <Link to="/settings/plugins" className="text-indigo-500 hover:text-indigo-400 font-medium text-sm">
            {t("uiCommon.pluginDisabledAdminLink")}
          </Link>
        )}
      </GlassCard>
    </ScreenWrap>
  );
}

// Fallback view when user lacks required permissions (403).
export function AccessDenied() {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <GlassCard className="mx-auto mt-12 flex max-w-md flex-col items-center gap-3 px-6 py-14 text-center">
        <PageHeader
          title={t("uiCommon.accessDeniedTitle")}
          description={t("uiCommon.accessDeniedBody")}
        />
      </GlassCard>
    </ScreenWrap>
  );
}

// Returns <Route> elements for all registered plugin routes.
export function pluginRouteElements() {
  return uiPlugins().flatMap((plugin) =>
    plugin.routes.map((route) => {
      const Page = route.Component;
      return (
        <Route
          key={route.path}
          path={route.path}
          element={
            <PluginGate plugin={plugin} route={route}>
              <Suspense fallback={null}>
                <Page />
              </Suspense>
            </PluginGate>
          }
        />
      );
    }),
  );
}
