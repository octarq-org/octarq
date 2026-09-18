import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { BookOpen, Bot, Boxes, FileText, Globe, Link2, Mail, Send, Server, Shield, Sparkles } from "lucide-react";
import { api, Org } from "./api";
import { BrandMark } from "./shell/BrandMark";
import { refreshBrand } from "./brand";
// Lazy-loaded route components.
const OverviewPage = lazy(() => import("./pages/Overview"));
const SettingsPage = lazy(() => import("./pages/Settings"));
const NotificationsPage = lazy(() => import("./pages/notifications"));
const InviteAcceptPage = lazy(() => import("./pages/InviteAccept"));
const ResetPasswordPage = lazy(() => import("./pages/ResetPassword"));
const StatusPage = lazy(() => import("./pages/Status"));
// The instance console (own /instance basename) — its own shell, no tenant
const InstanceConsole = lazy(() => import("./pages/instance/console"));
// The UI workbench. `import.meta.env.DEV` is statically false in a production
// build, so this is null there and the dynamic import below is dead code — Rollup
// emits no workbench chunk at all. A plain top-level import would ship it.
const DevWorkbench = import.meta.env.DEV ? lazy(() => import("./dev/workbench")) : null;
import { Modal, Button, toast, cn, Alert, RouteFallback, TableDensityProvider, TableDensity } from "./ui";
import { useTranslation } from "./i18n";
import { AreaId, useNavigation, clearCachedNav } from "./shell/areas";
import { RoleProvider } from "./shell/role";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/queryClient";
import { TopBar } from "./shell/TopBar";
import { CommandPalette } from "./shell/CommandPalette";
import { AreaPanel } from "./shell/AreaPanel";
import { ShellFooter } from "./shell/ShellFooter";
import { Login } from "./shell/Login";
import { uiOnboarding } from "./plugin-sdk";
import { pluginRouteElements, PluginUnavailable } from "./plugins/PluginRoutes";
import { PluginGateContext } from "./plugins/PluginGate";
import { InstanceExitRedirect } from "./pages/instance/redirect";


