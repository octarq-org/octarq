import { lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { Globe, Mail } from "lucide-react";
import { api, MenuItem, Org, PluginInfo } from "./api";
import { BrandMark } from "./shell/BrandMark";
// Route-level code splitting: each top-level page ships as its own chunk,
// loaded on first navigation behind the Suspense boundary below.
const OverviewPage = lazy(() => import("./pages/Overview"));
const SettingsPage = lazy(() => import("./pages/Settings"));
const InviteAcceptPage = lazy(() => import("./pages/InviteAccept"));
const ResetPasswordPage = lazy(() => import("./pages/ResetPassword"));
const StatusPage = lazy(() => import("./pages/Status"));
import { Modal, Button, toast, cn, Alert } from "./ui";
import { useTranslation } from "./i18n";
import { Area, AreaId, NavItem, STATIC_AREAS, SETTINGS_AREA, FOOTER_PLACEMENT, areaForPath, areaForCategory, menuIcon, pluginAreaToArea } from "./shell/areas";
import { RoleProvider, roleSatisfies } from "./shell/role";
import { TopBar } from "./shell/TopBar";
import { CommandPalette } from "./shell/CommandPalette";
import { AreaPanel } from "./shell/AreaPanel";
import { Login } from "./shell/Login";
import { uiAreas } from "./plugin-sdk";
import { pluginRouteElements, PluginUnavailable } from "./plugins/PluginRoutes";
import { PluginGateContext } from "./plugins/PluginGate";


// Fallback while a route's lazily-loaded chunk is fetched — a subtle centered
// spinner instead of a blank gap. Shared with the Settings sub-router. The spin
// animation degrades under the global prefers-reduced-motion rule.
export function RouteFallback() {
  return (
    <div className="grid h-64 place-items-center" role="status" aria-live="polite">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-foreground/15 border-t-foreground/60" />
    </div>
  );
}


// ─── App ──────────────────────────────────────────────────────────────────────

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);
  // Org role from /api/auth/me ("owner" | "admin" | "member") — advisory input
  // for requiredRole gating (sidebar filter + PluginGate pre-check). UX only.
  const [role, setRole] = useState<string | undefined>(undefined);

  useEffect(() => {
    api.me()
      .then((m) => { setUser(m.email || m.username || ""); setActiveOrgId(m.orgId); setRole(m.role); setAuthed(true); })
      .catch(() => setAuthed(false));
  }, []);

  let content;
  if (window.location.pathname === "/status" || window.location.pathname === "/status/") {
    content = <Suspense fallback={<RouteFallback />}><StatusPage /></Suspense>;
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
        }}
      />
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
          // The nav cache is per-session by nature — it holds whatever the last
          // signed-in user could see. Left behind, it paints their sidebar for
          // the next person to reach the login screen on this browser.
          clearCachedNav();
          setAuthed(false);
        }}
      />
    );
  }

  return (
    <>
      {content}
    </>
  );
}

// ─── Nav cache ────────────────────────────────────────────────────────────────

// The sidebar used to paint instantly from a build-time list of frontend menu
// entries. That list was a hand-maintained copy of what the Go plugins already
// declare, and the two drifted. Caching the last api.menus()/api.plugins()
// answer gives the same instant first paint from one source — and a strictly
// better one: it reflects the plugins THIS workspace has enabled, where a
// static list showed entries for features the workspace had turned off until
// the API contradicted it.
//
// Treated as a rendering hint, never as truth: it is replaced the moment the
// live answer lands, and any parse failure falls back to empty.
const NAV_CACHE_KEY = "octarq:nav-cache:v1";

interface CachedNav {
  menus: MenuItem[];
  plugins: PluginInfo[];
}

function readCachedNav(): CachedNav {
  try {
    const raw = localStorage.getItem(NAV_CACHE_KEY);
    if (!raw) return { menus: [], plugins: [] };
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed?.menus) || !Array.isArray(parsed?.plugins)) {
      return { menus: [], plugins: [] };
    }
    return { menus: parsed.menus, plugins: parsed.plugins };
  } catch {
    return { menus: [], plugins: [] };
  }
}

