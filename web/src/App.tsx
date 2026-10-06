import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { api, type Org } from "./api";
import { BrandMark } from "./shell/BrandMark";
import { refreshBrand } from "./brand";
import { cn, RouteFallback, TableDensity, TableDensityProvider, toast } from "@octarq/plugin-sdk";
import { useTranslation } from "./i18n";
import { clearCachedNav, useNavigation } from "./shell/areas";
import { RoleProvider } from "./shell/role";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/queryClient";
import { TopBar } from "./shell/TopBar";
import { CommandPalette } from "./shell/CommandPalette";
import { AreaPanel } from "./shell/AreaPanel";
import { ShellFooter } from "./shell/ShellFooter";
import { Login } from "./shell/Login";
import { uiOnboarding } from "./plugin-sdk";
import { AppRoutes } from "./shell/AppRoutes";
import { EmailVerifyBanner } from "./shell/EmailVerifyBanner";
import { CreateOrgModal } from "./shell/CreateOrgModal";
import { useShellEvents } from "./shell/useShellEvents";
import { InstanceExitRedirect } from "./pages/instance/redirect";

const InviteAcceptPage = lazy(() => import("./pages/InviteAccept"));
const ResetPasswordPage = lazy(() => import("./pages/ResetPassword"));
const StatusPage = lazy(() => import("./pages/Status"));
const InstanceConsole = lazy(() => import("./pages/instance/console"));

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);
  const [role, setRole] = useState<string | undefined>(undefined);
  const [tableDensity, setTableDensity] = useState<TableDensity>("comfortable");
  const [onboardingCompleted, setOnboardingCompleted] = useState<boolean | null>(() => {
    try {
      if (
        localStorage.getItem("onboarding_completed") === "true" ||
        localStorage.getItem("octarq:onboarding:completed") === "true"
      ) {
        return true;
      }
    } catch {
      /* ignore */
    }
    return null;
  });

  useEffect(() => {
    api.me()
      .then((m) => {
        setUser(m.email || m.username || "");
        setActiveOrgId(m.orgId);
        setRole(m.role);
        setAuthed(true);
      })
      .catch(() => setAuthed(false));

    api.getUserSettings()
      .then((s) => {
        if (s?.table_density === "compact" || s?.table_density === "comfortable") {
          setTableDensity(s.table_density as TableDensity);
        }
        const isDone = s?.onboarding_completed === "true" || s?.onboarding_dismissed === "true";
        setOnboardingCompleted(isDone);
        if (isDone) {
          try {
            localStorage.setItem("onboarding_completed", "true");
          } catch {
            /* ignore */
          }
        }
      })
      .catch(() => {
        if (onboardingCompleted === null) {
          setOnboardingCompleted(false);
        }
      });
  }, [onboardingCompleted]);

  const changeTableDensity = useCallback((d: TableDensity) => {
    setTableDensity(d);
    api.updateUserSettings("table_density", d).catch((err) => {
      if (import.meta.env.DEV) console.warn("[app] Failed to update table density:", err);
    });
  }, []);

  let content;
  if (window.location.pathname === "/status" || window.location.pathname === "/status/") {
    content = <Suspense fallback={<RouteFallback />}><StatusPage /></Suspense>;
  } else if (window.location.pathname.startsWith("/instance")) {
    content = <Suspense fallback={<RouteFallback />}><InstanceConsole /></Suspense>;
  } else if (window.location.pathname === "/link-settings") {
    content = <InstanceExitRedirect to="/instance/link-settings" />;
  } else if (window.location.pathname === "/admin/invite/accept") {
    content = <InviteAcceptPage />;
  } else if (window.location.pathname === "/admin/reset") {
    content = <ResetPasswordPage />;
  } else if (authed === null) {
    content = (
      <div className="octarq-aurora grid h-full place-items-center text-muted-foreground">
        <div className="flex flex-col items-center gap-3">
          <BrandMark size="md" />
          <span className="text-sm">loading…</span>
        </div>
      </div>
    );
  } else if (!authed) {
    content = (
      <Login
        onLogin={(u, orgId) => {
          setUser(u);
          setActiveOrgId(orgId);
          setAuthed(true);
          api.me()
            .then((m) => setRole(m.role))
            .catch((err) => {
              if (import.meta.env.DEV) console.warn("[app] Failed to fetch me after login:", err);
            });
          api.getUserSettings()
            .then((s) => {
              const isDone = s?.onboarding_completed === "true" || s?.onboarding_dismissed === "true";
              setOnboardingCompleted(isDone);
            })
            .catch(() => setOnboardingCompleted(false));
        }}
      />
    );
  } else if (
    uiOnboarding() &&
    (onboardingCompleted === false || window.location.pathname === "/onboarding")
  ) {
    const OnboardingFlow = uiOnboarding()!.Component;
    content = (
      <Suspense fallback={<RouteFallback />}>
        <OnboardingFlow
          onComplete={() => {
            setOnboardingCompleted(true);
          }}
        />
      </Suspense>
    );
  } else {
    content = (
      <Shell
        user={user}
        role={role}
        setRole={setRole}
        activeOrgId={activeOrgId}
        setActiveOrgId={setActiveOrgId}
        onLogout={async () => {
          try {
            await api.logout();
          } catch {
            /* clear locally even if the request fails */
          }
          clearCachedNav();
          setAuthed(false);
        }}
      />
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <TableDensityProvider density={tableDensity} onDensityChange={changeTableDensity}>
        {content}
      </TableDensityProvider>
    </QueryClientProvider>
  );
}

