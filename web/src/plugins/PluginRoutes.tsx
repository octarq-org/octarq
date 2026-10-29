// Maps composed UIPlugins into <Route> elements wrapped in PluginGate, with fallback components for unserved routes.
import { Suspense, type ReactNode } from "react";
import { Route, Link } from "react-router-dom";
import { PackageX, PowerOff, ShieldX } from "lucide-react";
import { useCurrentRole, roleSatisfies } from "../shell/role";
import { ScreenWrap, buttonVariants, useTranslation, RouteFallback } from "@octarq/plugin-sdk";
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

// One panel for all three states, so a 402, a 403 and a 404 read the same way:
// an icon, one reason, one path forward.
function FallbackCard({ icon, title, body, plugin, state, action }: FallbackCardProps) {
  return (
    <div className="mx-auto mt-10 flex max-w-xl flex-col items-center gap-3 rounded-2xl border border-border bg-card px-6 py-14 text-center shadow-[var(--shadow-card)]">
      <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-well text-muted-foreground ring-1 ring-border [&_svg]:h-5 [&_svg]:w-5">
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

// Fallback view for unserved or failed plugin routes (404). Also the app's
// catch-all route, where no plugin is known — `plugin` is optional there.
export function PluginUnavailable({ plugin }: { plugin?: UIPlugin }) {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <FallbackCard
        icon={<PackageX />}
        title={t("uiCommon.routeUnavailableTitle")}
        body={t("uiCommon.routeUnavailableBody")}
        plugin={plugin}
        state={plugin ? t("uiCommon.routeUnavailableState") : undefined}
      />
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
    </ScreenWrap>
  );
}

// Fallback view when user lacks required permissions (403).
export function AccessDenied({ plugin }: { plugin: UIPlugin }) {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
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
              {/* Not fallback={null}: this boundary is closer to the lazy page
                  than the shell's Suspense, so a null fallback would mask the
                  RouteFallback spinner and white-screen plugin pages while
                  their chunk loads. */}
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