function writeCachedNav(nav: CachedNav) {
  try {
    localStorage.setItem(NAV_CACHE_KEY, JSON.stringify(nav));
  } catch {
    /* quota or private mode — the cache is optional, the fetch is not */
  }
}

function clearCachedNav() {
  try {
    localStorage.removeItem(NAV_CACHE_KEY);
  } catch {
    /* nothing to do — a stale cache is replaced on the next successful fetch */
  }
}

// ─── Sidebar merge ────────────────────────────────────────────────────────────

// Merge every menu source into the final area list — ONE pipeline:
//   STATIC_AREAS      area/group shells (labels + order, no items);
//   backendMenus      every menu, from api.menus() — core and plugin alike;
// each item is routed to an area by the shared areaForCategory and into the
// group whose label matches its category. The first render is fed from the
// cached copy of the last api.menus() response (see NAV_CACHE_KEY) so the
// sidebar paints immediately without a fetch round-trip; the live answer
// replaces it a moment later.
// `role`/`isInstanceAdmin` drive the requiredRole filter: menu entries whose
// advisory requiredRole the current user doesn't meet are dropped here — the
// single place — so the sidebar AND the command palette (both fed by the
// resulting areas) agree. Ranking lives in roleSatisfies (shell/role.tsx),
// shared with PluginGate's route pre-check.
function mergeAreas(
  backendMenus: MenuItem[],
  plugins: PluginInfo[],
  role: string | undefined,
  isInstanceAdmin: boolean,
  backendLoaded: boolean,
): { areas: Area[]; footer: NavItem[] } {
  // Backend-driven gating: the set of paths the backend vouches for — every
  // menu it announces in api.menus() (active core + active plugin menus) PLUS
  // every path owned by a toggleable feature in api.plugins() (so a plugin
  // that's merely DISABLED, not absent, still counts as backed and is hidden by
  // disabledPaths below rather than dropped outright).
  const backendPaths = new Set<string>();
  for (const m of backendMenus) backendPaths.add(m.path);
  for (const p of plugins) for (const m of p.menus) backendPaths.add(m.path);

  // The backend is the only menu source. Frontend plugins contribute routes,
  // widgets and i18n; placement (label/category/icon/order) belongs to the Go
  // half's MenuProvider, which has to declare the path anyway for the gating
  // above. Nothing to merge, only to dedupe — a plugin could announce a path
  // twice across menus and its feature entry.
  const seenPaths = new Set<string>();
  const menus = backendMenus.filter((m) => {
    if (seenPaths.has(m.path)) return false;
    seenPaths.add(m.path);
    return true;
  });

  // Paths owned by a disabled Go plugin are hidden from the sidebar. Dynamic
  // plugin menus are already filtered server-side; this also drops statically
  // composed frontend items (core or Pro) whose backend half is toggled off.
  const disabledPaths = new Set(
    plugins.filter((p) => !p.enabled).flatMap((p) => p.menus.map((m) => m.path)),
  );

  // Top-level areas: the static ones plus any NEW areas declared by composed
  // frontend plugins (UIPlugin.areas → uiAreas()). A plugin area may carry
  // ordered group shells (UIArea.groups → pluginAreaToArea) — e.g. the Pro
  // Commerce area's Sales/Billing/Finance — or none, in which case its groups
  // are synthesized from menus by the category-merge below. Still-empty groups
  // and areas are dropped by the empty-area filter at the end. "settings" and
  // ids colliding with a static area can't be redeclared.
  const pluginAreas = uiAreas().filter(
    (pa) => pa.id !== "settings" && !STATIC_AREAS.some((sa) => sa.id === pa.id),
  );
  // SETTINGS_AREA joins the merge so plugin/backend menus categorized for
  // settings ("Instance"/"Account"/"Settings" → areaForCategory) land in its
  // groups — e.g. the Pro licensing plugin's octarq License in the Instance
  // group. The shell pulls the "settings" area back out of the result (it's the
  // gear, never a top-level tab) and applies the admin gate on the Instance
  // group; see `mergedSettingsArea` in Shell.
  const baseAreas = [...STATIC_AREAS, SETTINGS_AREA, ...pluginAreas.map(pluginAreaToArea)];

  const staticPaths = new Set(baseAreas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path))));
  const extras = menus.filter(
    (m) =>
      !staticPaths.has(m.path) &&
      !disabledPaths.has(m.path) &&
      roleSatisfies(m.requiredRole, role, isInstanceAdmin),
  );

  const toNavItem = (m: MenuItem): NavItem & { order: number } => {
    const KeyIcon = menuIcon(m.icon);
    return {
      id: m.id,
      label: m.label,
      Icon: KeyIcon ?? Globe,
      iconStr: KeyIcon ? undefined : m.icon,
      path: m.path,
      order: m.order ?? 0,
    };
  };

  // Footer-placed items (category "footer"/"resources" → FOOTER_PLACEMENT) are
  // pulled out of the area merge and returned separately for the rail footer.
  // They never match a real area id below, so they're naturally excluded there.
  const footer = extras
    .filter((m) => areaForCategory(m.category, pluginAreas) === FOOTER_PLACEMENT)
    .map(toNavItem)
    .sort((a, b) => a.order - b.order);

  const nextAreas = baseAreas.map((staticArea) => {
    // Deep copy groups to avoid mutating global STATIC_AREAS; drop items
    // owned by a plugin the workspace has disabled.
    const groups = staticArea.groups.map((g) => ({
      label: g.label,
      items: g.items.filter((i) => !disabledPaths.has(i.path)),
    }));

    // A category matching a plugin-declared area (id/title) lands there;
    // otherwise the built-in keyword routing applies — one pipeline.
    const areaExtras = extras.filter((m) => areaForCategory(m.category, pluginAreas) === staticArea.id);

    areaExtras.forEach((m) => {
      // Known icon keys resolve to lucide (single map in shell/areas.tsx);
      // anything else renders literally as text/emoji via iconStr.
      const KeyIcon = menuIcon(m.icon);
      const item = {
        id: m.id,
        label: m.label,
        Icon: KeyIcon ?? Globe,
        iconStr: KeyIcon ? undefined : m.icon,
        path: m.path,
        order: m.order ?? 0,
      };

      // In the Settings area a generic "settings" category means org workspace
      // configuration (SSO, white-label, Slack, …) → the Workspace group. ("Workspace"
      // itself can't be used as the category: it names the top-level operations area.)
      const effectiveCategory =
        staticArea.id === "settings" && (m.category || "").toLowerCase() === "settings"
          ? "Workspace"
          : (m.category || "");

      // Check if there is an existing group matching the category name (case-insensitive)
      const matchedGroup = groups.find(
        (g) => g.label.toLowerCase() === effectiveCategory.toLowerCase()
      );

      if (matchedGroup) {
        matchedGroup.items.push(item);
      } else {
        const groupName = effectiveCategory || "More";
        const dynamicGroup = groups.find((g) => g.label === groupName);
        if (dynamicGroup) {
          dynamicGroup.items.push(item);
        } else {
          groups.push({
            label: groupName,
            items: [item],
          });
        }
      }
    });

    groups.forEach((g) => {
      g.items.sort((a: any, b: any) => (a.order ?? 0) - (b.order ?? 0));
    });

    return {
      ...staticArea,
      groups: groups.filter((g) => g.items.length > 0),
    };
  });

  // Drop whole areas (e.g. "Commerce") that have no visible items left —
  // otherwise a disabled feature still shows an empty top-level section.
  return { areas: nextAreas.filter((a) => a.groups.length > 0), footer };
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
  const { t } = useTranslation();

  // Bumped on every workspace switch to remount the routed content, so each
  // page refetches for the new workspace — an in-app refresh that replaces the
  // old full-page window.location.reload().
  const [orgEpoch, setOrgEpoch] = useState(0);

  // Raw nav inputs from the API; `areas` is DERIVED from them (plus the
  // role/admin flags) so a late-arriving isInstanceAdmin re-runs the same
  // mergeAreas pipeline instead of a second filtering pass.
  const [backendNav, setBackendNav] = useState<{ menus: MenuItem[]; plugins: PluginInfo[] }>(
    readCachedNav,
  );
  // False until api.menus()/api.plugins() have answered at least once. Gates the
  // backend-driven orphan-drop in mergeAreas so the initial empty render doesn't
  // strip the always-composed core menus before the backend confirms them.
  const [backendLoaded, setBackendLoaded] = useState(false);
  const [orgs, setOrgs]   = useState<Org[]>([]);
  const [creatingOrg, setCreatingOrg] = useState(false);
  const [newOrgName, setNewOrgName]   = useState("");
  // Multi-workspace is a Pro feature. The OSS binary registers no Pro plugins,
  // so a non-empty plugin list means this is a Pro build where it's available.
  const [isProBuild, setIsProBuild] = useState(false);
  const [isInstanceAdmin, setIsInstanceAdmin] = useState(false);
  const [emailVerified, setEmailVerified] = useState<boolean | undefined>(undefined);
  const [dismissedVerifyBanner, setDismissedVerifyBanner] = useState(false);
  const [resendingVerify, setResendingVerify] = useState(false);

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

  const merged = useMemo(
    () => mergeAreas(backendNav.menus, backendNav.plugins, role, isInstanceAdmin, backendLoaded),
    [backendNav, role, isInstanceAdmin, backendLoaded],
  );
  // The Settings area is reached via the gear, never a top-level tab, so it's
  // held out of `areas` (tabs / areaForPath / command palette business areas)
  // and surfaced separately as the second-level rail for /settings.
  const areas = useMemo(() => merged.areas.filter((a) => a.id !== "settings"), [merged]);
  const mergedSettingsArea = useMemo(
    () => merged.areas.find((a) => a.id === "settings") ?? SETTINGS_AREA,
    [merged],
  );
  // Plugin-contributed footer items (e.g. the Pro Help plugin) shown among the
  // octarq resources in the rail footer.
  const footerItems = merged.footer;
  // Role inputs for PluginGate requiredRole pre-check.
  const roleCtx = useMemo(() => ({ role, isInstanceAdmin }), [role, isInstanceAdmin]);

  const pluginGateCtxValue = useMemo(() => {
    const disabledPlugins = new Set(backendNav.plugins.filter((p) => !p.enabled).map((p) => p.key));
    const disabledPaths = new Set(backendNav.plugins.filter((p) => !p.enabled).flatMap((p) => p.menus.map((m) => m.path)));
    return { disabledPlugins, disabledPaths, loaded: backendLoaded };
  }, [backendNav.plugins, backendLoaded]);

  // Every core settings page lives under /settings (one URL space — no /personal
  // tree). Plugin settings pages keep top-level paths (e.g. /sso, /whitelabel,
  // /license — a menu Category routed them into the Settings area), so the shell
  // stays in the settings context for those too; otherwise navigating to one
  // would drop the settings rail and orphan the highlight.
  const settingsPaths = useMemo(
    () => new Set(mergedSettingsArea.groups.flatMap((g) => g.items.map((i) => i.path))),
    [mergedSettingsArea],
  );
  const settingsActive =
    location.pathname.startsWith("/settings") ||
    [...settingsPaths].some((p) => location.pathname === p || location.pathname.startsWith(p + "/"));
  // Resolve against the merged runtime areas (static + plugin areas + dynamic
  // menu items) so paths owned by plugin-contributed areas highlight correctly.
  const activeArea: AreaId = settingsActive ? "settings" : areaForPath(location.pathname, areas);

  // Load orgs + dynamic menus + user settings layout. Also refreshes the org
  // role here (not just on mount) so switching to a workspace where the user
  // has a different role re-runs the sidebar/PluginGate role gating.
  useEffect(() => {
    api.me().then((m) => { setRole(m.role); setEmailVerified(m.emailVerified); }).catch(() => {});
    api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));
    api.settings().then((s) => setIsInstanceAdmin(!!s.isInstanceAdmin)).catch(() => {});

    Promise.all([api.menus().catch(() => []), api.plugins().catch(() => [])])
      .then(([backendMenus, plugins]) => {
        setIsProBuild(plugins.length > 0);
        setBackendNav({ menus: backendMenus, plugins });
        setBackendLoaded(true);
        writeCachedNav({ menus: backendMenus, plugins });
      })
      .catch(() => {});
  }, [activeOrgId]);

  // Settings pages that mutate the workspace list (rename) fire this instead of
  // reloading the page; refetch the orgs so the switcher/name update in place.
  useEffect(() => {
    const refreshOrgs = () => api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));
    const refreshPlugins = () => {
      Promise.all([api.menus().catch(() => []), api.plugins().catch(() => [])])
        .then(([backendMenus, plugins]) => {
          setIsProBuild(plugins.length > 0);
          setBackendNav({ menus: backendMenus, plugins });
          writeCachedNav({ menus: backendMenus, plugins });
        })
        .catch(() => {});
    };
    window.addEventListener("octarq:orgs-changed", refreshOrgs);
    window.addEventListener("octarq:plugins-changed", refreshPlugins);
    return () => {
      window.removeEventListener("octarq:orgs-changed", refreshOrgs);
      window.removeEventListener("octarq:plugins-changed", refreshPlugins);
    };
  }, []);

  // The Instance group is octarq-provided instance administration (Instance
  // Settings, octarq License, …) — visible only to instance admins.
  const currentSettingsArea = useMemo(() => {
    if (isInstanceAdmin) return mergedSettingsArea;
    return {
      ...mergedSettingsArea,
      groups: mergedSettingsArea.groups.filter((g) => g.label !== "Instance"),
    };
  }, [mergedSettingsArea, isInstanceAdmin]);

  const currentArea = settingsActive ? currentSettingsArea : (areas.find((a) => a.id === activeArea) ?? areas[0]);
  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name ?? t("app.personalWorkspace");

  // Apply an active-workspace change in-app: point the shell at the new org
  // (its useEffect refetches menus/plugins/role/settings), remount the routed
  // content so every page reloads its data, and land on Overview.
  function switchToOrg(id: number) {
    setActiveOrgId(id);
    setOrgEpoch((e) => e + 1);
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

  const selectArea = (id: AreaId) => {
    if (id === "settings") { navigate("/settings"); return; }
    const area = areas.find((a) => a.id === id);
    navigate(area?.groups[0]?.items[0]?.path ?? "/overview");
  };

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

  return (
    <RoleProvider value={roleCtx}>
    <div className="octarq-aurora flex h-screen w-full flex-col overflow-hidden text-foreground">
      {/* Keyboard skip link — first focusable element, visually hidden until
          focused, jumps past the nav chrome straight to page content. */}
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-3 focus:z-[60] focus:rounded-xl focus:bg-indigo-500 focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-white focus:shadow-glow"
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
                  await api.resendVerification(user);
                  toast.success(t("app.verificationSent"));
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
          currentPath={location.pathname}
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
        <div className="h-full overflow-y-auto [scrollbar-gutter:stable]">
          <div key={orgEpoch} className="mx-auto w-full max-w-6xl px-8 py-8">
            <Suspense fallback={<RouteFallback />}>
            <PluginGateContext.Provider value={pluginGateCtxValue}>
            <Routes>
              <Route path="/"           element={<Navigate to="/overview" replace />} />
              <Route path="/overview"   element={<OverviewPage />} />
              <Route path="/settings/*" element={<SettingsPage />} />
              <Route path="/admin/invite/accept" element={<InviteAcceptPage />} />
              <Route path="/admin/reset" element={<ResetPasswordPage />} />
              {/* Every business page — core (plugins/core) and edition-composed
                  (manifest) — flows through the same registry. */}
              {pluginRouteElements()}
              {/* Unknown paths 404-degrade to a neutral note instead of silently
                  redirecting — a Pro plugin path with no composed plugin lands
                  here, matching octarq's "not in this build" convention. */}
              <Route path="*"           element={<PluginUnavailable />} />
            </Routes>
            </PluginGateContext.Provider>
            </Suspense>
          </div>
        </div>
      </main>
      </div>

      <CommandPalette
        open={cmdOpen}
        onClose={() => setCmdOpen(false)}
        areas={areas}
        settingsArea={currentSettingsArea}
        onNavigate={(path) => { navigate(path); setCmdOpen(false); }}
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
  );
}
