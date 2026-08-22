// The instance console — a standalone shell for deployment-wide settings,
// mounted at its own /instance basename (see main.tsx). Deliberately NOT the
// tenant shell (App.tsx): no org switcher, no plugin menu pipeline, no
// workspace chrome. Instance admins only; everyone else gets a neutral notice
// and a way back to /admin — the server stays authoritative either way.
import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { Navigate, NavLink, Route, Routes } from "react-router-dom";
import { Menu } from "@base-ui/react/menu";
import { ArrowLeft, Globe, KeyRound, LayoutDashboard, Moon, Puzzle, Server, Sun } from "lucide-react";
import { uiInstanceRoutes } from "@octarq/plugin-sdk";
import { api, type MenuItem } from "../../api";
import { BrandMark } from "../../shell/BrandMark";
import { useAppName } from "../../brand";
import { LANGS, useTranslation } from "../../i18n";
import { useTheme, toggleTheme } from "../../theme";
import { Login } from "../../shell/Login";
import { MENU_ITEM, MENU_POPUP } from "../../shell/menuStyles";
import { translateNavItemLabel } from "../../shell/navI18n";
import { menuIcon } from "../../shell/areas";
import { Button, GlassCard, RouteFallback, cn } from "../../ui";
import { useInstanceReadiness } from "./shared";
import { ConsoleHome } from "./home";

// Each migrated panel stays its own chunk, loaded when its route opens.
const InstanceSettings = lazy(() => import("./settings").then((m) => ({ default: m.InstanceSettings })));
const AuthenticationSettings = lazy(() => import("./auth").then((m) => ({ default: m.AuthenticationSettings })));
const InstancePluginsSettings = lazy(() => import("./plugins").then((m) => ({ default: m.InstancePluginsSettings })));

// Rail items are the console's own navigation — frontend-declared, no menu
// pipeline (the console has no tenant sidebar). Labels localize via the same
// nav.<id> mechanism as every other sidebar entry; `label` is the fallback.
const RAIL: { id: string; label: string; path: string; Icon: React.ElementType }[] = [
  { id: "console-overview", label: "Overview", path: "/", Icon: LayoutDashboard },
  { id: "console-settings", label: "Instance Settings", path: "/settings", Icon: Server },
  { id: "console-auth", label: "Authentication", path: "/auth", Icon: KeyRound },
  { id: "console-plugins", label: "Installed plugins", path: "/plugins", Icon: Puzzle },
];

export default function InstanceConsole() {
  const { t } = useTranslation();
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [isAdmin, setIsAdmin] = useState(false);

  const check = useCallback(() => {
    api.me()
      .then((m) => { setAuthed(true); setIsAdmin(!!m.isInstanceAdmin); })
      .catch(() => setAuthed(false));
  }, []);

  useEffect(() => { check(); }, [check]);

  if (authed === null) {
    return (
      <div className="octarq-aurora grid h-full place-items-center text-muted-foreground">
        <div className="flex flex-col items-center gap-3">
          <BrandMark size="md" />
          <span className="text-sm">{t("instance.loading")}</span>
        </div>
      </div>
    );
  }
  if (!authed) {
    return (
      <div className="octarq-aurora h-full">
        <Login onLogin={check} />
      </div>
    );
  }
  if (!isAdmin) return <AccessDenied />;
  return <ConsoleShell />;
}

// Neutral, feature-free notice: no hint that instance administration exists,
// just "not for this account" and the way back. The server is authoritative.
function AccessDenied() {
  const { t } = useTranslation();
  return (
    <div className="octarq-aurora grid h-full place-items-center p-4">
      <GlassCard className="max-w-md px-5 py-10 text-center">
        <h1 className="font-display text-xl font-bold text-foreground">{t("instance.accessDeniedTitle")}</h1>
        <p className="mt-1.5 text-xs text-muted-foreground">{t("instance.accessDeniedDesc")}</p>
        <a
          href="/admin"
          className="mt-5 inline-flex items-center gap-1.5 text-xs font-medium text-accent-fg hover:text-accent-fg/80"
        >
          <ArrowLeft className="h-3.5 w-3.5" strokeWidth={1.75} />
          {t("instance.backToDashboard")}
        </a>
      </GlassCard>
    </div>
  );
}

