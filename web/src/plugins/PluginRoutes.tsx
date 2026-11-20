import { Suspense, type ReactNode } from "react";
import { Route, Link, matchPath } from "react-router-dom";
import { PackageX, PowerOff, ShieldX } from "lucide-react";
import { useCurrentRole, roleSatisfies } from "../shell/role";
import { buttonVariants, useTranslation, RouteFallback } from "@octarq/plugin-sdk";
import { uiPlugins, uiRoutes } from "@octarq/plugin-sdk";
import type { UIPlugin } from "@octarq/plugin-sdk";
import { PluginGate } from "./PluginGate";


function PluginIdLine({ plugin }: { plugin?: UIPlugin }) {
  const { t } = useTranslation();
  if (!plugin) return null;
  return (
    <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">
      {t("uiCommon.pluginId")}: <span className="text-foreground">{plugin.name}</span>
    </p>
  );
}

interface FallbackCardProps {
  icon: ReactNode;
  title: string;
  body: string;
  plugin?: UIPlugin;
  state?: ReactNode;
  action?: ReactNode;
}

function FallbackCard({ icon, title, body, plugin, state, action }: FallbackCardProps) {
  return (
    <div className="mx-auto mt-10 flex max-w-xl flex-col items-center gap-3 rounded-lg border border-border bg-card px-6 py-14 text-center">
      <div className="flex h-12 w-12 items-center justify-center rounded-lg border border-border bg-well text-muted-foreground [&_svg]:h-5 [&_svg]:w-5">
        {icon}
      </div>
      <h1 className="font-display text-[17px] font-semibold leading-tight tracking-[-0.01em] text-foreground">
        {title}
      </h1>
      <p className="max-w-sm text-[13px] leading-relaxed text-muted-foreground">{body}</p>
      {plugin && <PluginIdLine plugin={plugin} />}
      {state && <p className="text-xs text-muted-foreground">{state}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}

export function PluginUnavailable({ plugin }: { plugin?: UIPlugin }) {
  const { t } = useTranslation();
  return (
    <FallbackCard
      icon={<PackageX />}
      title={t("uiCommon.routeUnavailableTitle")}
      body={t("uiCommon.routeUnavailableBody")}
      plugin={plugin}
      state={plugin ? t("uiCommon.routeUnavailableState") : undefined}
    />
  );
}

export function PluginDisabled({ plugin }: { plugin: UIPlugin }) {
  const { t } = useTranslation();
  const roleCtx = useCurrentRole();
  const canEnable = roleSatisfies("admin", roleCtx.role, roleCtx.isInstanceAdmin);
  return (
    <FallbackCard
      icon={<PowerOff />}
      title={t("uiCommon.pluginDisabledTitle")}
      body={canEnable ? t("uiCommon.pluginDisabledAdminBody") : t("uiCommon.pluginDisabledBody")}
      plugin={plugin}
      state={t("uiCommon.pluginDisabledState")}
      action={
        canEnable ? (
          <Link to="/settings/plugins" className={buttonVariants({ variant: "secondary", size: "sm" })}>
            {t("uiCommon.pluginDisabledAdminLink")}
          </Link>
        ) : undefined
      }
    />
  );
}

export function AccessDenied({ plugin }: { plugin: UIPlugin }) {
  const { t } = useTranslation();
  return (
    <FallbackCard
      icon={<ShieldX />}
      title={t("uiCommon.accessDeniedTitle")}
      body={t("uiCommon.accessDeniedBody")}
      plugin={plugin}
      state={t("uiCommon.accessDeniedState")}
      action={
        <Link to="/settings/members" className={buttonVariants({ variant: "secondary", size: "sm" })}>
          {t("uiCommon.accessDeniedLink")}
        </Link>
      }
    />
  );
}

export function routeLayoutFor(
  routes: { path: string; layout?: "default" | "full" }[],
  pathname: string,
): "default" | "full" {
  for (const route of routes) {
    if (route.layout !== "full") continue;
    if (matchPath({ path: route.path, end: true }, pathname)) return "full";
  }
  return "default";
}

export function pluginRouteLayout(pathname: string): "default" | "full" {
  return routeLayoutFor(uiRoutes(), pathname);
}

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
              <Suspense fallback={<RouteFallback />}>
                <Page />
              </Suspense>
            </PluginGate>
          }
        />
      );
    }),
  );
}
