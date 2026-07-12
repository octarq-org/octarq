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
          // AI Inbox is a Pro plugin now — its menu entry is injected dynamically
          // (@octarq-org/plugin-ai, category "Messaging") only in a composed build.
        ],
      },
    ],
  },
  {
    id: "commerce",
    title: "Commerce",
    subtitle: "Revenue, store & cost analysis",
    Icon: Wallet,
    // Every commerce feature is a Pro plugin. These group shells stay so the
    // dynamic plugin menus land in the right group/order (matched by label) in a
    // composed build; in the OSS build all groups are empty and the whole
    // Commerce area is dropped by the empty-area filter in App.tsx.
    groups: [
      // Storefront + Licenses → @octarq-org/plugin-storefront, @octarq-org/plugin-issuer (category "Sales").
      { label: "Sales", items: [] },
      // Billing → @octarq-org/plugin-billing (category "Billing").
      { label: "Billing", items: [] },
      // Bookkeeping → @octarq-org/plugin-finance (category "Finance").
      { label: "Finance", items: [] },
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
        // Servers + SSH Vault are Pro plugins (plugins/vps, plugins/ssh-keys,
        // category "Hosting") — injected dynamically only in a composed build.
        label: "Hosting",
        items: [],
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
          { id: "abuse",    label: "Abuse Reports",      Icon: ShieldAlert, path: "/abuse" },
        ],
      },
      {
        label: "System",
        items: [
          { id: "audit", label: "Audit Log", Icon: ScrollText, path: "/audit" },
        ],
      },
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
      label: "Subscriptions",
      items: [
        { id: "billing", label: "Billing & Plan", Icon: CreditCard, path: "/settings/billing" },
        { id: "license", label: "License",        Icon: KeyRound,   path: "/settings/license" },
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
// their Menus() — see docs/SIDEBAR-MENU.md.
export function areaForCategory(cat?: string, pluginAreas: UIArea[] = []): AreaId {
  const c = (cat ?? "").toLowerCase();
  const pluginHit = pluginAreas.find(
    (a) => a.id.toLowerCase() === c || a.title.toLowerCase() === c,
  );
  if (pluginHit) return pluginHit.id;
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute") || c.includes("hosting")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("compliance") || c.includes("governance") || c.includes("audit") || c.includes("abuse") || c.includes("system")) return "insights";
  if (c.includes("commerce") || c.includes("sell") || c.includes("sale") || c.includes("billing") || c.includes("storefront") || c.includes("license") || c.includes("finance")) return "commerce";
  return "operations";
}

// ─── Plugin-contributed areas ───────────────────────────────────────────────

// The contract keeps UIArea.icon a string key (icon-library-free); the app maps
// it to lucide here, mirroring how menu iconStr stays a string until render.
// Unknown/missing keys fall back to Puzzle — a plugin area never breaks on an
// unmapped icon.
const AREA_ICONS: Record<string, React.ElementType> = {
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

// Materialize a plugin-declared area (UIPlugin.areas) into the app's Area
// shape: an empty shell whose groups are filled by the same menu-merge pipeline
// (areaForCategory) as every other area; empty shells are dropped at runtime.
export function pluginAreaToArea(a: UIArea): Area {
  return {
    id: a.id,
    title: a.title,
    subtitle: a.subtitle ?? "",
    Icon: AREA_ICONS[(a.icon ?? "").toLowerCase()] ?? Puzzle,
    groups: [],
  };
}