function ConsoleShell() {
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();
  const appName = useAppName();
  const [build, setBuild] = useState<{ version: string; commit: string; builtAt: string } | null>(null);
  const { checks, failed, reload } = useInstanceReadiness();
  const [instanceMenus, setInstanceMenus] = useState<MenuItem[]>([]);

  useEffect(() => {
    api.instanceBuild().then(setBuild).catch(() => {});
  }, []);

  // The backend is the only source of truth for which instance pages exist in
  // this build: an entry survives only when /api/instance/menus announces it
  // AND the frontend registers an instanceRoutes entry for the same path
  // (mirroring the tenant sidebar merge in App.tsx). Announce-without-frontend
  // would point at a blank page; frontend-without-announce means the plugin is
  // disabled or not compiled in.
  const registeredInstanceRoutes = useMemo(() => uiInstanceRoutes(), []);
  const registeredInstancePaths = useMemo(
    () => new Set(registeredInstanceRoutes.map((r) => r.path)),
    [registeredInstanceRoutes],
  );
  const instanceRoutes = useMemo(
    () => registeredInstanceRoutes.filter((r) => instanceMenus.some((m) => m.path === r.path)),
    [registeredInstanceRoutes, instanceMenus],
  );
  const pluginRail = useMemo(
    () =>
      instanceMenus
        .filter((m) => registeredInstancePaths.has(m.path))
        .map((m) => ({
          id: m.id,
          label: m.label,
          path: m.path,
          Icon: menuIcon(m.icon) ?? Puzzle,
        })),
    [instanceMenus, registeredInstancePaths],
  );

  useEffect(() => {
    api.instanceMenus().then(setInstanceMenus).catch(() => setInstanceMenus([]));
  }, []);

  const rail = [...RAIL, ...pluginRail];

  return (
    <div className="octarq-aurora flex h-screen w-full flex-col overflow-hidden text-foreground">
      {/* Header: instance identity + language/theme + the way back to /admin.
          No org switcher — the operator acts on the deployment, not a workspace. */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-border px-4">
        <div className="flex min-w-0 items-center gap-3">
          <BrandMark size="md" />
          <div className="min-w-0">
            <h1 className="truncate font-display text-[15px] font-bold leading-tight tracking-tight text-foreground">
              {t("instance.title")}
            </h1>
            <p className="truncate text-[11px] text-muted-foreground">{appName}</p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Menu.Root>
            <Menu.Trigger
              aria-label={t("common.language")}
              title={t("common.language")}
              className="flex h-9 items-center gap-1.5 rounded-xl px-2.5 text-xs font-semibold text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground data-[popup-open]:bg-surface-hover data-[popup-open]:text-foreground"
            >
              <Globe className="h-4 w-4 stroke-[1.75]" />
              <span className="uppercase">{lang}</span>
            </Menu.Trigger>
            <Menu.Portal>
              <Menu.Positioner side="bottom" align="end" sideOffset={8} className="z-50 outline-none">
                <Menu.Popup className={cn(MENU_POPUP, "w-40")}>
                  <Menu.RadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
                    <div className="space-y-0.5">
                      {LANGS.map((l) => (
                        <Menu.RadioItem
                          key={l.code}
                          value={l.code}
                          closeOnClick={true}
                          className="flex items-center justify-between rounded-lg px-2.5 py-1.5 text-xs font-medium text-muted-foreground outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground data-[checked]:bg-primary/10 data-[checked]:text-primary data-[checked]:font-bold"
                        >
                          <span>{l.label}</span>
                        </Menu.RadioItem>
                      ))}
                    </div>
                  </Menu.RadioGroup>
                </Menu.Popup>
              </Menu.Positioner>
            </Menu.Portal>
          </Menu.Root>

          <button
            onClick={toggleTheme}
            aria-label={theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
            title={theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
            className="flex h-9 w-9 items-center justify-center rounded-xl text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
          >
            {theme === "dark" ? <Sun className="h-5 w-5" strokeWidth={1.75} /> : <Moon className="h-5 w-5" strokeWidth={1.75} />}
          </button>

          <a
            href="/admin"
            className="flex h-9 items-center gap-1.5 rounded-xl px-3 text-xs font-semibold text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
          >
            <ArrowLeft className="h-4 w-4" strokeWidth={1.75} />
            {t("instance.backToDashboard")}
          </a>
        </div>
      </header>

      <div className="relative flex min-h-0 flex-1 overflow-hidden">
        <nav className="flex w-52 shrink-0 flex-col overflow-y-auto border-r border-border p-3">
          <div className="flex-1 space-y-0.5">
            {rail.map((item) => (
              <NavLink
                key={item.id}
                to={item.path}
                end={item.path === "/"}
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-[13px] transition-colors",
                    isActive
                      ? "bg-foreground/[0.06] font-medium text-foreground ring-1 ring-inset ring-border"
                      : "text-muted-foreground hover:bg-surface-hover hover:text-foreground",
                  )
                }
              >
                <item.Icon className={cn("h-[18px] w-[18px]", "shrink-0")} strokeWidth={1.75} />
                <span className="truncate">{translateNavItemLabel(t, item.id, item.label)}</span>
              </NavLink>
            ))}
          </div>
          <div className="border-t border-border px-2.5 pb-1 pt-3">
            {build && (
              <p className="font-mono tnum text-[10px] leading-relaxed text-foreground/40">
                {t("app.shellBuildFooter", {
                  version: build.version,
                  commit: build.commit ? (build.commit.length > 8 ? build.commit.slice(0, 8) : build.commit) : "unknown",
                  builtAt: build.builtAt,
                })}
              </p>
            )}
          </div>
        </nav>

        <main className="flex-1 overflow-y-auto [scrollbar-gutter:stable]">
          <div className="mx-auto w-full max-w-6xl px-4 py-4 sm:px-8 sm:py-5">
            <Suspense fallback={<RouteFallback />}>
              <Routes>
                <Route
                  path="/"
                  element={
                    checks === null ? (
                      failed ? <LoadFailedCard onRetry={reload} /> : <RouteFallback />
                    ) : (
                      <ConsoleHome checks={checks} onRefresh={reload} />
                    )
                  }
                />
                <Route path="/wizard" element={<Navigate to="/" replace />} />
                <Route path="/console" element={<Navigate to="/" replace />} />
                <Route path="/settings" element={<InstanceSettings />} />
                <Route path="/auth" element={<AuthenticationSettings />} />
                <Route path="/plugins" element={<InstancePluginsSettings />} />
                {instanceRoutes.map((route) => (
                  <Route key={route.path} path={route.path} element={<route.Component />} />
                ))}
                <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </Suspense>
          </div>
        </main>
      </div>
    </div>
  );
}

function LoadFailedCard({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <GlassCard className="p-8 text-center">
      <p className="text-sm text-foreground/80">{t("instance.loadFailed")}</p>
      <Button variant="outline" className="mt-4" onClick={onRetry}>
        {t("instance.retry")}
      </Button>
    </GlassCard>
  );
}
