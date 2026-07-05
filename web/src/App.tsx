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
import LicensesPage from "./pages/Licenses";
import BillingPage from "./pages/Billing";
import InboxAIPage from "./pages/InboxAI";
import AuditLogPage from "./pages/AuditLog";
import AbusePage from "./pages/Abuse";
import PersonalSettingsPage from "./pages/PersonalSettings";
import InviteAcceptPage from "./pages/InviteAccept";
import { Modal, Button, ScreenWrap, PageHeader, GlassCard } from "./ui";
import { useTranslation, LANGS } from "./i18n";

// ─── Area definitions ──────────────────────────────────────────────────────

type AreaId = "operations" | "commerce" | "assets" | "insights" | "settings";

interface NavItem {
  id: string;
  label: string;
  Icon: React.ElementType;
  iconStr?: string;
  path: string;
  badge?: string | number;
}

interface NavGroup {
  label: string;
  items: NavItem[];
}

interface Area {
  id: AreaId;
  title: string;
  subtitle: string;
  Icon: React.ElementType;
  groups: NavGroup[];
}

const STATIC_AREAS: Area[] = [
  {
    id: "operations",
    title: "Workspace",
    subtitle: "Daily traffic & communication",
    Icon: Workflow,
    groups: [
      {
        label: "Workspace",
        items: [
          { id: "overview", label: "Overview", Icon: LayoutDashboard, path: "/overview" },
        ],
      },
      {
        label: "Marketing",
        items: [
          { id: "links",   label: "Links",    Icon: Link2,  path: "/links" },
        ],
      },
      {
        label: "Messaging",
        items: [
          { id: "mail",    label: "Mail",     Icon: Mail,   path: "/mail" },
          { id: "inbox-ai", label: "AI Inbox", Icon: Bot,    path: "/inbox-ai" },
        ],
      },
    ],
  },
  {
    id: "commerce",
    title: "Commerce",
    subtitle: "Revenue, store & cost analysis",
    Icon: Wallet,
    groups: [
      {
        label: "Sales",
        items: [
          { id: "storefront", label: "Storefront", Icon: Store,      path: "/storefront" },
          { id: "licenses",   label: "Licenses",   Icon: KeyRound,   path: "/licenses" },
        ],
      },
      {
        label: "Billing",
        items: [
          { id: "billing",    label: "Billing",    Icon: CreditCard, path: "/billing" },
        ],
      },
      {
        label: "Finance",
        items: [
          { id: "finance",    label: "Bookkeeping",Icon: Wallet,     path: "/finance" },
        ],
      },
    ],
  },
  {
    id: "assets",
    title: "Infrastructure",
    subtitle: "Servers, network & databases",
    Icon: Boxes,
    groups: [
      {
        label: "Network",
        items: [
          { id: "domains", label: "DNS",          Icon: Globe,    path: "/domains" },
          { id: "certs",   label: "Certificates", Icon: Shield,  path: "/assets/certificates" },
        ],
      },
      {
        label: "Hosting",
        items: [
          { id: "vps",     label: "Servers",   Icon: Server,   path: "/vps" },
          { id: "sshkeys", label: "SSH Vault", Icon: KeyRound, path: "/sshkeys" },
        ],
      },
      {
        label: "Storage & Databases",
        items: [
          { id: "databases", label: "Databases", Icon: Database,  path: "/assets/databases" },
          { id: "storage",   label: "Object Storage", Icon: HardDrive, path: "/assets/storage" },
        ],
      },
    ],
  },
  {
    id: "insights",
    title: "Security & Admin",
    subtitle: "Abuse defense & activity logs",
    Icon: ShieldAlert,
    groups: [
      {
        label: "Security",
        items: [
          { id: "abuse",    label: "Abuse",      Icon: ShieldAlert, path: "/abuse" },
        ],
      },
      {
        label: "System",
        items: [
          { id: "audit",    label: "Audit",      Icon: ScrollText,  path: "/audit" },
        ],
      },
    ],
  },
];

