import { useEffect, useMemo, useRef, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { Globe, PanelLeft } from "lucide-react";
import { api, MenuItem, Org, PluginInfo } from "./api";
import { useAppName, brandInitial } from "./brand";
import OverviewPage from "./pages/Overview";
import SettingsPage from "./pages/Settings";
import PersonalSettingsPage from "./pages/PersonalSettings";
import InviteAcceptPage from "./pages/InviteAccept";
import { Modal, Button } from "./ui";
import { useTranslation } from "./i18n";
import { Area, AreaId, STATIC_AREAS, SETTINGS_AREA, areaForPath, areaForCategory, menuIcon, pluginAreaToArea } from "./shell/areas";
import { RoleProvider, roleSatisfies } from "./shell/role";
import { TopBar } from "./shell/TopBar";
import { CommandPalette } from "./shell/CommandPalette";
import { AreaPanel } from "./shell/AreaPanel";
import { Login } from "./shell/Login";
import { uiAreas, uiMenus } from "./plugin-sdk";
import { pluginRouteElements, PluginUnavailable } from "./plugins/PluginRoutes";


// ─── App ──────────────────────────────────────────────────────────────────────

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);
  // Org role from /api/auth/me ("owner" | "admin" | "member") — advisory input
  // for requiredRole gating (sidebar filter + ProGate pre-check). UX only.
  const [role, setRole] = useState<string | undefined>(undefined);
  const appName = useAppName();

  useEffect(() => {
    api.me()
      .then((m) => { setUser(m.username); setActiveOrgId(m.orgId); setRole(m.role); setAuthed(true); })
      .catch(() => setAuthed(false));
  }, []);

  let content;
  if (window.location.pathname === "/admin/invite/accept") {
    content = <InviteAcceptPage />;
  } else if (authed === null) {
    content = (
      <div className="octarq-aurora grid h-full place-items-center text-white/40">
        <div className="flex flex-col items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow flex items-center justify-center">
            <span className="font-display text-base font-extrabold text-white">{brandInitial(appName)}</span>
          </div>
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
        activeOrgId={activeOrgId}
        setActiveOrgId={setActiveOrgId}
        onLogout={async () => {
          try { await api.logout(); } catch { /* clear locally even if the request fails */ }
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

// ─── Sidebar merge ────────────────────────────────────────────────────────────

// Merge every menu source into the final area list — ONE pipeline:
//   STATIC_AREAS      area/group shells + the few shell-owned items (Overview);
//   uiMenus()         build-time-composed frontend plugins (core features AND
//                     Pro alike — see plugins/core/index.ts);
//   backendMenus      dynamic menus from Go plugins (api.menus());
// each item is routed to an area by the shared areaForCategory and into the
// group whose label matches its category. Called with empty backend data for
// the initial synchronous render (uiMenus() is populated at module eval, so
// core items never flash in and out), then again once the API answers.
// `role`/`isInstanceAdmin` drive the requiredRole filter: menu entries whose
// advisory requiredRole the current user doesn't meet are dropped here — the
// single place — so the sidebar AND the command palette (both fed by the
// resulting areas) agree. Ranking lives in roleSatisfies (shell/role.tsx),
// shared with ProGate's route pre-check.
function mergeAreas(
  backendMenus: MenuItem[],
  plugins: PluginInfo[],
  role: string | undefined,
  isInstanceAdmin: boolean,
): Area[] {
  // On duplicate paths the frontend plugin entry wins: the OSS backend also
  // announces core paths (/links, /mail, …) in api.menus() for API consumers,
  // but the composed core plugin carries the richer icon/category placement.
  const seenPaths = new Set<string>();
  const menus = [...uiMenus(), ...backendMenus].filter((m) => {
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
  // frontend plugins (UIPlugin.areas → uiAreas()). Plugin areas start as empty
  // shells — like Commerce's group shells — and are filled by the same
  // category-merge below; still-empty ones are dropped by the empty-area
  // filter at the end. "settings" and ids colliding with a static area can't
  // be redeclared.
  const pluginAreas = uiAreas().filter(
    (pa) => pa.id !== "settings" && !STATIC_AREAS.some((sa) => sa.id === pa.id),
  );
  const baseAreas = [...STATIC_AREAS, ...pluginAreas.map(pluginAreaToArea)];

  const staticPaths = new Set(baseAreas.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path))));
  const extras = menus.filter(
    (m) =>
      !staticPaths.has(m.path) &&
      !disabledPaths.has(m.path) &&
      roleSatisfies(m.requiredRole, role, isInstanceAdmin),
  );

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
      };

      // Check if there is an existing group matching the category name (case-insensitive)
      const matchedGroup = groups.find(
        (g) => g.label.toLowerCase() === (m.category || "").toLowerCase()
      );

      if (matchedGroup) {
        matchedGroup.items.push(item);
      } else {
        const groupName = m.category || "More";
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

    return {
      ...staticArea,
      groups: groups.filter((g) => g.items.length > 0),
    };
  });

  // Drop whole areas (e.g. "Commerce") that have no visible items left —
  // otherwise a disabled feature still shows an empty top-level section.
  return nextAreas.filter((a) => a.groups.length > 0);
}

// ─── Shell ────────────────────────────────────────────────────────────────────

function Shell({
  user,
  role,
  activeOrgId,
  setActiveOrgId,
  onLogout,
}: {
  user: string;
  role?: string;
  activeOrgId: number;
  setActiveOrgId: (id: number) => void;
  onLogout: () => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const { t } = useTranslation();

  // Raw nav inputs from the API; `areas` is DERIVED from them (plus the
  // role/admin flags) so a late-arriving isInstanceAdmin re-runs the same
  // mergeAreas pipeline instead of a second filtering pass.
  const [backendNav, setBackendNav] = useState<{ menus: MenuItem[]; plugins: PluginInfo[] }>(
    { menus: [], plugins: [] },
  );
  const [orgs, setOrgs]   = useState<Org[]>([]);
  const [creatingOrg, setCreatingOrg] = useState(false);
  const [newOrgName, setNewOrgName]   = useState("");
  // Multi-workspace is a Pro feature. The OSS binary registers no Pro plugins,
  // so a non-empty plugin list means this is a Pro build where it's available.
  const [isProBuild, setIsProBuild] = useState(false);
  const [isInstanceAdmin, setIsInstanceAdmin] = useState(false);

  // Collapse the second-level area panel to widen the content area. Persisted,
  // and kept in the layout (not AreaPanel) so it survives area switches.
  const [panelCollapsed, setPanelCollapsed] = useState(() => {
    try { return localStorage.getItem("area_panel_collapsed") === "1"; } catch { return false; }
  });
  const togglePanel = () => setPanelCollapsed((v) => {
    const next = !v;
    try { localStorage.setItem("area_panel_collapsed", next ? "1" : "0"); } catch { /* ignore */ }
    return next;
  });

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

  const areas = useMemo(
    () => mergeAreas(backendNav.menus, backendNav.plugins, role, isInstanceAdmin),
    [backendNav, role, isInstanceAdmin],
  );
  // The same role inputs, for ProGate's per-route requiredRole pre-check.
  const roleCtx = useMemo(() => ({ role, isInstanceAdmin }), [role, isInstanceAdmin]);

  const settingsActive = location.pathname.startsWith("/settings") || location.pathname.startsWith("/personal");
  // Resolve against the merged runtime areas (static + plugin areas + dynamic
  // menu items) so paths owned by plugin-contributed areas highlight correctly.
  const activeArea: AreaId = settingsActive ? "settings" : areaForPath(location.pathname, areas);

  // Load orgs + dynamic menus + user settings layout
  useEffect(() => {
    api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));
    api.settings().then((s) => setIsInstanceAdmin(!!s.isInstanceAdmin)).catch(() => {});

    Promise.all([api.menus().catch(() => []), api.plugins().catch(() => [])])
      .then(([backendMenus, plugins]) => {
        setIsProBuild(plugins.length > 0);
        setBackendNav({ menus: backendMenus, plugins });
      })
      .catch(() => {});
  }, [activeOrgId]);

  const currentSettingsArea = useMemo(() => {
    if (isInstanceAdmin) return SETTINGS_AREA;
    return {
      ...SETTINGS_AREA,
      groups: SETTINGS_AREA.groups.filter((g) => g.label !== "Instance"),
    };
  }, [isInstanceAdmin]);

  const currentArea = settingsActive ? currentSettingsArea : (areas.find((a) => a.id === activeArea) ?? areas[0]);
  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name ?? t("app.personalWorkspace");

  function handleCreateOrg(e: React.FormEvent) {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    api.createOrg({ name: newOrgName })
      .then((org) => api.switchOrg(org.id).then(() => window.location.reload()))
      .catch((e) => alert(e.message || t("app.createWorkspaceFailed")));
  }

  const selectArea = (id: AreaId) => {
    if (id === "settings") { navigate("/settings"); return; }
    const area = areas.find((a) => a.id === id);
    navigate(area?.groups[0]?.items[0]?.path ?? "/overview");
  };

  return (
    <RoleProvider value={roleCtx}>
    <div className="octarq-aurora flex h-screen w-full flex-col overflow-hidden text-white">
      <TopBar
        areas={areas}
        activeArea={activeArea}
        settingsActive={settingsActive}
        orgs={orgs}
        activeOrgId={activeOrgId}
        activeOrgName={activeOrgName}
        user={user}
        showWorkspaceSwitcher={isProBuild}
        onSelectArea={selectArea}
        onSwitchOrg={(id) =>
          api.switchOrg(id).then(() => { setActiveOrgId(id); window.location.reload(); })
        }
        onCreateOrg={() => setCreatingOrg(true)}
        onOpenSettings={() => navigate("/settings")}
        onOpenCommand={() => setCmdOpen(true)}
        onLogout={onLogout}
      />

      <div className="flex min-h-0 flex-1 overflow-hidden">
      <AnimatePresence mode="wait">
        {!panelCollapsed && (
          <AreaPanel
            key={activeArea}
            area={currentArea}
            currentPath={location.pathname}
            onCollapse={togglePanel}
          />
        )}
      </AnimatePresence>

      <main className="relative flex-1 overflow-hidden">
        {panelCollapsed && (
          <button
            onClick={togglePanel}
            title={t(`areas.${currentArea.id}.title`, currentArea.title)}
            className="group absolute left-0 top-1/2 z-30 flex -translate-y-1/2 items-center gap-1 rounded-r-xl border border-l-0 border-white/[0.08] bg-white/[0.04] py-3 pl-1 pr-1.5 text-white/45 backdrop-blur-xl transition-colors hover:bg-white/[0.08] hover:text-white"
          >
            <PanelLeft className="h-4 w-4 rotate-180" strokeWidth={1.75} />
          </button>
        )}
        <div className="h-full overflow-y-auto">
          <div className="mx-auto w-full max-w-6xl px-8 py-8">
            <Routes>
              <Route path="/"           element={<Navigate to="/overview" replace />} />
              <Route path="/overview"   element={<OverviewPage />} />
              <Route path="/settings/*" element={<SettingsPage />} />
              <Route path="/personal/*" element={<PersonalSettingsPage />} />
              <Route path="/admin/invite/accept" element={<InviteAcceptPage />} />
              {/* Every business page — core (plugins/core) and edition-composed
                  (manifest) — flows through the same registry. */}
              {pluginRouteElements()}
              {/* Unknown paths 404-degrade to a neutral note instead of silently
                  redirecting — a Pro plugin path with no composed plugin lands
                  here, matching octarq's "not in this build" convention. */}
              <Route path="*"           element={<PluginUnavailable />} />
            </Routes>
          </div>
        </div>
      </main>
      </div>

      <CommandPalette
        open={cmdOpen}
        onClose={() => setCmdOpen(false)}
        areas={areas}
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
            <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
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
