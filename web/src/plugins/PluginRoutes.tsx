// Maps composed UIPlugins into <Route> elements wrapped in PluginGate, with fallback components for unserved routes.
import { Suspense } from "react";
import { Route, Link } from "react-router-dom";
import { useCurrentRole, roleSatisfies } from "../shell/role";
import { GlassCard, PageHeader, ScreenWrap, useTranslation } from "@octarq/plugin-sdk";
import { uiPlugins } from "@octarq/plugin-sdk";
import type { UIPlugin } from "@octarq/plugin-sdk";
import { PluginGate } from "./PluginGate";

// The three fallbacks answer the same three questions every degraded route
// should: which plugin (id in mono), what its current state is, and where the
// state can be changed. The plugin id is a machine identifier → mono.

function PluginIdLine({ plugin }: { plugin?: UIPlugin }) {
  const { t } = useTranslation();
  if (!plugin) return null;
  return (
    <p className="text-sm text-foreground/60">
      {t("uiCommon.pluginId")}: <span className="font-mono text-foreground/80">{plugin.name}</span>
    </p>
  );
}

// Fallback view for unserved or failed plugin routes (404). Also the app's
// catch-all route, where no plugin is known — `plugin` is optional there.
export function PluginUnavailable({ plugin }: { plugin?: UIPlugin }) {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <GlassCard className="mx-auto mt-12 flex max-w-md flex-col items-center gap-3 px-5 py-10 text-center">
        <PageHeader
          title={t("uiCommon.routeUnavailableTitle")}
          description={t("uiCommon.routeUnavailableBody")}
        />
        <PluginIdLine plugin={plugin} />
        {plugin && <p className="text-xs text-foreground/45">{t("uiCommon.routeUnavailableState")}</p>}
      </GlassCard>
    </ScreenWrap>
  );
}

// View for plugins that are disabled in the current workspace.
export function PluginDisabled({ plugin }: { plugin: UIPlugin }) {
  const { t } = useTranslation();
  const roleCtx = useCurrentRole();
  const canEnable = roleSatisfies("admin", roleCtx.role, roleCtx.isInstanceAdmin);
  return (
    <ScreenWrap>
      <GlassCard className="mx-auto mt-12 flex max-w-md flex-col items-center gap-3 px-5 py-10 text-center">
        <PageHeader
          title={t("uiCommon.pluginDisabledTitle")}
          description={canEnable ? t("uiCommon.pluginDisabledAdminBody") : t("uiCommon.pluginDisabledBody")}
        />
        <PluginIdLine plugin={plugin} />
        <p className="text-xs text-foreground/45">{t("uiCommon.pluginDisabledState")}</p>
        {canEnable && (
          <Link to="/settings/plugins" className="text-primary hover:text-primary-hover font-medium text-sm">
            {t("uiCommon.pluginDisabledAdminLink")}
          </Link>
        )}
      </GlassCard>
    </ScreenWrap>
  );
}

// Fallback view when user lacks required permissions (403).
export function AccessDenied({ plugin }: { plugin: UIPlugin }) {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <GlassCard className="mx-auto mt-12 flex max-w-md flex-col items-center gap-3 px-5 py-10 text-center">
        <PageHeader
          title={t("uiCommon.accessDeniedTitle")}
          description={t("uiCommon.accessDeniedBody")}
        />
        <PluginIdLine plugin={plugin} />
        <p className="text-xs text-foreground/45">{t("uiCommon.accessDeniedState")}</p>
        <Link to="/settings/members" className="text-primary hover:text-primary-hover font-medium text-sm">
          {t("uiCommon.accessDeniedLink")}
        </Link>
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
