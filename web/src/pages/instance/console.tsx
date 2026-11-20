import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { Navigate, NavLink, Route, Routes } from "react-router-dom";
import { Menu } from "@base-ui/react/menu";
import { ArrowLeft, Globe, KeyRound, LayoutDashboard, Moon, Puzzle, Server, Sun } from "lucide-react";
import { uiInstanceRoutes, Button, PageCard, RouteFallback, cn } from "@octarq/plugin-sdk";
import { api, type MenuItem } from "../../api";
import { BrandMark } from "../../shell/BrandMark";
import { useAppName } from "../../brand";
import { LANGS, useTranslation } from "../../i18n";
import { useTheme, toggleTheme } from "../../theme";
import { Login } from "../../shell/Login";
import { MENU_ITEM, MENU_POPUP } from "../../shell/menuStyles";
import { translateNavItemLabel } from "../../shell/navI18n";
import { menuIcon } from "../../shell/areas";
import { useInstanceReadiness } from "./shared";
import { ConsoleHome } from "./home";

const InstanceSettings = lazy(() => import("./settings").then((m) => ({ default: m.InstanceSettings })));
const AuthenticationSettings = lazy(() => import("./auth").then((m) => ({ default: m.AuthenticationSettings })));
const InstancePluginsSettings = lazy(() => import("./plugins").then((m) => ({ default: m.InstancePluginsSettings })));

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
      <div className="grid h-full place-items-center bg-sidebar text-muted-foreground">
        <div className="flex flex-col items-center gap-3">
          <BrandMark size="md" />
          <span className="text-sm">{t("instance.loading")}</span>
        </div>
      </div>
    );
  }
  if (!authed) {
    return (
      <div className="h-full bg-sidebar">
        <Login onLogin={check} />
      </div>
    );
  }
  if (!isAdmin) return <AccessDenied />;
  return <ConsoleShell />;
}

function AccessDenied() {
  const { t } = useTranslation();
  return (
    <div className="grid h-full place-items-center bg-sidebar p-4">
      <PageCard className="mt-0 max-w-md px-5 py-10 text-center">
        <h1 className="text-xl font-semibold tracking-tight text-foreground">{t("instance.accessDeniedTitle")}</h1>
        <p className="mt-1.5 text-xs text-muted-foreground">{t("instance.accessDeniedDesc")}</p>
        <a
          href="/admin"
          className="mt-5 inline-flex items-center gap-1.5 text-xs font-medium text-accent-fg hover:text-accent-fg/80"
        >
          <ArrowLeft className="h-3.5 w-3.5" strokeWidth={1.75} />
          {t("instance.backToDashboard")}
        </a>
      </PageCard>
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
    <div className="flex h-dvh w-full overflow-hidden bg-sidebar text-foreground">
      <nav className="flex w-56 shrink-0 flex-col overflow-y-auto p-3">
        <div className="mb-3 flex items-center gap-2.5 px-1.5 py-1">
          <BrandMark size="sm" className="!h-7 !w-7 !rounded-md" />
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold leading-tight tracking-tight text-foreground">
              {t("instance.title")}
            </p>
            <p className="truncate text-[11px] text-muted-foreground">{appName}</p>
          </div>
        </div>
        <div className="flex-1 space-y-0.5">
          {rail.map((item) => (
            <NavLink
              key={item.id}
              to={item.path}
              end={item.path === "/"}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[13px] transition-colors duration-100",
                  isActive
                    ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                    : "text-muted-foreground hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground",
                )
              }
            >
              <item.Icon className="h-4 w-4 shrink-0" strokeWidth={1.75} />
              <span className="truncate">{translateNavItemLabel(t, item.id, item.label)}</span>
            </NavLink>
          ))}
        </div>
        <div className="border-t border-sidebar-border px-2 pb-1 pt-3">
          {build && (
            <p className="font-mono tnum text-[10px] leading-relaxed text-muted-foreground">
              {t("app.shellBuildFooter", {
                version: build.version,
                commit: build.commit ? (build.commit.length > 8 ? build.commit.slice(0, 8) : build.commit) : "unknown",
                builtAt: build.builtAt,
              })}
            </p>
          )}
        </div>
      </nav>

      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-background md:my-2 md:mr-2 md:rounded-xl md:border md:border-sidebar-border">
        <header className="flex h-12 shrink-0 items-center justify-between border-b border-border px-4">
          <span className="text-[13px] font-medium text-foreground">{t("instance.title")}</span>
          <div className="flex shrink-0 items-center gap-1.5">
            <Menu.Root>
              <Menu.Trigger
                aria-label={t("common.language")}
                title={t("common.language")}
                className="flex h-8 items-center gap-1.5 rounded-md border border-border bg-card px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground data-[popup-open]:bg-surface-hover data-[popup-open]:text-foreground"
              >
                <Globe className="h-3.5 w-3.5 stroke-[1.75]" />
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
                            className="flex items-center justify-between rounded-md px-2.5 py-1.5 text-xs font-medium text-muted-foreground outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground data-[checked]:bg-accent-soft data-[checked]:text-accent-fg data-[checked]:font-semibold"
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
              className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
            >
              {theme === "dark" ? <Sun className="h-4 w-4" strokeWidth={1.75} /> : <Moon className="h-4 w-4" strokeWidth={1.75} />}
            </button>

            <a
              href="/admin"
              className="flex h-8 items-center gap-1.5 rounded-md px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
            >
              <ArrowLeft className="h-3.5 w-3.5" strokeWidth={1.75} />
              {t("instance.backToDashboard")}
            </a>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto [scrollbar-gutter:stable]">
          <div className="px-4 py-6 sm:px-8 sm:py-8">
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
    <PageCard className="mt-0 p-8 text-center">
      <p className="text-sm text-foreground">{t("instance.loadFailed")}</p>
      <Button variant="outline" className="mt-4" onClick={onRetry}>
        {t("instance.retry")}
      </Button>
    </PageCard>
  );
}