const SETTINGS_AREA: Area = {
  id: "settings",
  title: "Settings",
  subtitle: "Workspace & profile configurations",
  Icon: Settings,
  groups: [
    {
      label: "Workspace",
      items: [
        { id: "general",       label: "General",     Icon: Settings,    path: "/settings/general" },
        { id: "plugins",       label: "Plugins",     Icon: Puzzle,      path: "/settings/plugins" },
        { id: "members",       label: "Members",     Icon: Users,       path: "/settings/members" },
        { id: "webhooks",      label: "Webhooks",    Icon: Webhook,     path: "/settings/webhooks" },
        { id: "notifications", label: "Alerts",      Icon: Bell,        path: "/settings/notifications" },
      ],
    },
    {
      label: "Account",
      items: [
        { id: "profile",  label: "My Profile", Icon: User,      path: "/personal/profile" },
        { id: "security", label: "Security",   Icon: Shield,    path: "/settings/security" },
        { id: "tokens",   label: "API Tokens", Icon: KeyRound,  path: "/personal/tokens" },
      ],
    },
    {
      label: "Subscriptions",
      items: [
        { id: "billing", label: "Billing & Plan", Icon: CreditCard, path: "/settings/billing" },
        { id: "license", label: "License",        Icon: KeyRound,   path: "/settings/license" },
      ],
    },
  ],
};

// Map a path to its area
// PATH_TO_AREA is derived from STATIC_AREAS so the path→area mapping has a single
// source of truth (the menu definition) instead of a parallel if/else chain.
const PATH_TO_AREA: { prefix: string; area: AreaId }[] = STATIC_AREAS
  .flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ prefix: i.path, area: a.id }))))
  .sort((x, y) => y.prefix.length - x.prefix.length); // longest prefix wins

function areaForPath(path: string): AreaId {
  // Settings/personal live in their own area (SETTINGS_AREA), not STATIC_AREAS.
  if (path.startsWith("/settings") || path.startsWith("/personal")) return "settings";
  const hit = PATH_TO_AREA.find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));
  return hit?.area ?? "operations";
}

