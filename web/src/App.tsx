import { useEffect, useMemo, useRef, useState } from "react";
import { NavLink, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import {
  Bot,
  Boxes,
  CalendarClock,
  CheckIcon,
  ChevronsUpDown,
  CreditCard,
  Globe,
  KeyRound,
  LayoutDashboard,
  LineChart,
  Link2,
  LogOut,
  Mail,
  ScrollText,
  Server,
  Settings,
  ShieldAlert,
  User,
  Wallet,
  Workflow,
  Puzzle,
  Bell,
  Users,
  Database,
  HardDrive,
  Shield,
  Store,
  PanelLeft,
  Webhook,
  Search,
} from "lucide-react";
import { api, ApiError, MenuItem, Org } from "./api";
import { useAppName, brandInitial } from "./brand";
import OverviewPage from "./pages/Overview";
import LinksPage from "./pages/Links";
import DomainsPage from "./pages/Domains";
import MailPage from "./pages/Mail";
import SettingsPage from "./pages/Settings";
import SSHKeysPage from "./pages/SSHKeys";
import VPSPage from "./pages/VPS";
import FinancePage from "./pages/Finance";
import StorefrontPage from "./pages/Storefront";
import BillingPage from "./pages/Billing";
import InboxAIPage from "./pages/InboxAI";
import AuditLogPage from "./pages/AuditLog";
import AbusePage from "./pages/Abuse";
import PersonalSettingsPage from "./pages/PersonalSettings";
import InviteAcceptPage from "./pages/InviteAccept";
import { Modal, Button, ScreenWrap, PageHeader, GlassCard } from "./ui";
import { useTranslation } from "./i18n";
import { Area, AreaId, STATIC_AREAS, SETTINGS_AREA, areaForPath, areaForCategory } from "./shell/areas";
import { TopBar } from "./shell/TopBar";
import { CommandPalette } from "./shell/CommandPalette";
import { AreaPanel } from "./shell/AreaPanel";
import { Login } from "./shell/Login";
import { uiMenus } from "./plugin-sdk";
import { pluginRouteElements, PluginUnavailable } from "./plugins/PluginRoutes";


// ─── App ──────────────────────────────────────────────────────────────────────

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);
  const appName = useAppName();

  useEffect(() => {
    api.me()
      .then((m) => { setUser(m.username); setActiveOrgId(m.orgId); setAuthed(true); })
      .catch(() => setAuthed(false));
  }, []);

  let content;
  if (window.location.pathname === "/admin/invite/accept") {
    content = <InviteAcceptPage />;
  } else if (authed === null) {
    content = (
      <div className="led-aurora grid h-full place-items-center text-white/40">
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
        onLogin={(u, orgId) => { setUser(u); setActiveOrgId(orgId); setAuthed(true); }}
      />
    );
  } else {
    content = (
      <Shell
        user={user}
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

// ─── Shell ────────────────────────────────────────────────────────────────────

function Shell({
  user,
  activeOrgId,
  setActiveOrgId,
  onLogout,
}: {
  user: string;
  activeOrgId: number;
  setActiveOrgId: (id: number) => void;
  onLogout: () => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const { t } = useTranslation();

  const [areas, setAreas] = useState<Area[]>(STATIC_AREAS);
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

  const settingsActive = location.pathname.startsWith("/settings") || location.pathname.startsWith("/personal");
  const activeArea: AreaId = settingsActive ? "settings" : areaForPath(location.pathname);

  // Load orgs + dynamic menus + user settings layout
  useEffect(() => {
    api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));
    api.settings().then((s) => setIsInstanceAdmin(!!s.isInstanceAdmin)).catch(() => {});

    const MASTER_MENU_ITEMS: Record<string, { label: string; Icon: React.ElementType; path: string; iconStr?: string }> = {
      overview: { label: "Overview", Icon: LayoutDashboard, path: "/overview" },
      links: { label: "Links", Icon: Link2, path: "/links" },
      mail: { label: "Mail", Icon: Mail, path: "/mail" },
      "inbox-ai": { label: "AI Inbox", Icon: Bot, path: "/inbox-ai" },
      domains: { label: "DNS", Icon: Globe, path: "/domains" },
      certs: { label: "Certificates", Icon: Shield, path: "/assets/certificates" },
      vps: { label: "Servers", Icon: Server, path: "/vps" },
      sshkeys: { label: "SSH Vault", Icon: KeyRound, path: "/sshkeys" },
      databases: { label: "Databases", Icon: Database, path: "/assets/databases" },
      storage: { label: "Object Storage", Icon: HardDrive, path: "/assets/storage" },
      finance: { label: "Bookkeeping", Icon: Wallet, path: "/finance" },
      abuse: { label: "Abuse", Icon: ShieldAlert, path: "/abuse" },
      audit: { label: "Audit", Icon: ScrollText, path: "/audit" },
    };

    Promise.all([api.menus().catch(() => []), api.plugins().catch(() => [])])
      .then(([backendMenus, plugins]) => {
        setIsProBuild(plugins.length > 0);
        // Sidebar entries from build-time-composed frontend plugins (UIPlugin.menu)
        // are folded in beside dynamic backend menus and placed by the same
        // areaForCategory logic — no parallel mechanism. Empty in the OSS build.
        const menus = [...backendMenus, ...uiMenus()];
        // Paths owned by a disabled plugin are hidden from the sidebar. Dynamic
        // plugin menus are already filtered server-side; this also drops the
        // statically-declared Pro items (Storefront, Servers, …) when off.
        const disabledPaths = new Set(
          plugins.filter((p) => !p.enabled).flatMap((p) => p.menus.map((m) => m.path)),
        );

        // Build full catalog including dynamic plugin menus
        const catalog = { ...MASTER_MENU_ITEMS };
        menus.forEach((m) => {
          if (!catalog[m.id]) {
            catalog[m.id] = {
              label: m.label,
              Icon: Globe,
              iconStr: m.icon,
              path: m.path,
            };
          }
        });

        const staticPaths = new Set(STATIC_AREAS.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path))));
        const extras = menus.filter((m) => !staticPaths.has(m.path));

        const nextAreas = STATIC_AREAS.map((staticArea) => {
          // Deep copy groups to avoid mutating global STATIC_AREAS; drop items
          // owned by a plugin the workspace has disabled.
          const groups = staticArea.groups.map((g) => ({
            label: g.label,
            items: g.items.filter((i) => !disabledPaths.has(i.path)),
          }));

          const areaExtras = extras.filter((m) => areaForCategory(m.category) === staticArea.id);

          areaExtras.forEach((m) => {
            const item = {
              id: m.id,
              label: m.label,
              Icon: Globe,
              iconStr: m.icon,
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
        setAreas(nextAreas.filter((a) => a.groups.length > 0));
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
    const area = areas.find((a) => a.id === id)!;
    navigate(area.groups[0]?.items[0]?.path ?? "/overview");
  };

  return (
    <div className="led-aurora flex h-screen w-full flex-col overflow-hidden text-white">
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
              <Route path="/links"      element={<LinksPage />} />
              <Route path="/domains"    element={<DomainsPage />} />
              <Route path="/mail"       element={<MailPage />} />
              <Route path="/inbox-ai"   element={<InboxAIPage />} />
              <Route path="/vps"        element={<VPSPage />} />
              <Route path="/sshkeys"    element={<SSHKeysPage />} />
              <Route path="/assets/certificates" element={<ComingSoonPage title={t("app.certsTitle")} description={t("app.certsDesc")} />} />
              <Route path="/assets/databases"    element={<ComingSoonPage title={t("app.databasesTitle")} description={t("app.databasesDesc")} />} />
              <Route path="/assets/storage"      element={<ComingSoonPage title={t("app.storageTitle")} description={t("app.storageDesc")} />} />
              <Route path="/finance"    element={<FinancePage />} />
              <Route path="/storefront" element={<StorefrontPage />} />
              <Route path="/billing"    element={<BillingPage />} />
              <Route path="/audit"      element={<AuditLogPage />} />
              <Route path="/abuse"      element={<AbusePage />} />
              <Route path="/settings/*" element={<SettingsPage />} />
              <Route path="/personal/*" element={<PersonalSettingsPage />} />
              <Route path="/admin/invite/accept" element={<InviteAcceptPage />} />
              {/* Build-time-composed frontend plugins (e.g. licenses). Empty in
                  the OSS build ⇒ their paths fall to the neutral fallback below. */}
              {pluginRouteElements()}
              {/* Unknown paths 404-degrade to a neutral note instead of silently
                  redirecting — a Pro plugin path with no composed plugin lands
                  here, matching led's "not in this build" convention. */}
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
  );
}

function ComingSoonPage({ title, description }: { title: string; description: string }) {
  const { t } = useTranslation();
  return (
    <ScreenWrap>
      <PageHeader title={title} description={description} />
      <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center">
        <div className="h-16 w-16 rounded-2xl bg-indigo-500/10 flex items-center justify-center text-indigo-400 mb-4 animate-pulse">
          <Boxes className="h-8 w-8" />
        </div>
        <h3 className="text-lg font-bold text-white mb-2">{t("app.comingSoonTitle")}</h3>
        <p className="text-sm text-white/50 max-w-sm leading-relaxed">
          {t("app.comingSoonBody")}
        </p>
      </GlassCard>
    </ScreenWrap>
  );
}
