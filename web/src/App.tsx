import { useEffect, useRef, useState } from "react";
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
  Sliders,
  Bell,
  Users,
  Database,
  HardDrive,
  Shield,
  Store,
  PanelLeft,
  Webhook,
} from "lucide-react";
import { api, ApiError, MenuItem, Org } from "./api";
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
import { Modal, Button, ScreenWrap, PageHeader, GlassCard } from "./ui";

// ─── Area definitions ──────────────────────────────────────────────────────

type AreaId = "operations" | "assets" | "insights" | "settings";

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
    title: "Operations",
    subtitle: "Run your day-to-day services",
    Icon: Workflow,
    groups: [
      {
        label: "Analytics",
        items: [
          { id: "overview", label: "Overview", Icon: LayoutDashboard, path: "/overview" },
        ],
      },
      {
        label: "Reach",
        items: [
          { id: "links",   label: "Short Links", Icon: Link2,  path: "/links" },
          { id: "mail",    label: "Mailbox",      Icon: Mail,   path: "/mail" },
          { id: "inbox-ai", label: "Inbox AI",    Icon: Bot,    path: "/inbox-ai" },
        ],
      },
      {
        label: "Selling",
        items: [
          { id: "storefront", label: "Storefront", Icon: Store,      path: "/storefront" },
          { id: "licenses",   label: "Licenses",   Icon: KeyRound,   path: "/licenses" },
          { id: "billing",    label: "Billing",    Icon: CreditCard, path: "/billing" },
        ],
      },
    ],
  },
  {
    id: "assets",
    title: "Assets",
    subtitle: "Infrastructure you own",
    Icon: Boxes,
    groups: [
      {
        label: "Network",
        items: [
          { id: "domains", label: "Domains",   Icon: Globe,    path: "/domains" },
          { id: "certs",   label: "Certificates", Icon: Shield,  path: "/assets/certificates" },
        ],
      },
      {
        label: "Compute",
        items: [
          { id: "vps",     label: "VPS",       Icon: Server,   path: "/vps" },
          { id: "sshkeys", label: "SSH Keys",  Icon: KeyRound, path: "/sshkeys" },
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
    title: "Compliance",
    subtitle: "Workspace governance & finance",
    Icon: ShieldAlert,
    groups: [
      {
        label: "Finance",
        items: [
          { id: "finance",  label: "Finance",   Icon: Wallet,      path: "/finance" },
        ],
      },
      {
        label: "Governance",
        items: [
          { id: "abuse",    label: "Abuse",      Icon: ShieldAlert, path: "/abuse" },
          { id: "audit",    label: "Audit Log",  Icon: ScrollText,  path: "/audit" },
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
        { id: "general", label: "General", Icon: Settings, path: "/settings/general" },
        { id: "members", label: "Workspace Members", Icon: Users, path: "/settings/members" },
      ],
    },
    {
      label: "Services",
      items: [
        { id: "links", label: "Short Links", Icon: Link2, path: "/settings/links" },
        { id: "mail", label: "Mailboxes", Icon: Mail, path: "/settings/mail" },
      ],
    },
    {
      label: "Plan & licensing",
      items: [
        { id: "billing", label: "Billing & Plan", Icon: CreditCard, path: "/settings/billing" },
        { id: "license", label: "License", Icon: KeyRound, path: "/settings/license" },
        { id: "llm", label: "LLM Providers", Icon: Bot, path: "/settings/llm" },
      ],
    },
    {
      label: "Integrations",
      items: [
        { id: "providers", label: "DNS Providers", Icon: Globe, path: "/settings/providers" },
        { id: "smtp", label: "SMTP Senders", Icon: Mail, path: "/settings/smtp" },
        { id: "signin", label: "Sign-in (OAuth)", Icon: KeyRound, path: "/settings/signin" },
        { id: "webhooks", label: "Webhooks", Icon: Webhook, path: "/settings/webhooks" },
        { id: "notifications", label: "Alert Hooks", Icon: Bell, path: "/settings/notifications" },
      ],
    },
    {
      label: "Personal",
      items: [
        { id: "profile", label: "My Profile", Icon: User, path: "/personal/profile" },
        { id: "tokens", label: "API Tokens", Icon: KeyRound, path: "/personal/tokens" },
        { id: "menu", label: "Sidebar Menu", Icon: Sliders, path: "/personal/menu" },
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
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("finance") || c.includes("business") || c.includes("compliance") || c.includes("governance")) return "insights";
  return "operations"; // includes commerce/selling
}

// ─── App ──────────────────────────────────────────────────────────────────────

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);

  useEffect(() => {
    api.me()
      .then((m) => { setUser(m.username); setActiveOrgId(m.orgId); setAuthed(true); })
      .catch(() => setAuthed(false));
  }, []);

  if (authed === null) {
    return (
      <div className="led-aurora grid h-full place-items-center text-white/40">
        <div className="flex flex-col items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow flex items-center justify-center">
            <span className="font-display text-base font-extrabold text-white">L</span>
          </div>
          <span className="text-sm">loading…</span>
        </div>
      </div>
    );
  }

  if (!authed) {
    return (
      <Login
        onLogin={(u, orgId) => { setUser(u); setActiveOrgId(orgId); setAuthed(true); }}
      />
    );
  }

  return (
    <Shell
      user={user}
      activeOrgId={activeOrgId}
      setActiveOrgId={setActiveOrgId}
      onLogout={() => setAuthed(false)}
    />
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

  const [areas, setAreas] = useState<Area[]>(STATIC_AREAS);
  const [orgs, setOrgs]   = useState<Org[]>([]);
  const [creatingOrg, setCreatingOrg] = useState(false);
  const [newOrgName, setNewOrgName]   = useState("");

  const settingsActive = location.pathname.startsWith("/settings") || location.pathname.startsWith("/personal");
  const activeArea: AreaId = settingsActive ? "settings" : areaForPath(location.pathname);

  // Load orgs + dynamic menus + user settings layout
  useEffect(() => {
    api.orgs().catch(() => []).then((os) => setOrgs(os as Org[]));

    const MASTER_MENU_ITEMS: Record<string, { label: string; Icon: React.ElementType; path: string; iconStr?: string }> = {
      overview: { label: "Overview", Icon: LayoutDashboard, path: "/overview" },
      links: { label: "Short Links", Icon: Link2, path: "/links" },
      mail: { label: "Mailbox", Icon: Mail, path: "/mail" },
      "inbox-ai": { label: "Inbox AI", Icon: Bot, path: "/inbox-ai" },
      domains: { label: "Domains", Icon: Globe, path: "/domains" },
      certs: { label: "Certificates", Icon: Shield, path: "/assets/certificates" },
      vps: { label: "VPS", Icon: Server, path: "/vps" },
      sshkeys: { label: "SSH Keys", Icon: KeyRound, path: "/sshkeys" },
      databases: { label: "Databases", Icon: Database, path: "/assets/databases" },
      storage: { label: "Object Storage", Icon: HardDrive, path: "/assets/storage" },
      finance: { label: "Finance", Icon: Wallet, path: "/finance" },
      abuse: { label: "Abuse", Icon: ShieldAlert, path: "/abuse" },
      audit: { label: "Audit Log", Icon: ScrollText, path: "/audit" },
    };

    Promise.all([
      api.menus(),
      api.getUserSettings().catch(() => ({})),
    ]).then(([menus, settings]: [MenuItem[], any]) => {
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

      let customLayout: any = null;
      if (settings.menu_layout) {
        try {
          customLayout = JSON.parse(settings.menu_layout);
        } catch (e) {
          console.error("Failed to parse custom menu layout:", e);
        }
      }

      if (customLayout && typeof customLayout === "object") {
        // Construct areas according to user settings
        const nextAreas = STATIC_AREAS.map((staticArea) => {
          const areaLayout = customLayout[staticArea.id];
          if (!areaLayout || !Array.isArray(areaLayout.groups)) {
            // If area is not customized, append unassigned plugins belonging to this area to Plugins group
            const staticPaths = new Set(staticArea.groups.flatMap(g => g.items.map(i => i.path)));
            const extras = menus.filter(m => !staticPaths.has(m.path) && areaForCategory(m.category) === staticArea.id);
            if (!extras.length) return staticArea;
            return {
              ...staticArea,
              groups: [
                ...staticArea.groups,
                {
                  label: "More",
                  items: extras.map(m => ({
                    id: m.id,
                    label: m.label,
                    Icon: Globe,
                    iconStr: m.icon,
                    path: m.path,
                  })),
                },
              ],
            };
          }

          // Build groups for this area
          const groups = areaLayout.groups.map((g: any) => {
            const items = (g.items || [])
              .map((itemId: string) => ({ id: itemId, ...catalog[itemId] }))
              .filter((item: any) => item.path); // keep only valid items present in catalog
            return {
              label: g.name,
              items: items as NavItem[],
            };
          }).filter((g: any) => g.items.length > 0);

          // Append any unassigned items belonging to this area
          const layoutItemIds = new Set(areaLayout.groups.flatMap((g: any) => g.items || []));
          const unassignedItems = Object.keys(catalog)
            .filter((id) => {
              const item = catalog[id];
              // Decide area for this item
              const targetAreaId = areaForPath(item.path);
              return targetAreaId === staticArea.id && !layoutItemIds.has(id);
            })
            .map((id) => ({ id, ...catalog[id] }));

          if (unassignedItems.length > 0) {
            groups.push({
              label: "Uncategorized",
              items: unassignedItems as NavItem[],
            });
          }

          return {
            ...staticArea,
            groups,
          };
        });
        setAreas(nextAreas);
      } else {
        // Fallback: static areas + append any dynamic plugins to the "More" group
        const staticPaths = new Set(STATIC_AREAS.flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => i.path))));
        const extras = menus.filter((m) => !staticPaths.has(m.path));
        
        setAreas(
          STATIC_AREAS.map((area) => {
            const extraItems = extras
              .filter((m) => areaForCategory(m.category) === area.id)
              .map((m) => ({
                id: m.id,
                label: m.label,
                Icon: Globe,
                iconStr: m.icon,
                path: m.path,
              }));
            if (!extraItems.length) return area;
            return {
              ...area,
              groups: [
                ...area.groups,
                { label: "More", items: extraItems },
              ],
            };
          })
        );
      }
    }).catch(() => {});
  }, [activeOrgId]);

  const currentArea = settingsActive ? SETTINGS_AREA : (areas.find((a) => a.id === activeArea) ?? areas[0]);
  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name ?? "Personal Workspace";

  function handleCreateOrg(e: React.FormEvent) {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    api.createOrg({ name: newOrgName })
      .then((org) => api.switchOrg(org.id).then(() => window.location.reload()))
      .catch((e) => alert(e.message || "Failed to create organization"));
  }

  return (
    <div className="led-aurora flex h-screen w-full overflow-hidden text-white">
      <IconRail
        areas={areas}
        activeArea={activeArea}
        settingsActive={settingsActive}
        orgs={orgs}
        activeOrgId={activeOrgId}
        activeOrgName={activeOrgName}
        user={user}
        onSelectArea={(id) => {
          const area = areas.find((a) => a.id === id)!;
          const firstPath = area.groups[0]?.items[0]?.path ?? "/overview";
          navigate(firstPath);
        }}
        onSwitchOrg={(id) =>
          api.switchOrg(id).then(() => { setActiveOrgId(id); window.location.reload(); })
        }
        onCreateOrg={() => setCreatingOrg(true)}
        onOpenSettings={() => navigate("/settings")}
        onLogout={onLogout}
      />

      <AnimatePresence mode="wait">
        <AreaPanel
          key={activeArea}
          area={currentArea}
          currentPath={location.pathname}
        />
      </AnimatePresence>

      <main className="relative flex-1 overflow-hidden">
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
              <Route path="*"           element={<Navigate to="/overview" replace />} />
            </Routes>
          </div>
        </div>
      </main>

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

// ─── IconRail ─────────────────────────────────────────────────────────────────

function RailButton({
  active,
  onClick,
  children,
  label,
  expanded,
}: {
  active?: boolean;
  onClick?: () => void;
  children: React.ReactNode;
  label: string;
  expanded?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      aria-label={label}
      title={expanded ? undefined : label}
      className={`group relative flex h-11 items-center rounded-xl text-white/55 transition-colors hover:text-white ${
        expanded ? "w-48 justify-start gap-3 px-3" : "w-11 justify-center"
      }`}
    >
      {active && (
        <motion.span
          layoutId="rail-active"
          transition={{ type: "spring", stiffness: 500, damping: 38 }}
          className="absolute inset-0 rounded-xl bg-white/[0.08] ring-1 ring-inset ring-white/10"
        />
      )}
      {active && (
        <span className="absolute -left-2 top-1/2 h-5 w-1 -translate-y-1/2 rounded-full bg-indigo-400" />
      )}
      <span className={`relative flex items-center ${active ? "text-white" : ""}`}>{children}</span>
      {expanded && (
        <span className={`relative whitespace-nowrap text-sm font-medium ${active ? "text-white" : ""}`}>{label}</span>
      )}
    </button>
  );
}

function IconRail({
  areas,
  activeArea,
  settingsActive,
  orgs,
  activeOrgId,
  activeOrgName,
  user,
  onSelectArea,
  onSwitchOrg,
  onCreateOrg,
  onOpenSettings,
  onLogout,
}: {
  areas: Area[];
  activeArea: AreaId;
  settingsActive: boolean;
  orgs: Org[];
  activeOrgId: number;
  activeOrgName: string;
  user: string;
  onSelectArea: (id: AreaId) => void;
  onSwitchOrg: (id: number) => void;
  onCreateOrg: () => void;
  onOpenSettings: () => void;
  onLogout: () => void;
}) {
  const [wsOpen, setWsOpen] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  // Pin the rail open to show top-level menu names; persisted across sessions.
  const [expanded, setExpanded] = useState(() => {
    try { return localStorage.getItem("rail_expanded") === "1"; } catch { return false; }
  });
  const toggleExpanded = () => {
    setExpanded((v) => {
      const next = !v;
      try { localStorage.setItem("rail_expanded", next ? "1" : "0"); } catch { /* ignore */ }
      return next;
    });
  };

  const initials = activeOrgName
    .split(/\s+/)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

  const userInitials = user.slice(0, 2).toUpperCase();

  return (
    <div className={`relative z-30 flex h-full flex-col items-center border-r border-white/[0.06] bg-[#07070b]/60 py-3 backdrop-blur-xl transition-[width] duration-200 ${expanded ? "w-56" : "w-16"}`}>
      {/* Logo */}
      <div className={`mb-3 flex items-center transition-all ${expanded ? "w-48 gap-3 px-1" : "h-10 w-10 justify-center"}`}>
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
          <span className="font-display text-base font-extrabold text-white">L</span>
        </div>
        {expanded && (
          <span className="font-display text-lg font-bold text-white tracking-wide">led</span>
        )}
      </div>

      {/* Workspace switcher */}
      <div className={`relative mb-4 ${expanded ? "w-48" : "w-10"}`}>
        <button
          onClick={() => setWsOpen((v) => !v)}
          aria-label="Switch workspace"
          className={`relative flex h-10 items-center rounded-xl text-xs font-semibold text-indigo-300 ring-1 ring-inset ring-white/10 transition hover:ring-white/25 bg-indigo-500/15 ${
            expanded ? "w-48 justify-start gap-3 pl-2 pr-2.5" : "w-10 justify-center"
          }`}
        >
          {expanded ? (
            <>
              <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-indigo-500/25 text-[10px] font-bold text-indigo-300">
                {initials}
              </span>
              <span className="flex-1 truncate text-sm font-medium text-white/90 text-left">
                {activeOrgName}
              </span>
              <ChevronsUpDown className="h-4 w-4 shrink-0 text-white/50" />
            </>
          ) : (
            <>
              {initials}
              <ChevronsUpDown className="absolute -bottom-1 -right-1 h-3 w-3 rounded bg-[#0c0c12] p-px text-white/50" />
            </>
          )}
        </button>

        <AnimatePresence>
          {wsOpen && (
            <>
              <div className="fixed inset-0 z-40" onClick={() => setWsOpen(false)} />
              <motion.div
                initial={{ opacity: 0, scale: 0.95, x: -4 }}
                animate={{ opacity: 1, scale: 1, x: 0 }}
                exit={{ opacity: 0, scale: 0.95 }}
                transition={{ duration: 0.14 }}
                className={`glass-strong absolute top-0 z-50 w-64 rounded-2xl p-1.5 shadow-2xl ${expanded ? "left-[216px]" : "left-12"}`}
              >
                <p className="px-2 py-1.5 text-[11px] font-medium uppercase tracking-wide text-white/40">
                  Workspaces
                </p>
                {orgs.map((o) => (
                  <button
                    key={o.id}
                    onClick={() => { onSwitchOrg(o.id); setWsOpen(false); }}
                    className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition hover:bg-white/5"
                  >
                    <span className="flex h-8 w-8 items-center justify-center rounded-lg text-[11px] font-semibold ring-1 ring-inset ring-white/10 bg-indigo-500/15 text-indigo-300">
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
                  + New workspace
                </button>
              </motion.div>
            </>
          )}
        </AnimatePresence>
      </div>

      <div className={`mb-4 h-px bg-white/10 transition-all ${expanded ? "w-48" : "w-7"}`} />

      {/* Area nav */}
      <nav className="flex flex-1 flex-col items-center gap-1.5">
        {areas.map((a) => (
          <RailButton
            key={a.id}
            label={a.title}
            expanded={expanded}
            active={activeArea === a.id && !settingsActive}
            onClick={() => onSelectArea(a.id)}
          >
            <a.Icon className="h-5 w-5" strokeWidth={1.75} />
          </RailButton>
        ))}
      </nav>

      {/* Bottom: collapse toggle + settings + avatar */}
      <div className="flex flex-col items-center gap-1.5">
        <RailButton label={expanded ? "Collapse" : "Expand"} expanded={expanded} onClick={toggleExpanded}>
          <PanelLeft className={`h-5 w-5 transition-transform ${expanded ? "" : "rotate-180"}`} strokeWidth={1.75} />
        </RailButton>
        <RailButton label="Settings" expanded={expanded} active={settingsActive} onClick={onOpenSettings}>
          <Settings className="h-5 w-5" strokeWidth={1.75} />
        </RailButton>

        <div className={`relative mt-1 ${expanded ? "w-48" : ""}`}>
          <button
            onClick={() => setUserOpen((v) => !v)}
            aria-label="Account menu"
            className={`flex items-center transition ${
              expanded
                ? "h-11 w-48 justify-start gap-3 rounded-xl hover:bg-white/5 text-white pl-1"
                : "h-9 w-9 justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15 hover:ring-white/30"
            }`}
          >
            {expanded ? (
              <>
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-indigo-400/30 to-violet-400/30 text-xs font-semibold text-white ring-1 ring-inset ring-white/15">
                  {userInitials}
                </span>
                <span className="flex-1 min-w-0 text-left">
                  <span className="block truncate text-sm font-medium text-white/90">{user}</span>
                </span>
              </>
            ) : (
              userInitials
            )}
          </button>

          <AnimatePresence>
            {userOpen && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setUserOpen(false)} />
                <motion.div
                  initial={{ opacity: 0, scale: 0.95, y: 6 }}
                  animate={{ opacity: 1, scale: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.95 }}
                  transition={{ duration: 0.14 }}
                  className={`glass-strong absolute bottom-0 z-50 w-60 rounded-2xl p-1.5 shadow-2xl ${
                    expanded ? "left-[216px]" : "left-12"
                  }`}
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
                    { Icon: User,       label: "Personal settings", path: "/personal" },
                    { Icon: CreditCard, label: "Billing & plan",    path: "/settings/billing" },
                  ].map((m) => (
                    <NavLink
                      key={m.label}
                      to={m.path}
                      onClick={() => setUserOpen(false)}
                      className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-white/75 transition hover:bg-white/5 hover:text-white"
                    >
                      <m.Icon className="h-4 w-4" />
                      {m.label}
                    </NavLink>
                  ))}
                  <div className="my-1 h-px bg-white/[0.08]" />
                  <button
                    onClick={onLogout}
                    className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left text-sm text-rose-300/90 transition hover:bg-rose-500/10"
                  >
                    <LogOut className="h-4 w-4" />
                    Sign out
                  </button>
                </motion.div>
              </>
            )}
          </AnimatePresence>
        </div>
      </div>
    </div>
  );
}

// ─── AreaPanel ────────────────────────────────────────────────────────────────

function AreaPanel({ area, currentPath }: { area: Area; currentPath: string }) {
  return (
    <motion.div
      initial={{ opacity: 0, x: -10 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: -10 }}
      transition={{ duration: 0.2 }}
      className="relative z-20 flex h-full w-60 flex-col border-r border-white/[0.06] bg-[#0c0c12]/40 backdrop-blur-xl"
    >
      {/* Header */}
      <div className="px-4 pb-3 pt-4">
        <h2 className="font-display text-[17px] font-bold tracking-tight text-white">{area.title}</h2>
        <p className="text-[12px] text-white/45">{area.subtitle}</p>
      </div>

      {/* Grouped nav */}
      <div className="flex-1 overflow-y-auto px-3 pb-3">
        {area.groups.map((group) => (
          <div key={group.label} className="mb-4">
            <p className="px-2 pb-1.5 pt-1 text-[11px] font-semibold uppercase tracking-wider text-white/30">
              {group.label}
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
                    <span className="relative flex-1">{item.label}</span>
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
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await api.login(u, p);
      const m = await api.me();
      onLogin(u, m.orgId);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : "login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="led-aurora grid h-full place-items-center">
      <form onSubmit={submit} className="glass-strong w-80 rounded-2xl p-6">
        <div className="mb-6 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-violet-500 shadow-glow">
            <span className="font-display text-xl font-extrabold text-white">L</span>
          </div>
          <h1 className="font-display text-lg font-semibold text-white">Sign in to led</h1>
        </div>

        <label className="label">Username</label>
        <input className="input mb-3" value={u} onChange={(e) => setU(e.target.value)} />

        <label className="label">Password</label>
        <input
          type="password"
          className="input mb-4"
          value={p}
          onChange={(e) => setP(e.target.value)}
          autoFocus
        />

        {err && <p className="mb-3 text-sm text-rose-400">{err}</p>}

        <button className="btn-primary w-full" disabled={busy}>
          {busy ? "…" : "Sign in"}
        </button>

        <div className="mt-4 flex flex-col gap-2">
          <div className="flex items-center gap-2 text-xs text-white/30">
            <span className="h-px flex-1 bg-white/10" />
            or
            <span className="h-px flex-1 bg-white/10" />
          </div>
          <a
            href="/auth/begin/google"
            className="flex items-center justify-center gap-2 rounded-xl border border-white/10 px-3 py-2 text-sm text-white/70 hover:bg-white/5 transition-colors"
          >
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
              <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
              <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
              <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z" fill="#FBBC05"/>
              <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
            </svg>
            Continue with Google
          </a>
          <a
            href="/auth/begin/github"
            className="flex items-center justify-center gap-2 rounded-xl border border-white/10 px-3 py-2 text-sm text-white/70 hover:bg-white/5 transition-colors"
          >
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/>
            </svg>
            Continue with GitHub
          </a>
        </div>
      </form>
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