// ─── App ──────────────────────────────────────────────────────────────────────

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);
  // Advisory org role from /api/auth/me for sidebar and PluginGate gating.
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
      // ignore
    }
    return null;
  });

  useEffect(() => {
    api.me()
      .then((m) => { setUser(m.email || m.username || ""); setActiveOrgId(m.orgId); setRole(m.role); setAuthed(true); })
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
          } catch {}
        }
      })
      .catch(() => {
        if (onboardingCompleted === null) {
          setOnboardingCompleted(false);
        }
      });
  }, []);

  // Persist-and-apply, handed to the preferences UI through the same provider
  // that carries the density down — no second channel for one piece of state.
  const changeTableDensity = useCallback((d: TableDensity) => {
    setTableDensity(d);
    api.updateUserSettings("table_density", d).catch(() => {});
  }, []);

  let content;
  if (window.location.pathname === "/status" || window.location.pathname === "/status/") {
    content = <Suspense fallback={<RouteFallback />}><StatusPage /></Suspense>;
  } else if (window.location.pathname.startsWith("/instance")) {
    // Same pattern as /status: the browser is on the console's basename, so
    // the tenant shell must not boot. The console handles its own auth gate.
    content = <Suspense fallback={<RouteFallback />}><InstanceConsole /></Suspense>;
  } else if (window.location.pathname === "/license") {
    content = <InstanceExitRedirect to="/instance/license" />;
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
          setUser(u); setActiveOrgId(orgId); setAuthed(true);
          // The login response carries no role — refetch me for it.
          api.me().then((m) => setRole(m.role)).catch(() => {});
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
          try { await api.logout(); } catch { /* clear locally even if the request fails */ }
          // Clear cached nav to avoid showing previous user's sidebar.
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



// ─── Shell ────────────────────────────────────────────────────────────────────

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

  // Bumped on every workspace switch to remount the routed content, so each
  // page refetches for the new workspace — an in-app refresh that replaces the
  // old full-page window.location.reload().
  const [orgEpoch, setOrgEpoch] = useState(0);

  const [orgs, setOrgs] = useState<Org[]>([]);
  const [creatingOrg, setCreatingOrg] = useState(false);
  const [newOrgName, setNewOrgName] = useState("");
  const [isInstanceAdmin, setIsInstanceAdmin] = useState(false);
  const [emailVerified, setEmailVerified] = useState<boolean | undefined>(undefined);
  const [dismissedVerifyBanner, setDismissedVerifyBanner] = useState(false);
  const [resendingVerify, setResendingVerify] = useState(false);

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

  // Collapse the second-level area panel to widen the content area. Persisted,
  // and kept in the layout (not AreaPanel) so it survives area switches. On
  // narrow screens the rail is an overlay drawer, so it starts collapsed there
  // regardless of the stored preference.
  const [panelCollapsed, setPanelCollapsed] = useState(() => {
    try {
      if (typeof window !== "undefined" && window.innerWidth < 768) return true;
      return localStorage.getItem("area_panel_collapsed") === "1";
    } catch { return false; }
  });
  const togglePanel = () => setPanelCollapsed((v) => {
    const next = !v;
    try { localStorage.setItem("area_panel_collapsed", next ? "1" : "0"); } catch { /* ignore */ }
    return next;
  });

  // Track the mobile breakpoint so the collapsed rail is `inert` only when it's
  // the off-screen drawer (mobile), not the usable icon rail (desktop).
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== "undefined" && window.matchMedia("(max-width: 767px)").matches,
  );
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 767px)");
    const onChange = () => setIsMobile(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  // ⌘K / Ctrl-K command palette for primary navigation.
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

  // Role inputs for PluginGate requiredRole pre-check.
  const roleCtx = useMemo(() => ({ role, isInstanceAdmin }), [role, isInstanceAdmin]);

  // Load orgs + user settings layout.
  useEffect(() => {
    api.me().then((m) => { setRole(m.role); setEmailVerified(m.emailVerified); }).catch(() => {});
    api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));
    api.settings().then((s) => setIsInstanceAdmin(!!s.isInstanceAdmin)).catch(() => {});
  }, [activeOrgId]);

  // Settings pages that mutate the workspace list (rename) fire this instead of
  // reloading the page; refetch the orgs so the switcher/name update in place.
  useEffect(() => {
    const refreshOrgs = () => api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));
    const refreshPlugins = () => nav.refreshNav();
    const refreshAuth = () => {
      api.me().then((m) => {
        setRole(m.role);
        setEmailVerified(m.emailVerified);
      }).catch(() => {});
    };
    window.addEventListener("octarq:orgs-changed", refreshOrgs);
    window.addEventListener("octarq:plugins-changed", refreshPlugins);
    window.addEventListener("octarq:auth-changed", refreshAuth);
    return () => {
      window.removeEventListener("octarq:orgs-changed", refreshOrgs);
      window.removeEventListener("octarq:plugins-changed", refreshPlugins);
      window.removeEventListener("octarq:auth-changed", refreshAuth);
    };
  }, [nav]);

  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name ?? t("app.personalWorkspace");

  // Apply an active-workspace change in-app: point the shell at the new org
  // (its useEffect refetches menus/plugins/role/settings), remount the routed
  // content so every page reloads its data, and land on Overview.
  function switchToOrg(id: number) {
    setActiveOrgId(id);
    setOrgEpoch((e) => e + 1);
    // Branding is per workspace, and brand.tsx caches it module-wide. The epoch
    // bump remounts the routed content but not module state, so without this the
    // new workspace renders under the previous one's colours, name and logo.
    // (The full-page reload this replaced used to invalidate that cache for free.)
    void refreshBrand();
    navigate("/overview");
  }

  function handleCreateOrg(e: React.FormEvent) {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    api.createOrg({ name: newOrgName })
      .then((org) => api.switchOrg(org.id).then(() => {
        setCreatingOrg(false);
        setNewOrgName("");
        switchToOrg(org.id);
        toast.success(t("app.workspaceCreated"));
      }))
      .catch((e) => toast.error(e.message || t("app.createWorkspaceFailed")));
  }



  // Move focus to the main region after route changes so keyboard and
  // screen-reader users land on the new page rather than being stranded on a
  // now-unmounted control. Skip the initial mount (don't steal focus on load);
  // preventScroll keeps the viewport steady.
  const mainRef = useRef<HTMLElement>(null);
  const firstNav = useRef(true);
  useEffect(() => {
    if (firstNav.current) { firstNav.current = false; return; }
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);

  const appRoutes = (
    <Suspense fallback={<RouteFallback />}>
      <PluginGateContext.Provider value={pluginGateCtxValue}>
        <Routes>
          <Route path="/"           element={<Navigate to="/overview" replace />} />
          {/* The Pro license is instance state, so its page lives in the
              /instance console. Crossing basenames needs a full page load —
              a router <Navigate> would resolve to /admin/instance. */}
          <Route path="/license"       element={<InstanceExitRedirect to="/instance/license" />} />
          <Route path="/link-settings" element={<InstanceExitRedirect to="/instance/link-settings" />} />
          <Route path="/onboarding"    element={<Navigate to="/overview" replace />} />
          <Route path="/overview"   element={<OverviewPage />} />
          <Route path="/notifications/*" element={<NotificationsPage />} />
          <Route path="/settings/*" element={<SettingsPage />} />
          <Route path="/admin/invite/accept" element={<InviteAcceptPage />} />
          <Route path="/admin/reset" element={<ResetPasswordPage />} />
          {pluginRouteElements()}
          {DevWorkbench && <Route path="/_dev/workbench" element={<DevWorkbench />} />}
          <Route path="*"           element={<PluginUnavailable />} />
        </Routes>
      </PluginGateContext.Provider>
    </Suspense>
  );

  return (
    <QueryClientProvider client={queryClient}>
      <RoleProvider value={roleCtx}>
      <div className="octarq-aurora flex h-screen w-full flex-col overflow-hidden text-foreground">
      {/* Keyboard skip link — first focusable element, visually hidden until
          focused, jumps past the nav chrome straight to page content. */}
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

      {emailVerified === false && !dismissedVerifyBanner && (
        <Alert
          variant="warning"
          align="center"
          icon={<Mail className="h-4 w-4" />}
          onDismiss={() => setDismissedVerifyBanner(true)}
          className="rounded-none border-x-0 border-t-0 text-xs py-2 px-4 z-40"
          actions={
            <button
              onClick={async () => {
                setResendingVerify(true);
                try {
                  const res = await api.resendVerification(user);
                  if (res.mailConfigured === false) {
                    toast.error(t("app.verificationMailNotConfigured"));
                  } else {
                    toast.success(t("app.verificationSent"));
                  }
                } catch (e: any) {
                  toast.error(e.message || "Failed to send verification email.");
                } finally {
                  setResendingVerify(false);
                }
              }}
              disabled={resendingVerify}
              className="px-2.5 py-1 rounded-lg bg-warning-bg hover:brightness-95 border border-warning-border text-warning-fg font-medium transition-colors disabled:opacity-50 text-xs"
            >
              {resendingVerify ? t("app.sending") : t("app.resendVerificationBtn")}
            </button>
          }
        >
          {t("app.verifyEmailBanner")}
        </Alert>
      )}

      <div className="relative flex min-h-0 flex-1 overflow-hidden">
      {/* Mobile scrim — below md the rail overlays content, so a tap-away layer
          closes it. Only rendered when the drawer is open on a small screen. */}
      {!panelCollapsed && (
        <button
          aria-label={t("app.collapseMenu")}
          onClick={togglePanel}
          className="absolute inset-0 z-20 bg-black/50 backdrop-blur-sm md:hidden"
        />
      )}
      {/* Second-level nav rail. On md+ it's an inline column whose width
          animates between a 64px icon rail (collapsed) and 240px (expanded) —
          it never disappears, so navigation stays reachable. Below md it's an
          absolute overlay drawer that slides off-screen when collapsed. */}
      <aside
        className={cn(
          "z-30 shrink-0 overflow-hidden transition-[width,transform] duration-300 ease-out",
          "absolute inset-y-0 left-0 w-60 md:relative md:inset-auto md:translate-x-0",
          panelCollapsed ? "max-md:-translate-x-full md:w-16" : "md:w-60",
        )}
        // On mobile the collapsed drawer is off-screen — `inert` drops its links
        // out of the tab order and the AT tree. On desktop the collapsed rail is
        // a usable icon strip, so it stays interactive.
        {...(panelCollapsed && isMobile ? { inert: "" } : {})}
      >
        <AreaPanel
          area={currentArea}
          currentPath={location.pathname + location.search}
          collapsed={panelCollapsed}
          onToggle={togglePanel}
          onNavigate={() => { if (window.innerWidth < 768) setPanelCollapsed(true); }}
          footerItems={footerItems}
          showWorkspaceSwitcher={isProBuild}
          orgs={orgs}
          activeOrgId={activeOrgId}
          activeOrgName={activeOrgName}
          onSwitchOrg={(id) =>
            api.switchOrg(id)
              .then(() => switchToOrg(id))
              .catch((e) => toast.error(e.message || t("app.switchWorkspaceFailed")))
          }
          onCreateOrg={() => setCreatingOrg(true)}
        />
      </aside>

      <main ref={mainRef} id="main-content" tabIndex={-1} className="relative flex-1 overflow-hidden outline-none">
        {helpActive ? (
          <div key={orgEpoch} className="h-full w-full overflow-hidden">
            {appRoutes}
          </div>
        ) : (
          <div className="h-full overflow-y-auto [scrollbar-gutter:stable]">
            <div key={orgEpoch} className="mx-auto w-full max-w-6xl px-4 py-4 sm:px-8 sm:py-5">
              {appRoutes}
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
        onNavigate={(path) => { navigate(path); setCmdOpen(false); }}
        actions={filteredActions}
      />

      {creatingOrg && (
        <Modal title={t("app.createWorkspace")} onClose={() => setCreatingOrg(false)}>
          <form onSubmit={handleCreateOrg} className="space-y-4">
            <div className="space-y-1.5">
              <label className="label">{t("app.workspaceName")}</label>
              <input
                className="input w-full"
                value={newOrgName}
                onChange={(e) => setNewOrgName(e.target.value)}
                placeholder={t("app.workspaceNamePlaceholder")}
                autoFocus
              />
            </div>
            <div className="flex justify-end gap-2.5 pt-4 border-t border-border">
              <Button type="button" variant="ghost" onClick={() => setCreatingOrg(false)}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="primary" disabled={!newOrgName.trim()}>
                {t("app.createAndSwitch")}
              </Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
      </RoleProvider>
    </QueryClientProvider>
  );
}
