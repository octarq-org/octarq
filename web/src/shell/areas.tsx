import {
  Bell,
  Bot,
  Boxes,
  CreditCard,
  Database,
  Globe,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  LineChart,
  Link2,
  Mail,
  Puzzle,
  ScrollText,
  Server,
  Settings,
  Shield,
  ShieldAlert,
  Store,
  User,
  Users,
  Wallet,
  Webhook,
  Workflow,
} from "lucide-react";
import type { UIArea } from "@octarq-org/plugin-sdk";

// ─── Area definitions ──────────────────────────────────────────────────────

// Areas are data-driven: the built-in ids below come from STATIC_AREAS, and a
// plugin may contribute a NEW top-level area (UIPlugin.areas → uiAreas()), so
// the id space is open — plain string, with "settings" special-cased where it
// matters (areaForPath, App's selectArea). Adding a built-in area now means
// editing STATIC_AREAS (+ areaForCategory keywords if it should attract
// dynamic menus) — no separate union to keep in sync.
export type AreaId = string;

export interface NavItem {
  id: string;
  label: string;
  Icon: React.ElementType;
  iconStr?: string;
  path: string;
  badge?: string | number;
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export interface Area {
  id: AreaId;
  title: string;
  subtitle: string;
  Icon: React.ElementType;
  groups: NavGroup[];
}

export const STATIC_AREAS: Area[] = [
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
      // Links → core plugin (plugins/core/links.ts, category "Marketing").
      { label: "Marketing", items: [] },
      // Mail → core plugin (plugins/core/mail.ts, category "Messaging").
      // AI Inbox is a Pro plugin — its menu entry is injected dynamically
      // (@octarq-org/plugin-ai, category "Messaging") only in a composed build.
      { label: "Messaging", items: [] },
    ],
  },
  {
    id: "commerce",
    title: "Commerce",
    subtitle: "Revenue, store & cost analysis",
    Icon: Wallet,
    // Empty group shells matched by label so plugin menus land in the right
    // group/order when a build composes plugins that target this area. In the
    // community core all groups are empty and the whole Commerce area is
    // dropped by the empty-area filter in App.tsx.
    groups: [
      { label: "Sales", items: [] },
      { label: "Billing", items: [] },
      { label: "Finance", items: [] },
    ],
  },
  {
    id: "assets",
    title: "Infrastructure",
    subtitle: "Servers, network & databases",
    Icon: Boxes,
    groups: [
      // DNS → core plugin (plugins/core/domains.ts); Certificates → core
      // plugin (plugins/core/assets.ts). Both use category "Network".
      { label: "Network", items: [] },
      // Servers + SSH Vault are Pro plugins (@octarq-org/plugin-infra,
      // category "Hosting") — injected dynamically only in a composed build.
      { label: "Hosting", items: [] },
      // Databases + Object Storage → core plugin (plugins/core/assets.ts,
      // category "Storage & Databases").
      { label: "Storage & Databases", items: [] },
    ],
  },
  {
    id: "insights",
    title: "Security & Admin",
    subtitle: "Abuse defense & activity logs",
    Icon: ShieldAlert,
    groups: [
      // Abuse Reports → core plugin (plugins/core/abuse.ts, category "Security").
      { label: "Security", items: [] },
      // Audit Log → core plugin (plugins/core/audit.ts, category "System").
      { label: "System", items: [] },
    ],
  },
];

export const SETTINGS_AREA: Area = {
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
      label: "Instance",
      items: [
        { id: "instance", label: "Instance Settings", Icon: Server, path: "/settings/instance" },
      ],
    },
  ],
};

// The path→area mapping is DERIVED from the area definitions (single source of
// truth — the menu data), never a parallel hardcoded map. Callers that have the
// merged runtime areas (static + plugin areas + dynamic menu items — see
// App.tsx) pass them in so plugin-contributed paths resolve too; the default
// covers the static-only case.
export function areaForPath(path: string, areas: Area[] = STATIC_AREAS): AreaId {
  // Settings/personal live in their own area (SETTINGS_AREA), not the areas list.
  if (path.startsWith("/settings") || path.startsWith("/personal")) return "settings";
  const hit = areas
    .flatMap((a) => a.groups.flatMap((g) => g.items.map((i) => ({ prefix: i.path, area: a.id }))))
    .sort((x, y) => y.prefix.length - x.prefix.length) // longest prefix wins
    .find(({ prefix }) => path === prefix || path.startsWith(prefix + "/"));
  return hit?.area ?? "operations";
}

// Map a dynamic menu category to an area. A category naming a plugin-declared
// area (by id or title) lands there; otherwise the built-in keyword routing
// applies. Keep the keywords in sync with the Category strings plugins set in
// their Menus() — see docs/PLUGINS.md.
export function areaForCategory(cat?: string, pluginAreas: UIArea[] = []): AreaId {
  const c = (cat ?? "").toLowerCase();
  const pluginHit = pluginAreas.find(
    (a) => a.id.toLowerCase() === c || a.title.toLowerCase() === c,
  );
  if (pluginHit) return pluginHit.id;
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute") || c.includes("hosting") || c.includes("storage") || c.includes("database")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("compliance") || c.includes("governance") || c.includes("audit") || c.includes("abuse") || c.includes("security") || c.includes("system")) return "insights";
  if (c.includes("commerce") || c.includes("sell") || c.includes("sale") || c.includes("billing") || c.includes("storefront") || c.includes("license") || c.includes("finance")) return "commerce";
  return "operations";
}

// ─── Plugin-contributed icons & areas ───────────────────────────────────────

// The contract keeps plugin icons (UIArea.icon, PluginMenuItem.icon) as string
// keys so it stays icon-library-free; the app maps them to lucide HERE — the
// single icon-key→component table for both plugin areas and plugin menu items
// (core plugins use these keys too). A menu icon that isn't a known key is
// rendered literally (emoji); an unknown AREA icon falls back to Puzzle.
const PLUGIN_ICONS: Record<string, React.ElementType> = {
  bell: Bell,
  bot: Bot,
  boxes: Boxes,
  "credit-card": CreditCard,
  database: Database,
  globe: Globe,
  "hard-drive": HardDrive,
  "key-round": KeyRound,
  "layout-dashboard": LayoutDashboard,
  "line-chart": LineChart,
  "link-2": Link2,
  mail: Mail,
  puzzle: Puzzle,
  "scroll-text": ScrollText,
  server: Server,
  settings: Settings,
  shield: Shield,
  "shield-alert": ShieldAlert,
  store: Store,
  user: User,
  users: Users,
  wallet: Wallet,
  webhook: Webhook,
  workflow: Workflow,
};

// Resolve a plugin menu icon key to its lucide component, or undefined when
// the string isn't a known key (then the sidebar renders it as literal text /
// emoji via NavItem.iconStr — the pre-existing dynamic-menu behavior).
export function menuIcon(key?: string): React.ElementType | undefined {
  return key ? PLUGIN_ICONS[key.toLowerCase()] : undefined;
}

// Materialize a plugin-declared area (UIPlugin.areas) into the app's Area
// shape: an empty shell whose groups are filled by the same menu-merge pipeline
// (areaForCategory) as every other area; empty shells are dropped at runtime.
export function pluginAreaToArea(a: UIArea): Area {
  return {
    id: a.id,
    title: a.title,
    subtitle: a.subtitle ?? "",
    Icon: menuIcon(a.icon) ?? Puzzle,
    groups: [],
  };
}