// Map a dynamic menu category to an area. Keep this in sync with the Category
// strings plugins set in their Menus() — see docs/SIDEBAR-MENU.md.
function areaForCategory(cat?: string): AreaId {
  const c = (cat ?? "").toLowerCase();
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute") || c.includes("hosting")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("compliance") || c.includes("governance") || c.includes("audit") || c.includes("abuse")) return "insights";
  if (c.includes("commerce") || c.includes("sell") || c.includes("billing") || c.includes("storefront") || c.includes("license") || c.includes("finance")) return "commerce";
  return "operations";
}

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
      .then(([menus, plugins]) => {
        setIsProBuild(plugins.length > 0);
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

  const currentArea = settingsActive ? SETTINGS_AREA : (areas.find((a) => a.id === activeArea) ?? areas[0]);
  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name ?? "Personal Workspace";

  function handleCreateOrg(e: React.FormEvent) {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    api.createOrg({ name: newOrgName })
      .then((org) => api.switchOrg(org.id).then(() => window.location.reload()))
      .catch((e) => alert(e.message || "Couldn't create the workspace"));
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
              <Route path="/assets/certificates" element={<ComingSoonPage title="Certificates & SSL" description="Track and renew your SSL certificates" />} />
              <Route path="/assets/databases"    element={<ComingSoonPage title="Managed Databases" description="Provision and monitor PostgreSQL, Redis, or MySQL database instances" />} />
              <Route path="/assets/storage"      element={<ComingSoonPage title="Object Storage" description="Configure Cloudflare R2, AWS S3, or Backblaze B2 buckets" />} />
              <Route path="/finance"    element={<FinancePage />} />
              <Route path="/storefront" element={<StorefrontPage />} />
              <Route path="/licenses"   element={<LicensesPage />} />
              <Route path="/billing"    element={<BillingPage />} />
              <Route path="/audit"      element={<AuditLogPage />} />
              <Route path="/abuse"      element={<AbusePage />} />
              <Route path="/settings/*" element={<SettingsPage />} />
              <Route path="/personal/*" element={<PersonalSettingsPage />} />
              <Route path="/admin/invite/accept" element={<InviteAcceptPage />} />
              <Route path="*"           element={<Navigate to="/overview" replace />} />
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
        <Modal title="Create Workspace" onClose={() => setCreatingOrg(false)}>
          <form onSubmit={handleCreateOrg} className="space-y-4">
            <div className="space-y-1.5">
              <label className="label">Workspace Name</label>
              <input
                className="input w-full"
                value={newOrgName}
                onChange={(e) => setNewOrgName(e.target.value)}
                placeholder="e.g. Acme Corporation"
                autoFocus
              />
            </div>
            <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
              <Button type="button" variant="ghost" onClick={() => setCreatingOrg(false)}>
                Cancel
              </Button>
              <Button type="submit" variant="primary" disabled={!newOrgName.trim()}>
                Create & Switch
              </Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

// ─── TopBar ───────────────────────────────────────────────────────────────────

function TopBar({
  areas,
  activeArea,
  settingsActive,
  orgs,
  activeOrgId,
  activeOrgName,
  user,
  showWorkspaceSwitcher,
  onSelectArea,
  onSwitchOrg,
  onCreateOrg,
  onOpenSettings,
  onOpenCommand,
  onLogout,
}: {
  areas: Area[];
  activeArea: AreaId;
  settingsActive: boolean;
  orgs: Org[];
  activeOrgId: number;
  activeOrgName: string;
  user: string;
  showWorkspaceSwitcher: boolean;
  onSelectArea: (id: AreaId) => void;
  onSwitchOrg: (id: number) => void;
  onCreateOrg: () => void;
  onOpenSettings: () => void;
  onOpenCommand: () => void;
  onLogout: () => void;
}) {
  const [wsOpen, setWsOpen] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const appName = useAppName();
  const { t, lang, setLang } = useTranslation();

  const initials = activeOrgName
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();
  const userInitials = user.slice(0, 2).toUpperCase();

  return (
    <header className="relative z-30 flex h-14 shrink-0 items-center gap-3 border-b border-white/[0.06] bg-[#07070b]/70 px-3 backdrop-blur-xl">
      {/* Brand */}
      <div className="flex items-center gap-2.5 pr-1">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
          <span className="font-display text-sm font-extrabold text-white">{brandInitial(appName)}</span>
        </div>
        <span className="hidden font-display text-[15px] font-bold tracking-wide text-white sm:block">{appName}</span>
      </div>

      {/* Workspace switcher — Pro only (multi-tenancy) */}
      {showWorkspaceSwitcher && (
      <div className="relative">
        <button
          onClick={() => setWsOpen((v) => !v)}
          aria-label={t("topbar.switchWorkspace")}
          className="flex h-9 items-center gap-2 rounded-xl bg-indigo-500/15 pl-1.5 pr-2 text-xs font-semibold text-indigo-300 ring-1 ring-inset ring-white/10 transition hover:ring-white/25"
        >
          <span className="flex h-6 w-6 items-center justify-center rounded-lg bg-indigo-500/25 text-[10px] font-bold text-indigo-300">
            {initials}
          </span>
          <span className="max-w-[130px] truncate text-sm font-medium text-white/90">{activeOrgName}</span>
          <ChevronsUpDown className="h-3.5 w-3.5 shrink-0 text-white/50" />
        </button>

        <AnimatePresence>
          {wsOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setWsOpen(false)} />
              <motion.div
                initial={{ opacity: 0, scale: 0.95, y: -4 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.95 }}
                transition={{ duration: 0.14 }}
                className="glass-strong absolute left-0 top-11 z-50 w-64 rounded-2xl p-1.5 shadow-2xl"
              >
                <p className="px-2 py-1.5 text-[11px] font-medium uppercase tracking-wide text-white/40">{t("topbar.workspaces")}</p>
                {orgs.map((o) => (
                  <button
                    key={o.id}
                    onClick={() => { onSwitchOrg(o.id); setWsOpen(false); }}
                    className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition hover:bg-white/5"
                  >
                    <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/15 text-[11px] font-semibold text-indigo-300 ring-1 ring-inset ring-white/10">
                      {o.name.slice(0, 2).toUpperCase()}
                    </span>
                    <span className="flex-1 truncate text-sm text-white">{o.name}</span>
                    {o.id === activeOrgId && <CheckIcon className="h-4 w-4 text-indigo-400" />}
                  </button>
                ))}
                <div className="my-1 h-px bg-white/[0.06]" />
                <button
                  onClick={() => { onCreateOrg(); setWsOpen(false); }}
                  className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-indigo-300 transition hover:bg-white/5"
                >
                  {t("topbar.newWorkspace")}
                </button>
              </motion.div>
            </>
          )}
        </AnimatePresence>
      </div>
      )}

      {/* Area tabs */}
      <nav className="ml-1 flex items-center gap-1 overflow-x-auto">
        {areas.map((a) => {
          const active = activeArea === a.id && !settingsActive;
          return (
            <button
              key={a.id}
              onClick={() => onSelectArea(a.id)}
              className={`relative flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                active ? "text-white" : "text-white/55 hover:text-white"
              }`}
            >
              {active && (
                <motion.span
                  layoutId="area-tab-active"
                  transition={{ type: "spring", stiffness: 500, damping: 40 }}
                  className="absolute inset-0 rounded-xl bg-white/[0.08] ring-1 ring-inset ring-white/10"
                />
              )}
              <a.Icon className="relative h-4 w-4" strokeWidth={1.75} />
              <span className="relative whitespace-nowrap">{t(`areas.${a.id}.title`, a.title)}</span>
            </button>
          );
        })}
      </nav>

      <div className="flex-1" />

      {/* Command palette trigger */}
      <button
        onClick={onOpenCommand}
        className="flex h-9 items-center gap-2 rounded-xl border border-white/[0.08] bg-white/[0.03] px-2.5 text-white/45 transition-colors hover:bg-white/[0.06] hover:text-white/70"
      >
        <Search className="h-4 w-4" />
        <span className="hidden text-xs md:block">{t("common.search")}</span>
        <kbd className="hidden rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-medium text-white/45 md:block">⌘K</kbd>
      </button>

      {/* Settings */}
      <button
        onClick={onOpenSettings}
        aria-label={t("topbar.settings")}
        title={t("topbar.settings")}
        className={`flex h-9 w-9 items-center justify-center rounded-xl transition-colors ${
          settingsActive ? "bg-white/[0.08] text-white ring-1 ring-inset ring-white/10" : "text-white/55 hover:bg-white/5 hover:text-white"
        }`}
      >
        <Settings className="h-5 w-5" strokeWidth={1.75} />
      </button>

      {/* User menu */}
      <div className="relative">
        <button
          onClick={() => setUserOpen((v) => !v)}
          aria-label={t("topbar.account")}
          className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15 transition hover:ring-white/30"
        >
          {userInitials}
        </button>

        <AnimatePresence>
          {userOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setUserOpen(false)} />
              <motion.div
                initial={{ opacity: 0, scale: 0.95, y: -4 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.95 }}
                transition={{ duration: 0.14 }}
                className="glass-strong absolute right-0 top-11 z-50 w-60 rounded-2xl p-1.5 shadow-2xl"
              >
                <div className="flex items-center gap-2.5 px-2 py-2">
                  <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15">
                    {userInitials}
                  </span>
                  <span className="min-w-0">
                    <span className="block truncate text-sm text-white">{user}</span>
                  </span>
                </div>
                <div className="my-1 h-px bg-white/[0.08]" />
                {[
                  { Icon: User, label: t("topbar.personalSettings"), path: "/personal" },
                  { Icon: CreditCard, label: t("topbar.billingPlan"), path: "/settings/billing" },
                ].map((m) => (
                  <NavLink
                    key={m.path}
                    to={m.path}
                    onClick={() => setUserOpen(false)}
                    className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-white/75 transition hover:bg-white/5 hover:text-white"
                  >
                    <m.Icon className="h-4 w-4" />
                    {m.label}
                  </NavLink>
                ))}
                <div className="my-1 h-px bg-white/[0.08]" />
                {/* Language switcher */}
                <div className="flex items-center gap-1 px-2 py-1.5">
                  <span className="mr-auto text-[11px] font-medium uppercase tracking-wide text-white/40">{t("common.language")}</span>
                  {LANGS.map((l) => (
                    <button
                      key={l.code}
                      onClick={() => setLang(l.code)}
                      className={`rounded-lg px-2 py-1 text-xs font-medium transition-colors ${
                        lang === l.code ? "bg-white/[0.1] text-white" : "text-white/50 hover:text-white"
                      }`}
                    >
                      {l.label}
                    </button>
                  ))}
                </div>
                <div className="my-1 h-px bg-white/[0.08]" />
                <button
                  onClick={onLogout}
                  className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-rose-300/90 transition hover:bg-rose-500/10"
                >
                  <LogOut className="h-4 w-4" />
                  {t("common.signOut")}
                </button>
              </motion.div>
            </>
          )}
        </AnimatePresence>
      </div>
    </header>
  );
}