function Shell({
  user,
  role,
  setRole,
  activeOrgId,
  setActiveOrgId,
  onLogout,
}: {
  user: string;
  role?: string;
  setRole: (role: string | undefined) => void;
  activeOrgId: number;
  setActiveOrgId: (id: number) => void;
  onLogout: () => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const { t, lang } = useTranslation();

  const [orgEpoch, setOrgEpoch] = useState(0);
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [creatingOrg, setCreatingOrg] = useState(false);
  const [newOrgName, setNewOrgName] = useState("");
  const [isInstanceAdmin, setIsInstanceAdmin] = useState(false);
  const [emailVerified, setEmailVerified] = useState<boolean | undefined>(undefined);
  const [dismissedVerifyBanner, setDismissedVerifyBanner] = useState(false);

  const nav = useNavigation({
    role,
    isInstanceAdmin,
    activeOrgId,
    lang,
  });

  const {
    areas,
    activeArea,
    currentArea,
    currentSettingsArea,
    footerItems,
    filteredActions,
    pluginGateCtxValue,
    settingsActive,
    helpActive,
    isProBuild,
    selectArea,
    helpDocsNav,
    helpArea,
  } = nav;

  const [panelCollapsed, setPanelCollapsed] = useState(() => {
    try {
      if (typeof window !== "undefined" && window.innerWidth < 768) return true;
      return localStorage.getItem("area_panel_collapsed") === "1";
    } catch {
      return false;
    }
  });

  const togglePanel = () =>
    setPanelCollapsed((v) => {
      const next = !v;
      try {
        localStorage.setItem("area_panel_collapsed", next ? "1" : "0");
      } catch {
        /* ignore */
      }
      return next;
    });

  const [isMobile, setIsMobile] = useState(
    () => typeof window !== "undefined" && window.matchMedia("(max-width: 767px)").matches,
  );

  useEffect(() => {
    const mq = window.matchMedia("(max-width: 767px)");
    const onChange = () => setIsMobile(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const [cmdOpen, setCmdOpen] = useState(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCmdOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const roleCtx = useMemo(() => ({ role, isInstanceAdmin }), [role, isInstanceAdmin]);

  useEffect(() => {
    api
      .me()
      .then((m) => {
        setRole(m.role);
        setEmailVerified(m.emailVerified);
      })
      .catch((err) => {
        if (import.meta.env.DEV) console.warn("[shell] Failed to refresh user profile:", err);
      });

    api
      .orgs()
      .then((os) => setOrgs(os))
      .catch((err) => {
        if (import.meta.env.DEV) console.warn("[shell] Failed to fetch orgs:", err);
        setOrgs([]);
      });

    api
      .settings()
      .then((s) => setIsInstanceAdmin(!!s.isInstanceAdmin))
      .catch((err) => {
        if (import.meta.env.DEV) console.warn("[shell] Failed to fetch settings:", err);
      });
  }, [activeOrgId, setRole]);

  useShellEvents({
    setOrgs: (fn) => setOrgs(fn),
    refreshNav: () => nav.refreshNav(),
    setRole,
    setEmailVerified,
  });

  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name ?? t("app.personalWorkspace");

  function switchToOrg(id: number) {
    setActiveOrgId(id);
    setOrgEpoch((e) => e + 1);
    void refreshBrand();
    navigate("/overview");
  }

  function handleCreateOrg(e: React.FormEvent) {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    api
      .createOrg({ name: newOrgName })
      .then((org) =>
        api.switchOrg(org.id).then(() => {
          setCreatingOrg(false);
          setNewOrgName("");
          switchToOrg(org.id);
          toast.success(t("app.workspaceCreated"));
        }),
      )
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : t("app.createWorkspaceFailed");
        toast.error(msg);
      });
  }

  const mainRef = useRef<HTMLElement>(null);
  const firstNav = useRef(true);
  useEffect(() => {
    if (firstNav.current) {
      firstNav.current = false;
      return;
    }
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);

  return (
    <RoleProvider value={roleCtx}>
      <div className="octarq-aurora flex h-screen w-full flex-col overflow-hidden text-foreground">
        <a
          href="#main-content"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-3 focus:z-[60] focus:rounded-xl focus:bg-primary focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-primary-foreground"
        >
          {t("app.skipToContent")}
        </a>
        <TopBar
          areas={areas}
          activeArea={activeArea}
          settingsActive={settingsActive}
          user={user}
          panelCollapsed={panelCollapsed}
          onTogglePanel={togglePanel}
          onSelectArea={selectArea}
          onOpenSettings={() => navigate("/settings")}
          onOpenCommand={() => setCmdOpen(true)}
          onLogout={onLogout}
          actions={filteredActions}
        />

        <EmailVerifyBanner
          user={user}
          emailVerified={emailVerified}
          dismissed={dismissedVerifyBanner}
          onDismiss={() => setDismissedVerifyBanner(true)}
        />

        <div className="relative flex min-h-0 flex-1 overflow-hidden">
          {!panelCollapsed && (
            <button
              aria-label={t("app.collapseMenu")}
              onClick={togglePanel}
              className="absolute inset-0 z-20 bg-black/50 backdrop-blur-sm md:hidden"
            />
          )}
          <aside
            className={cn(
              "z-30 shrink-0 overflow-hidden transition-[width,transform] duration-300 ease-out",
              "absolute inset-y-0 left-0 w-60 md:relative md:inset-auto md:translate-x-0",
              panelCollapsed ? "max-md:-translate-x-full md:w-16" : "md:w-60",
            )}
            {...(panelCollapsed && isMobile ? { inert: "" } : {})}
          >
            <AreaPanel
              area={currentArea}
              currentPath={location.pathname + location.search}
              collapsed={panelCollapsed}
              onToggle={togglePanel}
              onNavigate={() => {
                if (window.innerWidth < 768) setPanelCollapsed(true);
              }}
              footerItems={footerItems}
              showWorkspaceSwitcher={isProBuild}
              orgs={orgs}
              activeOrgId={activeOrgId}
              activeOrgName={activeOrgName}
              onSwitchOrg={(id) =>
                api
                  .switchOrg(id)
                  .then(() => switchToOrg(id))
                  .catch((err: unknown) => {
                    const msg = err instanceof Error ? err.message : t("app.switchWorkspaceFailed");
                    toast.error(msg);
                  })
              }
              onCreateOrg={() => setCreatingOrg(true)}
            />
          </aside>

          <main ref={mainRef} id="main-content" tabIndex={-1} className="relative flex-1 overflow-hidden outline-none">
            {helpActive ? (
              <div key={orgEpoch} className="h-full w-full overflow-hidden">
                <AppRoutes pluginGateCtxValue={pluginGateCtxValue} />
              </div>
            ) : (
              <div className="h-full overflow-y-auto [scrollbar-gutter:stable]">
                <div key={orgEpoch} className="mx-auto w-full max-w-6xl px-4 py-4 sm:px-8 sm:py-5">
                  <AppRoutes pluginGateCtxValue={pluginGateCtxValue} />
                  <ShellFooter />
                </div>
              </div>
            )}
          </main>
        </div>

        <CommandPalette
          open={cmdOpen}
          onClose={() => setCmdOpen(false)}
          areas={helpDocsNav.length > 0 ? [...areas, helpArea] : areas}
          settingsArea={currentSettingsArea}
          onNavigate={(path) => {
            navigate(path);
            setCmdOpen(false);
          }}
          actions={filteredActions}
        />

        <CreateOrgModal
          open={creatingOrg}
          newOrgName={newOrgName}
          onNewOrgNameChange={setNewOrgName}
          onSubmit={handleCreateOrg}
          onClose={() => setCreatingOrg(false)}
        />
      </div>
    </RoleProvider>
  );
}