// ─── CommandPalette ───────────────────────────────────────────────────────────

function CommandPalette({
  open,
  onClose,
  areas,
  onNavigate,
}: {
  open: boolean;
  onClose: () => void;
  areas: Area[];
  onNavigate: (path: string) => void;
}) {
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const { t } = useTranslation();

  // Flatten every nav item (areas + settings) into a flat, searchable list.
  // Labels are translated so search matches the language the user sees.
  const commands = useMemo(
    () =>
      [...areas, SETTINGS_AREA].flatMap((a) =>
        a.groups.flatMap((g) =>
          g.items.map((i) => ({
            id: i.path,
            label: t(`nav.${i.id}`, i.label),
            area: t(`areas.${a.id}.title`, a.title),
            group: t(`groups.${g.label}`, g.label),
            path: i.path,
            Icon: i.Icon,
            iconStr: i.iconStr,
          })),
        ),
      ),
    [areas, t],
  );

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return commands;
    return commands.filter(
      (c) =>
        c.label.toLowerCase().includes(needle) ||
        c.area.toLowerCase().includes(needle) ||
        c.group.toLowerCase().includes(needle) ||
        c.path.toLowerCase().includes(needle),
    );
  }, [q, commands]);

  useEffect(() => {
    if (open) {
      setQ("");
      setSel(0);
      const t = setTimeout(() => inputRef.current?.focus(), 20);
      return () => clearTimeout(t);
    }
  }, [open]);
  useEffect(() => { setSel(0); }, [q]);

  if (!open) return null;

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setSel((s) => Math.min(s + 1, filtered.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); const c = filtered[sel]; if (c) onNavigate(c.path); }
    else if (e.key === "Escape") { e.preventDefault(); onClose(); }
  };

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-black/50 px-4 pt-[12vh] backdrop-blur-sm"
      onClick={onClose}
    >
      <motion.div
        initial={{ opacity: 0, scale: 0.98, y: -8 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.14 }}
        className="glass-strong w-full max-w-xl overflow-hidden rounded-2xl shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-white/[0.08] px-4">
          <Search className="h-4 w-4 shrink-0 text-white/40" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onKey}
            placeholder={t("command.placeholder")}
            className="w-full bg-transparent py-3.5 text-sm text-white placeholder:text-white/35 focus:outline-none"
          />
          <kbd className="shrink-0 rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-medium text-white/40">esc</kbd>
        </div>
        <div className="max-h-[50vh] overflow-y-auto p-1.5">
          {filtered.length === 0 ? (
            <div className="px-3 py-8 text-center text-sm text-white/40">{t("command.empty", { q })}</div>
          ) : (
            filtered.map((c, i) => (
              <button
                key={c.id}
                onMouseEnter={() => setSel(i)}
                onClick={() => onNavigate(c.path)}
                className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors ${
                  i === sel ? "bg-white/[0.08]" : "hover:bg-white/[0.04]"
                }`}
              >
                {c.iconStr ? (
                  <span className="w-4 text-center text-sm">{c.iconStr}</span>
                ) : (
                  <c.Icon className="h-4 w-4 shrink-0 text-white/60" strokeWidth={1.75} />
                )}
                <span className="flex-1 truncate text-sm text-white">{c.label}</span>
                <span className="shrink-0 text-[11px] text-white/35">{c.area} · {c.group}</span>
              </button>
            ))
          )}
        </div>
      </motion.div>
    </div>
  );
}

// ─── AreaPanel ────────────────────────────────────────────────────────────────

function AreaPanel({ area, currentPath, onCollapse }: { area: Area; currentPath: string; onCollapse: () => void }) {
  const { t } = useTranslation();
  return (
    <motion.div
      initial={{ opacity: 0, x: -10 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: -10 }}
      transition={{ duration: 0.2 }}
      className="relative z-20 flex h-full w-60 flex-col border-r border-white/[0.06] bg-[#0c0c12]/40 backdrop-blur-xl"
    >
      {/* Header */}
      <div className="flex items-start justify-between gap-2 px-4 pb-3 pt-4">
        <div className="min-w-0">
          <h2 className="font-display text-[17px] font-bold tracking-tight text-white truncate">{t(`areas.${area.id}.title`, area.title)}</h2>
          <p className="text-[12px] text-white/45 truncate">{t(`areas.${area.id}.subtitle`, area.subtitle)}</p>
        </div>
        <button
          onClick={onCollapse}
          title="Collapse menu"
          className="mt-0.5 shrink-0 rounded-lg p-1.5 text-white/40 transition-colors hover:bg-white/[0.06] hover:text-white"
        >
          <PanelLeft className="h-4 w-4" strokeWidth={1.75} />
        </button>
      </div>

      {/* Grouped nav */}
      <div className="flex-1 overflow-y-auto px-3 pb-3">
        {area.groups.map((group) => (
          <div key={group.label} className="mb-4">
            <p className="px-2 pb-1.5 pt-1 text-[11px] font-semibold uppercase tracking-wider text-white/30">
              {t(`groups.${group.label}`, group.label)}
            </p>
            <div className="space-y-0.5">
              {group.items.map((item) => {
                const active = currentPath.startsWith(item.path);
                return (
                  <NavLink
                    key={item.id}
                    to={item.path}
                    className={`group relative flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-[13px] transition-colors ${
                      active ? "text-white" : "text-white/65 hover:text-white"
                    }`}
                  >
                    {active && (
                      <motion.span
                        layoutId="panel-active"
                        transition={{ type: "spring", stiffness: 500, damping: 40 }}
                        className="absolute inset-0 rounded-xl bg-white/[0.07] ring-1 ring-inset ring-white/10"
                      />
                    )}
                    {item.iconStr ? (
                      <span className={`relative text-sm ${active ? "text-indigo-300" : ""}`}>
                        {item.iconStr}
                      </span>
                    ) : (
                      <item.Icon
                        className={`relative h-[18px] w-[18px] ${active ? "text-indigo-300" : ""}`}
                        strokeWidth={1.75}
                      />
                    )}
                    <span className="relative flex-1">{t(`nav.${item.id}`, item.label)}</span>
                    {item.badge !== undefined && (
                      <span className="relative text-[11px] font-medium text-white/35">
                        {item.badge}
                      </span>
                    )}
                  </NavLink>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </motion.div>
  );
}

// ─── Login ────────────────────────────────────────────────────────────────────

function Login({ onLogin }: { onLogin: (u: string, orgId: number) => void }) {
  const [u, setU] = useState("admin");
  const [p, setP] = useState("");
  const [code, setCode] = useState("");
  const [needs2FA, setNeeds2FA] = useState(false);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [oauthConfig, setOauthConfig] = useState<{ googleEnabled: boolean; githubEnabled: boolean; registrationEnabled: boolean } | null>(null);
  const appName = useAppName();

  useEffect(() => {
    api.authConfig()
      .then(setOauthConfig)
      .catch(() => setOauthConfig({ googleEnabled: false, githubEnabled: false, registrationEnabled: false }));
  }, []);

  async function finishLogin(username: string) {
    const m = await api.me();
    onLogin(username, m.orgId);
  }

  async function doSubmit() {
    if (busy) return;
    setBusy(true);
    setErr("");
    try {
      if (mode === "register") {
        await api.register(u.trim(), p);
        await finishLogin(u.trim());
        return;
      }
      if (needs2FA) {
        await api.verify2FA(u, p, code.trim());
        await finishLogin(u);
        return;
      }
      const res = await api.login(u, p);
      if (res.twoFactorRequired) {
        setNeeds2FA(true);
        return;
      }
      await finishLogin(u);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : mode === "register" ? "sign up failed" : "login failed");
    } finally {
      setBusy(false);
    }
  }

  function switchMode(next: "login" | "register") {
    setMode(next);
    setErr("");
    setNeeds2FA(false);
    setCode("");
    setU(next === "register" ? "" : "admin");
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    doSubmit();
  }

  function onEnter(e: React.KeyboardEvent) {
    if (e.key === "Enter") { e.preventDefault(); doSubmit(); }
  }

  const hasOauth = oauthConfig && (oauthConfig.googleEnabled || oauthConfig.githubEnabled);

  return (
    <div className="led-aurora grid h-full place-items-center p-4">
      <div className="glass-strong w-full max-w-md rounded-2xl p-8 relative overflow-hidden shadow-glow">
        <div className="absolute top-0 right-0 h-32 w-32 bg-indigo-500/5 blur-3xl rounded-full pointer-events-none" />
        <div className="absolute -bottom-10 -left-10 h-32 w-32 bg-violet-500/5 blur-3xl rounded-full pointer-events-none" />
        
        <div className="mb-6 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
            <span className="font-display text-xl font-extrabold text-white">{brandInitial(appName)}</span>
          </div>
          <h1 className="font-display text-2xl font-bold text-white">{mode === "register" ? "Create your account" : `Sign in to ${appName}`}</h1>
          <p className="text-xs text-white/40 mt-1.5 leading-relaxed">{mode === "register" ? "Sign up to spin up your own workspace" : "Enter your credentials to access the operator workspace"}</p>
        </div>

        {err && (
          <div className="mb-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs flex gap-2 items-center">
            <ShieldAlert className="h-4 w-4 shrink-0" />
            <span>{err}</span>
          </div>
        )}

        <form onSubmit={submit} className="space-y-4">
          <div>
            <label className="label" htmlFor="login-username">{mode === "register" ? "Email" : "Username"}</label>
            <input
              id="login-username"
              type={mode === "register" ? "email" : "text"}
              name={mode === "register" ? "email" : "username"}
              className="input animate-none"
              value={u}
              onChange={(e) => setU(e.target.value)}
              onKeyDown={onEnter}
              autoComplete={mode === "register" ? "email" : "username"}
              placeholder={mode === "register" ? "you@domain.com" : "admin@domain.com"}
            />
          </div>

          <div>
            <label className="label" htmlFor="login-password">Password</label>
            <input
              id="login-password"
              type="password"
              name="password"
              className="input animate-none"
              value={p}
              onChange={(e) => setP(e.target.value)}
              onKeyDown={onEnter}
              autoComplete={mode === "register" ? "new-password" : "current-password"}
              autoFocus={!needs2FA}
              placeholder={mode === "register" ? "At least 8 characters" : "••••••••"}
            />
          </div>

          {needs2FA && (
            <div>
              <label className="label" htmlFor="login-otp">Authentication code</label>
              <input
                id="login-otp"
                name="otp"
                className="input animate-none"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                onKeyDown={onEnter}
                placeholder="6-digit code or recovery code"
                autoComplete="one-time-code"
                autoFocus
              />
            </div>
          )}

          <button type="submit" className="btn-primary w-full py-2.5 mt-2" disabled={busy}>
            {busy ? (mode === "register" ? "Creating..." : "Signing in...") : mode === "register" ? "Create account" : needs2FA ? "Verify OTP" : "Sign In"}
          </button>
        </form>

        {oauthConfig?.registrationEnabled && !needs2FA && (
          <p className="mt-5 text-center text-xs text-white/40">
            {mode === "register" ? (
              <>Already have an account?{" "}
                <button type="button" onClick={() => switchMode("login")} className="text-indigo-300 hover:underline font-medium">Sign in</button>
              </>
            ) : (
              <>Don't have an account?{" "}
                <button type="button" onClick={() => switchMode("register")} className="text-indigo-300 hover:underline font-medium">Create one</button>
              </>
            )}
          </p>
        )}

        {hasOauth && (
          <div className="mt-6 space-y-3">
            <div className="flex items-center gap-2 text-xs text-white/30">
              <span className="h-px flex-1 bg-white/10" />
              <span>or continue with</span>
              <span className="h-px flex-1 bg-white/10" />
            </div>
            <div className="grid grid-cols-1 gap-2">
              {oauthConfig.googleEnabled && (
                <a
                  href="/auth/begin/google"
                  className="flex items-center justify-center gap-2 rounded-xl border border-white/10 px-3 py-2.5 text-sm text-white/70 hover:bg-white/5 transition-colors font-medium"
                >
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
                    <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
                    <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z" fill="#FBBC05"/>
                    <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
                  </svg>
                  <span>Google</span>
                </a>
              )}
              {oauthConfig.githubEnabled && (
                <a
                  href="/auth/begin/github"
                  className="flex items-center justify-center gap-2 rounded-xl border border-white/10 px-3 py-2.5 text-sm text-white/70 hover:bg-white/5 transition-colors font-medium"
                >
                  <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/>
                  </svg>
                  <span>GitHub</span>
                </a>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}


function ComingSoonPage({ title, description }: { title: string; description: string }) {
  return (
    <ScreenWrap>
      <PageHeader title={title} description={description} />
      <GlassCard className="flex flex-col items-center justify-center py-20 px-6 text-center">
        <div className="h-16 w-16 rounded-2xl bg-indigo-500/10 flex items-center justify-center text-indigo-400 mb-4 animate-pulse">
          <Boxes className="h-8 w-8" />
        </div>
        <h3 className="text-lg font-bold text-white mb-2">Workspace Asset Integration Coming Soon</h3>
        <p className="text-sm text-white/50 max-w-sm leading-relaxed">
          We are currently building the direct connector client for this asset category. Look forward to direct API sync in the next update.
        </p>
      </GlassCard>
    </ScreenWrap>
  );
}
